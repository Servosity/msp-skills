// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"acronis-pp-cli/internal/cliutil"
	"acronis-pp-cli/internal/cliutil/testenv"
)

// TestNoCollectionUnderACredentialKeyBecomesRows: a response whose rows sit
// under a credential-named key (tokens, credentials, secrets) never reaches
// disk, whichever path unwraps it: a live read's write-through (the Codex
// input: clients list against {"tokens":[...]}), a mutation answer, a
// map-keyed wrapper, and the sole-collection fallbacks of pagination and sync.
func TestNoCollectionUnderACredentialKeyBecomesRows(t *testing.T) {
	home := testenv.Isolate(t, cliutil.DataDir, cliutil.CacheDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/2/clients":
			fmt.Fprint(w, `{"tokens":[{"client_id":"c1","value":"test-secret-wrapper-aaaa"}]}`)
		case "/api/2/clients/c3":
			fmt.Fprint(w, `{"id":"c3","client_id":"ordinary-client-c3","tokens":[{"value":"test-secret-wrapper-bbbb"}]}`)
		default:
			t.Error("unexpected request", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ACRONIS_BASE_URL", srv.URL)
	t.Setenv("ACRONIS_BEARER_AUTH", "fixture")
	for _, args := range [][]string{{"list"}, {"get", "c3"}} {
		flags := &rootFlags{configPath: filepath.Join(home, "config.toml"), dataSource: "auto", asJSON: true}
		cmd := newClientsCmd(flags)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("clients %v: %v\n%s", args, err, out.String())
		}
	}
	ctx := context.Background()
	writeMutationResponseToStore(ctx, "clients", "/api/2/clients", json.RawMessage(`{"credentials":[{"client_id":"c4","value":"test-secret-wrapper-cccc"}]}`), "")
	writeThroughCache(ctx, "clients", json.RawMessage(`{"secrets":{"00000000-0000-0000-0000-0000000000a1":{"client_id":"c5","value":"test-secret-wrapper-dddd"},"00000000-0000-0000-0000-0000000000a2":{"client_id":"c6","value":"test-secret-wrapper-dddd"}}}`))

	sentinels := []string{"test-secret-wrapper-aaaa", "test-secret-wrapper-bbbb", "test-secret-wrapper-cccc", "test-secret-wrapper-dddd"}
	sawOrdinary := false
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		for _, s := range sentinels {
			if bytes.Contains(body, []byte(s)) {
				t.Errorf("%s holds %s", p, s)
			}
		}
		if bytes.Contains(body, []byte("ordinary-client-c3")) {
			sawOrdinary = true
		}
		return nil
	})
	if !sawOrdinary {
		t.Error("the ordinary client row was not stored; the check proves nothing")
	}

	page := map[string]json.RawMessage{"tokens": json.RawMessage(`[{"id":"a1","value":"test-secret-wrapper-eeee"}]`)}
	if items, ok := extractPaginatedItemsFromObject(page, "/api/agent_manager/v2/agents", true); ok {
		t.Errorf("pagination unwrapped a credential-named collection: %s", items)
	}
	if items, ok := extractItemsFromEnvelope(page); ok {
		t.Errorf("sync unwrapped a credential-named collection: %s", items)
	}
	mapped := map[string]json.RawMessage{"credentials": json.RawMessage(`{"00000000-0000-0000-0000-0000000000a1":{"id":"a2","value":"test-secret-wrapper-ffff"},"00000000-0000-0000-0000-0000000000a2":{"id":"a3","value":"test-secret-wrapper-ffff"}}`)}
	if items, ok := extractItemsFromEnvelope(mapped); ok {
		t.Errorf("sync flattened a credential-named map: %s", items)
	}
}
