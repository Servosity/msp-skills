// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// zoom-phone hand-fix (credkit-wiring): the one rule for which field names carry
// a Zoom credential, shared by the response redaction, the store and what is held
// with the credential kit. The kit does the matching and the withholding; nothing
// here decides what leaves.

import (
	"strings"

	"zoom-phone-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an in-process
// test (no credkit.Start) would check every later statement and file against its
// fixtures. The built binaries prove the hold (internal/zoomblackbox). Production
// code never reassigns it.
var Hold = credkit.Hold

// SecretFieldKeys are the JSON fields that carry Zoom credentials (device SIP
// passwords, desk-phone and port-order PINs, smartphone activation codes, Team
// Chat channel tokens), in requests and responses alike.
var SecretFieldKeys = map[string]bool{
	"password":        true,
	"pin_code":        true,
	"pin":             true,
	"activation_code": true,
	"token":           true,
}

// tokenFieldKeys are the OAuth fields of a token request or response.
var tokenFieldKeys = map[string]bool{
	"access_token":  true,
	"refresh_token": true,
	"id_token":      true,
	"client_secret": true,
}

// zoomCursorNames end like a credential field but are pagination cursors.
var zoomCursorNames = map[string]bool{"page_token": true, "next_page_token": true, "sync_token": true}

// IsCredentialFieldName reports whether a flag, argument or JSON field name
// names a credential: one of SecretFieldKeys or the OAuth token fields, a name
// ending in one of them ("desk_phone_pin_code", "sip_password"), or one
// containing secret, password, passcode or api_key. Dashes and dots read as
// underscores; pagination cursors are not credentials.
func IsCredentialFieldName(name string) bool {
	n := strings.Trim(strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(strings.TrimSpace(name))), "_")
	if n == "" || zoomCursorNames[n] {
		return false
	}
	if SecretFieldKeys[n] || tokenFieldKeys[n] {
		return true
	}
	for _, keys := range []map[string]bool{SecretFieldKeys, tokenFieldKeys} {
		for k := range keys {
			if strings.HasSuffix(n, "_"+k) {
				return true
			}
		}
	}
	for _, part := range []string{"secret", "password", "passcode", "passwd", "api_key", "apikey", "private_key", "auth_header", "authorization"} {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}
