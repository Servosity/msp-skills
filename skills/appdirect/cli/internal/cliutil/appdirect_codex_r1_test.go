// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Codex review round 1 (appdirect 4.34.2 reprint, review repo ce4623f), each
// test written to fail on the code before its fix:
//   - holder-incomplete: a part of a credential below a length floor (a cookie
//     or parameter-list value), a credential
//     written as a JSON number, and a credential carried inside a URL or a
//     parameter list under a name that is not a credential were not held.
//   - secret-in-encoded-output: a fast pre-check skipped the byte-escape
//     decoder for hex written with letters ("7a:7a:7a:7a"); byte lists were
//     read only with an enumerated set of separators; a held value shorter
//     than eight characters had no needle in the 5- and 6-bit alphabets and
//     the radix encodings; and a numeric credential was matched only as the
//     digits it was written with.

import (
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func assertWithheld(t *testing.T, name, text, leak string) {
	t.Helper()
	if !HeldEchoes(text) {
		t.Errorf("%s: HeldEchoes missed %q", name, text)
	}
	if out := WithholdText(text); strings.Contains(out, leak) {
		t.Errorf("%s: WithholdText kept %q: %q", name, leak, out)
	}
	if out := string(WithholdBytes([]byte(text + "\n"))); strings.Contains(out, leak) {
		t.Errorf("%s: WithholdBytes kept %q: %q", name, leak, out)
	}
}

// Codex r1 P1 holder-incomplete (appdirect_secrets.go:202): a response
// carries {"cookie":"sid=pQ7","name":"pQ7"}; the cookie member is redacted
// and its 3-character value printed under "name". The class: every part of
// a credential is held at any length, a number is a credential under a
// credential name, and a credential inside a URL or a parameter list is one
// under any name.
func TestR1HolderHoldsEveryCredentialPart(t *testing.T) {
	for _, c := range []struct{ name, body, leak string }{
		{"cookie value below four characters", `{"cookie":"sid=pQ7","name":"pQ7"}`, "pQ7"},
		{"parameter-list value below four characters", `{"credentials":"a=Xy4&b=PubParam-81c3","label":"Xy4"}`, "Xy4"},
		{"numeric credential", `{"otp":736251,"name":"736251"}`, "736251"},
		{"numeric PIN", `{"pin":48217,"ref":"48217"}`, "48217"},
		{"passcode", `{"passcode":"Pc-77a1e","label":"Pc-77a1e"}`, "Pc-77a1e"},
		{"number inside a credential object", `{"credentials":{"code":40917},"ref":"40917"}`, "40917"},
		{"URL credential under any name", `{"redirectUrl":"https://cb.example.net/x?access_token=Zq81Tok","note":"Zq81Tok"}`, "Zq81Tok"},
		{"parameter list under any name", `{"data":"access_token=Pr2mTok&expires_in=3600","note":"Pr2mTok"}`, "Pr2mTok"},
	} {
		HoldCredentialFields([]byte(c.body))
		assertWithheld(t, c.name, c.body, c.leak)
	}
	// A measure under a credential-like name is not a credential: a count is
	// neither held nor redacted.
	measures := `{"tokenCount":5,"total_tokens":12,"passwordMinLength":10,"max_tokens":7}`
	HoldCredentialFields([]byte(measures))
	if out := WithholdText(measures); out != measures {
		t.Errorf("a measure was withheld: %q", out)
	}
	if HeldEchoes(`{"quantity":12,"seats":10}`) {
		t.Error("a count under a credential-like name was held")
	}
}

// Codex r1 P1 secret-in-encoded-output (appdirect_encodings.go:170): with
// {"password":"zzzz"} held, {"name":"7a:7a:7a:7a"} printed the password as
// separated hex (no decimal digit for the eight-digit pre-check to count).
func TestR1ByteEscapesWrittenWithHexLetters(t *testing.T) {
	HoldCredentialFields([]byte(`{"password":"zzzz"}`))
	for _, text := range []string{`{"password":"zzzz","name":"7a:7a:7a:7a"}`, "7a:7a:7a:7a", "7A 7A 7A 7A", `\x7a\x7a\x7a\x7a`, "=7A=7A=7A=7A"} {
		assertWithheld(t, "hex letters "+text, text, strings.Trim(text, `{}"`))
	}
}

// The byte-list family is read whatever separates the bytes: any run of
// characters that are not digits, and the 0x, \x, U+, &#x introducers.
func TestR1ByteListsWithAnySeparator(t *testing.T) {
	for _, h := range []string{"Wq4mP9", "LongSepSecret-7f3a"} {
		FunnelHold(h)
		for _, sep := range []string{",    ", "  ,  ", " / ", "_", ";\t\t", " - ", "\n", " | ", "+"} {
			for _, f := range []string{"%d", "%02x", "%02X", "0x%02X", "%03o", "%08b", `\x%02x`, "&#%d;", "U+%04X"} {
				text := perByte(h, f, sep)
				assertWithheld(t, fmt.Sprintf("%q sep %q fmt %q", h, sep, f), "[ "+text+" ]", text)
			}
		}
	}
}

// A held value shorter than eight characters, alone and inside a longer
// stream at every byte alignment, in every alphabet the output chokepoint
// withholds a longer value in.
func TestR1ShortHeldValueInEveryAlphabet(t *testing.T) {
	const short = "Kv7q2"
	FunnelHold(short)
	enc := func(b []byte) map[string]string {
		out := map[string]string{
			"uuencode":    sixBit(b, uuAlphabet),
			"xxencode":    sixBit(b, xxAlphabet),
			"crypt64":     sixBit(b, cryptAlphabet),
			"bcrypt64":    sixBit(b, bcryptAlphabet),
			"crockford32": fiveBit(b, crockford32),
			"z-base-32":   fiveBit(b, zBase32),
			"base58":      radix(b, base58BTC),
			"base62":      radix(b, base62Digits),
			"base36":      radix(b, base36Digits),
		}
		out["base32"] = base32.StdEncoding.EncodeToString(b)
		out["base32-lower"] = strings.ToLower(base32.StdEncoding.EncodeToString(b))
		out["base32hex-lower"] = strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
		out["base64"] = base64.StdEncoding.EncodeToString(b)
		out["base64url"] = base64.RawURLEncoding.EncodeToString(b)
		out["hex-mixed"] = mixedCase(hex.EncodeToString(b))
		a85 := make([]byte, ascii85.MaxEncodedLen(len(b)))
		out["ascii85"] = string(a85[:ascii85.Encode(a85, b)])
		return out
	}
	for name, form := range enc([]byte(short)) {
		assertWithheld(t, "alone "+name, "value: "+form+" (end)", form)
	}
	// Inside a longer stream: the printed text starts anywhere in the
	// encoding (k characters cut from its front), so the value sits at any
	// bit alignment of what is shown.
	for name, form := range enc([]byte("tuvwxyzs" + short + "Q")) {
		if strings.HasPrefix(name, "base36") || strings.HasPrefix(name, "base58") || strings.HasPrefix(name, "base62") {
			continue // a radix encoding is of the whole number: no alignment inside it
		}
		for k := 0; k <= 7 && k < len(form); k++ {
			shown := form[k:]
			assertWithheld(t, fmt.Sprintf("inside a stream, %d characters cut, %s", k, name), "value: "+shown+" (end)", shown)
		}
	}
}

// A numeric credential is matched as the number it is, however it is written.
func TestR1NumericCredentialInEveryNumberForm(t *testing.T) {
	HoldCredentialFields([]byte(`{"otp":736251}`))
	n := big.NewInt(736251)
	for _, text := range []string{"7.36251e5", "7.36251E+05", "736251.0", "736251.000", "736,251", "736_251", "736'251",
		"0x" + n.Text(16), "0X" + strings.ToUpper(n.Text(16)), "0o" + n.Text(8), "0b" + n.Text(2), "+736251", "0736251"} {
		assertWithheld(t, "number "+text, `{"n":"`+text+`"}`, text)
	}
	// Other numbers stay as they are.
	for _, text := range []string{`{"total":736252,"ratio":7.36252e5,"id":"0x2b3bfb","year":2026}`, "736 apples and 251 pears"} {
		if WithholdText(text) != text {
			t.Errorf("false positive on %q: %q", text, WithholdText(text))
		}
	}
	_ = strconv.Itoa
}

func mixedCase(s string) string {
	b := []byte(s)
	for i := range b {
		if i%2 == 0 && 'a' <= b[i] && b[i] <= 'f' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

// The holder reads a body whatever its encoding: AppDirect documents XML
// responses beside JSON (spec.yaml: application/xml), and a token endpoint or
// proxy can answer form-encoded or plain text. A credential under a
// credential name in markup, or in a parameter list or "Name: value" line of
// a text body, is held like one in JSON.
func TestR1HolderReadsEveryBodyEncoding(t *testing.T) {
	for _, c := range []struct{ name, body, out, leak string }{
		{"XML element", `<user><email>a@b.example</email><password>XmlPw-4c1e</password></user>`, `{"note":"XmlPw-4c1e"}`, "XmlPw-4c1e"},
		{"XML CDATA element", `<creds><clientSecret><![CDATA[CdataSec-7a2f]]></clientSecret></creds>`, "seen CdataSec-7a2f", "CdataSec-7a2f"},
		{"XML attribute", `<app id="9" accessToken = 'AttrTok-3d9b'/>`, "AttrTok-3d9b", "AttrTok-3d9b"},
		{"form-encoded body", `grant_type=client_credentials&access_token=FormTok-8e1c&expires_in=3600`, "FormTok-8e1c", "FormTok-8e1c"},
		{"plain-text header dump", "HTTP/1.1 200 OK\nX-Api-Key: TextKey-2b6d\nContent-Type: text/plain\n", "key TextKey-2b6d", "TextKey-2b6d"},
	} {
		HoldCredentialFields([]byte(c.body))
		assertWithheld(t, c.name, c.out, c.leak)
	}
	// Markup that names no credential holds nothing.
	HoldCredentialFields([]byte(`<company><name>Plain Markup Co</name><website>https://plain.example</website></company>`))
	if HeldEchoes("Plain Markup Co") {
		t.Error("a markup value under a name that is no credential was held")
	}
}
