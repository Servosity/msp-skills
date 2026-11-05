// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: doctor-verdict-tracks-reality), ported from
// skills/watchguard/cli/internal/cli/doctor_verdict.go.
//
// The generated doctor probed GET /v2/user but called any answer other than
// 401 or 403 "not verified" while leaving the api row at "reachable (HTTP 404
// at /)", so a base_url aimed at the wrong host read as a healthy API
// (tools/maintainer/check_doctor_truth.py, issue #282). This probe classifies
// the authenticated answer and turns the api row red when the endpoint is not
// there.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"cloudally-pp-cli/internal/client"
)

// doctorProbePath is CloudAlly's own-user record. It takes no argument, so it
// works for an account login and a partner login alike.
const doctorProbePath = "/v2/user"

const doctorWrongBaseHint = "base_url is not the CloudAlly API root. Unset CLOUDALLY_BASE_URL (the default is https://api.cloudally.com) or point it at the CloudAlly API, and check base_url in the file named by config_path."

// doctorProbeCredentials sets the credentials row, and the api row when the
// answer proves base_url is not the CloudAlly API.
func doctorProbeCredentials(ctx context.Context, c *client.Client, report map[string]any) {
	headers := map[string]string{"User-Agent": "cloudally-cli"}
	body, err := c.GetWithHeadersNoCache(ctx, doctorProbePath, map[string]string{}, headers)
	var apiErr *client.APIError
	switch {
	case err == nil:
		if !doctorLooksLikeAPIAnswer(body) {
			// An app shell, a gateway page or `{}` proves nothing.
			report["credentials"] = fmt.Sprintf("ERROR not verified: GET %s answered without a CloudAlly JSON answer - %s", doctorProbePath, doctorWrongBaseHint)
			report["api"] = fmt.Sprintf("ERROR the host answered but %s did not return CloudAlly JSON - %s", doctorProbePath, doctorWrongBaseHint)
			return
		}
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", doctorProbePath)
	case errors.As(err, &apiErr):
		switch status := apiErr.StatusCode; {
		case status == 401:
			report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the access token is invalid or expired; run `cloudally-cli auth login` (or `auth login --partner`) again, and check CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET.", doctorProbePath)
		case status == 403:
			report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the token is accepted but cannot read this resource; check the API application's access.", doctorProbePath)
		case status == 404:
			report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", doctorProbePath)
			report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - %s", doctorProbePath, doctorWrongBaseHint)
		default:
			report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, doctorProbePath)
		}
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorLooksLikeAPIAnswer: a JSON array, or a JSON object with at least one
// member. Every CloudAlly read answers one of those.
func doctorLooksLikeAPIAnswer(body []byte) bool {
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return false
	}
	switch b[0] {
	case '[':
		return json.Valid(b)
	case '{':
		var m map[string]json.RawMessage
		return json.Unmarshal(b, &m) == nil && len(m) > 0
	}
	return false
}
