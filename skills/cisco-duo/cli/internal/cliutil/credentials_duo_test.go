// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"path/filepath"
	"testing"
)

// A pinned subaccount moves the data dir to <base>/subaccounts/<id>; the
// parent's saved credentials must still be found at the base.
func TestCredentialsPathStaysAtParentBaseWhenPinned(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CISCO_DUO_DATA_DIR", filepath.Join(base, "subaccounts", "DAAAAAAAAAAAAAAAAAAA"))
	t.Setenv("PP_CISCO_DUO_PARENT_DATA_DIR", base)
	got, err := credentialsPath()
	if err != nil || got != filepath.Join(base, credentialsFileName) {
		t.Fatalf("pinned credentials path = %q, %v; want the parent base", got, err)
	}
	t.Setenv("PP_CISCO_DUO_PARENT_DATA_DIR", "")
	t.Setenv("CISCO_DUO_DATA_DIR", base)
	if got, err := credentialsPath(); err != nil || got != filepath.Join(base, credentialsFileName) {
		t.Fatalf("unpinned credentials path = %q, %v", got, err)
	}
}
