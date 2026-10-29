// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// pax8 handfix (undecoded-encodings-every-alignment, Codex r1 4.34.2 class
// secret-in-encoded-output): the shared funnel decodes JSON, percent, HTML,
// markup, base64 and hex views, and pax8_output.go matches ASCII85, Z85 and
// base32 as needles. Every other text encoding of a held value is closed
// here, by family rather than by name:
//
//   - per-byte numeric encodings, any introducer, terminator and separator:
//     quoted-printable (=41, soft line breaks), \x41, 0x41, %u0041, A,
//     \u{41}, \x{41}, \U00000041, &#x41; and &#65;, U+0041, hex pairs and
//     four-digit code points separated by :, -, space, comma or dot, decimal
//     byte lists ("65,66" and "[65, 66]"), octal escapes
//     (\101) and octal lists, binary octets (spaced or run together). They are
//     decoded into views (byteEscapeViews) of the text and of every view the
//     funnel itself makes (so a percent- or JSON-escaped quoted-printable
//     stream is read too), and each view is searched like the text.
//   - fixed-alphabet bit encodings the funnel does not decode (uuencode with
//     either zero character, xxencode, crypt and bcrypt base64, Crockford and
//     z-base-32): matched as needles of the characters a held value fixes at
//     each byte alignment (alphabetCores).
//   - group encodings the funnel does not decode: RFC 1924 base85 (4 bytes to
//     5 characters) and RFC 9285 base45 (2 bytes to 3), matched as the run
//     of whole groups inside a held value at each alignment (groupCores).
//   - whole-number radix encodings (base36, base58, base62): matched as the
//     whole value's encoding (radixNeedles).
//   - character transforms: the value reversed, upper- and lower-cased,
//     ROT13 and ROT47 (transformedForms), held beside it.

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const byteEscapeSep = `[ \t,:;|.\-]`

var (
	// hexEscapeRun is a run of at least four introduced hex bytes; hexEscapeTok
	// reads one byte of it. The longer introducers come first: matching is
	// case-insensitive, so \u00 would otherwise take the head of \U00000041.
	hexEscapeRun = regexp.MustCompile(`(?i)(?:(?:\\U000000|\\u\{0*|\\x\{0*|\\u00|\\x|%u00|&#x0*|0x|u\+00|=|%|#)[0-9a-f]{2}[;}]?` + byteEscapeSep + `{0,2}){4,}`)
	hexEscapeTok = regexp.MustCompile(`(?i)(?:\\U000000|\\u\{0*|\\x\{0*|\\u00|\\x|%u00|&#x0*|0x|u\+00|=|%|#)([0-9a-f]{2})`)
	// sepHexRun is a run of bare hex pairs with a separator between each.
	sepHexRun = regexp.MustCompile(`(?i)(?:^|[^0-9a-z])([0-9a-f]{2}(?:` + byteEscapeSep + `{1,2}[0-9a-f]{2}){3,})(?:$|[^0-9a-z])`)
	hexPair   = regexp.MustCompile(`(?i)[0-9a-f]{2}`)
	// codepointRun is a run of separated four-digit code points (0041 0042).
	codepointRun = regexp.MustCompile(`(?i)(?:^|[^0-9a-z])(00[0-9a-f]{2}(?:` + byteEscapeSep + `{1,2}00[0-9a-f]{2}){3,})(?:$|[^0-9a-z])`)
	codepointTok = regexp.MustCompile(`(?i)00([0-9a-f]{2})`)
	// decRun is a run of separated numbers of one to three digits (decimal
	// or, all of three octal digits, octal byte values).
	decRun   = regexp.MustCompile(`(?:^|[^0-9])(\d{1,3}(?:` + byteEscapeSep + `{1,3}\d{1,3}){3,})(?:$|[^0-9])`)
	decTok   = regexp.MustCompile(`\d+`)
	octEsc   = regexp.MustCompile(`(?:\\[0-7]{1,3}){4,}`)
	octTok   = regexp.MustCompile(`\\([0-7]{1,3})`)
	binRun   = regexp.MustCompile(`(?i)(?:^|[^01])((?:0b)?[01]{8}(?:` + byteEscapeSep + `{0,2}(?:0b)?[01]{8}){3,})(?:$|[^01])`)
	binTok   = regexp.MustCompile(`[01]{8}`)
	qpSoftNL = strings.NewReplacer("=\r\n", "", "=\n", "")
)

// byteEscapeViews returns, for each per-byte numeric encoding run in text,
// the text with that run replaced by its bytes and the bytes alone; nil when
// there is none.
func byteEscapeViews(text string) []string {
	var views []string
	add := func(start, end int, decoded []byte) {
		if len(decoded) < shortMinLen {
			return
		}
		views = append(views, string(decoded), text[:start]+string(decoded)+text[end:])
	}
	if strings.Contains(text, "=\n") || strings.Contains(text, "=\r\n") {
		if s := qpSoftNL.Replace(text); s != text {
			views = append(views, s)
			for _, v := range byteEscapeViews(s) {
				views = append(views, v)
			}
		}
	}
	for _, m := range hexEscapeRun.FindAllStringIndex(text, -1) {
		var b []byte
		for _, tok := range hexEscapeTok.FindAllStringSubmatch(text[m[0]:m[1]], -1) {
			n, _ := strconv.ParseUint(tok[1], 16, 8)
			b = append(b, byte(n))
		}
		add(m[0], m[1], b)
	}
	for _, m := range sepHexRun.FindAllStringSubmatchIndex(text, -1) {
		var b []byte
		for _, p := range hexPair.FindAllString(text[m[2]:m[3]], -1) {
			n, _ := strconv.ParseUint(p, 16, 8)
			b = append(b, byte(n))
		}
		add(m[2], m[3], b)
	}
	for _, m := range codepointRun.FindAllStringSubmatchIndex(text, -1) {
		var b []byte
		for _, tok := range codepointTok.FindAllStringSubmatch(text[m[2]:m[3]], -1) {
			n, _ := strconv.ParseUint(tok[1], 16, 8)
			b = append(b, byte(n))
		}
		add(m[2], m[3], b)
	}
	for _, m := range decRun.FindAllStringSubmatchIndex(text, -1) {
		toks := decTok.FindAllString(text[m[2]:m[3]], -1)
		var dec, oct []byte
		decOK, octOK := true, true
		for _, tok := range toks {
			if n, err := strconv.ParseUint(tok, 10, 8); err == nil {
				dec = append(dec, byte(n))
			} else {
				decOK = false
			}
			if n, err := strconv.ParseUint(tok, 8, 8); err == nil && len(tok) == 3 {
				oct = append(oct, byte(n))
			} else {
				octOK = false
			}
		}
		if decOK {
			add(m[2], m[3], dec)
		}
		if octOK {
			add(m[2], m[3], oct)
		}
	}
	for _, m := range octEsc.FindAllStringIndex(text, -1) {
		var b []byte
		for _, tok := range octTok.FindAllStringSubmatch(text[m[0]:m[1]], -1) {
			if n, err := strconv.ParseUint(tok[1], 8, 8); err == nil {
				b = append(b, byte(n))
			}
		}
		add(m[0], m[1], b)
	}
	for _, m := range binRun.FindAllStringSubmatchIndex(text, -1) {
		var b []byte
		for _, tok := range binTok.FindAllString(text[m[2]:m[3]], -1) {
			n, _ := strconv.ParseUint(tok, 2, 8)
			b = append(b, byte(n))
		}
		add(m[2], m[3], b)
	}
	return views
}

// byteEscapeEchoes reports a held value in a byte-escape view of text or of
// any view the funnel decodes from it (the funnel's own matcher on each, and a
// substring match for the short held values the funnel leaves out).
func byteEscapeEchoes(text string, held []string) bool {
	if !mayHoldByteEscapes(text) {
		return false
	}
	// The funnel's views are decoded too only when the text holds an escape
	// that could hide a byte-escape run (a percent, backslash or entity
	// escape): building every view of a large plain output costs more than
	// the rest of the check.
	bases := []string{text}
	if strings.ContainsAny(text, "%\\&") {
		if views, _ := funnelViews(text); len(views) > 0 {
			bases = views
		}
	}
	seen := map[string]bool{}
	for _, base := range bases {
		for _, v := range byteEscapeViews(base) {
			if seen[v] {
				continue
			}
			seen[v] = true
			if windowedEchoes(v, held) {
				return true
			}
			for _, h := range held {
				if len(h) >= shortMinLen && len(h) < heldMatchFloor && strings.Contains(v, h) {
					return true
				}
			}
			at := lettersAndDigits(v)
			for _, h := range held {
				if n := lettersAndDigits(h); len(n) >= joinedMinAlnum && strings.Contains(at, n) {
					return true
				}
			}
		}
	}
	return false
}

// mayHoldByteEscapes is a cheap pre-check: a byte-escape run needs at least
// eight digits.
func mayHoldByteEscapes(text string) bool {
	n := 0
	for i := 0; i < len(text); i++ {
		if c := text[i]; '0' <= c && c <= '9' {
			if n++; n >= 8 {
				return true
			}
		}
	}
	return false
}

// bitAlphabet is a fixed-alphabet encoding with bits per character.
type bitAlphabet struct {
	bits     int
	alphabet string
	fold     bool // case-insensitive: the lowercase needle too
}

var undecodedBitAlphabets = []bitAlphabet{
	{6, "`!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_", false}, // uuencode
	{6, " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_", false}, // uuencode (space for zero)
	{6, "+-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", false},   // xxencode
	{6, "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", false},   // crypt
	{6, "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", false},   // bcrypt
	{5, "0123456789ABCDEFGHJKMNPQRSTVWXYZ", true},                                    // Crockford base32
	{5, "ybndrfg8ejkmcpqxot1uwisza345h769", false},                                   // z-base-32
}

// alphabetCores is, for each undecoded bit alphabet and each byte offset of v
// in a longer stream, the run of characters whose bits lie wholly inside v
// (at least heldMatchFloor of them), so v is matched wherever it sits in an
// encoded stream.
func alphabetCores(v string) []string {
	var out []string
	for _, a := range undecodedBitAlphabets {
		group := 1
		for (8*group)%a.bits != 0 {
			group++
		}
		for off := 0; off < group; off++ {
			buf := make([]byte, off+len(v)+group)
			copy(buf[off:], v)
			lo := (8*off + a.bits - 1) / a.bits
			hi := 8 * (off + len(v)) / a.bits
			if hi-lo < heldMatchFloor {
				continue
			}
			var sb strings.Builder
			for k := lo; k < hi; k++ {
				sb.WriteByte(a.alphabet[bitsAt(buf, k*a.bits, a.bits)])
			}
			out = append(out, sb.String())
			if a.fold {
				out = append(out, strings.ToLower(sb.String()))
			}
		}
	}
	return out
}

func bitsAt(buf []byte, start, n int) int {
	v := 0
	for i := 0; i < n; i++ {
		bit := start + i
		v = v<<1 | int(buf[bit/8]>>(7-uint(bit%8))&1)
	}
	return v
}

const (
	rfc1924Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"
	base45Alphabet  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
)

// groupCores is, at each byte alignment, the encoding of the whole groups
// inside v: RFC 1924 base85 (big-endian 4-byte groups) and base45 (2-byte
// groups, least significant character first).
func groupCores(v string) []string {
	var out []string
	for a := 0; a < 4; a++ {
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%4]
		if len(core) < 8 {
			continue
		}
		var sb strings.Builder
		for i := 0; i < len(core); i += 4 {
			n := uint32(core[i])<<24 | uint32(core[i+1])<<16 | uint32(core[i+2])<<8 | uint32(core[i+3])
			var g [5]byte
			for j := 4; j >= 0; j-- {
				g[j] = rfc1924Alphabet[n%85]
				n /= 85
			}
			sb.Write(g[:])
		}
		out = append(out, sb.String())
	}
	for a := 0; a < 2; a++ {
		core := []byte(v[min(a, len(v)):])
		core = core[:len(core)-len(core)%2]
		if len(core) < 6 {
			continue
		}
		var sb strings.Builder
		for i := 0; i < len(core); i += 2 {
			n := int(core[i])<<8 | int(core[i+1])
			sb.WriteByte(base45Alphabet[n%45])
			sb.WriteByte(base45Alphabet[(n/45)%45])
			sb.WriteByte(base45Alphabet[n/2025])
		}
		out = append(out, sb.String())
	}
	return out
}

// otherEncodingNeedles is every needle this file adds for a held value: its
// fixed-alphabet, group and radix encodings and its character transforms.
func otherEncodingNeedles(v string) []string {
	if len(v) < shortMinLen {
		return nil
	}
	out := alphabetCores(v)
	out = append(out, groupCores(v)...)
	out = append(out, radixNeedles(v)...)
	return append(out, transformedForms(v)...)
}

// transformedForms is v reversed, upper- and lower-cased, ROT13 and ROT47
// (values of heldMatchFloor or more characters; each differing from v).
func transformedForms(v string) []string {
	if len(v) < heldMatchFloor {
		return nil
	}
	b := []byte(v)
	rev := make([]byte, len(b))
	r13 := make([]byte, len(b))
	r47 := make([]byte, len(b))
	for i, c := range b {
		rev[len(b)-1-i] = c
		r13[i], r47[i] = c, c
		switch {
		case 'a' <= c && c <= 'z':
			r13[i] = 'a' + (c-'a'+13)%26
		case 'A' <= c && c <= 'Z':
			r13[i] = 'A' + (c-'A'+13)%26
		}
		if '!' <= c && c <= '~' {
			r47[i] = '!' + (c-'!'+47)%94
		}
	}
	var out []string
	for _, f := range []string{string(rev), strings.ToUpper(v), strings.ToLower(v), string(r13), string(r47)} {
		if f != v {
			out = append(out, f)
		}
	}
	return out
}

var radixAlphabets = []string{
	"0123456789abcdefghijklmnopqrstuvwxyz",                           // base36
	"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ",                           // base36 upper
	"123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz",     // base58 (Bitcoin)
	"123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ",     // base58 (Flickr)
	"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", // base62
	"0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", // base62 (lower first)
}

// radixNeedles is v as one big-endian number in each whole-number radix
// encoding.
func radixNeedles(v string) []string {
	if len(v) < heldMatchFloor {
		return nil
	}
	var out []string
	for _, alphabet := range radixAlphabets {
		n := new(big.Int).SetBytes([]byte(v))
		base := big.NewInt(int64(len(alphabet)))
		m := new(big.Int)
		var digits []byte
		for n.Sign() > 0 {
			n.DivMod(n, base, m)
			digits = append(digits, alphabet[m.Int64()])
		}
		for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
			digits[i], digits[j] = digits[j], digits[i]
		}
		out = append(out, string(digits))
	}
	return out
}
