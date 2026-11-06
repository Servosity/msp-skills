// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: makes `doctor` report a verdict that tracks reality
// (comet-backup handfix: doctor-verdict-tracks-reality, issue #282).
//
// The generated doctor called any HTTP response to `GET /` "reachable" and
// printed "present, not verified" for the credential, so a base_url aimed at
// the wrong host, a rejected admin password and a working install all read
// healthy. It also never named the variables that point the connector at the
// operator's Comet Server when the shipped placeholder (https://{server}) was
// still in force.
//
// Comet has no GET API at all: every admin call is a POST to /api/v1/admin/...
// carrying Username, AuthType and Password (or SessionKey) form fields, and a
// refused login comes back as HTTP 401/403 or as a CometAPIResponseMessage
// {Status: 401/403} on HTTP 200. So the probe is one admin call, the same one
// `server version` makes (AdminMetaVersion), sent through the client every
// command uses, and the answer is classified by what it is: a version record
// proves the login, a refusal rejects it, and a 404, an HTML page or JSON that
// is not Comet's says base_url is not a Comet Server.
//
// Every verdict this file writes is program-authored text (status codes, the
// probe path, a validated MAJOR.MINOR.PATCH version); no response body or
// server message is copied into a credentials verdict.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"comet-backup-pp-cli/internal/client"
)

const (
	doctorBaseURLEnv   = "COMET_BACKUP_BASE_URL"
	doctorServerEnv    = "COMET_BACKUP_SERVER"
	doctorProbePath    = pathMetaVersion
	doctorProbeCmd     = "server version"
	doctorProbeTimeout = 30 * time.Second
)

// doctorBaseRemedy names both variables: COMET_BACKUP_SERVER fills the
// shipped https://{server} template, COMET_BACKUP_BASE_URL overrides it.
const doctorBaseRemedy = "Set " + doctorServerEnv + " to your Comet Server's host[:port], or " +
	doctorBaseURLEnv + " to its full URL (https://host[:port])."

const doctorLoginRemedy = "Set COMET_BACKUP_USERNAME to a Comet Server administrator and COMET_BACKUP_PASSWORD " +
	"to its password (or COMET_BACKUP_SESSION_KEY to an admin session key)."

// doctorBaseURLIsCometStandIn reports the stand-in host the CLI fills the
// {server} template with under PRINTING_PRESS_VERIFY=1 (config.Load): never
// an operator's server, so it is refused like the unresolved template.
func doctorBaseURLIsCometStandIn(base string) bool {
	u, err := url.Parse(strings.TrimSpace(base))
	return err == nil && strings.EqualFold(u.Hostname(), "server_placeholder")
}

// doctorPlaceholderVerdict is the verdict for the shipped placeholder base
// URL: nothing is dialled, and both rows name the variables that fix it. The
// row prefixes are the generated wording, which --fail-on matches.
func doctorPlaceholderVerdict(report map[string]any) {
	report["api"] = "not configured (base_url is a placeholder) - " + doctorBaseRemedy
	if _, set := report["credentials"]; !set {
		report["credentials"] = "skipped (base_url is a placeholder) - the admin login is checked once " +
			doctorServerEnv + " or " + doctorBaseURLEnv + " names your Comet Server."
	}
}

type doctorProbeKind int

const (
	doctorProbeOK          doctorProbeKind = iota // a Comet version record
	doctorProbeRefused                            // HTTP or envelope 401/403
	doctorProbeNotThere                           // 404, 405, 410
	doctorProbeNotComet                           // 2xx that is not a Comet answer
	doctorProbeServerError                        // 5xx
	doctorProbeRateLimited                        // 429
	doctorProbeOther                              // any other status
	doctorProbeNoAnswer                           // the request never got a response
)

type doctorProbeResult struct {
	kind    doctorProbeKind
	status  int
	version string
	err     error
}

// doctorVersion is the leading MAJOR.MINOR[.PATCH[.BUILD]] of a Comet version
// string; only this much of a server-supplied value is ever printed.
var doctorVersion = regexp.MustCompile(`^\d{1,4}\.\d{1,4}(\.\d{1,6}){0,2}`)

// classifyDoctorProbe sorts one probe answer. data and err are what the
// client returned: a non-2xx (and a 2xx carrying a Comet failure envelope with
// a Message) is an *client.APIError; a 2xx HTML page is an error with a 2xx
// status; anything else is the body.
func classifyDoctorProbe(data []byte, status int, err error) doctorProbeResult {
	var apiErr *client.APIError
	switch {
	case status == http.StatusTooManyRequests:
		return doctorProbeResult{kind: doctorProbeRateLimited, status: status}
	case errors.As(err, &apiErr):
		return classifyDoctorStatus(apiErr.StatusCode)
	case err != nil && status >= 200 && status < 300:
		// An HTML page (or other non-JSON) where Comet answers JSON.
		return doctorProbeResult{kind: doctorProbeNotComet, status: status}
	case err != nil:
		return doctorProbeResult{kind: doctorProbeNoAnswer, err: err}
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(data, &body) != nil || body == nil {
		return doctorProbeResult{kind: doctorProbeNotComet, status: status}
	}
	// A Comet failure envelope the client passed through because it carries
	// no Message ({"Status":403} on HTTP 200).
	if raw, ok := body["Status"]; ok {
		var envStatus int
		if json.Unmarshal(raw, &envStatus) == nil && envStatus >= 400 {
			return classifyDoctorStatus(envStatus)
		}
	}
	var version string
	if raw, ok := body["Version"]; ok && json.Unmarshal(raw, &version) == nil {
		if v := doctorVersion.FindString(strings.TrimSpace(version)); v != "" {
			return doctorProbeResult{kind: doctorProbeOK, status: status, version: v}
		}
	}
	return doctorProbeResult{kind: doctorProbeNotComet, status: status}
}

func classifyDoctorStatus(status int) doctorProbeResult {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return doctorProbeResult{kind: doctorProbeRefused, status: status}
	case status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusGone:
		return doctorProbeResult{kind: doctorProbeNotThere, status: status}
	case status == http.StatusTooManyRequests:
		return doctorProbeResult{kind: doctorProbeRateLimited, status: status}
	case status >= 500:
		return doctorProbeResult{kind: doctorProbeServerError, status: status}
	default:
		return doctorProbeResult{kind: doctorProbeOther, status: status}
	}
}

// doctorProbeCredentials makes ONE authenticated Comet admin call and writes
// the credentials verdict and, when the answer proves the api row was
// optimistic (or confirms the server), the api verdict. A credentials verdict
// that is already "refused:" (a credential present but not loaded) is kept:
// the probe then only classifies the server.
func doctorProbeCredentials(ctx context.Context, c *client.Client, report map[string]any) {
	keepCreds := strings.HasPrefix(fmt.Sprint(report["credentials"]), "refused:")
	setCreds := func(v string) {
		if !keepCreds {
			report["credentials"] = v
		}
	}
	authType, hasLogin := "", false
	if c != nil {
		authType, hasLogin = client.CometLoginConfigured(c.Config)
	}
	if c == nil {
		setCreds("ERROR not verified: no client to probe with.")
		return
	}
	call := "POST " + doctorProbePath
	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()
	data, status, err := c.PostQueryFormWithParams(ctx, doctorProbePath, nil, url.Values{})
	r := classifyDoctorProbe(data, status, err)
	noLogin := ""
	if !hasLogin {
		noLogin = " No Comet admin login is configured, so the probe carried none: " + doctorLoginRemedy
	}
	switch r.kind {
	case doctorProbeOK:
		if hasLogin {
			setCreds(fmt.Sprintf("valid (admin login verified with %s, AuthType %s)", call, authType))
			report["api"] = fmt.Sprintf("reachable: Comet Server %s answered %s", r.version, call)
			return
		}
		// A Comet Server never answers an admin call without a login.
		setCreds("WARN not verified: the server answered " + call + " as if signed in, but no admin login was sent." + noLogin)
		report["api"] = "WARN the host answered an admin call that carried no login, which a Comet Server refuses; check that base_url is your Comet Server. " + doctorBaseRemedy
	case doctorProbeRefused:
		if hasLogin {
			setCreds(fmt.Sprintf("ERROR rejected (status %d from %s) - the administrator username, password or session key is wrong, expired, or lacks admin rights. %s", r.status, call, doctorLoginRemedy))
		} else {
			setCreds(fmt.Sprintf("ERROR not verified (status %d from %s) - the server refused the call.%s", r.status, call, noLogin))
		}
		report["api"] = fmt.Sprintf("WARN the host answered %s but refused the admin login, so it is not confirmed to be your Comet Server. %s", call, doctorBaseRemedy)
	case doctorProbeNotThere:
		setCreds(fmt.Sprintf("ERROR not verified (HTTP %d from %s) - the endpoint does not exist at this base_url, so the admin login was never checked.%s", r.status, call, noLogin))
		report["api"] = fmt.Sprintf("ERROR the host answered but %s is not there - base_url is not your Comet Server. %s", call, doctorBaseRemedy)
	case doctorProbeNotComet:
		// Comet answers its admin API with JSON and has no GET endpoint, so
		// nothing else can confirm the login.
		setCreds(fmt.Sprintf("ERROR not verified: %s answered %d without Comet's version record, and Comet Server exposes no argument-free GET endpoint to probe instead. %s%s", call, r.status, doctorBaseRemedy, noLogin))
		report["api"] = "ERROR the host answered but not like a Comet Server - base_url is not your Comet Server. " + doctorBaseRemedy
	case doctorProbeServerError:
		setCreds(fmt.Sprintf("WARN not verified (HTTP %d from %s) - the server returned an error, so the admin login was never checked. Run `comet-backup-cli %s` to see the full response.%s", r.status, call, doctorProbeCmd, noLogin))
	case doctorProbeRateLimited:
		setCreds(fmt.Sprintf("WARN not verified (HTTP 429 from %s) - rate limited; run doctor again in a minute.%s", call, noLogin))
	case doctorProbeOther:
		setCreds(fmt.Sprintf("ERROR not verified (HTTP %d from %s). Run `comet-backup-cli %s` to see the full response.%s", r.status, call, doctorProbeCmd, noLogin))
		report["api"] = fmt.Sprintf("WARN the host answered HTTP %d to a Comet admin call - check that base_url is your Comet Server. %s", r.status, doctorBaseRemedy)
	case doctorProbeNoAnswer:
		switch {
		case errors.Is(r.err, client.ErrPlaintextCredential):
			setCreds("ERROR not verified: the probe was refused before sending, because plaintext HTTP would expose the admin login; use https.")
		case errors.Is(r.err, ErrOriginChanged):
			setCreds("ERROR not verified: the probe was refused because it would leave the configured Comet Server. " + doctorBaseRemedy)
		default:
			setCreds("ERROR not verified: " + call + " got no answer from the server; see the api row." + noLogin)
		}
		// The error text names the host and the transport failure; it goes
		// in the api row, never in a credentials verdict.
		report["api"] = fmt.Sprintf("WARN the host answered GET / but %s failed: %v", call, r.err)
	}
}
