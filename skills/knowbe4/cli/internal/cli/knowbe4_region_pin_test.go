// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"knowbe4-pp-cli/internal/client"
	"knowbe4-pp-cli/internal/config"
)

type countingTripper struct{ hits []string }

func (c *countingTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	c.hits = append(c.hits, r.URL.Host)
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
}

// knowbe4 handfix region-origin-pinned: the generated base URL is the template
// https://{region}.api.knowbe4.com/v1. Pinning it unresolved
// refused every request to the documented default region; the pin is the
// resolved origin when the client is built, and a region changed afterwards
// (sync --path-context region=..., an MCP tool argument) cannot move the key.
func TestPinFollowsTheResolvedRegion(t *testing.T) {
	const base = "https://{region}.api.knowbe4.com/v1"
	rt := &countingTripper{}
	cfg := &config.Config{TemplateVars: map[string]string{"region": "eu"}}
	c := &client.Client{BaseURL: base, Config: cfg, HTTPClient: &http.Client{Transport: rt}}
	guardClient(c)

	req, _ := http.NewRequest(http.MethodGet, client.ResolvedBaseURL(base, cfg.TemplateVars)+"/agent", nil)
	if _, err := c.HTTPClient.Do(req); err != nil {
		t.Fatalf("the configured region's host was refused: %v", err)
	}

	cfg.TemplateVars["region"] = "evil.example/x?"
	req, _ = http.NewRequest(http.MethodGet, client.ResolvedBaseURL(base, cfg.TemplateVars)+"/agent", nil)
	if _, err := c.HTTPClient.Do(req); !errors.Is(err, ErrOriginChanged) {
		t.Fatalf("a region changed after the client was built moved the request: err %v", err)
	}
	if len(rt.hits) != 1 || rt.hits[0] != "eu.api.knowbe4.com" {
		t.Fatalf("transport saw %v, want only the configured region's host", rt.hits)
	}

	for region, want := range map[string]string{
		"us":              "https://us.api.knowbe4.com:443",
		"de":              "https://de.api.knowbe4.com:443",
		"evil.example/x?": "",
		"evil.example#":   "",
		"a@b":             "",
		"":                "",
		"x.attacker":      "",
	} {
		if got := pinnedOrigin(base, map[string]string{"region": region}); got != want {
			t.Errorf("region %q pinned %q, want %q", region, got, want)
		}
	}
	if got := pinnedOrigin("http://127.0.0.1:8080/v1", nil); !strings.HasPrefix(got, "http://127.0.0.1:8080") {
		t.Errorf("an override base URL (KNOWBE4_BASE_URL) pinned %q", got)
	}
}
