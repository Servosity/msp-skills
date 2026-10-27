// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the one visa discovery starts at
// every "visa" name and holds only values with the visa shape (Codex round 2
// and the round 2 adversarial review, press 4.34.0 reprint). Hand-fix code:
// cove-short-credential-funnel.

package coverpc

import "testing"

const scanVisa = "FRESHVISAq8W3e5R7t9Y1u2I4K"

func discovers(body, want string) bool {
	for _, v := range Visas([]byte(body)) {
		if v == want {
			return true
		}
	}
	return false
}

// Every shape a visa can follow its name in is discovered, including the one
// the old fallback pattern missed: an earlier "name:" pair swallowing it.
func TestVisaScannerFindsEveryNamedVisa(t *testing.T) {
	for name, body := range map[string]string{
		"name then visa=X":       "error: visa=" + scanVisa,
		"success text":           "result: visa=" + scanVisa,
		"spaced and annotated":   "upstream: visa = " + scanVisa + " (rotated)",
		"prose":                  "expired; renew with visa " + scanVisa,
		"query string":           "GET /jsonapi?a=1&visa=" + scanVisa + "&b=2",
		"percent encoded":        "msg=renew%20visa%3D" + scanVisa,
		"xml element":            "<r><visa>" + scanVisa + "</visa></r>",
		"inside a JSON string":   `{"message":"bad visa=` + scanVisa + `"}`,
		"JSON member upper case": `{"VISA":"` + scanVisa + `"}`,
		"header line":            "X-Cove-Visa: " + scanVisa,
		"cookie header":          "Set-Cookie: visa=" + scanVisa + "; Path=/",
		"single quoted":          "{'visa': '" + scanVisa + "'}",
		"second of two names":    "visa: required; visa=" + scanVisa,
	} {
		if !discovers(body, scanVisa) {
			t.Errorf("%s: visa not discovered in %q: %q", name, body, Visas([]byte(body)))
		}
	}
}

// A word, number or null next to a visa name is not a visa: nothing is held,
// so nothing can later mask or withhold ordinary output.
func TestVisaScannerNeverHoldsANonVisa(t *testing.T) {
	for _, body := range []string{
		"visa: required",
		"visa: authentication required",
		"visa required for this call",
		`{"visa":0}`,
		`{"visa":null}`,
		`{"visa":true}`,
		`{"visa":"short-visa"}`,
		`{"visa":"has spaces in it but is long"}`,
		`{"visas":"` + scanVisa + `","provisa":"` + scanVisa + `","visa_id":"` + scanVisa + `"}`,
		`{"detail":"visa expired"}`,
	} {
		if got := Visas([]byte(body)); len(got) != 0 {
			t.Errorf("held %q from %q", got, body)
		}
	}
}

func TestIsVisaShape(t *testing.T) {
	for v, want := range map[string]bool{
		scanVisa:                       true,
		"rotated-visa-0123456789":      true,
		"abc.def_ghi~jkl+mno/pq==":     true,
		"short-visa":                   false,
		"requiredrequired with":        false,
		"0123456789abcdef":             true,
		"0123456789abcde":              false,
		"visa=" + scanVisa:             false,
		"==" + scanVisa:                false,
		scanVisa + "=x":                false,
		"FRESHVISAq8W3e5R7t9Y1u2I4K\n": false,
	} {
		if got := IsVisaShape(v); got != want {
			t.Errorf("IsVisaShape(%q) = %v, want %v", v, got, want)
		}
	}
}
