// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/huntress/handfixes.json under
// the "secret-echo-decoded-funnel" entry. The class these tests close: a
// successful body reaches the caller (stdout, an MCP tool result, the local
// store) without the complete success-body check. Every member below is a
// path that did so: a cached body served before the check, a cache entry an
// older build wrote, a body over the funnel's 1 MiB cap whose echo is split
// across the check's windows by removable padding, and a binary envelope whose
// content_type (a response header) was never checked.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"huntress-pp-cli/internal/config"
)

const (
	classFunnelKey      = "hk_successclass_key"
	classFunnelSecret16 = "abcdefghijklmnop"
	classFunnelSecret8  = "abcdefgh"
	classFunnelLive     = `{"account":{"name":"Acme Dental"}}`
)

func newClassFunnelServer(t *testing.T, contentType string, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func newClassFunnelClient(t *testing.T, srv *httptest.Server, secret string, cached bool) *Client {
	t.Helper()
	c := New(&config.Config{BaseURL: srv.URL, HuntressApiKey: classFunnelKey, HuntressApiSecret: secret}, 30*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.NoCache = !cached
	if cached && c.cacheDir == "" {
		t.Fatal("test needs a response cache dir")
	}
	return c
}

// classFunnelPadded is head + n copies of pad + tail.
func classFunnelPadded(head, pad string, n int, tail string) []byte {
	return []byte(head + strings.Repeat(pad, n) + tail)
}

// Member 1: GetWithHeaders returned a fresh cache entry before the success
// funnel ran. Every cache read must run the same check as a live answer: a
// cached body that fails it is refused and deleted, never served, and the
// call falls through to the live answer (which is checked again).
func TestSuccessFunnelClassCachedBodyIsChecked(t *testing.T) {
	ctx := context.Background()
	getters := map[string]func(*Client) (json.RawMessage, error){
		"Get": func(c *Client) (json.RawMessage, error) { return c.Get(ctx, "/v1/account", nil) },
		"GetWithHeaders": func(c *Client) (json.RawMessage, error) {
			return c.GetWithHeaders(ctx, "/v1/account", nil, nil)
		},
		"GetWithHeadersValues": func(c *Client) (json.RawMessage, error) {
			return c.GetWithHeadersValues(ctx, "/v1/account", url.Values{}, nil)
		},
	}
	envelope, err := wrapBinaryResponse("application/x-"+classFunnelSecret16, []byte("plain bytes"))
	if err != nil {
		t.Fatal(err)
	}
	cachedBodies := map[string]struct {
		body   []byte
		secret string
	}{
		"json-echo":                  {[]byte(`{"account":{"name":"` + classFunnelSecret16 + `"}}`), classFunnelSecret16},
		"envelope-content-type-echo": {envelope, classFunnelSecret16},
		"padded-large-echo":          {classFunnelPadded(`{"account":{"name":"abcd`, `\u0000`, 200000, `efgh"}}`), "abcd"},
	}
	for gname, get := range getters {
		for bname, cached := range cachedBodies {
			t.Run(gname+"/"+bname, func(t *testing.T) {
				t.Setenv("HUNTRESS_HOME", t.TempDir())
				srv, hits := newClassFunnelServer(t, "application/json", []byte(classFunnelLive))
				secret := classFunnelSecret16
				if bname == "padded-large-echo" {
					secret = classFunnelSecret8
				}
				c := newClassFunnelClient(t, srv, secret, true)
				c.writeCache("/v1/account", nil, json.RawMessage(cached.body))
				cacheFile := filepath.Join(c.cacheResourceDir("/v1/account"), c.cacheKeyFor(http.MethodGet, "/v1/account", nil, nil, nil)+".json")
				got, err := get(c)
				if strings.Contains(string(got), cached.secret) {
					t.Fatalf("a cached body that repeats a held credential was served: %.200s", got)
				}
				if err != nil || string(got) != classFunnelLive {
					t.Fatalf("want the live answer after the cached body was refused, got %.200s (%v)", got, err)
				}
				if hits.Load() != 1 {
					t.Fatalf("live server hits = %d, want 1", hits.Load())
				}
				if data, rerr := os.ReadFile(cacheFile); rerr == nil && strings.Contains(string(data), cached.secret) {
					t.Fatalf("the refused cache entry is still on disk")
				}
			})
		}
	}
}

// Control for member 1: an ordinary cached body is still served from the
// cache, unchanged, without a live request.
func TestSuccessFunnelClassCleanCachedBodyIsServed(t *testing.T) {
	t.Setenv("HUNTRESS_HOME", t.TempDir())
	srv, hits := newClassFunnelServer(t, "application/json", []byte(`{"account":{"name":"From The Server"}}`))
	c := newClassFunnelClient(t, srv, classFunnelSecret16, true)
	c.writeCache("/v1/account", nil, json.RawMessage(classFunnelLive))
	got, err := c.Get(context.Background(), "/v1/account", nil)
	if err != nil || string(got) != classFunnelLive {
		t.Fatalf("clean cached body not served: %s (%v)", got, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("a clean cache hit went to the server (%d hits)", hits.Load())
	}
}

// Member 1 (pre-fix entries): a fresh cache entry written by an older build,
// which served cached bodies unchecked, must never be read after upgrading,
// and is removed from disk when the client binds its cache.
func TestSuccessFunnelClassPreFixCacheEntriesAreNeverRead(t *testing.T) {
	for name, legacy := range map[string]string{
		// Codex r2 input: secret abcdefghijklmnop, an older build's entry for GET /v1/account.
		"codex-r2-echo": `{"account":{"name":"` + classFunnelSecret16 + `"}}`,
		// A clean stale body proves the entry is not read at all (the check
		// alone would serve it).
		"clean-stale": `{"account":{"name":"Stale From An Older Build"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HUNTRESS_HOME", t.TempDir())
			srv, hits := newClassFunnelServer(t, "application/json", []byte(classFunnelLive))
			older := newClassFunnelClient(t, srv, classFunnelSecret16, true)
			// The 4.34.0 layout: <cache>/resources/<tag>/<cacheKeyFor>.json.
			legacyRoot := filepath.Join(older.cacheDir, "resources")
			legacyDir := filepath.Join(legacyRoot, cacheResourceTag("/v1/account"))
			legacyFile := filepath.Join(legacyDir, older.cacheKeyFor(http.MethodGet, "/v1/account", nil, nil, nil)+".json")
			if err := os.MkdirAll(legacyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacyFile, []byte(legacy), 0o600); err != nil {
				t.Fatal(err)
			}
			c := newClassFunnelClient(t, srv, classFunnelSecret16, true)
			got, err := c.Get(context.Background(), "/v1/account", nil)
			if strings.Contains(string(got), classFunnelSecret16) {
				t.Fatalf("an older build's cache entry served the credential: %s", got)
			}
			if err != nil || string(got) != classFunnelLive {
				t.Fatalf("want the live answer, not an older build's cache entry: %s (%v)", got, err)
			}
			if hits.Load() != 1 {
				t.Fatalf("live server hits = %d, want 1", hits.Load())
			}
			if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
				t.Fatalf("an older build's cache entry is still on disk (%v)", err)
			}
		})
	}
}

// Member 2: a body over the funnel's 1 MiB cap was checked in overlapping
// 1 MiB windows, so removable padding split an echo across windows. Over the
// cap, a body that is not JSON is refused, a JSON token over the cap is
// refused, and every JSON token (keys, strings, numbers) is checked whole.
func TestSuccessFunnelClassLargeBodyIsCheckedWhole(t *testing.T) {
	for name, tc := range map[string]struct {
		contentType string
		body        []byte
	}{
		// Codex r2 input.
		"codex-r2-octet-stream": {"application/octet-stream", classFunnelPadded("abcd", "\x00", 600000, "efgh")},
		// 1100001 pad bytes, so the envelope's base64 never happens to align
		// "efgh" with a fragment of the held Basic header (an unrelated
		// signal), and only the over-cap rule is tested.
		"octet-stream-split-across-windows": {"application/octet-stream", classFunnelPadded("abcd", "\x00", 1100001, "efgh")},
		"text-over-cap":                     {"text/plain", classFunnelPadded("abcd", "\x00", 1100000, "efgh")},
		"json-token-over-cap":               {"application/json", classFunnelPadded(`{"blob":"abcd`, `\u0000`, 200000, `efgh"}`)},
		"json-escape-padding-straddles-window": {"application/json", classFunnelPadded(
			`{"pad":"`+strings.Repeat("z", 900000)+`","blob":"abcd`, `\u0000`, 40000, `efgh"}`)},
		"json-invalid-utf8-padding-straddles-window": {"application/json", classFunnelPadded(
			`{"pad":"`+strings.Repeat("z", 900000)+`","blob":"abcd`, "\xff", 240000, `efgh"}`)},
		"json-key-straddles-window": {"application/json", classFunnelPadded(
			`{"pad":"`+strings.Repeat("z", 900000)+`","abcd`, `\u0000`, 40000, `efgh":1}`)},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := newClassFunnelServer(t, tc.contentType, tc.body)
			c := newClassFunnelClient(t, srv, classFunnelSecret8, false)
			got, err := c.Get(context.Background(), "/v1/account", nil)
			if err == nil {
				t.Fatalf("a %d-byte body whose padding splits a held credential was returned (%d bytes)", len(tc.body), len(got))
			}
			if strings.Contains(err.Error(), "abcd") {
				t.Fatalf("the refusal repeats the echo: %.200s", err)
			}
		})
	}
}

// Control for member 2: an ordinary JSON body over the cap is returned
// byte-identical (the check is not "refuse everything large").
func TestSuccessFunnelClassLargeCleanJSONIsReturned(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"agents":[`)
	for i := 0; b.Len() < 1600000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":`)
		b.WriteString(strings.Repeat("7", 1+i%5))
		b.WriteString(`,"hostname":"Front Desk PC","platform":"windows","notes":"` + strings.Repeat("q", i%300) + `"}`)
	}
	b.WriteString(`],"pagination":{}}`)
	body := b.String()
	srv, _ := newClassFunnelServer(t, "application/json", []byte(body))
	c := newClassFunnelClient(t, srv, classFunnelSecret8, false)
	got, err := c.Get(context.Background(), "/v1/agents", nil)
	if err != nil || string(got) != body {
		t.Fatalf("a clean %d-byte JSON body was not returned unchanged (%d bytes, %v)", len(body), len(got), err)
	}
}

// Member 3: a binary answer was checked, then wrapped in an envelope whose
// content_type is the response's Content-Type header, unchecked. The envelope
// as returned is what the check must see.
func TestSuccessFunnelClassBinaryEnvelopeIsChecked(t *testing.T) {
	srv, _ := newClassFunnelServer(t, "application/x-"+classFunnelSecret16, []byte("%PDF-1.7 plain bytes"))
	c := newClassFunnelClient(t, srv, classFunnelSecret16, false)
	got, err := c.Get(context.Background(), "/v1/reports/1", nil)
	if strings.Contains(string(got), classFunnelSecret16) {
		t.Fatalf("a binary envelope repeated a held credential from the Content-Type header: %s", got)
	}
	if err == nil {
		t.Fatalf("want a refusal, got %s", got)
	}
	// Control: an ordinary binary answer still arrives as an envelope.
	raw := []byte("%PDF-1.7 plain bytes")
	srv2, _ := newClassFunnelServer(t, "application/pdf", raw)
	c2 := newClassFunnelClient(t, srv2, classFunnelSecret16, false)
	env, err := c2.Get(context.Background(), "/v1/reports/1", nil)
	if err != nil {
		t.Fatalf("ordinary binary answer refused: %v", err)
	}
	if back, ct, ok := UnwrapBinaryResponse(env); !ok || string(back) != string(raw) || ct != "application/pdf" {
		t.Fatalf("ordinary binary envelope changed: %s", env)
	}
}

// Member 3 (envelope text over the cap): the envelope's base64 text is what a
// consumer reads, and it has views of its own. Here the bytes are chosen so
// their base64 text is a hex run that spells the credential with 550,000
// removable NULs between its halves: the bytes alone hold no echo, and the
// envelope text (over 1 MiB) splits it across any fixed window.
func TestSuccessFunnelClassLargeBinaryEnvelopeTextIsChecked(t *testing.T) {
	text := "61626364" + strings.Repeat("00", 550000) + "65666768"
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > successFunnelChunk {
		t.Fatalf("test bytes must stay under the cap, got %d", len(raw))
	}
	srv, _ := newClassFunnelServer(t, "application/octet-stream", raw)
	c := newClassFunnelClient(t, srv, classFunnelSecret8, false)
	got, err := c.Get(context.Background(), "/v1/reports/1", nil)
	if err == nil {
		t.Fatalf("a %d-byte envelope whose base64 text spells a held credential in hex was returned", len(got))
	}
}

// Guard: a JSON answer shaped like a binary envelope but carrying an extra
// field is checked whole, like any other answer; reading it as an envelope
// never drops a field from the check.
func TestSuccessFunnelClassEnvelopeShapedJSONIsCheckedWhole(t *testing.T) {
	body := `{"_pp_binary":true,"content_type":"application/pdf","encoding":"base64","bytes":5,"data":"aGVsbG8=","note":"` + classFunnelSecret16 + `"}`
	srv, _ := newClassFunnelServer(t, "application/json", []byte(body))
	c := newClassFunnelClient(t, srv, classFunnelSecret16, false)
	got, err := c.Get(context.Background(), "/v1/reports/1", nil)
	if err == nil || strings.Contains(string(got), classFunnelSecret16) {
		t.Fatalf("an envelope-shaped JSON answer with a credential was returned: %s (%v)", got, err)
	}
}
