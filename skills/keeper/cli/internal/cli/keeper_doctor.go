// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// keeper handfix (doctor-verdict-tracks-reality): doctor's verdicts come from one
// authenticated read of Service Mode's queue status, not a bare request to the host:
// GET / only proves a web server answers. The read runs no Commander command, so it
// works under any -c allowlist and touches no vault or tenant.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"keeper-pp-cli/internal/client"
	"keeper-pp-cli/internal/cliutil"
	"keeper-pp-cli/internal/config"
)

const doctorProbePath = "/api/v2/queue/status"

// doctorFinishVerdicts sets the credentials row, and corrects the api row when the
// probe proves it wrong. reachErr is the bare request's outcome.
func doctorFinishVerdicts(ctx context.Context, c *client.Client, cfg *config.Config, reachErr error, report map[string]any) {
	var reachAPIErr *client.APIError
	switch {
	case cfg.AuthHeader() == "":
		return // no credential: the auth row says so
	case errors.Is(reachErr, client.ErrPlaintextCredential) || errors.Is(reachErr, ErrPlaintextKey):
		report["credentials"] = "not probed (plaintext HTTP); use https"
		return
	case reachErr != nil && !errors.As(reachErr, &reachAPIErr):
		report["credentials"] = "skipped (API unreachable)"
		return
	case cliutil.IsVerifyEnv():
		report["credentials"] = "WARN not verified: verify mode sends nothing"
		return
	}
	notServiceMode := "ERROR the host answered but " + doctorProbePath + " is not Service Mode's: base_url is not a Keeper Service Mode API. Set KEEPER_BASE_URL to your Service Mode host."
	body, err := c.Get(ctx, doctorProbePath, nil)
	var apiErr *client.APIError
	status := 0
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
	}
	switch {
	case err == nil && json.Valid(body):
		report["credentials"] = "valid (verified with GET " + doctorProbePath + ")"
	case err == nil:
		report["credentials"] = "ERROR not verified: the answer from " + doctorProbePath + " is not JSON."
		report["api"] = notServiceMode
	case status == 401:
		report["credentials"] = "ERROR rejected (HTTP 401 from " + doctorProbePath + "): the key is invalid or expired. service-create prints a new one; set KEEPER_API_KEY."
	case status == 403:
		report["credentials"] = "scope-limited (HTTP 403 from " + doctorProbePath + "): the key is accepted but refused this read."
	case status == 404 && doctorHealthOK(ctx, c):
		// keepercommander/service/api/routes.py: /health in both modes, /api/v2 only in queue mode.
		report["credentials"] = "WARN not verified (HTTP 404 from " + doctorProbePath + ", but /health answers): a Service Mode in legacy mode looks like this, and so does another server with a /health. `keeper-cli companies list` tests both the host and the key."
	case status == 404:
		report["credentials"] = "ERROR not verified (HTTP 404 from " + doctorProbePath + "): Service Mode is not at this base_url, so the key was not tested."
		report["api"] = notServiceMode
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s).", status, doctorProbePath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorHealthOK reports the liveness answer Service Mode gives in either mode.
func doctorHealthOK(ctx context.Context, c *client.Client) bool {
	body, err := c.Get(ctx, "/health", nil)
	var h struct {
		Status string `json:"status"`
	}
	return err == nil && json.Unmarshal(body, &h) == nil && h.Status == "ok"
}
