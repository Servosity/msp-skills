// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.

package client

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"acronis-pp-cli/internal/config"
)

const cacheCredentialSentinel = "CacheSecretSentinel7f3a"

// cachedBodies walks the response cache and returns each cached body.
func cachedBodies(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		out[path] = string(body)
		return nil
	})
	return out
}

// TestCredentialResponsesAreNotCached pins that a GET answer carrying a
// credential field (a client secret in a list, a password nested in a
// detail, anything from the identity provider) is never written to the
// on-disk response cache, while a plain answer, and an answer whose only
// "code" is a status word, still are.
func TestCredentialResponsesAreNotCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/2/clients":
			_, _ = w.Write([]byte(`{"items":[{"client_id":"listed-client","client_secret":"` + cacheCredentialSentinel + `","tenant_id":"t-1"}]}`))
		case "/api/2/clients/c-2":
			_, _ = w.Write([]byte(`{"client_id":"c-2","settings":{"label":"x","password":"` + cacheCredentialSentinel + `"}}`))
		case "/api/2/idp/token_info":
			_, _ = w.Write([]byte(`{"id":"idp-plain","scope":"` + cacheCredentialSentinel + `"}`))
		case "/api/2/report.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("password=" + cacheCredentialSentinel + "\n"))
		case "/api/task_manager/v2/activities":
			_, _ = w.Write([]byte(`{"items":[{"id":"a-1","result":{"code":"ok"}}]}`))
		default:
			_, _ = w.Write([]byte(`{"id":"t-1","name":"PlainTenantName","client_id":"plain-client"}`))
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	c := New(&config.Config{BaseURL: srv.URL, AcronisBearerAuth: "test-secret-bbbbbbbbbbbb"}, 10*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = cacheDir
	for _, path := range []string{"/api/2/clients", "/api/2/clients/c-2", "/api/2/idp/token_info", "/api/2/tenants/t-1", "/api/task_manager/v2/activities"} {
		if _, err := c.Get(context.Background(), path, nil); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}
	// A non-JSON body may fail the read; it must not reach the cache first.
	_, _ = c.Get(context.Background(), "/api/2/report.txt", nil)

	bodies := cachedBodies(t, cacheDir)
	plain, status := false, false
	for path, body := range bodies {
		if strings.Contains(body, cacheCredentialSentinel) {
			t.Errorf("a credential-bearing answer was cached: %s", path)
		}
		if strings.Contains(body, "PlainTenantName") {
			plain = true
		}
		if strings.Contains(body, `"code":"ok"`) {
			status = true
		}
	}
	if !plain {
		t.Fatalf("the plain answer was not cached either (%d files); the check above proves nothing", len(bodies))
	}
	if !status {
		t.Error("an answer whose only code is a status word was not cached")
	}

	// A refused write also drops the stale entry for the same key, so an
	// older answer is not served in its place.
	c.writeCache("/api/2/tenants/t-9", nil, json.RawMessage(`{"id":"t-9","name":"stale"}`))
	c.writeCache("/api/2/tenants/t-9", nil, json.RawMessage(`{"id":"t-9","api_token":"`+cacheCredentialSentinel+`"}`))
	if got, ok := c.readCache("/api/2/tenants/t-9", nil); ok {
		t.Errorf("a stale entry was served after a refused write: %s", got)
	}
}

func TestResponseNotCacheable(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		want       bool
	}{
		{"/api/2/tenants/t-1", `{"id":"t-1","name":"Acme","client_id":"c","tenant_id":"t","type":"partner","status":"enabled","usage":1,"value":2}`, false},
		{"/api/2/clients", `{"items":[{"client_id":"c","client_secret":"s3cr3t"}]}`, true},
		{"/api/2/clients", `{"items":[{"client_id":"c","client_secret":""}]}`, false},
		{"/api/2/users/u-1", `{"id":"u-1","auth":{"Password":"x"}}`, true},
		{"/api/2/users/u-1", `{"id":"u-1","ACCESS_TOKEN":"x"}`, true},
		{"/api/2/idp/token", `{"id":"x"}`, true},
		{"/api/2/IDP/introspect?x=1", `{"id":"x"}`, true},
		{"/api/2/clients/c-1/secret", `{"id":"x"}`, true},
		{"/api/task_manager/v2/activities", `{"items":[{"result":{"code":"error"}}]}`, false},
		{"/api/2/x", `{"code":"123456"}`, true},
		{"/api/2/x", "not json", true},
		{"/api/2/x", "", false},
		{"/api/2/x", `{"id":"y","to` + string(rune(92)) + `u006ben":"z"}`, true},
	} {
		if got := responseNotCacheable(tc.path, json.RawMessage(tc.body)); got != tc.want {
			t.Errorf("responseNotCacheable(%q, %s) = %v, want %v", tc.path, tc.body, got, tc.want)
		}
	}
}
