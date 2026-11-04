package credkit

import "testing"

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
