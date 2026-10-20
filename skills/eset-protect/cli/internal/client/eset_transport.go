// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output). ESET Connect transport behavior the
// generated client does not model:
//
//   - 202 Accepted + `response-id`: a slow ESET Connect request is cached
//     server-side and answered 202 with a `response-id` header. The client must
//     re-issue the SAME request with a `response-id` request header until the
//     answer is no longer 202 (ESET keeps the cached result about 10 minutes;
//     only the token may change between attempts).
//   - 401 mid-run: access tokens live one hour, so a long sync can outlive its
//     token. On the first 401 of a logical request (initial call or any poll)
//     the transport asks Reauth for a fresh bearer and replays once.
//   - Credential host pinning: with AllowHost set, a request that carries an
//     Authorization header to any other host is refused before it is sent, so
//     no flag that rewrites a URL (e.g. sync --path-context region=...) can
//     steer the bearer token off ESET.
//
// A re-minted token is published through the transport (SetAuth), never by
// mutating the shared config, so concurrent requests read it race-free.
// It wraps the generated client's transport via a client hook, so no generated
// file is edited.
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"eset-protect-pp-cli/internal/secrets"
)

// pendingPollBudget caps how long one request may stay pending. ESET drops the
// cached result after about 10 minutes; the request context (--timeout) can
// end polling sooner.
const pendingPollBudget = 10 * time.Minute

var (
	PendingPollStart = 1 * time.Second
	pendingPollMax   = 15 * time.Second
)

// EsetTransport re-issues 202 responses with the response-id header and
// replays one 401 after re-authenticating.
type EsetTransport struct {
	Base http.RoundTripper
	// Reauth returns a fresh Authorization header value ("Bearer ...") after
	// the server rejected `rejected`. Nil disables 401 replay.
	Reauth func(ctx context.Context, rejected string) (string, error)
	// AllowHost, when set, must accept a request's host before the transport
	// sends a credentialed request there.
	AllowHost func(host string) bool

	auth atomic.Value // string: bearer that replaces the request's after a re-mint
	// unavailable holds a token mint failure that is not a credential
	// rejection (eset_token_unavailable.go).
	unavailable tokenUnavailable
}

// SetAuth publishes the Authorization value to use instead of the one the
// generated client stamped (which still holds the expired token). The value
// is registered as a secret first, so an error body that echoes the
// refreshed bearer is masked like the original one (handfixes.json:
// secret-registry-masks-every-boundary).
func (t *EsetTransport) SetAuth(header string) {
	secrets.Register(header)
	t.auth.Store(header)
}

// Auth returns the published Authorization value, or "".
func (t *EsetTransport) Auth() string {
	if v, ok := t.auth.Load().(string); ok {
		return v
	}
	return ""
}

func (t *EsetTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// ErrPendingTimeout is returned when ESET kept answering 202 past the budget.
type ErrPendingTimeout struct {
	ResponseID string
	Waited     time.Duration
}

func (e *ErrPendingTimeout) Error() string {
	return fmt.Sprintf("ESET Connect request still pending after %s (response-id %s); retry the same command later or narrow the query", e.Waited.Round(time.Second), e.ResponseID)
}

// PendingWithoutIDError is a 202 Accepted that carries no response-id, so the
// result cannot be collected. It is an answer (often a proxy stripping the
// header), not a missing one.
type PendingWithoutIDError struct {
	Method, URL string
}

func (e *PendingWithoutIDError) Error() string {
	return fmt.Sprintf("ESET Connect answered 202 Accepted without a response-id header for %s %s; the result cannot be collected (a proxy may be stripping the header)", e.Method, e.URL)
}

// CredentialHostRefusedError is the credential host pin's verdict: the
// request carried an Authorization header to a host that is not an ESET
// Connect host. It is final, so the client fails at once instead of retrying
// it as a network error (IsNonRetryableTransportError).
type CredentialHostRefusedError struct {
	Host string
}

func (e *CredentialHostRefusedError) Error() string {
	return fmt.Sprintf("refusing to send ESET credentials to %q: not an ESET Connect host", e.Host)
}

// IsNonRetryableTransportError reports a transport verdict that replaying the
// request cannot change: a credential host refusal (the host is the same on
// every attempt) or a request ESET kept pending past its budget (ESET has
// already dropped the cached result). The client's network-error retry loop
// consults it so neither is retried with backoff.
func IsNonRetryableTransportError(err error) bool {
	var refused *CredentialHostRefusedError
	if errors.As(err, &refused) {
		return true
	}
	// A failed 401 re-mint already asked the token service; replaying the
	// request would only ask it again.
	var reauth *ReauthFailedError
	if errors.As(err, &reauth) {
		return true
	}
	var unavailable *TokenUnavailableError
	if errors.As(err, &unavailable) {
		return true
	}
	var pending *ErrPendingTimeout
	return errors.As(err, &pending)
}

// ReauthFailedError is a request ESET answered 401 whose re-mint then failed
// for a reason other than a token service outage. Err is the classified
// mint failure (a rejection exits 4, anything else 5); Error is its text.
type ReauthFailedError struct {
	Err error
}

func (e *ReauthFailedError) Error() string { return e.Err.Error() }
func (e *ReauthFailedError) Unwrap() error { return e.Err }

func (t *EsetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := snapshotBody(req)
	if err != nil {
		return nil, err
	}
	req = cloneWithBody(req, body)
	if t.AllowHost != nil && req.Header.Get("Authorization") != "" && !t.AllowHost(req.URL.Host) {
		return nil, &CredentialHostRefusedError{Host: req.URL.Host}
	}
	if h := t.Auth(); h != "" && strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		req.Header.Set("Authorization", h)
	}
	reauthed := false
	send := func(r *http.Request) (*http.Response, error) {
		resp, err := t.base().RoundTrip(cloneWithBody(r, body))
		if err != nil || resp.StatusCode != http.StatusUnauthorized || reauthed || t.Reauth == nil || r.Header.Get("Authorization") == "" {
			return resp, err
		}
		reauthed = true
		header, rerr := t.Reauth(r.Context(), r.Header.Get("Authorization"))
		if rerr != nil {
			// Hand-fix reauth-failure-keeps-its-class: the 401 is not the
			// answer once the re-mint itself failed. Returning it read an
			// ESET outage during the re-mint as a credential rejection
			// (exit 4); the re-mint's own error carries its class (exit 4
			// for a rejection, 5 for anything else, and a token outage
			// reads as TokenUnavailableError so an auto read can fall back).
			drain(resp)
			var unavailable *TokenUnavailableError
			if errors.As(rerr, &unavailable) {
				return nil, unavailable
			}
			return nil, &ReauthFailedError{Err: rerr}
		}
		if header == "" || header == r.Header.Get("Authorization") {
			return resp, nil
		}
		drain(resp)
		req.Header.Set("Authorization", header)
		r.Header.Set("Authorization", header)
		return t.base().RoundTrip(cloneWithBody(r, body))
	}

	resp, err := send(req)
	if err != nil || resp.StatusCode != http.StatusAccepted {
		return resp, err
	}
	responseID := resp.Header.Get("response-id")
	if responseID == "" {
		// A 202 without response-id cannot be collected; treating its body as
		// the answer would report a still-running query as an empty result.
		drain(resp)
		return nil, &PendingWithoutIDError{Method: req.Method, URL: req.URL.Redacted()}
	}

	ctx := req.Context()
	start := time.Now()
	interval := PendingPollStart
	for time.Since(start) < pendingPollBudget {
		drain(resp)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		if interval = interval * 3 / 2; interval > pendingPollMax {
			interval = pendingPollMax
		}
		poll := cloneWithBody(req, body)
		poll.Header.Set("response-id", responseID)
		if resp, err = send(poll); err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusAccepted {
			return resp, nil
		}
		if id := resp.Header.Get("response-id"); id != "" {
			responseID = id
		}
	}
	drain(resp)
	return nil, &ErrPendingTimeout{ResponseID: responseID, Waited: time.Since(start)}
}

func snapshotBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	defer req.Body.Close()
	return io.ReadAll(req.Body)
}

func cloneWithBody(req *http.Request, body []byte) *http.Request {
	out := req.Clone(req.Context())
	if body == nil {
		out.Body = http.NoBody
		out.GetBody = nil
		return out
	}
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	out.ContentLength = int64(len(body))
	return out
}

// drainMaxBytes and drainMaxWait bound how much of a discarded body (a 202
// pending answer, a 401 before the replay) drain reads so the connection can
// be reused.
var (
	drainMaxBytes int64 = 64 << 10
	drainMaxWait        = 2 * time.Second
)

// drain discards a response body the transport will not return, then closes
// it. It never waits on a body that does not finish: the streaming download
// client has no overall timeout, so an unbounded read of a 202 body that
// never ends hung a quarantine download forever (Codex round 2). Reading
// stops after drainMaxBytes or drainMaxWait, whichever comes first; Close
// then drops the connection and ends the read (handfixes.json:
// pending-body-drain-is-bounded).
func drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, drainMaxBytes))
	}()
	timer := time.NewTimer(drainMaxWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	_ = resp.Body.Close()
}
