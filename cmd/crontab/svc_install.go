// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dvgamerr-app/crontab-windows/app"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func exePath() (string, error) {
	var err error
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		var fi os.FileInfo

		p += ".exe"
		fi, err = os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		}
	}
	return "", err
}

func installOrUpdateService(name, desc string, opts app.Options) (bool, error) {
	opts = normalizeInstallOptions(opts)
	exepath, err := exePath()
	if err != nil {
		return false, err
	}
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err == nil {
		defer s.Close()
		return false, s.UpdateConfig(updateServiceConfig(desc, exepath, opts))
	}
	s, err = m.CreateService(
		name,
		exepath,
		createServiceConfig(desc),
		serviceArgs(opts)...,
	)
	if err != nil {
		return false, err
	}
	defer s.Close()
	return true, nil
}

func installAndStartService(name, desc string, opts app.Options) error {
	opts = normalizeInstallOptions(opts)
	logger, err := app.NewLogger(opts)
	if err != nil {
		return err
	}
	defer logger.Close()

	logger.Infof("install requested service=%q crontab=%q home=%q log=%q", name, opts.CrontabPath, opts.HomeDir, opts.LogPath)
	installed, err := installOrUpdateService(name, desc, opts)
	if err != nil {
		err = explainServiceError(err)
		logger.Errorf("service install/update failed service=%q error=%v", name, err)
		return err
	}
	if installed {
		logger.Infof("service installed service=%q start_type=automatic", name)
	} else {
		logger.Infof("service already installed; config updated service=%q start_type=automatic", name)
	}

	if err := startService(name); err != nil {
		err = explainServiceError(err)
		logger.Errorf("service start failed service=%q error=%v", name, err)
		return err
	}
	state, err := queryServiceState(name)
	if err != nil {
		err = explainServiceError(err)
		logger.Errorf("service status check failed service=%q error=%v", name, err)
		return err
	}
	if state != svc.Running {
		err := fmt.Errorf("service %s started but state is %d", name, state)
		logger.Errorf("service start verification failed service=%q state=%d", name, state)
		return err
	}
	logger.Infof("service started service=%q state=running", name)
	return nil
}

func createServiceConfig(desc string) mgr.Config {
	return mgr.Config{
		DisplayName:  desc,
		Description:  desc,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorIgnore,
	}
}

func updateServiceConfig(desc, exepath string, opts app.Options) mgr.Config {
	return mgr.Config{
		ServiceType:    windows.SERVICE_NO_CHANGE,
		StartType:      mgr.StartAutomatic,
		ErrorControl:   windows.SERVICE_NO_CHANGE,
		DisplayName:    desc,
		Description:    desc,
		BinaryPathName: serviceBinaryPath(exepath, opts),
	}
}

func serviceArgs(opts app.Options) []string {
	return []string{
		"service",
		"--home", opts.HomeDir,
		"--file", opts.CrontabPath,
		"--log", opts.LogPath,
	}
}

func serviceBinaryPath(exepath string, opts app.Options) string {
	args := append([]string{exepath}, serviceArgs(opts)...)
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, syscall.EscapeArg(arg))
	}
	return strings.Join(quoted, " ")
}

func normalizeInstallOptions(opts app.Options) app.Options {
	defaults := app.DefaultOptions()
	if opts.HomeDir == "" {
		opts.HomeDir = defaults.HomeDir
	}
	if opts.CrontabPath == "" {
		opts.CrontabPath = app.CrontabPathForHome(opts.HomeDir)
	}
	if opts.LogPath == "" {
		opts.LogPath = defaults.LogPath
	}
	return opts
}

func removeService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return explainServiceError(fmt.Errorf("service %s is not installed: %w", name, err))
	}
	defer s.Close()
	status, err := s.Query()
	if err == nil && status.State != svc.Stopped {
		_, _ = s.Control(svc.Stop)
		if err := waitServiceState(s, svc.Stopped, 20*time.Second); err != nil {
			return explainServiceError(fmt.Errorf("could not stop service before delete: %w", err))
		}
	}
	err = s.Delete()
	if err != nil {
		return explainServiceError(err)
	}
	return nil
}

func explainServiceError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "marked for deletion") {
		return fmt.Errorf("%w; Windows is still deleting the old crontab service. Close Services/sc handles or reboot, then run crontab again", err)
	}
	if strings.Contains(msg, "disabled") {
		return fmt.Errorf("%w; run crontab from an Administrator shell so it can set the service back to automatic start", err)
	}
	return err
}
