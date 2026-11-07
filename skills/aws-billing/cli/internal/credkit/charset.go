// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The characters a charset reading keeps, and the tables that read them.
// Text in any charset that carries a held value, or a text encoding of one,
// is made of relevant characters: printable ASCII and whitespace, a held
// value's own characters (a control character included, and their
// canonical components), and every
// character whose normalization contains one of those (a fullwidth form,
// "Ⅷ", "ß"). A reading keeps the runs of relevant characters and drops the
// rest, which can carry neither.

import (
	"bytes"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// compat maps each character from U+0080 up that normalization changes to
// what it becomes.
var compat = sync.OnceValue(func() map[rune]string { return compatIn(compatRanges) })

// compatRanges is where normalization changes any character: the BMP, the
// SMP and the CJK compatibility ideographs supplement (the rest is
// ideographs, tags, private use and unassigned; TestCompatRanges proves it
// against every code point for the Unicode tables this kit is built with).
// ponytail: scanning every code point costs 65 ms on a check's first use.
var compatRanges = [][2]rune{{0x80, 0x1FFFF}, {0x2F800, 0x2FA1F}}

func compatIn(ranges [][2]rune) map[rune]string {
	out := map[rune]string{}
	for _, rg := range ranges {
		for r := rg[0]; r <= rg[1]; r++ {
			if !utf8.ValidRune(r) || inert(r) {
				continue
			}
			s := string(r)
			if n := appendNormSegment(nil, []byte(s)); string(n) != s {
				out[r] = string(n)
			}
		}
	}
	return out
}

func asciiRelevant(c byte) bool { return c >= 0x20 && c < 0x7f || isSpaceByte(c) }

// asciiCompat is every character whose normalization holds a relevant
// ASCII character: relevant for every held set.
var asciiCompat = sync.OnceValue(func() []rune {
	var out []rune
	for r, n := range compat() {
		for i := 0; i < len(n); i++ {
			if asciiRelevant(n[i]) {
				out = append(out, r)
				break
			}
		}
	}
	slices.Sort(out)
	return out
})

type relevance struct {
	ascii  [2]uint64       // U+0000..U+007F
	bmp    [1 << 10]uint64 // U+0080..U+FFFF
	astral map[rune]bool
	high   []rune // every relevant character from U+0080 up, sorted
}

func (r *relevance) has(c rune) bool {
	switch {
	case c < 0:
		return false
	case c < 0x80:
		return r.asc(byte(c))
	case c < 0x10000:
		return r.bmp[c>>6]&(1<<(c&63)) != 0
	}
	return r.astral[c]
}

// asc reports whether ASCII character c is relevant: printable, whitespace,
// or any held value's own (a control character included).
func (r *relevance) asc(c byte) bool { return c < 0x80 && r.ascii[c>>6]&(1<<(c&63)) != 0 }

func (r *relevance) add(c rune) {
	if c < 0x80 || r.has(c) {
		return
	}
	if c < 0x10000 {
		r.bmp[c>>6] |= 1 << (c & 63)
	} else {
		r.astral[c] = true
	}
	r.high = append(r.high, c)
}

func newRelevance(norms [][]byte) relevance {
	r := relevance{astral: map[rune]bool{}}
	for c := byte(0); c < 0x80; c++ {
		if asciiRelevant(c) {
			r.ascii[c>>6] |= 1 << (c & 63)
		}
	}
	held := map[rune]bool{}
	for _, n := range norms {
		for _, c := range string(n) {
			switch {
			case c < 0x80:
				r.ascii[c>>6] |= 1 << (c & 63) // a held control character
			case c != utf8.RuneError:
				held[c] = true
			}
		}
	}
	for _, c := range asciiCompat() {
		r.add(c)
	}
	for c := range held {
		r.add(c)
		for _, d := range norm.NFD.String(string(c)) {
			r.add(d)
		}
	}
	if len(held) > 0 {
		for c, n := range compat() {
			for _, d := range n {
				if held[d] {
					r.add(c)
					break
				}
			}
		}
	}
	slices.Sort(r.high)
	return r
}

// runs calls fn with every maximal run of relevant characters in b, read
// as UTF-8 (an invalid byte ends a run).
func (r *relevance) runs(b []byte, fn func([]byte)) {
	start := -1
	for i := 0; i < len(b); {
		c := b[i]
		sz := 1
		ok := false
		if c < utf8.RuneSelf {
			ok = r.asc(c)
		} else {
			var cr rune
			cr, sz = utf8.DecodeRune(b[i:])
			ok = !(cr == utf8.RuneError && sz == 1) && r.has(cr)
		}
		if ok {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			fn(b[start:i])
			start = -1
		}
		i += sz
	}
	if start >= 0 {
		fn(b[start:])
	}
}

// byteTables is every single-byte charset's character for each byte (-1
// for none).
var byteTables = sync.OnceValue(func() [][256]rune {
	var out [][256]rune
	for _, e := range charmap.All {
		var t [256]rune
		dec := e.NewDecoder()
		for i := 0; i < 256; i++ {
			d, _, err := transform.Bytes(dec, []byte{byte(i)})
			r, n := utf8.DecodeRune(d)
			if err != nil || n != len(d) || r == utf8.RuneError {
				r = -1
			}
			t[i] = r
		}
		out = append(out, t)
	}
	return out
})

// tableGroup is the single-byte charsets that read the same relevant
// character from each byte. A byte is special when its character is not
// itself as ASCII: a reading with no special byte is the bytes' own ASCII,
// which the unit as written already carries.
type tableGroup struct {
	char      [256]rune // relevant character, or -1
	special   [4]uint64
	asciiSafe bool // reads relevant ASCII as itself: special bytes are all above 0x7F
	// For an ASCII-safe group: each byte's character in NFKC, whether
	// nothing before it combines with it (a normalization boundary), and an
	// id equal across groups exactly when the byte reads the same in NFKC
	// beside any neighbours (-1 for no relevant character).
	form [256]string
	bb   [256]bool
	fid  [256]int32
}

func (g *tableGroup) isSpecial(c byte) bool { return g.special[c>>6]&(1<<(c&63)) != 0 }

func groupTables(r *relevance) []tableGroup {
	var out []tableGroup
	seen := map[[256]rune]bool{}
	for _, t := range byteTables() {
		var g tableGroup
		for i, c := range t {
			if c < 0 || !r.has(c) {
				c = -1
			}
			g.char[i] = c
			if c >= 0 && (i >= 0x80 || c != rune(i)) { // a byte above 0x7F is no character on its own
				g.special[i>>6] |= 1 << (i & 63)
			}
		}
		if g.special == [4]uint64{} || seen[g.char] {
			continue
		}
		g.asciiSafe = true
		for i := 0; i < 0x80; i++ {
			if g.char[i] != rune(i) && r.asc(byte(i)) {
				g.asciiSafe = false
			}
		}
		seen[g.char] = true
		out = append(out, g)
	}
	ids := map[string]int32{}
	intern := func(k string) int32 {
		if id, ok := ids[k]; ok {
			return id
		}
		ids[k] = i32(len(ids))
		return ids[k]
	}
	for gi := range out {
		g := &out[gi]
		for i, c := range g.char {
			g.fid[i] = -1
			if c < 0 || !g.asciiSafe {
				continue
			}
			g.form[i] = norm.NFKC.String(string(c))
			g.bb[i] = norm.NFKC.PropertiesString(string(c)).BoundaryBefore()
			if g.bb[i] {
				g.fid[i] = intern("f" + g.form[i])
			} else {
				g.fid[i] = intern("c" + string(c))
			}
		}
	}
	return out
}

// multiIndex reads the multi-byte charsets for relevant characters only:
// every byte sequence the decoder reads as characters other than ASCII,
// all of them relevant, found by decoding (multiSpellings), so a sequence
// the decoder accepts and the encoder never writes is found too, and so is
// one it reads as two characters.
type multiIndex struct {
	encs  []encoding.Encoding
	pair  [1 << 16]uint8 // bit e: (lead, trail) reads as relevant characters in encs[e]
	lead  [256]uint8     // bit e: a 1-, 3- or 4-byte sequence of encs[e] starts with this byte
	chars []map[string]string
}

var multiEncodings = []encoding.Encoding{
	japanese.ShiftJIS, japanese.EUCJP, korean.EUCKR,
	simplifiedchinese.GBK, simplifiedchinese.GB18030, traditionalchinese.Big5,
}

// multiSpellings is, per multi-byte charset, every sequence its decoder
// reads as characters with none of them ASCII, by what it reads: each 1-
// and 2-byte sequence from a byte above 0x7F, EUC-JP's 3-byte ones, and
// GB18030's 4-byte ones in the BMP, each decoded. (GB18030's 4-byte
// sequences above the BMP are one per character, the encoder's.)
var multiSpellings = sync.OnceValue(func() []map[string][]string {
	out := make([]map[string][]string, len(multiEncodings))
	for e, enc := range multiEncodings {
		m := map[string][]string{}
		dec := enc.NewDecoder()
		// One decoding per prefix: each candidate sequence, then a line
		// break that no sequence here contains, so the reading of each is
		// the text between two breaks.
		read := func(prefix []byte, from, to int) {
			src := make([]byte, 0, (to-from)*(len(prefix)+2))
			for c := from; c < to; c++ {
				if c == '\n' {
					continue
				}
				src = append(append(append(src, prefix...), byte(c)), '\n')
			}
			dst, _, _ := transform.Bytes(dec, src)
			seqs := bytes.Split(src, []byte{'\n'})
			for i, t := range bytes.Split(dst, []byte{'\n'}) {
				if i < len(seqs) && len(t) > 0 && utf8.Valid(t) &&
					!bytes.ContainsFunc(t, func(r rune) bool { return r < 0x80 || r == utf8.RuneError }) {
					m[string(t)] = append(m[string(t)], string(seqs[i]))
				}
			}
		}
		read(nil, 0x80, 0x100)
		for lead := 0x80; lead < 0x100; lead++ {
			read([]byte{byte(lead)}, 0, 0x100)
		}
		if enc == japanese.EUCJP {
			for b := 0xA1; b < 0xFF; b++ {
				read([]byte{0x8F, byte(b)}, 0xA1, 0xFF)
			}
		}
		if enc == simplifiedchinese.GB18030 {
			// The decoder reads a second byte up to 0x3F (gbk.go), so a
			// BMP character has spellings the encoder never writes.
			for b1 := 0x81; b1 <= 0x84; b1++ {
				for b2 := 0x30; b2 <= 0x3F; b2++ {
					for b3 := 0x81; b3 <= 0xFE; b3++ {
						read([]byte{byte(b1), byte(b2), byte(b3)}, 0x30, 0x3A)
					}
				}
			}
		}
		out[e] = m
	}
	return out
})

func newMultiIndex(r *relevance) *multiIndex {
	x := &multiIndex{encs: multiEncodings}
	all := multiSpellings()
	for e, enc := range x.encs {
		chars := map[string]string{}
		add := func(b, s string) {
			chars[b] = s
			if len(b) == 2 {
				x.pair[int(b[0])<<8|int(b[1])] |= 1 << e
			} else {
				x.lead[b[0]] |= 1 << e
			}
		}
		for s, seqs := range all[e] {
			if !strings.ContainsFunc(s, func(c rune) bool { return !r.has(c) }) {
				for _, b := range seqs {
					add(b, s)
				}
			}
		}
		en := enc.NewEncoder()
		for _, c := range r.high {
			if b, err := en.Bytes([]byte(string(c))); err == nil && len(b) > 0 && len(b) <= 4 && b[0] >= 0x80 { // exact: no encoder here writes a character in more than 4 bytes
				add(string(b), string(c))
				if len(b) == 4 && enc == simplifiedchinese.GB18030 {
					for _, a := range gb18030Aliases(b) {
						add(a, string(c))
					}
				}
			}
		}
		x.chars = append(x.chars, chars)
	}
	return x
}

// gb18030Aliases is every other 4-byte spelling GB18030's decoder reads
// as b does: its first two bytes count (b1-0x81)*10 + (b2-0x30), with b2
// up to 0x3F (gbk.go).
func gb18030Aliases(b []byte) []string {
	k := (int(b[0])-0x81)*10 + int(b[1]) - 0x30
	var out []string
	for b2 := 0; b2 < 16; b2++ {
		if b1 := (k - b2) / 10; (k-b2)%10 == 0 && b1 >= 0 && b1 <= 0xFE-0x81 && b2 != int(b[1])-0x30 {
			out = append(out, string([]byte{byte(0x81 + b1), byte(0x30 + b2), b[2], b[3]}))
		}
	}
	return out
}

// at reads the relevant characters encs[e] decodes at b[0], if any.
func (x *multiIndex) at(e int, b []byte) (string, int) {
	if len(b) >= 2 && x.pair[int(b[0])<<8|int(b[1])]&(1<<e) != 0 { // exact: b[:2] exists
		return x.chars[e][string(b[:2])], 2
	}
	if x.lead[b[0]]&(1<<e) != 0 {
		for _, n := range []int{1, 3, 4} {
			if len(b) >= n {
				if c, ok := x.chars[e][string(b[:n])]; ok {
					return c, n
				}
			}
		}
	}
	return "", 0
}
