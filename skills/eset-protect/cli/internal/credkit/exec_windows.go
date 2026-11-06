// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// attachHeld passes r's handle to the child and names it.
func attachHeld(cmd *exec.Cmd, r *os.File) string {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(r.Fd()))
	return "handle:" + strconv.FormatUint(uint64(r.Fd()), 10)
}

func openHeld(spec string) *os.File {
	n, err := strconv.ParseUint(strings.TrimPrefix(spec, "handle:"), 10, 64)
	if err != nil || !strings.HasPrefix(spec, "handle:") || n == 0 {
		return nil
	}
	return os.NewFile(uintptr(n), "credkit-held")
}

// setCmdLine sets the command line the child gets.
func setCmdLine(cmd *exec.Cmd, line string) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = line
}

// cmdLine is the command line SysProcAttr sets in place of argv, if any.
func cmdLine(cmd *exec.Cmd) []string {
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.CmdLine != "" {
		return []string{cmd.SysProcAttr.CmdLine}
	}
	return nil
}
