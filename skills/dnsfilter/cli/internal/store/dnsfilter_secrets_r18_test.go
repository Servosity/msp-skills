// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests (handfixes.json: secrets-never-persisted), Codex
// round 18 and adversarial round 19.

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// r18Row is a stored row whose note is text, and the same text as JSON in a
// string one level down.
func r18Rows(t *testing.T, text string) []string {
	t.Helper()
	q := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return []string{
		`{"id":"1","note":` + q(text) + `}`,
		`{"id":"1","note":` + q(`{"memo":`+q(text)+`}`) + `}`,
	}
}

// r18Withheld asserts the text rule names text and the store scrub keeps no
// trace of marker, for the text and for it as JSON in a string.
func r18Withheld(t *testing.T, text, marker string) {
	t.Helper()
	if !TextNamesSecret(text) {
		t.Errorf("TextNamesSecret(%q) = false, want true", text)
	}
	for _, row := range r18Rows(t, text) {
		if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(marker)) {
			t.Errorf("store kept %q: %s", text, out)
		}
	}
}

func r18Kept(t *testing.T, text string) {
	t.Helper()
	if TextNamesSecret(text) {
		t.Errorf("TextNamesSecret(%q) = true, want an ordinary string", text)
	}
}

// DNSFilter's API key travels as "Authorization: KEY", so authorization (and
// Proxy-Authorization) is a secret name for keys and text labels alike. A key
// that merely mentions authorization (authorized_domains, authorization_url)
// is not, and the CLI's own messages about the Authorization header stay
// readable.
func TestAuthorizationIsASecretName(t *testing.T) {
	const m = "R19AUTH01"
	for _, text := range []string{
		"Authorization: " + m,
		"Tested with curl -H 'Authorization: " + m + "' https://api.dnsfilter.com/v1/current_user",
		"Authorization=" + m,
		`-H "Authorization:` + m + `"`,
		"Authorization: Bearer " + m,
		"Proxy-Authorization: Basic " + m,
		"HTTP_AUTHORIZATION=" + m,
		`{"Authorization":"` + m + `"} sent`,
		"Authorization:\n  " + m,
	} {
		r18Withheld(t, text, m)
	}
	for _, key := range []string{"Authorization", "authorization", "Proxy-Authorization", "proxy_authorization", "HTTP_AUTHORIZATION", "x-authorization"} {
		if !IsSecretKey(key) || !KeyNamesSecret(key) {
			t.Errorf("%q is not a secret key", key)
		}
	}
	row := json.RawMessage(`{"id":"1","headers":{"Authorization":"` + m + `","Accept":"application/json"}}`)
	if out := ScrubPersistedSecrets(row); bytes.Contains(out, []byte(m)) || !bytes.Contains(out, []byte("application/json")) {
		t.Errorf("an Authorization key was not dropped alone: %s", out)
	}
	// Adding the label never lifts a pair the earlier vocabulary withheld: a
	// placeholder after it (<token>) still arms the text that follows, as it
	// did when Authorization was not a label (the scan's first pass).
	for _, text := range []string{
		`4821Authorization: <token>"string": falsehostname<value>`,
		"Authorization: <token> laptops: 4821",
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false: the new label lifted a pair the earlier rule withheld", text)
		}
	}
	for _, key := range []string{"authorized_domains", "authorization_url", "authorization_status", "auth0_sub", "include_auth_providers"} {
		if IsSecretKey(key) {
			t.Errorf("IsSecretKey(%q) = true, want false", key)
		}
	}
	for _, text := range []string{
		`warning: the key starts with "Bearer "; DNSFilter expects the bare API key in the Authorization header and the CLI sends it as-is. Save it again without the "Bearer " prefix.`,
		`WARN configured, but the key starts with "Bearer ": DNSFilter expects the bare API key in the Authorization header and the CLI sends it as-is; save it again without the prefix`,
		`warning: ignoring the Authorization entry under [headers] in config.toml: requests carry the API key saved by auth set-token or set in DNSFILTER_API_KEY. Remove that entry.`,
		"Authorization: <token>", `-H "Authorization: <value>"`, "Authorization:", `{"Authorization":null}`,
	} {
		r18Kept(t, text)
	}
}

// After a secret-named flag any next token is its value (fail closed), a
// value that starts with '-' included; only a long flag or the end of the
// line carries nothing.
func TestFlagValueStartingWithADashIsAValue(t *testing.T) {
	const m = "R18LEAK"
	for _, text := range []string{
		"deploy --password -" + m,
		"agent.exe --password -" + m,
		"run -token -" + m + " --verbose",
		"--pin -" + m,
		"x -v --password -" + m,
		"--secret_key\t-" + m,
	} {
		r18Withheld(t, text, m)
	}
	for _, text := range []string{
		"--password --verbose", "--password", "agent.exe --password\n-v", "--password -", "--token <value>",
		"sync --full --resources networks", "--name -HQ", "agent.exe --nkey --quiet",
	} {
		if text == "--password -" {
			// a lone dash (stdin) is a value: fail closed
			if !TextNamesSecret(text) {
				t.Errorf("TextNamesSecret(%q) = false, want true (a lone dash is a value)", text)
			}
			continue
		}
		r18Kept(t, text)
	}
}

// XML CDATA, name/value attribute pairs and fullwidth separators are the same
// pairs in other encodings; each is withheld, and their empty or ordinary
// forms are kept.
func TestXMLCDATAAttributesAndFullwidthSeparators(t *testing.T) {
	const m = "R19SENT01"
	for _, text := range []string{
		"<password><![CDATA[" + m + "]]></password>",
		"<secret_key><![CDATA[" + m + "]]></secret_key>",
		"Agent profile: <key>NKEY</key><string><![CDATA[" + m + "]]></string>",
		"<NKEY><![CDATA[" + m + "]]></NKEY>",
		"<password>\n  <![CDATA[" + m + "]]>\n</password>",
		"<password><![CDATA[<" + m + ">]]></password>",
		"<password><![cdata[" + m,
		"<![CDATA[<password>" + m + "</password>]]>",
		`<Property Id="NKEY" Value="` + m + `" />`,
		`<add key="NKEY" value="` + m + `" />`,
		`<add value='` + m + `' key='password'/>`,
		`<param name="Uninstall PIN" value=` + m + `>`,
		`<input type="password" name="pw" value="` + m + `">`,
		`<entry key="db.password">` + m + `</entry>`,
		`<field name="Uninstall PIN">` + m + `</field>`,
		`<meta name="csrf-token" content="` + m + `">`,
		`<entry key="secret_key"><![CDATA[` + m + `]]></entry>`,
		`<add key="NKEY" value="` + m,
		"PIN：" + m, "secret_key＝" + m, "Password：" + m,
		"卸载 PIN：" + m, "Uninstall PIN﹕" + m, "token∶" + m, "API Key ﹦ " + m,
		"ＰＩＮ：" + m, "PIN（HQ）：" + m,
		"token&#58; " + m, "token&#x3A;" + m, "Uninstall PIN&colon; " + m, "secret_key&#61;" + m,
		"&lt;password&gt;" + m + "&lt;/password&gt;", `&lt;add key=&quot;NKEY&quot; value=&quot;` + m + `&quot;/&gt;`,
	} {
		r18Withheld(t, text, m)
	}
	for _, text := range []string{
		"<password><![CDATA[]]></password>", `<add key="NKEY" value="" />`, `<add key="theme" value="dark"/>`,
		`<input type="password" value="">`, `<input name="password"/> Remember me`, `<input name="password"> Remember me`,
		`<label for="password">Password</label>`, `<add key="token_type" value="Bearer"/>`, "PIN：",
		`<meta name="description" content="Reset your password">`, `<meta name="csrf-token" content="">`,
		"Tom &amp; Jerry: friends", "token&#58;", "A &#92; B: c", "&#999999; password&#x;",
		`<label id="pin">Enter PIN</label>`, `<span id="token">Token expires soon</span>`, `<option value="token">API token</option>`,
		"比率 1：2", "Password Policy： strict", "<![CDATA[plain text]]>",
	} {
		r18Kept(t, text)
	}
}

// Past the decode depth bound a string that still holds '{' or '[' is
// withheld: JSON embedded after prose ('prefix {"p a s s w o r d":"x"}') was
// never decoded there, so its keys were never judged.
func TestDepthBoundFailsClosedAfterProse(t *testing.T) {
	const m = "R18DEPTH"
	if !KeyNamesSecret("p a s s w o r d") || TextNamesSecret(`prefix {"p a s s w o r d":"`+m+`"}`) {
		t.Fatal("the fixture must be a key only the key rule reads")
	}
	for _, levels := range []int{maxNestedJSONDepth, maxNestedJSONDepth + 1, maxNestedJSONDepth + 3} {
		s := `prefix {"p a s s w o r d":"` + m + `"}`
		for i := 0; i < levels; i++ {
			b, err := json.Marshal([]string{s})
			if err != nil {
				t.Fatal(err)
			}
			s = string(b)
		}
		b, _ := json.Marshal(s)
		row := `{"id":"1","note":` + string(b) + `}`
		if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(m)) {
			t.Fatalf("encoded %d times: the embedded key survived the depth bound: %.300s", levels, out)
		}
	}
	// Shallower, the embedded object is decoded and only its key is dropped.
	s := `prefix {"p a s s w o r d":"` + m + `","site":"HQ"}`
	for i := 0; i < 3; i++ {
		b, _ := json.Marshal([]string{s})
		s = string(b)
	}
	b, _ := json.Marshal(s)
	out := ScrubPersistedSecrets(json.RawMessage(`{"id":"1","note":` + string(b) + `}`))
	if bytes.Contains(out, []byte(m)) || !bytes.Contains(out, []byte("HQ")) {
		t.Fatalf("within the bound: %s", out)
	}
}

// An object cut off mid-record (its closing brace lost) gets the key rule on
// the quoted keys left in it, so a key only the key rule reads ("pass word")
// is judged as in the whole object; at any escape level, in a CSV cell too.
func TestCutOffObjectKeysGetTheKeyRule(t *testing.T) {
	const m = "Z9QX7R18"
	bs := string(rune(92))
	escaped := `{` + bs + `"pass word` + bs + `":` + bs + `"` + m + bs + `",` + bs + `"padding` + bs + `":` + bs + `"xxx`
	unicodeKey := `{"pass w` + bs + `u006Frd":"` + m + `","padding":"xx`
	if !strings.Contains(unicodeKey, bs+"u006F") || !strings.Contains(escaped, bs+`"pass word`) {
		t.Fatal("the escape fixtures lost their backslashes")
	}
	for _, text := range []string{
		`{"pass word":"` + m + `","padding":"` + strings.Repeat("x", 300),
		`error 422: {"id":7, "pass word": "` + m + `", "padding":"xx`,
		`{"ok":true,` + "\n" + `  "p a s s w o r d" : ` + m + `, "x`,
		`upstream said ` + escaped,
		unicodeKey,
		`"[{""pass word"":""` + m + `"",""padding"":""xx`,
	} {
		if !CutOffKeyNamesSecret(text) {
			t.Errorf("CutOffKeyNamesSecret(%q) = false, want true", text)
		}
		for _, row := range r18Rows(t, text) {
			if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(m)) {
				t.Errorf("store kept the cut-off key in %q: %s", text, out)
			}
		}
	}
	for _, text := range []string{
		`{"pass word":null,"padding":"xx`, `{"pass word":true,"pad`, `{"pass word":"","x`,
		`{"name":"HQ","padding":"xx`, `She said "pass word": nothing`, `{"note":"see the pass word: page`,
		`{"id":7,"secret_key_id":5,"x`,
	} {
		if CutOffKeyNamesSecret(text) {
			t.Errorf("CutOffKeyNamesSecret(%q) = true, want false", text)
		}
	}
}

// A stored CSV text whose cells hold JSON with doubled quotes is judged cell
// by cell as JSON: a long ordinary CSV is never withheld for its size, and a
// key only the key rule reads is dropped from its cell.
func TestCSVJSONCellsAreDecoded(t *testing.T) {
	const m = "R19CSVKEY"
	cell := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return `"` + strings.ReplaceAll(string(b), `"`, `""`) + `"`
	}
	var b strings.Builder
	b.WriteString("agents,count,org_id\n")
	for i := 0; i < 300; i++ {
		rec := map[string]any{"hostname": fmt.Sprintf("H%d", i), "last_sync": "2026-10-01T10:00:00Z", "tags": []string{"a"}}
		if i == 150 {
			rec["p a s s w o r d"] = m
		}
		fmt.Fprintf(&b, "%s,1,%d\n", cell([]any{rec}), i)
	}
	text := b.String()
	if strings.Count(text, "{") < maxFailedSegmentDecodes {
		t.Fatal("the fixture does not exceed the decode budget")
	}
	doc, _ := json.Marshal(map[string]string{"id": "1", "csv": text})
	out := string(ScrubPersistedSecrets(doc))
	if strings.Contains(out, m) {
		t.Fatalf("the key-rule-only key survived in its CSV cell")
	}
	if strings.Contains(out, WithheldValuePlaceholder) || !strings.Contains(out, "H299") || !strings.Contains(out, "H0") {
		t.Fatalf("an ordinary CSV was withheld or lost records: %.300s", out)
	}
	clean := strings.Replace(text, `,""p a s s w o r d"":""`+m+`""`, "", 1)
	if strings.Contains(clean, m) {
		t.Fatal("could not build the clean fixture")
	}
	doc, _ = json.Marshal(map[string]string{"id": "1", "csv": clean})
	if out := ScrubPersistedSecrets(doc); !bytes.Equal(out, doc) {
		t.Fatalf("a clean CSV of 300 records was changed: %.300s", out)
	}
	// Not a cell: a quote that does not open one, an unclosed cell.
	for _, tc := range []struct {
		text  string
		start int
	}{{`x"[1]"`, 2}, {`"[1]`, 1}, {`a ,"[1]"`, 3}} {
		if _, _, ok := CSVJSONCell(tc.text, tc.start); ok {
			t.Errorf("CSVJSONCell(%q, %d) = ok, want not a cell", tc.text, tc.start)
		}
	}
	if content, end, ok := CSVJSONCell(`a,"[""x""]",b`, 3); !ok || content != `["x"]` || end != 10 {
		t.Errorf("CSVJSONCell = %q, %d, %v", content, end, ok)
	}
}

// Every rule this round added stays linear: 1 MB of adversarial input for
// each finishes well within two seconds (the per-element closing-tag search
// took 16 s on 400 KB of <token>x).
func TestRound18RulesStayLinear(t *testing.T) {
	const size = 1 << 20
	rep := func(s string) string { return strings.Repeat(s, size/len(s)+1)[:size] }
	inputs := map[string]string{
		"placeholder elements":      rep("<token>x"),
		"value elements":            rep("<value>x "),
		"unclosed CDATA":            rep("<![CDATA[<x "),
		"CDATA sections":            rep("<p><![CDATA[a]]></p>"),
		"attribute tags":            rep(`<add key="NKEY" value="" `),
		"unclosed attribute quotes": rep(`<a b="`),
		"attribute pairs":           rep(`<add key="theme" value="dark"/>`),
		"flags":                     rep("--password --verbose "),
		"short flags":               rep("-x -y "),
		"cut-off keys":              rep(`{"pass word":null,`),
		"open quotes after braces":  rep(`{"`),
		"doubled quotes":            rep(`,""`),
		"fullwidth colons":          rep("PIN：："),
		"csv cells":                 rep(`,"[{""a"":1}]"`),
		"broken csv cells":          rep(`,"[""`),
		"glued labels":              rep(`""LAPTOP-SPIN1"",`),
	}
	for name, text := range inputs {
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			start := time.Now()
			TextNamesSecret(text)
			CutOffKeyNamesSecret(text)
			SecretPairInJSONText(text)
			doc, _ := json.Marshal(map[string]string{"v": text})
			ScrubPersistedSecrets(doc)
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("1 MB of %s took %v", name, d)
			}
		})
	}
}
