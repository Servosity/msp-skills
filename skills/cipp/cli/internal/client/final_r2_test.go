// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the final Codex session's
// round-2 input, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel".

package client

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

// entityEcho is the configured key aaaaaaaaaaaa base64-encoded twice, each
// character written as an HTML decimal entity, as Codex sent it.
const entityEcho = "&#87;&#86;&#100;&#71;&#97;&#70;&#108;&#88;&#82;&#109;&#104;&#90;&#86;&#48;&#90;&#111;&#87;&#86;&#100;&#71;&#97;&#65;&#61;&#61;"

// TestEntityEncodedBearerPastCapIsWithheld: Codex's exact input on the CLI
// path, cache on. One JSON value holds the entity-encoded echo and 1048576
// periods: over the 64 KiB cap, so the body is withheld and nothing is cached.
func TestEntityEncodedBearerPastCapIsWithheld(t *testing.T) {
	clearVerifyEnv(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"` + entityEcho + strings.Repeat(".", 1048576) + `"}`))
	}))
	defer api.Close()
	c := New(&config.Config{BaseURL: api.URL, CippApiKey: "aaaaaaaaaaaa", AuthSource: "env:CIPP_API_KEY", Path: filepath.Join(t.TempDir(), "config.toml")}, 30*time.Second, 0)
	cache := t.TempDir()
	c.cacheDir = cache
	data, _ := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"})
	if strings.Contains(string(data), "&#87;&#86;") {
		t.Fatal("the entity-encoded bearer came back in the body")
	}
	_ = filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "&#87;&#86;") {
				t.Errorf("the entity-encoded bearer was cached in %s", p)
			}
		}
		return nil
	})
}

// TestSpacedBearerValueIsWithheld: the round-1 input. The spaced echo, with
// whitespace removed, is within the cap and gets the full recursive decoding:
// withheld as carrying the credential.
func TestSpacedBearerValueIsWithheld(t *testing.T) {
	c := &Client{}
	body := []byte(`{"echo":"WVdG aFlX RmhZ V0Zo WVdG aA==` + strings.Repeat(" ", 70000) + `"}`)
	if _, err := c.redactSuccessBody(body, "Bearer aaaaaaaaaaaa"); !errors.Is(err, ErrSuccessBodyWithheld) {
		t.Fatalf("the spaced echo was not withheld as carrying the credential: %v", err)
	}
}

// TestProseValueOverCapIsWithheld: the documented cost. A single value of
// ordinary prose, 70 KiB once spaces are removed, is over the cap and
// withholds the body, though it carries no credential.
func TestProseValueOverCapIsWithheld(t *testing.T) {
	c := &Client{}
	sentence := "The quick brown fox jumps over the lazy dog. "
	prose := strings.Repeat(sentence, 70<<10/len(strings.ReplaceAll(sentence, " ", ""))+1)
	if n := len(strings.ReplaceAll(prose, " ", "")); n < 70<<10 {
		t.Fatalf("fixture is %d bytes without spaces, want at least 70 KiB", n)
	}
	body := []byte(`{"notes":"` + prose + `"}`)
	if _, err := c.redactSuccessBody(body, "Bearer aaaaaaaaaaaa"); !errors.Is(err, ErrSuccessBodyValueTooLarge) {
		t.Fatalf("a 70 KiB prose value was not withheld as too large: %v", err)
	}
}
