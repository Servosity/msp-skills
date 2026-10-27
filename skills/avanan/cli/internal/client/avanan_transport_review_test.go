// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"avanan-pp-cli/internal/config"
)

// handshakeRecorder answers every request from a function and records
// what reached each host, without touching the network.
type handshakeRecorder struct {
	mu     sync.Mutex
	hits   []string
	bodies []string
	reply  func(r *http.Request) *http.Response
}

func (rt *handshakeRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.hits = append(rt.hits, r.URL.Scheme+"://"+r.URL.Host+r.URL.Path)
	body := ""
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}
	rt.bodies = append(rt.bodies, body)
	rt.mu.Unlock()
	return rt.reply(r), nil
}

func textResponse(r *http.Request, status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

// TestHandshakeRefusesPlaintextNonLoopbackHost: the handshake carries the
// client secret (Infinity: in the body) or a replayable signature (legacy). It
// runs on its own http.Client, so the generated client's plaintext guard never
// saw it, and `auth login --base-url http://...` sent the credential in clear.
func TestHandshakeRefusesPlaintextNonLoopbackHost(t *testing.T) {
	for _, base := range []string{
		"http://cloudinfra-gw.portal.checkpoint.com",
		"http://smart-api-production-1-us.avanan.net",
	} {
		rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
			return textResponse(r, 200, `{"data":{"token":"tok","expiresIn":3600}}`, nil)
		}}
		tr, _ := newTestTransport(t, rt, base)
		req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
		if _, _, err := tr.Mint(req); err == nil {
			t.Errorf("%s: handshake over plaintext http succeeded; want refusal", base)
		} else if !errors.Is(err, ErrPlaintextCredential) {
			t.Errorf("%s: error = %v, want ErrPlaintextCredential", base, err)
		}
		if len(rt.hits) != 0 {
			t.Errorf("%s: handshake reached the wire over plaintext: %v", base, rt.hits)
		}
	}
}

// TestHandshakeNeverFollowsARedirect: the handshake client had Go's default
// redirect policy, so a 307 replayed {"accessKey": secret} (and a legacy
// signature) to whatever host the Location named.
func TestHandshakeNeverFollowsARedirect(t *testing.T) {
	for _, base := range []string{
		"https://cloudinfra-gw.portal.checkpoint.com",
		"https://smart-api-production-1-us.avanan.net",
	} {
		rt := &handshakeRecorder{}
		rt.reply = func(r *http.Request) *http.Response {
			if r.URL.Host == "evil.example" {
				return textResponse(r, 200, `{"data":{"token":"attacker","expiresIn":3600}}`, nil)
			}
			h := http.Header{}
			h.Set("Location", "https://evil.example/collect")
			return textResponse(r, http.StatusTemporaryRedirect, "", h)
		}
		tr, _ := newTestTransport(t, rt, base)
		req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
		_, _, err := tr.Mint(req)
		if err == nil {
			t.Errorf("%s: handshake followed a redirect and minted a token", base)
		}
		for i, hit := range rt.hits {
			if strings.Contains(hit, "evil.example") {
				t.Errorf("%s: redirect target received the handshake (body %q)", base, rt.bodies[i])
			}
		}
	}
}

// TestHandshakeErrorNeverEchoesTheVendorBody: scrub replaced only the raw
// secret, so an echo in any other encoding reached stderr. The handshake error
// carries a status and a fixed diagnostic, never the body.
func TestHandshakeErrorNeverEchoesTheVendorBody(t *testing.T) {
	secret := "abcd/12+3456=="
	encodings := []string{
		secret,
		url.QueryEscape(secret),
		url.PathEscape(secret),
		base64.StdEncoding.EncodeToString([]byte(secret)),
		strings.ReplaceAll(secret, "/", `\/`),
		`abcd/12+3456==`,
	}
	for _, base := range []string{
		"https://cloudinfra-gw.portal.checkpoint.com",
		"https://smart-api-production-1-us.avanan.net",
	} {
		for _, enc := range encodings {
			rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
				return textResponse(r, 400, `{"error":"bad accessKey=`+enc+`"}`, nil)
			}}
			tr, cfg := newTestTransport(t, rt, base)
			cfg.ClientSecret = secret
			req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
			_, _, err := tr.Mint(req)
			if err == nil {
				t.Fatalf("%s: handshake 400 returned no error", base)
			}
			if strings.Contains(err.Error(), enc) || strings.Contains(err.Error(), "bad accessKey") {
				t.Errorf("%s: handshake error echoes the vendor body (%q): %v", base, enc, err)
			}
			if !strings.Contains(err.Error(), "400") {
				t.Errorf("%s: handshake error lost the HTTP status: %v", base, err)
			}
		}
	}
}

// TestEnvelopeFailureIsNotAnHTTPSuccess: Avanan answers HTTP 200 with the real
// outcome in responseEnvelope.responseCode (spec components.schemas
// ResponseEnvelope). Every generated command read the HTTP status alone, so a
// rejected quarantine printed success and exited 0. The envelope is read from
// the decoded body: a gzip answer whose envelope failed was a success.
func TestEnvelopeFailureIsNotAnHTTPSuccess(t *testing.T) {
	for _, gz := range []bool{false, true} {
		testEnvelopeFailureIsNotAnHTTPSuccess(t, gz)
	}
}

func testEnvelopeFailureIsNotAnHTTPSuccess(t *testing.T, gz bool) {
	for _, tc := range []struct {
		body       string
		wantFailed bool
	}{
		{`{"responseEnvelope":{"responseCode":400,"responseText":"Invalid scope"},"responseData":[]}`, true},
		{`{"responseEnvelope":{"responseCode":500,"responseText":"internal_error"},"responseData":[]}`, true},
		{`{"responseEnvelope":{"responseCode":7,"responseText":"odd"},"responseData":[]}`, true},
		{`{"responseEnvelope":{"responseCode":200,"responseText":"OK"},"responseData":[{"taskId":"T"}]}`, false},
		{`{"responseEnvelope":{"responseCode":0},"responseData":[]}`, false},
		{`{"responseData":[]}`, false},
		{`not json at all`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if !gz {
				_, _ = w.Write([]byte(tc.body))
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = zw.Write([]byte(tc.body))
			_ = zw.Close()
		}))
		tr := &AvananTransport{base: http.DefaultTransport, cfg: &config.Config{BaseURL: srv.URL}}
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1.0/action/entity", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		// Set by hand, so the stdlib transport hands the gzip bytes through
		// as the generated client receives them.
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := (&http.Client{Transport: tr}).Do(req)
		if err != nil {
			srv.Close()
			t.Fatalf("request: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		srv.Close()
		failed := resp.StatusCode < 200 || resp.StatusCode >= 300
		if failed != tc.wantFailed {
			t.Errorf("gzip=%v body %s: HTTP status %d, want failed=%v", gz, tc.body, resp.StatusCode, tc.wantFailed)
		}
		if string(body) != tc.body {
			t.Errorf("body was altered: got %q want %q", body, tc.body)
		}
	}
}

// TestTransportNeverAttachesACredentialOverPlaintext: the generated client's
// plaintext guard decides "credentialed" from AuthHeader() (the app id), but
// the transport attaches the stored session token itself. With a token-only
// config, `http://<infinity gateway>` received the bearer token in clear.
func TestTransportNeverAttachesACredentialOverPlaintext(t *testing.T) {
	for _, cfg := range []*config.Config{
		{BaseURL: "http://cloudinfra-gw.portal.checkpoint.com", AvananToken: "stored-session-token", TokenExpiry: timeFarFuture()},
		{BaseURL: "http://smart-api-production-1-us.avanan.net", AvananAppId: "US:app", ClientSecret: "secret-value"},
	} {
		rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
			return textResponse(r, 200, `{"responseData":[]}`, nil)
		}}
		tr := &AvananTransport{base: rt, cfg: cfg}
		if cfg.AvananToken != "" {
			tr.token, tr.expiry = cfg.AvananToken, cfg.TokenExpiry
		}
		req, _ := http.NewRequest(http.MethodGet, cfg.BaseURL+"/v1.0/scopes", nil)
		resp, err := tr.RoundTrip(req)
		if err == nil {
			resp.Body.Close()
			t.Errorf("%s: credentialed request over plaintext http was sent", cfg.BaseURL)
		}
		if len(rt.hits) != 0 {
			t.Errorf("%s: reached the wire over plaintext: %v", cfg.BaseURL, rt.hits)
		}
	}
	// Loopback stays allowed (local test servers and proxies).
	rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
		return textResponse(r, 200, `{"responseData":[]}`, nil)
	}}
	tr := &AvananTransport{base: rt, cfg: &config.Config{BaseURL: "http://127.0.0.1:9", AvananToken: "tok", TokenExpiry: timeFarFuture()}}
	tr.token, tr.expiry = "tok", timeFarFuture()
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:9/v1.0/scopes", nil)
	if resp, err := tr.RoundTrip(req); err != nil {
		t.Errorf("loopback request refused: %v", err)
	} else {
		resp.Body.Close()
	}
}

// TestLegacyHandshakeRefusesAFailedEnvelope: an envelope-form handshake answer
// whose responseCode fails is not a token, whatever responseData holds.
func TestLegacyHandshakeRefusesAFailedEnvelope(t *testing.T) {
	rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
		return textResponse(r, 200, `{"responseEnvelope":{"responseCode":401,"responseText":"bad signature"},"responseData":"not-a-token"}`, nil)
	}}
	tr, _ := newTestTransport(t, rt, "https://smart-api-production-1-us.avanan.net")
	req, _ := http.NewRequest(http.MethodGet, "https://smart-api-production-1-us.avanan.net/v1.0/scopes", nil)
	if tok, _, err := tr.Mint(req); err == nil {
		t.Errorf("handshake with a failed envelope minted token %q", tok)
	}
}
