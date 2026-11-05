// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "save-never-leaves-an-unverified-secret" and
// "explicit-config-logout-never-falls-back" entries.

package cliutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A credentials file that fails the private-file guard never keeps the
// secret on disk. Windows Chmod cannot restrict an inherited DACL, so the
// guard runs on the temporary file before the secret is written (the
// previous file stays untouched) and again on the published file (which is
// removed). The guard is injected so this runs on every OS.
func TestSaveCredentialsNeverLeavesAnUnverifiedSecret(t *testing.T) {
	isolateCredsHome(t)
	path, err := CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	const token = "R17-NEW-TOKEN-MUST-NOT-LAND"
	holds := func() string {
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, _ := os.ReadFile(filepath.Join(filepath.Dir(path), e.Name()))
			if strings.Contains(string(data), token) {
				return e.Name()
			}
		}
		return ""
	}
	orig := SavedCredentialsPermsCheck
	t.Cleanup(func() { SavedCredentialsPermsCheck = orig })
	refuseOn := func(n int) {
		calls := 0
		SavedCredentialsPermsCheck = func(p string) error {
			calls++
			if calls == n {
				return errors.New("token file DACL grants access to a broad group (BU); restrict it")
			}
			return orig(p)
		}
	}

	if err := SaveCredentialsAt(path, &Credentials{DnsfilterApiKey: "previous"}); err != nil {
		t.Fatal(err)
	}
	refuseOn(1)
	err = SaveCredentialsAt(path, &Credentials{DnsfilterApiKey: token})
	var permErr *CredentialsPermissionError
	if !errors.As(err, &permErr) || permErr.Path != path || !strings.Contains(err.Error(), "not saved") {
		t.Fatalf("temporary file refused: SaveCredentialsAt() = %v, want a 'not saved' permission refusal for %s", err, path)
	}
	if name := holds(); name != "" {
		t.Fatalf("temporary file refused: the secret is on disk in %s", name)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "previous") {
		t.Fatalf("temporary file refused: the previous credentials file changed: %q (%v)", data, err)
	}

	refuseOn(2)
	err = SaveCredentialsAt(path, &Credentials{DnsfilterApiKey: token})
	if !errors.As(err, &permErr) {
		t.Fatalf("published file refused: SaveCredentialsAt() = %v, want a permission refusal", err)
	}
	if name := holds(); name != "" {
		t.Fatalf("published file refused: the secret is on disk in %s", name)
	}
}

// The logout record holds no credential, reads as logged out, and stays
// recognisable when its access fails the guard (it is not a secret).
func TestCredentialsLogoutMarker(t *testing.T) {
	isolateCredsHome(t)
	resetCredsPermsWarnings()
	dir, err := DataDir() // inside the ACL-locked sandbox, so a saved key passes the guard on Windows
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tenant", "data", credentialsFileName)
	if err := WriteCredentialsLogoutMarker(path); err != nil {
		t.Fatal(err)
	}
	creds, status, err := loadCredentialsWithStatus(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !status.LoggedOut || creds.HasValues() {
		t.Fatalf("logout record: LoggedOut=%v HasValues=%v, want true/false", status.LoggedOut, creds.HasValues())
	}
	if has, err := CredentialsFileHasValuesAt(path); err != nil || has {
		t.Fatalf("CredentialsFileHasValuesAt(logout record) = %v, %v", has, err)
	}
	if err := SaveCredentialsAt(path, &Credentials{DnsfilterApiKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if _, status, _ := loadCredentialsWithStatus(path, true); status.LoggedOut {
		t.Fatal("a saved credential still reads as logged out")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := WriteCredentialsLogoutMarker(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, status, _ := loadCredentialsWithStatus(path, true); !status.LoggedOut || status.Refusal == nil {
		t.Fatalf("a loose logout record: LoggedOut=%v refusal=%v, want true and a refusal", status.LoggedOut, status.Refusal)
	}
}
