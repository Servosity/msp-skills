// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

const splitUnitSecret = "SplitUnitSecretABCD1234efgh5678"

func TestHeldEchoesFindsSplitValues(t *testing.T) {
	FunnelHold(splitUnitSecret)
	a, b := splitUnitSecret[:15], splitUnitSecret[15:]
	for name, text := range map[string]string{
		"across lines":         a + "\n" + b,
		"adjacent json values": `{"first":"` + a + `","second":"` + b + `"}`,
		"array values":         `["` + a + `","` + b + `"]`,
		"escaped newline":      `{"msg":"` + a + `\n` + b + `"}`,
		"table cells":          "| " + a + " | " + b + " |",
		"whole":                "prefix " + splitUnitSecret,
	} {
		if !HeldEchoes(text) {
			t.Errorf("%s: %q not found", name, text)
		}
	}
	for _, text := range []string{a, b, `{"first":"` + a + `","x":"y"}`, "unrelated output"} {
		if HeldEchoes(text) {
			t.Errorf("false positive on %q", text)
		}
	}
}

func TestWithholdBytesSplitAcrossLinesWithholdsOnlyThoseLines(t *testing.T) {
	FunnelHold(splitUnitSecret)
	a, b := splitUnitSecret[:15], splitUnitSecret[15:]
	in := "header\n" + a + "\n" + b + "\nfooter\n"
	out := string(WithholdBytes([]byte(in)))
	if strings.Contains(out, a) && strings.Contains(out, b) {
		t.Fatalf("split value survived: %q", out)
	}
	if !strings.HasPrefix(out, "header\n") || !strings.HasSuffix(out, "footer\n") {
		t.Fatalf("unrelated lines were withheld: %q", out)
	}
}

func TestJSONStringValuesSkipsKeys(t *testing.T) {
	got := jsonStringValues(`noise {"k1":"v1","k2":{"k3":["a","b"],"k4":1},"k5":"v5"}`)
	if got != "v1abv5" {
		t.Fatalf("got %q", got)
	}
}

// Large compact JSON (--agent output of a big org) must pass unchanged
// when it repeats no held value: the funnel's work cap fails closed per call,
// so detection runs in windows small enough to stay under it. A held value
// anywhere in it, including across a window boundary, is still found.
func TestLargeCompactJSONIsNotWithheldForItsSize(t *testing.T) {
	const secret = "LargeOutputSecretK3y9Q7w2"
	FunnelHold(secret)
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; sb.Len() < 400<<10; i++ {
		fmt.Fprintf(&sb, `{"id":"%d","name":"Sensor%dPingHTTPDiskFree","usage":"Extraqueryparamtoinjectintoflatlistsyncrequestsrepeatablekeyvalue%x","status":"DOWN"},`, i, i, i*7919)
	}
	text := strings.TrimSuffix(sb.String(), ",") + "]"
	if HeldEchoes(text) || WithholdText(text) != text {
		t.Fatal("a large compact JSON output that repeats no held value was withheld")
	}
	for _, at := range []int{len(text) / 3, 16<<10 - 10, 32<<10 - 5} {
		with := text[:at] + secret + text[at:]
		if !HeldEchoes(with) {
			t.Fatalf("held value at offset %d not found", at)
		}
	}
}

// A credential-named member's value is redacted and held, so the same value
// under another name in the same output is withheld too (a username that is
// also the password).
func TestRedactedMemberValueIsWithheldElsewhere(t *testing.T) {
	const pw = "MemberHeldPw-5e1a9c"
	out := WithholdText(`{"password":"` + pw + `","username":"` + pw + `","note":"ok"}`)
	if strings.Contains(out, pw) || !strings.Contains(out, `"note":"ok"`) {
		t.Fatalf("got %s", out)
	}
}

// Class undecoded-encoding-gap (Codex review r1): the shared funnel does not
// decode base32, base32hex, ASCII85 or Z85, so those encodings of every held
// value must be matched as needles. Every held value of heldMatchFloor or more
// characters is matched in each of them, whole and inside a longer encoded
// stream at every byte alignment (any prefix and suffix), upper and lower case
// for base32. The reported input: an 8-character password whose whole base32
// form (no aligned 10-byte core exists) printed unchanged.
func TestUndecodedEncodingsMatchAtEveryLengthAndAlignment(t *testing.T) {
	type encoder struct {
		name string
		enc  func([]byte) string
		unit int // the stream is padded to a multiple of unit bytes (Z85 needs whole groups)
	}
	a85 := func(b []byte) string {
		buf := make([]byte, ascii85.MaxEncodedLen(len(b)))
		return string(buf[:ascii85.Encode(buf, b)])
	}
	encoders := []encoder{
		{"base32", base32.StdEncoding.EncodeToString, 1},
		{"base32-nopad", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString, 1},
		{"base32-lower", func(b []byte) string { return strings.ToLower(base32.StdEncoding.EncodeToString(b)) }, 1},
		{"base32hex", base32.HexEncoding.EncodeToString, 1},
		{"base32hex-lower", func(b []byte) string { return strings.ToLower(base32.HexEncoding.EncodeToString(b)) }, 1},
		{"ascii85", a85, 1},
		{"z85", z85, 4},
	}
	for n := heldMatchFloor; n <= 40; n++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("undecoded-gap-%d", n)))
		v := base64.RawStdEncoding.EncodeToString(sum[:])[:n]
		FunnelHold(v)
		for _, e := range encoders {
			for pre := 0; pre < 5; pre++ {
				for suf := 0; suf < 5; suf++ {
					stream := append(append([]byte("\x01\x02\x03\x04"[:min(pre, 4)]+strings.Repeat("\x05", max(pre-4, 0))), v...), "\x06\x07\x08\x09"[:min(suf, 4)]...)
					for len(stream)%e.unit != 0 {
						stream = append(stream, '\x0a')
					}
					text := e.enc(stream)
					if !HeldEchoes(text) || strings.Contains(WithholdText(`{"name":"`+text+`"}`), text) {
						t.Errorf("%d-character value as %s with %d byte(s) before and %d after: %q not withheld", n, e.name, pre, suf, text)
					}
				}
			}
		}
	}
}

// Codex r1 (4.34.2 reprint) class secret-in-encoded-output: a value held with
// an auth scheme in front of it (a config file's [headers] Authorization =
// "Bearer x") is matched by the credential alone in every encoding, not only
// with the scheme word, as the funnel's own needles already do for base64.
func TestSchemePrefixedHeldValueMatchesBareInEveryEncoding(t *testing.T) {
	const bare = "GraphHeaderToken-5e7a2c9" // 24 bytes: whole Z85 groups
	FunnelHold("Bearer " + bare)
	a85 := func(b []byte) string {
		buf := make([]byte, ascii85.MaxEncodedLen(len(b)))
		return string(buf[:ascii85.Encode(buf, b)])
	}
	for name, text := range map[string]string{
		"plain":     bare,
		"base32":    base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(bare)),
		"base32hex": base32.HexEncoding.EncodeToString([]byte(bare)),
		"ascii85":   a85([]byte(bare)),
		"z85":       z85([]byte(bare)),
	} {
		if !HeldEchoes(text) || strings.Contains(WithholdText(`{"value":[{"id":"u1","displayName":"`+text+`"}]}`), text) {
			t.Errorf("%s of the bare credential printed unchanged: %q", name, text)
		}
	}
}
