// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/config"
)

const redactPropertySentinel = "DTSECRET-4K8W"

// TestRedactCredentialFieldsPropertyNeverReturnsTheToken is the round-3 class
// test for device-token-redacted-at-decode: for deviceToken in every spelling,
// in every valid shape and every malformed, truncated, concatenated or
// non-JSON variant, whatever RedactCredentialFields hands to callers holds no
// recoverable token, a body is withheld only when it is not one JSON value,
// and every valid body is accepted.
func TestRedactCredentialFieldsPropertyNeverReturnsTheToken(t *testing.T) {
	corpus := credvariants.Corpus(credentialResponseKeys, redactPropertySentinel, RedactedValue, 31, true)
	withheld := 0
	for _, v := range corpus {
		out, ok := RedactCredentialFields([]byte(v))
		valid := json.Valid([]byte(v))
		if valid && !ok {
			t.Fatalf("a valid body was withheld: %q", v)
		}
		if !ok {
			withheld++
			continue
		}
		if credvariants.Recoverable(out, redactPropertySentinel) {
			t.Fatalf("the token reached the caller:\n in %q\nout %q", v, out)
		}
		if shown := RedactCredentialFieldsOrWithhold([]byte(v)); credvariants.Recoverable(shown, redactPropertySentinel) {
			t.Fatalf("the error text holds the token: %q", shown)
		}
	}
	if withheld < 1000 {
		t.Fatalf("corpus too small: %d withheld of %d", withheld, len(corpus))
	}
}

// TestClientFunnelsNeverReturnOrCacheTheToken drives a sample of the corpus
// through client.do (success and error responses) with the response cache on:
// no returned body, no error text and no byte under the cache directory holds
// a recoverable token, and the Codex round-3 body ({}]{"deviceToken":...})
// is an error, not data.
func TestClientFunnelsNeverReturnOrCacheTheToken(t *testing.T) {
	corpus := credvariants.Corpus(credentialResponseKeys, redactPropertySentinel, RedactedValue, 32, false)
	corpus = append(corpus, `{}]{"deviceToken":"`+redactPropertySentinel+`"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i, _ := strconv.Atoi(r.URL.Query().Get("i"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("status") != "" {
			w.WriteHeader(http.StatusBadRequest)
		}
		_, _ = w.Write([]byte(corpus[i]))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = t.TempDir()

	codex := len(corpus) - 1
	for i := 0; i < len(corpus); i++ {
		if i%5 != 0 && i != codex {
			continue
		}
		params := map[string]string{"i": strconv.Itoa(i)}
		if i%10 == 5 {
			params["status"] = "400"
		}
		data, err := c.Get(context.Background(), "/v1/devices", params)
		if credvariants.Recoverable(data, redactPropertySentinel) {
			t.Fatalf("client returned the token for %q: %s", corpus[i], data)
		}
		if err != nil && credvariants.Recoverable([]byte(err.Error()), redactPropertySentinel) {
			t.Fatalf("an error carried the token for %q: %v", corpus[i], err)
		}
		if i == codex && (err == nil || !strings.Contains(err.Error(), CredentialBodyWithheld)) {
			t.Fatalf("the Codex round-3 body must be an error, got data %s err %v", data, err)
		}
	}
	_ = filepath.WalkDir(c.cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil && credvariants.Recoverable(b, redactPropertySentinel) {
			t.Fatalf("a cache entry holds the token: %s: %s", path, b)
		}
		if rerr == nil && filepath.Ext(path) == ".json" && !json.Valid(b) {
			t.Fatalf("a cache entry is not one JSON value: %s", path)
		}
		return nil
	})
}

// TestResponseCacheNeverWritesABodyThatIsNotOneJSONValue covers the cache
// half of the round-3 rule for every persist key: the Codex body and a
// truncated installer list are returned (the CLI then reports them) but never
// written to the response cache, and an entry an earlier build left is
// deleted, not served.
func TestResponseCacheNeverWritesABodyThatIsNotOneJSONValue(t *testing.T) {
	bodies := []string{
		`{}]{"deviceToken":"` + cacheLinkSentinel + `"}`,
		`{"installers":[{"uuid":"b1","downloadUrl":"https://download.example/` + cacheLinkSentinel,
		`{"enrollmentLinks":{}}{"uri":"` + cacheLinkSentinel + `"}`,
		`{"gpoSccmFile":"` + cacheLinkSentinel + `"}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i, _ := strconv.Atoi(r.URL.Query().Get("i"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bodies[i]))
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = t.TempDir()
	for i := range bodies {
		_, _ = c.Get(context.Background(), "/v1/installers", map[string]string{"i": strconv.Itoa(i)})
	}
	if hits := cacheDirHolds(t, c.cacheDir, cacheLinkSentinel); len(hits) > 0 {
		t.Fatalf("a body that is not one JSON value reached the response cache: %v", hits)
	}
	path := "/v1/installers"
	params := map[string]string{"i": "9"}
	file := filepath.Join(c.cacheResourceDir(path), c.cacheKeyFor(http.MethodGet, path, params, nil, nil)+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(bodies[0]), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.readCache(path, params); ok {
		t.Fatal("a cached body that is not one JSON value was served")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the cached body that is not one JSON value was left on disk: %v", err)
	}
}
