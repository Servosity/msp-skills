// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the kit-reader
// pass's round 1 classes, from that review's failing inputs. A saved profile's
// credential values are held when the store is read. An MCP read or an
// unconfirmed preview changes no local file, in the MCP server process as in
// its CLI children.

package credwire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func dirsEnv(s sandbox) []string {
	return s.env(
		"KASEYA_VSA_CONFIG_DIR="+filepath.Join(s.home, "cfg"),
		"KASEYA_VSA_DATA_DIR="+filepath.Join(s.home, "data"),
		"KASEYA_VSA_STATE_DIR="+filepath.Join(s.home, "state"),
		"KASEYA_VSA_CACHE_DIR="+filepath.Join(s.home, "cache"),
		"KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3",
	)
}

func writeTree(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestKitSavedProfileCredentialNeverShown(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "ProfileSecretQ4821"
	for _, store := range []string{
		`{"profiles":{"p":{"name":"p","values":{"headers":"` + secret + `"}}}}`,
		// The first of two members: a parse that keeps one sees only the second.
		`{"profiles":{"p":{"name":"p","values":{"headers":"` + secret + `","headers":"x"}}}}`,
	} {
		s := newSandbox(t)
		writeTree(t, filepath.Join(s.home, "cfg", "profiles.json"), store)
		for _, args := range [][]string{{"profile", "show", "p", "--json"}, {"profile", "list", "--json"}} {
			res := run(t, cliBin, dirsEnv(s), args...)
			requireAbsent(t, strings.Join(args, " ")+" stdout", res.stdout, secret)
			requireAbsent(t, strings.Join(args, " ")+" stderr", res.stderr, secret)
		}
	}
}

func TestKitMCPReadAndPreviewChangeNoLocalFile(t *testing.T) {
	_, mcp := binaries(t)
	old := time.Now().Add(-time.Hour)

	t.Run("unconfirmed execute keeps an expired cache file", func(t *testing.T) {
		s := newSandbox(t)
		stale := filepath.Join(s.home, "cache", "http", "resources", "demo", "stale.json")
		writeTree(t, stale, "{}")
		if err := os.Chtimes(stale, old, old); err != nil {
			t.Fatal(err)
		}
		frames, _ := mcpCall(t, mcp, dirsEnv(s), "kaseya-vsa_execute", map[string]any{
			"endpoint_id": "groups.create", "params": map[string]any{"Name": "example"},
		})
		if !strings.Contains(frames, "confirmation_required") {
			t.Fatalf("no preview: %s", frames)
		}
		if _, err := os.Stat(stale); err != nil {
			t.Errorf("the preview removed %s: %v", stale, err)
		}
	})

	t.Run("sql writes no store owner marker", func(t *testing.T) {
		s := newSandbox(t)
		writeTree(t, filepath.Join(s.home, "data", "data.db"), "")
		env := append(dirsEnv(s), "KASEYA_VSA_TOKEN_ID=OwnerTokenIdK2231", "KASEYA_VSA_TOKEN_SECRET=OwnerTokenSecretK7740")
		before := snapshot(t, s.home)
		mcpCall(t, mcp, env, "sql", map[string]any{"query": "SELECT 1"})
		if changed := snapshotChanges(before, snapshot(t, s.home)); len(changed) > 0 {
			t.Errorf("sql changed %v", changed)
		}
	})

	t.Run("learnings warnings creates no directory", func(t *testing.T) {
		s := newSandbox(t)
		state := filepath.Join(s.home, "state")
		mcpCall(t, mcp, dirsEnv(s), "learnings_list", map[string]any{"warnings": true})
		if _, err := os.Stat(state); err == nil {
			t.Errorf("learnings_list created %s", state)
		}
	})
}
