// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor answered "reachable" whenever the configured host
// returned ANY HTTP response to `GET /`, and it never probed the credential:
// it printed "present, not verified" and left the operator to guess. The
// helpers here probe a real argument-free GET endpoint with the real (Duo
// v5-signed) credential and classify the answer by status code.
//
// See issue #282 and skills/cisco-duo/handfixes.json
// (doctor-verdict-tracks-reality). The placeholder refusal itself is emitted
// natively by printing-press 4.33 (doctorBaseURLIsPlaceholder in doctor.go).

package cli

import (
	"context"
	"errors"
	"fmt"

	"cisco-duo-pp-cli/internal/client"
)

// doctorBaseURLRemedy names the variables that point this connector at the
// operator's own Duo deployment. Duo's API root is per customer
// (https://api-XXXXXXXX.duosecurity.com), so the usual fix is the API host;
// CISCO_DUO_BASE_URL overrides the whole root.
const doctorBaseURLRemedy = "set CISCO_DUO_API_HOST to your Duo API hostname (api-XXXXXXXX.duosecurity.com, shown on the Admin API application page) or CISCO_DUO_BASE_URL to the full API root"

// doctorProbePath is the read the credential probe issues. Listing one user
// needs only "Grant resource - Read", the permission every Duo Admin API
// application for user management carries (lockout triage, phone-swap and
// the audits all need it), so a correctly scoped key is never misreported as
// scope-limited because the probe chose an admin-only endpoint.
const (
	doctorProbePath = "/admin/v1/users?limit=1"
	doctorProbeCmd  = "users list"
)

// doctorProbeCredentials issues ONE authenticated read and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic. client.ProbeGet goes through c.do, which bypasses the response
// cache, so a revoked key cannot be reported healthy from a cached 200.
func doctorProbeCredentials(ctx context.Context, c *client.Client, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	apiPath, cmdPath := doctorProbePath, doctorProbeCmd
	status, err := c.ProbeGet(ctx, apiPath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with a signed GET %s)", apiPath)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the integration key, secret key or API host is wrong, or the key belongs to a different Duo account.", apiPath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the keys work but the Admin API application lacks Grant resource - Read, which triage, audits and fixes all need.", apiPath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", apiPath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the Duo Admin API root; %s.", apiPath, doctorBaseURLRemedy)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - Duo returned a server error, so the credential was never checked.", status, apiPath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, apiPath, bin, cmdPath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorReachIsRefusedRedirect reports whether the reachability probe failed
// only because the server answered `/` with a redirect the client refuses to
// follow. The server responded, so the host is reachable and the credential
// probe decides.
func doctorReachIsRefusedRedirect(err error) bool {
	return errors.Is(err, client.ErrRedirectProtocolDowngrade) ||
		errors.Is(err, client.ErrRedirectUnsupportedScheme) ||
		errors.Is(err, client.ErrRedirectPrivateDestination)
}
