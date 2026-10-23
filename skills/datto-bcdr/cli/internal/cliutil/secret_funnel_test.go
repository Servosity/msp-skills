// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for the shared secret-echo output funnel
// (secret_funnel.go). Ships byte-identical with it.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"strings"
	"testing"
	"time"
)

// bs is one backslash, built at run time so no fixture loses an escape on its
// way into the source.
var bs = string(rune(92))

func funnelTestSecrets() []string {
	return []string{
		"clsSENTapikey.0a1+2b3/4c5=6d7e8f", // transport-sensitive characters
		"pct%41amp&amp;lt<gt>q\"x'9z",      // decodable text inside the secret itself
		"s3cr3t-Value_With:Colon~!",        // ':' and '~'
	}
}

func jsonU(v string) string {
	var b strings.Builder
	for _, r := range v {
		fmt.Fprintf(&b, "%su%04x", bs, r)
	}
	return b.String()
}

func jsonX(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		fmt.Fprintf(&b, "%sx%02x", bs, v[i])
	}
	return b.String()
}

// lowerEscapes lowercases only the hex digits of each %XX escape.
func lowerEscapes(v string) string {
	b := []byte(v)
	for i := 0; i+2 < len(b); i++ {
		if b[i] == '%' {
			b[i+1] = strings.ToLower(string(b[i+1]))[0]
			b[i+2] = strings.ToLower(string(b[i+2]))[0]
		}
	}
	return string(b)
}

func pctEvery(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		fmt.Fprintf(&b, "%%%02X", v[i])
	}
	return b.String()
}

func entities(v string, format string) string {
	var b strings.Builder
	for _, r := range v {
		fmt.Fprintf(&b, format, r)
	}
	return b.String()
}

// funnelEncodings is every way a server, a proxy or a log formatter can render
// one value. Each must be recognized.
func funnelEncodings(v string) map[string]string {
	b := []byte(v)
	q := url.QueryEscape(v)
	p := url.PathEscape(v)
	b64 := base64.StdEncoding.EncodeToString(b)
	return map[string]string{
		"raw":              v,
		"query":            q,
		"path":             p,
		"url-every-byte":   pctEvery(v),
		"url2":             url.QueryEscape(q),
		"url3":             url.QueryEscape(url.QueryEscape(q)),
		"url-lower-hex":    lowerEscapes(url.QueryEscape(url.QueryEscape(v))),
		"html":             html.EscapeString(v),
		"html-decimal":     entities(v, "&#%d;"),
		"html-hex":         entities(v, "&#x%X;"),
		"html-of-url":      html.EscapeString(q),
		"json-u":           jsonU(v),
		"json-slash":       strings.ReplaceAll(v, "/", bs+"/"),
		"json-x":           jsonX(v),
		"json-u-brace":     entities(v, bs+"u{%x}"),
		"double-json":      strings.ReplaceAll(jsonU(v), bs, bs+bs),
		"base64":           b64,
		"base64-raw":       base64.RawStdEncoding.EncodeToString(b),
		"base64url":        base64.URLEncoding.EncodeToString(b),
		"base64url-raw":    base64.RawURLEncoding.EncodeToString(b),
		"base64-offset1":   base64.StdEncoding.EncodeToString(append([]byte("x"), b...)),
		"base64-offset2":   base64.StdEncoding.EncodeToString(append([]byte("xy"), b...)),
		"base64-suffix":    base64.StdEncoding.EncodeToString(append(append([]byte{}, b...), []byte(" tail")...)),
		"base64-glued":     "token=" + b64,
		"base64-dotglued":  "." + base64.StdEncoding.EncodeToString(append([]byte("zz"), b...)),
		"base64-glued-1":   "a" + b64,
		"base64-glued-2":   "ab" + b64,
		"base64-glued-3":   "rq-" + b64,
		"base64-of-url":    base64.StdEncoding.EncodeToString([]byte(q)),
		"url-of-base64":    url.QueryEscape(b64),
		"base64-of-base64": base64.StdEncoding.EncodeToString([]byte(b64)),
		"base64-mime-wrap": b64[:10] + "\r\n" + b64[10:],
		"base64-json-wrap": b64[:10] + bs + "n" + b64[10:],
		"hex":              hex.EncodeToString(b),
		"hex-upper":        strings.ToUpper(hex.EncodeToString(b)),
		"hex-offset":       "f" + hex.EncodeToString(b),
		"control-split":    v[:5] + "\x00" + v[5:],
		"format-split":     v[:5] + "​" + v[5:],
		"tag-split":        v[:5] + "<b>" + v[5:],
		"url2-in-tag":      "<a href=\"" + url.QueryEscape(q) + "\">x</a>",
		"entity-of-pct":    strings.ReplaceAll(q, "%", "&#37;"),
	}
}

func funnelShapes(enc string) map[string]string {
	return map[string]string{
		"json":  `{"error":"bad request","detail":"` + enc + `"}`,
		"html":  `<html><head><title>Error ` + enc + `</title></head><body>no</body></html>`,
		"plain": "GET /v2/devices returned HTTP 403: " + enc + " (request rejected)",
		"start": enc + " was rejected",
	}
}

func TestFunnelRecognizesEveryEncodingOfAHeldSecret(t *testing.T) {
	secrets := funnelTestSecrets()
	cases := 0
	for _, secret := range secrets {
		for encName, enc := range funnelEncodings(secret) {
			for shapeName, text := range funnelShapes(enc) {
				cases++
				if !FunnelEchoes(text, secrets) {
					t.Errorf("secret %q encoded %s in a %s answer was not recognized: %q", secret, encName, shapeName, text)
				}
				if got := FunnelWithhold(text, secrets); !FunnelWithheld(got) {
					t.Errorf("secret %q encoded %s in a %s answer was not withheld", secret, encName, shapeName)
				}
			}
		}
	}
	if cases < 300 {
		t.Fatalf("oracle shrank to %d cases", cases)
	}
}

func TestFunnelRecognizesThePartsOfAHeldBasicHeader(t *testing.T) {
	user, pass := "api-key-as-username-123", "p4ssw0rd-long-enough"
	pair := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	for _, held := range []string{"Basic " + pair, pair} {
		for _, echo := range []string{pass, user, base64.StdEncoding.EncodeToString([]byte(pass)), url.QueryEscape(pass)} {
			if !FunnelEchoes(`{"error":"`+echo+`"}`, []string{held}) {
				t.Errorf("held %q: echo %q of one half of the pair was not recognized", held, echo)
			}
		}
	}
}

func TestFunnelRecognizesTheTokenAfterASchemeWord(t *testing.T) {
	tok := "eyJhbGciOiJIUzI1NiJ9.payload-part.sig-part"
	if !FunnelEchoes("invalid "+base64.StdEncoding.EncodeToString([]byte(tok)), []string{"Bearer " + tok}) {
		t.Error("the bearer token's base64 echo was not recognized when the held value carries the scheme word")
	}
}

func TestFunnelLeavesCleanTextAlone(t *testing.T) {
	secrets := funnelTestSecrets()
	secret := secrets[0]
	clean := []string{
		`{"data":[{"id":1,"name":"device-a"}],"next":null}`,
		`{"error":"HTTP 403","body":"token ****` + secret[len(secret)-4:] + ` rejected"}`,
		"GET /v2/devices/2 returned HTTP 403: " + FunnelWithheldText,
		base64.StdEncoding.EncodeToString([]byte(strings.Repeat("unrelated binary payload ", 40))),
		hex.EncodeToString([]byte("an unrelated hex dump of some bytes")),
		"<html><head><title>Service Unavailable</title></head></html>",
		secret[:len(secret)/2] + " ... only half of it",
	}
	for _, text := range clean {
		if FunnelEchoes(text, secrets) {
			t.Errorf("clean text was flagged: %q", text)
		}
		if got := FunnelWithhold(text, secrets); got != text {
			t.Errorf("clean text was changed: %q -> %q", text, got)
		}
	}
}

func TestFunnelIgnoresShortAndEmptySecrets(t *testing.T) {
	if FunnelEchoes("the id is abc123 and x", []string{"", "   ", "abc123"}) {
		t.Error("a value under the minimum length matched decoded text")
	}
	if FunnelEchoes("", funnelTestSecrets()) {
		t.Error("empty text matched")
	}
}

func TestFunnelFailsClosed(t *testing.T) {
	secrets := funnelTestSecrets()
	if !FunnelEchoes(strings.Repeat("a", funnelMaxInput+1), secrets) {
		t.Error("text over the input cap was not withheld")
	}
	// Past the work cap the funnel withholds, even for clean text: lower the
	// cap and feed a clean text that needs several views.
	perByte, base := funnelWorkPerByte, funnelWorkBase
	defer func() { funnelWorkPerByte, funnelWorkBase = perByte, base }()
	funnelWorkPerByte, funnelWorkBase = 2, 0
	if !FunnelEchoes("<b>clean &amp; plain</b> %41%42 "+bs+"q and nothing held here", secrets) {
		t.Error("work past the cap was not withheld")
	}
}

// TestFunnelWorkCapLetsRealErrorBodiesThrough is the other direction of the
// work cap: ordinary large error answers that hold no secret are not withheld.
func TestFunnelWorkCapLetsRealErrorBodiesThrough(t *testing.T) {
	secrets := funnelTestSecrets()
	var page strings.Builder
	page.WriteString("<!DOCTYPE html>\n<html><head><title>Attention Required! | Example</title>\n<style>body{width:100%;margin:0}</style>\n<script>var a=\"x" + bs + "u0041\";</script></head>\n<body>\n")
	for page.Len() < 200000 {
		page.WriteString("<div class=\"row\">&nbsp;Ray ID: 8a1b2c3d4e5f6a7b &amp; more text &lt;here&gt; %2F path\n<a href=\"/cdn-cgi/l/email-protection#a1b2c3d4e5f6a7b8c9d0\">x</a></div>\n")
	}
	var js strings.Builder
	js.WriteString(`{"errors":[`)
	for js.Len() < funnelMaxInput-512 {
		js.WriteString(`{"code":"E1001","message":"Field ` + bs + `"name` + bs + `" is invalid` + bs + `nsee docs ` + bs + `u00e9","id":"` + base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(js.Len()))) + `"},`)
	}
	js.WriteString(`{}]}`)
	for name, text := range map[string]string{"200 KB HTML error page": page.String(), "1 MB JSON error body": js.String()} {
		if FunnelEchoes(text, secrets) {
			t.Errorf("a %s holding no secret was withheld (work cap too tight)", name)
		}
	}
}

// TestFunnelCodexRound2 holds the round-2 review's failing inputs.
func TestFunnelCodexRound2(t *testing.T) {
	// A held secret containing a literal backslash, echoed HTML-escaped: the
	// view that keeps the backslash must still be searched.
	backslashSecret := "ab" + bs + "q<def"
	if !FunnelEchoes("ab"+bs+"q&lt;def", []string{backslashSecret}) {
		t.Error("an HTML-escaped echo of a secret holding a backslash was not recognized")
	}
	// Eight base64 layers around a control character and the secret: the
	// deepest view is matched before any text-like or control filter.
	v := "\nabcdefgh"
	for i := 0; i < 8; i++ {
		v = base64.StdEncoding.EncodeToString([]byte(v))
	}
	if !FunnelEchoes(v, []string{"abcdefgh"}) {
		t.Error("8 base64 layers around a control character and the secret were not recognized")
	}
}

func TestFunnelHeldBackstop(t *testing.T) {
	held := "backstop-held-value-0a1+2b3/4c5="
	FunnelHold(held, "", "   ")
	echo := base64.StdEncoding.EncodeToString([]byte(held))
	if got := FunnelHeld("operation failed: " + echo); !FunnelWithheld(got) {
		t.Errorf("FunnelHeld did not withhold an echo of a held value: %q", got)
	}
	clean := "operation failed: HTTP 500 from the server"
	if got := FunnelHeld(clean); got != clean {
		t.Errorf("FunnelHeld changed clean text: %q", got)
	}
	// Values a funnel call is given are held from then on.
	other := "given-to-a-funnel-call-7x9y"
	_ = FunnelEchoes("unrelated", []string{other})
	if got := FunnelHeld("x " + url.QueryEscape(url.QueryEscape(other))); !FunnelWithheld(got) {
		t.Errorf("a value given to FunnelEchoes was not held afterwards: %q", got)
	}
}

func TestFunnelWithheldTextDoesNotLookLikeAnAuthError(t *testing.T) {
	// The words the generated auth-error classifier (LooksLikeAuthError) and
	// the MCP error classifier look for.
	lower := strings.ToLower(FunnelWithheldText)
	for _, word := range []string{"key", "token", "unauthorized", "forbidden", "authenticat", "credential", "api_key", "http 4"} {
		if strings.Contains(lower, word) {
			t.Errorf("the withheld notice contains %q, which an error classifier matches", word)
		}
	}
}

func TestFunnelFollowsDeepNesting(t *testing.T) {
	secrets := funnelTestSecrets()
	nested := secrets[0]
	for layer := 1; layer <= 12; layer++ {
		nested = base64.StdEncoding.EncodeToString([]byte(nested))
		if !FunnelEchoes("GET /v2/devices returned HTTP 400: "+nested, secrets) {
			t.Fatalf("%d nested base64 layers of a held secret were not recognized", layer)
		}
	}
}

func TestFunnelUndoesEveryJavaScriptEscape(t *testing.T) {
	secrets := funnelTestSecrets()
	v := secrets[0]
	for _, esc := range []string{"v", "0", "b", "f", "n", "r", "t"} {
		text := v[:5] + bs + esc + v[5:]
		if !FunnelEchoes(text, secrets) {
			t.Errorf("a secret split by the escape %q was not recognized: %q", bs+esc, text)
		}
	}
	// An identity escape (a backslash before an ordinary character); the
	// letters and digits that start a real escape are left as they are.
	var b strings.Builder
	for _, r := range v {
		if strings.ContainsRune("bfnrtvux0123456789", r) {
			b.WriteRune(r)
			continue
		}
		b.WriteString(bs + string(r))
	}
	if !FunnelEchoes(b.String(), secrets) {
		t.Errorf("a secret written with identity escapes was not recognized: %q", b.String())
	}
}

func TestFunnelStaysLinearOnMalformedEscapes(t *testing.T) {
	secrets := funnelTestSecrets()
	text := strings.Repeat(bs+"u{", 1<<18)
	done := make(chan bool, 1)
	go func() { done <- FunnelEchoes(text, secrets) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("decoding 768 KiB of malformed \\u{ escapes did not finish in 10 s")
	}
}

func TestFunnelKeepsTheAuthErrorReading(t *testing.T) {
	secrets := funnelTestSecrets()
	echo := base64.StdEncoding.EncodeToString([]byte(secrets[0]))
	authish := `{"error":"invalid token","echo":"` + echo + `"}`
	got := FunnelWithhold(authish, secrets)
	if got == authish || strings.Contains(got, echo) {
		t.Fatalf("the echo was not withheld: %q", got)
	}
	if !funnelReadsAsAuth(got) {
		t.Errorf("a withheld answer that read as an auth error no longer does: %q", got)
	}
	plain := `{"error":"bad request","echo":"` + echo + `"}`
	if got := FunnelWithhold(plain, secrets); got != FunnelWithheldText {
		t.Errorf("a withheld answer that did not read as an auth error gained the auth reading: %q", got)
	}
}
