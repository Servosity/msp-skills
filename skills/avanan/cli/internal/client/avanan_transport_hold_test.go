// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package client

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"avanan-pp-cli/internal/config"
	"avanan-pp-cli/internal/credkit"
)

// funnelHolds reports whether the credential kit withholds text that repeats v.
func funnelHolds(v string) bool {
	dir, err := os.MkdirTemp("", "hold-probe-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	return errors.Is(credkit.WriteFile(filepath.Join(dir, "probe"), []byte("<html><title>"+v+"</title></html>"), 0o600), credkit.ErrWithheld)
}

// TestTransportHoldsEveryMintedTokenBeforeUsingIt: every session token the
// transport mints, on either handshake, is registered with the process-wide
// funnel before the request that carries it is sent. The CLI's and MCP
// server's error backstops withhold only held values, and maskErrorBody only
// rewrites non-2xx bodies, so an unheld minted token echoed in a 2xx HTML
// title reached the CLI error and the MCP result.
func TestTransportHoldsEveryMintedTokenBeforeUsingIt(t *testing.T) {
	for _, tc := range []struct {
		name, base, authPath, authBody, token string
	}{
		{
			name: "legacy handshake", base: "https://smart-api-production-1-us.avanan.net",
			authPath: "/v1.0/auth", token: "hold-legacy-minted-AAA/0123456789+abc",
		},
		{
			name: "infinity handshake", base: "https://cloudinfra-gw.portal.checkpoint.com",
			authPath: "/v2/auth/external", token: "hold-infinity-minted-BBB/0123456789+abc",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authBody := tc.token
			if tc.authPath == "/v2/auth/external" {
				authBody = `{"data":{"token":"` + tc.token + `","expiresIn":3600}}`
			}
			var mu sync.Mutex
			heldWhenSent := map[string]bool{}
			rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
				if r.URL.Path == tc.authPath {
					return textResponse(r, http.StatusOK, authBody, nil)
				}
				mu.Lock()
				heldWhenSent[r.URL.Path] = funnelHolds(tc.token)
				mu.Unlock()
				return textResponse(r, http.StatusOK, "<html><title>"+tc.token+"</title></html>", http.Header{"Content-Type": {"text/html"}})
			}}
			tr, _ := newTestTransport(t, rt, tc.base)
			req, _ := http.NewRequest(http.MethodGet, tc.base+"/v1.0/scopes", nil)
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			drainAndClose(resp)
			if len(heldWhenSent) != 1 {
				t.Fatalf("expected exactly one data request, saw %v", heldWhenSent)
			}
			for path, held := range heldWhenSent {
				if !held {
					t.Errorf("the minted token was not held when %s was sent with it", path)
				}
			}
		})
	}
}

// TestTransportMintHoldsTheToken: Mint (the `auth login` path) registers the
// token it mints, on either handshake, before handing it back.
func TestTransportMintHoldsTheToken(t *testing.T) {
	for _, tc := range []struct {
		name, base, authBody, token string
	}{
		{"legacy", "https://smart-api-production-1-us.avanan.net", "hold-mint-legacy-CCC/0123456789+abc", "hold-mint-legacy-CCC/0123456789+abc"},
		{"infinity", "https://cloudinfra-gw.portal.checkpoint.com", `{"data":{"token":"hold-mint-infinity-DDD/0123456789+abc","expiresIn":3600}}`, "hold-mint-infinity-DDD/0123456789+abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
				return textResponse(r, http.StatusOK, tc.authBody, nil)
			}}
			tr, _ := newTestTransport(t, rt, tc.base)
			req, _ := http.NewRequest(http.MethodGet, tc.base+"/v1.0/scopes", nil)
			tok, _, err := tr.Mint(req)
			if err != nil {
				t.Fatalf("Mint: %v", err)
			}
			if tok != tc.token {
				t.Fatalf("Mint returned %q, want %q", tok, tc.token)
			}
			if !funnelHolds(tc.token) {
				t.Errorf("Mint returned a token the process-wide funnel does not hold")
			}
		})
	}
}

// TestTransportHoldsItsConfiguredCredentials: the app id, the secret and the
// stored session token (from the config or credentials file) are held by the
// transport itself, not only when the generated client.New happened to run
// first: InstallAvananAuth registers them, and so does the first RoundTrip of
// a transport built any other way, before the request leaves.
func TestTransportHoldsItsConfiguredCredentials(t *testing.T) {
	t.Run("InstallAvananAuth", func(t *testing.T) {
		cfg := &config.Config{
			BaseURL:      "https://smart-api-production-1-us.avanan.net",
			AvananAppId:  "hold-install-app-EEE-0001",
			ClientSecret: "hold-install-secret-EEE-0001",
			AvananToken:  "hold-install-stored-token-EEE-0001",
		}
		c := &Client{Config: cfg, HTTPClient: &http.Client{}}
		if err := InstallAvananAuth(c); err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{cfg.AvananAppId, cfg.ClientSecret, cfg.AvananToken} {
			if !funnelHolds(v) {
				t.Errorf("InstallAvananAuth left %q unheld", v)
			}
		}
	})
	t.Run("first RoundTrip", func(t *testing.T) {
		const base = "https://smart-api-production-1-us.avanan.net"
		var heldAtSend []bool
		var tr *AvananTransport
		var cfg *config.Config
		rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
			heldAtSend = []bool{funnelHolds(cfg.AvananAppId), funnelHolds(cfg.ClientSecret), funnelHolds(cfg.AvananToken)}
			return textResponse(r, http.StatusOK, `{"responseData":[]}`, http.Header{"Content-Type": {"application/json"}})
		}}
		tr, cfg = newTestTransport(t, rt, base)
		cfg.AvananAppId = "hold-roundtrip-app-FFF-0002"
		cfg.ClientSecret = "hold-roundtrip-secret-FFF-0002"
		cfg.AvananToken = "hold-roundtrip-stored-token-FFF-0002"
		tr.token, tr.expiry = cfg.AvananToken, timeFarFuture()
		req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		drainAndClose(resp)
		if len(heldAtSend) != 3 || !heldAtSend[0] || !heldAtSend[1] || !heldAtSend[2] {
			t.Errorf("configured credentials held when the request was sent (app id, secret, stored token) = %v, want all true", heldAtSend)
		}
	})
}

// TestTransportHoldsTheHandshakeSignature: on legacy hosts the handshake
// carries no token, so its x-av-sig is the credential on that leg (replayable
// within the vendor's clock window). It is held before the handshake is sent.
func TestTransportHoldsTheHandshakeSignature(t *testing.T) {
	const base = "https://smart-api-production-1-us.avanan.net"
	sigHeld := map[string]bool{}
	rt := &handshakeRecorder{reply: func(r *http.Request) *http.Response {
		if r.URL.Path == "/v1.0/auth" {
			sig := r.Header.Get("x-av-sig")
			sigHeld[sig] = funnelHolds(sig)
			return textResponse(r, http.StatusOK, "hold-sig-minted-GGG/0123456789+abc", nil)
		}
		return textResponse(r, http.StatusOK, `{"responseData":[]}`, http.Header{"Content-Type": {"application/json"}})
	}}
	tr, cfg := newTestTransport(t, rt, base)
	cfg.AvananAppId = "hold-sig-app-GGG-0003"
	cfg.ClientSecret = "hold-sig-secret-GGG-0003"
	req, _ := http.NewRequest(http.MethodGet, base+"/v1.0/scopes", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	drainAndClose(resp)
	if len(sigHeld) != 1 {
		t.Fatalf("expected one handshake, saw %d", len(sigHeld))
	}
	for sig, held := range sigHeld {
		if sig == "" || !held {
			t.Errorf("handshake signature %q held when sent = %v, want a held signature", sig, held)
		}
	}
}
