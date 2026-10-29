// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand/v2"
	"mime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

// The credential most tests hold, and a different value of the same shape
// that must never match it.
const (
	cred  = "Sk3tch-Pa55word-QX7"
	other = "Zq9-different-VALUE"
)

// holding resets the held set to vals under source "test".
func holding(t *testing.T, vals ...string) {
	t.Helper()
	resetForTest(Declarations{})
	Hold("test", vals...)
}

func leaks(unit string) bool { return withheld([]byte(unit)) }

func TestContract2aWholeValue(t *testing.T) {
	holding(t, cred)
	for _, u := range []string{cred, "prefix" + cred + "suffix", `{"token":"` + cred + `"}`, "x" + cred} {
		if !leaks(u) {
			t.Errorf("whole value of 8+ characters anywhere: %q not withheld", u)
		}
	}
	if leaks("nothing held appears here") {
		t.Error("clean unit withheld")
	}
}

func TestContract2aShortValueWholeTokenOnly(t *testing.T) {
	holding(t, "abc")
	// "fabc": UTF-8 read from the offset of the a is "abc", whole (kit5rv).
	for _, u := range []string{"abc", "the abc token", `{"k":"abc"}`, "abc\n", "key=abc;", "fabc"} {
		if !leaks(u) {
			t.Errorf("short value as a whole token: %q not withheld", u)
		}
	}
	// The over-withholding guard: a 3-character credential inside a longer
	// word, a letter or digit after it, is not withheld.
	for _, u := range []string{"xabcx", "abcdef", "abc1", "Fabcd and labcx"} {
		if leaks(u) {
			t.Errorf("short value inside a longer word: %q withheld", u)
		}
	}
}

func TestContract2aShortNumberCounts(t *testing.T) {
	holding(t, "4711")
	if !leaks("pin: 4711") {
		t.Error("contract 1 numbers count: a short numeric credential as a token not withheld")
	}
	if leaks("id 47110") {
		t.Error("short numeric credential inside a longer number withheld")
	}
}

func TestContract2bPiece(t *testing.T) {
	holding(t, cred)
	if !leaks("see Pa55word here") {
		t.Error("8-character piece not withheld")
	}
	if !leaks("tail: rd-QX7 and Sk3tch-P") {
		t.Error("8-character piece at the value's start not withheld")
	}
	if leaks("see Pa55wor here; and Sk3tch- too") {
		t.Error("7-character pieces withheld (contract 6.6: advisory only)")
	}
}

func TestContract2CharacterIsCodePoint(t *testing.T) {
	holding(t, "αβγδεζηθικλμ")
	if !leaks("x αβγδεζηθ x") {
		t.Error("8 code points (16 bytes) not withheld")
	}
	if leaks("x αβγδεζη x") {
		t.Error("7 code points withheld")
	}
}

func TestContract2CharacterIsByteForInvalidUTF8(t *testing.T) {
	v := "\xff\xfe\xfd\xfc\xfb\xfa\xf9\xf8\xf7\xf6"
	holding(t, v)
	if !leaks("a" + v[1:9] + "b") {
		t.Error("8-byte piece of a value that is not UTF-8 not withheld")
	}
	if leaks("a" + v[1:8] + "b") {
		t.Error("7-byte piece withheld")
	}
}

func TestContract3NFKCAndCaseFolding(t *testing.T) {
	holding(t, cred)
	var full strings.Builder
	for _, r := range cred {
		full.WriteRune(r - 0x21 + 0xFF01)
	}
	for _, u := range []string{strings.ToUpper(cred), strings.ToLower(cred), full.String()} {
		if !leaks(u) {
			t.Errorf("NFKC or case-folded form %q not withheld", u)
		}
	}
	holding(t, strings.ToUpper(cred))
	if !leaks(strings.ToLower(cred)) {
		t.Error("folding is not applied to the held side")
	}
}

func TestContract3Numbers(t *testing.T) {
	holding(t, "0123456789012")
	for _, u := range []string{"123456789012", "000123456789012", "123,456,789,012", "123_456_789_012", "123 456 789 012",
		"1.23456789012e11", "1.23456789012E+11", "12345678901.2e1"} {
		if !leaks("n=" + u + ";") {
			t.Errorf("integer spelling %q of a digit-only credential not withheld", u)
		}
	}
	if leaks("n=9.87654321098e11;") {
		t.Error("a different number withheld")
	}
}

// encodings is every contract 3 decoder applied to v, by name.
func encodings(v string) map[string]string {
	b := []byte(v)
	e := map[string]string{}
	for i, pre := range []string{"", "x", "xy"} {
		in := []byte(pre + v + "tail")
		e[fmt.Sprintf("base64 std align %d", i)] = "AAAA" + base64.StdEncoding.EncodeToString(in) + "AAAA"
		e[fmt.Sprintf("base64 url raw align %d", i)] = base64.RawURLEncoding.EncodeToString(in)
	}
	e["base64 padded"] = base64.StdEncoding.EncodeToString(b)
	e["base64 wrapped"] = wrap(base64.StdEncoding.EncodeToString([]byte(strings.Repeat(v, 4))), 20)
	for i, pre := range []string{"", "x", "xyz", "wxyz1"} {
		e[fmt.Sprintf("base32 std align %d", i)] = base32.StdEncoding.EncodeToString([]byte(pre + v))
		e[fmt.Sprintf("base32 hex lower unpadded align %d", i)] = strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(pre + v)))
	}
	e["hex lower"] = hex.EncodeToString(b)
	e["hex upper"] = strings.ToUpper(hex.EncodeToString(b))
	e["hex colon"] = sepHex(b, ":")
	e["hex space"] = sepHex(b, " ")
	e["hex odd alignment"] = "f" + hex.EncodeToString(b)
	e["percent"] = pctAll(b)
	e["percent repeated"] = strings.ReplaceAll(pctAll(b), "%", "%25")
	e["quoted-printable"] = qpAll(b)
	var ent strings.Builder
	for i, r := range v {
		if i%2 == 0 {
			fmt.Fprintf(&ent, "&#%d;", r)
		} else {
			fmt.Fprintf(&ent, "&#x%X;", r)
		}
	}
	e["entities"] = ent.String()
	var uesc, xesc, oesc strings.Builder
	for _, c := range b {
		fmt.Fprintf(&uesc, `\u%04x`, c)
		fmt.Fprintf(&xesc, `\x%02x`, c)
		fmt.Fprintf(&oesc, `\%03o`, c)
	}
	e["json unicode escapes"] = `"` + uesc.String() + `"`
	e["hex escapes"] = xesc.String()
	e["octal escapes"] = oesc.String()
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	w.Write(b)
	w.Close()
	e["ascii85"] = a85.String()
	e["ascii85 wrapped"] = "<~" + a85.String() + "~>"
	pad := []byte(v)
	for len(pad)%4 != 0 {
		pad = append(pad, '.')
	}
	e["z85"] = base85(z85Alphabet, pad)
	e["rfc1924"] = base85(rfc1924Alphabet, pad)
	e["rot13"] = rot13(v)
	le, be := utf16Bytes(v, false), utf16Bytes(v, true)
	e["utf-16le"] = string(le)
	e["utf-16be"] = string(be)
	e["utf-16le bom"] = "\xff\xfe" + string(le)
	e["utf-16be bom"] = "\xfe\xff" + string(be)
	e["mime b"] = "Subject: " + mime.BEncoding.Encode("utf-8", v)
	e["mime q"] = "Subject: " + mime.QEncoding.Encode("utf-8", v)
	ebc, _ := charmap.CodePage037.NewEncoder().String(v)
	e["ebcdic charset"] = ebc
	e["gzip"] = string(gz(b)) + "TRAILING"
	e["zlib"] = string(zl(b)) + "TRAILING"
	e["raw deflate"] = string(df(b)) + "TRAILING"
	var dec, hx []string
	for _, r := range v {
		dec = append(dec, fmt.Sprint(int(r)))
		hx = append(hx, fmt.Sprintf("0x%x", r))
	}
	e["decimal code list"] = strings.Join(dec, ", ")
	e["hex code list"] = strings.Join(hx, " ")
	e["chain base64 of gzip"] = base64.StdEncoding.EncodeToString(gz(b))
	e["chain hex of base64"] = hex.EncodeToString([]byte(base64.StdEncoding.EncodeToString(b)))
	e["chain percent of json escapes"] = pctAll([]byte(uesc.String()))
	return e
}

func TestContract3Decoders(t *testing.T) {
	holding(t, cred)
	for name, enc := range encodings(cred) {
		if !leaks(enc) {
			t.Errorf("decoder %s: encoded credential not withheld: %q", name, enc)
		}
	}
	// Negative: the same encodings of a value that is not held.
	for name, enc := range encodings(other) {
		if leaks(enc) {
			t.Errorf("decoder %s: encoding of an unheld value withheld: %q", name, enc)
		}
	}
}

func TestContract3NonASCIICharsets(t *testing.T) {
	v := "パスワード秘密の鍵です"
	holding(t, v)
	sj, err := japanese.ShiftJIS.NewEncoder().String(v)
	if err != nil {
		t.Fatal(err)
	}
	if !leaks("data: " + sj) {
		t.Error("Shift-JIS credential not withheld")
	}
	if !leaks(mime.BEncoding.Encode("utf-8", v)) {
		t.Error("MIME encoded word with a non-ASCII credential not withheld")
	}
	l1 := "pässwörd-ünïcode"
	holding(t, l1)
	enc, _ := charmap.ISO8859_1.NewEncoder().String(l1)
	if !leaks(enc) {
		t.Error("ISO-8859-1 credential not withheld")
	}
	if !leaks(mime.QEncoding.Encode("iso-8859-1", l1)) {
		t.Error("MIME Q word in ISO-8859-1 not withheld")
	}
}

func TestContract3SurrogatePairs(t *testing.T) {
	v := "pass😀word-123"
	holding(t, v)
	var b strings.Builder
	for _, u := range utf16.Encode([]rune(v)) {
		fmt.Fprintf(&b, `\u%04x`, u)
	}
	if !leaks(`{"t":"` + b.String() + `"}`) {
		t.Error("JSON escapes with a surrogate pair not withheld")
	}
}

func TestContract3JoinedValues(t *testing.T) {
	holding(t, cred)
	parts := []string{cred[:7], cred[7:14], cred[14:]}
	doc := fmt.Sprintf(`{"a":%q,"b":%q,"c":%q}`, parts[0], parts[1], parts[2])
	if !leaks(doc) {
		t.Error("credential split across adjacent JSON string values not withheld")
	}
	// Form bodies are out of scope since the amended contract (section 3,
	// joined values): JSON only.
	for _, p := range parts {
		if leaks(fmt.Sprintf(`{"a":%q}`, p)) {
			t.Errorf("a 7-character part alone withheld: %q", p)
		}
	}
}

func TestContract3BudgetExhaustionWithholds(t *testing.T) {
	holding(t, cred)
	defer func(d, p, b, u int) { maxDepth, budgetPerByte, budgetBase, maxUnit = d, p, b, u }(maxDepth, budgetPerByte, budgetBase, maxUnit)
	nested := other
	for range 4 {
		nested = base64.StdEncoding.EncodeToString([]byte(nested))
	}
	if leaks(nested) {
		t.Fatal("clean nested unit withheld with the full budget")
	}
	maxDepth = 2
	if !leaks(nested) {
		t.Error("depth limit ran out and the unit read as clean")
	}
	maxDepth = 16
	budgetPerByte, budgetBase = 1, 16
	if !leaks(nested) {
		t.Error("decoded-bytes limit ran out and the unit read as clean")
	}
	budgetPerByte, budgetBase = 256, 4<<20
	maxUnit = 8
	if !leaks("a clean unit longer than the cap") {
		t.Error("a unit over the size cap read as clean")
	}
}

func TestContract3DecoderErrorNeverClean(t *testing.T) {
	holding(t, cred)
	trunc := gz([]byte(cred + strings.Repeat(" padding", 40)))
	trunc = trunc[:len(trunc)-12] // the checksum and some data gone
	if !leaks(string(trunc)) {
		t.Error("a truncated gzip stream carrying the credential read as clean")
	}
	if !leaks("%ZZ%" + pctAll([]byte(cred)) + "%G") {
		t.Error("an invalid percent escape next to the encoded credential made the unit clean")
	}
	if !leaks("!!!" + base64.StdEncoding.EncodeToString([]byte(cred)) + "@@@=") {
		t.Error("base64 with junk around it read as clean")
	}
}

func TestContract1UnknownHeldSetWithholdsAll(t *testing.T) {
	holding(t, cred)
	markUnknown()
	if !leaks("a perfectly clean line") {
		t.Error("an unreadable credential source did not fail closed")
	}
}

func TestCleanOutputsStayClean(t *testing.T) {
	// The short value is a 4-digit PIN, the realistic short shape. A held
	// 3-letter value withholds the help text below since kit5rv: a decoded
	// view's ROT13 reads "nop" as "abc" at a reading's start, which the
	// left edge (match.go tokenAt) now counts, as contract 3 requires.
	holding(t, cred, "4711", "0123456789012")
	for _, u := range []string{
		`{"tickets":[{"id":12,"title":"Printer offline","state":"open","owner":"j.doe@example.com"}]}`,
		"ID  TITLE             STATE\n12  Printer offline   open\n",
		"Error: unknown flag: --limt\nRun 'zammad-cli --help' for usage.\n",
	} {
		if leaks(u) {
			t.Errorf("ordinary output withheld: %q", u)
		}
	}
}

func wrap(s string, n int) string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	return strings.Join(append(out, s), "\n")
}

func sepHex(b []byte, sep string) string {
	var p []string
	for _, c := range b {
		p = append(p, fmt.Sprintf("%02x", c))
	}
	return strings.Join(p, sep)
}

func pctAll(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		fmt.Fprintf(&s, "%%%02X", c)
	}
	return s.String()
}

func qpAll(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		fmt.Fprintf(&s, "=%02X", c)
	}
	return s.String()
}

func base85(alpha string, b []byte) string {
	var s strings.Builder
	for i := 0; i+4 <= len(b); i += 4 {
		v := new(big.Int).SetBytes(b[i : i+4]).Uint64()
		var g [5]byte
		for j := 4; j >= 0; j-- {
			g[j] = alpha[v%85]
			v /= 85
		}
		s.Write(g[:])
	}
	return s.String()
}

func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func utf16Bytes(s string, bigEndian bool) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func zl(b []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func df(b []byte) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

// compat scans only compatRanges: every code point outside them is left as
// it is by normalization, for the Unicode tables this kit is built with.
func TestCompatRanges(t *testing.T) {
	full := compatIn([][2]rune{{0x80, unicode.MaxRune}})
	if got := compat(); len(got) != len(full) {
		for r := range full {
			if _, ok := got[r]; !ok {
				t.Errorf("U+%04X normalizes to %q but lies outside compatRanges", r, full[r])
			}
		}
	}
}

// The fixed Huffman table reads every symbol the bit-at-a-time reader does.
func TestFixedSymTable(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	b := make([]byte, 64)
	for range 2000 {
		for i := range b {
			b[i] = byte(rnd.Uint32())
		}
		pos := rnd.IntN(len(b) * 8)
		x, y := bitReader{b: b, pos: pos}, bitReader{b: b, pos: pos}
		for x.pos < len(b)*8 {
			s1, ok1 := x.fixedSym()
			s2, ok2 := y.fixedSymBits()
			if s1 != s2 || ok1 != ok2 || ok1 && x.pos != y.pos {
				t.Fatalf("at bit %d: table %d %v pos %d, bits %d %v pos %d", pos, s1, ok1, x.pos, s2, ok2, y.pos)
			}
			if !ok1 {
				break
			}
		}
	}
}
