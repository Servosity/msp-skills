// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "token-response-redaction" entry.

package client

import (
	"strings"
	"testing"
)

// TestRedactTokenResponseWithholdsEscapedMemberNames: a token member whose
// NAME is escaped (access\u005ftoken, access%5Ftoken, access&#95;token) slips
// past a literal-name pattern; the decoded check withholds the whole text.
func TestRedactTokenResponseWithholdsEscapedMemberNames(t *testing.T) {
	const tok = "abc-demo-access-value"
	for _, body := range []string{
		`{"error":"x","error_description":"Rejected: {\"access\\u005ftoken\":\"\\u0061bc-demo-access-value\"}"}`,
		`{"error":"x","error_description":"Rejected: access%5Ftoken=abc-demo-access-value"}`,
		`{"error":"x","error_description":"Rejected: access&#95;token=abc-demo-access-value"}`,
		`error=x&refresh\u005ftoken=abc-demo-access-value`,
		`{"error":"x","error_description":"Rejected: {\"access%5Ftoken\":\"abc-demo-access-value\"}; 100%"}`,
	} {
		got := RedactTokenResponse(body)
		if strings.Contains(got, tok) || strings.Contains(got, `\u0061bc-demo-access`) {
			t.Fatalf("token survived redaction:\n in: %s\nout: %s", body, got)
		}
	}
	// An ordinary Azure AD error stays readable.
	plain := `{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided."}`
	if got := RedactTokenResponse(plain); !strings.Contains(got, "AADSTS7000215") {
		t.Fatalf("a plain error was withheld: %s", got)
	}
}

// TestTokenErrorTextShowsOnlyPlainProse: the class fix for the redaction
// findings. Whatever an authority puts in its error, only an enumerated OAuth
// code and AADSTS numbers are shown; no vendor prose reaches the output.
func TestTokenErrorTextShowsOnlyPlainProse(t *testing.T) {
	const tok = "abc-demo-access-value"
	for _, body := range []string{
		`{"error":"x","error_description":"Rejected: {\"access%5Ftoken\":\"abc-demo-access-value\"}; 100%"}`,
		`{"error":"x","error_description":"Rejected: {\"access\\u005ftoken\":\"\\u0061bc-demo-access-value\"}"}`,
		`{"error":"x","error_description":"access token abc-demo-access-value"}`,
		`{"error":"x","error_description":"YWNjZXNzX3Rva2VuPWFiYy1kZW1vLWFjY2Vzcy12YWx1ZQ== abc-demo-access-value=1"}`,
		`{"error":"x","error_description":"eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.sig"}`,
		`access_token=abc-demo-access-value`,
		`<html>abc-demo-access-value</html>`,
		`{"error":"abc_demo_access_value_TOKENlike","error_description":"x"}`,
		`{"error":"x","error_description":"Rejected YWNjZXNzX3Rva2VuOmFiYy1kZW1vLWFjY2Vzcy12YWx1ZQ"}`,
		// Codex round 3: the token echoed in plain prose next to an AADSTS code.
		`{"error":"invalid_client","access_token":"\u0061bc-demo-access-value","error_description":"AADSTS7000215: abc-demo-access-value"}`,
		`{"error":"invalid_client","error_description":"AADSTS7000215 abc demo access value"}`,
	} {
		got := TokenErrorText([]byte(body))
		for _, leak := range []string{tok, "eyJhbGci", "YWNjZXNzX3Rva2Vu", "TOKENlike", "abc demo access"} {
			if strings.Contains(got, leak) {
				t.Fatalf("vendor text survived:\n in: %s\nout: %s", body, got)
			}
		}
	}
	for body, want := range map[string]string{
		`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided. Trace ID: 00000000-0000-0000-0000-000000000001"}`: "invalid_client: AADSTS7000215 (description withheld)",
		`{"error":"invalid_client","access_token":"x","error_description":"AADSTS7000215: minted_bearer_value"}`:                                         "invalid_client: AADSTS7000215 (description withheld)",
		`{"error":"made_up_code","error_description":"AADSTS700016 and AADSTS90002"}`:                                                                    "(code withheld): AADSTS700016, AADSTS90002 (description withheld)",
		`{"error":"temporarily_unavailable"}`: "temporarily_unavailable",
	} {
		if got := TokenErrorText([]byte(body)); got != want {
			t.Fatalf("TokenErrorText(%s) = %q, want %q", body, got, want)
		}
	}
}
