//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// TestPrivateFileDACLBeforeWrite (handfixes.json: private-file-dacl-before-write):
// under a directory whose inheritable DACL grants Everyone read, a plainly
// written file inherits that access (the control fails VerifyCredsPerms),
// while AtomicWritePrivateFile's file must pass it.
func TestPrivateFileDACLBeforeWrite(t *testing.T) {
	me, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:PAI(A;OICI;FA;;;" + me + ")(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}

	control := filepath.Join(dir, "control.toml")
	if err := os.WriteFile(control, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if VerifyCredsPerms(control) == nil {
		t.Fatal("control: a file under an Everyone-readable directory passed VerifyCredsPerms, so this test proves nothing")
	}

	private := filepath.Join(dir, "credentials.toml")
	if err := AtomicWritePrivateFile(private, []byte("access_token = \"test-secret\"\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCredsPerms(private); err != nil {
		t.Fatalf("AtomicWritePrivateFile left an unsafe DACL: %v", err)
	}
}
