// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality.
//
// Before this file existed, doctor answered "OK API: reachable" whenever the
// configured host returned ANY HTTP response to `GET /` - including a 404 from
// a wrong base URL, a 401 from a dead token, and a 200 from a vendor's marketing
// page. It also never probed the credential at all on this connector: it printed
// "present, not verified" and left the operator to guess. The mirror defect hit
// correctly-configured operators: a connector whose shipped base_url is a
// placeholder dialled the placeholder host and rendered a DNS failure as
// "FAIL API: unreachable", which reads as "your install is broken" when the real
// answer is "one environment variable is unset".
//
// The three helpers here give doctor a verdict it can defend: refuse to dial a
// placeholder, probe a real read endpoint with the real credential, and classify
// the answer by status code instead of by substring.
//
// See issue #282 and skills/gradient/handfixes.json.
//
// Generated from a template: the import block must be gofmt-sorted after
// substitution, because the module path sorts before or after github.com/
// depending on the slug. Always run gofmt -w over the generated files.

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gradient-pp-cli/internal/client"
)

// doctorBaseURLEnv is the environment variable that points this connector at
// the operator's own instance. Named in remedies so the operator is told the
// fix, not just the symptom.
const doctorBaseURLEnv = "GRADIENT_BASE_URL"

// doctorShippedBaseURL is the literal base_url this connector ships with. An
// operator who never set doctorBaseURLEnv is still on this exact value.
//
// Matching it EXACTLY matters. An earlier version of this check matched any
// host containing "your-", which refuses a real operator domain: an MSP at
// cipp.your-itdepartment.co.uk would be told their working install is a
// placeholder and doctor would never dial it. Refusing to check a healthy
// install is the same class of defect as blessing a broken one, pointed the
// other way.
const doctorShippedBaseURL = "https://app.usegradient.com/api"

// doctorInfoKeys are report entries rendered without a status indicator:
// paths, versions, and the free-text hints that tell an operator how to get a
// credential. They are NOT health checks.
//
// doctorExitForFailOn used to scan every value in the report for the substrings
// "error", "missing", "invalid" and "unreachable". That made --fail-on=error
// trip on a perfectly healthy connector whose auth hint happened to contain the
// word "missing", or whose suggested read command was named something like
// `errors list`. Excluding the informational keys keeps --fail-on keyed to the
// checks and nothing else.
var doctorInfoKeys = map[string]bool{
	"config_path":       true,
	"base_url":          true,
	"auth_source":       true,
	"version":           true,
	"auth_hint":         true,
	"auth_key_url":      true,
	"auth_instructions": true,
	"agentcookie":       true,
	"db_path":           true,
}

func doctorIsInfoKey(key string) bool { return doctorInfoKeys[key] }

// doctorBaseURLIsPlaceholder reports whether base is still the value this
// connector shipped with rather than the operator's own instance.
//
// Two shapes ship today: an unresolved template ("https://{tenant}.example/api")
// and a literal stand-in ("https://your-instance.zammad.com/api/v1"). Both must
// be refused BEFORE dialling. Refusing matters most for the literal shape -
// several placeholder hosts resolve through live wildcard DNS and answer real
// HTTP, so dialling one produces a confident green verdict against a host the
// operator has never heard of.
func doctorBaseURLIsPlaceholder(base string) bool {
	if base == "" {
		return false // "not configured" is a separate, already-handled state
	}
	// An unresolved path/host template.
	if strings.ContainsAny(base, "{}") {
		return true
	}
	// Still exactly what shipped: the operator has set nothing.
	if doctorShippedBaseURL != "" && strings.EqualFold(strings.TrimRight(base, "/"), strings.TrimRight(doctorShippedBaseURL, "/")) {
		return true
	}
	// A literal upper-case stand-in left in the path, e.g. /company/YOUR_REALM_ID.
	if strings.Contains(base, "YOUR_") {
		return true
	}
	// Reserved example domains can never be a real instance (RFC 2606), so
	// matching the host suffix here cannot hit an operator's own domain the way
	// a bare "your-" substring could.
	host := strings.ToLower(base)
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#:"); i >= 0 {
		host = host[:i]
	}
	for _, reserved := range []string{"example.com", "example.net", "example.org", "example.edu", "invalid", "localhost.example"} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return true
		}
	}
	return false
}

// doctorReadProbe walks the Cobra tree for an endpoint-mirror command that is
// safe and complete to call with no arguments, and returns the API path to
// probe plus the dotted command path to name in a remedy.
//
// Requirements, in order: the leaf mirrors a real endpoint (pp:endpoint), the
// endpoint is a GET (pp:method) so probing cannot mutate anything, it carries a
// concrete path (pp:path), its verb is list/get, and it takes no positional
// arguments. Framework commands like `feedback list` read local files and would
// recreate the false confidence this probe exists to remove.
func doctorReadProbe(root *cobra.Command) (apiPath string, cmdPath string) {
	if root == nil {
		return "", ""
	}
	// Two passes. A list/get leaf makes the nicest remedy line, but insisting on
	// that verb found nothing at all on 25 connectors whose generated leaves are
	// named after the vendor's own wording (retrieve, search, describe, show), so
	// the second pass accepts any GET. pp:method GET plus an argument-free
	// concrete path is the whole safety guarantee here; the verb name never was.
	var fallbackAPI, fallbackCmd string
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, path []string) {
		if apiPath != "" {
			return
		}
		for _, child := range cmd.Commands() {
			childPath := append(append([]string{}, path...), child.Name())
			if p := doctorProbeableLeaf(child); p != "" {
				verb := strings.ToLower(strings.SplitN(child.Use, " ", 2)[0])
				if verb == "list" || verb == "get" {
					apiPath, cmdPath = p, strings.Join(childPath, " ")
					return
				}
				if fallbackAPI == "" {
					fallbackAPI, fallbackCmd = p, strings.Join(childPath, " ")
				}
			}
			// Recurse into Hidden parents: printed CLIs hide raw resource
			// groups to keep --help curated, but their leaves stay runnable.
			walk(child, childPath)
			if apiPath != "" {
				return
			}
		}
	}
	walk(root, nil)
	if apiPath == "" {
		apiPath, cmdPath = fallbackAPI, fallbackCmd
	}
	return apiPath, cmdPath
}

// doctorProbeableLeaf returns the leaf's API path when it is safe to probe,
// or "" when it is not.
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
	if cmd.Args == nil {
		return path
	}
	if cmd.Args(cmd, []string{}) != nil {
		return ""
	}
	return path
}

// doctorProbeCredentials issues ONE authenticated read and writes BOTH the
// credentials verdict and, when the probe proves the API row was optimistic,
// a corrected api verdict. client.ProbeGet goes through c.do, which neither
// reads nor writes the response cache, so a credential revoked server-side
// cannot be reported healthy from a cached 200.
//
// It sets api as well as credentials because the two rows are one fact. The
// api row is decided by a bare request to the host, which a vendor's marketing
// page or login redirect answers happily; leaving it green next to a failed
// credential probe is how an operator reads "reachable" as "working".
func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	// Ask the CLIENT whether it has a credential, not the config doctor loaded.
	// They can disagree - a connector that resolves credentials through a client
	// profile leaves the client with nothing to send while doctor's own config
	// still looks populated. Probing anyway produces "valid" from a request that
	// carried no credential at all, which is the exact false-OK this file exists
	// to remove. avanan was doing this.
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	apiPath, cmdPath := doctorReadProbe(root)
	if apiPath == "" {
		report["credentials"] = "WARN not verified: this API exposes no argument-free GET endpoint to probe. Run any read command to confirm the credential works end-to-end."
		// The api row was decided by a bare request to the host, which a vendor's
		// marketing page, login redirect or 404 page answers happily. With no
		// endpoint to probe there is nothing behind that row, so it must not read
		// as a clean bill of health.
		report["api"] = "WARN reachable at the transport level only - no argument-free GET endpoint exists to confirm this is the vendor's API or that the credential works."
		return
	}
	status, err := c.ProbeGet(ctx, apiPath)
	switch {
	case err == nil:
		report["credentials"] = fmt.Sprintf("valid (verified with GET %s)", apiPath)
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the credential is invalid, expired, or for a different tenant.", apiPath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the credential is accepted but lacks permission for this endpoint.", apiPath)
	case status == 404:
		doctorWrongBaseURL(apiPath, report)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the vendor API returned a server error, so the credential was never checked.", status, apiPath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s). Run `%s %s` to see the full response.", status, apiPath, bin, cmdPath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorWrongBaseURL records the verdict for a probe that reached a server
// which has never heard of the endpoint. That is the signature of a base URL
// pointing at the right company but the wrong service - a vendor's web UI
// instead of its API, or an API root missing its version or /api suffix. The
// credential is not at fault and must not be blamed, and the api row must stop
// claiming the connector is reachable in any useful sense.
func doctorWrongBaseURL(apiPath string, report map[string]any) {
	report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was never checked.", apiPath)
	report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not this vendor's API root. Point %s at the API root.", apiPath, doctorBaseURLEnv)
}

// doctorUnexpectedStatus classifies a credential probe that came back with a
// status the connector did not anticipate. It replaced a line that read
// "ok (HTTP %d from %s, but auth was accepted)" and rendered green: a 404 there
// meant the probe never reached an endpoint that could check the credential, so
// calling it "ok" turned a misconfigured install into an all-green report.
func doctorUnexpectedStatus(status int, path string, report map[string]any) {
	if status == 404 {
		doctorWrongBaseURL(path, report)
		return
	}
	report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the endpoint did not confirm the credential.", status, path)
}
