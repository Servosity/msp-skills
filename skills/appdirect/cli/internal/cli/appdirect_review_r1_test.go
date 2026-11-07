// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"appdirect-pp-cli/internal/store"
)

// Codex r1 (action1 reprint on 4.34.2) class incomplete paging read as
// complete, on AppDirect's two documented list shapes: a billing list is a
// plain JSON array paged by offset (wire name start) and count
// (spec.yaml /billing/v1/subscriptions: count default 250), and an account v2
// list is a page (PagedCompanyAccountV2: content plus page.number, size,
// totalElements, totalPages) paged by page and size. The fake honors the
// paging parameters it is sent, and each list holds more records than one
// page, so sync must read every page to store them all.
func TestSyncReadsEveryPage(t *testing.T) {
	const subs, companies = 263, 121
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
			if id, _, ok := r.BasicAuth(); !ok || id != "paging-client-id" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			mints.Add(1)
			_, _ = io.WriteString(w, `{"access_token":"paging-minted-token-6c1e","expires_in":3600}`)
		case r.Header.Get("Authorization") != "Bearer paging-minted-token-6c1e":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/api/billing/v1/subscriptions":
			start, _ := strconv.Atoi(q.Get("start"))
			count := 250
			if c, err := strconv.Atoi(q.Get("count")); err == nil && c > 0 {
				count = c
			}
			items := []map[string]any{}
			for i := start; i < subs && i < start+count; i++ {
				items = append(items, map[string]any{"id": fmt.Sprintf("sub-%04d", i), "status": "ACTIVE"})
			}
			_ = json.NewEncoder(w).Encode(items)
		case r.URL.Path == "/api/account/v2/companies":
			page, _ := strconv.Atoi(q.Get("page"))
			size := 50
			if s, err := strconv.Atoi(q.Get("size")); err == nil && s > 0 {
				size = s
			}
			content := []map[string]any{}
			for i := page * size; i < companies && i < (page+1)*size; i++ {
				content = append(content, map[string]any{"uuid": fmt.Sprintf("co-%04d", i), "name": fmt.Sprintf("Company %d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"links": []any{}, "content": content,
				"page": map[string]any{"number": page, "size": size, "totalElements": companies, "totalPages": (companies + size - 1) / size}})
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"APPDIRECT_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"APPDIRECT_CONFIG": filepath.Join(dir, "config.toml"), "APPDIRECT_BASE_URL": srv.URL + "/api", "APPDIRECT_TOKEN_URL": "",
		"APPDIRECT_CLIENT_ID": "paging-client-id", "APPDIRECT_CLIENT_SECRET": "paging-client-secret-3b7d", "APPDIRECT_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	db := filepath.Join(dir, "billing.db")
	var out, errb bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--db", db, "--full", "--resources", "billing-v1-subscriptions,account-v2-companies", "--json"})
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
	for rt, want := range map[string]int{"billing-v1-subscriptions": subs, "account-v2-companies": companies} {
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
// store identity, as it reaches AppDirect: each reseller marketplace is its
// own tenant. Marketplace A was used with a pasted token (auth set-token) and
// synced; the operator then points the CLI at marketplace B with B's client
// pair. The default store is named by the token in effect when no client is
// configured and adopted into the client's store on the next run, so B's
// commands read A's records. B's local views must show only what B synced.
func TestStoreOfAnotherMarketplaceIsNeverAdopted(t *testing.T) {
	marketplace := func(tag string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
				_, _ = io.WriteString(w, `{"access_token":"Mt`+tag[len(tag)-1:]+`-k3Vq8Zp2Wx","expires_in":3600}`)
			case r.URL.Path == "/api/billing/v1/subscriptions" && r.URL.Query().Get("start") == "0":
				_, _ = io.WriteString(w, `[{"id":"sub-`+tag+`","status":"ACTIVE","creationDate":1760000000000,"company":{"id":"co-`+tag+`","name":"Company-`+tag+`"}}]`)
			default:
				_, _ = io.WriteString(w, `[]`)
			}
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	a, b := marketplace("MarketA"), marketplace("MarketB")
	dir := t.TempDir()
	for k, v := range map[string]string{"APPDIRECT_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "APPDIRECT_CONFIG": "",
		"APPDIRECT_TOKEN_URL": "", "APPDIRECT_CLIENT_ID": "", "APPDIRECT_CLIENT_SECRET": "", "APPDIRECT_NO_CONFIG_WRITE": ""} {
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
	t.Setenv("APPDIRECT_BASE_URL", a.URL+"/api")
	run("pTk-Zq81vLw0Yh3Nc6Rb\n", "auth", "set-token")
	if out := run("", "sync", "--resources", "billing-v1-subscriptions", "--json"); !strings.Contains(out, `"total_records": 1`) {
		t.Fatalf("marketplace A's sync stored nothing; the test did not run:\n%s", out)
	}
	t.Setenv("APPDIRECT_BASE_URL", b.URL+"/api")
	t.Setenv("APPDIRECT_CLIENT_ID", "cid-Wv6Ty3Pq")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "Sct-Bn4Rk7Vd2Qe")
	if out := run("", "subs", "changed", "--since", "3650d", "--json"); strings.Contains(out, "sub-MarketA") {
		t.Errorf("marketplace B's view reads marketplace A's synced subscription (A's store was adopted into B's):\n%s", out)
	}
	if out := run("", "sync", "--resources", "billing-v1-subscriptions", "--json"); !strings.Contains(out, `"errored": 0`) {
		t.Fatalf("marketplace B's sync failed:\n%s", out)
	}
	out := run("", "subs", "changed", "--since", "3650d", "--json")
	if !strings.Contains(out, "sub-MarketB") {
		t.Fatalf("marketplace B's view lost its own subscription; the control did not run:\n%s", out)
	}
	if strings.Contains(out, "sub-MarketA") {
		t.Errorf("marketplace B's view reads marketplace A's synced subscription:\n%s", out)
	}
}
