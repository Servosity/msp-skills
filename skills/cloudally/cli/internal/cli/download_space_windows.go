// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build windows

package cli

import (
	"fmt"
	"golang.org/x/sys/windows"
)

// freeBytes reports the free space available on the volume holding path.
func freeBytes(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(free), nil
}

// ensureSpace refuses to start a download that would fill the drive.
func ensureSpace(dest string, want int64) error {
	free, err := freeBytes(dest)
	if err != nil {
		// Not knowing is not a reason to stop; the write itself will fail
		// if the drive is full.
		return nil
	}
	need := want
	if need <= 0 {
		need = 0
	}
	if free-need < freeSpaceMargin {
		return fmt.Errorf("only %.2f GiB free on %s and the next archive needs about %.2f GiB; stopping rather than filling the drive",
			float64(free)/(1<<30), dest, float64(need)/(1<<30))
	}
	return nil
}
