// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored test support (not generator output); pinned by
// skills/eset-protect/handfixes.json under "redaction-sentinel-variants".
// Only _test.go files import this package, so it never reaches a binary.

package credvariants

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// EncodedForms are the binary-to-text and transformed spellings of s a
// server could echo: base64 (both alphabets, padded or not, at every byte
// offset inside a longer blob, line-wrapped, of the query-escaped value, of
// UTF-16, twice), base32 (both alphabets, either case), hex (either case, of
// UTF-8 and UTF-16), rot13, fullwidth, \x and %u escapes, quoted-printable,
// overlong UTF-8, one character per space and with zero-width joiners.
// Fixed marks the ones the registry masks in place; the rest may be withheld.
func EncodedForms(s string) []SecretForm {
	b := []byte(s)
	std := base64.StdEncoding.EncodeToString(b)
	var wrapped strings.Builder
	for i := 0; i < len(std); i += 6 {
		end := i + 6
		if end > len(std) {
			end = len(std)
		}
		wrapped.WriteString(std[i:end])
		if end < len(std) {
			wrapped.WriteString("\r\n")
		}
	}
	le, be := utf16Bytes(s, false), utf16Bytes(s, true)
	var slashX, pctU, qp, overlong, spaced, joined strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&slashX, `\x%02x`, s[i])
		c := s[i]
		if c >= 33 && c <= 126 && c != '=' {
			qp.WriteByte(c)
		} else {
			fmt.Fprintf(&qp, "=%02X", c)
		}
		if c < utf8.RuneSelf {
			overlong.WriteByte(0xC0 | c>>6)
			overlong.WriteByte(0x80 | c&0x3F)
		} else {
			overlong.WriteByte(c)
		}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&pctU, "%%u%04X", u)
	}
	for i, r := range s {
		if i > 0 {
			spaced.WriteByte(' ')
			joined.WriteString("\u200b")
		}
		spaced.WriteRune(r)
		joined.WriteRune(r)
	}
	return []SecretForm{
		{"base64", std, true},
		{"base64-raw-url", base64.RawURLEncoding.EncodeToString(b), true},
		{"base64-offset-1", base64.StdEncoding.EncodeToString(append([]byte("p"), b...)), false},
		{"base64-offset-2", base64.RawURLEncoding.EncodeToString(append([]byte("pw"), b...)) + "x", false},
		{"base64-in-form", base64.StdEncoding.EncodeToString([]byte("grant_type=password&password=" + url.QueryEscape(s) + "&x=1")), false},
		{"base64-wrapped", wrapped.String(), false},
		{"base64-twice", base64.StdEncoding.EncodeToString([]byte(std)), false},
		{"base64-thrice", base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString([]byte(std)))), false},
		{"base64-of-rot13", base64.StdEncoding.EncodeToString([]byte(rot13(s))), false},
		{"base32-mixed-case-offset", mixedCase(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(append([]byte("p"), b...))), false},
		{"hex-utf16be-lower", hex.EncodeToString(be), true},
		{"base64-utf16le", base64.StdEncoding.EncodeToString(le), true},
		{"base32", base32.StdEncoding.EncodeToString(b), true},
		{"base32-hex-lower", strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), true},
		{"hex", hex.EncodeToString(b), true},
		{"hex-upper-utf16be", strings.ToUpper(hex.EncodeToString(be)), true},
		{"rot13", rot13(s), false},
		{"fullwidth", fullwidth(s), false},
		{"slash-x", slashX.String(), false},
		{"percent-u", pctU.String(), false},
		{"quoted-printable", qp.String(), false},
		{"overlong-utf8", overlong.String(), false},
		{"spaced", spaced.String(), false},
		{"zero-width-joined", joined.String(), false},
	}
}

// mixedCase alternates the case of each letter.
func mixedCase(s string) string {
	b := []byte(s)
	for i := range b {
		if i%2 == 1 && 'A' <= b[i] && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func utf16Bytes(s string, bigEndian bool) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2*len(units))
	for _, u := range units {
		hi, lo := byte(u>>8), byte(u&0xFF)
		if bigEndian {
			out = append(out, hi, lo)
		} else {
			out = append(out, lo, hi)
		}
	}
	return out
}

func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case 'A' <= r && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func fullwidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ':
			return 0x3000
		case 0x21 <= r && r <= 0x7E:
			return r + 0xFEE0
		}
		return r
	}, s)
}

// EncodedRecoverable is the oracle for EncodedForms, written apart from the
// masker with regexps and the standard decoders: SecretRecoverable, or the
// secret (or a ten-byte run of it) after decoding any base64, base32 or hex
// run at every offset (whitespace and "=" inside a run ignored, UTF-16 read
// both ways), rot13, fullwidth folding, \x, %u, quoted-printable and
// overlong UTF-8 decoding, or with whitespace and zero-width characters
// removed; two levels deep.
func EncodedRecoverable(out []byte, secret string) bool {
	return encodedRecoverable(string(out), secret, 0)
}

// encodedRecoverable searches every view two levels deep, and a binary
// decoding that is printable text up to six levels deep.
func encodedRecoverable(text, secret string, depth int) bool {
	if SecretRecoverable([]byte(text), secret) {
		return true
	}
	if depth >= 6 {
		return false
	}
	views, decoded := oracleViews(text, len(secret))
	if depth >= 2 {
		views = nil
		for _, v := range decoded {
			if printableText(v) {
				views = append(views, v)
			}
		}
	}
	for _, v := range views {
		if encodedRecoverable(v, secret, depth+1) {
			return true
		}
	}
	return false
}

func printableText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

var (
	oWhitespace = regexp.MustCompile(`[\s\x{00a0}\x{2000}-\x{200a}\x{202f}\x{205f}\x{3000}]+`)
	oInvisible  = regexp.MustCompile(`[\x{200b}\x{200c}\x{200d}\x{2060}\x{feff}\x{180e}]`)
	oB64Run     = regexp.MustCompile(`[A-Za-z0-9+/_=-]+`)
	oHexRun     = regexp.MustCompile(`[0-9A-Fa-f]+`)
	oB32Run     = regexp.MustCompile(`[A-Za-z2-7=]+`)
	oB32HexRun  = regexp.MustCompile(`[0-9A-Va-v=]+`)
	oQPSoft     = regexp.MustCompile(`=\r?\n`)
	oQP         = regexp.MustCompile(`=([0-9A-Fa-f]{2})`)
	oPctU       = regexp.MustCompile(`%[uU]([0-9A-Fa-f]{4})`)
	oPctURun    = regexp.MustCompile(`(?:%[uU][0-9A-Fa-f]{4})+`)
	oSlashX     = regexp.MustCompile(`\\[xX]([0-9A-Fa-f]{2})`)
)

// oracleViews decodes text; n is the secret's length in bytes, which
// bounds the shortest run that can hold an encoding of it.
func oracleViews(text string, n int) (views, decoded []string) {
	min := func(v, max int) int {
		if v > max {
			return max
		}
		return v
	}
	b64Min, hexMin, b32Min := min((8*n+5)/6, 8), min(2*n, 16), min((8*n+4)/5, 13)
	var out []string
	add := func(v string) {
		if v != "" && v != text {
			out = append(out, v)
		}
	}
	addBytes := func(b []byte) {
		add(string(b))
		decoded = append(decoded, string(b))
		for start := 0; start < 2 && start < len(b); start++ {
			for _, bigEndian := range []bool{false, true} {
				units := make([]uint16, 0, len(b)/2)
				for i := start; i+1 < len(b); i += 2 {
					if bigEndian {
						units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
					} else {
						units = append(units, uint16(b[i+1])<<8|uint16(b[i]))
					}
				}
				add(string(utf16.Decode(units)))
			}
		}
	}
	unhex := func(m string) byte {
		v, _ := strconv.ParseUint(m, 16, 8)
		return byte(v)
	}
	stripped := oWhitespace.ReplaceAllString(text, "")
	add(stripped)
	add(oInvisible.ReplaceAllString(text, ""))
	add(rot13(text))
	add(strings.Map(func(r rune) rune {
		switch {
		case r == 0x3000:
			return ' '
		case 0xFF01 <= r && r <= 0xFF5E:
			return r - 0xFEE0
		}
		return r
	}, text))
	add(oQP.ReplaceAllStringFunc(oQPSoft.ReplaceAllString(text, ""), func(m string) string { return string([]byte{unhex(m[1:])}) }))
	add(oPctURun.ReplaceAllStringFunc(text, func(m string) string {
		var units []uint16
		for _, u := range oPctU.FindAllStringSubmatch(m, -1) {
			v, _ := strconv.ParseUint(u[1], 16, 16)
			units = append(units, uint16(v))
		}
		return string(utf16.Decode(units))
	}))
	add(oSlashX.ReplaceAllStringFunc(text, func(m string) string { return string([]byte{unhex(m[2:])}) }))
	add(decodeOverlong(text))
	for _, t := range []string{text, stripped} {
		for _, run := range oB64Run.FindAllString(t, -1) {
			if len(run) < b64Min {
				continue
			}
			clean := strings.NewReplacer("=", "", "-", "+", "_", "/").Replace(run)
			for off := 0; off < 4 && off < len(clean); off++ {
				s := clean[off:]
				if len(s)%4 == 1 {
					s = s[:len(s)-1]
				}
				if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
					addBytes(b)
				}
			}
		}
		for _, run := range oHexRun.FindAllString(t, -1) {
			if len(run) < hexMin {
				continue
			}
			for off := 0; off < 2; off++ {
				s := run[off:]
				if len(s)%2 == 1 {
					s = s[:len(s)-1]
				}
				if b, err := hex.DecodeString(s); err == nil {
					addBytes(b)
				}
			}
		}
		for _, alpha := range []struct {
			re  *regexp.Regexp
			enc *base32.Encoding
		}{{oB32Run, base32.StdEncoding}, {oB32HexRun, base32.HexEncoding}} {
			for _, run := range alpha.re.FindAllString(t, -1) {
				if len(run) < b32Min {
					continue
				}
				clean := strings.ToUpper(strings.ReplaceAll(run, "=", ""))
				for off := 0; off < 8 && off < len(clean); off++ {
					s := clean[off:]
					for len(s) > 0 && (len(s)%8 == 1 || len(s)%8 == 3 || len(s)%8 == 6) {
						s = s[:len(s)-1]
					}
					if b, err := alpha.enc.WithPadding(base32.NoPadding).DecodeString(s); err == nil {
						addBytes(b)
					}
				}
			}
		}
	}
	return out, decoded
}

// decodeOverlong reads every overlong two-byte UTF-8 sequence (C0/C1 lead)
// as the ASCII byte it spells.
func decodeOverlong(s string) string {
	b := []byte(s)
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if (b[i] == 0xC0 || b[i] == 0xC1) && i+1 < len(b) && b[i+1]&0xC0 == 0x80 {
			out = append(out, (b[i]&0x1F)<<6|b[i+1]&0x3F)
			i++
			continue
		}
		out = append(out, b[i])
	}
	return string(out)
}
