// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for superops_url_redact.go
// (superops-base-url-never-printed).

package client

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"

	"superops-pp-cli/internal/config"
)

const (
	chainTestUser  = "chainUserQe41"
	chainTestPass  = "chainPassWr52"
	chainTestToken = "sop-chain-token-6c3d"
)

func chainTestClient() *Client {
	cfg := &config.Config{
		BaseURL:          "http://" + chainTestUser + ":" + chainTestPass + "@127.0.0.1:9",
		SuperopsApiToken: chainTestToken,
	}
	return &Client{BaseURL: cfg.BaseURL, Config: cfg}
}

// net/http's url.Error keeps the username. Removing it must not cut the
// chain: a refused connection still reads as a *net.OpError (data-source
// fallback) and a timeout still is context.DeadlineExceeded (workflow
// timeouts).
func TestMaskErrorKeepsTheChainWhenOnlyUserinfoIsRemoved(t *testing.T) {
	c := chainTestClient()
	shown := "http://" + chainTestUser + ":***@127.0.0.1:9/msp"
	refused := &url.Error{Op: "Post", URL: shown, Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}}
	got := c.maskError(refused)
	if strings.Contains(got.Error(), chainTestUser) || !strings.Contains(got.Error(), `"http://127.0.0.1:9/msp"`) {
		t.Fatalf("maskError text = %q", got.Error())
	}
	var opErr *net.OpError
	if !errors.As(got, &opErr) {
		t.Fatalf("a refused connection no longer unwraps to *net.OpError: %#v", got)
	}
	timeout := &url.Error{Op: "Post", URL: shown, Err: context.DeadlineExceeded}
	if got := c.maskError(timeout); !errors.Is(got, context.DeadlineExceeded) || strings.Contains(got.Error(), chainTestUser) {
		t.Fatalf("a timeout lost its class or kept the username: %v", got)
	}
}

// When a credential was masked too, nothing unwraps to the original wording.
func TestMaskErrorCutsTheChainWhenACredentialWasMasked(t *testing.T) {
	c := chainTestClient()
	echoed := &url.Error{Op: "Get", URL: "http://" + chainTestUser + ":***@127.0.0.1:9/msp?token=" + chainTestToken, Err: errors.New("boom")}
	got := c.maskError(echoed)
	var ue *url.Error
	if errors.As(got, &ue) {
		t.Fatalf("an error whose credential was masked still unwraps to the original: %v", ue)
	}
	for _, secret := range []string{chainTestUser, chainTestToken} {
		if strings.Contains(got.Error(), secret) {
			t.Fatalf("maskError kept %q: %q", secret, got.Error())
		}
	}
}

// The client's display text: a URL loses userinfo, query and fragment; the
// userinfo is masked wherever a server echoes it.
func TestClientDisplayNeverShowsBaseURLUserinfo(t *testing.T) {
	c := chainTestClient()
	if got := c.displayURL(c.BaseURL + "/msp"); got != "http://127.0.0.1:9/msp" {
		t.Fatalf("displayURL = %q", got)
	}
	if got := c.displayURL("/msp"); got != "/msp" {
		t.Fatalf("displayURL(path) = %q", got)
	}
	for _, echo := range []string{
		"user " + chainTestUser + " is not allowed",
		"password " + chainTestPass + " is wrong",
		"Basic Y2hhaW5Vc2VyUWU0MTpjaGFpblBhc3NXcjUy",
	} {
		if got := c.maskCredentialText(echo); strings.Contains(got, chainTestUser) || strings.Contains(got, chainTestPass) || strings.Contains(got, "Y2hhaW5Vc2VyUWU0MTpjaGFpblBhc3NXcjUy") {
			t.Fatalf("maskCredentialText(%q) = %q", echo, got)
		}
	}
}
