// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package dsblackbox

// Class held-value-in-an-undecoded-encoding: the fake API also echoes each
// credential in the text encodings the credential leak contract (section 3)
// names and the kit decodes: quoted-printable (whole and only some bytes),
// case changes, ROT13, fullwidth forms (NFKC), octal escapes, a list of
// character codes, and quoted-printable under base64. Forms outside that
// closed list (combining marks, %uXXXX, overlong UTF-8, UTF-7) are not part
// of the oracle.

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func qpForm(s string, some bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if !some || i%3 == 1 {
			fmt.Fprintf(&b, "=%02X", s[i])
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func rot13Form(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		}
	}
	return string(b)
}

func eachForm(s, format string) string {
	var b strings.Builder
	for _, r := range s {
		if format == "fullwidth" {
			if r >= 0x21 && r <= 0x7e {
				r += 0xfee0
			}
			b.WriteRune(r)
			continue
		}
		fmt.Fprintf(&b, format, r)
	}
	return b.String()
}

func codesForm(s string) string {
	parts := make([]string, 0, len(s))
	for _, r := range s {
		parts = append(parts, fmt.Sprint(int(r)))
	}
	return strings.Join(parts, ",")
}

// textEncodedForms is appended to echoForms.
func textEncodedForms(s string) []string {
	return []string{
		qpForm(s, false),
		qpForm(s, true),
		strings.ToUpper(s),
		strings.ToLower(s),
		rot13Form(s),
		eachForm(s, "fullwidth"),
		eachForm(s, `\%03o`),
		codesForm(s),
		base64.StdEncoding.EncodeToString([]byte(qpForm(s, false))),
	}
}
