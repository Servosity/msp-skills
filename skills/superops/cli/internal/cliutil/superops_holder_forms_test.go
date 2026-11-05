// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring matching and
// collection. A held value is found in every decoded form, at any length, and
// A URL credential is collected in every form; an email address is not.

package cliutil

import (
	"fmt"
	"strings"
	"testing"
)

func percentEveryByte(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "%%%02X", s[i])
	}
	return b.String()
}

func jsonUnicodeEscapes(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

func htmlHexEntities(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, "&#x%x;", r)
	}
	return b.String()
}

// A header value's parts are each a credential: the value after a scheme word
// and every value of a name=value list.
func TestHeaderComponentsAreHeld(t *testing.T) {
	cfg := struct{ Headers map[string]string }{map[string]string{
		"Cookie":        "sid=CookieSid4340x; theme=dark",
		"Authorization": `Digest username="hc-user", response="DigestResp4340x"`,
		"X-Auth":        "Token HdrTokPart4340x",
	}}
	got := strings.Join(CredentialStrings(&cfg), "\n")
	for _, v := range []string{"CookieSid4340x", "DigestResp4340x", "HdrTokPart4340x"} {
		if !strings.Contains("\n"+got+"\n", "\n"+v+"\n") {
			t.Errorf("header component %q is not a credential string:\n%s", v, got)
		}
	}
}
