// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"strings"
	"testing"
)

// The chokepoint redacts the password of any URL it sees, held or not
// (Codex r2 class output-before-chokepoint, fixed after the stop rule).
func TestURLPasswordsRedactedWithoutAHolder(t *testing.T) {
	for _, tc := range []struct{ text, pw string }{
		{"global credentials for https://ro:NotHeldPw-1a2b@vc.example.com were not applied", "NotHeldPw-1a2b"},
		{`untrusted config names base_url "https://ro:NotHeldPw-3c4d@other.example.com:bad/"`, "NotHeldPw-3c4d"},
		{"base_url=https://ro:Raw/Sl4sh@Pw-5e6f@other.example.com/api ok", "Raw/Sl4sh@Pw-5e6f"},
		{`{"url":"https:\/\/ro:NotHeldPw-7a8b@vc.example.com\/api"}`, "NotHeldPw-7a8b"},
	} {
		got := WithholdText(tc.text)
		if strings.Contains(got, tc.pw) || !strings.Contains(got, RedactedValue) {
			t.Errorf("%q -> %q: the URL password survived", tc.text, got)
		}
	}
	for _, keep := range []string{"https://graph.microsoft.com/v1.0/users", "https://graph.microsoft.com:443/v1.0/users?$top=5", "mailto:ro@contoso.com"} {
		if got := WithholdText(keep); got != keep {
			t.Errorf("%q changed to %q: no password in it", keep, got)
		}
	}
}

// Every value a configuration or credential file carries is held when its
// bytes are read, whether the file is then used or refused, and even when the
// file or a URL in it does not parse.
func TestHoldTOMLCredentialsHoldsEveryFileValue(t *testing.T) {
	HoldTOMLCredentials([]byte("base_url = \"https://ro:FileUrlPw-9c0d@other.example.com:bad/\"\npassword = \"FilePassw0rd-1e2f\"\n[nested]\naccess_token = \"FileToken-3a4b5c\"\n"))
	HoldTOMLCredentials([]byte("base_url = \"https://ro:Sl4sh/HeldPw-2b3c@other.example.com/api\"\n"))
	HoldTOMLCredentials([]byte("this is = not [toml\npassword = BrokenTomlPw-6d7e\nbase_url = https://u:BrokenUrlPw-8f9a@h.example.com/x\n"))
	for _, v := range []string{"FileUrlPw-9c0d", "FilePassw0rd-1e2f", "FileToken-3a4b5c", "BrokenTomlPw-6d7e", "BrokenUrlPw-8f9a", "Sl4sh/HeldPw-2b3c"} {
		if !HeldEchoes("x " + v + " x") {
			t.Errorf("%s not held", v)
		}
	}
	if HeldEchoes("other.example.com") {
		t.Errorf("a host name was held")
	}
}
