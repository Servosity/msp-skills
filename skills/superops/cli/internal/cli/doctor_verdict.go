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
// The helpers here give doctor a verdict it can defend: refuse to dial a
// placeholder, probe the GraphQL API with the real credential, and classify
// the answer by status code and GraphQL error code instead of by substring.
//
// See issue #282 and skills/superops/handfixes.json.
//
// Generated from a template: the import block must be gofmt-sorted after
// substitution, because the module path sorts before or after github.com/
// depending on the slug. Always run gofmt -w over the generated files.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"superops-pp-cli/internal/client"
	"superops-pp-cli/internal/cliutil"
)

// doctorBaseURLEnv is the environment variable that points this connector at
// the operator's own instance. Named in remedies so the operator is told the
// fix, not just the symptom.
const doctorBaseURLEnv = "SUPEROPS_BASE_URL"

// doctorShippedBaseURL is the literal base_url this connector ships with. An
// operator who never set doctorBaseURLEnv is still on this exact value.
//
// Matching it EXACTLY matters. An earlier version of this check matched any
// host containing "your-", which refuses a real operator domain: an MSP at
// cipp.your-itdepartment.co.uk would be told their working install is a
// placeholder and doctor would never dial it. Refusing to check a healthy
// install is the same class of defect as blessing a broken one, pointed the
// other way.
//
// Empty for superops: it ships its real production host (api.superops.ai), so
// matching it refused every healthy default install and skipped the probe.
const doctorShippedBaseURL = ""

// doctorBaseURLIsShippedPlaceholder reports whether base is still the value this
// connector shipped with rather than the operator's own instance.
//
// Two shapes ship today: an unresolved template ("https://{tenant}.example/api")
// and a literal stand-in ("https://your-instance.zammad.com/api/v1"). Both must
// be refused BEFORE dialling. Refusing matters most for the literal shape -
// several placeholder hosts resolve through live wildcard DNS and answer real
// HTTP, so dialling one produces a confident green verdict against a host the
// operator has never heard of.
func doctorBaseURLIsShippedPlaceholder(base string) bool {
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
	for _, reserved := range []string{"example.com", "example.net", "example.org", "example.edu", "invalid", "example"} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return true
		}
	}
	return false
}

// doctorGraphQLProbe is the one read doctor sends. SuperOps is a GraphQL API:
// every call is POST /msp, and the GET paths the spec gives its typed commands
// are placeholders the API does not serve, so probing one reported a healthy
// install as a wrong base_url. One ticket page with only listInfo selected is
// the smallest read every technician token can make.
const doctorGraphQLProbe = `query($first: Int, $page: Int) { getTicketList(input: {pageSize: $first, page: $page}) { listInfo { totalCount } } }`

// doctorProbeCredentials issues ONE authenticated GraphQL read and writes BOTH
// the credentials verdict and, when the probe proves the API row was
// optimistic, a corrected api verdict. client.Post goes through c.do, which
// neither reads nor writes the response cache, so a credential revoked
// server-side cannot be reported healthy from a cached 200. The server's error
// text is never printed (it can echo what was sent); only its error codes are.
func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	// Ask the CLIENT whether it has a credential, not the config doctor loaded.
	if c == nil || c.Config == nil || c.Config.AuthHeader() == "" {
		report["credentials"] = "ERROR not verified: the client has no credential to send, so any response would say nothing about authentication."
		return
	}
	if c.DryRun {
		report["credentials"] = "not verified (--dry-run sends nothing)"
		return
	}
	if cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv() {
		report["credentials"] = "not verified (PRINTING_PRESS_VERIFY mock mode sends nothing)"
		return
	}
	const probePath = "/msp"
	body, status, err := c.Post(ctx, probePath, client.GraphQLRequest{Query: doctorGraphQLProbe, Variables: map[string]any{"first": 1, "page": 1}})
	if err == nil {
		codes, failed, decodeErr := doctorGraphQLErrorCodes(body)
		// Hand-fix (handfixes.json credkit-wiring): the denial is read from every code; only the codes shown are withheld.
		denied, shown := failed && doctorGraphQLDenied(codes), doctorShownCodes(codes)
		switch {
		case decodeErr != nil:
			report["credentials"] = fmt.Sprintf("WARN not verified (POST %s answered with a body that is not a GraphQL response) - check %s points at the API root.", probePath, doctorBaseURLEnv)
		case denied:
			report["credentials"] = fmt.Sprintf("ERROR rejected (GraphQL %s from %s) - the credential is invalid, expired, lacks ticket access, or is for a different tenant (check SUPEROPS_SUBDOMAIN).", strings.Join(shown, ","), probePath)
		case failed:
			report["credentials"] = fmt.Sprintf("WARN not verified (POST %s returned GraphQL errors%s). Run `%s raw query '{ getTicketList(input: {pageSize: 1}) { listInfo { totalCount } } }'` to see them.", probePath, doctorCodeSuffix(shown), bin)
		default:
			report["credentials"] = fmt.Sprintf("valid (verified with POST %s getTicketList)", probePath)
		}
		return
	}
	switch {
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the credential is invalid, expired, or for a different tenant.", probePath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the credential is accepted but lacks permission for this endpoint.", probePath)
	case status == 400:
		report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 400 from %s) - SuperOps rejects requests without a valid tenant; set SUPEROPS_SUBDOMAIN to your subdomain (Settings > MSP Information).", probePath)
	case status == 404:
		doctorWrongBaseURL(probePath, report)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the vendor API returned a server error, so the credential was not tried.", status, probePath)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s).", status, probePath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err.Error())
	}
}

// doctorGraphQLErrorCodes decodes a GraphQL response. failed is true when the
// errors array is non-empty, whatever it says; codes holds only the
// extensions.code values that are plain identifiers.
func doctorGraphQLErrorCodes(body []byte) (codes []string, failed bool, err error) {
	var resp struct {
		Errors []struct {
			Extensions struct {
				Code any `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, err
	}
	for _, e := range resp.Errors {
		if code, ok := e.Extensions.Code.(string); ok && doctorPlainCode(code) {
			codes = append(codes, code)
		}
	}
	return codes, len(resp.Errors) > 0, nil
}

func doctorPlainCode(code string) bool {
	if code == "" || len(code) > 40 {
		return false
	}
	for _, r := range code {
		if !(r == '_' || r == '-' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func doctorGraphQLDenied(codes []string) bool {
	for _, c := range codes {
		switch strings.ToUpper(c) {
		case "FORBIDDEN", "UNAUTHENTICATED", "UNAUTHORIZED", "PERMISSION_DENIED":
			return true
		}
	}
	return false
}

// doctorShownCodes is the codes a report may show: a code is vendor text
// too, and one that repeats a held credential in any form is never shown.
func doctorShownCodes(codes []string) []string {
	var shown []string
	for _, c := range codes {
		if !cliutil.HeldText(c) {
			shown = append(shown, c)
		}
	}
	return shown
}

func doctorCodeSuffix(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return ": " + strings.Join(codes, ",")
}

// doctorWrongBaseURL records the verdict for a probe that reached a server
// which has never heard of the endpoint. That is the signature of a base URL
// pointing at the right company but the wrong service - a vendor's web UI
// instead of its API, or an API root missing its version or /api suffix. The
// credential is not at fault and must not be blamed, and the api row must stop
// claiming the connector is reachable in any useful sense.
func doctorWrongBaseURL(apiPath string, report map[string]any) {
	report["credentials"] = fmt.Sprintf("ERROR not verified (HTTP 404 from %s) - the endpoint does not exist at this base_url, so the credential was not tried.", apiPath)
	report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not this vendor's API root. Point %s at the API root.", apiPath, doctorBaseURLEnv)
}
