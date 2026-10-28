//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "os"

// CreatePrivateTemp: os.CreateTemp already creates the file 0600 here. The
// Windows twin sets an owner-only DACL at creation (handfixes.json
// private-file-owner-only-dacl).
func CreatePrivateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}
