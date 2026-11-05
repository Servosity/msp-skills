// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Codex r1 (appdirect 4.34.2 reprint) classes, each test written to fail on the
// code before its fix:
//   - secret-in-encoded-output: a byte-escape or radix encoding the shared
//     funnel does not decode (quoted-printable "=41=42", \x41, 0x41 lists,
//     decimal, octal and binary byte lists, uuencode, xxencode, crypt and
//     bcrypt alphabets, Crockford and z-base-32, base36/58/62).
//   - credential-split-across-outbound-parts: a held value reassembled from
//     several parts of one outbound request (query values, path segments,
//     headers, body values), in any order.
//   - holder-incomplete: a credential under a standard auth name
//     (Authorization, Cookie, ...) or carried as a "Bearer x" value under any
//     name.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const encodedClassSecret = "ABCDEFGHabcdefgh12345678"

func perByte(s, format, sep string) string {
	parts := make([]string, 0, len(s))
	for i := 0; i < len(s); i++ {
		parts = append(parts, fmt.Sprintf(format, s[i]))
	}
	return strings.Join(parts, sep)
}

// sixBit encodes b with a 6-bit alphabet (uuencode, xxencode, crypt, bcrypt).
func sixBit(b []byte, alphabet string) string {
	var sb strings.Builder
	for i := 0; i < len(b); i += 3 {
		var g [3]byte
		copy(g[:], b[i:])
		n := uint32(g[0])<<16 | uint32(g[1])<<8 | uint32(g[2])
		for j := 18; j >= 0; j -= 6 {
			sb.WriteByte(alphabet[(n>>uint(j))&63])
		}
	}
	return sb.String()
}

func radix(b []byte, alphabet string) string {
	n := new(big.Int).SetBytes(b)
	base := big.NewInt(int64(len(alphabet)))
	var out []byte
	m := new(big.Int)
	for n.Sign() > 0 {
		n.DivMod(n, base, m)
		out = append(out, alphabet[m.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func fiveBit(b []byte, alphabet string) string {
	var sb strings.Builder
	var acc uint64
	bits := 0
	for _, c := range b {
		acc = acc<<8 | uint64(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(alphabet[(acc>>uint(bits))&31])
		}
	}
	if bits > 0 {
		sb.WriteByte(alphabet[(acc<<uint(5-bits))&31])
	}
	return sb.String()
}

const (
	uuAlphabet     = "`!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_"
	xxAlphabet     = "+-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	cryptAlphabet  = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	bcryptAlphabet = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	crockford32    = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	zBase32        = "ybndrfg8ejkmcpqxot1uwisza345h769"
	base58BTC      = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	base62Digits   = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	base36Digits   = "0123456789abcdefghijklmnopqrstuvwxyz"
)

func encodedClassForms(s string) map[string]string {
	b := []byte(s)
	return map[string]string{
		"quoted-printable":       perByte(s, "=%02X", ""),
		"quoted-printable-lower": perByte(s, "=%02x", ""),
		"quoted-printable-soft":  perByte(s[:12], "=%02X", "") + "=\r\n" + perByte(s[12:], "=%02X", ""),
		"backslash-x":            perByte(s, `\x%02x`, ""),
		"0x-comma":               perByte(s, "0x%02x", ","),
		"0x-comma-space":         perByte(s, "0x%02X", ", "),
		"hex-colon":              perByte(s, "%02x", ":"),
		"hex-space":              perByte(s, "%02X", " "),
		"hex-dash":               perByte(s, "%02x", "-"),
		"percent-u":              perByte(s, "%%u%04X", ""),
		"es6-unicode":            perByte(s, `\u{%x}`, ""),
		"html-hex":               perByte(s, "&#x%x;", ""),
		"html-decimal":           perByte(s, "&#%d;", ""),
		"decimal-comma":          perByte(s, "%d", ","),
		"decimal-space":          perByte(s, "%d", " "),
		"decimal-array":          "[" + perByte(s, "%d", ", ") + "]",
		"octal-escapes":          perByte(s, `\%03o`, ""),
		"octal-space":            perByte(s, "%03o", " "),
		"binary-space":           perByte(s, "%08b", " "),
		"binary-run":             perByte(s, "%08b", ""),
		"uuencode":               sixBit(b, uuAlphabet),
		"xxencode":               sixBit(b, xxAlphabet),
		"crypt64":                sixBit(b, cryptAlphabet),
		"bcrypt64":               sixBit(b, bcryptAlphabet),
		"crockford32":            fiveBit(b, crockford32),
		"z-base-32":              fiveBit(b, zBase32),
		"base58":                 radix(b, base58BTC),
		"base62":                 radix(b, base62Digits),
		"base36":                 radix(b, base36Digits),
	}
}

func TestEveryByteEncodingOfAHeldValueIsWithheld(t *testing.T) {
	FunnelHold(encodedClassSecret)
	for name, enc := range encodedClassForms(encodedClassSecret) {
		for _, text := range []string{enc, "feedback note: " + enc + " (end)", `{"message":"` + strings.ReplaceAll(strings.ReplaceAll(enc, `\`, `\\`), `"`, `\"`) + `"}`} {
			if !HeldEchoes(text) {
				t.Errorf("%s: HeldEchoes missed %q", name, text)
				continue
			}
			if out := WithholdText(text); strings.Contains(out, enc) {
				t.Errorf("%s: WithholdText kept the encoded secret: %q", name, out)
			}
			if out := string(WithholdBytes([]byte(text + "\n"))); strings.Contains(out, enc) {
				t.Errorf("%s: WithholdBytes kept the encoded secret: %q", name, out)
			}
		}
	}
	// Ordinary output with numbers, hex ids, dates and lists stays as it is.
	for _, text := range []string{
		`{"id":"0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9","quantity":5,"price":15.39,"createdDate":"2026-10-27T13:20:13.153-0700"}`,
		"Company\tInvoices\tTotal\nAcme\t3\t120.50\n",
		"[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]",
		"=20=3D=0A plain quoted-printable text that holds no secret",
		"mac 00:1a:2b:3c:4d:5e and ipv6 fe80::1ff:fe23:4567:890a",
	} {
		if HeldEchoes(text) || WithholdText(text) != text {
			t.Errorf("false positive on %q", text)
		}
	}
}

func outboundCounter(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestOutboundRefusesAHeldValueReassembledFromItsParts(t *testing.T) {
	const secret = "OutboundSplitSecret-7c41e9a2"
	FunnelHold(secret)
	srv, hits := outboundCounter(t)
	a, b := secret[:12], secret[12:]
	t1, t2, t3 := secret[:9], secret[9:18], secret[18:]
	cases := []struct {
		name   string
		rawURL string
		header http.Header
		body   string
	}{
		{"two query values", srv.URL + "/?a=" + a + "&b=" + b, nil, ""},
		{"two query values reversed", srv.URL + "/?b=" + b + "&a=" + a, nil, ""},
		{"query values around another", srv.URL + "/?a=" + a + "&x=unrelated&b=" + b, nil, ""},
		{"three query values shuffled", srv.URL + "/?c=" + t3 + "&a=" + t1 + "&b=" + t2, nil, ""},
		{"percent-encoded halves", srv.URL + "/?a=" + url.QueryEscape(a) + "&b=" + url.PathEscape(b), nil, ""},
		{"path segments", srv.URL + "/" + a + "/x/" + b, nil, ""},
		{"query and header", srv.URL + "/?a=" + a, http.Header{"X-Note": {b}}, ""},
		{"two headers", srv.URL + "/", http.Header{"X-One": {b}, "X-Two": {a}}, ""},
		{"url and body", srv.URL + "/?a=" + a, nil, `{"note":"` + b + `"}`},
		{"fragment and query", srv.URL + "/?q=" + b + "#" + a, nil, ""},
	}
	for _, c := range cases {
		resp, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, c.rawURL, c.header, []byte(c.body), 5*time.Second)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, ErrOutboundHoldsSecret) {
			t.Errorf("%s: sent (err %v)", c.name, err)
		}
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("the receiver got %d request(s) carrying the split secret", n)
	}
	// Control: an unrelated request is sent.
	resp, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, srv.URL+"/hook?a=one&b=two", http.Header{"X-Note": {"ok"}}, []byte(`{"note":"fine"}`), 5*time.Second)
	if err != nil {
		t.Fatalf("control request refused: %v", err)
	}
	_ = resp.Body.Close()
	if atomic.LoadInt32(hits) != 1 {
		t.Fatalf("control request did not arrive")
	}
}

func TestStandardAuthNamesAreCredentialNames(t *testing.T) {
	for _, name := range []string{"Authorization", "authorization", "Proxy-Authorization", "WWW-Authenticate", "Cookie", "Set-Cookie",
		"X-Auth-Token", "x-auth", "auth", "sessionId", "session_key", "session", "bearer", "jwt", "id_token", "signature", "sig",
		"client_assertion", "SAMLResponse", "otp", "x-api-key", "api_key", "private_key", "clientSecret"} {
		if !SecretFieldName(name) {
			t.Errorf("%q is not read as a credential name", name)
		}
	}
	for _, name := range []string{"name", "id", "companyId", "productId", "status", "authorName", "price", "description", "website"} {
		if SecretFieldName(name) {
			t.Errorf("%q is read as a credential name", name)
		}
	}
}

func TestResponseCredentialUnderAnyAuthNameIsHeld(t *testing.T) {
	for i, body := range []string{
		`{"id":"00000000-0000-4000-8000-000000000001","authorization":"Bearer RespAuthCredential-739182","name":"RespAuthCredential-739182"}`,
		`{"id":"x","Cookie":"sid=RespCookieCredential-55a1","name":"RespCookieCredential-55a1"}`,
		`{"id":"x","note":"Bearer RespNoteCredential-8b2f0c","name":"RespNoteCredential-8b2f0c"}`,
		`{"id":"x","detail":{"header":"Basic UmVzcEJhc2ljQ3JlZDpwdzEyMzQ1Njc4"},"name":"UmVzcEJhc2ljQ3JlZDpwdzEyMzQ1Njc4"}`,
	} {
		HoldCredentialFields([]byte(body))
		secret := []string{"RespAuthCredential-739182", "RespCookieCredential-55a1", "RespNoteCredential-8b2f0c", "UmVzcEJhc2ljQ3JlZDpwdzEyMzQ1Njc4"}[i]
		if out := WithholdText(`{"name":"` + secret + `"}`); strings.Contains(out, secret) {
			t.Errorf("case %d: the credential printed under another name: %s", i, out)
		}
		if out := WithholdText(body); strings.Contains(out, secret) {
			t.Errorf("case %d: the credential printed in the response itself: %s", i, out)
		}
	}
}

// TestNoAppDirectSpecNameReadsAsACredential keeps the credential-name rule from
// redacting AppDirect data: of every property and parameter name in the Partner
// API document, only provisioning "key" reads as a credential.
func TestNoAppDirectSpecNameReadsAsACredential(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if err != nil {
		t.Skipf("spec.json not beside the module: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if props, ok := x["properties"].(map[string]any); ok {
				for k := range props {
					names[k] = true
				}
			}
			if params, ok := x["parameters"].([]any); ok {
				for _, p := range params {
					if m, ok := p.(map[string]any); ok {
						if n, ok := m["name"].(string); ok {
							names[n] = true
						}
					}
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	if len(names) < 50 {
		t.Fatalf("read only %d names from spec.json", len(names))
	}
	// The AppDirect API's own credential fields (user passwords, the client
	// pair and tokens of /oauth2/token, the 2FA secret and code).
	credentials := map[string]bool{"password": true, "client_id": true, "client_secret": true, "access_token": true,
		"refresh_token": true, "secret_key": true, "mfa_code": true}
	for n := range names {
		if SecretFieldName(n) != credentials[n] {
			t.Errorf("AppDirect field %q: credential name = %v, want %v", n, SecretFieldName(n), credentials[n])
		}
	}
}
