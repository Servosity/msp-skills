//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// restrictPrivateFile is a no-op off Windows: Chmod already sets the mode
// bits VerifyCredsPerms checks (handfixes.json: private-file-dacl-before-write).
func restrictPrivateFile(string) error { return nil }
