// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/config"
)

const binaryDeclaredSentinel = "OCTETSECRET-7Q2Z"

// binaryDeclaredContentTypes are the Content-Types a server or proxy might
// put on a JSON endpoint's body: every binary family isBinaryResponseContentType
// wraps, with parameters and odd case, plus the textual ones.
var binaryDeclaredContentTypes = []string{
	"application/octet-stream",
	"APPLICATION/OCTET-STREAM",
	"application/octet-stream; charset=binary",
	"application/zip",
	"application/pdf",
	"application/gzip",
	"application/x-msdownload",
	"application/vnd.ms-excel",
	"application/protobuf",
	"image/png",
	"audio/wav",
	"video/mp4",
	"font/woff2",
	"multipart/mixed",
	"binary/octet-stream",
	"application/json",
	"text/plain",
	"",
}

// binaryRecoverable is credvariants.Recoverable that also opens a
// _pp_binary envelope (base64), the shape that hid the token from every
// text-level check before the fix.
func binaryRecoverable(b []byte, sentinel string) bool {
	if credvariants.Recoverable(b, sentinel) {
		return true
	}
	var env struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(b, &env) == nil && env.Data != "" {
		if raw, err := base64.StdEncoding.DecodeString(env.Data); err == nil {
			return credvariants.Recoverable(raw, sentinel)
		}
	}
	return false
}

func binaryIsEnvelope(b []byte) bool {
	var env struct {
		PPBinary bool `json:"_pp_binary"`
	}
	return json.Unmarshal(b, &env) == nil && env.PPBinary
}

// TestOnlyDeclaredBinaryEndpointsAreEnveloped (Codex round 4, P1): the
// client base64-wrapped any response labelled with a binary Content-Type
// before RedactCredentialFields ran, so a device body labelled
// application/octet-stream reached CLI and MCP output with deviceToken
// intact, and the response cache stored the envelope (HoldsPersistCredential
// saw only base64). For every Content-Type above and every corpus variant of
// every Credential-tier key (valid, malformed, truncated, non-JSON), a GET on
// an endpoint that is not declared binary, success and error status, with
// the cache on: the body is never an envelope, no returned byte or error
// text holds a recoverable deviceToken, and no cache byte holds any key's
// sentinel. A request declared binary (the quarantine downloads) still gets
// the envelope under a binary Content-Type, and under application/json when
// the body is not JSON.
func TestOnlyDeclaredBinaryEndpointsAreEnveloped(t *testing.T) {
	scopes := map[string]string{
		"deviceToken": "/v1/devices",
		"downloadUrl": "/v1/installers",
		"gpoSccmFile": "/v1/gpo-sccm-files:generate",
		"uri":         "/v1/mobile-devices:enrollment-links",
	}
	type variant struct {
		key, path, body string
	}
	var variants []variant
	for _, key := range cliutil.PersistCredentialKeys() {
		path, ok := scopes[key]
		if !ok {
			t.Fatalf("no scope path for Credential-tier key %s", key)
		}
		for i, body := range credvariants.Corpus([]string{key}, binaryDeclaredSentinel, RedactedValue, 41, false) {
			if i%3 == 0 {
				variants = append(variants, variant{key, path, body})
			}
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		i, _ := strconv.Atoi(q.Get("i"))
		c, _ := strconv.Atoi(q.Get("ct"))
		if ct := binaryDeclaredContentTypes[c]; ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if q.Get("status") != "" {
			w.WriteHeader(http.StatusBadRequest)
		}
		_, _ = w.Write([]byte(variants[i].body))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = t.TempDir()

	calls := 0
	for i, v := range variants {
		for ci := range binaryDeclaredContentTypes {
			// Each variant under two of the Content-Types, every
			// Content-Type under every key's variants.
			if (i+ci)%9 != 0 {
				continue
			}
			params := map[string]string{"i": strconv.Itoa(i), "ct": strconv.Itoa(ci)}
			if i%4 == 1 {
				params["status"] = "400"
			}
			calls++
			data, err := c.Get(context.Background(), v.path, params)
			if binaryIsEnvelope(data) {
				t.Fatalf("%s (Content-Type %q) is not declared binary but was enveloped: %s", v.path, binaryDeclaredContentTypes[ci], data)
			}
			if binaryRecoverable(data, binaryDeclaredSentinel) && v.key == "deviceToken" {
				t.Fatalf("deviceToken reached the caller under Content-Type %q:\n in %q\nout %s", binaryDeclaredContentTypes[ci], v.body, data)
			}
			if err != nil && v.key == "deviceToken" && binaryRecoverable([]byte(err.Error()), binaryDeclaredSentinel) {
				t.Fatalf("an error carried deviceToken under Content-Type %q: %v", binaryDeclaredContentTypes[ci], err)
			}
		}
	}
	if calls < 1000 {
		t.Fatalf("too few calls: %d", calls)
	}
	_ = filepath.WalkDir(c.cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil && binaryRecoverable(b, binaryDeclaredSentinel) {
			t.Fatalf("a cache entry holds a Credential-tier value: %s: %s", path, b)
		}
		if rerr == nil && binaryIsEnvelope(b) {
			t.Fatalf("a cache entry is a binary envelope: %s", path)
		}
		return nil
	})

	// The declared binary requests keep their envelope.
	file := []byte("PK\x03\x04\x1b]52;c;deviceToken\x07")
	bin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.URL.Query().Get("ct"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		_, _ = w.Write(file)
	}))
	t.Cleanup(bin.Close)
	bc := New(&config.Config{BaseURL: bin.URL}, 5*time.Second, 0)
	bc.HTTPClient = bin.Client()
	for _, ct := range []string{"application/octet-stream", "application/zip", "application/json", ""} {
		data, _, err := bc.do(context.Background(), http.MethodPost, "/v1/quarantined-objects:download", map[string]string{"ct": ct}, map[string]any{}, map[string]string{BinaryResponseHeader: "true"})
		if err != nil {
			t.Fatalf("declared binary (Content-Type %q): %v", ct, err)
		}
		raw, _, ok := UnwrapBinaryResponse(data)
		if !ok || string(raw) != string(file) {
			t.Fatalf("declared binary (Content-Type %q) lost its envelope: %s", ct, data)
		}
	}
	// And a JSON body on a declared binary request under a JSON type is
	// still JSON (redacted), as before.
	jsonBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deviceToken":"` + binaryDeclaredSentinel + `"}`))
	}))
	t.Cleanup(jsonBody.Close)
	jc := New(&config.Config{BaseURL: jsonBody.URL}, 5*time.Second, 0)
	jc.HTTPClient = jsonBody.Client()
	data, _, err := jc.do(context.Background(), http.MethodPost, "/v1/quarantined-objects:download", nil, map[string]any{}, map[string]string{BinaryResponseHeader: "true"})
	if err != nil || binaryIsEnvelope(data) || strings.Contains(string(data), binaryDeclaredSentinel) {
		t.Fatalf("a JSON body on a declared binary request: %s, %v", data, err)
	}
}
