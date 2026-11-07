// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"cork-pp-cli/internal/config"
)

// cork handfix: redirects-never-leave-the-origin. A credential goes only to the
// configured scheme, host and port. Dropping Authorization on a cross-origin hop
// is not enough: a 307/308 replays the POST body (integrations connect carries
// credential_fields), so every hop off the origin is refused outright.
func TestRedirectNeverLeavesTheOrigin(t *testing.T) {
	c := New(&config.Config{BaseURL: "https://api.corkinc.com/api/v1"}, time.Second, 0)
	first, _ := http.NewRequest(http.MethodPost, "https://api.corkinc.com/api/v1/integrations", strings.NewReader(`{"credential_fields":{"api_key":"s"}}`))
	for _, target := range []string{
		"https://93.184.215.14/collect",         // another host (public literal: no DNS)
		"https://api.corkinc.com:8443/api/v1/x", // same host, another port
		"http://api.corkinc.com/api/v1/x",       // same host, another scheme
	} {
		next, _ := http.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		err := c.HTTPClient.CheckRedirect(next, []*http.Request{first})
		if !errors.Is(err, ErrRedirectLeavesOrigin) {
			t.Errorf("redirect to %s: got %v, want ErrRedirectLeavesOrigin", target, err)
		}
	}
	same, _ := http.NewRequest(http.MethodGet, "https://API.corkinc.com:443/api/v1/clients?page=2", nil)
	if err := c.HTTPClient.CheckRedirect(same, []*http.Request{first}); err != nil {
		t.Errorf("same-origin redirect refused: %v", err)
	}
	// The guard installed on a bare client (guardHTTPClient) must refuse too.
	bare := &http.Client{}
	guardHTTPClient(bare, "https://api.corkinc.com/api/v1")
	off, _ := http.NewRequest(http.MethodGet, "https://93.184.215.14/collect", nil)
	if err := bare.CheckRedirect(off, []*http.Request{first}); !errors.Is(err, ErrRedirectLeavesOrigin) {
		t.Errorf("guardHTTPClient: got %v, want ErrRedirectLeavesOrigin", err)
	}
}
