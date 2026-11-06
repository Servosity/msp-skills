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
// See issue #282 and skills/rewst/handfixes.json.
//
// Generated from a template: the import block must be gofmt-sorted after
// substitution, because the module path sorts before or after github.com/
// depending on the slug. Always run gofmt -w over the generated files.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"rewst-pp-cli/internal/client"
)

// doctorBaseURLEnv is the environment variable that points this connector at
// the operator's own instance. Named in remedies so the operator is told the
// fix, not just the symptom.
const doctorBaseURLEnv = "REWST_BASE_URL"

// doctorShippedBaseURL is the literal base_url this connector ships with. An
// operator who never set doctorBaseURLEnv is still on this exact value.
//
// Matching it EXACTLY matters. An earlier version of this check matched any
// host containing "your-", which refuses a real operator domain: an MSP at
// cipp.your-itdepartment.co.uk would be told their working install is a
// placeholder and doctor would never dial it. Refusing to check a healthy
// install is the same class of defect as blessing a broken one, pointed the
// other way.
// rewst handfix doctor-graphql-probe: Rewst ships its real US API host
// (https://api.rewst.io), not a placeholder, so nothing here may match it.
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
	for _, reserved := range []string{"example.com", "example.net", "example.org", "example.edu", "invalid", "localhost.example", "example", "test"} {
		if host == reserved || strings.HasSuffix(host, "."+reserved) {
			return true
		}
	}
	return false
}

// doctorGraphQLProbe is the one read doctor sends (rewst handfix
// doctor-graphql-probe). Rewst is a GraphQL API: every call is POST /graphql,
// and the GET path the press gives its typed commands is a placeholder the
// gateway does not serve, so probing it reported a healthy install as a wrong
// base_url. The token's own organization is the smallest read every API
// client token can make, and it changes nothing.
const doctorGraphQLProbe = `query DoctorProbe { userOrganization { id } }`

// doctorProbeCredentials issues ONE authenticated GraphQL read and writes BOTH
// the credentials verdict and, when the probe proves the API row was
// optimistic, a corrected api verdict. client.Post goes through c.do, which
// neither reads nor writes the response cache, so a credential revoked
// server-side cannot be reported healthy from a cached 200. Rewst answers an
// authorization failure with HTTP 200 and an errors array, so the errors array
// decides the verdict whatever the status; its message text is never printed
// (it can echo what was sent), only plain-identifier error codes.
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
	const probePath = "/graphql"
	body, status, err := c.Post(ctx, probePath, map[string]any{"query": doctorGraphQLProbe})
	// graphql-errors-are-failures turns a 2xx answer carrying errors into an
	// HTTP error (401/403/400/502 by its code); its body is the (masked)
	// answer, read as GraphQL here whatever status it became.
	gqlFailed := false
	var apiErr *client.APIError
	if err != nil && errors.As(err, &apiErr) && graphQLErrorsPopulated([]byte(apiErr.Body)) {
		body, status, err, gqlFailed = []byte(apiErr.Body), http.StatusOK, nil, true
	}
	if err == nil {
		r, decodeErr := doctorDecodeGraphQL(body)
		if gqlFailed {
			r.failed = true // the errors member was there, whatever the masked text still decodes to
		}
		switch {
		case decodeErr != nil && !gqlFailed:
			report["credentials"] = fmt.Sprintf("WARN not verified (POST %s answered with a body that is not a GraphQL response) - check %s points at the API root.", probePath, doctorBaseURLEnv)
		case r.failed && r.denied:
			report["credentials"] = fmt.Sprintf("ERROR rejected (GraphQL %s from %s) - the token is invalid, expired, revoked, or for another region (check %s).", doctorCodesOr(r.codes, "errors"), probePath, doctorBaseURLEnv)
		case r.failed:
			report["credentials"] = fmt.Sprintf("WARN not verified (POST %s returned GraphQL errors%s). Run `%s organizations` to see them.", probePath, doctorCodeSuffix(r.codes), bin)
		case r.dataNotObject:
			report["credentials"] = fmt.Sprintf("valid (POST %s answered with data)", probePath)
		case !r.hasOrg:
			report["credentials"] = fmt.Sprintf("WARN not verified (POST %s answered no organization for this token) - the token may be for another region (check %s).", probePath, doctorBaseURLEnv)
		default:
			report["credentials"] = fmt.Sprintf("valid (verified with POST %s userOrganization)", probePath)
		}
		return
	}
	switch {
	case status == 401:
		report["credentials"] = fmt.Sprintf("ERROR rejected (HTTP 401 from %s) - the token is invalid, expired, or for another region.", probePath)
	case status == 403:
		report["credentials"] = fmt.Sprintf("scope-limited (HTTP 403 from %s) - the token is accepted but lacks permission for this read.", probePath)
	case status == 404:
		doctorWrongBaseURL(probePath, report)
	case status >= 500:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s) - the vendor API returned a server error, so the credential was never checked.", status, probePath)
	case status >= 200 && status < 300:
		// The client refuses a 2xx it cannot read as JSON (a login page, a
		// proxy's HTML): the host answered, but not as the GraphQL API.
		report["credentials"] = fmt.Sprintf("WARN not verified (POST %s answered with a body that is not a GraphQL response) - check %s points at the API root.", probePath, doctorBaseURLEnv)
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from %s).", status, probePath)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorGraphQLResult is what the probe's answer says: failed when the errors
// array is non-empty whatever it says (any failure signal wins); denied when a
// code or a message reads as an authorization failure; codes holds only the
// extensions.code values that are plain identifiers; hasOrg when data.userOrganization carries an id;
// dataNotObject when data is present but cannot name an organization (an array, say).
// ponytail: dataNotObject reads as valid, because an authenticated 2xx with no
// errors is all a token check can ask of it. A host answering {"data":[...]} at
// POST /graphql therefore verifies; no GraphQL server answers that.
type doctorGraphQLResult struct {
	failed, denied, hasOrg, dataNotObject bool
	codes                                 []string
}

var doctorDeniedMessage = regexp.MustCompile(`(?i)(not\s+authori[sz]ed|unauthori[sz]ed|unauthenticated|forbidden|permission\s+denied|invalid\s+(api\s+)?token|token\s+(is\s+)?(invalid|expired|revoked))`)

func doctorDecodeGraphQL(body []byte) (doctorGraphQLResult, error) {
	var r doctorGraphQLResult
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    any `json:"message"`
			Extensions struct {
				Code any `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return r, err
	}
	hasData := len(resp.Data) > 0 && string(resp.Data) != "null"
	if !hasData && resp.Errors == nil {
		return r, fmt.Errorf("neither data nor errors")
	}
	r.failed = len(resp.Errors) > 0
	for _, e := range resp.Errors {
		code, _ := e.Extensions.Code.(string)
		if doctorDeniedCode(code) {
			r.denied = true
		}
		if msg, ok := e.Message.(string); ok && doctorDeniedMessage.MatchString(msg) {
			r.denied = true
		}
		// A code is vendor text too: the credential kit checks every byte the
		// report prints, so only its plain-identifier shape is decided here.
		if doctorPlainCode(code) {
			r.codes = append(r.codes, code)
		}
	}
	if !hasData {
		return r, nil
	}
	var data struct {
		UserOrganization *struct {
			ID any `json:"id"`
		} `json:"userOrganization"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		r.dataNotObject = true
		return r, nil
	}
	if o := data.UserOrganization; o != nil && o.ID != nil && fmt.Sprint(o.ID) != "" {
		r.hasOrg = true
	}
	return r, nil
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

func doctorDeniedCode(code string) bool {
	switch strings.ToUpper(code) {
	case "FORBIDDEN", "UNAUTHENTICATED", "UNAUTHORIZED", "PERMISSION_DENIED", "INVALID_TOKEN", "AUTH_ERR": // AUTH_ERR: Rewst's gateway, observed 2026-10-30
		return true
	}
	return false
}

func doctorCodeSuffix(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return ": " + strings.Join(codes, ",")
}

func doctorCodesOr(codes []string, fallback string) string {
	if len(codes) == 0 {
		return fallback
	}
	return strings.Join(codes, ",")
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
