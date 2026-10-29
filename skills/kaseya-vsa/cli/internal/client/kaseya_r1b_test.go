// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Review round 1 (Codex, 2026-10-28) [P1] class: the output chokepoint
// (cliutil.HeldEchoes / WithholdText, in front of CLI output, MCP results,
// --deliver, export and the store) missed encodings the client's own masker
// (maskCredentialText) recognizes, so a response name holding the ROT13 of
// the token secret printed unchanged. The property tested here: every text
// the client masker would change, the chokepoint withholds. The corpus is
// every needle the masker builds for each held value, every encoder it knows
// applied to the secret, and every pair of those encoders.

import (
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"net/url"
	"sort"
	"strings"
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

const (
	r1bSecret = "Secret" + "Alpha901"     // split: a synthetic fixture, not a credential for secret scanners
	r1bID     = "kvsaid" + "4d2a9c71x0q5" // split: as above
)

func r1bHeld() []string {
	return []string{r1bSecret, "Basic " + base64.StdEncoding.EncodeToString([]byte(r1bID+":"+r1bSecret))}
}

// r1bEncoders is every transform the client masker encodes needles with or
// decodes views through, as an encoder.
func r1bEncoders() map[string]func(string) string {
	interleave := func(sep string) func(string) string {
		return func(s string) string { return strings.Join(strings.Split(s, ""), sep) }
	}
	return map[string]func(string) string{
		"rot13":       credentialRot13,
		"fullwidth":   credentialFullwidth,
		"percent-u":   credentialPercentU,
		"qp":          credentialQuotedPrintable,
		"upper":       strings.ToUpper,
		"lower":       strings.ToLower,
		"json-u":      func(s string) string { return credentialJSONUnicode(s, "0123456789abcdef") },
		"json-slash":  credentialJSONSlash,
		"html-all":    credentialHTMLAll,
		"html-hex":    func(s string) string { return credentialHTMLHexAll(s, "0123456789ABCDEF") },
		"slash-x":     func(s string) string { return credentialSlashX(s, "0123456789abcdef") },
		"slash-X":     func(s string) string { return strings.ReplaceAll(credentialSlashX(s, "0123456789ABCDEF"), `\x`, `\X`) },
		"percent-all": func(s string) string { return credentialPercentAll(s, "0123456789abcdef") },
		"query":       url.QueryEscape,
		"path":        url.PathEscape,
		"b64":         func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"b64url-raw":  func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) },
		"b32":         func(s string) string { return base32.StdEncoding.EncodeToString([]byte(s)) },
		"b32hex":      func(s string) string { return base32.HexEncoding.EncodeToString([]byte(s)) },
		"hex":         func(s string) string { return hex.EncodeToString([]byte(s)) },
		"hex-mixed":   func(s string) string { return credentialMixedHex(hex.EncodeToString([]byte(s))) },
		"utf16le-hex": func(s string) string { return hex.EncodeToString(credentialUTF16LE(s)) },
		"utf16be-raw": func(s string) string { return string(credentialUTF16BE(s)) },
		"spaced":      interleave(" "),
		"nbsp":        interleave(" "),
		"zero-width":  interleave("​"),
		"overlong": func(s string) string {
			var b []byte
			for i := 0; i < len(s); i++ {
				b = append(b, 0xC0|s[i]>>6, 0x80|s[i]&0x3F)
			}
			return string(b)
		},
		"utf7-plus": func(s string) string { return strings.ReplaceAll(s, "+", "+-") },
	}
}

// r1bCorpus is the set of encoded forms, keyed by how each was made.
func r1bCorpus() map[string]string {
	out := map[string]string{}
	for _, v := range r1bHeld() {
		v = strings.TrimSpace(v)
		vals := []string{v}
		if _, tok, ok := strings.Cut(v, " "); ok {
			vals = append(vals, tok)
		}
		for _, val := range vals {
			out["as-written "+val[:4]] = val
			out["query "+val[:4]] = url.QueryEscape(val)
			out["path "+val[:4]] = url.PathEscape(val)
			n := 0
			addEncodedCredentialMasks(val, func(needle string) {
				out[fmt.Sprintf("needle %s #%d", val[:4], n)] = needle
				n++
			})
		}
	}
	enc := r1bEncoders()
	names := make([]string, 0, len(enc))
	for k := range enc {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, a := range names {
		out[a] = enc[a](r1bSecret)
		for _, b := range names {
			out[b+"("+a+")"] = enc[b](enc[a](r1bSecret))
		}
	}
	return out
}

func TestR1BChokepointWithholdsEveryFormTheClientMaskerKnows(t *testing.T) {
	held := r1bHeld()
	cliutil.FunnelHold(held...)
	corpus := r1bCorpus()
	keys := make([]string, 0, len(corpus))
	for k := range corpus {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var detected, missed int
	for _, k := range keys {
		form := corpus[k]
		texts := []string{"name=" + form + " end"}
		if lit, err := json.Marshal(form); err == nil && strings.Contains(string(lit), form) {
			texts = append(texts, `{"Data":[{"Id":1,"Name":`+string(lit)+`}]}`)
		}
		for _, text := range texts {
			if (*Client)(nil).maskCredentialText(text, held...) == text {
				continue // the client masker does not recognize this form either
			}
			detected++
			if !cliutil.HeldEchoes(text) {
				missed++
				if missed <= 40 {
					t.Errorf("%s: the client masker withholds %q but the chokepoint does not", k, text)
				}
				continue
			}
			if got := cliutil.WithholdText(text); strings.Contains(got, form) {
				t.Errorf("%s: WithholdText kept the form: %q", k, got)
			}
		}
	}
	if missed > 40 {
		t.Errorf("... and %d more forms missed", missed-40)
	}
	if detected < 500 {
		t.Fatalf("the client masker recognized only %d corpus texts; the corpus proves nothing", detected)
	}
	t.Logf("%d corpus texts the client masker recognizes; %d missed by the chokepoint", detected, missed)
}

// maskerPins is the SHA-256 of each client function the chokepoint's detector
// (maskerEchoDetector, kaseya_mask.go) mirrors rather than calls (comments
// dropped, gofmt layout): maskCredentialText's needle builder, the span list
// credentialRevealSpans walks, and finishCredentialMask's rule for when a
// text changes. Everything else the detector runs is the client's own code.
// A reprint that changes one of these bodies fails here: update
// maskerEchoDetector or credentialMaskSpans to match, then re-pin.
var maskerPins = map[string]string{
	"maskCredentialText":    "a980776c12d537bc244a1c4ebc729284a6f410d38527eb38d98371c022a2f496",
	"credentialRevealSpans": "9d3f36561cc8b19b819908fe663c51443d468890ba107be3fda77368247efbc9",
	"finishCredentialMask":  "5b2be6218af39bbbda400fed2240b73b00ac67de83ee00263da84a7ccc63313b",
}

func TestR1BMaskCredentialTextUnchangedSinceTheChokepointMirroredIt(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "client.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		pin, ok := maskerPins[fn.Name.Name]
		if !ok {
			continue
		}
		found[fn.Name.Name] = true
		var b strings.Builder
		if err := printer.Fprint(&b, fset, fn.Body); err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(b.String()))); got != pin {
			t.Errorf("%s changed (body sha256 %s): update maskerEchoDetector / credentialMaskSpans in kaseya_mask.go to match, then re-pin", fn.Name.Name, got)
		}
	}
	for name := range maskerPins {
		if !found[name] {
			t.Errorf("%s not found in client.go", name)
		}
	}
}

// Review round 2 (Codex, 2026-10-28) [P1] class credential-provenance: a
// credential equal to an encoding the chokepoint derives from another (the
// MCP HTTP token set to the ASCII85 of the token secret) was taken for a
// derived needle, so the client masker never saw it (its ROT13 printed) and
// its own encodings were never derived. Both hold orders: the token held with
// the secret, and held after the secret had been expanded.
func TestR2HeldCredentialIsNeverTreatedAsADerivedNeedle(t *testing.T) {
	a85 := func(s string) string {
		b := make([]byte, ascii85.MaxEncodedLen(len(s)))
		return string(b[:ascii85.Encode(b, []byte(s))])
	}
	if a85(strings.Repeat("0", 32)) != strings.Repeat("0JG17", 8) {
		t.Fatal("premise: the review's token is the ASCII85 of its secret")
	}
	for _, tc := range []struct {
		order, secret string
		later         bool
	}{
		{"token held with the secret", strings.Repeat("0", 32), false},
		{"token held after the secret was expanded", strings.Repeat("1", 32), true},
	} {
		token := a85(tc.secret)
		if tc.later {
			cliutil.FunnelHold(tc.secret)
			cliutil.HeldEchoes("expand the held set now")
			cliutil.FunnelHold(token)
		} else {
			cliutil.FunnelHold(tc.secret, token)
		}
		for what, text := range map[string]string{
			"the token's ROT13 (client masker)":            "feedback " + credentialRot13(token),
			"the token's ASCII85 (its own derived needle)": "x " + a85(token) + " y",
			"the secret's ASCII85 at byte 1 (derived)":     "x " + a85(tc.secret[1:29]) + " y",
		} {
			if !cliutil.HeldEchoes(text) {
				t.Errorf("%s: %s is not withheld: %q", tc.order, what, text)
			}
		}
	}
}
