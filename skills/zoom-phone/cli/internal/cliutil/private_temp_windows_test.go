//go:build windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

// shareDirWithEveryone gives dir an inheritable ACE granting Everyone read,
// the shape of C:\Users\Public (codex pass-4 round-1 input).
func shareDirWithEveryone(t *testing.T, dir string) {
	t.Helper()
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

// A credential file written under a directory other users can read is still
// private: the inherited Everyone ACE never reaches it.
func TestAtomicWriteCredentialFile_PublicDirectoryStaysPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "zoom-review")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	shareDirWithEveryone(t, dir)
	path := filepath.Join(dir, "accounts", "acme", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteCredentialFile(path, []byte("client_secret = \"x\"\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCredsPerms(path); err != nil {
		t.Fatalf("credential file under a shared directory is not private: %v", err)
	}
	if err := AtomicWritePrivateFile(filepath.Join(dir, "other.json"), []byte("{}"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCredsPerms(filepath.Join(dir, "other.json")); err != nil {
		t.Fatalf("private file under a shared directory is not private: %v", err)
	}
}

func TestSecurityAttributesSize(t *testing.T) {
	if got := reflect.TypeOf(windows.SecurityAttributes{}).Size(); got != securityAttributesSize {
		t.Fatalf("securityAttributesSize = %d, struct is %d", securityAttributesSize, got)
	}
}
