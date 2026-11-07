// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (NOT generated; handfixes.json credkit-wiring, no-response-cache,
// dry-run-sends-nothing, token-minted-per-client): applied to every API client
// (CLI, novel commands and MCP) through registerClientHook, so no generated
// file changes.
//
//   - credkit-wiring: every request goes through the credential kit's transport,
//     which sends every held value only to the API base URL this process uses
//     (credwire.Declarations) and holds the credentials a response declares.
//   - no-response-cache: the on-disk response cache is off, so raw Datto bodies
//     never sit in cache files.
//   - dry-run-sends-nothing: under --dry-run the transport beneath the kit
//     refuses every request, so a missed preview branch fails closed.
//   - token-minted-per-client: the hook then makes sure the client carries a
//     bearer for the host and API key in effect, minted through a token client
//     on this client's own trust settings. Never under --dry-run or the
//     verifier. The 4.24 print lost the 4.22 newClient mint: a key and secret
//     alone answered 401 until `auth login`, on the CLI and the MCP server alike.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"datto-rmm-pp-cli/internal/client"
	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/config"
)

// ErrDryRunSend marks a request refused because the command runs under --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		base := kitClient(c)
		return mintForClient(c, base)
	})
}

// kitClient routes c through the kit and returns the transport beneath it: the
// token client is built on that, with its own kit transport, so the fixed public
// OAuth client it adds below the kit is never read as a held credential.
func kitClient(c *client.Client) http.RoundTripper {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	c.NoCache = true
	if c.DryRun {
		c.HTTPClient.Transport = config.OutboundGuard(dryRunTripper{})
		return nil
	}
	base := c.HTTPClient.Transport
	c.HTTPClient.Transport = config.OutboundGuard(base)
	return base
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s://%s)", ErrDryRunSend, req.Method, req.URL.Scheme, req.URL.Host)
}

// dattoNoMint holds the configurations whose clients must not mint: an MCP
// call that only previews sends nothing, so it needs no token (datto-rmm handfix
// mcp-preview-never-mints).
var dattoNoMint sync.Map // *config.Config -> struct{}

// WithoutMintFor marks cfg so the clients built from it do not mint; the
// returned func clears the mark.
func WithoutMintFor(cfg *config.Config) func() {
	dattoNoMint.Store(cfg, struct{}{})
	return func() { dattoNoMint.Delete(cfg) }
}

// mintForClient puts a bearer for the host and API key in effect on c. Never
// under --dry-run, the verifier, or for a configuration marked by WithoutMintFor.
func mintForClient(c *client.Client, base http.RoundTripper) error {
	if c == nil || c.Config == nil || c.DryRun || cliutil.IsVerifyEnv() {
		return nil
	}
	if _, skip := dattoNoMint.Load(c.Config); skip {
		return nil
	}
	tokenClient := config.NewDattoTokenClient(c.BaseURL, base)
	if err := config.EnsureDattoTokenWith(context.Background(), tokenClient, c.Config); err != nil {
		return authErr(err)
	}
	return nil
}
