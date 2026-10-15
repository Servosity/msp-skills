// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: doctor-verdict-tracks-reality), ported from
// skills/connectsecure/cli/internal/cli/doctor_verdict.go.
//
// The generated doctor never checked the credential: it printed "present, not
// verified" and called any HTTP answer to `GET /` reachable, so a base_url
// aimed at the wrong host and a rejected token both read as healthy
// (tools/maintainer/check_doctor_truth.py, issue #282). These helpers probe a
// real authenticated WatchGuard Cloud endpoint and classify the answer.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/config"
)

const (
	// doctorAccountProbePath reads the configured account's own record, which
	// any API key can read for its own account; it also proves
	// WATCHGUARD_ACCOUNT_ID.
	doctorAccountProbePath = "/rest/platform/accounts/v2/accounts/{accountId}"
	// doctorTokenProbePath takes no path parameter (the account comes from
	// the token), for a process with no WATCHGUARD_ACCOUNT_ID.
	doctorTokenProbePath = "/rest/platform/activation/v1/recentactivations"
)

const doctorWrongBaseHint = "base_url is not the WatchGuard Cloud API root. Point WATCHGUARD_BASE_URL at your region's API (https://api.usa.cloud.watchguard.com, https://api.deu.cloud.watchguard.com or https://api.jpn.cloud.watchguard.com) or unset it, and check base_url in the file named by config_path."

// doctorProbePath picks the authenticated GET doctor sends.
func doctorProbePath(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.TemplateVars["accountId"]) != "" {
		return doctorAccountProbePath
	}
	return doctorTokenProbePath
}

// doctorProbeCredentials sets the credentials row (and the api row when the
// answer proves the base_url is not the WatchGuard Cloud API).
func doctorProbeCredentials(ctx context.Context, c *client.Client, cfg *config.Config, report map[string]any) {
	path := doctorProbePath(cfg)
	body, err := c.GetWithHeadersNoCache(ctx, path, nil, nil)
	var apiErr *client.APIError
	switch {
	case err == nil:
		if !doctorLooksLikeAPIAnswer(body) {
			// A gateway page, a plaintext "Access denied" or `{}` proves nothing.
			report["credentials"] = fmt.Sprintf("ERROR not verified: GET %s answered without a WatchGuard Cloud JSON answer - %s", path, doctorWrongBaseHint)
			return
		}
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", path)
	case errors.As(err, &apiErr):
		switch status := apiErr.StatusCode; {
		case status == 401:
			report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the token or WatchGuard-API-Key is invalid or expired; check WATCHGUARD_CLIENT_ID, WATCHGUARD_CLIENT_SECRET and WATCHGUARD_API_KEY.", path)
		case status == 403:
			report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the credentials are accepted but cannot read this resource; check the API key's access and WATCHGUARD_ACCOUNT_ID.", path)
		case status == 404:
			report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url (or WATCHGUARD_ACCOUNT_ID names no account), so the credential was never checked.", path)
			report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - %s", path, doctorWrongBaseHint)
		default:
			report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, path)
		}
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorLooksLikeAPIAnswer: a JSON array, or a JSON object with at least one
// member. Every WatchGuard Cloud read answers one of those.
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
