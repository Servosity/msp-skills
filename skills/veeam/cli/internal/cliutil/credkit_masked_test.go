// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package cliutil

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"veeam-pp-cli/internal/credkit"
)

func TestIsCredentialMask(t *testing.T) {
	for _, v := range []string{
		"••••••", "******", "\uFFFD\uFFFD\uFFFD\uFFFD\uFFFD\uFFFD", "●●●", "•••", "------", "############",
	} {
		if !IsCredentialMask(v) {
			t.Errorf("IsCredentialMask(%q) = false, want true", v)
		}
	}
	for _, v := range []string{
		"",        // empty
		"•", "••", // shorter than 3
		"xxxxxx", "XXXXXXXX", // letters are no mask
		"000000", "11111111", // digits
		"ⅠⅠⅠ", "²²²", // other numbers
		"\u0301\u0301\u0301", // combining marks
		"•••*", "**••",       // more than one character
		"Password1", "Passw0rd!", "••••••x",
		"\xe2\x80\xa2\xe2\x80", // not valid UTF-8
		"\xff\xff\xff",         // bytes a decoder reads as U+FFFD
		"ááá",
	} {
		if IsCredentialMask(v) {
			t.Errorf("IsCredentialMask(%q) = true, want false", v)
		}
	}
}

// recordHolds swaps Hold for a recorder for one test.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var held []string
	prev := Hold
	Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { Hold = prev })
	return &held
}

func TestHoldMaskedResponseFieldsJSON(t *testing.T) {
	held := recordHolds(t)
	body := `{"data":[{"password":"••••••","rootPassword":"******","sshPrivateKey":"\ufffd\ufffd\ufffd",` +
		`"name":"Personal","passwordHint":"hint-Qv8LmZ2x"}],"meta":{"password":null,"rootPassword":true}}`
	if !HoldMaskedResponseFields([]byte(body), "application/json; charset=utf-8") {
		t.Fatal("a JSON body was withheld")
	}
	if len(*held) != 0 {
		t.Errorf("masks, nulls and other fields were held: %q", *held)
	}

	*held = nil
	body = `{"data":{"username":"u","password":"Password1",` +
		`"password":"dupQ7vLx2Rk9","PASSWORD":"upperK8wN3qZ6","p\u0061ssword":"escT5yH2mL9c",` +
		`"details":{"rootPassword":["rootJ3vB8nQ1",{"inner":"deepP9rL4tW7"}],"sshPrivateKey":{"pem":"sshM4kZ8qT2v"}},` +
		`"numbers":{"password":7391586420},"accent":{"password":"a\u00e9bcdefgh"},` +
		`"latin1":{"password":"` + "\xe4\xe4\xe4" + `"},"masked":{"password":"\u2022\u2022\u2022"}}}` + "\n" +
		`{"password":"lineTwoW6cV1b"}`
	if !HoldMaskedResponseFields([]byte(body), "application/json") {
		t.Fatal("a JSON-lines body was withheld")
	}
	for _, want := range []string{
		"Password1", "dupQ7vLx2Rk9", "upperK8wN3qZ6", "escT5yH2mL9c", "rootJ3vB8nQ1", "deepP9rL4tW7",
		"sshM4kZ8qT2v", "7391586420", "aébcdefgh", `a\u00e9bcdefgh`, "\xe4\xe4\xe4", "lineTwoW6cV1b",
	} {
		if !slices.Contains(*held, want) {
			t.Errorf("%q was not held; held %q", want, *held)
		}
	}
	for _, not := range []string{"•••", "u", "username"} {
		if slices.Contains(*held, not) {
			t.Errorf("%q was held", not)
		}
	}
}

func TestHoldMaskedResponseFieldsNotJSON(t *testing.T) {
	held := recordHolds(t)
	utf16le := func(s string) []byte {
		b := []byte{0xff, 0xfe}
		for _, u := range utf16.Encode([]rune(s)) {
			b = append(b, byte(u), byte(u>>8))
		}
		return b
	}
	for _, c := range []struct {
		name, ct string
		body     []byte
	}{
		{"form", "application/x-www-form-urlencoded", []byte("user=u&password=formQ7vLx2Rk9")},
		{"key-value lines", "text/plain", []byte("username: u\nrootPassword : kvK8wN3qZ6")},
		{"json-like text", "text/html", []byte(`<pre>{'sshPrivateKey': 'pemT5yH2mL9c'}</pre>`)},
		{"escaped key", "text/plain", []byte(`x "p\u0061ssword": "escJ3vB8nQ1"`)},
		{"percent-encoded key", "text/plain", []byte("p%61ssword=pctP9rL4tW7")},
		{"broken JSON", "application/json", []byte(`{"password":"brokenM4kZ8q"`)},
		{"utf-16 with a byte-order mark", "application/json", utf16le(`{"password":"wideW6cV1bN8"}`)},
		{"utf-16 declared", "application/json; charset=UTF-16LE", utf16le(`{"password":"wideW6cV1bN8"}`)[2:]},
	} {
		if HoldMaskedResponseFields(c.body, c.ct) {
			t.Errorf("%s: a body that names a credential field and is not JSON was let through", c.name)
		}
	}
	*held = nil
	for _, c := range []struct {
		name, ct string
		body     []byte
	}{
		{"empty", "", nil},
		{"error prose", "text/html", []byte("<h1>401</h1><p>Invalid username or password.</p>")},
		{"binary without the names", "application/octet-stream", []byte{0x4d, 0x5a, 0x90, 0x00, 0x03}},
		{"a utf-16 name in an undeclared binary", "application/octet-stream", utf16le("Password:")[2:]},
	} {
		if !HoldMaskedResponseFields(c.body, c.ct) {
			t.Errorf("%s: withheld a body that names no credential field the way a field is written", c.name)
		}
	}
	if len(*held) != 0 {
		t.Errorf("held from bodies that carry no field: %q", strings.Join(*held, " "))
	}
}

func TestHoldMaskedResponseHeaders(t *testing.T) {
	held := recordHolds(t)
	HoldMaskedResponseHeaders(http.Header{
		"Location":     {"https://vspc.example/x?a=1&password=locQ7vLx2Rk9&b=2#rootPassword=fragK8wN3qZ6"},
		"Password":     {" hdrT5yH2mL9c "},
		"Cf-Mitigated": {`sshPrivateKey: "cfJ3vB8nQ1zX"`},
		"Link":         {"<https://vspc.example/?p%61ssword=pctP9rL4tW7d>"},
		"X-Spaced":     {"password = two words here"},
		"X-Masked":     {"password=••••••", "rootPassword: ******"},
		"X-Prose":      {"Invalid username or password"},
	})
	for _, want := range []string{"locQ7vLx2Rk9", "fragK8wN3qZ6", "hdrT5yH2mL9c", "cfJ3vB8nQ1zX", "pctP9rL4tW7d", "two", "two words here"} {
		if !slices.Contains(*held, want) {
			t.Errorf("%q was not held; held %q", want, *held)
		}
	}
	for _, not := range []string{"••••••", "******", "Invalid", "username", "1", "2"} {
		if slices.Contains(*held, not) {
			t.Errorf("%q was held; held %q", not, *held)
		}
	}
}
