// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, so a base_url aimed at the
// FortiGate's web UI (every API path 404s) and a revoked REST API token both
// rendered healthy. The placeholder branch also never named FORTINET_BASE_URL.
//
// FortiOS needs no endpoint discovery: monitor/system/status is readable by any
// REST API admin regardless of its admin profile, so it is the probe.
//
// See issue #282 and skills/fortinet/handfixes.json.

package cli

import (
	"context"
	"fmt"

	"fortinet-pp-cli/internal/client"
)

// doctorBaseURLEnv is named in remedies so the operator is told the fix.
const doctorBaseURLEnv = "FORTINET_BASE_URL"

// doctorProbePath / doctorProbeCmd: the read every FortiOS token can make, and
// the command that makes it, for the remedy line.
const (
	doctorProbePath = "/monitor/system/status"
	doctorProbeCmd  = "monitor status"
)

// doctorProbeCredentials issues ONE authenticated GET and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic. ProbeGet goes through c.do, which bypasses the response cache, so
// a token revoked on the FortiGate cannot be reported healthy from a cached 200.
func doctorProbeCredentials(ctx context.Context, c *client.Client, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	status, err := c.ProbeGet(ctx, doctorProbePath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", doctorProbePath)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the REST API token is invalid, revoked, or this host is not in the API admin's trusted hosts.", doctorProbePath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the token is accepted but its admin profile cannot read system status.", doctorProbePath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the token was never checked.", doctorProbePath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the FortiOS REST API root. Point %s at https://<fortigate>[:port]/api/v2.", doctorProbePath, doctorBaseURLEnv)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the FortiGate returned a server error, so the token was never checked.", status, doctorProbePath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, doctorProbePath, bin, doctorProbeCmd)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
