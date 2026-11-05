// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/url"
	"slices"
	"testing"
)

func heldBy(t *testing.T, f func()) []string {
	t.Helper()
	var got []string
	old, oldURL := Hold, HoldURL
	Hold = func(_ string, values ...string) { got = append(got, values...) }
	HoldURL = func(_ string, raw string) { got = append(got, "url:"+raw) }
	defer func() { Hold, HoldURL = old, oldURL }()
	f()
	return got
}

// TestBaseURLTableIsStillSearched: a table under a base-url key is a config table, so its
// credential fields are held like any other (the MCP server re-reads the file per call).
func TestBaseURLTableIsStillSearched(t *testing.T) {
	for _, file := range []string{
		"[base-url]\npassword = \"Qz7rT9kLpV2mNxW4\"\n",
		"[\"base url\"]\ndevice_id = \"Qz7rT9kLpV2mNxW4\"\n",
		"[[base-url]]\npassword = \"Qz7rT9kLpV2mNxW4\"\n",
		"[baseUrl.x]\nsid = \"Qz7rT9kLpV2mNxW4\"\n",
	} {
		if got := heldBy(t, func() { HoldConfigFile([]byte(file)) }); !slices.Contains(got, "Qz7rT9kLpV2mNxW4") {
			t.Errorf("%q: credential not held, held %q", file, got)
		}
	}
}

// TestFeedbackEndpointHoldsOnlyCredentials: the feedback endpoint is sent nowhere but its
// own host, so only its password and credential-named query values reach the kit's URL
// reader; an ordinary query such as ?v=1 must not withhold every line that carries a 1.
func TestFeedbackEndpointHoldsOnlyCredentials(t *testing.T) {
	if got := heldBy(t, func() { HoldURLCredentials(SentNowhere, "https://hooks.example.net/fb?v=1&lang=en#top=1") }); len(got) != 0 {
		t.Errorf("ordinary query values held: %q", got)
	}
	got := heldBy(t, func() {
		HoldURLCredentials(SentNowhere, "https://u:UrlPw-9kLp@hooks.example.net/fb?%70assword=First?Second&token=Tok-7Yh&v=1")
	})
	if len(got) != 1 {
		t.Fatalf("want one URL for the kit, got %q", got)
	}
	u, err := url.Parse(got[0][len("url:"):])
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := u.User.Password(); p != "UrlPw-9kLp" {
		t.Errorf("password not passed: %q", got[0])
	}
	q := u.Query()
	if q.Get("password") != "First?Second" || q.Get("token") != "Tok-7Yh" || q.Has("v") {
		t.Errorf("credential query values wrong: %q", got[0])
	}
	// Does not parse: the kit gets it whole.
	if got := heldBy(t, func() { HoldURLCredentials(SentNowhere, "https://h/x?a=1;password=Zz9") }); len(got) != 1 || got[0] != "url:https://h/x?a=1;password=Zz9" {
		t.Errorf("unparsable query not handed over whole: %q", got)
	}
}
