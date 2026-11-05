// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring.

package cliutil

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// held captures what the code gives the kit (Hold): the kit's held set has no
// reset, so these tests read the values handed over, not the kit's verdict. The
// kit's matching is its own (tools/maintainer/credkit corpus tests).
var held []string

func init() {
	Hold = func(_ string, values ...string) { held = append(held, values...) }
}

func isHeld(v string) bool { return slices.Contains(held, v) }

// Every SUPEROPS_* value but the named settings, and every URL userinfo in
// any environment value or argument, is held when the process starts.
func TestHoldProcessCredentials(t *testing.T) {
	holdProcessCredentials([]string{
		"SUPEROPS_API_TOKEN=envTokQ~1",
		"SUPEROPS_MCP_HTTP_TOKEN=httpTokQ~2",
		"superops_future_secret=futQ~3",
		"SUPEROPS_REGION=Q~region",
		"HTTPS_PROXY=http://pxQ~:pxPassQ~4@proxy.example:8080",
		"EMAIL=mailQ~@example.com",
	}, []string{"superops-cli", "a@b"})
	for _, want := range []string{"envTokQ~1", "httpTokQ~2", "futQ~3", "pxQ~", "pxPassQ~4"} {
		if !slices.Contains(held, want) {
			t.Errorf("not held: %q", want)
		}
	}
	for _, not := range []string{"Q~region", "mailQ~", "a"} {
		if slices.Contains(held, not) {
			t.Errorf("held a non-credential: %q", not)
		}
	}
}

// Fail-closed: every string a struct reaches is a credential unless its field
// is named non-secret; a BaseURL gives its userinfo and Headers every value
// but the non-secret names.
func TestCredentialStringsIsFailClosed(t *testing.T) {
	type inner struct{ Deep []string }
	type cfg struct {
		BaseURL   string
		Path      string
		Token     string
		NewSecret string
		Headers   map[string]string
		Nested    *inner
		Expiry    time.Time
		private   map[string]string
	}
	c := cfg{
		BaseURL:   "https://bu9:bp@h.example/x",
		Path:      "/etc/config.toml",
		Token:     "tok",
		NewSecret: "added-by-a-reprint",
		Headers:   map[string]string{"Authorization": "Bearer hdr", "customersubdomain": "acme", "X-Api-Key": "k"},
		Nested:    &inner{Deep: []string{"deep"}},
		Expiry:    time.Now(),
		private:   map[string]string{"pk": "pv"},
	}
	got := CredentialStrings(c, "Path")
	for _, want := range []string{"bu9", "bp", "tok", "added-by-a-reprint", "Bearer hdr", "k", "deep", "pk", "pv"} {
		if !slices.Contains(got, want) {
			t.Errorf("CredentialStrings misses %q: %q", want, got)
		}
	}
	for _, not := range []string{"/etc/config.toml", "acme", "https://bu9:bp@h.example/x", "h.example", "Local", "UTC"} {
		if slices.Contains(got, not) {
			t.Errorf("CredentialStrings holds non-secret %q: %q", not, got)
		}
	}
}

// A credentials file's values are held whether or not they are applied.
func TestCredentialsHoldEveryValue(t *testing.T) {
	c := &Credentials{BaseURL: "https://cu9:cpQ~6@h.example", SuperopsApiToken: "storeTokQ~7", ClientID: "cidQ~8", TokenExpiry: time.Now()}
	c.hold()
	for _, want := range []string{"cu9", "cpQ~6", "storeTokQ~7", "cidQ~8"} {
		if !slices.Contains(held, want) {
			t.Errorf("not held: %q", want)
		}
	}
}

// Round-1 input of the second next-batch review: every URL in a text gives
// its userinfo to the holder, not only the first.
func TestHoldURLUserinfoHoldsEveryURLInAText(t *testing.T) {
	HoldURLUserinfo(`mutation { f(note:"https://example.net/a https://rv-unit-user:SecondUnitPass4340@example.org/b") }`)
	for _, v := range []string{"rv-unit-user", "SecondUnitPass4340"} {
		if !isHeld(v) {
			t.Errorf("%s is not held", v)
		}
	}
}

// A credentials file's values are held at every parse, including the probe
// that runs on a file refused for its mode.
func TestCredentialsBytesHaveValuesHoldsWhatItRead(t *testing.T) {
	const heldValue = "probe-secret-4340y"
	if !credentialsBytesHaveValues([]byte("api_token = \"" + heldValue + "\"")) {
		t.Fatal("the probe found no values")
	}
	if !isHeld(heldValue) {
		t.Error("the probed token is not held")
	}
}

// A one or two letter username beside a password matches nearly all output in
// some decoded view and the kit then drops even its notice, so it is not held,
// and neither is the Basic form the kit would split it from. Alone it is the
// secret, and the password is held at any length.
func TestURLUserinfoSecretsSkipsAShortUsernameBesideAPassword(t *testing.T) {
	got := URLUserinfoSecrets("https://u:pw@h.example/x")
	if slices.Contains(got, "u") || !slices.Contains(got, "pw") {
		t.Errorf("short username beside a password: %q", got)
	}
	for _, v := range got {
		if strings.HasPrefix(v, "Basic ") {
			t.Errorf("a Basic form would hold the short username: %q", got)
		}
	}
	if got := URLUserinfoSecrets("https://t0@h.example/x"); !slices.Contains(got, "t0") {
		t.Errorf("a username with no password is the secret: %q", got)
	}
	if got := URLUserinfoSecrets("https://bob:pw@h.example/x"); !slices.Contains(got, "bob") {
		t.Errorf("a username of three bytes is held: %q", got)
	}
}
