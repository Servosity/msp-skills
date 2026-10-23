// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"resourceguru-pp-cli/internal/cliutil"
	"resourceguru-pp-cli/internal/config"
)

// TestConcurrentCredentialWritersRepeatedRounds repeats the race in
// TestConcurrentCredentialWritersLeaveParseableCredentials with more writers
// and more rounds. On Windows a writer that renames its temp file over
// credentials.toml while another writer has that file open (reading it or
// replacing it) gets "Access is denied" or a sharing violation; the single
// two-writer round only hits that window now and then (unifi-network twice and
// resourceguru once in CI on 2026-10-23), so a missing rename retry passed most
// runs. Here every round must leave one complete writer's value and no writer
// may fail, so the unretried rename fails this test on most Windows runs.
// Hand-owned (handfixes.json: windows-credential-rename-retry).
func TestConcurrentCredentialWritersRepeatedRounds(t *testing.T) {
	home, _ := resetCredentialEnv(t)
	// os.UserHomeDir reads USERPROFILE on Windows; keep this test's writes in
	// its own temp home instead of the runner's real profile.
	t.Setenv("USERPROFILE", home)
	const rounds, writers = 25, 4
	for round := 0; round < rounds; round++ {
		want := map[string]bool{}
		var wg sync.WaitGroup
		for w := 0; w < writers; w++ {
			cfg := &config.Config{Path: filepath.Join(t.TempDir(), fmt.Sprintf("writer-%d.toml", w))}
			token := fmt.Sprintf("writer-%d-round-%d-secret", w, round)
			want[token] = true
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := saveConfigCredential(cfg, token); err != nil {
					t.Errorf("round %d: save credential %s: %v", round, token, err)
				}
			}()
		}
		wg.Wait()
		if t.Failed() {
			return
		}
		creds, ok, err := cliutil.LoadCredentials()
		if err != nil {
			t.Fatalf("round %d: LoadCredentials() error = %v", round, err)
		}
		if !ok {
			t.Fatalf("round %d: credentials file missing after concurrent writes", round)
		}
		if got := credentialValue(creds); !want[got] {
			t.Fatalf("round %d: credential value = %q, want one complete writer", round, got)
		}
	}
}
