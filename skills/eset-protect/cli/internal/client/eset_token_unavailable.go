// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "auto-falls-back-when-token-service-is-down".
//
// The ESET Connect token is minted while the client is built. When that mint
// fails for a reason that is not a credential rejection (the token service
// answers 5xx or 429, or cannot be reached), the client is still returned,
// carrying the failure: every request then fails at once with it, without
// touching the network or the response cache and without retries. A read in
// --data-source auto treats it like an unreachable API and answers from the
// local mirror (isNetworkError); every other caller gets the same error, and
// the same exit status (5), that the client constructor used to return. A
// credential rejection never gets here: the constructor still fails (exit 4).
package client

import (
	"errors"
	"sync/atomic"
)

// TokenUnavailableError is a request refused because the token mint failed
// for a reason that is not a credential rejection. Err is the classified mint
// failure; Error and the exit status are exactly that failure's.
type TokenUnavailableError struct {
	Err error
}

func (e *TokenUnavailableError) Error() string { return e.Err.Error() }
func (e *TokenUnavailableError) Unwrap() error { return e.Err }

// IsTokenUnavailable reports whether err is (or wraps) a TokenUnavailableError.
func IsTokenUnavailable(err error) bool {
	var t *TokenUnavailableError
	return errors.As(err, &t)
}

// tokenUnavailable is the per-transport verdict; the client reads it through
// its EsetTransport.
type tokenUnavailable struct {
	err atomic.Value // *TokenUnavailableError
}

// SetTokenUnavailable records that no token could be minted for this client
// (cause is the classified mint failure). Requests through the transport's
// client then fail with a TokenUnavailableError.
func (t *EsetTransport) SetTokenUnavailable(cause error) {
	t.unavailable.err.Store(&TokenUnavailableError{Err: cause})
}

// TokenUnavailable returns the recorded mint failure, or nil.
func (t *EsetTransport) TokenUnavailable() error {
	if t == nil {
		return nil
	}
	if v, ok := t.unavailable.err.Load().(*TokenUnavailableError); ok && v != nil {
		return v
	}
	return nil
}

// tokenUnavailable returns the mint failure recorded on c's ESET transport.
func (c *Client) tokenUnavailable() error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	if t, ok := c.HTTPClient.Transport.(*EsetTransport); ok {
		return t.TokenUnavailable()
	}
	return nil
}
