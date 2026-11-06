// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored test (handfixes.json store-scope-names-the-instance): a config
// that fails to load says nothing about which instance this run is for, so the
// run reads no instance's mirror. The real binary, from the review's input.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"immybot-pp-cli/internal/store"
)

func TestAConfigThatFailsToLoadReadsNoInstancesMirror(t *testing.T) {
	cli, _ := buildE2EBinaries(t)
	stub := newStubImmy(t)
	e2eHome(t, stub)
	t.Setenv("IMMYBOT_BASE_URL", "")
	t.Setenv("IMMYBOT_TOKEN_URL", "")
	mirror := t.TempDir()
	s, err := store.Open(filepath.Join(mirror, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	row := json.RawMessage(`{"id":7,"name":"ALPHA-DC","tenantId":11,"tenant":"Alpha","online":true}`)
	if err := s.Upsert("computers", "7", row); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(bad, []byte("base_url = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMMYBOT_SUBDOMAIN", "beta")
	t.Setenv("IMMYBOT_DATA_DIR", mirror)
	out := e2eRun(t, cli, "", "--config", bad, "--json", "--no-learn", "computer-dossier", "7")
	if strings.Contains(out, "ALPHA-DC") {
		t.Fatalf("a beta run with an unreadable config answered from alpha's mirror:\n%s", out)
	}
}
