// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 3: the decoders. step (match.go) reads every part of a view with every
// decoder that applies and queues what they decode as at most two views at
// the next depth: byte parts, read by every decoder, and text readings (a
// charset's characters, ROT13, NFKC), read by the text decoders only. The
// matcher checks every part of every view, so decodings chain to any depth
// the budget allows. A decoder never decides a unit is clean: on bad input
// it queues what it could decode and the other paths go on.
//
// A part that reads as text as written (valid UTF-8, no control character
// but whitespace or escape) is decoded whole. Any other part is decoded as
// its runs of relevant characters (charset.go), each on its own; the
// whole-text readings (ROT13 and joined values) apply to whole text only.
//
// What a decoder produces that is not whole text (the decodings of runs:
// base-N alignments, a run's escapes) is collected into one part per view,
// the pieces separated by a NUL byte. A NUL ends every run, so each piece
// is read on its own by every run-based decoder; the charsets and
// decompressors read every offset and alignment, so they still read each
// piece at each of its own, and only add readings across the NUL.
//
// No decoding is dropped for its length: one byte can expand (NFKC, a
// decompressor) and decode on. Every limit on a path is in limits.go and
// spends the budget.

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"html"
	"io"
	"math"
	mbits "math/bits"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// sep separates the pieces of a view's joined part. 0xFF is not UTF-8,
// in no base-N alphabet, no multi-byte lead, an invalid deflate block type,
// and never half of a UTF-16 or UTF-32 character up to 0xFF but beside a
// NUL.
const sep = 0xFF

// Memo keys: a decoder reads a given input once per unit. Each queue that
// reads its input its own way (a short value anywhere, a text view, a final
// view) has a key of its own, so an input queued one way is never skipped
// when it comes again for another.
const (
	idRun uint64 = iota + 1
	idROT13
	idOut
	idRunOut
	idTextOut
	idCharOut
	idFinalOut
	idBase // + alphabet index
)

// decoding collects one view's decodings, by provenance: bk[n] is what
// decodes from inside n headerless raw deflate streams.
type decoding struct {
	s     *scan
	depth int
	any   bool // the view is run decoders' outputs (view.any)
	text  bool // the view is a text view (view.text)
	final bool // the view's decodings are matched, not decoded further (view.final)
	tail  bool // the view's parts end where the unit ends (view.tail)
	// onlyJoin sends what a substitution decoder makes (changed) to
	// joined values alone (a cut charset reading's whole text, wide).
	onlyJoin bool
	joinDeep int // onlyJoin's substitutions deep
	// tailEnd is the last byte of the part being read, when it ends where
	// the unit ends: a reading that reaches it ends there too.
	tailEnd *byte
	bk      [maxRawNest + 1]bucket
	f       int // the bucket outputs go to
	buf     []byte
	pos     []int32
	toks    []codeTok
	ents    []codeEnt
}

type bucket struct {
	bytes [][]byte // whole parts, read by every decoder
	text  [][]byte // whole text readings, read by the text decoders
	junk  []byte   // decodings of runs, NUL-separated: one byte part
	tjunk []byte   // substitution text readings (NFKC), NUL-separated: one text part
	tany  []byte   // charset and wide readings, NUL-separated: one text part in
	// which a short value counts anywhere, since every start offset of a
	// stream decoder is a candidate (contract 3, Candidates)
	bjunk []byte // bytes broken raw deflate streams yield: one final text part
	rjunk []byte // run decoders' outputs, NUL-separated: one byte part in
	// which a short value counts anywhere, since every span of a run is a
	// candidate and so a view boundary
	tbytes [][]byte // bytes and text, for decodings of a whole part that
	ttext  [][]byte // ends where the unit ends: they end there too
}

// Decoder families: a large part's families run as tasks of their own
// (match.go step); famAll runs them all.
const (
	famAll = iota
	famHit
	famRaw
	famWide
	famTables
	famMulti
	famRest // the text decoders but base-N
	famBase // + alphabet index
)

var bigFamilies = func() []int {
	f := []int{famHit, famRaw, famWide, famTables, famMulti, famRest}
	for i := range alphabets {
		f = append(f, famBase+i)
	}
	return f
}()

// part reads p (a text reading when text) with the decoders of family fam;
// tr, when not nil, is how the text decoders read p, shared by its tasks.
func (d *decoding) part(p []byte, text bool, fam int, tr *textRuns) {
	s := d.s
	d.text = text
	d.tailEnd = nil
	if d.tail && len(p) > 0 {
		d.tailEnd = &p[len(p)-1]
	}
	in := func(f int) bool { return fam == famAll || fam == f }
	if in(famHit) && s.hitIn(p, d.any) {
		s.stop(leak)
		return
	}
	if text && d.final {
		// A final view (limits.go, brokenText) is read by the byte
		// decoders that find their input by a header or a pattern
		// (gzip, zlib, UTF-16 and UTF-32); what they decode is matched.
		if in(famRaw) {
			d.headed(p)
		}
		if in(famWide) {
			d.wides(p)
		}
	}
	if !text {
		if in(famRaw) {
			if d.f >= maxRawNest {
				s.stop(exhausted) // past the depth limit: a stream here goes unread
				return
			}
			d.compressed(p)
		}
		if in(famWide) {
			d.wides(p)
		}
		if in(famTables) {
			d.tables(p)
		}
		if in(famMulti) {
			d.multiByte(p)
		}
	}
	if fam == famHit || fam > famHit && fam < famRest {
		return
	}
	// Blunt (kit15rv, decoder-binary-intermediate-pruning, a class of many
	// rounds): a held value that is not UTF-8 has a byte no run holds, so
	// while one is held the substitution decoders also read every part
	// that does not read as text whole, as contract 3's Candidates says.
	// Held values are text in ordinary use, so this never runs there.
	binaryWhole := s.m.binary && (fam == famAll || fam == famRest)
	if tr == nil {
		// Every family in one task: the runs are read as they are found.
		if readsAsText(p) {
			d.decText(p, true, fam)
			return
		}
		if binaryWhole {
			d.subst(p, true)
		}
		d.runs(p, func(r []byte) { d.decText(r, false, fam) })
	} else if tr.get(d, p); tr.whole {
		d.decText(p, true, fam)
		return
	} else {
		if binaryWhole {
			d.subst(p, true)
		}
		for i := 0; i+1 < len(tr.runs) && s.ok(); i += 2 {
			d.decText(p[tr.runs[i]:tr.runs[i+1]], false, fam)
		}
	}
	// A view that does not read as text is read by the text decoders in
	// its runs of relevant characters, but a JSON document is one line of
	// the view whatever else the view holds (contract 3, joined values):
	// its lines are joined whole, not as the runs.
	if s.ok() && (fam == famAll || fam == famRest) {
		d.joined(p, d.tail)
	}
}

// runs calls fn with each run of relevant characters in p not yet read at
// this raw nesting, one character long included unless normalization
// leaves it as it is and it is no Ascii85 group (exact: one character is
// no candidate of any other decoder but NFKC's, decText).
func (d *decoding) runs(p []byte, fn func([]byte)) {
	d.s.m.rel.runs(p, func(r []byte) {
		if _, n := utf8.DecodeRune(r); n == len(r) && allInert(r) && !a85Group(r) {
			return
		}
		if len(r) < 8 || d.once(idRun, r) { // exact: a run read again decodes the same
			fn(r)
		}
	})
}

// allNUL reports whether every byte of b is NUL.
func allNUL(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// a85Group reports whether t is one Ascii85 group character (z, or btoa's
// y), which decodes to four bytes alone (kit14rv: the one-character
// shortcuts dropped it).
func a85Group(t []byte) bool { return len(t) == 1 && (t[0] == 'z' || t[0] == 'y') }

// textRuns is how the text decoders of a large part's tasks read it: whole
// when it reads as text, else its runs (decoding.runs), by offset.
type textRuns struct {
	once  sync.Once
	whole bool
	runs  []int32
}

func (t *textRuns) get(d *decoding, p []byte) {
	t.once.Do(func() {
		if t.whole = readsAsText(p); !t.whole {
			d.runs(p, func(r []byte) {
				i := i32(cap(p) - cap(r)) // r's offset in p
				t.runs = append(t.runs, i, i+i32(len(r)))
			})
		}
	})
}

// once is the closure's once for this decoding's raw nesting: an input
// read inside more raw deflate streams is a different input. A final view
// (its decodings matched, not decoded further) keeps its own record, so a
// read there never stands for one where recursion is allowed (kit14rv,
// decoder-context-dedupe); it is skipped when such a read came first.
func (d *decoding) once(id uint64, b []byte) bool {
	id ^= uint64(d.f&0xffff) << 48 // the same bits as uint64(d.f) << 48
	if d.final {
		return d.s.onceUnless(id^1<<47, id, b)
	}
	return d.s.once(id, b)
}

// readsAsText reports whether b reads as text: valid UTF-8 with no control
// character but whitespace and escape (ISO-2022 text).
func readsAsText(b []byte) bool {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= utf8.RuneSelf {
			return utf8.Valid(b[i:]) && !bytes.ContainsFunc(b[i:], func(r rune) bool { return r < 0x20 && !isSpaceByte(byte(r&0xff)) && r != 0x1b || r == 0x7f })
		}
		if c < 0x20 && !isSpaceByte(c) && c != 0x1b || c == 0x7f {
			return false
		}
	}
	return true
}

// out adds a decoding: whole when it reads as text (a document a writer
// encoded), else as a piece of the view's NUL-separated part.
func (d *decoding) out(b []byte) {
	if d.tiny(b, false) || !d.once(idOut, b) { // exact: a view read already
		return
	}
	if readsAsText(b) {
		d.whole(b)
		return
	}
	if d.s.charge(len(b) + 1) {
		k := &d.bk[d.f]
		k.junk = append(append(k.junk, sep), b...)
	}
}

// tiny reports whether b is shorter than tinyView bytes. It is matched
// here, as every view is (after NFKC and case folding), and not decoded
// further (limits.go, tinyView). anywhere is hitIn's.
func (d *decoding) tiny(b []byte, anywhere bool) bool {
	// exact: no decoder reads anything out of NUL bytes (TestFix1BNULRunsDecodeToNothing)
	// and no held value matches them unless it has one.
	if len(b) >= tinyView && !d.s.m.heldNUL && allNUL(b) {
		return true
	}
	if len(b) >= tinyView && !d.final {
		return false
	}
	// exact: with no short or digit-only value held, fewer than 4 bytes can
	// match only through a character normalization expands.
	if len(b) > 0 && (len(b) >= tinyView || d.s.m.short || len(d.s.m.numWhole) > 0 || !allInert(b)) && d.s.hitIn(b, anywhere) {
		d.s.stop(leak)
	}
	return true
}

// compared compares b under section 2, a short value anywhere in it, and
// decodes it no further.
func (d *decoding) compared(b []byte) {
	if d.s.charge(len(b)+1) && d.s.hitIn(b, true) {
		d.s.stop(leak)
	}
}

// runOut adds a run decoder's output: a piece of the view's part in which
// a short value counts anywhere.
func (d *decoding) runOut(b []byte) {
	if d.tiny(b, true) || !d.once(idRunOut, b) { // exact: a view read already
		return
	}
	if d.s.charge(len(b) + 1) {
		k := &d.bk[d.f]
		k.rjunk = append(append(k.rjunk, sep), b...)
	}
}

// whole adds b as a part of its own, once per unit.
func (d *decoding) whole(b []byte) {
	if !d.tiny(b, false) && d.s.fresh(b, false, d.f) && d.s.charge(len(b)) {
		d.bk[d.f].bytes = append(d.bk[d.f].bytes, bytes.Clone(b))
	}
}

// wholeOf adds b, a decoding of the whole part, as a part of its own: one
// that ends where the unit ends when the part does (a separate kind for
// fresh, so a copy of it elsewhere does not hide where it ends).
func (d *decoding) wholeOf(b []byte) {
	if !d.tail {
		d.whole(b)
		return
	}
	if !d.tiny(b, false) && d.s.fresh(b, false, d.f+tailKinds) && d.s.charge(len(b)) {
		d.bk[d.f].tbytes = append(d.bk[d.f].tbytes, bytes.Clone(b))
	}
}

// tailKinds sets the fresh keys of parts that end where the unit ends
// apart from the others.
const tailKinds = maxRawNest + 1

// endsTail reports whether b, a slice of the part being read, ends where
// the unit ends.
func (d *decoding) endsTail(b []byte) bool {
	return d.tailEnd != nil && len(b) > 0 && &b[len(b)-1] == d.tailEnd
}

func (d *decoding) textOut(b []byte, whole bool) {
	if d.tiny(b, false) || !whole && !d.once(idTextOut, b) || !d.s.charge(len(b)+1) {
		return
	}
	if whole && d.tail {
		if d.s.fresh(b, true, d.f+tailKinds) {
			d.bk[d.f].ttext = append(d.bk[d.f].ttext, bytes.Clone(b))
		}
		return
	}
	if whole {
		if d.s.fresh(b, true, d.f) {
			d.bk[d.f].text = append(d.bk[d.f].text, bytes.Clone(b))
		}
		return
	}
	d.bk[d.f].tjunk = append(append(d.bk[d.f].tjunk, sep), b...)
}

// charOut adds a charset or wide (UTF-16/UTF-32) reading: a text reading
// of a stream decoder, which starts at every byte offset (contract 3,
// Candidates), so a short value anywhere in it counts. It is a text view
// (read by the text decoders, not re-read by the charsets: limits.go
// textViews), unlike a run decoder's byte output.
func (d *decoding) charOut(b []byte) {
	if d.tiny(b, true) || !d.once(idCharOut, b) || !d.s.charge(len(b)+1) {
		return
	}
	d.bk[d.f].tany = append(append(d.bk[d.f].tany, sep), b...)
}

// changed adds a decoding that rewrote text in place: whole when its input
// was whole.
func (d *decoding) changed(b []byte, whole bool) {
	if d.onlyJoin {
		// Blunt (kit15rv round 2): a substitution's output is read by the
		// substitution decoders again (a repeated encoding, contract 3
		// Recursion), to maxDepth; deeper fails closed.
		d.joined(b, d.tail)
		if d.joinDeep >= maxDepth {
			d.s.stop(exhausted)
			return
		}
		d.joinDeep++
		d.subst(b, true)
		d.joinDeep--
		return
	}
	if whole {
		d.wholeOf(b)
		return
	}
	d.out(b)
}

// fresh reports whether b is not yet queued as a part of this kind at
// provenance f (decoding.pv), and records it. A byte part is read by every decoder, so a
// text part that repeats one is not fresh.
func (c *closure) fresh(b []byte, text bool, f int) bool {
	return c.freshKey(hash2(b), text, f)
}

func (c *closure) freshKey(k [2]uint64, text bool, f int) bool {
	k[0] ^= uint64((f+1)&0xff) << 56 // the same bits as uint64(f+1) << 56
	if !text {
		return c.seen.add(k)
	}
	t := k
	t[0] ^= 1 << 55
	return c.seen.addUnless(t, k)
}

// subst runs the substitution decoders (percent-encoding, quoted-printable,
// character references, string escapes, MIME encoded words) over t.
func (d *decoding) subst(t []byte, whole bool) {
	if bytes.IndexByte(t, '%') >= 0 || bytes.IndexByte(t, '+') >= 0 && bytes.IndexByte(t, '=') >= 0 {
		d.percent(t, whole)
	}
	if bytes.IndexByte(t, '=') >= 0 {
		if out := qpDecode(t, false); !bytes.Equal(out, t) {
			d.changed(out, whole)
		}
	}
	if bytes.IndexByte(t, '&') >= 0 {
		if out := html.UnescapeString(string(t)); out != string(t) {
			d.changed([]byte(out), whole)
		}
	}
	if bytes.IndexByte(t, '\\') >= 0 {
		for _, identity := range []bool{false, true} {
			for octalMax := 3; octalMax >= 1; octalMax-- {
				if out := unescapeN(t, identity, octalMax); !bytes.Equal(out, t) {
					d.changed(out, whole)
				}
			}
		}
	}
	if bytes.Contains(t, []byte("=?")) {
		d.mime(t, whole)
	}
}

// decText runs the text decoders over t, a whole text or one run.
func (d *decoding) decText(t []byte, whole bool, fam int) {
	s := d.s
	if _, n := utf8.DecodeRune(t); n == len(t) && !whole && !a85Group(t) {
		// exact: one character is no candidate of any decoder but NFKC's
		// and Ascii85's (a85Group).
		if !allInert(t) && fam != famHit && fam < famBase {
			d.textOut(appendNFKC(nil, t), false)
		}
		return
	}
	for i := range alphabets {
		if !s.ok() {
			return
		}
		if fam == famAll || fam == famBase+i {
			d.baseN(&alphabets[i], uint64(i), t)
		}
	}
	if fam != famAll && fam != famRest {
		return
	}
	d.hex(t)
	d.subst(t, whole)
	d.codeLists(t)
	// The normalized views: NFKC, and case folded (contract 3). NFKC of a
	// text view is a text view (limits.go, textViews).
	if !allInert(t) {
		out := appendNFKC(d.s.dst[:0], t)
		if d.s.dst = out; bytes.Equal(out, t) {
		} else if d.text {
			d.textOut(out, whole)
		} else {
			d.changed(out, whole)
		}
	}
	// The case-folded view is compared as every view is (folded), and not
	// decoded further (limits.go, foldView).
	// ROT13 reads a whole text part only (limits.go, rot13TextOnly and
	// rot13WholeText): a charset reading or a decoder's text output queued
	// as runs is not rotated.
	if whole {
		d.rot13(t)
	}
	d.joined(t, whole && d.tail)
}

// alphabet is one base-N alphabet: base64 (standard and URL-safe at once),
// base32 standard and hex (any case), and the three base85 alphabets.
type alphabet struct {
	val    [256]int8 // digit value, or -1
	bits   int       // bits per digit; 0 for base85
	align  int       // alignments: digits per whole group
	pad    bool      // '=' inside a run is padding
	ignore [256]bool // bytes the decoder accepts and ignores: inside a span (contract 3, Candidates)
	a85    bool      // Ascii85: z and y groups
}

var alphabets = func() []alphabet {
	mk := func(chars string, bits, align int) alphabet {
		a := alphabet{bits: bits, align: align}
		for i := range a.val {
			a.val[i] = -1
		}
		for i := 0; i < len(chars); i++ {
			a.val[chars[i]] = int8(i)
		}
		return a
	}
	const (
		upperL = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerL = "abcdefghijklmnopqrstuvwxyz"
		digits = "0123456789"
	)
	b64 := mk(upperL+lowerL+digits+"+/", 6, 4)
	b64.val['-'], b64.val['_'] = 62, 63
	b64.pad = true
	b32 := mk(upperL+"234567", 5, 8)
	b32h := mk(digits+"ABCDEFGHIJKLMNOPQRSTUV", 5, 8)
	for _, a := range []*alphabet{&b64, &b32, &b32h} {
		a.ignore['\r'], a.ignore['\n'] = true, true // Go's base64 and base32
	}
	for _, a := range []*alphabet{&b32, &b32h} {
		a.pad = true
		for c := 'a'; c <= 'z'; c++ {
			a.val[c] = a.val[c-'a'+'A']
		}
	}
	a85 := mk(ascii85Alphabet, 0, 5)
	a85.a85 = true
	// Go's ascii85 also ignores space and control characters. ponytail: a
	// span is read across the line breaks and tabs of a text part only.
	// Across spaces every line of prose becomes one run, and across the
	// control bytes of a binary part (split into runs of relevant characters
	// first) ordinary output runs out of deflate budget; see the kit3rv
	// notes for both measurements.
	a85.ignore['\t'], a85.ignore['\n'], a85.ignore['\r'] = true, true, true
	return []alphabet{b64, b32, b32h, a85, mk(z85Alphabet, 0, 5), mk(rfc1924Alphabet, 0, 5)}
}()

const (
	ascii85Alphabet = "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstu"
	z85Alphabet     = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"
	rfc1924Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"
)

func (a *alphabet) in(c byte) bool { return a.val[c] >= 0 || a.a85 && (c == 'z' || c == 'y') }

// baseN decodes every run of a's digits in t at every alignment (a run can
// start inside the blob it belongs to).
func (d *decoding) baseN(a *alphabet, id uint64, t []byte) {
	s := d.s
	run := d.buf[:0]
	flush := func() {
		if len(run) > 0 && (len(run) < 8 || d.once(idBase+id, run)) { // exact: a run decoded already
			for k := 0; k < a.align && k < len(run); k++ {
				s.dst = a.decode(s.dst[:0], run[k:])
				d.runOut(s.dst)
			}
		}
		run = run[:0]
	}
	for i := 0; i < len(t) && s.ok(); i++ {
		c := t[i]
		switch {
		case a.in(c):
			run = append(run, c)
		case c == '=' && a.pad && len(run) > 0:
		case a.ignore[c]:
		default:
			flush()
		}
	}
	flush()
	d.buf = run[:0]
}

func (a *alphabet) decode(out, r []byte) []byte {
	if a.bits > 0 {
		var acc uint32
		bits := 0
		for _, c := range r {
			acc = acc<<a.bits | uint32(int64(a.val[c])&math.MaxUint32) // uint32(a.val[c]), sign extension included
			bits += a.bits
			if bits >= 8 {
				bits -= 8
				out = append(out, byte(acc>>uint(bits)&0xff))
			}
		}
		return out
	}
	var val uint64
	n := 0
	for _, c := range r {
		if a.a85 && n == 0 && (c == 'z' || c == 'y') {
			if c == 'z' {
				out = append(out, 0, 0, 0, 0)
			} else {
				out = append(out, ' ', ' ', ' ', ' ')
			}
			continue
		}
		if a.val[c] < 0 {
			continue
		}
		val = val*85 + uint64(a.val[c])
		if n++; n == 5 {
			out = append(out, byte(val>>24&0xff), byte(val>>16&0xff), byte(val>>8&0xff), byte(val&0xff))
			val, n = 0, 0
		}
	}
	if n > 1 { // a short last group is padded with the highest digit
		k := n - 1
		for ; n < 5; n++ {
			val = val*85 + 84
		}
		b := [4]byte{byte(val >> 24 & 0xff), byte(val >> 16 & 0xff), byte(val >> 8 & 0xff), byte(val & 0xff)}
		out = append(out, b[:k]...)
	}
	return out
}

// a85Len is how many bytes an Ascii85 run decodes to.
func a85Len(r []byte) int {
	n, g := 0, 0
	for _, c := range r {
		switch {
		case g == 0 && (c == 'z' || c == 'y'):
			n += 4
		case g == 4:
			n, g = n+4, 0
		default:
			g++
		}
	}
	return n + max(0, g-1)
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

func b64Value(c byte) byte { return byte(int(alphabets[0].val[c]) & 0xff) } // byte(val[c]), -1 included

func lenientBase64(r []byte) []byte {
	return alphabets[0].decode(nil, bytes.ReplaceAll(r, []byte{'='}, nil))
}

// hex: digit pairs in any case, each with an optional 0x, 0X or \x
// prefix, any run of separators (space, comma, colon, hyphen) between two
// pairs (contract 3). A chain of pairs is read from every place a pair can
// start that no earlier chain read as a pair: a later start would read a
// stretch of that chain.
func (d *decoding) hex(t []byte) {
	s := d.s
	if cap(d.pos) < len(t) {
		d.pos = make([]int32, len(t))
	}
	seen := d.pos[:len(t)]
	clear(seen)
	pairAt := func(i int) int { // where the pair's digits start, or -1
		switch {
		case i+1 < len(t) && isHexDigit(t[i]) && isHexDigit(t[i+1]):
			return i
		case i+3 < len(t) && (t[i] == '0' && (t[i+1] == 'x' || t[i+1] == 'X') || t[i] == '\\' && t[i+1] == 'x') &&
			isHexDigit(t[i+2]) && isHexDigit(t[i+3]):
			return i + 2
		}
		return -1
	}
	for i := 0; i < len(t) && s.ok(); i++ {
		if seen[i] != 0 {
			continue
		}
		p := pairAt(i)
		if p < 0 {
			continue
		}
		out := s.dst[:0]
		for at := i; p >= 0; {
			seen[at], seen[p] = 1, 1 // the pair, and its prefix
			out = append(out, hexVal(t[p])<<4|hexVal(t[p+1]))
			at = p + 2
			if p = pairAt(at); p < 0 {
				q := at
				for q < len(t) && (t[q] == ' ' || t[q] == ',' || t[q] == ':' || t[q] == '-') {
					q++
				}
				if q > at {
					at, p = q, pairAt(q)
				}
			}
		}
		s.dst = out
		if len(out) < 8 || d.once(idBase+99, out) { // exact: a chain decoded already
			d.runOut(out)
		}
	}
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// percent: %XX and %uXXXX, and form encoding's '+' for a space where the
// text has a key=value pair. Repeated percent-encoding decodes one layer
// per depth.
func (d *decoding) percent(t []byte, whole bool) {
	if bytes.IndexByte(t, '%') >= 0 {
		out := make([]byte, 0, len(t))
		for i := 0; i < len(t); i++ {
			c := t[i]
			if c == '%' {
				if i+5 < len(t) && (t[i+1] == 'u' || t[i+1] == 'U') && allHex(t[i+2:i+6]) {
					out = utf8.AppendRune(out, hexRune(t[i+2:i+6]))
					i += 5
					continue
				}
				if i+2 < len(t) && isHexDigit(t[i+1]) && isHexDigit(t[i+2]) {
					out = append(out, hexVal(t[i+1])<<4|hexVal(t[i+2]))
					i += 2
					continue
				}
			}
			out = append(out, c)
		}
		if !bytes.Equal(out, t) {
			d.changed(out, whole)
		}
	}
	if bytes.IndexByte(t, '+') >= 0 && bytes.IndexByte(t, '=') >= 0 {
		d.changed(bytes.ReplaceAll(t, []byte{'+'}, []byte{' '}), whole)
	}
}

func allHex(b []byte) bool {
	for _, c := range b {
		if !isHexDigit(c) {
			return false
		}
	}
	return true
}

func hex16(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<4 | uint32(hexVal(c))
	}
	return v
}

// hexRune is rune(hex16(b)) for every b a caller passes: at most 6 hex
// digits fit in a rune, and 8 digits above U+7FFFFFFF read as -1, which no
// caller accepts (utf8.ValidRune), exactly as the wrapped negative did.
func hexRune(b []byte) rune {
	v := hex16(b)
	if v > math.MaxInt32 {
		return -1
	}
	return rune(v)
}

// codeRune is v as a rune. Every v reaching it is a code point (codeValue
// and codeEdge stop above unicode.MaxRune); anything larger reads as
// utf8.RuneError rather than wrapping into an unrelated character.
func codeRune(v uint64) rune {
	if v > unicode.MaxRune {
		return utf8.RuneError
	}
	return rune(v)
}

// i32 is n as the int32 the compact tables store. Every n is a count or an
// offset inside one unit (maxUnit), far below 2^31; one that is not is a kit
// bug, and the panic ends the program before it writes anything.
func i32(n int) int32 {
	if n < math.MinInt32 || n > math.MaxInt32 {
		panic("credkit: int32 overflow")
	}
	return int32(n)
}

// u64 is n >= 0 as a uint64; a negative n is a kit bug (see i32).
func u64(n int) uint64 {
	if n < 0 {
		panic("credkit: negative count")
	}
	return uint64(n)
}

// toInt is v as an int; v was stored from an int (see u64).
func toInt(v uint64) int {
	if v > math.MaxInt {
		panic("credkit: int overflow")
	}
	return int(v)
}

// qpDecode: quoted-printable =XX and soft line breaks, and (RFC 2045) the
// blanks at the end of an encoded line dropped (kit14rv).
func qpDecode(b []byte, underscoreSpace bool) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '_' && underscoreSpace {
			out = append(out, ' ')
			continue
		}
		if c == ' ' || c == '\t' {
			j := i
			for j < len(b) && (b[j] == ' ' || b[j] == '\t') {
				j++
			}
			if j == len(b) || b[j] == '\n' || b[j] == '\r' && j+1 < len(b) && b[j+1] == '\n' {
				i = j - 1
				continue
			}
			out = append(out, b[i:j]...)
			i = j - 1
			continue
		}
		if c != '=' {
			out = append(out, c)
			continue
		}
		j := i + 1
		for j < len(b) && (b[j] == ' ' || b[j] == '\t') {
			j++
		}
		if j < len(b) && b[j] == '\n' {
			i = j
			continue
		}
		if j+1 < len(b) && b[j] == '\r' && b[j+1] == '\n' {
			i = j + 1
			continue
		}
		if i+2 < len(b) && isHexDigit(b[i+1]) && isHexDigit(b[i+2]) {
			out = append(out, hexVal(b[i+1])<<4|hexVal(b[i+2]))
			i += 2
			continue
		}
		out = append(out, c)
	}
	return out
}

var simpleEscapes = [256]byte{'a': 7, 'b': 8, 'f': 12, 'n': 10, 'r': 13, 't': 9, 'v': 11, 'e': 27,
	'\\': '\\', '\'': '\'', '"': '"', '?': '?', '/': '/'}

// unescape decodes string escapes (JSON \uXXXX with surrogate pairs, \xXX,
// Go, TOML, JavaScript and C escapes, octal); an unknown escape is kept, or
// with identity its backslash is dropped.
func unescape(b []byte, identity bool) []byte { return unescapeN(b, identity, 3) }

// unescapeN reads string escapes, an octal escape taking at most octalMax
// digits. octalMax is 3 for the greedy reading the contract names; decText
// also reads it at 2 and 1, the readings a candidate that ends inside the
// octal escape would give (contract 3, Candidates and String escapes), so a
// held value whose last byte is the first one or two digits of a longer
// octal run is found without decoding the source at every end position.
func unescapeN(b []byte, identity bool, octalMax int) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		c := b[i+1]
		if r := simpleEscapes[c]; r != 0 {
			out = append(out, r)
			i++
			continue
		}
		switch {
		case c == 'u' && i+2 < len(b) && b[i+2] == '{':
			end := bytes.IndexByte(b[i+3:], '}')
			if end > 0 && end <= 6 && allHex(b[i+3:i+3+end]) {
				if r := hexRune(b[i+3 : i+3+end]); utf8.ValidRune(r) {
					out = utf8.AppendRune(out, r)
					i += 3 + end
					continue
				}
			}
		case c == 'u' && i+5 < len(b) && allHex(b[i+2:i+6]):
			r := hexRune(b[i+2 : i+6])
			if utf16.IsSurrogate(r) && i+11 < len(b) && b[i+6] == '\\' && b[i+7] == 'u' && allHex(b[i+8:i+12]) {
				if d := utf16.DecodeRune(r, hexRune(b[i+8:i+12])); d != unicode.ReplacementChar {
					out = utf8.AppendRune(out, d)
					i += 11
					continue
				}
			}
			out = utf8.AppendRune(out, r)
			i += 5
			continue
		case c == 'U' && i+9 < len(b) && allHex(b[i+2:i+10]):
			if r := hexRune(b[i+2 : i+10]); utf8.ValidRune(r) {
				out = utf8.AppendRune(out, r)
				i += 9
				continue
			}
		case c == 'x' && i+2 < len(b) && isHexDigit(b[i+2]):
			j := i + 2
			for j < len(b) && j < i+4 && isHexDigit(b[j]) {
				j++
			}
			out = append(out, byte(hex16(b[i+2:j])&0xff))
			i = j - 1
			continue
		case c >= '0' && c <= '7':
			val, j := 0, i+1
			for j < len(b) && j < i+1+octalMax && b[j] >= '0' && b[j] <= '7' && val*8+int(b[j]-'0') <= 0xFF {
				val = val*8 + int(b[j]-'0')
				j++
			}
			out = append(out, byte(val))
			i = j - 1
			continue
		}
		if identity {
			out = append(out, c)
			i++
			continue
		}
		out = append(out, b[i])
	}
	return out
}

// rot13 rotates the ASCII letters of a whole text, once: the rotation of
// the rotation is the text itself. (The matcher also holds every value's
// ROT13 form, so a run that is ROT13 as written is found as well.)
func (d *decoding) rot13(t []byte) {
	if !d.once(idROT13, t) {
		return
	}
	out := []byte(rot13s(string(t)))
	if !bytes.Equal(out, t) && d.once(idROT13, out) {
		d.textOut(out, true)
	}
}

// codeLists: decimal or hex code points separated by commas or spaces
// ("97, 98", "0x61 0x62", "U+0061 U+0062"), read as characters and, for codes
// up to 0xFF, as bytes (contract 3, Character-code lists). Each reading has
// its own tokens: a decimal token is a run of ASCII digits, a hex token a
// run of hex digits with an optional 0x (or U+) prefix. A span starts and
// ends at any byte (Candidates), and the list read is the whole one
// (limits.go runDecoderSpans), so whatever stands before a list's first run
// or after its last run does not end the list: the first token is the
// longest suffix of its run that is a code point, the last the longest
// prefix of its run that is one. A run that is no code point ends the list
// before it with that prefix and starts the next with that suffix.
type codeTok struct {
	start, digits, end int // start is before a prefix; digits after it
}

func (d *decoding) codeLists(b []byte) {
	for _, hexMode := range []bool{false, true} {
		toks := codeRuns(b, hexMode, d.toks[:0])
		d.toks = toks[:0]
		if len(toks) < 2 { // exact: contract 3, a list is two or more tokens
			continue
		}
		base := uint64(10)
		if hexMode {
			base = 16
		}
		for _, bytesMode := range []bool{false, true} {
			var cur []byte
			ents := d.ents[:0]
			add := func(v uint64, digits []byte) {
				cur = codeAppend(cur, v, bytesMode)
				ents = append(ents, codeEnt{v, digits})
			}
			flush := func() {
				if len(ents) >= 2 { // exact: contract 3, a list is two or more tokens
					// runOut, not out: every sub-list of two or more tokens
					// is its own candidate (contract 3, Candidates), so a
					// short value anywhere in the decoded list counts, as it
					// does for the other run decoders.
					d.runOut(cur)
					d.codeSpans(ents, base, bytesMode)
				}
				cur, ents = cur[:0], ents[:0]
			}
			for n, t := range toks {
				digits := b[t.digits:t.end]
				if len(ents) > 0 && !codeGap(b[toks[n-1].end:t.start]) {
					flush()
				}
				if len(ents) > 0 {
					if v, ok := codeValue(digits, base); ok {
						add(v, digits)
						continue
					}
					if v, k := codeEdge(digits, base, false); k > 0 {
						add(v, digits[:k])
					}
					flush()
				}
				if v, k := codeEdge(digits, base, true); k > 0 {
					add(v, digits[len(digits)-k:])
				}
			}
			flush()
			d.ents = ents[:0]
		}
	}
}

// codeEnt is one token of a code list: its value and the digits it read.
type codeEnt struct {
	v      uint64
	digits []byte
}

func codeAppend(b []byte, v uint64, bytesMode bool) []byte {
	if bytesMode && v <= 0xFF {
		return append(b, byte(v))
	}
	return utf8.AppendRune(b, codeRune(v))
}

// codeSpans compares the spans of a code list that start or end inside a
// token (contract 3, Candidates: every span the decoder accepts; the ninth
// reading bound decodes only the whole list further). Such a span reads
// its first token from where it starts and its last token to where it
// ends, so its output is the list's with other characters at its edges.
// Only an edge character that can be part of a held value is read, and
// only the characters near it: an 8-character piece, or a whole value
// shorter than 8, fits in 8 characters (32 bytes in the byte reading).
func (d *decoding) codeSpans(ents []codeEnt, base uint64, bytesMode bool) {
	rel := &d.s.m.rel
	keep := func(v uint64) bool { return bytesMode && v >= 0x80 && v <= 0xFF || rel.has(codeRune(v)) }
	// cuts is the values of digits' shorter suffixes (or prefixes).
	cuts := func(e codeEnt, suffix bool, out []uint64) []uint64 {
		for k := len(e.digits) - 1; k >= 1; k-- {
			part := e.digits[:k]
			if suffix {
				part = e.digits[len(e.digits)-k:]
			}
			if v, ok := codeValue(part, base); ok && v != e.v && keep(v) && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
		return out
	}
	win := 8
	if bytesMode {
		win = 32
	}
	var buf []byte
	var pre, suf []uint64
	for j := 1; j < len(ents) && d.s.ok(); j++ { // ends inside token j, starts at a whole token
		if pre = cuts(ents[j], false, pre[:0]); len(pre) == 0 {
			continue
		}
		buf = buf[:0]
		for t := max(0, j-win+1); t < j; t++ {
			buf = codeAppend(buf, ents[t].v, bytesMode)
		}
		for _, w := range pre {
			d.compared(codeAppend(buf, w, bytesMode))
		}
	}
	for i := 0; i+1 < len(ents) && d.s.ok(); i++ { // starts inside token i
		if suf = cuts(ents[i], true, suf[:0]); len(suf) == 0 {
			continue
		}
		for _, v := range suf {
			buf = codeAppend(buf[:0], v, bytesMode)
			for j := i + 1; j < len(ents) && j < i+win; j++ {
				for _, w := range cuts(ents[j], false, pre[:0]) { // and ends inside token j
					d.compared(codeAppend(buf, w, bytesMode))
				}
				buf = codeAppend(buf, ents[j].v, bytesMode)
			}
			d.compared(buf)
		}
	}
	if len(d.s.m.numWhole) > 0 {
		d.codeNumberSpans(ents, base, bytesMode, cuts)
	}
}

// numberChar reports whether c can be part of a Numbers spelling (contract
// 3): a digit, a sign, the point, e or E, or a group separator.
func numberChar(v uint64) bool {
	return v < 0x80 && (isDigit(byte(v)) || strings.IndexByte("+-.eE,_' ", byte(v)) >= 0)
}

// codeNumberSpans compares, for a held all-digit value, the code-list spans
// that start or end inside a token over every token a spelling can reach.
// A Numbers spelling has no length bound (leading zeros, an exponent's
// zeros), so the eight-character windows of codeSpans do not reach all of
// it; a spelling is a run of characters numberChar accepts, so it reaches
// as far as the tokens around the cut read as such characters, and no
// further. Each comparison counts a value anywhere (compared), so one per
// place a span can end covers every start inside it.
func (d *decoding) codeNumberSpans(ents []codeEnt, base uint64, bytesMode bool, cuts func(codeEnt, bool, []uint64) []uint64) {
	var buf []byte
	var pre, suf []uint64
	numeric := func(vs []uint64) []uint64 {
		return slices.DeleteFunc(vs, func(v uint64) bool { return !numberChar(v) })
	}
	// Ends inside token j, starting at a whole token: from the first token
	// of the run before j.
	for j := 1; j < len(ents) && d.s.ok(); j++ {
		if pre = numeric(cuts(ents[j], false, pre[:0])); len(pre) == 0 {
			continue
		}
		l := j
		for l > 0 && numberChar(ents[l-1].v) {
			l--
		}
		buf = buf[:0]
		for t := l; t < j; t++ {
			buf = codeAppend(buf, ents[t].v, bytesMode)
		}
		for _, w := range pre {
			d.compared(codeAppend(buf, w, bytesMode))
		}
	}
	// Starts inside token i: to the end of the run after it, and to inside
	// each token j of it or just past it.
	for i := 0; i+1 < len(ents) && d.s.ok(); i++ {
		if suf = numeric(cuts(ents[i], true, suf[:0])); len(suf) == 0 {
			continue
		}
		for _, v := range suf {
			buf = codeAppend(buf[:0], v, bytesMode)
			j := i + 1
			for ; j < len(ents) && d.s.ok(); j++ {
				// A spelling that needs this cut and that end starts with
				// it: from any other first character (a sign, a zero, a
				// separator, e), dropping it leaves a spelling the loop
				// above compares.
				if v == '.' || v >= '1' && v <= '9' {
					for _, w := range numeric(cuts(ents[j], false, pre[:0])) {
						d.compared(codeAppend(buf, w, bytesMode))
					}
				}
				if !numberChar(ents[j].v) {
					break
				}
				buf = codeAppend(buf, ents[j].v, bytesMode)
			}
			d.compared(buf)
		}
	}
}

// codeRuns is b's tokens for one reading: maximal runs of the reading's
// digits, a hex run with its 0x or U+ prefix.
func codeRuns(b []byte, hexMode bool, toks []codeTok) []codeTok {
	digit := isDigit
	if hexMode {
		digit = isHexDigit
	}
	for i := 0; i < len(b); {
		j := i
		if hexMode && i+2 < len(b) && isHexDigit(b[i+2]) &&
			(b[i] == '0' && (b[i+1] == 'x' || b[i+1] == 'X') || (b[i] == 'U' || b[i] == 'u') && b[i+1] == '+') {
			j = i + 2
		}
		if !digit(b[j]) {
			i++
			continue
		}
		k := j
		for k < len(b) && digit(b[k]) {
			k++
		}
		toks = append(toks, codeTok{start: i, digits: j, end: k})
		i = k
	}
	return toks
}

// codeGap reports whether the bytes between two tokens separate them in
// one list: one or more commas and spaces (the kit also reads ASCII
// punctuation and whitespace there).
func codeGap(gap []byte) bool {
	return len(gap) > 0 && !bytes.ContainsFunc(gap, func(r rune) bool {
		return r >= utf8.RuneSelf || r < 0x20 && !isSpaceByte(byte(r&0xff)) || unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}

// codeValue is digits' value when it is a code point (a Unicode scalar
// value; leading zeros pad without changing it).
func codeValue(digits []byte, base uint64) (uint64, bool) {
	var v uint64
	for _, c := range digits {
		v = v*base + uint64(hexVal(c))
		if v > unicode.MaxRune {
			return 0, false
		}
	}
	return v, len(digits) > 0 && (v < 0xD800 || v > 0xDFFF)
}

// codeEdge is the longest suffix (or prefix) of digits that is a code
// point, and its length (0 for none). Adding a digit never lowers the
// value, so once it passes U+10FFFF no longer one is a code point.
func codeEdge(digits []byte, base uint64, suffix bool) (uint64, int) {
	var v, mul uint64 = 0, 1
	best, bestLen := uint64(0), 0
	for k := 1; k <= len(digits); k++ {
		if suffix {
			c := uint64(hexVal(digits[len(digits)-k]))
			if c != 0 && mul > unicode.MaxRune {
				break
			}
			v += c * mul
			if mul <= unicode.MaxRune {
				mul *= base
			}
		} else {
			v = v*base + uint64(hexVal(digits[k-1]))
		}
		if v > unicode.MaxRune {
			break
		}
		if v < 0xD800 || v > 0xDFFF {
			best, bestLen = v, k
		}
	}
	return best, bestLen
}

var mimeWordRE = regexp.MustCompile(`=\?([^?\s]+)\?([BbQq])\?([^?\s]*)\?=`)

// mime: RFC 2047 encoded words, adjacent words joined, in the charset each
// names (any charset golang.org/x/text knows), and as raw bytes.
func (d *decoding) mime(t []byte, whole bool) {
	if out, ok := decodeMIMEWords(t, true); ok {
		d.changed(out, whole)
		raw, _ := decodeMIMEWords(t, false)
		d.changed(raw, whole)
	}
}

func decodeMIMEWords(b []byte, convert bool) ([]byte, bool) {
	locs := mimeWordRE.FindAllSubmatchIndex(b, -1)
	if len(locs) == 0 {
		return nil, false
	}
	var out []byte
	last := 0
	for n, l := range locs {
		gap := b[last:l[0]]
		if n == 0 || len(bytes.TrimFunc(gap, unicode.IsSpace)) > 0 {
			out = append(out, gap...)
		}
		payload := b[l[6]:l[7]]
		var raw []byte
		if upper(b[l[4]]) == 'B' {
			raw = lenientBase64(payload)
		} else {
			raw = qpDecode(payload, true)
		}
		if convert {
			if enc := charsetByName(string(b[l[2]:l[3]])); enc != nil {
				if dd, _, err := transform.Bytes(enc.NewDecoder(), raw); err == nil || len(dd) > 0 {
					raw = dd
				}
			}
		}
		out = append(out, raw...)
		last = l[1]
	}
	return append(out, b[last:]...), true
}

func charsetByName(name string) encoding.Encoding {
	if i := bytes.IndexByte([]byte(name), '*'); i >= 0 { // RFC 2231 language suffix
		name = name[:i]
	}
	if e, err := htmlindex.Get(name); err == nil && e != nil {
		return e
	}
	if e, err := ianaindex.MIME.Encoding(name); err == nil && e != nil {
		return e
	}
	return nil
}

// compressed: gzip and zlib streams wherever their header occurs, read to
// their end or first error, and raw deflate at every offset. What any
// stream decodes is a view however it stopped (contract 3): a raw deflate
// stream has no header, so nearly every offset decodes a few bytes before
// an invalid code, and those bytes are a view like any other. The bytes
// after a stream's end are a view too. A long repeat makes
// almost every offset a broken fixed-Huffman prefix, so the output this path
// decodes is drawn from a per-unit budget (inflate, deflLeft); when it runs
// out the unit is withheld, which bounds this path's cost.
func (d *decoding) compressed(p []byte) {
	d.headed(p)
	d.rawStreams(p)
}

// headed reads every gzip and zlib stream in p, at every offset a header
// starts.
func (d *decoding) headed(p []byte) { d.headedTo(p, len(p)) }

// headedTo is headed for the headers that start before limit. Each stream
// is read as far as p goes.
func (d *decoding) headedTo(p []byte, limit int) {
	s := d.s
	for i := 0; i < limit && i+2 < len(p) && s.ok(); i++ {
		switch {
		case p[i] == 0x1f && p[i+1] == 0x8b && p[i+2] == 8:
			if s.gz == nil {
				s.gz = new(gzip.Reader)
			}
			r := bytes.NewReader(p[i:])
			if s.gz.Reset(r) == nil {
				s.gz.Multistream(false)
				d.inflated(s.gz, r, p)
			}
		case p[i]&0x0f == 8 && p[i]>>4 <= 7 && (uint16(p[i])<<8|uint16(p[i+1]))%31 == 0:
			var err error
			r := bytes.NewReader(p[i:])
			if s.zl == nil {
				s.zl, err = zlib.NewReader(r)
			} else {
				err = s.zl.(zlib.Resetter).Reset(r, nil)
			}
			if err == nil {
				d.inflated(s.zl, r, p)
			}
		}
	}
}

// rawStreams reads every headerless raw deflate stream in p, at every
// offset.
func (d *decoding) rawStreams(p []byte) {
	s := d.s
	s.defl.reset()
	after := func(int) {} // limits.go, rawSuffix
	f := d.f
	d.f++
	defer func() { d.f = f }()
	m := s.defl
	for i := 0; i < len(p) && s.ok(); i++ {
		if p[i]&6 == 6 {
			continue // block type 3: no stream starts here
		}
		res := m.fixedEnds(p, i)
		cur := m.id
		switch res {
		case inflateJoin:
			// It joined an attempt that broke, and breaks no later. When
			// what it decoded before the join is the end of what the other
			// had there, every back-reference after it reads the same bytes:
			// its output is a stretch of the other's, already checked.
			o, ja := m.outs[m.from], m.joinA
			pre, _, _ := inflateFixed(p[i:], s.inf[:0], max(0, int(s.budget.Load())), m.joinBit)
			s.inf = pre[:0]
			if ja <= len(o) && len(pre) <= ja && bytes.Equal(pre, o[ja-len(pre):ja]) {
				cur = m.id
				if cur >= 0 {
					m.outs[cur] = o[ja-len(pre):]
				}
				continue
			}
			if d.sharedTail(p, pre, o, ja, i) {
				continue
			}
			fallthrough
		case inflateBroken:
			if m.n == 0 && cur < 0 {
				continue // exact: no output gives no view, and no later attempt joins it
			}
			out, end, _ := d.inflate(p[i:])
			if out == nil {
				return // the fixed-Huffman decode budget ran out: withheld
			}
			if end == inflateLimit {
				s.stop(exhausted)
				return
			}
			if cur = m.id; cur >= 0 {
				m.outs[cur] = bytes.Clone(out)
			}
			d.broken(out)
		case inflateDone, inflateRedo:
			out, end, used := d.inflate(p[i:])
			if out == nil {
				return // the fixed-Huffman decode budget ran out: withheld
			}
			switch end {
			case inflateLimit:
				s.stop(exhausted)
			case inflateDone:
				// A stream that joins one that ended decodes what it
				// decoded wherever their histories agree (a stream repeated
				// through the part, read from each offset). When its output
				// is, byte for byte, a stretch of the other's, which was
				// queued whole, it is nothing new: every decoder reads
				// every offset of the other.
				if res == inflateRedo {
					o, ja, jb := m.outs[m.from], m.joinA, m.joinB
					if o != nil && ja <= len(o) && jb <= ja && jb <= len(out) && ja-jb+len(out) <= len(o) &&
						bytes.Equal(out, o[ja-jb:ja-jb+len(out)]) {
						if cur = m.id; cur >= 0 {
							m.outs[cur], m.done[cur] = o[ja-jb:ja-jb+len(out)], true
						}
						break
					}
				}
				if cur = m.id; cur >= 0 {
					m.outs[cur], m.done[cur] = bytes.Clone(out), true
				}
				d.whole(out)
				after(i + used)
			case inflateBroken:
				// It joined one that ended, but a copy past its own output
				// stopped it first: what it decoded is a view all the same.
				d.broken(out)
			case inflateOther:
				if n := d.flate(p, i); n >= 0 {
					after(n)
				}
			}
		case inflateOther:
			if p[i]&6 == 0 && (i+5 > len(p) || p[i+1] != ^p[i+3] || p[i+2] != ^p[i+4]) {
				continue // a stored block's length and its complement disagree
			}
			if !deflateCanStart(p[i:], 1) {
				continue // exact: no output before the first block fails
			}
			if n := d.flate(p, i); n >= 0 {
				after(n)
				if cur = m.id; cur >= 0 {
					m.done[cur] = true
				}
			}
		}
	}
}

// inflate decodes the fixed-Huffman stream at b with the shared scratch,
// drawing its output from the per-unit decode budget (deflLeft) so a long
// repeat, which makes almost every offset a broken prefix, cannot make the
// check quadratic: when that budget runs out the unit is withheld. It
// returns a nil slice then.
func (d *decoding) inflate(b []byte) ([]byte, inflateEnd, int) {
	s := d.s
	lim := min(max(0, int(s.budget.Load())), int(s.deflLeft.Load()))
	if lim <= 0 {
		s.stop(exhausted)
		return nil, 0, 0
	}
	out, end, n := inflateFixed(b, s.inf[:0], lim, -1)
	s.inf = out[:0]
	s.deflLeft.Add(-int64(len(out)))
	return out, end, n
}

// flate decodes the raw deflate stream at p[i:] with a stored or dynamic
// block: what it decodes is a view however it stops. It returns where in p
// the stream ended, or -1.
func (d *decoding) flate(p []byte, i int) int {
	s := d.s
	r := bytes.NewReader(p[i:])
	if s.fl == nil {
		s.fl = flate.NewReader(r)
	} else {
		_ = s.fl.(flate.Resetter).Reset(r, nil)
	}
	out, ended := s.readAll(s.fl, false)
	switch {
	case !s.ok():
		return -1
	case ended:
		d.whole(out)
		return len(p) - r.Len()
	}
	d.broken(out)
	return -1
}

// broken adds the bytes a broken raw deflate stream yields: a final view
// (limits.go, brokenText), cut short where it only repeats the end of one
// already added (limits.go, brokenSuffix).
func (d *decoding) broken(b []byte) {
	if n := d.s.defl.trimmed(b); n < len(b) {
		if readsAcross(b[:n], n-brokenWindow) {
			d.acrossCut(b, n-brokenWindow)
		}
		b = b[:n]
	}
	d.finalOut(b)
}

// sharedTail adds the output of an attempt that joined an earlier one, when
// that is what the memo proves it to be: pre, what it decoded before the join,
// then o from ja, the rest of what the earlier one decoded (limits.go,
// brokenSuffix, and fixedCopies for the proof). It reports false if not, and
// the caller decodes the attempt.
func (d *decoding) sharedTail(p, pre, o []byte, ja, at int) bool {
	s, m := d.s, d.s.defl
	if trimOff || o == nil || ja > len(o) {
		return false
	}
	if low := m.lowestRead(p, m.from, ja); low < ja { // exact: see fixedCopies
		same := 0
		for same < min(len(pre), ja) && pre[len(pre)-1-same] == o[ja-1-same] {
			same++
		}
		if ja-low > same { // exact: a copy reads what differs
			return false
		}
	}
	rest := o[ja:]
	if len(rest) < 2*brokenWindow { // exact: nothing to cut
		return false
	}
	kept := append(append(s.kept[:0], pre...), rest[:brokenWindow]...)
	s.kept = kept
	if !cutAllowed(kept) { // exact: see brokenSuffix
		return false
	}
	if sharedTailHook != nil {
		sharedTailHook(p[at:], pre, rest)
	}
	if readsAcross(kept, len(pre)) {
		d.acrossCut(append(slices.Clip(kept[:len(pre)]), rest...), len(pre))
	}
	d.finalOut(kept)
	return true
}

// readsAcross reports whether a final view b cut past limit has a reading of
// acrossCut's that starts before limit: past it, b is another output's bytes
// to their end, so a reading that starts there is that output's.
func readsAcross(b []byte, limit int) bool {
	for i := 0; i < limit; i++ {
		switch c := b[i]; {
		case c == '{' || c == '[':
			if jsonStart(b[i:]) {
				return true
			}
		case i+2 < len(b) && (c == 0x1f && b[i+1] == 0x8b && b[i+2] == 8 || c&0x0f == 8 && c>>4 <= 7 && (uint16(c)<<8|uint16(b[i+1]))%31 == 0):
			return true
		}
	}
	return false
}

// acrossCut does for a final view b, cut past n, the readings that are not of
// a window: a compressed stream that starts before n and a JSON document open
// across the cut. They read b itself, as the view would have, and what they
// decode is matched and not decoded further, as a final view's decodings are.
func (d *decoding) acrossCut(b []byte, n int) {
	s, final := d.s, d.final
	inf := s.inf // b is in it, and the streams read into it
	s.inf = nil
	d.final = true
	d.headedTo(b, n)
	d.joined(b, false)
	d.final, s.inf = final, inf
}

// finalOut adds a final view (limits.go, brokenText): read as a text view
// and by the header and pattern byte decoders, and what those decode is
// matched, not decoded further.
func (d *decoding) finalOut(b []byte) {
	if d.tiny(b, false) || !d.once(idFinalOut, b) || !d.s.charge(len(b)+1) {
		return
	}
	k := &d.bk[d.f]
	k.bjunk = append(append(k.bjunk, sep), b...)
}

// inflated queues what a gzip or zlib stream read from r decodes, however
// it ended, and the bytes of p after it when it ended (rest is what r reads
// from).
func (d *decoding) inflated(r io.Reader, rest *bytes.Reader, p []byte) {
	out, ended := d.s.readAll(r, true)
	if len(out) > 0 && d.s.ok() {
		d.whole(out)
	}
	if ended && rest.Len() > 0 && d.s.ok() {
		d.whole(p[len(p)-rest.Len():])
	}
}

// readAll reads r to its end, its first error or the budget, and reports
// whether the stream ended cleanly. It charges what it reads when charge
// is set; else it stops with the unit exhausted when the output outgrows
// the budget.
func (s *scan) readAll(r io.Reader, charge bool) ([]byte, bool) {
	out := s.inf[:0]
	if s.rbuf == nil {
		s.rbuf = make([]byte, 4096)
	}
	for {
		n, err := r.Read(s.rbuf)
		out = append(out, s.rbuf[:n]...)
		if charge && !s.charge(n) || len(out) > int(s.budget.Load()) {
			s.stop(exhausted)
			return nil, false
		}
		if err != nil {
			s.inf = out[:0]
			return out, err == io.EOF
		}
	}
}

// wides, tables and multiByte read p in every charset golang.org/x/text/encoding has: UTF-16
// and UTF-32 at every alignment, both byte orders (a byte-order mark is
// one more character), the single-byte charsets and the multi-byte ones. A
// UTF-16 or UTF-32 reading is read as bytes (its characters up to 0xFF,
// so a nested UTF-16 layer decodes again) and as text (its relevant
// characters, where one is above 0xFF); every other reading is text.
func (d *decoding) wides(p []byte) {
	for _, unit := range []int{2, 4} {
		for _, big := range []bool{false, true} {
			for off := 0; off < unit && off+unit <= len(p); off++ {
				d.wide(p[off:], unit, big)
			}
		}
	}
}

func (d *decoding) wide(b []byte, unit int, big bool) {
	s, rel := d.s, &d.s.m.rel
	by := s.dst[:0]
	tx := d.buf[:0]
	hasWide := false
	endText := func() {
		if hasWide {
			d.charOut(tx)
		}
		tx, hasWide = tx[:0], false
	}
	quote, open, cut, trig, changes := false, false, false, false, false
	flag := func(a rune) {
		quote, open = quote || a == '"', open || a == '{' || a == '['
		trig = trig || a < 0x80 && strings.IndexByte("\"%\\&=+", byte(a&0xff)) >= 0
	}
	cm := compat()
	for i := 0; i+unit <= len(b) && s.ok(); i += unit {
		c, skip := wideRune(b[i:], unit, big)
		i += skip
		if c >= 0 && c <= 0xFF {
			by = append(by, byte(c))
		} else if len(by) > 0 {
			d.wideOut(by)
			by = by[:0]
		}
		if c >= 0 && rel.has(c) {
			tx = utf8.AppendRune(tx, c)
			hasWide = hasWide || c > 0xFF
			// Its normalized form (compat), whole: a fullwidth brace is
			// "{", a circled twelve "12". Blunt (kit15rv round 6, the
			// sixth round of this class: each narrower trigger, structure
			// only, then an escape window, then an open-escape state, was
			// beaten by a decoder whose syntax it did not model): any
			// compatibility character asks for the normalized reading.
			if n, ok := cm[c]; ok && c >= 0x80 {
				changes = true
				for _, a := range n {
					flag(a)
				}
			} else {
				flag(c)
			}
		} else {
			cut = true
			if len(tx) > 0 {
				endText()
			}
		}
	}
	d.wideOut(by)
	endText()
	s.dst, d.buf = by[:0], tx[:0]
	if cut && (quote && open || trig) && s.ok() {
		// The reading was cut at characters no held value has, which may
		// be a document's member names or structure: its documents are
		// joined over the whole reading, each such character read as
		// U+FFFD (contract 3, Charsets and Joined values). Blunt
		// (kit15rv, charset-context-pruning, a class of many rounds): when
		// the reading holds a quote or a character a substitution decoder
		// starts at, the structure may be written through a decoder (a
		// percent-encoded brace), so the substitution decoders' readings
		// of the whole reading are joined as well.
		// Each stretch of such characters is one U+FFFD: no held value has
		// one, so the stretch's length changes no match, and a reading of
		// ordinary bytes (almost all such characters) stays small.
		// Blunt (kit15rv round 3): when a compatibility character (a
		// fullwidth brace) may be the structure, the reading with each
		// character in its normalized form (compat) is joined too.
		var full, alt []byte
		gap := false
		for i := 0; i+unit <= len(b); i += unit {
			c, skip := wideRune(b[i:], unit, big)
			i += skip
			if c < 0 || !rel.has(c) {
				if !gap {
					full = utf8.AppendRune(full, utf8.RuneError)
					if changes {
						alt = utf8.AppendRune(alt, utf8.RuneError)
					}
				}
				gap = true
				continue
			}
			gap = false
			full = utf8.AppendRune(full, c)
			if changes {
				if n, ok := cm[c]; ok && c >= 0x80 {
					alt = append(alt, n...)
				} else {
					alt = utf8.AppendRune(alt, c)
				}
			}
		}
		readings := [][]byte{full}
		if changes {
			readings = append(readings, alt)
		}
		for _, r := range readings {
			if !s.charge(len(r)) {
				break
			}
			d.joined(r, d.tail)
			if !d.final && bytes.ContainsAny(r, "\"%\\&=+") {
				// What the substitution decoders make of the whole reading
				// is joined too (a percent-encoded brace): only joined,
				// since every other reading of it lies inside one run.
				d.onlyJoin = true
				d.subst(r, true)
				d.onlyJoin = false
			}
		}
	}
}

// wideRune reads the UTF-16 or UTF-32 character at the start of b (-1 when
// it is no character) and how many more bytes than unit it took (a
// surrogate pair).
func wideRune(b []byte, unit int, big bool) (rune, int) {
	var c rune
	switch {
	case unit == 2 && big:
		c = rune(b[0])<<8 | rune(b[1])
	case unit == 2:
		c = rune(b[1])<<8 | rune(b[0])
	case big:
		c = rune(b[0])<<24 | rune(b[1])<<16 | rune(b[2])<<8 | rune(b[3])
	default:
		c = rune(b[3])<<24 | rune(b[2])<<16 | rune(b[1])<<8 | rune(b[0])
	}
	if c > unicode.MaxRune || c < 0 {
		return -1, 0
	}
	if unit == 2 && utf16.IsSurrogate(c) && 4 <= len(b) { // exact: a surrogate pair is two units
		n := rune(b[3])<<8 | rune(b[2])
		if big {
			n = rune(b[2])<<8 | rune(b[3])
		}
		if r := utf16.DecodeRune(c, n); r != unicode.ReplacementChar {
			return r, unit
		}
	}
	return c, 0
}

// wideOut adds a UTF-16 or UTF-32 reading's bytes (its characters up to
// 0xFF): read as bytes, so a nested layer decodes again, and, since a
// charset reading starts at every byte offset (contract 3, Candidates),
// compared with a short value counted anywhere.
func (d *decoding) wideOut(b []byte) {
	// exact: with no short or digit-only value held, anywhere matches as
	// the comparison out makes does.
	if len(b) > 0 && (d.s.m.short || len(d.s.m.numWhole) > 0) && d.s.hitIn(b, true) {
		d.s.stop(leak)
		return
	}
	d.out(b)
}

// tables reads p in each group of single-byte charsets that has a special
// byte in p (charset.go), keeping the runs that hold one. The groups that
// read ASCII as themselves differ from the bytes as written only at their
// special bytes, all above 0x7F: each stretch of bytes relevant in any of
// them is read once per distinct reading, not once per group.
func (d *decoding) tables(p []byte) {
	m := d.s.m
	for i := 0; i < len(p); i++ {
		if p[i] < 0x80 || m.relMask[p[i]] == 0 {
			continue
		}
		l, r := i, i+1
		for l > 0 && m.relMask[p[l-1]] != 0 {
			l--
		}
		for r < len(p) && m.relMask[p[r]] != 0 {
			r++
		}
		d.highStretch(p[l:r])
		i = r
	}
	var pres [4]uint64
	for _, c := range p {
		pres[c>>6] |= 1 << (c & 63)
	}
	for gi := range m.tables {
		g := &m.tables[gi]
		if g.asciiSafe || pres[0]&g.special[0]|pres[1]&g.special[1]|pres[2]&g.special[2]|pres[3]&g.special[3] == 0 {
			continue
		}
		start, special := -1, false
		quote, open, cut := false, false, false
		for i, c := range p {
			if g.char[c] < 0 {
				if special {
					d.tableRun(g, p[start:i])
				}
				start, special, cut = -1, false, true
				continue
			}
			if start < 0 {
				start = i
			}
			special = special || g.isSpecial(c)
			r := g.char[c]
			quote, open = quote || r == '"', open || r == '{' || r == '['
		}
		if special {
			d.tableRun(g, p[start:])
		}
		if quote && open && cut && d.s.ok() {
			// As in wide: the reading's documents are joined over the whole
			// reading, a character no held value has read as U+FFFD.
			var full []byte
			for _, c := range p {
				r := g.char[c]
				if r < 0 {
					r = utf8.RuneError
				}
				full = utf8.AppendRune(full, r)
			}
			if d.s.charge(len(full)) {
				d.joined(full, d.tail)
			}
		}
	}
}

// highStretch reads st, bytes each relevant in some ASCII-safe group, in
// every such group that reads its high bytes differently from the groups
// before it.
func (d *decoding) highStretch(st []byte) {
	m := d.s.m
	var union uint64
	for _, c := range st {
		if c >= 0x80 {
			union |= m.relMask[c]
		}
	}
	if one := oneHigh(st); one >= 0 {
		// One high byte: a reading for each way the groups read it.
		for _, gi := range m.distinct[st[one]] {
			d.groupRuns(&m.tables[gi], st)
		}
		return
	}
	var reps [64]*tableGroup
	nrep := 0
next:
	for ; union != 0; union &= union - 1 {
		g := &m.tables[m.high[mbits.TrailingZeros64(union)]]
		for _, h := range reps[:nrep] {
			same := true
			for _, c := range st {
				if c >= 0x80 && g.fid[c] != h.fid[c] {
					same = false
					break
				}
			}
			if same {
				continue next
			}
		}
		reps[nrep] = g
		nrep++
		d.groupRuns(g, st)
	}
}

// oneHigh is the index of st's one byte above 0x7F, or -1.
func oneHigh(st []byte) int {
	at := -1
	for i, c := range st {
		if c >= 0x80 {
			if at >= 0 {
				return -1
			}
			at = i
		}
	}
	return at
}

// groupRuns reads g's runs in st that hold a byte above 0x7F.
func (d *decoding) groupRuns(g *tableGroup, st []byte) {
	{
		start, high := -1, false
		for k, c := range st {
			switch {
			case c < 0x80:
			case g.char[c] < 0:
				if high {
					d.tableRun(g, st[start:k])
				}
				start, high = -1, false
				continue
			default:
				high = true
			}
			if start < 0 {
				start = k
			}
		}
		if high {
			d.tableRun(g, st[start:])
		}
	}
}

// tableRun queues g's reading of b. A group that reads ASCII as itself
// differs from the bytes as written only at characters no text decoder
// reads (every escape and alphabet is ASCII), so its reading is queued in
// NFKC, the one decoder that reads them: what the reading itself would
// decode, the bytes as written already do. Where each character after the
// first starts a normalization segment, the reading's NFKC is its
// characters' NFKC forms in turn.
func (d *decoding) tableRun(g *tableGroup, b []byte) {
	run := d.buf[:0]
	if g.asciiSafe {
		for k, c := range b {
			switch {
			case c < 0x80:
				run = append(run, c)
				continue
			case k == 0 || g.bb[c]:
				run = append(run, g.form[c]...)
				continue
			}
			// A character combines with the one before it.
			run = run[:0]
			for _, c := range b {
				run = utf8.AppendRune(run, g.char[c])
			}
			d.buf = run[:0]
			run = appendNFKC(d.s.dst[:0], run)
			d.s.dst = run[:0]
			if d.endsTail(b) {
				d.jsonDoc(run, true)
			}
			d.charOut(run)
			return
		}
	} else {
		for _, c := range b {
			run = utf8.AppendRune(run, g.char[c])
		}
	}
	d.buf = run[:0]
	if d.endsTail(b) {
		d.jsonDoc(run, true) // a document open where the unit ends, in this reading
	}
	d.charOut(run)
}

// multiByte reads p in the multi-byte charsets for relevant characters:
// from every byte sequence that encodes one, it reads on while the bytes
// encode relevant characters (and back over the relevant ASCII before it),
// once for each place a reading can start. ISO-2022-JP and HZ switch modes
// with escapes (modal).
func (d *decoding) multiByte(p []byte) {
	s := d.s
	high := false
	for _, c := range p {
		if c >= 0x80 {
			high = true
			break
		}
	}
	d.modal(japanese.ISO2022JP, p, 0x1b, jpSwitch)
	d.modal(simplifiedchinese.HZGB2312, p, '~', hzSwitch)
	if !high {
		return
	}
	x := s.m.multiIndex()
	words := (len(p) + 63) / 64
	covered := make([]uint64, words*len(x.encs))
	for i := 0; i < len(p) && s.ok(); i++ {
		if p[i] < 0x80 {
			continue
		}
		mask := x.lead[p[i]]
		if i+1 < len(p) {
			mask |= x.pair[int(p[i])<<8|int(p[i+1])]
		}
		for e := 0; mask != 0; e, mask = e+1, mask>>1 {
			cov := covered[e*words : (e+1)*words]
			if mask&1 == 0 || cov[i/64]&(1<<(i%64)) != 0 {
				continue
			}
			if _, n := x.at(e, p[i:]); n == 0 {
				continue
			}
			start := i
			for start > 0 && d.s.m.rel.asc(p[start-1]) {
				start--
			}
			run := append(d.buf[:0], p[start:i]...)
			j := i
			for j < len(p) {
				if p[j] < 0x80 {
					if !d.s.m.rel.asc(p[j]) {
						break
					}
					run = append(run, p[j])
					j++
					continue
				}
				c, n := x.at(e, p[j:])
				if n == 0 {
					break
				}
				cov[j/64] |= 1 << (j % 64)
				run = append(run, c...)
				j += n
			}
			if j == len(p) && d.tail {
				d.jsonDoc(run, true) // a document open where the unit ends, in this reading
			}
			d.charOut(run)
			d.buf = run[:0]
		}
	}
}

// modal reads p in a charset that switches modes with escapes (contract 3,
// Charsets: a reading from every byte offset). A reading starts in ASCII
// mode and reads byte by byte until an introducer (ESC, or HZ's ~); at a
// switch the decoder accepts, every reading takes the same mode and agrees
// from there on. So the readings are the one from offset 0 and, for each
// switch, the ones from the byte after it and from the byte after the
// introducer before it; every other reading is a suffix of one of those,
// and charOut counts a short value anywhere. An introducer the decoder
// does not accept as a switch reads as U+FFFD, which is no reading of any
// character, so it starts no reading of its own. A reading identical to
// the bytes as written is that view, read already.
func (d *decoding) modal(enc encoding.Encoding, p []byte, intro byte, isSwitch func([]byte) bool) {
	starts, last := []int{0}, -1
	for i := bytes.IndexByte(p, intro); i >= 0; {
		if isSwitch(p[i:]) {
			starts = append(starts, i+1, last+1)
		}
		last = i
		j := bytes.IndexByte(p[i+1:], intro)
		if j < 0 {
			break
		}
		i += 1 + j
	}
	if len(starts) == 1 {
		return // no switch: the reading is the bytes, high bytes as U+FFFD
	}
	slices.Sort(starts)
	for _, k := range slices.Compact(starts) {
		if !d.s.ok() {
			return
		}
		out, _, _ := transform.Bytes(enc.NewDecoder(), p[k:])
		if bytes.Equal(out, p[k:]) { // exact: the view as written
			continue
		}
		d.joined(out, d.tail) // whole: its other characters are a document's structure and member names
		d.s.m.rel.runs(out, func(r []byte) { d.charOut(r) })
	}
}

// jpSwitch reports whether b starts with an escape Go's ISO-2022-JP
// decoder accepts (to JIS X 0208, JIS X 0212, ASCII, JIS X 0201 Roman or
// Katakana).
func jpSwitch(b []byte) bool {
	// exact: the escapes are three and four bytes long; a shorter tail is none.
	return len(b) >= 3 && b[0] == 0x1b && (b[1] == '$' && (b[2] == '@' || b[2] == 'B') ||
		b[1] == '(' && (b[2] == 'B' || b[2] == 'J' || b[2] == 'I') ||
		// exact: as above.
		len(b) >= 4 && b[1] == '$' && b[2] == '(' && b[3] == 'D')
}

// hzSwitch reports whether b starts with a tilde sequence Go's HZ decoder
// accepts: ~{ and ~} switch modes, ~~ is a tilde, ~ and a line feed is
// nothing.
func hzSwitch(b []byte) bool {
	// exact: a tilde sequence is two bytes long; a shorter tail is none.
	return len(b) >= 2 && b[0] == '~' && strings.IndexByte("{}~\n", b[1]) >= 0
}

// joined: each JSON document's string values joined in document order,
// as the parser decodes them, member names and values of other types left
// out (contract 3, joined values): the view when it is a document, and
// each of its lines that is one (and each run of a binary view that is
// one). A document still open where the view ends
// (one write of a stream, a cut body) is joined over what it holds, its
// unfinished string included. A join is a text view (limits.go, joinText).
//
// atEnd: b ends where the unit ends, so a document open there may continue
// in the stream's next write (jsonDoc reports it).
func (d *decoding) joined(b []byte, atEnd bool) {
	d.jsonDoc(b, atEnd) // a run of a decoded piece is that piece, or more than the contract asks
	if bytes.IndexByte(b, '\n') < 0 {
		return
	}
	// A line before the last ends at its line break: no later write
	// continues it. The last line's starts are starts of b, read above.
	for line := range bytes.SplitSeq(b, []byte{'\n'}) {
		d.jsonDoc(line, false)
	}
}

// jsonDoc joins the JSON documents of b. One can start at every '{' or
// '[', whatever stands before it: a charset reading starts at every byte
// offset and its start is a view boundary (contract 3, Candidates and
// Charsets). A start an earlier parse read as structure begins a part of
// that parse, whose join holds its join, so it is not parsed again. A parse
// after the first spends the budget by the bytes it read.
//
// atEnd: b ends where its view ends, and a document open there is
// reported (closure.open): a start after the last quote joins nothing but
// may still hold one open. A start skipped as structure of an earlier
// parse is inside that parse's document, which is then open too.
func (d *decoding) jsonDoc(b []byte, atEnd bool) {
	end := bytes.LastIndexByte(b, '"') // a start with nothing to join has no string after it
	var opens []uint64                 // a bit for each start an earlier parse read as structure
	marked := func(k int) bool { return opens != nil && opens[k/64]&(1<<(k%64)) != 0 }
	mark := func(at int) {
		if opens == nil {
			opens = make([]uint64, len(b)/64+1)
		}
		opens[at/64] |= 1 << (at % 64)
	}
	nb, nk := -2, -2 // the next '{' and '[' (-1: none; -2: not yet looked for)
	next := func(c byte, at, k int) int {
		if at >= k || at == -1 {
			return at
		}
		if i := bytes.IndexByte(b[k:end], c); i >= 0 {
			return k + i
		}
		return -1
	}
	first := true
starts:
	for k := 0; k < end; k++ {
		nb, nk = next('{', nb, k), next('[', nk, k)
		switch {
		case nb < 0 && nk < 0:
			break starts
		case nk < 0 || nb >= 0 && nb < nk:
			k = nb
		default:
			k = nk
		}
		if marked(k) || !jsonStart(b[k:]) {
			continue
		}
		out, read, open := jsonStrings(b[k:], func(at int) { mark(at + k) })
		if !first && !d.s.charge(read) {
			return
		}
		first = false
		if open && atEnd {
			d.s.open.Store(true)
		}
		d.finalOut(out) // limits.go, joinFinal
	}
	if !atEnd || d.s.open.Load() {
		return
	}
	for k := end + 1; k < len(b); k++ {
		if b[k] != '{' && b[k] != '[' || marked(k) || !jsonStart(b[k:]) {
			continue
		}
		_, read, open := jsonStrings(b[k:], func(at int) { mark(at + k) })
		if !d.s.charge(read) {
			return
		}
		if open {
			d.s.open.Store(true)
			return
		}
	}
}

// jsonStart reports whether t, which starts with '{' or '[', can start a
// document: the parser would read past its first byte.
func jsonStart(t []byte) bool {
	for _, c := range t[1:] {
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			continue
		case t[0] == '{':
			return c == '"' || c == '}'
		default:
			return strings.IndexByte("\"{[]-0123456789tfn", c) >= 0
		}
	}
	return true // open where the view ends
}

// partialStart reports whether c can start a token of an encoding that a
// later write completes (jsonStrings).
func partialStart(c byte) bool {
	return c >= 0x80 || c < 0x20 || c == 0x7f || strings.IndexByte("%\\&=+~", c) >= 0
}

// closedString reports whether s, the inside of a JSON string, holds the
// quote that ends it.
func closedString(s []byte) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return true
		}
	}
	return false
}

// jsonStrings is the string values of the JSON document t joined, as far
// as t is a document or the start of one, how many bytes it read, and
// whether a document is still open where t ends; open is called with the
// offset of each '{' and '[' it reads as structure. A document counts as
// open where t ends when the parse runs out of input inside it, and when
// it stops at a token still being written that a later write may complete
// into structure: a byte that starts an escape, percent-encoding,
// character reference, quoted-printable or MIME word, UTF-7 or HZ
// sequence, a control or non-ASCII byte (a partial character or code
// list), with no space, tab or line feed from it to the end of t.
func jsonStrings(t []byte, open func(int)) ([]byte, int, bool) {
	dec := json.NewDecoder(bytes.NewReader(t))
	dec.UseNumber() // a number out of float64 range (1e1000) is a token, not an error that cuts the join short
	type frame struct{ obj, wantKey bool }
	var stack []frame
	var out []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			read := int(dec.InputOffset()) // at a syntax error: all it buffered
			atEnd := len(stack) > 0
			if se, ok := err.(*json.SyntaxError); ok {
				read = int(se.Offset)
				atEnd = false
				// The offset is after the bad byte for an error inside a
				// value and at it for one between tokens: either may be
				// the token's start. A carriage return does not end the
				// token: quoted-printable's soft line break is = CR LF.
				for _, at := range []int{read - 1, read} {
					if at >= 0 && at < len(t) && partialStart(t[at]) && !bytes.ContainsAny(t[at:], " \t\n") {
						atEnd = true
					}
				}
			}
			if err == io.EOF || len(stack) == 0 || stack[len(stack)-1].obj && stack[len(stack)-1].wantKey {
				return out, read, atEnd
			}
			// An open document: the string it ends inside, unfinished.
			rest := bytes.TrimLeft(t[read:], " \t\r\n,:")
			if len(rest) > 0 && rest[0] == '"' && !closedString(rest[1:]) {
				out = append(out, unescape(rest[1:], false)...)
				read = len(t)
			}
			return out, read, atEnd
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				open(int(dec.InputOffset()) - 1)
				stack = append(stack, frame{obj: t == '{', wantKey: t == '{'})
			} else if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].obj {
					stack[len(stack)-1].wantKey = true
				}
			}
			continue
		case string:
			if len(stack) > 0 && stack[len(stack)-1].obj && stack[len(stack)-1].wantKey {
				stack[len(stack)-1].wantKey = false
				continue // a member name
			}
			out = append(out, t...)
		}
		if len(stack) > 0 && stack[len(stack)-1].obj {
			stack[len(stack)-1].wantKey = true
		}
	}
}
