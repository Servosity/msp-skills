// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jira-service-management-pp-cli/internal/config"
)

// Handfix jsm-config-headers-stay-on-origin: a header from the config file's
// [headers] table is credential material whatever its name. It never follows a
// redirect off the configured origin and is never sent over plaintext http to a
// non-loopback host.
func TestConfiguredHeadersNeverLeaveTheOrigin(t *testing.T) {
	cfg := &config.Config{BaseURL: "https://acme.atlassian.net/rest/servicedeskapi", Headers: map[string]string{"X-Client-Token": "secret"}}
	c := New(cfg, 5*time.Second, 0)
	orig := httptest.NewRequest(http.MethodGet, "https://acme.atlassian.net/rest/servicedeskapi/request/SD-1", nil)
	orig.Header.Set("X-Client-Token", "secret")
	for _, target := range []string{"https://203.0.113.10/collect", "http://acme.atlassian.net/rest/servicedeskapi/request/SD-1", "https://acme.atlassian.net:8443/x"} {
		next := httptest.NewRequest(http.MethodGet, target, nil)
		next.Header = orig.Header.Clone() // what net/http copies onto the hop
		next.Header.Set("Accept", "application/json")
		if err := c.HTTPClient.CheckRedirect(next, []*http.Request{orig}); err != nil {
			continue // refused outright: nothing is sent
		}
		if v := next.Header.Get("X-Client-Token"); v != "" {
			t.Errorf("redirect to %s carries the configured header X-Client-Token=%q", target, v)
		}
		if next.Header.Get("Accept") == "" {
			t.Errorf("redirect to %s lost the non-secret Accept header", target)
		}
	}
}

type recordingTransport struct{ sent bool }

func (r *recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.sent = true
	return nil, errors.New("recorded")
}

func TestConfiguredHeaderRefusedOverPlaintext(t *testing.T) {
	cfg := &config.Config{BaseURL: "http://203.0.113.10/rest/servicedeskapi", Headers: map[string]string{"X-Client-Token": "secret"}}
	c := New(cfg, 2*time.Second, 0)
	rt := &recordingTransport{}
	c.HTTPClient.Transport = rt
	c.NoCache = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.Get(ctx, "/request/SD-1", nil)
	if rt.sent {
		t.Fatalf("the configured header was sent over plaintext http to a non-loopback host (err %v)", err)
	}
	if !errors.Is(err, ErrPlaintextCredential) {
		t.Errorf("err = %v, want ErrPlaintextCredential", err)
	}
}

// A configured value under a name the allowlist keeps (User-Agent,
// X-ExperimentalApi) is still the operator's secret and never leaves the origin.
func TestConfiguredHeaderUnderAllowlistedNameNeverLeavesTheOrigin(t *testing.T) {
	cfg := &config.Config{BaseURL: "https://acme.atlassian.net/rest/servicedeskapi", Headers: map[string]string{"user-agent": "secret-ua", "X-ExperimentalApi": "secret-x"}}
	c := New(cfg, 5*time.Second, 0)
	orig := httptest.NewRequest(http.MethodGet, "https://acme.atlassian.net/rest/servicedeskapi/request/SD-1", nil)
	orig.Header.Set("User-Agent", "secret-ua")
	orig.Header.Set("X-ExperimentalApi", "secret-x")
	next := httptest.NewRequest(http.MethodGet, "https://203.0.113.10/collect", nil)
	next.Header = orig.Header.Clone()
	if err := c.HTTPClient.CheckRedirect(next, []*http.Request{orig}); err != nil {
		return
	}
	for _, name := range []string{"User-Agent", "X-ExperimentalApi"} {
		if v := next.Header.Get(name); v != "" {
			t.Errorf("redirect off the origin carries configured %s=%q", name, v)
		}
	}
}
