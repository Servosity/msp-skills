// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-bms handfix credential-endpoint-result-held: the answer of an endpoint
// whose path names a credential is held whole, whatever its member names.

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"kaseya-bms-pp-cli/internal/cliutil"
)

type kbmsFixedAnswer string

func (b kbmsFixedAnswer) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(bytes.NewReader([]byte(b))), Request: req}, nil
}

func kbmsAnswer(t *testing.T, path, body string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "https://api.bms.kaseya.com"+path, nil)
	resp, err := credentialResultTripper{next: kbmsFixedAnswer(body)}.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
}

func TestCredentialEndpointResultIsHeldUnderAnyName(t *testing.T) {
	inHoldingChild(t)
	const itg = "itgPasswordValue-4c1e9b7a2d"
	kbmsAnswer(t, "/v2/integrations/itglue/passwords/1001", `{"Success":true,"Error":null,"Result":"`+itg+`"}`)
	if !cliutil.Held("the password is " + itg) {
		t.Fatalf("the IT Glue password a credential endpoint returned as a bare Result string is not held")
	}
	// An object answer keeps the name-based holding, which is the kit's
	// (ResponseFields; kbmsblackbox proves the token pair a refresh returns
	// never prints): its expiry is data, not held here.
	const exp = "2031-04-05T06:07:08Z"
	kbmsAnswer(t, "/v2/security/refreshtoken", `{"Success":true,"Result":{"AccessToken":"refreshAnswerToken-7d3a1f9c","AccessTokenExpireOn":"`+exp+`"}}`)
	if cliutil.Held(exp) {
		t.Fatalf("the expiry timestamp a credential endpoint returned was held")
	}
	const plain = "plainSecretBody-2b8e6c1a"
	kbmsAnswer(t, "/v2/integrations/itglue/passwords/1002", plain)
	if !cliutil.Held(plain) {
		t.Fatalf("a non-JSON credential answer is not held")
	}
}

func TestOrdinaryResultIsNotHeld(t *testing.T) {
	const title = "Printer jammed on floor 3-9f2c"
	kbmsAnswer(t, "/v2/servicedesk/tickets/5000", `{"Success":true,"Result":{"Id":5000,"Title":"`+title+`"}}`)
	if cliutil.Held(title) {
		t.Fatalf("an ordinary ticket title was held: the credential-path rule over-matches")
	}
}
