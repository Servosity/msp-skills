// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/ascii85"
	"fmt"
	"html"
	"net/url"
	"strings"
	"testing"
)

// Class undecoded-encoding-gap (Codex r2, fixed after the stop rule): a held
// needle shorter than the shared funnel's floor (a short credential, or one
// ASCII85 group of a longer one) is searched in every view the funnel builds,
// not only the raw, JSON-unescaped and percent-decoded ones.
func TestShortNeedlesSearchedInEveryFunnelView(t *testing.T) {
	views := map[string]func(string) string{
		"html": html.EscapeString,
		"html-numeric": func(s string) string {
			var b strings.Builder
			for _, r := range s {
				fmt.Fprintf(&b, "&#%d;", r)
			}
			return b.String()
		},
		"json-unicode": func(s string) string {
			var b strings.Builder
			for _, r := range s {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
			return b.String()
		},
		"percent-all": func(s string) string {
			var b strings.Builder
			for i := 0; i < len(s); i++ {
				fmt.Fprintf(&b, "%%%02X", s[i])
			}
			return b.String()
		},
		"query-escape": url.QueryEscape,
		"tags":         func(s string) string { return "<b>" + s[:len(s)/2] + "</b>" + s[len(s)/2:] },
		"identity": func(s string) string {
			var b strings.Builder
			for _, r := range s {
				b.WriteString(`\` + string(r))
			}
			return b.String()
		},
	}
	for _, pw := range []string{"P@ssw0rd", "Kq8#mZ2!xR", "ShrtPw7"} {
		FunnelHold(pw)
		stream := []byte("a" + pw + "xyz")
		buf := make([]byte, ascii85.MaxEncodedLen(len(stream)))
		a85 := string(buf[:ascii85.Encode(buf, stream)])
		for name, enc := range views {
			for _, form := range []string{pw, a85} {
				text := `{"name":"` + enc(form) + `","power_state":"POWERED_OFF"}`
				if !HeldEchoes(text) {
					t.Errorf("%s view of %q (password %q) not withheld: %s", name, form, pw, text)
				}
			}
		}
	}
}
