// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"net/http"
	"testing"

	"aws-billing-pp-cli/internal/client"
)

// no-response-cache and dry-run-sends-nothing: every client the hooks see has
// its response cache off, and a --dry-run client's transport sends nothing.
func TestClientHookTurnsTheCacheOffAndDryRunSendsNothing(t *testing.T) {
	c := &client.Client{HTTPClient: &http.Client{Transport: http.DefaultTransport}, DryRun: true}
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if !c.NoCache {
		t.Error("the response cache is on")
	}
	req, _ := http.NewRequest(http.MethodPost, "https://ce.us-east-1.amazonaws.com/", nil)
	if _, err := c.HTTPClient.Transport.RoundTrip(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("a --dry-run client sent a request: %v", err)
	}
}
