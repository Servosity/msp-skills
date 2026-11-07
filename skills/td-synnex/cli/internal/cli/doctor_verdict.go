// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: makes `doctor` report a verdict that tracks reality.
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the token, so a base_url aimed at a web
// page and a revoked token both rendered healthy (issue #282). Every StreamOne
// ION API path is account-scoped, so the probe needs TD_SYNNEX_ACCOUNT_ID:
// GET /accounts/{accountId}/customers?pageSize=1 is the cheapest read every
// reseller user can make, and it proves the token, the account and the base
// URL at once.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"td-synnex-pp-cli/internal/client"
)

const doctorProbeCmd = "customers list"

// doctorBaseURLEnv names the variable that overrides the API root.
const doctorBaseURLEnv = "TD_SYNNEX_BASE_URL"

// doctorNotSignedInVerdict is the credentials row when no access token is loaded.
const doctorNotSignedInVerdict = "ERROR not configured: no StreamOne ION access token. Pipe your ION refresh token to `td-synnex-cli auth mint --refresh-token-stdin` (ION portal: Settings > Users > Edit > OAuth credentials)."

// doctorProbeCredentials issues ONE authenticated, uncached GET and writes the
// credentials verdict, correcting the api verdict when the probe proves it was
// optimistic.
// The signature is the fleet helper's (root unused here), so the onboard
// replay recognises this variant.
func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = doctorNotSignedInVerdict
		return
	}
	acct := strings.TrimSpace(c.Config.TemplateVars["accountId"])
	if acct == "" {
		report["credentials"] = "WARN not verified: no argument-free GET endpoint - every StreamOne ION API path needs your account ID. Set TD_SYNNEX_ACCOUNT_ID and run doctor again."
		report["api"] = "WARN not verified as the StreamOne ION API: a host answered at /, but without TD_SYNNEX_ACCOUNT_ID no API path can be checked."
		return
	}
	path := "/accounts/" + url.PathEscape(acct) + "/customers"
	_, err := c.GetWithHeadersNoCache(ctx, path, map[string]string{"pageSize": "1"}, nil)
	var apiErr *client.APIError
	status := 0
	if errors.As(err, &apiErr) {
		status = apiErr.StatusCode
	}
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", path)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the access token is expired or revoked. Run `%s auth mint` (access tokens last 2 hours).", path, bin)
	case status == 403:
		report["credentials"] = fmt.Sprintf("WARN scope-limited (HTTP 403 from %s) - the token is accepted but this ION user cannot read account %s. Check TD_SYNNEX_ACCOUNT_ID and the user's role.", path, acct)
	case status == 404 || status >= 500:
		doctorUnexpectedStatus(status, path, report)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, path, bin, doctorProbeCmd)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorUnexpectedStatus writes the verdict for a probe status that says
// nothing about the token: a 404 means the base URL or account ID is wrong, a
// 5xx means ION failed before checking it.
func doctorUnexpectedStatus(status int, path string, report map[string]any) {
	if status == 404 {
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the path does not exist at this base_url, so the token was never checked.", path)
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the StreamOne ION API root, or TD_SYNNEX_ACCOUNT_ID is wrong. Unset %s to use https://ion.tdsynnex.com/api/v3.", path, doctorBaseURLEnv)
		return
	}
	report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - ION returned a server error, so the token was never checked.", status, path)
}
