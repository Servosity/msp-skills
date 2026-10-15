// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build windows

package main

// TEMPORARY - proof that the windows smoke catches a binary that cannot run
// on a stock Windows machine. Never merged. Code that builds paths from $HOME
// works on every Unix and on a CI runner whose shell exports HOME, and dies
// for a Windows user, who has no HOME.

import "os"

func init() {
	if os.Getenv("HOME") == "" {
		panic("HOME is not set")
	}
}
