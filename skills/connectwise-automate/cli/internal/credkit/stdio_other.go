// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !linux && !darwin && !windows

package credkit

import (
	"errors"
	"os"
)

// openNonblock: not used where the output layer is not built.
const openNonblock = 0

// openNofollow: not used where the output layer is not built.
const openNofollow = 0

// The output layer is built for Linux, macOS and Windows only. Elsewhere it
// cannot be installed, so Start exits before writing anything (contract 4).
func redirectStd(int) (orig, r *os.File, err error) {
	return nil, nil, errors.New("credkit: no output layer for this platform")
}

func releaseStd(int) {}

// fdAppends: a descriptor's own O_APPEND is not read on this platform;
// a file opened with O_APPEND through os is (appendMode).
func fdAppends(*os.File) bool { return false }

const fdFlagReadable = false

func fdAppendFlag(*os.File) (app, known bool) { return false, false }

// appendAt writes b at f's end, off: a descriptor's flags do not change
// after the open on this platform.
func appendAt(f *os.File, b []byte, _ int64) (int, error) { return f.Write(b) }

// retargetLog: the standard logger writes to fd 2 or to nothing here, and the
// redirect does not move it.
func retargetLog() {}

// quietRuntime has nothing to do: the output layer is not built here.
func quietRuntime() (resume func()) { return func() {} }
