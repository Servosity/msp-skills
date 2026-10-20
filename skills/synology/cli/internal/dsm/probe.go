// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIInfoProbePath is DSM's unauthenticated API discovery call, asking only
// for the login API. Every DSM answers it with HTTP 200 and
// {"success":true,"data":{"SYNO.API.Auth":{...}}}; a router's admin page, a
// proxy, a captive portal or any other web server does not.
const APIInfoProbePath = "/webapi/query.cgi?api=SYNO.API.Info&method=query&version=1&query=SYNO.API.Auth"

// NotDSMError reports a host that answered the probe, but not as a DSM
// WebAPI. Reason never quotes the response: it names a status code, a DSM
// error code or the shape of the body. It never says "HTTP <code>": the
// generated error classifier reads that as the API's own answer to the
// command (404 = resource not found).
type NotDSMError struct{ Reason string }

func (e *NotDSMError) Error() string {
	return "the NAS address does not answer as a DSM WebAPI: " + e.Reason
}

// ProbeTransportError reports a probe that never got an answer (unreachable
// host, timeout, certificate refused). It is a TransportError: Error() is
// describeTransport's kind, never Go's text; Unwrap keeps the cause for
// errors.Is / errors.As.
type ProbeTransportError = TransportError

// ProbeDSM asks origin for SYNO.API.Info without credentials and returns nil
// only when the answer is DSM's: HTTP 200 with a success envelope whose data
// lists SYNO.API.Auth. It is the one rule that decides whether a password may
// be sent: Login runs it before every credential POST and doctor reports it.
// A redirect is not followed (DSM answers its WebAPI directly), so a host
// cannot vouch for another one. The probe carries nothing secret: no
// session, no credential and no cookie (hc's cookie jar is not used), since
// the host has not yet shown it is a DSM. Callers that are about to send a
// credential or a session use VerifyDSM, which runs this once per address.
func ProbeDSM(ctx context.Context, hc *http.Client, origin string) error {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+APIInfoProbePath, nil)
	if err != nil {
		return &NotDSMError{Reason: "the address is not a usable URL"}
	}
	pc := NoRedirect(hc)
	pc.Jar = nil // a session cookie never goes to a host before it answers as DSM
	resp, err := pc.Do(req)
	var redir *RedirectError
	if errors.As(err, &redir) {
		return &NotDSMError{Reason: probeRedirectReason(redir.Status)}
	}
	if err != nil {
		return SanitizeTransport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return SanitizeTransport(errors.New("reading the response failed"))
	}
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return &NotDSMError{Reason: probeRedirectReason(resp.StatusCode)}
	case resp.StatusCode != http.StatusOK:
		return &NotDSMError{Reason: fmt.Sprintf("the host answered SYNO.API.Info with status %d; DSM answers it with status 200", resp.StatusCode)}
	case BodyLooksLikeHTML(body):
		return &NotDSMError{Reason: "the host answered SYNO.API.Info with an HTML page"}
	}
	data, _, failed := Envelope(body)
	switch {
	case failed:
		// The code is not quoted: a host that is not DSM chose it.
		return &NotDSMError{Reason: "the host answered SYNO.API.Info with a failure envelope; DSM answers it without a login"}
	case data == nil:
		return &NotDSMError{Reason: `the host answered SYNO.API.Info without DSM's {"success":true,"data":...} envelope`}
	}
	var apis map[string]json.RawMessage
	if json.Unmarshal(data, &apis) != nil {
		return &NotDSMError{Reason: "the host answered SYNO.API.Info with an unexpected payload"}
	}
	if _, ok := apis[authAPI]; !ok {
		return &NotDSMError{Reason: "the host's SYNO.API.Info answer does not list SYNO.API.Auth, the DSM login API"}
	}
	return nil
}

// BodyLooksLikeHTML reports a web page (a DSM login page, a router admin
// page, a proxy or captive portal) rather than an API answer.
func BodyLooksLikeHTML(body []byte) bool {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if len(lower) > 2048 {
		lower = lower[:2048]
	}
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		(strings.HasPrefix(lower, "<") && (strings.Contains(lower, "<html") || strings.Contains(lower, "<body") || strings.Contains(lower, "<head") || strings.Contains(lower, "<title")))
}

// probeRedirectReason names a redirect answer to the probe by status only.
func probeRedirectReason(status int) string {
	return fmt.Sprintf("the host answered SYNO.API.Info with status %d (a redirect); DSM answers its WebAPI without redirecting - if it redirects http:// to https://, use the https:// address", status)
}
