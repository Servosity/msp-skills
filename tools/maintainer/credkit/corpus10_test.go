package credkit

import (
	"strings"
	"testing"
)

// corpus10rv, the leak list's first run on a real adopter (axcient): a password
// written raw in a base URL, with a '/' in it, ended the authority early, so the
// kit held nothing and url.Parse's own error printed it whole.
func TestCorpus10BaseURLPasswordWithSlash(t *testing.T) {
	for _, tc := range []struct{ url, pw string }{
		{"https://cuAbCd:clsKsynthabc.xyz+abc/def=ghi@127.0.0.1:9", "clsKsynthabc.xyz+abc/def=ghi"},
		{"https://cuAbCd:12345/def=ghijk@127.0.0.1:9", "12345/def=ghijk"}, // parses: host cuAbCd, port 12345
		{"https://cuAbCd:clsK?synth#abc@127.0.0.1:9", "clsK?synth#abc"},
	} {
		d := Declarations{BaseURL: BaseURL{ConfigKey: "base_url"}}
		gatherString(t, d, "config.toml", `base_url = "`+tc.url+`"`+"\n")
		if !isHeld(tc.pw) {
			t.Errorf("%s: password %q not held", tc.url, tc.pw)
		}
	}
	// Through the declared variable, with a line feed in the password (Codex round 5).
	d := Declarations{BaseURL: BaseURL{Env: "ACME_BASE_URL"}}
	resetForTest(d)
	atStartup(func() { holdBaseURLLocked("https://u:syntheticFirst4829\nsyntheticSecond1375@api.example.invalid") })
	if !isHeld("syntheticFirst4829\nsyntheticSecond1375") {
		t.Error("a line feed in a raw password ended the reading")
	}
	// An ordinary base URL holds nothing new.
	d = Declarations{BaseURL: BaseURL{ConfigKey: "base_url"}}
	gatherString(t, d, "config.toml", `base_url = "https://api.example.com:8443/v1/items"`+"\n")
	if isHeld("8443/v1/items") || isHeld("api.example.com") {
		t.Error("an ordinary base URL held a piece of itself")
	}
}

// A credential a vendor stores as an object key beneath a declared response
// field (halopsa's custom fields): row vendor-key-as-name. Every such key, spelled
// any way; a key outside a declared field is not held.
func TestCorpus10KeyBeneathDeclaredField(t *testing.T) {
	long := strings.Repeat("k3y", 40)
	for _, key := range []string{"cvAbCdEfGhIjKl", "clsKwWx09J91.JVD+qQh/g1R=wZQ", "deadbeef271828", "correcthorse", "secret-secret-secret-secret", "ttl", long} {
		for _, body := range []string{`{"customfields":[{"access_token":{"` + key + `":true}}]}`, `{"access_token":{"a":{"` + key + `":1}}}`, `{"access_token":{"` + key + `":true}}`} {
			resetForTest(Declarations{ResponseFields: []string{"access_token"}})
			scanAll(strings.NewReader(body), []string{"access_token"}, asJSON)
			if !isHeld(key) {
				t.Errorf("%s: key %q not held", body, key)
			}
		}
	}
	resetForTest(Declarations{ResponseFields: []string{"access_token"}})
	scanAll(strings.NewReader(`{"data":{"cv9OutsideKeyValue":1},"access_token":"tokValue12345"}`), []string{"access_token"}, asJSON)
	if isHeld("cv9OutsideKeyValue") || !isHeld("tokValue12345") {
		t.Errorf("held %v", heldValues())
	}
}

// The kit's identifier exemption reaches a key beneath the field it names.
func TestCorpus10KeyBeneathIdentifierField(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"label"}, Identifiers: []Identifier{{Source: "response:label", Reason: "public"}}})
	scanAll(strings.NewReader(`{"label":{"cvAbCdEfGhIjKl":1}}`), []string{"label"}, asJSON)
	if isHeld("cvAbCdEfGhIjKl") {
		t.Errorf("an identifier's key held: %v", heldValues())
	}
}
