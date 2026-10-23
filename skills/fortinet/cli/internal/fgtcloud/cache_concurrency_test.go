// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fgtcloud

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestWriteCacheConcurrentWritersAndReaders is two runs refreshing the
// FortiGate Cloud token at once while a third reads the cache. On Windows a
// rename over a file another process holds open fails with "Access is
// denied"; before writeCache used the shared retrying writer, that failure
// made saveCache delete the cache, refresh token included. Every write must
// succeed and every read must see one complete token.
// Hand-owned (handfixes.json: fgtcloud-token-cache-rename-retry).
func TestWriteCacheConcurrentWritersAndReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fortigate-cloud-token.json")
	if err := writeCache(path, &cachedToken{User: "seed", AccessToken: "seed"}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	const rounds, writers, reads = 25, 4, 8
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		for w := 0; w < writers; w++ {
			tok := &cachedToken{User: fmt.Sprintf("writer-%d", w), AccessToken: fmt.Sprintf("round-%d-writer-%d", round, w)}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := writeCache(path, tok); err != nil {
					t.Errorf("round %d: writeCache(%s): %v", round, tok.User, err)
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < reads; r++ {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("round %d: read cache: %v", round, err)
					return
				}
				var got cachedToken
				if err := json.Unmarshal(data, &got); err != nil || got.AccessToken == "" {
					t.Errorf("round %d: cache is not one complete token (%v): %q", round, err, data)
					return
				}
			}
		}()
		wg.Wait()
		if t.Failed() {
			return
		}
	}
}
