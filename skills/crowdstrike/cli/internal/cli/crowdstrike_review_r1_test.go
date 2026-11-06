// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"crowdstrike-pp-cli/internal/store"
)

// Codex r1 (action1 reprint on 4.34.2) class incomplete paging read as
// complete, on CrowdStrike's documented list shape: a Falcon list answers
// {"meta":{"pagination":{"offset","limit","total"}},"resources":[...],
// "errors":[]} and pages by offset and limit. The fake honors the paging
// parameters it is sent, and each list holds more records than one page, so
// sync must read every page to store them all: the combined hosts list (records
// keyed device_id) and the combined prevention policies list.
func TestSyncReadsEveryPage(t *testing.T) {
	const hosts, policies = 263, 121
	var mints atomic.Int32
	page := func(w http.ResponseWriter, q url.Values, total int, rec func(i int) map[string]any) {
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit := 100
		if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
			limit = l
		}
		items := []map[string]any{}
		for i := offset; i < total && i < offset+limit; i++ {
			items = append(items, rec(i))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{"pagination": map[string]any{"offset": offset, "limit": limit, "total": total}},
			"resources": items, "errors": []any{}})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if form.Get("client_id") != "paging-client-id" || form.Get("client_secret") != "paging-client-secret-3b7d" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			mints.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"paging-minted-token-6c1e","token_type":"bearer","expires_in":1799}`)
		case r.Header.Get("Authorization") != "Bearer paging-minted-token-6c1e":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/devices/combined/devices/v1":
			page(w, q, hosts, func(i int) map[string]any {
				return map[string]any{"device_id": fmt.Sprintf("dev-%04d", i), "hostname": fmt.Sprintf("host-%d", i), "status": "normal"}
			})
		case r.URL.Path == "/policy/combined/prevention/v1":
			page(w, q, policies, func(i int) map[string]any {
				return map[string]any{"id": fmt.Sprintf("pol-%04d", i), "name": fmt.Sprintf("Policy %d", i), "platform_name": "Windows", "enabled": true}
			})
		default:
			_, _ = io.WriteString(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":0}},"resources":[],"errors":[]}`)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"CROWDSTRIKE_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"CROWDSTRIKE_CONFIG": filepath.Join(dir, "config.toml"), "CROWDSTRIKE_BASE_URL": srv.URL, "CROWDSTRIKE_TOKEN_URL": "",
		"FALCON_CLIENT_ID": "paging-client-id", "FALCON_CLIENT_SECRET": "paging-client-secret-3b7d", "CROWDSTRIKE_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	db := filepath.Join(dir, "falcon.db")
	var out, errb bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--db", db, "--full", "--resources", "devices,policy-combined-prevention-v1", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if mints.Load() == 0 {
		t.Fatalf("sync never minted; the fake answered nothing:\n%s%s", out.String(), errb.String())
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for rt, want := range map[string]int{"devices": hosts, "policy-combined-prevention-v1": policies} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: sync stored %d of %d records; it stopped before the last page:\n%s", rt, n, want, strings.TrimSpace(errb.String()))
		}
	}
}

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity, as it reaches CrowdStrike: each Falcon cloud (and each
// tenant on it) is its own tenant. Cloud A was used with a pasted token (auth
// set-token) and synced; the operator then points the CLI at cloud B with B's
// client pair. The default store is named by the token in effect when no
// client is configured and adopted into the client's store on the next run,
// so B's commands read A's records. B's local reads must show only what B
// synced.
func TestStoreOfAnotherCloudIsNeverAdopted(t *testing.T) {
	cloud := func(tag, minted string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
				_, _ = io.WriteString(w, `{"access_token":"`+minted+`","token_type":"bearer","expires_in":1799}`)
			case r.URL.Path == "/devices/combined/devices/v1" && (r.URL.Query().Get("offset") == "" || r.URL.Query().Get("offset") == "0"):
				_, _ = io.WriteString(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":1}},"resources":[{"device_id":"dev-`+tag+`","hostname":"Host-`+tag+`","status":"normal"}],"errors":[]}`)
			default:
				_, _ = io.WriteString(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":0}},"resources":[],"errors":[]}`)
			}
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	// The tokens share no text with the records, so no record repeats a held
	// value (a record that does is withheld from the store, not adopted).
	a, b := cloud("CloudA", "mintedFalconTokenA-6e2d91"), cloud("CloudB", "mintedFalconTokenB-3a8c47")
	dir := t.TempDir()
	for k, v := range map[string]string{"CROWDSTRIKE_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "CROWDSTRIKE_CONFIG": "",
		"CROWDSTRIKE_TOKEN_URL": "", "FALCON_CLIENT_ID": "", "FALCON_CLIENT_SECRET": "", "CROWDSTRIKE_NO_CONFIG_WRITE": ""} {
		t.Setenv(k, v)
	}
	run := func(stdin string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	// hostsInStore is every host the store a local read opens right now
	// holds: MCPStorePath scopes the default store by the credential in
	// effect, as every command does when it starts.
	hostsInStore := func() string {
		t.Helper()
		path, err := MCPStorePath()
		if err != nil {
			t.Fatal(err)
		}
		s, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		rows, err := s.DB().Query(`SELECT id FROM resources WHERE resource_type = 'devices'`)
		if err != nil {
			return ""
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		return strings.Join(ids, " ")
	}
	t.Setenv("CROWDSTRIKE_BASE_URL", a.URL)
	run("pastedFalconToken-7d3e91c2a4\n", "auth", "set-token")
	if out := run("", "sync", "--resources", "devices", "--json"); !strings.Contains(out, `"total_records": 1`) {
		t.Fatalf("cloud A's sync stored nothing; the test did not run:\n%s", out)
	}
	if got := hostsInStore(); got != "dev-CloudA" {
		t.Fatalf("cloud A's store holds %q, want its own host; the control did not run", got)
	}
	t.Setenv("CROWDSTRIKE_BASE_URL", b.URL)
	t.Setenv("FALCON_CLIENT_ID", "falconClientIdB-5b9e")
	t.Setenv("FALCON_CLIENT_SECRET", "falconClientSecretB-7c3e")
	if got := hostsInStore(); strings.Contains(got, "dev-CloudA") {
		t.Errorf("cloud B's local reads open a store holding cloud A's synced host (A's store was adopted into B's): %q", got)
	}
	if out := run("", "sync", "--resources", "devices", "--json"); !strings.Contains(out, `"errored": 0`) {
		t.Fatalf("cloud B's sync failed:\n%s", out)
	}
	got := hostsInStore()
	if !strings.Contains(got, "dev-CloudB") {
		t.Fatalf("cloud B's store lost its own host; the control did not run: %q", got)
	}
	if strings.Contains(got, "dev-CloudA") {
		t.Errorf("cloud B's store holds cloud A's synced host: %q", got)
	}
}
