// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the large-body scan of
// the success-body funnel, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel".

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

const largeBearer = "s3cr3t/Key+Value=42"

// listUsersBody is a /ListUsers-shaped array of at least size bytes whose last
// user carries field (already JSON: a quoted string) as its employeeId.
func listUsersBody(size int, field string) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; b.Len() < size; i++ {
		fmt.Fprintf(&b, `{"id":"%08d-1111-2222-3333-444455556666","displayName":"User %d","userPrincipalName":"user%d@t.example","mail":"user%d@t.example","accountEnabled":true,"jobTitle":"Engineer","department":"IT","businessPhones":["+1 555 0100"],"assignedLicenses":[{"skuId":"c7df2760-2c81-4ef7-b578-5b5392b571df","disabledPlans":[]}],"createdDateTime":"2024-01-01T00:00:00Z"},`, i, i, i, i)
	}
	fmt.Fprintf(&b, `{"id":"last","userPrincipalName":"last@t.example","employeeId":%s}]`, field)
	return []byte(b.String())
}

func quoted(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSpace(buf.String())
}

func largeClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	clearVerifyEnv(t)
	return New(&config.Config{BaseURL: baseURL, CippApiKey: largeBearer, AuthSource: "env:CIPP_API_KEY", Path: filepath.Join(t.TempDir(), "config.toml")}, 30*time.Second, 0)
}

// TestLargeBodyCleanPassesUnchanged: a clean 3 MiB /ListUsers body is past the
// shared detector's whole-scan limit and comes back, and is cached, byte for
// byte.
func TestLargeBodyCleanPassesUnchanged(t *testing.T) {
	body := listUsersBody(3<<20, `"E-1001"`)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer api.Close()
	c := largeClient(t, api.URL)
	cache := t.TempDir()
	c.cacheDir = cache
	data, err := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"})
	if err != nil {
		t.Fatalf("a clean 3 MiB body was refused: %v", err)
	}
	if !bytes.Equal(data, body) {
		t.Fatalf("a clean 3 MiB body changed (%d bytes back, %d sent)", len(data), len(body))
	}
	cached := false
	_ = filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && bytes.Equal(b, body) {
				cached = true
			}
		}
		return nil
	})
	if !cached {
		t.Fatal("the clean 3 MiB body was not cached unchanged")
	}
}

// largeEncodings is the bearer in every form the scan must find: each mask the
// client builds, base64 and base64url inside a longer blob at all three byte
// alignments, mixed-case hex, and partial escapes.
func largeEncodings(t *testing.T, c *Client) map[string]string {
	t.Helper()
	out := map[string]string{}
	masks, _, _ := c.credentialMaskSet(true, []string{"Bearer " + largeBearer})
	for i, m := range masks {
		out[fmt.Sprintf("mask-%03d", i)] = m.needle
	}
	for k := 0; k < 3; k++ {
		blob := append(append([]byte("\x01\x02\x03")[:k], largeBearer...), "tail-bytes"...)
		out[fmt.Sprintf("base64-std-align-%d", k)] = "data:" + base64.StdEncoding.EncodeToString(blob)
		out[fmt.Sprintf("base64-url-align-%d", k)] = "data:" + base64.RawURLEncoding.EncodeToString(blob)
	}
	h := []byte(hex.EncodeToString([]byte(largeBearer)))
	for i := range h {
		if i%3 == 0 && h[i] >= 'a' {
			h[i] -= 'a' - 'A'
		}
	}
	out["hex-mixed-case"] = string(h)
	var pct, ent strings.Builder
	for i, r := range largeBearer {
		if i%2 == 0 {
			fmt.Fprintf(&pct, "%%%02x", r)
			fmt.Fprintf(&ent, "&#%d;", r)
		} else {
			pct.WriteRune(r)
			ent.WriteRune(r)
		}
	}
	out["percent-partial"] = pct.String()
	out["html-partial"] = ent.String()
	return out
}

// TestLargeBodyEveryEncodingWithheld: the bearer once, at the end of a 3 MiB
// /ListUsers body, in each encoding: every body is withheld as carrying a
// credential.
func TestLargeBodyEveryEncodingWithheld(t *testing.T) {
	c := largeClient(t, "http://example.test")
	auth := "Bearer " + largeBearer
	cases := map[string]string{
		// The JSON-level escape: the raw body spells the bearer's first
		// character as \u0073; a JSON reader sees the bearer itself.
		"json-escaped": `"\u0073` + largeBearer[1:] + `"`,
	}
	for name, form := range largeEncodings(t, c) {
		cases[name] = quoted(form)
	}
	for name, field := range cases {
		field := field
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, err := c.redactSuccessBody(listUsersBody(3<<20, field), auth)
			if !errors.Is(err, ErrSuccessBodyWithheld) {
				t.Errorf("a 3 MiB body with the bearer was not withheld: err=%v, %d bytes back", err, len(out))
			}
		})
	}
}

// TestLargeBodyZeroWidthSplitWithheld: the bearer split by format and
// zero-width characters, as written and JSON-escaped, at the end of a 3 MiB
// body, is withheld.
func TestLargeBodyZeroWidthSplitWithheld(t *testing.T) {
	c := largeClient(t, "http://example.test")
	auth := "Bearer " + largeBearer
	head, tail := largeBearer[:8], largeBearer[8:]
	cases := map[string]string{
		"u200b-x200000": quoted(head + strings.Repeat("\u200b", 200000) + tail),
		"mixed-format":  quoted(head[:4] + "\u200c" + head[4:] + "\u200d\u2060" + tail[:3] + "\ufeff" + tail[3:]),
		"json-escaped":  `"` + head + strings.Repeat(`\u200b`, 1000) + tail + `"`,
		"base64-split": quoted(func() string {
			s := base64.StdEncoding.EncodeToString([]byte(largeBearer))
			return s[:6] + "\u2060\u200b" + s[6:]
		}()),
	}
	for name, field := range cases {
		out, err := c.redactSuccessBody(listUsersBody(3<<20, field), auth)
		if !errors.Is(err, ErrSuccessBodyWithheld) {
			t.Errorf("%s: a zero-width-split bearer was not withheld: err=%v, %d bytes back", name, err, len(out))
		}
	}
}
