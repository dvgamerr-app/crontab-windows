// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows
// +build windows

package main

import (
	"context"

	"github.com/dvgamerr/crontab/app"
	"github.com/pkg/errors"
)

// This is the name you will use for the NET START command
const svcName = "crontab"

// This is the name that will appear in the Services control panel
const svcNameLong = "Crontab for Windows"

// This is assigned the full SHA1 hash from GIT
var sha1ver string

func svcLauncher(ctx context.Context) error {
	err := app.Run(ctx, svcName, sha1ver, serviceOptionsFromArgs())
	if err != nil {
		return errors.Wrap(err, "app.run")
	}

	return nil
}
