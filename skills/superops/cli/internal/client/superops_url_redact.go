// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the client never shows a username or password
// from its base URL (superops-base-url-never-printed). Wired into the
// generated displayURL, maskCredentialText and maskError.

package client

import "superops-pp-cli/internal/cliutil"

// baseURLUserinfoSecrets is the userinfo of this client's base URL as
// credential values (cliutil.URLUserinfoSecrets), for maskCredentialText to
// mask and hold like any other credential.
func (c *Client) baseURLUserinfoSecrets() []string {
	if c == nil {
		return nil
	}
	out := cliutil.URLUserinfoSecrets(c.BaseURL)
	if c.Config != nil {
		out = append(out, cliutil.URLUserinfoSecrets(c.Config.BaseURL)...)
	}
	return out
}

// urlRedactedError is an error whose text lost only a URL's userinfo. Unlike
// maskedError it keeps the original reachable through errors.Is and
// errors.As: net/http's url.Error carries the username (it masks only the
// password), and a timeout or a refused connection must still classify as one
// (data-source fallback, workflow timeouts).
type urlRedactedError struct {
	msg string
	err error
}

func (e urlRedactedError) Error() string { return e.msg }
func (e urlRedactedError) Unwrap() error { return e.err }

// urlUserinfoOnlyMasked returns err with the text msg, still unwrapping to
// err, when msg is raw with only URL userinfo removed; nil otherwise (a
// credential was masked, so nothing may unwrap to the original wording).
func urlUserinfoOnlyMasked(err error, raw, msg string) error {
	if msg != cliutil.RedactURLUserinfo(raw) {
		return nil
	}
	return urlRedactedError{msg: msg, err: err}
}
