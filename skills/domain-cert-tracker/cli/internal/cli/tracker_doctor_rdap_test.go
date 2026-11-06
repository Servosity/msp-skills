// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// A /help answer that is null, an array, {}, an error object (RFC 9083
// section 6) or carries a malformed rdapConformance (section 4.1) is not a
// working RDAP server.
func TestIsRDAPResponse(t *testing.T) {
	for body, want := range map[string]bool{
		`null`:                               false,
		`[]`:                                 false,
		`{}`:                                 false,
		`{"error":"backend unavailable"}`:    false,
		`{"rdapConformance":[]}`:             false,
		`{"rdapConformance":"rdap_level_0"}`: false,
		`{"rdapConformance":[null]}`:         false,
		`{"rdapConformance":[""]}`:           false,
		`{"rdapConformance":["rdap_level_0",null]}`:         false,
		`{"rdapConformance":["rdap_level_0"],"notices":[]}`: true,
		`{"errorCode":503,"title":"unavailable"}`:           false,
		`{"data":[],"items":[],"results":[]}`:               true,
	} {
		if got := isRDAPResponse([]byte(body)); got != want {
			t.Errorf("%s: got %v want %v", body, got, want)
		}
	}
}
