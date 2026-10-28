// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// Class store-shared-across-vcenters: the store is named for the normalized
// origin of the effective base URL, with or without a credential, and no run
// falls back to an unscoped data.db.
func TestStoreScopeFollowsTheNormalizedOrigin(t *testing.T) {
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	dir := t.TempDir()
	// An unscoped store from an earlier build: no origin can be read off it.
	if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := func(credential, base string) string {
		t.Helper()
		setDefaultDBScopeIdentity(credential, credential, base)
		got := defaultDBPathInDir(dir)
		if filepath.Base(got) == "data.db" {
			t.Errorf("credential %q at %q opened the unscoped data.db", credential, base)
		}
		return got
	}
	for _, credential := range []string{"", "Basic cm86cHc="} {
		same := [][]string{
			{"https://127.0.0.1", "https://127.0.0.1:443", "https://127.0.0.1/", "HTTPS://127.0.0.1:443/sdk"},
			{"https://VC01.Example.com", "https://vc01.example.com:443/"},
			{"http://vc01.example.com", "http://vc01.example.com:80"},
		}
		for _, spellings := range same {
			want := at(credential, spellings[0])
			for _, s := range spellings[1:] {
				if got := at(credential, s); got != want {
					t.Errorf("credential %q: %s and %s name one vCenter but two stores (%s, %s)", credential, spellings[0], s, want, got)
				}
			}
		}
		distinct := []string{
			"https://vc01.example.com", "https://vc02.example.com", "https://vc01.example.com:8443",
			"http://vc01.example.com", "not a url", "",
		}
		seen := map[string]string{}
		for _, base := range distinct {
			got := at(credential, base)
			if prev, ok := seen[got]; ok && !(base == "" && prev == "not a url") {
				t.Errorf("credential %q: %q and %q share the store %s", credential, prev, base, got)
			}
			seen[got] = base
		}
	}
	if at("", "https://vc01.example.com") == at("Basic cm86cHc=", "https://vc01.example.com") {
		t.Error("a run with no credential shares the credentialed store")
	}
}
