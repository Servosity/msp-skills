// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// TestTokenErrorShowsOnlyAnAllowlistedCode: the token endpoint is where the
// password is sent, so its free text is never shown, in any encoding (Codex
// review r1 of the publish run: a fullwidth rot13 echo of the password in
// error_description). Only a known OAuth2 error code is shown.
func TestTokenErrorShowsOnlyAnAllowlistedCode(t *testing.T) {
	for _, desc := range []string{"Ｃ＠ｆｆ－Ｊ０！", "User account is disabled", "anything at all"} {
		got := esetTokenErrorReason(esetTokenResponse{Error: "invalid_grant", ErrorDescription: desc})
		if got != "invalid_grant" {
			t.Errorf("description %q: reason %q, want only the code", desc, got)
		}
		if strings.Contains(got, desc) {
			t.Errorf("description %q shown: %q", desc, got)
		}
	}
	if got := esetTokenErrorReason(esetTokenResponse{Error: "unauthorized_client", ErrorDescription: "invalid_username_or_password"}); got != "unauthorized_client: invalid_username_or_password" {
		t.Errorf("ESET's code in the description: %q", got)
	}
	if got := esetTokenErrorReason(esetTokenResponse{Error: "made up", ErrorDescription: "x"}); got != "unrecognized error code" {
		t.Errorf("unknown code: %q", got)
	}
	if got := esetTokenErrorReason(esetTokenResponse{ErrorDescription: "x"}); got != "no access_token in response" {
		t.Errorf("no code: %q", got)
	}
}
