// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package main

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("could not retrieve service status: %v", err)
	}
	if status.State == svc.Running {
		return nil
	}

	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return waitServiceState(s, svc.Running, 20*time.Second)
}

func controlService(name string, c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	return waitServiceState(s, to, 10*time.Second, status)
}

func queryServiceState(name string) (svc.State, error) {
	m, err := mgr.Connect()
	if err != nil {
		return svc.Stopped, err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return svc.Stopped, fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return svc.Stopped, fmt.Errorf("could not retrieve service status: %v", err)
	}
	return status.State, nil
}

func waitServiceState(s *mgr.Service, to svc.State, timeoutAfter time.Duration, initial ...svc.Status) error {
	var status svc.Status
	var err error
	if len(initial) > 0 {
		status = initial[0]
	} else {
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	timeout := time.Now().Add(timeoutAfter)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}
