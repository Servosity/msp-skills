// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (NOT generated; handfixes.json credkit-wiring, no-response-cache,
// dry-run-sends-nothing): applied to every API client (CLI, novel commands and
// MCP) through registerClientHook, so no generated file changes.
//
//   - credkit-wiring: every request goes through the credential kit's
//     transport, which sends the Afi API key (the Authorization header) only
//     to the configured base URL (credwire.Declarations), never over plain
//     http and never across a redirect to another origin.
//   - no-response-cache: the on-disk response cache is off, so raw Afi bodies
//     never sit in cache files.
//   - dry-run-sends-nothing: under --dry-run the transport beneath the kit
//     refuses every request, so a missed preview branch fails closed.

import (
	"errors"
	"fmt"
	"net/http"

	"afi-pp-cli/internal/client"
	"afi-pp-cli/internal/credkit"
)

// ErrDryRunSend marks a request refused because the command runs under --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		kitClient(c)
		return nil
	})
}

func kitClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	c.NoCache = true
	if c.DryRun {
		c.HTTPClient.Transport = credkit.Transport(dryRunTripper{})
		return
	}
	c.HTTPClient.Transport = credkit.Transport(c.HTTPClient.Transport)
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s://%s)", ErrDryRunSend, req.Method, req.URL.Scheme, req.URL.Host)
}
