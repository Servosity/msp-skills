// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: every answer the generated client
// reads has its visas held before any status, content-type or cache branch,
// and every success answer passes the cove success check on its raw bytes
// before it is wrapped, cached or replayed (Codex round 1, press 4.34.0
// reprint). Hand-fix code: cove-short-credential-funnel.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/config"
)

// coveEsc writes a JSON escape without spelling a backslash in this file:
// "~" becomes a backslash.
func coveEsc(s string) string { return strings.ReplaceAll(s, "~", string(rune(92))) }

// coveAnswerClient is a client for a loopback server that answers every
// request with status, Content-Type ct and body answer, counting the hits.
func coveAnswerClient(t *testing.T, status int, ct, answer string) (*Client, *int64) {
	t.Helper()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	// No retry: a 5xx answer is reported at once.
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0), &hits
}

var coveB64Run = regexp.MustCompile(`[A-Za-z0-9+/_-]{8,}={0,2}`)

// coveLeaks reports whether text shows secret as written or inside any
// base64 or base64url run at any of its four alignments: an oracle written
// apart from the funnel it tests.
func coveLeaks(text, secret string) bool {
	if strings.Contains(text, secret) {
		return true
	}
	for _, run := range coveB64Run.FindAllString(text, -1) {
		run = strings.TrimRight(run, "=")
		for off := 0; off < 4 && off < len(run); off++ {
			s := run[off:]
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
				for cut := len(s); cut > len(s)-4 && cut > 0; cut-- {
					if b, err := enc.DecodeString(s[:cut]); err == nil && strings.Contains(string(b), secret) {
						return true
					}
				}
			}
		}
	}
	return false
}

func coveHolds(v string) bool {
	for _, h := range cliutil.FunnelHeldValues() {
		if h == v {
			return true
		}
	}
	return false
}

// Codex round 1, finding 1: an HTTP 200 octet-stream answer carrying a visa
// was base64-wrapped whole before the success check, so the visa reached the
// result in a reversible encoding. The raw bytes now pass the check first.
func TestCoveBinarySuccessIsCheckedBeforeWrapping(t *testing.T) {
	const visa = "new-visa-1234567890"
	c, _ := coveAnswerClient(t, 200, "application/octet-stream", `{"visa":"new-visa-1234567890","result":{"result":{}}}`)
	data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err != nil && coveLeaks(err.Error(), visa) {
		t.Fatalf("the refusal shows the visa: %v", err)
	}
	if coveLeaks(string(data), visa) {
		t.Fatalf("the returned visa reached the result: %s", data)
	}
	if raw, _, ok := UnwrapBinaryResponse(data); ok && coveLeaks(string(raw), visa) {
		t.Fatalf("the wrapped payload carries the visa: %s", raw)
	}
	if !coveHolds(visa) {
		t.Fatal("the returned visa is not held for the backstops")
	}
}

// A binary answer that repeats a visa anywhere but its top-level visa member
// is refused, not wrapped.
func TestCoveBinarySuccessRefusesAnEcho(t *testing.T) {
	const visa = "nested-bin-visa-0123456789"
	c, _ := coveAnswerClient(t, 200, "application/zip", `{"visa":"nested-bin-visa-0123456789","result":{"result":{"Note":"nested-bin-visa-0123456789"}}}`)
	data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err == nil {
		t.Fatalf("a binary answer repeating the visa was returned: %s", data)
	}
	if coveLeaks(err.Error(), visa) {
		t.Fatalf("the refusal shows the visa: %v", err)
	}
}

// A clean binary answer is still wrapped and round-trips byte for byte.
func TestCoveBinarySuccessCleanStillWraps(t *testing.T) {
	payload := "\x89PNG\r\n\x1a\nclean-image-bytes"
	c, _ := coveAnswerClient(t, 200, "image/png", payload)
	data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err != nil {
		t.Fatalf("a clean binary answer was refused: %v", err)
	}
	raw, ct, ok := UnwrapBinaryResponse(data)
	if !ok || string(raw) != payload || ct != "image/png" {
		t.Fatalf("clean binary answer changed: ok=%v ct=%q raw=%q", ok, ct, raw)
	}
}

// Codex round 1, finding 2: an HTTP error answer's fresh visa was never
// held, so neither the error masker nor the backstops knew it.
func TestCoveHTTPErrorHoldsTheReturnedVisa(t *testing.T) {
	const visa = "fresh-visa-1234567890"
	c, _ := coveAnswerClient(t, 400, "application/json", `{"visa":"fresh-visa-1234567890","error":{"code":-32603,"data":13501,"message":"refused"}}`)
	_, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err == nil {
		t.Fatal("expected an HTTP error")
	}
	if coveLeaks(err.Error(), visa) {
		t.Fatalf("the error shows the returned visa: %v", err)
	}
	if !strings.Contains(err.Error(), "13501") {
		t.Fatalf("the error lost the server's code (masked, not withheld whole, is expected): %v", err)
	}
	if !coveHolds(visa) {
		t.Fatal("the returned visa is not held for the backstops")
	}
	if shown := cliutil.FunnelHeldAll("upstream said " + visa); coveLeaks(shown, visa) {
		t.Fatalf("the backstop does not withhold the returned visa: %s", shown)
	}
}

// An HTTP 500 plain-text answer that carries a JSON visa member.
func TestCoveServerErrorTextHoldsTheReturnedVisa(t *testing.T) {
	const visa = "plain-visa-0123456789"
	c, _ := coveAnswerClient(t, 500, "text/plain", `upstream failure: {"visa":"plain-visa-0123456789","error":{"message":"boom"}}`)
	_, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
	if err == nil {
		t.Fatal("expected an HTTP error")
	}
	if coveLeaks(err.Error(), visa) {
		t.Fatalf("the error shows the returned visa: %v", err)
	}
	if !coveHolds(visa) {
		t.Fatal("the returned visa is not held for the backstops")
	}
}

// A visa under an escaped member name ("visa" spelled with a JSON escape),
// in a binary success answer and in an HTTP error answer.
func TestCoveEscapedVisaMemberName(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		ct     string
	}{
		"binary success": {200, "application/octet-stream"},
		"http error":     {400, "application/json"},
	} {
		t.Run(name, func(t *testing.T) {
			visa := "esc-visa-" + strings.ReplaceAll(name, " ", "-") + "-0123456789"
			answer := coveEsc(`{"~u0076isa":"` + visa + `","result":{"result":{"Version":"5.1"}}}`)
			c, _ := coveAnswerClient(t, tc.status, tc.ct, answer)
			data, _, err := c.PostQueryWithParams(context.Background(), "/jsonapi", nil, coveRawBody("sent-visa-0123456789"))
			if err != nil && coveLeaks(err.Error(), visa) {
				t.Fatalf("the error shows the visa: %v", err)
			}
			if coveLeaks(string(data), visa) {
				t.Fatalf("the result shows the visa: %s", data)
			}
			if raw, _, ok := UnwrapBinaryResponse(data); ok && coveLeaks(string(raw), visa) {
				t.Fatalf("the wrapped payload carries the visa: %s", raw)
			}
			if !coveHolds(visa) {
				t.Fatal("the visa is not held for the backstops")
			}
		})
	}
}

// A stored answer that carries a visa (one a build before this check wrote,
// or a file changed on disk) is never replayed: it is a cache miss, its visa
// is held, and the live answer is fetched and checked instead.
func TestCoveCachedReplayIsChecked(t *testing.T) {
	for name, stored := range map[string]string{
		"binary envelope": "",
		"json answer":     `{"visa":"VISA","result":{"result":{"Version":"5.1"}}}`,
		"escaped name":    coveEsc(`{"~u0076isa":"VISA","result":{"result":{}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			visa := "cached-visa-" + strings.ReplaceAll(name, " ", "-") + "-0123456789"
			c, hits := coveAnswerClient(t, 200, "application/json", `{"result":{"result":{"Live":true}}}`)
			if stored == "" {
				env, err := wrapBinaryResponse("application/octet-stream", []byte(`{"visa":"`+visa+`","result":{"result":{}}}`))
				if err != nil {
					t.Fatal(err)
				}
				stored = string(env)
			}
			stored = strings.ReplaceAll(stored, "VISA", visa)
			params := map[string]string{"q": "1"}
			c.writeCacheWithHeaders("/status", params, nil, json.RawMessage(stored))
			data, err := c.Get(context.Background(), "/status", params)
			if err != nil {
				t.Fatalf("live fetch failed: %v", err)
			}
			if coveLeaks(string(data), visa) {
				t.Fatalf("a stored visa was replayed: %s", data)
			}
			if atomic.LoadInt64(hits) != 1 || !strings.Contains(string(data), `"Live":true`) {
				t.Fatalf("the stored answer was replayed instead of fetched (hits=%d): %s", atomic.LoadInt64(hits), data)
			}
			if !coveHolds(visa) {
				t.Fatal("the stored visa is not held for the backstops")
			}
		})
	}
}

// A clean stored answer is still replayed (the check does not disable the
// cache).
func TestCoveCachedCleanReplayStillServed(t *testing.T) {
	c, hits := coveAnswerClient(t, 200, "application/json", `{"result":{"result":{"Live":true}}}`)
	params := map[string]string{"q": "2"}
	c.writeCacheWithHeaders("/status", params, nil, json.RawMessage(`{"result":{"result":{"Cached":true}}}`))
	data, err := c.Get(context.Background(), "/status", params)
	if err != nil || !strings.Contains(string(data), `"Cached":true`) || atomic.LoadInt64(hits) != 0 {
		t.Fatalf("clean cached answer not replayed (hits=%d, err=%v): %s", atomic.LoadInt64(hits), err, data)
	}
}

// A binary GET answer carrying a visa never lands in the response cache.
func TestCoveBinaryAnswerNeverCachesAVisa(t *testing.T) {
	const visa = "store-visa-0123456789"
	c, _ := coveAnswerClient(t, 200, "application/octet-stream", `{"visa":"store-visa-0123456789","result":{"result":{}}}`)
	if _, err := c.Get(context.Background(), "/status", map[string]string{"q": "3"}); err != nil && coveLeaks(err.Error(), visa) {
		t.Fatalf("the error shows the visa: %v", err)
	}
	if c.cacheDir == "" {
		t.Fatal("no cache dir: the test does not reach the cache")
	}
	_ = filepath.WalkDir(c.cacheDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(p) // #nosec G304 -- test temp dir.
		if rerr == nil && coveLeaks(string(b), visa) {
			t.Fatalf("the cache file %s carries the visa: %s", p, b)
		}
		return nil
	})
}
