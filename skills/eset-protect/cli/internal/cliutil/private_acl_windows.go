//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"golang.org/x/sys/windows"
)

// restrictPrivateFile (handfixes.json: private-file-dacl-before-write) gives
// path a protected DACL that grants only the current user, before any secret
// is written to it. Chmod does not touch NTFS ACLs, so a file created under a
// directory that grants Users read access inherited that access, and the
// tokens were readable until (and after) VerifyCredsPerms refused them.
func restrictPrivateFile(path string) error {
	me, err := currentUserSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + me + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
