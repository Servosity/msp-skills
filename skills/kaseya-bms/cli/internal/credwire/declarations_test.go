// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"path/filepath"
	"testing"
)

// The kit reads a Files entry as a credential file and holds every string in
// nested JSON, so the profile store is never one: its credential values are
// held precisely at startup instead (holdProfileStore).
func TestProfileStoreIsNoKitCredentialFile(t *testing.T) {
	home := t.TempDir()
	for k, v := range map[string]string{"KASEYA_BMS_HOME": home, "HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"), "KASEYA_BMS_CONFIG": "", "KASEYA_BMS_CONFIG_DIR": ""} {
		t.Setenv(k, v)
	}
	other := t.TempDir()
	files := credentialFiles([]string{"--home", other, "--config", filepath.Join(other, "c.toml"), "profile", "list"})
	if len(files) == 0 {
		t.Fatal("no credential files at all; the case did not run")
	}
	for _, f := range files {
		if filepath.Base(f) == "profiles.json" {
			t.Errorf("credentialFiles returns the profile store %s to the kit as a credential file", f)
		}
	}
}
