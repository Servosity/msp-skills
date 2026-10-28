// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (secret-field-persistence class): a live read may show a
// response secret to the operator, but the response cache never writes it.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/cliutil/testenv"
	"blackpoint-pp-cli/internal/config"
)

func TestResponseCacheNeverWritesASecretField(t *testing.T) {
	const pw = "Hunter2SentinelPW"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"e1","username":"bob@acme.test","password":"` + pw + `"}]}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, BlackpointApiToken: "test-secret-token-XXXXXXXX"}, 5*time.Second, 0)
	dir := t.TempDir()
	c.cacheDir = dir
	out, err := c.Get(context.Background(), "/vulnerability-management/darkweb/exposures", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), pw) || !strings.Contains(string(out), "bob@acme.test") {
		t.Fatalf("the live read must replace the field and keep the rest of the row: %s", out)
	}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), pw) {
				t.Fatalf("cache file %s holds the response password", p)
			}
		}
		return nil
	})
}

// secretHidingBodies hold a response secret the decoded value does not show:
// encoding/json keeps the last of a duplicate key, decodes only the first
// top-level value, and gives up on text it cannot parse. Each must still be
// kept out of the cache in its original bytes.
func secretHidingBodies(pw string) map[string]string {
	return map[string]string{
		"duplicate-key":        `{"data":[{"id":"e1","password":"` + pw + `","password":null}]}`,
		"duplicate-case":       `{"data":[{"id":"e1","Password":"` + pw + `","password":null}]}`,
		"duplicate-bool":       `{"data":[{"id":"e1","password":"` + pw + `","password":true}]}`,
		"duplicate-nested":     `{"data":[{"id":"e1","credentials":{"value":"` + pw + `"},"credentials":null}]}`,
		"duplicate-deep":       `{"data":[{"id":"e1","cisco":{"skey":"` + pw + `","skey":""}}]}`,
		"second-value":         `{"data":[]} {"data":[{"id":"e1","password":"` + pw + `"}]}`,
		"trailing-text":        `{"data":[]}garbage"password":"` + pw + `"`,
		"invalid-after-prefix": `{"data":[{"id":"e1","password":"` + pw + `"},]}`,
		"truncated":            `{"data":[{"id":"e1","password":"` + pw + `"}`,
		"bad-escape-in-secret": `{"data":[{"id":"e1","password":"` + pw + `\q"}]}`,
		"unstripped-prefix":    ")]}'\n)]}'" + `{"data":[{"id":"e1","password":"` + pw + `"}]}`,
	}
}

func TestResponseCacheSeesEveryKeyOccurrence(t *testing.T) {
	testenv.Isolate(t)
	const pw = "breached-secret-123"
	for name, body := range secretHidingBodies(pw) {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			c := New(&config.Config{BaseURL: srv.URL, BlackpointApiToken: "test-secret-token-XXXXXXXX"}, 5*time.Second, 0)
			dir := t.TempDir()
			c.cacheDir = dir
			out, err := c.Get(context.Background(), "/vulnerability-management/darkweb/exposures", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) == 0 {
				t.Fatal("the live read must still return the body to the operator")
			}
			if f := cacheFileHolding(t, dir, pw); f != "" {
				t.Fatalf("cache file %s holds the response password", f)
			}
		})
	}
}

// A body with no secret value still caches, including keys the name rule
// does not treat as secrets and secret-named keys holding only flags/nulls.
func TestResponseCacheStillWritesACleanBody(t *testing.T) {
	testenv.Isolate(t)
	const body = `{"data":[{"id":"e1","token_hint":"abc","keyboard":"us","password":null,"passwordExposed":true,"credential":""}]}`
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, BlackpointApiToken: "test-secret-token-XXXXXXXX"}, 5*time.Second, 0)
	dir := t.TempDir()
	c.cacheDir = dir
	for i := 0; i < 2; i++ {
		out, err := c.Get(context.Background(), "/vulnerability-management/darkweb/exposures", nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != body {
			t.Fatalf("read %d returned %s", i, out)
		}
	}
	if hits != 1 {
		t.Fatalf("a clean body must be served from the cache on the second read; the server saw %d requests", hits)
	}
	if cacheFileHolding(t, dir, `"token_hint":"abc"`) == "" {
		t.Fatal("a clean body was not written to the cache")
	}
}

func cacheFileHolding(t *testing.T, dir, needle string) string {
	t.Helper()
	found := ""
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && found == "" {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), needle) {
				found = p
			}
		}
		return nil
	})
	return found
}
