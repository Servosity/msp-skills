// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "secret-fields-never-persisted" entry.

package client

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResponseCacheNeverStoresSecretFields: the HTTP response cache wrote
// every GET answer to disk; an answer carrying a credential-named field is
// not cached at all, while an ordinary answer still is.
func TestResponseCacheNeverStoresSecretFields(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-7731"
	dir := t.TempDir()
	c := &Client{BaseURL: "https://nas.test:5001", cacheDir: dir}
	c.writeCache("/webapi/entry.cgi?api=SYNO.Backup.Task&method=list&version=1", nil, json.RawMessage(`{"task_list":[{"name":"x","repo_password":"`+sentinel+`"}]}`))
	c.writeCache("/webapi/entry.cgi?api=SYNO.Core.Package&method=list&version=2", nil, json.RawMessage(`{"packages":[{"id":"ActiveBackup"}]}`))
	cached := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), sentinel) {
			t.Fatalf("%s carries the secret", p)
		}
		if strings.Contains(string(b), "ActiveBackup") {
			cached++
		}
		return nil
	})
	if cached != 1 {
		t.Fatalf("an answer without secrets was not cached (%d)", cached)
	}
}
