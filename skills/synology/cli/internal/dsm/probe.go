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
// host, timeout, certificate refused). Error() is describeTransport's kind,
// never Go's text; Unwrap keeps the cause for errors.Is / errors.As.
type ProbeTransportError struct{ err error }

func (e *ProbeTransportError) Error() string { return describeTransport(e.err) }
func (e *ProbeTransportError) Unwrap() error { return e.err }

// ProbeDSM asks origin for SYNO.API.Info without credentials and returns nil
// only when the answer is DSM's: HTTP 200 with a success envelope whose data
// lists SYNO.API.Auth. It is the one rule that decides whether a password may
// be sent: Login runs it before every credential POST and doctor reports it.
// A redirect is not followed (DSM answers its WebAPI directly), so a host
// cannot vouch for another one.
func ProbeDSM(ctx context.Context, hc *http.Client, origin string) error {
	if hc == nil {
		hc = http.DefaultClient
	}
	noRedirect := *hc
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+APIInfoProbePath, nil)
	if err != nil {
		return &NotDSMError{Reason: "the address is not a usable URL"}
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return &ProbeTransportError{err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &ProbeTransportError{err: errors.New("reading the response failed")}
	}
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return &NotDSMError{Reason: fmt.Sprintf("the host answered SYNO.API.Info with status %d (a redirect); DSM answers its WebAPI without redirecting - if it redirects http:// to https://, use the https:// address", resp.StatusCode)}
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
