// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !windows

package credkit

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// attachHeld passes r to the child as an extra file and names its fd.
func attachHeld(cmd *exec.Cmd, r *os.File) string {
	cmd.ExtraFiles = append(cmd.ExtraFiles, r)
	return "fd:" + strconv.Itoa(2+len(cmd.ExtraFiles))
}

func openHeld(spec string) *os.File {
	n, err := strconv.Atoi(strings.TrimPrefix(spec, "fd:"))
	if err != nil || !strings.HasPrefix(spec, "fd:") || n < 3 {
		return nil
	}
	return os.NewFile(uintptr(n), "credkit-held")
}

// setCmdLine: Unix has no command line.
func setCmdLine(*exec.Cmd, string) {}

// cmdLine is a command line that replaces argv: none on Unix.
func cmdLine(*exec.Cmd) []string { return nil }
