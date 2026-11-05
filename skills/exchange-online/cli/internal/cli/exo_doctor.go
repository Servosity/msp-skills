// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// exchange-online handfix (doctor-verdict-tracks-reality): doctor's verdicts come
// from one read-only cmdlet, not from a bare request to the host. Every Exchange
// call is a POST for a tenant, so the press's argument-free GET probe finds nothing,
// and GET / on outlook.office365.com only proves a web server answers (it redirects
// to a sign-in page, which the origin pin refuses). The tenant is the token's own
// tid claim, so the probe asks the question a real command would.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/config"
)

// doctorProbeCmdlet reads the tenant's accepted domains: every tenant has one, any
// Exchange role can read them, and the answer is short.
const doctorProbeCmdlet = "Get-AcceptedDomain"

// doctorNoTenant is the tenant asked about for a token that names none: the API
// refuses it, which is the honest answer for an opaque token.
const doctorNoTenant = "00000000-0000-0000-0000-000000000000"

// doctorTenant is the token's tid claim, or doctorNoTenant.
func doctorTenant(token string) string {
	token = strings.TrimSpace(token)
	if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			var claims struct {
				Tid string `json:"tid"`
			}
			if json.Unmarshal(payload, &claims) == nil && tenantRe.MatchString(claims.Tid) {
				return claims.Tid
			}
		}
	}
	return doctorNoTenant
}

// doctorFinishVerdicts sets the credentials row, and corrects the api row when the
// probe proves it wrong. reachErr is the bare request's outcome.
func doctorFinishVerdicts(ctx context.Context, c *client.Client, cfg *config.Config, reachErr error, report map[string]any) {
	// A redirect off the API host still means the host answered.
	if errors.Is(reachErr, ErrOriginChanged) {
		report["api"] = "reachable (it redirects / off the API host)"
		reachErr = nil
	}
	var reachAPIErr *client.APIError
	switch {
	case cfg.AuthHeader() == "":
		return // no credential: the auth row says so
	case errors.Is(reachErr, client.ErrPlaintextCredential):
		report["credentials"] = "not probed (plaintext HTTP); use https"
		return
	case reachErr != nil && !errors.As(reachErr, &reachAPIErr):
		report["credentials"] = "skipped (API unreachable)"
		return
	case cliutil.IsVerifyEnv():
		report["credentials"] = "WARN not verified: verify mode sends nothing"
		return
	}
	path := "/adminapi/beta/" + doctorTenant(jwtExpirySource(cfg)) + "/InvokeCommand#" + doctorProbeCmdlet
	_, _, err := c.Post(ctx, path, map[string]any{})
	var apiErr *client.APIError
	status := 0
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
	}
	switch {
	case err == nil:
		report["credentials"] = "valid (verified with " + doctorProbeCmdlet + ")"
	case status == 401:
		report["credentials"] = "ERROR rejected (HTTP 401 from " + doctorProbeCmdlet + "): the token is invalid, expired, or for another tenant or resource. Mint a new one with resource https://outlook.office365.com."
	case status == 403:
		report["credentials"] = "scope-limited (HTTP 403 from " + doctorProbeCmdlet + "): the token is accepted but lacks an Exchange role."
	case status == 404:
		report["credentials"] = "ERROR not verified (HTTP 404 from " + doctorProbeCmdlet + "): the admin API is not at this base_url, so the token was not tested."
		report["api"] = "ERROR the host answered but /adminapi/ is not there: base_url is not the Exchange Online API. Unset EXCHANGE_ONLINE_BASE_URL to use https://outlook.office365.com."
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s).", status, doctorProbeCmdlet)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
