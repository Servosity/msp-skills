// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// aws-billing handfixes applied to every generated API client (CLI, novel
// commands and MCP) through registerClientHook, so no generated file changes.
// The shared credential kit guards the client's sends (client.OutboundGuard);
// these two are not credential checks:
//
//   - no-response-cache: the on-disk response cache is off, so a cached body
//     never answers a request it was not the answer to.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"errors"
	"fmt"
	"net/http"

	"aws-billing-pp-cli/internal/client"
)

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		if c == nil || c.HTTPClient == nil {
			return nil
		}
		c.NoCache = true
		if c.DryRun {
			c.HTTPClient.Transport = dryRunTripper{}
		}
		return nil
	})
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s)", ErrDryRunSend, req.Method, req.URL.Host)
}
