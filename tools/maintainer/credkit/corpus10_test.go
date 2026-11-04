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
// field (halopsa's custom fields): row vendor-key-as-name. A key that reads as a
// field name stays free, or every ordinary "ttl" would be withheld.
func TestCorpus10KeyBeneathDeclaredField(t *testing.T) {
	long := strings.Repeat("k3y", 40)
	for _, tc := range []struct{ body, want string }{
		{`{"customfields":[{"access_token":{"cvAbCdEfGhIjKl":true}}]}`, "cvAbCdEfGhIjKl"},
		{`{"access_token":{"a":{"cv9aBcDeFgH2jKlM":1}}}`, "cv9aBcDeFgH2jKlM"},
		{`{"access_token":{"` + long + `":true}}`, long},
		{`{"access_token":{"clsKwWx09J91.JVD+qQh/g1R=wZQ":true}}`, "clsKwWx09J91.JVD+qQh/g1R=wZQ"},
		{`{"access_token":{"a3f9c2d1e8b7":true}}`, "a3f9c2d1e8b7"},
		{`{"access_token":{"thispassphraseislongerthanany":true}}`, "thispassphraseislongerthanany"},
	} {
		resetForTest(Declarations{ResponseFields: []string{"access_token"}})
		scanAll(strings.NewReader(tc.body), []string{"access_token"}, asJSON)
		if !isHeld(tc.want) {
			t.Errorf("%s: key %q not held", tc.body, tc.want)
		}
	}
	for _, name := range []string{"ttl", "value", "expires_in", "expiresAt", "userId", "token_type", "refreshToken", "client-id", "ID", "sha256", "accessTokenExpiresAt", "X-Request-Id"} {
		resetForTest(Declarations{ResponseFields: []string{"access_token"}})
		scanAll(strings.NewReader(`{"access_token":{"`+name+`":1}}`), []string{"access_token"}, asJSON)
		if isHeld(name) {
			t.Errorf("a field name %q held", name)
		}
	}
	// A key outside any declared field is not held, whatever it reads as.
	resetForTest(Declarations{ResponseFields: []string{"access_token"}})
	scanAll(strings.NewReader(`{"data":{"cv9OutsideKeyValue":1},"access_token":"tokValue12345"}`), []string{"access_token"}, asJSON)
	if isHeld("cv9OutsideKeyValue") || !isHeld("tokValue12345") {
		t.Errorf("held %v", heldValues())
	}
}
