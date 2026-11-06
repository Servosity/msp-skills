// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (NOT generated): rewst hand-fixes applied to every API client
// (CLI, novel commands and MCP) through registerClientHook, so no generated
// file changes. The credential kit's outbound guard is already the client's
// transport (client.New, handfixes.json credkit-wiring): it sends the Bearer
// token only to the base URL the program uses (credwire.Declarations).
//
//   - no-response-cache: the on-disk response cache is off, so raw Rewst
//     bodies (an org variable's value included) never sit in cache files.
//   - dry-run-sends-nothing: under --dry-run the transport beneath the kit
//     refuses every request, so a missed preview branch fails closed.
//   - graphql-errors-are-failures: a 2xx GraphQL answer carrying errors is
//     rewritten to an HTTP failure (rewst_graphql_errors.go), outside the kit,
//     so it reads the answer the kit has already checked.

import (
	"errors"
	"fmt"
	"net/http"

	"rewst-pp-cli/internal/client"
	"rewst-pp-cli/internal/credkit"
)

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(graphQLErrorsTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	next := c.HTTPClient.Transport
	if c.DryRun {
		next = credkit.Transport(dryRunTripper{})
	}
	c.HTTPClient.Transport = graphQLErrorsTripper{next: next} // rewst handfix graphql-errors-are-failures
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s://%s)", ErrDryRunSend, req.Method, req.URL.Scheme, req.URL.Host)
}
