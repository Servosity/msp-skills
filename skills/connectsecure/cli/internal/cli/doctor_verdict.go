// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor never checked the credential: it printed "present, not
// verified" and called any HTTP answer to `GET /` reachable, so a base_url aimed
// at the wrong host and a rejected token both read as healthy. These helpers
// probe a real authenticated endpoint and classify the answer by status code.
// See issue #282 and skills/connectsecure/handfixes.json.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectsecure-pp-cli/internal/client"
)

// doctorProbePath is the cheapest argument-free authenticated read: one company.
const doctorProbePath = "/r/company/companies"

// doctorProbeCredentials sets the credentials row (and the api row when the
// answer proves the base_url is not the pod API host).
func doctorProbeCredentials(ctx context.Context, c *client.Client, reachErr error, report map[string]any) {
	var apiErr *client.APIError
	if reachErr != nil && !errors.As(reachErr, &apiErr) {
		report["credentials"] = "skipped (API unreachable)"
		return
	}
	body, err := c.GetWithHeadersNoCache(ctx, doctorProbePath, map[string]string{"limit": "1"}, nil)
	switch {
	case err == nil:
		var env struct {
			Status  *bool           `json:"status"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if json.Unmarshal(body, &env) != nil || (env.Status == nil && !bytes.HasPrefix(bytes.TrimSpace(env.Data), []byte("["))) {
			// Neither the {"status": ...} envelope nor a data list: a gateway
			// page, a plaintext "Access denied", or `{}` proves nothing.
			report["credentials"] = fmt.Sprintf("ERROR not verified: GET %s answered HTTP 200 without a ConnectSecure JSON answer - check CONNECTSECURE_BASE_URL is your pod API URL.", doctorProbePath)
			return
		}
		if env.Status != nil && !*env.Status {
			report["credentials"] = fmt.Sprintf("ERROR rejected: GET %s returned status false (%s).", doctorProbePath, c.SafeServerText(env.Message))
			return
		}
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", doctorProbePath)
	case errors.As(err, &apiErr):
		switch status := apiErr.StatusCode; {
		case status < 300: // a 200 {"status": false} whose message echoed a secret (body withheld)
			report["credentials"] = fmt.Sprintf("ERROR rejected: GET %s returned status false (%s).", doctorProbePath, apiErr.Body)
		case status == 401:
			report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the token is invalid or expired; check CONNECTSECURE_TENANT, CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET.", doctorProbePath)
		case status == 403:
			report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the token is accepted but this API user cannot read companies.", doctorProbePath)
		case status == 404:
			report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", doctorProbePath)
			report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - set CONNECTSECURE_BASE_URL to your pod API URL (Profile > API Documentation).", doctorProbePath)
		default:
			report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, doctorProbePath)
		}
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
