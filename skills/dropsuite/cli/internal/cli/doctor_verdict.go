// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// Before this file existed, doctor answered "reachable" whenever the configured
// host returned ANY HTTP response to `GET /` - including a 404 from a wrong
// base URL, a 401 from a dead token, and a 200 from a web UI. It never probed
// the credential: it printed "present, not verified" and left the operator to
// guess. And with no host configured it dialled the default partner host
// dropsuite.us without ever saying that the host is a per-partner setting, so a
// partner on another Dropsuite API host was told the tokens or the network were
// at fault, never which variable to set.
//
// Ported to dropsuite from the fleet's doctor-verdict-tracks-reality hand-fix
// (press 4.34.2 shape) and adapted: the credential is the reseller pair
// (X-Access-Token and X-Reseller-Token), the probe is GET /users (the
// reseller-level read that lists organizations; its body carries every
// organization's authentication_token, which the credential kit holds as a
// declared response field, so the body is discarded here and never printed),
// and the shipped https://{host}/api with its default host dropsuite.us is a
// real API that is still probed. What changes on the default host is the
// remedy: when the probe does not verify there, the api row says the default
// host is in force and names DROPSUITE_HOST and DROPSUITE_BASE_URL.
//
// See issue #282 and skills/dropsuite/handfixes.json.

package cli

import (
	"context"
	"fmt"
	"strings"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
	"dropsuite-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

// doctorBaseURLEnv is the environment variable that points this connector at
// the partner's own API URL. Named in remedies so the operator is told the
// fix, not just the symptom.
const doctorBaseURLEnv = "DROPSUITE_BASE_URL"

// doctorHostEnv fills {host} in the shipped base URL https://{host}/api.
const doctorHostEnv = "DROPSUITE_HOST"

// doctorShippedBaseURL and doctorDefaultHost are what config.Load leaves in
// force when the operator sets neither variable nor a config base_url or
// template_vars host (internal/config/config.go).
const (
	doctorShippedBaseURL = "https://{host}/api"
	doctorDefaultHost    = "dropsuite.us"
)

// doctorProbePath is the read the credential probe issues: GET /users, which
// takes the reseller pair, changes nothing, and needs no argument. Every other
// argument-free read is organization-scoped (an organization token), so it
// would not test the installed credential, and the generate* routes mint a
// portal sign-in link (the route guard refuses them for a read).
const doctorProbePath = "/users"

// doctorHostRemedy names both ways to point the CLI at the partner's API host.
const doctorHostRemedy = "set " + doctorHostEnv + " to the host of the API URL shown in Settings > API Settings, or " +
	doctorBaseURLEnv + " to that whole URL"

// doctorReadProbe returns the API path to probe and the command that runs it.
// The path is GET /users when the tree has a safe, argument-free leaf for it
// (organizations list); otherwise nothing, and the caller reports the
// credential as not verified rather than probing an organization-scoped route.
func doctorReadProbe(root *cobra.Command) (apiPath string, cmdPath string) {
	if root == nil {
		return "", ""
	}
	if cmdPath = doctorLeafForPath(root, nil, doctorProbePath); cmdPath != "" {
		return doctorProbePath, cmdPath
	}
	return "", ""
}

// doctorLeafForPath returns the command path of the first probeable leaf
// whose API path is exactly want, or "" when there is none.
func doctorLeafForPath(cmd *cobra.Command, path []string, want string) string {
	for _, child := range cmd.Commands() {
		childPath := append(append([]string{}, path...), child.Name())
		if doctorProbeableLeaf(child) == want {
			return strings.Join(childPath, " ")
		}
		if found := doctorLeafForPath(child, childPath, want); found != "" {
			return found
		}
	}
	return ""
}

// doctorProbeableLeaf returns the leaf's API path when it is safe to probe,
// or "" when it is not: a runnable endpoint mirror (pp:endpoint) for a GET
// (pp:method) on a concrete path (pp:path) that takes no positional argument.
func doctorProbeableLeaf(cmd *cobra.Command) string {
	if cmd == nil || cmd.Hidden || cmd.HasSubCommands() || !cmd.Runnable() {
		return ""
	}
	if cmd.Annotations["pp:endpoint"] == "" {
		return ""
	}
	// GET only. A probe that could POST is not a health check.
	if !strings.EqualFold(cmd.Annotations["pp:method"], "GET") {
		return ""
	}
	path := cmd.Annotations["pp:path"]
	// A templated path needs an id we do not have.
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "{}") {
		return ""
	}
	// Positional path params are advertised in Use as <id> or [id].
	if strings.ContainsAny(cmd.Use, "<[") {
		return ""
	}
	if cmd.Args != nil && cmd.Args(cmd, []string{}) != nil {
		return ""
	}
	return path
}

// doctorProbeCredentials issues ONE authenticated read and writes BOTH the
// credentials verdict and, when the probe proves the API row was optimistic,
// a corrected api verdict. client.ProbeGet goes through c.do and the same
// transports every command uses (origin pin, credential kit), and this client
// never caches responses (handfixes.json no-response-cache), so a token
// revoked server-side cannot be reported healthy from a cached 200. The
// response body is discarded: it carries organization tokens.
func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	// Ask the CLIENT whether it has a credential, not the config doctor loaded:
	// a client profile can leave the client with nothing to send while doctor's
	// own config still looks populated, and probing anyway would call a request
	// that carried no credential "valid".
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	apiPath, cmdPath := doctorReadProbe(root)
	if apiPath == "" {
		report["credentials"] = "WARN not verified: this build has no argument-free GET /users command to probe. Run any reseller read command to confirm the credential works end-to-end."
		report["api"] = "WARN reachable at the transport level only - no argument-free GET endpoint exists to confirm this is the Dropsuite partner API or that the credential works."
		return
	}
	status, err := c.ProbeGet(ctx, apiPath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", apiPath)
	case status == 401:
		msg := fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the Authentication Token or the Reseller Token is invalid, expired, or issued for another Dropsuite API host.", apiPath)
		if strings.TrimSpace(c.Config.DropsuiteResellerToken) == "" {
			msg += " DROPSUITE_RESELLER_TOKEN is not set, and GET /users takes it together with DROPSUITE_ACCESS_TOKEN."
		}
		report["credentials"] = msg
	case status == 403:
		report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the credential is accepted but lacks permission for this endpoint.", apiPath)
	case status == 404:
		doctorUnexpectedStatus(status, apiPath, report)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the Dropsuite API returned a server error, so the credential is still unverified.", status, apiPath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, apiPath, bin, cmdPath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
	if err != nil && status != 403 {
		doctorNameDefaultHost(c.Config, report)
	}
}

// doctorWrongBaseURL records the verdict for a probe that reached a server
// which has never heard of the endpoint: a base URL pointing at a web UI
// instead of the partner API, or an API URL missing its /api suffix. The
// credential is not at fault and must not be blamed, and the api row must stop
// claiming the connector is reachable in any useful sense.
func doctorWrongBaseURL(apiPath string, report map[string]any) {
	report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential is still unverified.", apiPath)
	report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not the Dropsuite partner API root (https://<host>/api); %s.", apiPath, doctorHostRemedy)
}

// doctorUnexpectedStatus classifies a credential probe that came back with a
// status the connector did not anticipate. A 404 means the probe never reached
// an endpoint that could check the credential, so it names a wrong base URL.
func doctorUnexpectedStatus(status int, path string, report map[string]any) {
	if status == 404 {
		doctorWrongBaseURL(path, report)
		return
	}
	report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, path)
}

// doctorDefaultHostInForce reports whether the API URL doctor dials is the
// shipped template filled with the default host because the operator named no
// host: no DROPSUITE_BASE_URL, no config base_url, no DROPSUITE_HOST, and no
// other host in the config's template_vars. That default is a real partner
// API, so doctor still probes it; it only cannot claim the host is the
// partner's own until the credential verifies there.
func doctorDefaultHostInForce(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/") != doctorShippedBaseURL {
		return false // DROPSUITE_BASE_URL or a config base_url chose the URL
	}
	if strings.TrimSpace(cliutil.EnvOverride(doctorHostEnv)) != "" {
		return false // the operator named the host, even if it is the default one
	}
	return strings.EqualFold(strings.TrimSpace(cfg.TemplateVars["host"]), doctorDefaultHost)
}

// doctorNameDefaultHost is called when the default host is the likely fault:
// the credential probe ran there and did not verify, or the host did not
// answer at all. A partner on another Dropsuite API host gets a 401, a 404 or
// no answer from dropsuite.us, and nothing else in the report would tell them
// the host is the fix. So when the default host is in force the api row may
// not read as healthy, and it names the variables that move the CLI to the
// partner's own host. An api row that already names the host variable (a 404:
// doctorWrongBaseURL) is left alone.
func doctorNameDefaultHost(cfg *config.Config, report map[string]any) {
	if !doctorDefaultHostInForce(cfg) {
		return
	}
	api, _ := report["api"].(string)
	if api == "" || strings.Contains(api, doctorHostEnv) {
		return
	}
	where := "the default Dropsuite API host " + doctorDefaultHost + " is in force because " + doctorHostEnv +
		" and " + doctorBaseURLEnv + " are unset; if your partner API is on another host, " + doctorHostRemedy
	if strings.HasPrefix(api, "reachable") {
		report["api"] = "WARN " + api + ", but the credential did not verify there: " + where + "."
		return
	}
	report["api"] = api + "; " + where + "."
}
