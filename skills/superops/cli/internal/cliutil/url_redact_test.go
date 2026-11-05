// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for url_redact.go
// (superops-base-url-never-printed).

package cliutil

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	redactTestUser = "redactUserYt63"
	redactTestPass = "redactPassMw28"
)

func TestDisplayURLNeverShowsUserinfo(t *testing.T) {
	ui := redactTestUser + ":" + redactTestPass + "@"
	for _, tc := range []struct{ in, want string }{
		{"http://" + ui + "127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"https://" + redactTestUser + "@api.superops.ai", "https://api.superops.ai"},
		{"https://" + ui + "proxy.example:8443/superops/", "https://proxy.example:8443/superops"},
		{"http://" + ui + "127.0.0.1:8080/msp", "http://127.0.0.1:8080/msp"},
		{"  http://" + ui + "127.0.0.1:8080  ", "http://127.0.0.1:8080"},
		{"http://" + redactTestUser + ":p%40ss@host.example", "http://host.example"},
		{"http://a@" + ui + "host.example", "http://host.example"},
		// Query and fragment can carry a credential and never name the
		// server, so a display drops them.
		{"http://" + ui + "host.example/?sig=" + redactTestPass + "#" + redactTestPass, "http://host.example"},
		{"https://api.superops.ai", "https://api.superops.ai"},
		{"https://euapi.superops.ai/", "https://euapi.superops.ai"},
		{"", ""},
		// A path reference has no authority, so no userinfo: shown as is.
		{"/msp", "/msp"},
		{"/graphql?first=50", "/graphql?first=50"},
		{redactTestUser + "@host.example", URLWithheld},
		// Never parsed as scheme://host: the placeholder, none of the text.
		{"http://" + ui + "127.0.0.1:notaport", URLWithheld},
		{"http://" + ui + "127.0.0.1:notaport/msp", URLWithheld},
		{"http:" + ui + "host.example", URLWithheld},
		{ui + "host.example", URLWithheld},
		{"//" + ui + "host.example", URLWithheld},
		{"http://" + redactTestUser + ":" + redactTestPass + `\@host.example`, URLWithheld},
		{"http://" + redactTestUser + "/x:" + redactTestPass + "@host.example", URLWithheld},
		// The parser saw no userinfo, but an '@' is left in the path, query
		// or fragment: what it shows as the host may be the username.
		{"http://" + redactTestUser + ":12#" + redactTestPass + "@host.example", URLWithheld},
		{"http://" + redactTestUser + ":12?" + redactTestPass + "@host.example", URLWithheld},
		// The same, with the '@' percent-encoded once or twice.
		{"https://" + redactTestUser + ":443/" + redactTestPass + "%40api.superops.ai", URLWithheld},
		{"https://" + redactTestUser + ":443/" + redactTestPass + "%2540api.superops.ai", URLWithheld},
		{"https://" + redactTestUser + ":12?" + redactTestPass + "%2540api.superops.ai", URLWithheld},
		{"https://" + redactTestUser + ":12#" + redactTestPass + "%40api.superops.ai", URLWithheld},
	} {
		got := DisplayURL(tc.in)
		if got != tc.want {
			t.Errorf("DisplayURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
		low := strings.ToLower(got)
		for _, secret := range []string{redactTestUser, redactTestPass} {
			if strings.Contains(low, strings.ToLower(secret)) {
				t.Errorf("DisplayURL(%q) = %q shows %q", tc.in, got, secret)
			}
		}
	}
}

// URLWithoutUserinfo is the rule the tenant identity uses (lowercased there):
// scheme, host, port and path; false for anything that is not scheme://host.
func TestURLWithoutUserinfo(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"http://u:p@127.0.0.1:8080/", "http://127.0.0.1:8080", true},
		{"https://Api.SuperOps.ai/MSP/", "https://Api.SuperOps.ai/MSP", true},
		{"https://h.example/?q=1#f", "https://h.example", true},
		{"http://u:p@h:bad", "", false},
		{"https://svc:443/pw%40api.superops.ai", "", false},
		{"/msp", "", false},
		{"", "", false},
	} {
		got, ok := URLWithoutUserinfo(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("URLWithoutUserinfo(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// An '@' in any percent encoding outside the userinfo url.Parse takes means
// part of a username or password was read as host, port, path, query or
// fragment. A real userinfo, an escaped '@' inside it and an IPv6 zone are not.
func TestURLUserinfoDisplaced(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://svc:443/pw@api.superops.ai", true},
		{"https://svc:443/pw%40api.superops.ai", true},
		{"https://svc:443/pw%2540api.superops.ai", true},
		{"https://svc:443/pw%252540api.superops.ai", true},
		{"https://svc:12?pw@api.superops.ai", true},
		{"https://svc:12#pw%40api.superops.ai", true},
		{"https://svc:pw%2Fx%40api.superops.ai", true},
		{"https://u:p@h.example/pw%40x", true},
		{"svc:pw/x@h.example", true},
		{"https%3A%2F%2Fsvc%3Apw%40h.example", true},
		{"https://svc:pw%40x%zz/msp", true},
		{"https://api.superops.ai", false},
		{"https://u:p%40ss@h.example/msp", false},
		{"http://a@u:p@h.example", false},
		{"http://u:p@[fe80::1%25en0]:8080", false},
		{"https://h.example/msp?x=%zz", false},
		{"/msp", false},
		{"", false},
	} {
		if got := URLUserinfoDisplaced(tc.in); got != tc.want {
			t.Errorf("URLUserinfoDisplaced(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// A '?' or '#' outside the userinfo starts a query or fragment, which a base
// URL never needs; one escaped inside a password does not.
func TestURLHasQueryOrFragment(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://api.superops.ai/?sig=x", true},
		{"https://api.superops.ai?sig=x", true},
		{"https://api.superops.ai#f", true},
		{"https://api.superops.ai/msp?", true},
		{"http://u:p@h.example/x?y", true},
		{"http://u:12?pw@h.example", true},
		{"https://api.superops.ai", false},
		{"https://u:p%3Fq%23r@h.example/msp", false},
		{"http://a?b@u:p@h.example", true},
		{"/msp", false},
		{"", false},
	} {
		if got := URLHasQueryOrFragment(tc.in); got != tc.want {
			t.Errorf("URLHasQueryOrFragment(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestURLUserinfoTooShort(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://svc:pw@api.superops.ai", true},
		{"https://svc:p%40ss@api.superops.ai", true},
		{"https://Zq7x@api.superops.ai", true},
		{"https://Zq7x:@api.superops.ai", true},
		{"https://svc:Zq7xLong8@api.superops.ai/msp", false},
		{"https://Zq7xLong8@api.superops.ai", false},
		{"https://api.superops.ai", false},
		{"https://api.superops.ai/u@x", false},
		{"", false},
	} {
		if got := URLUserinfoTooShort(tc.in); got != tc.want {
			t.Errorf("URLUserinfoTooShort(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRedactURLUserinfoRemovesItFromText(t *testing.T) {
	u, p := redactTestUser, redactTestPass
	for _, tc := range []struct{ in, want string }{
		// net/http's url.Error masks the password and keeps the username.
		{`GET /msp: Get "http://` + u + `:***@127.0.0.1:9/msp": dial tcp 127.0.0.1:9: connect: connection refused`,
			`GET /msp: Get "http://127.0.0.1:9/msp": dial tcp 127.0.0.1:9: connect: connection refused`},
		// url.Parse errors quote the whole raw URL.
		{`creating request: parse "http://` + u + `:` + p + `@127.0.0.1:notaport/msp": invalid port ":notaport" after host`,
			`creating request: parse "http://127.0.0.1:notaport/msp": invalid port ":notaport" after host`},
		{"base_url http://" + u + "@h.example and https://" + u + ":" + p + "@k.example:8443/x",
			"base_url http://h.example and https://k.example:8443/x"},
		{"http://a@" + u + ":" + p + "@h.example/x", "http://h.example/x"},
		{"(see http://" + u + ":" + p + "@h.example)", "(see http://h.example)"},
		// JSON with escaped slashes, and the percent-encoded form.
		{`{"url":"http:\/\/` + u + `:` + p + `@h.example\/msp"}`, `{"url":"http:\/\/h.example\/msp"}`},
		{"next=http%3A%2F%2F" + u + "%3A" + p + "%40h.example%2Fmsp", "next=http%3A%2F%2Fh.example%2Fmsp"},
		{"next=HTTP%3a%2f%2f" + u + "%40h.example", "next=HTTP%3a%2f%2fh.example"},
		// Nothing to remove: text is unchanged.
		{"contact bob@example.com about it", "contact bob@example.com about it"},
		{"mailto:bob@example.com", "mailto:bob@example.com"},
		{"https://api.superops.ai/msp", "https://api.superops.ai/msp"},
		{"http://h.example/users/bob@example.com?x=a@b#c@d", "http://h.example/users/bob@example.com?x=a@b#c@d"},
		{"http://h.example then mail bob@example.com", "http://h.example then mail bob@example.com"},
		{"", ""},
	} {
		if got := RedactURLUserinfo(tc.in); got != tc.want {
			t.Errorf("RedactURLUserinfo(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

func TestURLUserinfoSecrets(t *testing.T) {
	got := URLUserinfoSecrets("http://" + redactTestUser + ":" + redactTestPass + "@h.example")
	for _, want := range []string{redactTestUser, redactTestPass, redactTestUser + ":" + redactTestPass} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("URLUserinfoSecrets misses %q: %q", want, got)
		}
	}
	// No length exemption (credkit-wiring): a short username and
	// a short password are both credentials.
	got = URLUserinfoSecrets("http://svc:pw@h.example")
	for _, want := range []string{"svc", "pw", "svc:pw"} {
		if !slices.Contains(got, want) {
			t.Errorf("URLUserinfoSecrets dropped %q: %q", want, got)
		}
	}
	// The Basic header net/http sends from the userinfo, which a server can
	// echo, is a credential at any length.
	if basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("svc:pw")); !strings.Contains(strings.Join(got, "\n"), basic) {
		t.Errorf("URLUserinfoSecrets misses %q: %q", basic, got)
	}
	for _, none := range []string{"https://api.superops.ai", "/msp", "", "https://h.example/users/a@b"} {
		if got := URLUserinfoSecrets(none); len(got) != 0 {
			t.Errorf("URLUserinfoSecrets(%q): no userinfo, got %q", none, got)
		}
	}
	if got := URLUserinfoSecrets(redactTestUser + ":" + redactTestPass + "@h.example"); len(got) == 0 {
		t.Errorf("userinfo written without a scheme was not read: %q", got)
	}
	got = URLUserinfoSecrets("http://" + redactTestUser + ":p%40ss%2Fx@h.example")
	if !strings.Contains(strings.Join(got, "\n"), "p@ss/x") || !strings.Contains(strings.Join(got, "\n"), "p%40ss%2Fx") {
		t.Errorf("an escaped password is held decoded and as written: %q", got)
	}
	if got := URLUserinfoSecrets("http://" + redactTestUser + ":" + redactTestPass + "@h:bad"); len(got) == 0 {
		t.Errorf("a URL that does not parse still holds its userinfo: %q", got)
	}
}

// A URL's userinfo goes to the kit.
func TestHoldURLUserinfoFeedsTheKit(t *testing.T) {
	user, pass := "heldUserQq19Lm", "heldPassVb33Zx"
	HoldURLUserinfo("http://" + user + ":" + pass + "@h.example")
	for _, v := range []string{user, pass} {
		if !isHeld(v) {
			t.Errorf("%s was not held", v)
		}
	}
}

// An error body can be up to the response cap and hold many URLs: the scan is
// linear, so a large text with a URL every few bytes is fast.
func TestRedactURLUserinfoIsLinear(t *testing.T) {
	text := strings.Repeat("see http://u:p@h.example/x and ", 40000)
	start := time.Now()
	got := RedactURLUserinfo(text)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("RedactURLUserinfo took %s on %d bytes", elapsed, len(text))
	}
	if strings.Contains(got, "u:p@") || !strings.Contains(got, "see http://h.example/x and ") {
		t.Fatalf("large text not redacted: %q", got[:80])
	}
}
