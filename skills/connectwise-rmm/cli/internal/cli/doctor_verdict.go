// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor never checked the credential: it printed "present, not
// verified" and called any HTTP answer to `GET /` reachable, so a base_url aimed
// at the wrong host and a rejected client secret both read as healthy. It also
// read "auth: not configured" for an operator who had set the client ID and
// secret, because the client-credentials pair only becomes a bearer token after
// the first mint. These helpers probe a real authenticated endpoint and classify
// the answer by status code. See issue #282 and skills/connectwise-rmm/handfixes.json.

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectwise-rmm-pp-cli/internal/client"
	"connectwise-rmm-pp-cli/internal/config"
)

// doctorProbePath needs a valid bearer token but no OAuth scope, so it checks
// the credential without depending on which scopes the API key was granted.
const doctorProbePath = "/v1/rate_limits"

// doctorClientCredentialsConfigured reports whether the operator supplied the
// client ID and secret the CLI mints its bearer token from.
func doctorClientCredentialsConfigured(cfg *config.Config) bool {
	id, secret := cfg.AsioClientCredentials()
	return id != "" && secret != ""
}

// doctorProbeCredentials sets the credentials row (and the api row when the
// answer proves the base_url is wrong). reachErr is the outcome of the bare
// reachability request, which is also where the first token mint happens.
func doctorProbeCredentials(ctx context.Context, c *client.Client, cfg *config.Config, reachErr error, report map[string]any) {
	if cfg.AuthHeader() == "" && !doctorClientCredentialsConfigured(cfg) {
		report["credentials"] = "ERROR not verified: no credential is configured; set CONNECTWISE_RMM_CLIENT_ID and CONNECTWISE_RMM_CLIENT_SECRET."
		return
	}
	var apiErr *client.APIError
	if reachErr != nil && !errors.As(reachErr, &apiErr) {
		if strings.Contains(reachErr.Error(), "client_credentials mint: HTTP") {
			report["api"] = "reachable (the token endpoint answered; see credentials)"
			report["credentials"] = fmt.Sprintf("ERROR rejected: the token request failed (%s). Check the client ID and secret; if they are right, run `connectwise-rmm-cli scopes`, because one scope the key was not granted fails the whole token request.", reachErr)
			return
		}
		report["credentials"] = "skipped (API unreachable)"
		return
	}
	_, err := c.GetWithHeadersNoCache(ctx, doctorProbePath, nil, nil)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", doctorProbePath)
	case errors.As(err, &apiErr):
		switch status := apiErr.StatusCode; {
		case status == 401:
			report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the token is invalid, expired, or for a different region.", doctorProbePath)
		case status == 403:
			report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the token is accepted but lacks permission for this endpoint.", doctorProbePath)
		case status == 404:
			report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", doctorProbePath)
			report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the ConnectWise RMM partner API root. Unset CONNECTWISE_RMM_BASE_URL to follow your client ID's region, or point it at the API root.", doctorProbePath)
		default:
			report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, doctorProbePath)
		}
	case strings.Contains(err.Error(), "client_credentials mint: HTTP"):
		report["credentials"] = fmt.Sprintf("ERROR rejected: the token request failed (%s). Check the client ID and secret; if they are right, run `connectwise-rmm-cli scopes`.", err)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
