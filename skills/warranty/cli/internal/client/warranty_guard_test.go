// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-host-pinning).

package client

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"warranty-pp-cli/internal/config"
)

type recordRT struct {
	got  []*http.Request
	resp func(*http.Request) *http.Response
}

func (r *recordRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = append(r.got, req)
	if r.resp != nil {
		return r.resp(req), nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}, Request: req}, nil
}

func guardedClient(t *testing.T, rt http.RoundTripper) *http.Client {
	t.Helper()
	c := &Client{BaseURL: "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5", Config: &config.Config{},
		HTTPClient: &http.Client{Transport: rt, Timeout: 5 * time.Second}}
	InstallGuard(c)
	if !c.NoCache {
		t.Fatal("InstallGuard must turn the response cache off")
	}
	return c.HTTPClient
}

func TestGuardRefusesOtherOrigins(t *testing.T) {
	rt := &recordRT{}
	hc := guardedClient(t, rt)
	for _, u := range []string{
		"https://evil.example/collect",
		"http://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/asset-entitlements", // scheme differs
		"https://apigtwb2c.us.dell.com:8443/PROD/sbil/eapi/v5/x",            // port differs
		"https://supportapi.lenovo.com.evil.example/v2.5/warranty",
	} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer secret-token-value")
		if _, err := hc.Do(req); !errors.Is(err, ErrHostNotAllowed) {
			t.Errorf("%s: err = %v, want ErrHostNotAllowed", u, err)
		}
	}
	if len(rt.got) != 0 {
		t.Fatalf("a refused request reached the transport: %d", len(rt.got))
	}
	req, _ := http.NewRequest(http.MethodGet, "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/asset-entitlements?servicetags=1ABC234", nil)
	if _, err := hc.Do(req); err != nil {
		t.Fatalf("the configured Dell origin was refused: %v", err)
	}
}

func TestGuardRefusesCrossOriginRedirect(t *testing.T) {
	rt := &recordRT{resp: func(req *http.Request) *http.Response {
		loc := "https://evil.example/steal"
		if strings.HasSuffix(req.URL.Path, "/same") {
			loc = "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/landing"
		}
		if strings.HasSuffix(req.URL.Path, "/landing") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`)), Header: http.Header{}, Request: req}
		}
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": {loc}}, Request: req}
	}}
	hc := guardedClient(t, rt)
	req, _ := http.NewRequest(http.MethodGet, "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/away", nil)
	if _, err := hc.Do(req); !errors.Is(err, ErrHostNotAllowed) {
		t.Fatalf("cross-origin redirect: err = %v", err)
	}
	for _, r := range rt.got {
		if r.URL.Host == "evil.example" {
			t.Fatal("the redirect target was dialed")
		}
	}
	req, _ = http.NewRequest(http.MethodGet, "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/same", nil)
	if resp, err := hc.Do(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("same-origin redirect: resp=%v err=%v", resp, err)
	}
}

func TestGuardVendorCredentials(t *testing.T) {
	t.Setenv(EnvAppleClientID, "")
	t.Setenv(EnvAppleKeyID, "")
	t.Setenv(EnvApplePrivateKey, "")
	rt := &recordRT{}
	hc := guardedClient(t, rt)
	req, _ := http.NewRequest(http.MethodGet, "https://supportapi.lenovo.com/v2.5/warranty?Serial=PF1ABCDE", nil)
	if _, err := hc.Do(req); !errors.Is(err, ErrVendorNotConfigured) {
		t.Errorf("Lenovo without ClientID: err = %v", err)
	}
	req, _ = http.NewRequest(http.MethodGet, "https://api-business.apple.com/v1/orgDevices", nil)
	if _, err := hc.Do(req); !errors.Is(err, ErrVendorNotConfigured) || !strings.Contains(err.Error(), EnvApplePrivateKey) {
		t.Errorf("Apple without credentials: err = %v", err)
	}
	if len(rt.got) != 0 {
		t.Fatalf("an unconfigured vendor was dialed: %d", len(rt.got))
	}
}

func TestOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://Api-Business.apple.com/v1":   AppleAPIOrigin,
		"https://supportapi.lenovo.com:443/x": LenovoOrigin,
		"http://127.0.0.1:8080/a":             "http://127.0.0.1:8080",
	} {
		if got := originOf(in); got != want {
			t.Errorf("originOf(%q) = %q want %q", in, got, want)
		}
	}
}

func TestGuardPinsCredentialHeadersPerVendor(t *testing.T) {
	t.Setenv(EnvLenovoClientKey, "lenovo-key-pin-0001")
	rt := &recordRT{}
	hc := guardedClient(t, rt)
	req, _ := http.NewRequest(http.MethodGet, "https://apigtwb2c.us.dell.com/PROD/sbil/eapi/v5/asset-entitlements", nil)
	req.Header.Set("Authorization", "Bearer dell")
	req.Header.Set("ClientID", "lenovo-key-pin-0001")
	req.Header.Set("X-Api-Key", "k")
	req.Header.Set("Accept", "application/json")
	if _, err := hc.Do(req); err != nil {
		t.Fatal(err)
	}
	got := rt.got[len(rt.got)-1].Header
	if got.Get("Authorization") == "" || got.Get("ClientID") != "" || got.Get("X-Api-Key") != "" || got.Get("Accept") == "" {
		t.Fatalf("Dell request headers = %v", got)
	}
	req, _ = http.NewRequest(http.MethodGet, "https://supportapi.lenovo.com/v2.5/warranty?Serial=PF1ABCDE", nil)
	req.Header.Set("ClientID", "lenovo-key-pin-0001")
	req.Header.Set("Authorization", "Bearer dell-token")
	if _, err := hc.Do(req); err != nil {
		t.Fatal(err)
	}
	got = rt.got[len(rt.got)-1].Header
	if got.Get("ClientID") != "lenovo-key-pin-0001" || got.Get("Authorization") != "" {
		t.Fatalf("Lenovo request headers = %v", got)
	}
}
