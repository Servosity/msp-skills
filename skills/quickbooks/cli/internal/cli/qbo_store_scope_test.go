// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/config"
)

// Intuit access tokens expire after an hour and every refresh returns a new
// one, so the default local store must be named for the company (Intuit host
// plus realm), never for the token: otherwise every offline command after a
// refresh reads a new, empty store and still exits 0.
func TestQBOLocalStoreFollowsTheCompanyNotTheToken(t *testing.T) {
	testenv.Isolate(t)
	var requests int64
	inner := fakeQBOQuery(t, 5, &requests)
	defer inner.Close()
	company := regexp.MustCompile(`^/v3/company/[0-9]+`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = company.ReplaceAllString(r.URL.Path, "")
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("QUICKBOOKS_DB_PATH", "")

	run := func(token, realm string, args ...string) string {
		t.Helper()
		t.Setenv("QUICKBOOKS_BASE_URL", srv.URL+"/v3/company/"+realm)
		t.Setenv("QUICKBOOKS_ACCESS_TOKEN", token)
		root := newRootCmd(&rootFlags{})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs(append(args, "--json"))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v failed: %v\nstderr: %s", args, err, errBuf.String())
		}
		return out.String()
	}
	count := func(out string) int {
		t.Helper()
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		return len(rows)
	}

	run("access-token-one", "1001", "sync", "--resources", "customers", "--full")
	if n := count(run("access-token-one", "1001", "customers", "top", "--by", "balance", "--limit", "100")); n != 5 {
		t.Fatalf("same token: customers top read %d rows, want 5", n)
	}
	if n := count(run("access-token-two", "1001", "customers", "top", "--by", "balance", "--limit", "100")); n != 5 {
		t.Fatalf("after a token refresh: customers top read %d rows, want the 5 synced rows", n)
	}
	// A different company keeps its own store: QBO ids collide across realms.
	if n := count(run("access-token-two", "2002", "customers", "top", "--by", "balance", "--limit", "100")); n != 0 {
		t.Fatalf("another realm read %d rows from the first company's store", n)
	}
}

// Every released version kept one unscoped data.db. A first run after the
// upgrade that does not know the company yet (no realm, so the base URL is the
// placeholder) must read that file without claiming it for the token, so the
// next configured run still finds the books.
func TestQBOUpgradeKeepsTheReleasedStoreWhenTheFirstRunHasNoRealm(t *testing.T) {
	testenv.Isolate(t)
	var requests int64
	inner := fakeQBOQuery(t, 4, &requests)
	defer inner.Close()
	company := regexp.MustCompile(`^/v3/company/[0-9]+`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = company.ReplaceAllString(r.URL.Path, "")
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "data.db")
	exec := func(env map[string]string, args ...string) string {
		t.Helper()
		for k, v := range env {
			t.Setenv(k, v)
		}
		root := newRootCmd(&rootFlags{})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs(append(args, "--json"))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v failed: %v\nstderr: %s", args, err, errBuf.String())
		}
		return out.String()
	}
	rows := func(out string) int {
		t.Helper()
		var r []map[string]any
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		return len(r)
	}
	// The released layout: one data.db with no owner marker.
	exec(map[string]string{"QUICKBOOKS_DB_PATH": legacy, "QUICKBOOKS_BASE_URL": srv.URL + "/v3/company/1001", "QUICKBOOKS_ACCESS_TOKEN": "token-one"},
		"sync", "--resources", "customers", "--full")
	t.Setenv("QUICKBOOKS_DB_PATH", "")
	t.Setenv("QUICKBOOKS_BASE_URL", "")
	t.Setenv("QUICKBOOKS_REALM_ID", "")
	if n := rows(exec(map[string]string{"QUICKBOOKS_ACCESS_TOKEN": "token-two"}, "customers", "top", "--by", "balance")); n != 4 {
		t.Fatalf("first run without a realm read %d rows from the released store, want 4", n)
	}
	if _, err := os.Stat(legacy + ".owner"); err == nil {
		t.Fatalf("a run that does not know the company claimed the released store")
	}
	if n := rows(exec(map[string]string{"QUICKBOOKS_BASE_URL": srv.URL + "/v3/company/1001", "QUICKBOOKS_ACCESS_TOKEN": "token-three"}, "customers", "top", "--by", "balance")); n != 4 {
		t.Fatalf("configured run after the upgrade read %d rows, want the 4 released rows", n)
	}
}

// Codex next-batch review 3, round 1: a native Intuit base URL with no realm
// path (https://quickbooks.api.intuit.com) named a scope, so the first run
// claimed the released data.db for it and the corrected run opened an empty
// store (zero outstanding). The class: only a base URL that names a company
// (/v3/company/<realm>) names or claims a store.
func TestQBOOnlyABaseURLThatNamesACompanyClaimsTheReleasedStore(t *testing.T) {
	for _, base := range []string{
		"https://quickbooks.api.intuit.com",
		"https://sandbox-quickbooks.api.intuit.com/v3",
		"https://quickbooks.api.intuit.com/v3/company/",
		"https://quickbooks.api.intuit.com/v3/company/YOUR_REALM_ID",
		"http://127.0.0.1:8080",
	} {
		if got := config.StoreScopeCompany(&config.Config{BaseURL: base}); got != "" {
			t.Errorf("%s names the store %q", base, got)
		}
	}
	if got := config.StoreScopeCompany(&config.Config{BaseURL: "https://quickbooks.api.intuit.com/v3/company/1001"}); got == "" {
		t.Error("a base URL with a realm names no store")
	}

	testenv.Isolate(t)
	var requests int64
	inner := fakeQBOQuery(t, 4, &requests)
	defer inner.Close()
	company := regexp.MustCompile(`^/v3/company/[0-9]+`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = company.ReplaceAllString(r.URL.Path, "")
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "data.db")
	exec := func(base string, args ...string) int {
		t.Helper()
		t.Setenv("QUICKBOOKS_BASE_URL", base)
		t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "token-scope")
		root := newRootCmd(&rootFlags{})
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		root.SetArgs(append(args, "--json"))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v failed: %v\nstderr: %s", args, err, errBuf.String())
		}
		var rows []map[string]any
		_ = json.Unmarshal(out.Bytes(), &rows)
		return len(rows)
	}
	t.Setenv("QUICKBOOKS_DB_PATH", legacy)
	exec(srv.URL+"/v3/company/1001", "sync", "--resources", "customers", "--full")
	t.Setenv("QUICKBOOKS_DB_PATH", "")
	t.Setenv("QUICKBOOKS_REALM_ID", "")
	exec("https://quickbooks.api.intuit.com", "customers", "top", "--by", "balance")
	if _, err := os.Stat(legacy + ".owner"); err == nil {
		t.Fatal("a base URL with no realm claimed the released store")
	}
	if n := exec(srv.URL+"/v3/company/1001", "customers", "top", "--by", "balance"); n != 4 {
		t.Fatalf("the corrected run read %d rows, want the 4 released rows", n)
	}
}
