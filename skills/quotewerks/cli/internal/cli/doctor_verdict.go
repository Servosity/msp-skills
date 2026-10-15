// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the API key, so a base_url aimed at a web
// page (every API path 404s) and a revoked key both rendered healthy.
//
// GET /api/v1/qw/tables lists the tenant's tables, needs only a valid key (no
// write access), and is what `tables list` calls, so it is the probe.
//
// See issue #282 and skills/quotewerks/handfixes.json.

package cli

import (
	"context"
	"fmt"

	"quotewerks-pp-cli/internal/client"
)

// doctorProbePath / doctorProbeCmd: the read every QuoteWerks Web key can make,
// and the command that makes it, for the remedy line.
const (
	doctorProbePath = "/api/v1/qw/tables"
	doctorProbeCmd  = "tables list"
)

// doctorProbeCredentials issues ONE authenticated GET and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic. ProbeGet goes through c.do, which bypasses the response cache, so
// a revoked key cannot be reported healthy from a cached 200.
func doctorProbeCredentials(ctx context.Context, c *client.Client, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no API key to send, so any response would say nothing about authentication."
		return
	}
	status, err := c.ProbeGet(ctx, doctorProbePath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", doctorProbePath)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the API key is invalid, revoked, or not a QuoteWerks Web key. Create one under Setup > Options > Zapier and API.", doctorProbePath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the key is accepted but cannot list tables; the API requires a user with MasterRights.", doctorProbePath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the key was never checked.", doctorProbePath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the QuoteWerks Web API root. Unset QUOTEWERKS_BASE_URL to use https://qwwapi.quotewerks.com.", doctorProbePath)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - QuoteWerks Web returned a server error, so the key was never checked.", status, doctorProbePath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, doctorProbePath, bin, doctorProbeCmd)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
