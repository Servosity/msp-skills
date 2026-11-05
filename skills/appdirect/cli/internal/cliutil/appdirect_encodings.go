// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// appdirect handfix (undecoded-encodings-every-alignment, Codex r1 4.34.2 class
// secret-in-encoded-output): the shared funnel decodes JSON, percent, HTML,
// markup, base64 and hex views, and appdirect_output.go matches ASCII85, Z85 and
// base32 as needles. Every other text encoding of a held value is closed
// here, by family rather than by name:
//
//   - per-byte numeric encodings, any introducer, terminator and separator:
//     quoted-printable (=41, soft line breaks), \x41, 0x41, %u0041, A,
//     \u{41}, &#x41; and &#65;, U+0041, hex pairs separated by :, -, space,
//     comma or dot, decimal byte lists ("65,66" and "[65, 66]"), octal escapes
//     (\101) and octal lists, binary octets (spaced or run together). They are
//     decoded into views (byteEscapeViews) of the text and of every view the
//     funnel itself makes (so a percent- or JSON-escaped quoted-printable
//     stream is read too), and each view is searched like the text.
//   - fixed-alphabet bit encodings the funnel does not decode (uuencode with
//     either zero character, xxencode, crypt and bcrypt base64, Crockford and
//     z-base-32): matched as needles of the characters a held value fixes at
//     each byte alignment (alphabetCores).
//   - whole-number radix encodings (base36, base58, base62): matched as the
//     whole value's encoding (radixNeedles).

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const byteEscapeSep = `[ \t,:;|.\-]`

// byteRuns are the run patterns of one byte-escape reading: hexEscapeRun is a
// run of introduced hex bytes, sepHexRun bare hex pairs with a separator
// between each, decRun separated numbers of one to three digits (decimal or,
// all of three octal digits, octal byte values), octEsc octal escapes, binRun
// binary octets. min is the shortest run (in bytes) read.
type byteRuns struct {
	min                                     int
	hexEscapeRun, sepHexRun, decRun, octEsc *regexp.Regexp
	binRun                                  *regexp.Regexp
}

func newByteRuns(min int) *byteRuns {
	more := strconv.Itoa(min - 1)
	return &byteRuns{
		min:          min,
		hexEscapeRun: regexp.MustCompile(`(?i)(?:(?:\\x|\\u00|\\u\{0*|%u00|&#x0*|0x|u\+00|=|%|#)[0-9a-f]{2}[;}]?` + byteEscapeSep + `{0,2}){` + strconv.Itoa(min) + `,}`),
		sepHexRun:    regexp.MustCompile(`(?i)(?:^|[^0-9a-z])([0-9a-f]{2}(?:` + byteEscapeSep + `{1,2}[0-9a-f]{2}){` + more + `,})(?:$|[^0-9a-z])`),
		decRun:       regexp.MustCompile(`(?:^|[^0-9])(\d{1,3}(?:` + byteEscapeSep + `{1,3}\d{1,3}){` + more + `,})(?:$|[^0-9])`),
		octEsc:       regexp.MustCompile(`(?:\\[0-7]{1,3}){` + strconv.Itoa(min) + `,}`),
		binRun:       regexp.MustCompile(`(?:^|[^01])([01]{8}(?:` + byteEscapeSep + `{0,2}[01]{8}){` + more + `,})(?:$|[^01])`),
	}
}

var (
	// longRuns read runs of four bytes or more (a held value of shortMinLen
	// characters or more lies in such a run); shortRuns read single bytes, for
	// a held value shorter than that (Codex r1 appdirect class short credential
	// in output). shortRuns only run while such a value is held.
	longRuns     = newByteRuns(shortMinLen)
	shortRuns    = newByteRuns(1)
	hexEscapeTok = regexp.MustCompile(`(?i)(?:\\x|\\u00|\\u\{0*|%u00|&#x0*|0x|u\+00|=|%|#)([0-9a-f]{2})`)
	hexPair      = regexp.MustCompile(`(?i)[0-9a-f]{2}`)
	decTok       = regexp.MustCompile(`\d+`)
	octTok       = regexp.MustCompile(`\\([0-7]{1,3})`)
	binTok       = regexp.MustCompile(`[01]{8}`)
	qpSoftNL     = strings.NewReplacer("=\r\n", "", "=\n", "")
)

// byteEscapeViews returns, for each per-byte numeric encoding run in text,
// the text with that run replaced by its bytes and the bytes alone; nil when
// there is none.
func byteEscapeViews(text string) []string { return byteEscapeViewsOf(text, longRuns) }

func byteEscapeViewsOf(text string, runs *byteRuns) []string {
	var views []string
	add := func(start, end int, decoded []byte) {
		if len(decoded) < runs.min {
			return
		}
		views = append(views, string(decoded))
		if runs.min >= shortMinLen {
			// The text with the run decoded in place (a value split between
			// the run and its neighbours). Runs of a single byte are read
			// alone: one copy of the text per number would be quadratic.
			views = append(views, text[:start]+string(decoded)+text[end:])
		}
	}
	if strings.Contains(text, "=\n") || strings.Contains(text, "=\r\n") {
		if s := qpSoftNL.Replace(text); s != text {
			views = append(views, s)
			for _, v := range byteEscapeViewsOf(s, runs) {
				views = append(views, v)
			}
		}
	}
	for _, m := range runs.hexEscapeRun.FindAllStringIndex(text, -1) {
		var b []byte
		for _, tok := range hexEscapeTok.FindAllStringSubmatch(text[m[0]:m[1]], -1) {
			n, _ := strconv.ParseUint(tok[1], 16, 8)
			b = append(b, byte(n))
		}
		add(m[0], m[1], b)
	}
	for _, m := range runs.sepHexRun.FindAllStringSubmatchIndex(text, -1) {
		var b []byte
		for _, p := range hexPair.FindAllString(text[m[2]:m[3]], -1) {
			n, _ := strconv.ParseUint(p, 16, 8)
			b = append(b, byte(n))
		}
		add(m[2], m[3], b)
	}
	for _, m := range runs.decRun.FindAllStringSubmatchIndex(text, -1) {
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
	for _, m := range runs.octEsc.FindAllStringIndex(text, -1) {
		var b []byte
		for _, tok := range octTok.FindAllStringSubmatch(text[m[0]:m[1]], -1) {
			if n, err := strconv.ParseUint(tok[1], 8, 8); err == nil {
				b = append(b, byte(n))
			}
		}
		add(m[0], m[1], b)
	}
	for _, m := range runs.binRun.FindAllStringSubmatchIndex(text, -1) {
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
	short := false
	for _, h := range held {
		if len(h) < shortMinLen {
			short = true // a very short value is read in runs of any length too
			break
		}
	}
	if !short && !mayHoldByteEscapes(text) {
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
		views := byteEscapeViewsOf(base, longRuns)
		if short {
			views = append(views, byteEscapeViewsOf(base, shortRuns)...)
		}
		for _, v := range views {
			if seen[v] {
				continue
			}
			seen[v] = true
			if windowedEchoes(v, held) {
				return true
			}
			for _, h := range held {
				if len(h) < heldMatchFloor && strings.Contains(v, h) {
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

// mayHoldByteEscapes is a cheap pre-check that never skips a run the
// decoders read: the shortest (shortMinLen one-digit decimal or octal tokens)
// holds shortMinLen digits, and a hex run holds hex letters too (Codex r1
// 4.34.2 class secret-in-encoded-output: the check counted eight decimal
// digits, so "7a:7a:7a:7a" was never decoded).
func mayHoldByteEscapes(text string) bool {
	n := 0
	for i := 0; i < len(text); i++ {
		if isHexDigit(text[i]) {
			if n++; n >= shortMinLen {
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
