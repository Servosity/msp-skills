// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"testing"
)

// maskOracleEncoders are valid (reversible) encodings a server could put a
// held secret through before echoing it: percent forms in either hex case,
// the form encoding ('+' for a space), and HTML entities, named or numeric.
var maskOracleEncoders = map[string]func(string) string{
	"query":     url.QueryEscape,
	"path":      url.PathEscape,
	"queryLow":  func(s string) string { return lowerHex(url.QueryEscape(s)) },
	"pctAll":    func(s string) string { return pctEveryByte(s, "%%%02X") },
	"pctAllLow": func(s string) string { return pctEveryByte(s, "%%%02x") },
	"html":      html.EscapeString,
	"htmlNum": func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, "&#%d;", r)
			}
		}
		return b.String()
	},
}

func pctEveryByte(s, format string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, format, c)
		}
	}
	return b.String()
}

func lowerHex(s string) string {
	b := []byte(s)
	for i := 0; i+2 < len(b); i++ {
		if b[i] == '%' {
			for j := i + 1; j <= i+2; j++ {
				if b[j] >= 'A' && b[j] <= 'F' {
					b[j] += 'a' - 'A'
				}
			}
		}
	}
	return string(b)
}

// TestMaskOracleEveryEncodingComposition drives every held secret through
// every composition of up to three encoders and checks the echo never
// survives masking: as an entire string value (any secret length) and inside
// longer text (secrets of 8 or more characters). The oracle is the encoded
// echo itself, independent of the masker's decoder.
func TestMaskOracleEveryEncodingComposition(t *testing.T) {
	secrets := []string{"alpha beta/foo", "pw%2Fabcdef", `p@ss w"rd&9931+x`, "Ünïcødé Pass 42", "pw%2Fab", "abc", "a+b c%41"}
	names := make([]string, 0, len(maskOracleEncoders))
	for n := range maskOracleEncoders {
		names = append(names, n)
	}
	chains := [][]string{{}}
	for depth := 0; depth < 3; depth++ {
		for _, c := range chains {
			if len(c) != depth {
				continue
			}
			for _, n := range names {
				chains = append(chains, append(append([]string{}, c...), n))
			}
		}
	}
	checked := 0
	for _, secret := range secrets {
		for _, chain := range chains {
			echo := secret
			for _, n := range chain {
				echo = maskOracleEncoders[n](echo)
			}
			cases := map[string]string{"whole": echo}
			if len(secret) >= secretSubstringMin {
				cases["inside"] = "note " + echo + " end"
			}
			for kind, value := range cases {
				body, _ := json.Marshal(map[string]any{"v": value, "n": 482913})
				out := MaskResponse(body, secret)
				var got map[string]any
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatalf("masked answer is not JSON for %q via %v: %s", secret, chain, out)
				}
				v, _ := got["v"].(string)
				if strings.Contains(v, echo) {
					t.Fatalf("%s echo of %q via %v survived masking: %q", kind, secret, chain, v)
				}
				if got["n"] != float64(482913) {
					t.Fatalf("a number was rewritten: %s", out)
				}
				checked++
			}
		}
	}
	// Control: text that is not a secret at any decoding is left alone.
	body := []byte(`{"v":"%2Fhello+world&amp; 100%"}`)
	if out := MaskResponse(body, secrets...); string(out) != string(body) {
		t.Fatalf("a value that carries no secret was masked: %s", out)
	}
	if checked < 4000 {
		t.Fatalf("oracle ran only %d cases", checked)
	}
}
