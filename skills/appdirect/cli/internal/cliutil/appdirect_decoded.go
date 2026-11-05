// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// appdirect handfix (every-text-encoding-withheld; Codex r1 4.34.2 class
// secret-in-encoded-output, closed by decoding instead of by enumerated
// needles). Three readings of the text, each compared with the held values
// after decoding, so a form is caught by what it decodes to rather than by a
// list of spellings:
//
//   - byte tokens (byteTokenEchoes): every run of hex digits in the text is
//     one token, read as a byte in decimal, hex, octal and binary, whatever
//     separates the runs (any run of other characters: commas, spaces,
//     slashes, underscores, newlines, the \x, 0x, U+, &#x, %u and = escape
//     introducers). A held value whose bytes are spelled by consecutive tokens
//     is matched. The enumerated byte-list separators ([ \t,:;|.-], one to
//     three of them) and the eight-decimal-digit pre-check missed "7a:7a:7a:7a"
//     and any list with a wider gap.
//   - short values in every alphabet (shortAlphabetEchoes): a held value under
//     heldMatchFloor has too few characters fixed in a 5- or 6-bit or radix
//     encoding for a needle, so every run of each alphabet's characters is
//     decoded at all eight bit alignments (any starting point in a longer
//     stream), base85 runs at all five group alignments, and radix runs as one
//     number, and the bytes are searched for the value.
//   - numbers (numericEchoes): a held number is matched as the value it is,
//     however it is written: an exponent, a fraction of zeros, thousands
//     separators, hex, octal and binary literals.
//
// They run on the text and on every view the shared funnel decodes from it
// (funnelViews), under the funnel's work budget; a budget that runs out is
// treated as a match (fail closed), as the funnel does.

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// echoesForms is HeldEchoes over an explicit list of held forms: every
// matcher the output chokepoint runs.
func echoesForms(text string, held []string) bool {
	if len(held) == 0 || text == "" {
		return false
	}
	// decodedEchoes: byte tokens, short values in every alphabet, numbers
	// (this file); scatteredEchoes: units in any order, gaps between
	// (appdirect_scattered.go).
	return windowedEchoes(text, held) || joinedEchoes(text, held) || decodedEchoes(text, held) || shortEchoes(text, held) || byteEscapeEchoes(text, held) ||
		scatteredEchoes(text, held)
}

// EchoesValues reports whether text repeats any of values in any form the
// output chokepoint withholds a held value in: each value is expanded the way
// holdEncodedCores expands a held one (its scheme-less form, the funnel's
// needles, its derived parts and every encoding of each) without holding it.
func EchoesValues(text string, values []string) bool {
	seen := map[string]bool{}
	var forms []string
	add := func(f string) {
		if f = strings.TrimSpace(f); f != "" && !seen[f] {
			seen[f] = true
			forms = append(forms, f)
		}
	}
	for _, v := range values {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		base := append([]string{v, funnelSchemeless(v)}, funnelNeedles([]string{v})...)
		base = append(base, credentialParts(v)...)
		for _, f := range base {
			add(f)
			for _, core := range encodedCores(f) {
				add(core)
			}
		}
	}
	return echoesForms(text, forms)
}

// decodedEchoes runs the three decoding readings on text and its views.
func decodedEchoes(text string, held []string) bool {
	var short, numeric []string
	for _, h := range held {
		if len(h) < heldMatchFloor {
			short = append(short, h)
		}
		if numericHeld.MatchString(h) {
			numeric = append(numeric, h)
		}
	}
	views := []string{text}
	if len(short) > 0 || strings.ContainsAny(text, "%\\&") {
		var complete bool
		if views, complete = funnelViews(text); !complete {
			return true // the work budget ran out: fail closed, as the funnel does
		}
	}
	tokenHeld := make([]string, 0, len(held))
	for _, h := range held {
		if !isEncodedForm(h) {
			tokenHeld = append(tokenHeld, h)
		}
	}
	for _, v := range views {
		if byteTokenEchoes(v, tokenHeld) {
			return true
		}
		if len(short) > 0 && shortAlphabetEchoes(v, short) {
			return true
		}
		if len(numeric) > 0 && numericEchoes(v, numeric) {
			return true
		}
	}
	return false
}

// ---- byte tokens ----

type byteToken struct {
	vals [4]int16 // the byte this run can be: decimal, hex, octal, binary (-1: none)
}

func isHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// byteTokens splits text into its hex-digit runs and reads each as a byte.
// A "0" run followed by an x or o introducer ("0x41", "0o101") is the
// introducer, not a byte; a run starting "0b" followed only by binary digits
// is also read without the "0b".
func byteTokens(text string) []byteToken {
	var out []byteToken
	for i := 0; i < len(text); {
		if !isHexDigit(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && isHexDigit(text[j]) {
			j++
		}
		run := text[i:j]
		if run == "0" && j < len(text) && (text[j] == 'x' || text[j] == 'X' || text[j] == 'o' || text[j] == 'O') && j+1 < len(text) && isHexDigit(text[j+1]) {
			i = j
			continue
		}
		out = append(out, readByteToken(run))
		i = j
	}
	return out
}

func readByteToken(run string) byteToken {
	t := byteToken{vals: [4]int16{-1, -1, -1, -1}}
	if len(run) > 2 && (run[:2] == "0b" || run[:2] == "0B") && strings.Trim(run[2:], "01") == "" {
		if n, err := strconv.ParseUint(run[2:], 2, 16); err == nil && n <= 255 {
			t.vals[3] = int16(n)
		}
		return t
	}
	digits := strings.TrimLeft(run, "0")
	if digits == "" {
		digits = "0"
	}
	if len(digits) <= 4 {
		if n, err := strconv.ParseUint(digits, 10, 16); err == nil && n <= 255 {
			t.vals[0] = int16(n)
		}
		if n, err := strconv.ParseUint(digits, 16, 16); err == nil && n <= 255 {
			t.vals[1] = int16(n)
		}
		if n, err := strconv.ParseUint(digits, 8, 16); err == nil && n <= 255 {
			t.vals[2] = int16(n)
		}
	}
	if len(digits) <= 8 && strings.Trim(run, "01") == "" {
		if n, err := strconv.ParseUint(digits, 2, 16); err == nil && n <= 255 {
			t.vals[3] = int16(n)
		}
	}
	return t
}

func (t byteToken) can(b byte) bool {
	for _, v := range t.vals {
		if v == int16(b) {
			return true
		}
	}
	return false
}

// byteTokenEchoes reports a held value spelled byte by byte by consecutive
// tokens of text.
func byteTokenEchoes(text string, held []string) bool {
	if len(held) == 0 {
		return false
	}
	toks := byteTokens(text)
	if len(toks) == 0 {
		return false
	}
	var at [256][]int32
	for i, t := range toks {
		for _, v := range t.vals {
			if v >= 0 && (len(at[v]) == 0 || at[v][len(at[v])-1] != int32(i)) {
				at[v] = append(at[v], int32(i))
			}
		}
	}
	for _, h := range held {
		if h == "" || len(h) > len(toks) {
			continue
		}
	next:
		for _, i := range at[h[0]] {
			if int(i)+len(h) > len(toks) {
				break
			}
			for k := 1; k < len(h); k++ {
				if !toks[int(i)+k].can(h[k]) {
					continue next
				}
			}
			return true
		}
	}
	return false
}

// ---- short values in every alphabet ----

type decodeAlphabet struct {
	bits int
	idx  [256]int16
}

func newDecodeAlphabet(bits int, alphabet string, fold bool, aliases map[byte]byte) *decodeAlphabet {
	a := &decodeAlphabet{bits: bits}
	for i := range a.idx {
		a.idx[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		c := alphabet[i]
		a.idx[c] = int16(i)
		if fold {
			a.idx[asciiFold(c, true)] = int16(i)
			a.idx[asciiFold(c, false)] = int16(i)
		}
	}
	for from, to := range aliases {
		a.idx[from] = a.idx[to]
		if fold {
			a.idx[asciiFold(from, true)] = a.idx[to]
		}
	}
	return a
}

func asciiFold(c byte, lower bool) byte {
	if lower && 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	if !lower && 'a' <= c && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// shortBitAlphabets are the bit-packed alphabets a short held value is
// decoded from: base64 (standard and URL), the 6-bit family the funnel does
// not decode (uuencode with either zero, xxencode, crypt, bcrypt), the base32
// family (standard, hex, Crockford with its I/L/O aliases, z-base-32) and hex.
var shortBitAlphabets = []*decodeAlphabet{
	newDecodeAlphabet(6, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", false, nil),
	newDecodeAlphabet(6, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", false, nil),
	newDecodeAlphabet(6, "`!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_", false, nil),
	newDecodeAlphabet(6, " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_", false, nil),
	newDecodeAlphabet(6, "+-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", false, nil),
	newDecodeAlphabet(6, "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", false, nil),
	newDecodeAlphabet(6, "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", false, nil),
	newDecodeAlphabet(5, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", true, nil),
	newDecodeAlphabet(5, "0123456789ABCDEFGHIJKLMNOPQRSTUV", true, nil),
	newDecodeAlphabet(5, "0123456789ABCDEFGHJKMNPQRSTVWXYZ", true, map[byte]byte{'I': '1', 'L': '1', 'O': '0'}),
	newDecodeAlphabet(5, "ybndrfg8ejkmcpqxot1uwisza345h769", false, nil),
	newDecodeAlphabet(4, "0123456789abcdef", true, nil),
}

// shortRadixAlphabets are read as one big-endian number per run.
var shortRadixAlphabets = func() []*decodeAlphabet {
	var out []*decodeAlphabet
	for i, a := range radixAlphabets {
		out = append(out, newDecodeAlphabet(0, a, i < 2, nil))
	}
	return out
}()

// shortBase85Alphabets are the radix-85 family, 5 characters per 4 bytes.
var shortBase85Alphabets = func() []*decodeAlphabet {
	var out []*decodeAlphabet
	for _, a := range base85Alphabets {
		out = append(out, newDecodeAlphabet(0, a, false, nil))
	}
	return out
}()

// maxRadixRun bounds the run a radix alphabet decodes as one number (the
// conversion is quadratic in its length); a longer run is treated as a match
// (fail closed), never skipped.
const maxRadixRun = 16384

func shortAlphabetEchoes(text string, short []string) bool {
	minLen := len(short[0])
	for _, h := range short {
		minLen = min(minLen, len(h))
	}
	found := newShortFinder(short).in
	for _, a := range shortBitAlphabets {
		for _, run := range alphabetRuns(text, a, (8*minLen+a.bits-1)/a.bits) {
			if bitAlignedEchoes(run, a, found) {
				return true
			}
		}
	}
	for _, a := range shortBase85Alphabets {
		for _, run := range alphabetRuns(text, a, minLen+1) {
			for off := 0; off < 5 && off < len(run); off++ {
				if found(base85Decode(run[off:], a)) {
					return true
				}
			}
		}
	}
	for _, a := range shortRadixAlphabets {
		// A run of fewer characters than minLen bytes need cannot hold one.
		for _, run := range alphabetRuns(text, a, radixMinRun(a, minLen)) {
			if len(run) > maxRadixRun {
				return true
			}
			if found(radixDecode(run, a)) {
				return true
			}
		}
	}
	return false
}

// alphabetRuns returns the maximal runs of text's characters in a, as
// symbol values, at least minRun long.
func alphabetRuns(text string, a *decodeAlphabet, minRun int) [][]byte {
	var out [][]byte
	var cur []byte
	flush := func() {
		if len(cur) >= minRun {
			out = append(out, cur)
		}
		cur = nil
	}
	for i := 0; i < len(text); i++ {
		if v := a.idx[text[i]]; v >= 0 && v < 256 {
			cur = append(cur, byte(v))
		} else {
			flush()
		}
	}
	flush()
	return out
}

// bitAlignedEchoes decodes the symbols of run (a.bits each) into bytes
// starting at each of the eight bit offsets, so a value that begins anywhere
// in a longer stream is read whole at one of them.
func bitAlignedEchoes(run []byte, a *decodeAlphabet, found func([]byte) bool) bool {
	total := len(run) * a.bits
	buf := make([]byte, 0, total/8+1)
	for off := 0; off < 8 && off < total; off++ {
		buf = buf[:0]
		var acc byte
		nbits := 0
		skip := off
		for _, s := range run {
			for b := a.bits - 1; b >= 0; b-- {
				if skip > 0 {
					skip--
					continue
				}
				acc = acc<<1 | (s>>uint(b))&1
				if nbits++; nbits == 8 {
					buf = append(buf, acc)
					acc, nbits = 0, 0
				}
			}
		}
		if found(buf) {
			return true
		}
	}
	return false
}

func base85Decode(run []byte, a *decodeAlphabet) []byte {
	var out []byte
	for i := 0; i+5 <= len(run); i += 5 {
		var v uint64
		for _, s := range run[i : i+5] {
			v = v*85 + uint64(s)
		}
		if v > 0xFFFFFFFF {
			continue
		}
		var be [8]byte
		binary.BigEndian.PutUint64(be[:], v)
		out = append(out, be[4:]...)
	}
	if rest := len(run) % 5; rest >= 2 {
		var v uint64
		for _, s := range run[len(run)-rest:] {
			v = v*85 + uint64(s)
		}
		for k := rest; k < 5; k++ {
			v = v*85 + 84
		}
		if v <= 0xFFFFFFFF {
			var be [8]byte
			binary.BigEndian.PutUint64(be[:], v)
			out = append(out, be[4:4+rest-1]...)
		}
	}
	return out
}

func radixBase(a *decodeAlphabet) int64 {
	base := int64(0)
	for _, v := range a.idx {
		base = max(base, int64(v)+1)
	}
	return base
}

// radixMinRun is the fewest characters of a's radix that carry n bytes.
func radixMinRun(a *decodeAlphabet, n int) int {
	base, k, capacity := radixBase(a), 0, 1.0
	for capacity < float64(uint64(1)<<min(uint(8*max(n-1, 0)), 62)) {
		capacity *= float64(base)
		k++
	}
	return max(k, 1)
}

func radixDecode(run []byte, a *decodeAlphabet) []byte {
	n := new(big.Int)
	b := big.NewInt(radixBase(a))
	d := new(big.Int)
	for _, s := range run {
		n.Mul(n, b)
		n.Add(n, d.SetInt64(int64(s)))
	}
	return n.Bytes()
}

// shortFinder finds any of a set of short byte strings in one pass: values
// are indexed by their first two bytes (one-byte values by their byte).
type shortFinder struct {
	one   [256]bool
	two   map[uint16][][]byte
	empty bool
}

func newShortFinder(values []string) *shortFinder {
	f := &shortFinder{two: map[uint16][][]byte{}, empty: true}
	for _, v := range values {
		switch {
		case v == "":
		case len(v) == 1:
			f.one[v[0]] = true
			f.empty = false
		default:
			k := uint16(v[0])<<8 | uint16(v[1])
			f.two[k] = append(f.two[k], []byte(v))
			f.empty = false
		}
	}
	return f
}

func (f *shortFinder) in(b []byte) bool {
	if f.empty {
		return false
	}
	for i := 0; i < len(b); i++ {
		if f.one[b[i]] {
			return true
		}
		if i+1 >= len(b) {
			break
		}
		for _, v := range f.two[uint16(b[i])<<8|uint16(b[i+1])] {
			if bytes.HasPrefix(b[i:], v) {
				return true
			}
		}
	}
	return false
}

// ---- numbers ----

// numericHeld is a held value that is a number.
var numericHeld = regexp.MustCompile(`^[+-]?\d+(\.\d+)?$`)

// numberLiteral is a number however it is written: a hex, octal or binary
// literal, or a decimal with thousands separators, a fraction and an exponent.
var numberLiteral = regexp.MustCompile(`[+-]?(?:0[xX][0-9a-fA-F_]+|0[oO][0-7_]+|0[bB][01_]+|\d[\d,_']*(?:\.\d*)?(?:[eE][+-]?\d+)?|\.\d+(?:[eE][+-]?\d+)?)`)

// numericEchoes reports a held number written in text as the same value.
func numericEchoes(text string, numeric []string) bool {
	targets := make([]*big.Rat, 0, len(numeric))
	for _, h := range numeric {
		if r, ok := new(big.Rat).SetString(h); ok {
			targets = append(targets, r.Abs(r))
		}
	}
	if len(targets) == 0 {
		return false
	}
	for _, lit := range numberLiteral.FindAllString(text, -1) {
		r, ok := parseNumberLiteral(lit)
		if !ok {
			continue
		}
		r.Abs(r)
		for _, t := range targets {
			if r.Cmp(t) == 0 {
				return true
			}
		}
	}
	return false
}

func parseNumberLiteral(lit string) (*big.Rat, bool) {
	s := strings.TrimLeft(lit, "+-")
	s = strings.NewReplacer(",", "", "_", "", "'", "").Replace(s)
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			n, ok := new(big.Int).SetString(s[2:], base)
			if !ok {
				return nil, false
			}
			return new(big.Rat).SetInt(n), true
		}
	}
	if strings.HasSuffix(s, ".") {
		s = strings.TrimSuffix(s, ".")
	}
	if len(s) > 400 {
		return nil, false // an exponent this long is not a credential's spelling
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		if exp, err := strconv.Atoi(s[i+1:]); err != nil || exp > 400 || exp < -400 {
			return nil, false
		}
	}
	return new(big.Rat).SetString(s)
}
