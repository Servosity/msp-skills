// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, pointing the operator at
// `objectschema list`, an Assets command that needs JSM Premium plus a cloudId
// and workspaceId. The placeholder branch never named the variable to set.
//
// GET /servicedesk is argument-free and answers for any authenticated account
// (agents and portal customers alike), so it is the probe.
//
// See issue #282 and skills/jira-service-management/handfixes.json.

package cli

import (
	"context"
	"fmt"

	"jira-service-management-pp-cli/internal/client"
)

// The variables an operator sets to fix the base URL, named in remedies.
const (
	doctorSiteEnv    = "JIRA_SERVICE_MANAGEMENT_SITE"
	doctorBaseURLEnv = "JIRA_SERVICE_MANAGEMENT_BASE_URL"
)

// doctorProbePath / doctorProbeCmd: the read every JSM account can make, and
// the command that makes it, for the remedy line.
const (
	doctorProbePath = "/servicedesk"
	doctorProbeCmd  = "servicedesk list"
)

// doctorProbeCredentials issues ONE authenticated GET and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic. ProbeGet goes through c.do, which bypasses the response cache, so
// a revoked token cannot be reported healthy from a cached 200.
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
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the email or API token is wrong, or the token was revoked at id.atlassian.com.", doctorProbePath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the token is accepted but this account cannot list service desks.", doctorProbePath)
	case status == 404:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the token was never checked.", doctorProbePath)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the JSM request-API root. Set %s to your site subdomain, or %s to https://<site>.atlassian.net/rest/servicedeskapi.", doctorProbePath, doctorSiteEnv, doctorBaseURLEnv)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - Atlassian returned a server error, so the token was never checked.", status, doctorProbePath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, doctorProbePath, bin, doctorProbeCmd)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}
