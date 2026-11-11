// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 2 (a) and (b), and contract 3's budget. A unit leaks when, after
// any chain of the decodings in decode.go and charset.go, it carries a held
// value whole (a value under 8 characters only as a whole token) or a
// contiguous piece of one 8 or more characters long. Both sides are compared
// after NFKC and case folding. A character is a code point, or a byte where
// the text is not valid UTF-8. When the depth or decoded-bytes budget runs
// out before every path is checked, the unit is withheld exactly as for a
// match.
//
// The matcher is one multi-pattern automaton (Aho-Corasick) over every
// normalized held value's 8-character pieces, its short values and the
// ROT13 forms of both, run once over each view: its cost is linear in the
// bytes it reads and does not grow with the number of values held. The
// decodings form a closure of views. A view is a list of parts, each
// decoded on its own; a decoder decodes a given part (or, for the
// run-based decoders, a given run) once per unit however many views carry
// it, so near-copies of a view (an escape decoding of a long text) cost only
// what they change. Nothing is pruned by length or by character class: the
// only limits are the contract's depth and decoded-bytes budget.

import (
	"bytes"
	"compress/gzip"
	"hash/maphash"
	"io"
	mbits "math/bits"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type verdict int32

const (
	clean verdict = iota
	leak
	exhausted
)

// pieceLen is contract 2(b)'s piece length, and 2(a)'s whole-token bound.
const pieceLen = 8

type matcher struct {
	ac       *automaton          // nil when every pattern is a word
	words    *wordSet            // the patterns of exactly pieceLen bytes that count anywhere
	numWhole map[string]struct{} // canonical forms of digit-only values
	numWin   map[string]struct{} // 8-digit windows of those forms
	maxLen   int                 // the longest normalized value, in bytes
	short    bool                // a value shorter than pieceLen is held
	heldNUL  bool                // a held value has a NUL byte
	norms    [][]byte
	binary   bool // a held value is not UTF-8 (decoding.part reads non-text parts whole too)

	prep    sync.Once
	rel     relevance    // the characters a charset reading keeps
	tables  []tableGroup // single-byte charsets, grouped by what they read here
	relMask [256]uint64  // per byte: the ASCII-safe groups it is relevant in (all, for relevant ASCII)
	high    []int        // the ASCII-safe groups, by bit
	// per byte above 0x7F: one ASCII-safe group for each way they read it
	distinct [256][]int
	multiO   sync.Once
	multi    *multiIndex // multi-byte charsets

	// prefixes is the first eight bytes of every pattern when the matcher
	// is of constTable's class (consttable.go), else nil.
	prefixes map[uint64]struct{}
	rec      func([]byte) // ConstantTable's: every text matched (nil on every matcher a sink uses)
}

func (m *matcher) prepare() {
	m.prep.Do(func() {
		m.rel = newRelevance(m.norms)
		m.binary = slices.ContainsFunc(m.norms, func(n []byte) bool { return !utf8.Valid(n) })
		m.tables = groupTables(&m.rel)
		for c := 0; c < 0x80; c++ {
			if m.rel.asc(byte(c)) {
				m.relMask[c] = ^uint64(0)
			}
		}
		for gi := range m.tables {
			g := &m.tables[gi]
			if g.asciiSafe && len(m.high) < 64 { // exact: past 64 groups a table is read in full
				for c := 0x80; c < 0x100; c++ {
					if g.char[c] >= 0 {
						m.relMask[c] |= 1 << len(m.high)
					}
				}
				m.high = append(m.high, gi)
			} else {
				g.asciiSafe = false // ponytail: read in full past 64 groups (there are about 30)
			}
			if g.asciiSafe {
				for c := 0x80; c < 0x100; c++ {
					if g.fid[c] >= 0 && !slices.ContainsFunc(m.distinct[c], func(h int) bool { return m.tables[h].fid[c] == g.fid[c] }) {
						m.distinct[c] = append(m.distinct[c], gi)
					}
				}
			}
		}
	})
}

func (m *matcher) multiIndex() *multiIndex {
	m.multiO.Do(func() { m.multi = newMultiIndex(&m.rel) })
	return m.multi
}

func newMatcher(vals []string) *matcher { return buildMatcher(vals, nil, false, false) }

// newWholeMatcher finds each value only whole, by 2(a): the collision
// check with public text, where a piece is no collision.
func newWholeMatcher(vals []string) *matcher { return buildMatcher(vals, nil, false, true) }

// newRequestMatcher is the matcher for an outbound request, judged by
// contract 2(c) alone: a held value in any form, so a short value counts
// anywhere, not only as a whole token.
func newRequestMatcher(vals []string) *matcher { return buildMatcher(vals, nil, true, false) }

// buildMatcher builds the matcher for vals. A value in userCut is a Basic
// pair "user:password" whose user is an Identifier: a piece lying wholly
// inside "user:" is not held, by position alone (KF-JSM-1, userKeep).
func buildMatcher(vals []string, userCut map[string]bool, anywhere, whole bool) *matcher {
	m := &matcher{numWhole: map[string]struct{}{}, numWin: map[string]struct{}{}}
	pats := map[string]bool{} // normalized pattern -> short (whole token only)
	for _, v := range vals {
		m.heldNUL = m.heldNUL || strings.IndexByte(v, 0) >= 0
		n := normalize([]byte(v))
		cs := charStarts(n)
		if !utf8.ValidString(v) {
			cs = byteStarts(n) // contract 2: a character is a byte for a value that is not valid UTF-8
		}
		nc := len(cs) - 1
		if nc <= 0 {
			continue
		}
		// Contract 2 counts characters on the value as held: normalizing can
		// shorten it (a combining mark composes), so its length and its
		// 8-character pieces are taken as written, each piece then
		// normalized, besides the pieces of the normalized value.
		rv := []byte(v)
		rs := charStarts(rv)
		if !utf8.ValidString(v) {
			rs = byteStarts(rv)
		}
		rnc := len(rs) - 1
		m.norms = append(m.norms, n)
		m.maxLen = max(m.maxLen, len(n))
		long := nc >= pieceLen || rnc >= pieceLen
		keepRaw, keepNorm := func(int) bool { return true }, func(int) bool { return true }
		if userCut[v] {
			keepRaw, keepNorm = userKeep(v, rs, cs)
		}
		if long && whole {
			pats[string(n)], pats[rot13s(string(n))] = false, false
		} else if long {
			for i := 0; i+pieceLen <= nc; i++ {
				if !keepNorm(i) {
					continue
				}
				w := string(n[cs[i]:cs[i+pieceLen]])
				pats[w], pats[rot13s(w)] = false, false
			}
			for i := 0; i+pieceLen <= rnc && !bytes.Equal(rv, n); i++ {
				if !keepRaw(i) {
					continue
				}
				w := string(normalize(rv[rs[i]:rs[i+pieceLen]]))
				pats[w], pats[rot13s(w)] = false, false
			}
			if nc < pieceLen {
				pats[string(n)], pats[rot13s(string(n))] = false, false
			}
		} else {
			m.short = true
			for _, w := range []string{string(n), rot13s(string(n))} {
				if _, ok := pats[w]; !ok {
					pats[w] = !anywhere
				}
			}
		}
		if allDigits(n) {
			m.numWhole[canonDigits(n)] = struct{}{}
			for i := 0; i+pieceLen <= len(n) && !whole; i++ {
				m.numWin[string(n[i:i+pieceLen])] = struct{}{} // pieces as written
			}
		}
	}
	if len(pats) == 0 {
		return nil
	}
	// A charset reading keeps the characters of every pattern held, not
	// only of each normalized value: a piece normalized after it is cut
	// can compose a character the whole value does not have.
	for w := range pats {
		m.norms = append(m.norms, []byte(w))
	}
	// Every pattern, before the words leave pats below: constPrefixes
	// takes each pattern's first eight bytes, a word's included.
	m.prefixes = constPrefixes(m, pats)
	// A pattern of exactly pieceLen bytes that counts anywhere (each piece
	// of a long value) is a word in a set, not states of the automaton
	// (#612): the dense automaton took about 200 KB per 130-character
	// cookie, and a sync holds a new cookie from every page. hitIn reads
	// words as the automaton reads patterns, ASCII upper case as lower.
	words := map[uint64]struct{}{}
	for p, short := range pats {
		if len(p) == pieceLen && !short && useWords {
			words[word8([]byte(p))] = struct{}{}
			delete(pats, p)
		}
	}
	if len(words) > 0 {
		m.words = newWordSet(words)
	}
	if len(pats) > 0 {
		m.ac = newAutomaton(pats)
	}
	return m
}

// useWords: tests turn it off to compare with the automaton alone.
var useWords = true

// lowerASCII reads ASCII upper case as lower case, as the automaton's
// classes do.
var lowerASCII = func() (t [256]byte) {
	for i := range t {
		t[i] = byte(i)
	}
	for c := 'A'; c <= 'Z'; c++ {
		t[c] = byte(c + 'a' - 'A')
	}
	return
}()

// word8 is b's pieceLen bytes, ASCII upper case as lower, as one word.
func word8(b []byte) (w uint64) {
	for _, c := range b {
		w = w<<8 | uint64(lowerASCII[c])
	}
	return w
}

// wordSet is a set of words with a bitmap in front of it: most text
// matches no word, and a bitmap bit costs less than a lookup.
type wordSet struct {
	bits  []uint64
	shift uint
	set   map[uint64]struct{}
}

func newWordSet(words map[uint64]struct{}) *wordSet {
	// At least 16 bits per word: a text word that is in no set finds its
	// bit set about one time in 16, and then the set says no.
	n := 1 << 12
	for n < 16*len(words) {
		n <<= 1
	}
	ws := &wordSet{bits: make([]uint64, n/64), shift: uint(64 - mbits.TrailingZeros(uint(n))), set: words}
	for w := range words {
		h := ws.slot(w)
		ws.bits[h/64] |= 1 << (h % 64)
	}
	return ws
}

func (ws *wordSet) slot(w uint64) uint64 { return (w * 0x9E3779B97F4A7C15) >> ws.shift }

func (ws *wordSet) has(w uint64) bool {
	h := ws.slot(w)
	if ws.bits[h/64]&(1<<(h%64)) == 0 {
		return false
	}
	_, ok := ws.set[w]
	return ok
}

// userKeep reports, for the Basic pair v with its user cut, which of its
// 8-character pieces stay held: by index as written (rs, v's character
// starts) and normalized (cs, the normalized pair's). A piece is released
// only when every character of it is a character of "user:", by position:
// as written, it ends at or before the colon's end; normalized, only when the
// normalized pair is the normalized user followed by the normalized ":" and
// password (so each of its characters comes from one side), and the piece
// ends at or before that colon. Any other piece, one with a password
// character in it, stays held exactly as for the pair held whole; so does
// every piece of a pair that is not valid UTF-8 (contract 2 counts it by
// bytes).
func userKeep(v string, rs, cs []int) (raw, norm func(int) bool) {
	all := func(int) bool { return true }
	user, pass, ok := strings.Cut(v, ":")
	if !ok || !utf8.ValidString(v) {
		return all, all
	}
	raw = func(i int) bool { return rs[i+pieceLen] > len(user)+1 }
	nu, nr := normalize([]byte(user)), normalize([]byte(":"+pass))
	if len(nr) == 0 || nr[0] != ':' || !bytes.Equal(normalize([]byte(v)), append(slices.Clip(nu), nr...)) {
		return raw, all
	}
	return raw, func(i int) bool { return cs[i+pieceLen] > len(nu)+1 }
}

// rot13s rotates the ASCII letters of s (a normalized value is lower case).
func rot13s(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		}
	}
	return string(b)
}

// automaton is a deterministic Aho-Corasick automaton over bytes, with
// ASCII upper case read as lower case (the patterns are normalized).
type automaton struct {
	class [256]uint8
	width int
	next  []int32  // state*width + class
	out   []int32  // state -> index into outs, or -1
	outs  [][]apat // the patterns that end at a state
}

type apat struct {
	n     int  // length in bytes
	short bool // contract 2(a): a short value counts only as a whole token
}

func newAutomaton(pats map[string]bool) *automaton {
	a := &automaton{}
	keys := make([]string, 0, len(pats))
	for p := range pats {
		keys = append(keys, p)
	}
	slices.Sort(keys)
	n := 1
	for _, p := range keys {
		for i := 0; i < len(p); i++ {
			if a.class[p[i]] == 0 {
				a.class[p[i]] = uint8(n)
				n++
				if n > 255 {
					n = 255 // ponytail: more than 254 distinct pattern bytes share the last class; a shared class only adds candidates the outputs then reject
				}
			}
		}
	}
	for c := 'A'; c <= 'Z'; c++ {
		a.class[c] = a.class[c+'a'-'A']
	}
	a.width = n
	// The trie.
	type node struct {
		kids map[uint8]int32
		pats []apat
	}
	nodes := []node{{kids: map[uint8]int32{}}}
	for _, p := range keys {
		s := int32(0)
		for i := 0; i < len(p); i++ {
			c := a.class[p[i]]
			nx, ok := nodes[s].kids[c]
			if !ok {
				nx = i32(len(nodes))
				nodes = append(nodes, node{kids: map[uint8]int32{}})
				nodes[s].kids[c] = nx
			}
			s = nx
		}
		nodes[s].pats = append(nodes[s].pats, apat{n: len(p), short: pats[p]})
	}
	// Breadth first: failure links, the full transition table, and each
	// state's outputs merged with its failure state's.
	a.next = make([]int32, len(nodes)*n)
	a.out = make([]int32, len(nodes))
	fail := make([]int32, len(nodes))
	order := []int32{0}
	for i := 0; i < len(order); i++ {
		s := order[i]
		for c := 0; c < n; c++ {
			if nx, ok := nodes[s].kids[uint8(c)]; ok && c != 0 {
				if s != 0 {
					fail[nx] = a.next[int(fail[s])*n+c]
				}
				a.next[int(s)*n+c] = nx
				order = append(order, nx)
			} else if s != 0 {
				a.next[int(s)*n+c] = a.next[int(fail[s])*n+c]
			}
		}
		pats := slices.Clone(nodes[s].pats)
		if s != 0 && a.out[fail[s]] >= 0 {
			pats = append(pats, a.outs[a.out[fail[s]]]...)
		}
		a.out[s] = -1
		if len(pats) > 0 {
			a.out[s] = i32(len(a.outs))
			a.outs = append(a.outs, pats)
		}
	}
	return a
}

// hit reports whether the normalized text n carries a held value.
func (m *matcher) hit(n []byte) bool { return m.hitIn(n, false) }

// hitIn is hit, with a short value counted anywhere when anywhere is set
// (a run decoder's output, every span of which is a view of its own).
func (m *matcher) hitIn(n []byte, anywhere bool) bool {
	if m.rec != nil {
		m.rec(n)
	}
	if ws := m.words; ws != nil {
		var w uint64
		for i, c := range n {
			w = w<<8 | uint64(lowerASCII[c])
			if i >= pieceLen-1 && ws.has(w) {
				return true
			}
		}
	}
	if a := m.ac; a != nil {
		s := int32(0)
		for i, c := range n {
			s = a.next[int(s)*a.width+int(a.class[c])]
			if o := a.out[s]; o >= 0 {
				for _, p := range a.outs[o] {
					if !p.short || anywhere || tokenAt(n, i+1-p.n, i+1) {
						return true
					}
				}
			}
		}
	}
	return len(m.numWhole) > 0 && m.numberHit(n, anywhere)
}

// hitRaw is hit on bytes not yet normalized.
func (m *matcher) hitRaw(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if isASCIIBytes(b) {
		return m.hit(b) // the automaton reads ASCII upper case as lower case
	}
	return m.hit(normalize(b))
}

// tokenAt reports whether n[i:j] is a whole token (contract 2(a)): its
// right side is the end of the text or a character that is not a letter or
// digit. Only the right side is tested: UTF-8, and every charset that reads ASCII
// as itself, start a reading at every byte offset (contract 3, Candidates
// and Charsets), and a reading's start is a view boundary, so a short
// value's left side is always one. Its right side is not: every such
// reading runs to the view's end.
func tokenAt(n []byte, i, j int) bool {
	return i >= 0 && !wordAfter(n[j:])
}

func wordAfter(b []byte) bool {
	r, sz := utf8.DecodeRune(b)
	return sz > 0 && !(r == utf8.RuneError && sz == 1) && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// withheld reports whether unit must be withheld: it carries a held value,
// the budget ran out (contract 3), or the held set is unknown.
func withheld(unit []byte) bool {
	m, unknown := currentMatcher()
	return unknown || m.check(unit) != clean
}

// view is a set of parts decoded at the same depth. Parts are never read
// across each other. A text view is a text reading (of a charset, or a
// binary part's text runs): the matcher and the text decoders read it, the
// charsets and decompressors do not.
type view struct {
	parts [][]byte
	depth int
	text  bool
	any   bool // run decoders' outputs: a short value counts anywhere
	final bool // its decodings are matched, not decoded further (limits.go, brokenText)
	tail  bool // its parts end where the unit ends: a document open there is reported (checkOpen)
	raw   int  // how many headerless raw deflate streams it decodes from inside
}

// closure is one check's state that its workers share: what is decoded
// and queued already, the budget, and the verdict.
type closure struct {
	m        *matcher
	budget   atomic.Int64
	deflLeft atomic.Int64 // bytes left for broken/retry fixed-Huffman decodes
	v        atomic.Int32 // a verdict
	open     atomic.Bool  // a JSON document is open where a view ends (decode.go jsonDoc)
	done     *stripedSet  // (decoder, raw nesting, input) already decoded or queued
	seen     *stripedSet  // parts already queued, by kind and raw nesting (decode.go fresh)
	scans    []*scan      // one per worker
}

// scan is one worker's scratch.
type scan struct {
	*closure
	norm   []byte
	dst    []byte
	inf    []byte
	kept   []byte // sharedTail's scratch
	rbuf   []byte // readAll's read buffer
	gz     *gzip.Reader
	zl, fl io.ReadCloser
	defl   *deflateMemo
	segs   map[string]string // normalized segments (appendNormalized)
	fin    *scan             // readFinal's scan
}

func (c *closure) ok() bool         { return c.v.Load() == int32(clean) }
func (c *closure) stop(v verdict)   { c.v.CompareAndSwap(int32(clean), int32(v)) }
func (c *closure) verdict() verdict { return verdict(c.v.Load()) }

var seeds = [2]maphash.Seed{maphash.MakeSeed(), maphash.MakeSeed()}

func hash2(b []byte) [2]uint64 {
	return [2]uint64{maphash.Bytes(seeds[0], b), maphash.Bytes(seeds[1], b)}
}

// check is the matcher over one unit: leak, exhausted, or clean.
func (m *matcher) check(unit []byte) verdict {
	v, _ := m.checkOpen(unit)
	return v
}

// checkOpen is check, and, for a clean unit, whether a JSON document is
// open where one of its views ends: a stream's next write may continue it,
// so its head must stay in the stream's context (files.go window.add).
// It may report a document no later write can continue (one open where an
// inner view ends): that keeps more context, never less.
func (m *matcher) checkOpen(unit []byte) (verdict, bool) {
	v, open, _, _ := m.checkWork(unit)
	return v, open
}

// checkWork is checkOpen and the work the check did: the decoded bytes it
// drew from the budget and the raw deflate output it drew from its own (the
// tests that hold ordinary output's work linear in its size).
func (m *matcher) checkWork(unit []byte) (_ verdict, _ bool, decoded, deflated int64) {
	if m == nil || len(unit) == 0 {
		return clean, false, 0, 0
	}
	if len(unit) > maxUnit {
		return exhausted, false, 0, 0
	}
	if len(unit) >= bigUnit { // exact: scheduling, every unit is checked in full either way
		defer bigCheck()()
	}
	m.prepare()
	c := &closure{m: m}
	c.budget.Store(int64(budgetPerByte*len(unit) + budgetBase))
	c.deflLeft.Store(int64(min(deflPerByte*len(unit)+deflBase, deflMax)))
	c.done, c.seen = setPool.Get().(*stripedSet), setPool.Get().(*stripedSet)
	c.done.init(2 * len(unit)) // about two inputs per byte: rarely regrown
	c.seen.init(len(unit) / 16)
	b0, d0 := c.budget.Load(), c.deflLeft.Load()
	c.fresh(unit, false, 0)
	defer c.release()
	// Level by level: every part of a level is matched and decoded (its
	// decoders in parallel when the level is large), and what they decode
	// is the next level.
	level := []view{{parts: [][]byte{unit}, tail: true}}
	for depth := 1; len(level) > 0 && c.ok(); depth++ {
		level = c.step(level, depth)
	}
	return c.verdict(), c.open.Load(), b0 - c.budget.Load(), d0 - c.deflLeft.Load()
}

// task is one part's decoders of one family, or all of them.
// task is one large part's decoders of one family, or every decoder over
// a batch of a view's small parts.
type task struct {
	parts [][]byte
	size  int
	v     *view
	fam   int
	tr    *textRuns // shared by a large part's tasks
	d     decoding
}

const (
	bigPart    = 16 << 10 // a part this large runs each decoder family as a task
	parLevel   = 64 << 10 // a level this large runs on several workers
	maxWorkers = 8
)

// step reads one level's views and returns the next level's, depth deep:
// per raw nesting, one view of the byte parts and one of the text
// readings, in task order, each kind's decodings of runs joined into one
// part.
func (c *closure) step(level []view, depth int) []view {
	var tasks []task
	size := 0
	for i := range level {
		v := &level[i]
		batch := -1 // the task collecting this view's small parts
		for _, p := range v.parts {
			size += len(p)
			if len(p) >= bigPart { // exact: scheduling, every part is read either way
				// Every family, whatever the view: decoding.part alone
				// decides which decoders read a part (kit13rv: a final
				// text view lost its UTF-16, UTF-32, gzip and zlib
				// readings here once it reached this size).
				tr := new(textRuns)
				for _, f := range bigFamilies {
					tasks = append(tasks, task{parts: [][]byte{p}, size: len(p), v: v, fam: f, tr: tr})
				}
				continue
			}
			if batch < 0 || tasks[batch].size >= bigPart {
				batch = len(tasks)
				tasks = append(tasks, task{v: v, fam: famAll})
			}
			t := &tasks[batch]
			t.parts, t.size = append(t.parts, p), t.size+len(p)
		}
	}
	run := func(s *scan, t *task) {
		t.d = decoding{s: s, depth: depth, f: t.v.raw, any: t.v.any, final: t.v.final, tail: t.v.tail}
		for _, p := range t.parts {
			t.d.part(p, t.v.text, t.fam, t.tr)
		}
	}
	workers := 1
	if size >= parLevel {
		workers = min(runtime.GOMAXPROCS(0), maxWorkers, len(tasks))
	}
	c.done.par, c.seen.par = workers > 1, workers > 1
	if workers == 1 {
		s := c.worker(0)
		for i := 0; i < len(tasks) && c.ok(); i++ {
			run(s, &tasks[i])
		}
	} else {
		// The largest parts first, so the longest tasks start soonest.
		order := make([]int, len(tasks))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return tasks[order[a]].size > tasks[order[b]].size })
		var next atomic.Int64
		var wg sync.WaitGroup
		for w := range workers {
			s := c.worker(w)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for c.ok() {
					i := int(next.Add(1)) - 1
					if i >= len(order) {
						return
					}
					run(s, &tasks[order[i]])
				}
			}()
		}
		wg.Wait()
	}
	if !c.ok() {
		return nil
	}
	// The level is read: let the collector have it while the next one is
	// assembled (#612).
	for i := range level {
		level[i].parts = nil
	}
	for i := range tasks {
		tasks[i].parts = nil
	}
	var out []view
	for f := 0; f <= maxRawNest; f++ {
		var by, tx, ta, rj, bj, tby, ttx [][]byte
		var tjunk, tany, rjunk, bjunk []byte
		for i := range tasks {
			k := &tasks[i].d.bk[f]
			by, tx = append(by, k.bytes...), append(tx, k.text...)
			tjunk, rjunk = append(tjunk, k.tjunk...), append(rjunk, k.rjunk...)
			tany, bjunk = append(tany, k.tany...), append(bjunk, k.bjunk...)
			tby, ttx = append(tby, k.tbytes...), append(ttx, k.ttext...)
			k.tjunk, k.rjunk, k.tany, k.bjunk = nil, nil, nil, nil // copied: the collector may have them
		}
		if len(bjunk) > 0 && c.fresh(bjunk, true, f) {
			bj = append(bj, bjunk)
		}
		if len(tjunk) > 0 && c.fresh(tjunk, true, f) {
			tx = append(tx, tjunk)
		}
		if len(tany) > 0 && c.fresh(tany, true, f) {
			ta = append(ta, tany)
		}
		if len(rjunk) > 0 && c.fresh(rjunk, false, f) {
			rj = append(rj, rjunk)
		}
		for _, v := range []view{{parts: by}, {parts: tx, text: true}, {parts: ta, text: true, any: true}, {parts: rj, any: true}, {parts: bj, text: true, final: true},
			{parts: tby, tail: true}, {parts: ttx, text: true, tail: true}} {
			if len(v.parts) == 0 {
				continue
			}
			if depth > maxDepth {
				c.stop(exhausted)
				return nil
			}
			v.depth, v.raw = depth, f
			out = append(out, v)
		}
	}
	return out
}

func (c *closure) worker(i int) *scan {
	for len(c.scans) <= i {
		c.scans = append(c.scans, &scan{closure: c, segs: map[string]string{}, defl: memoPool.Get().(*deflateMemo)})
	}
	return c.scans[i]
}

func (c *closure) release() {
	for _, s := range c.scans {
		for _, t := range []*scan{s, s.fin} {
			if t != nil {
				t.defl.reset() // what it holds goes to the collector, not the pool (#612)
				memoPool.Put(t.defl)
			}
		}
	}
	// A set a large unit grew is left to the collector, not pooled: the
	// pool kept the largest set any unit had grown, so a sync's memory
	// ratcheted up with each larger record and stayed there (#612).
	for _, t := range []*stripedSet{c.done, c.seen} {
		slots := 0
		for i := range t.stripes {
			slots += cap(t.stripes[i].keys)
		}
		if slots <= keptSlots { // exact: memory, not reading
			setPool.Put(t)
		}
	}
}

// keptSlots is the most key slots (16 bytes each) a pooled set keeps.
const keptSlots = 1 << 20

// bigUnit: a unit this large is checked one at a time, with the collector
// held close to the live heap while it runs (#612). Such a check holds a
// few hundred bytes of decodings and dedup keys per unit byte at once and
// allocates about a hundred times its live heap, so with the default
// collector the heap ran to twice the live heap, and syncing several
// resources at once added one such check per resource: a 1 GB container
// was killed on a 300 KB ticket. Smaller units run as before.
const bigUnit = 64 << 10

// bigGCPercent is the collector's target while a large unit is checked: a
// heap about a quarter above the live heap, for more collector work.
const bigGCPercent = 25

var bigMu sync.Mutex // a leaf lock: held only while a unit is checked

// bigCheck waits for any other large check to end, tightens the collector,
// and returns what undoes both.
func bigCheck() func() {
	bigMu.Lock()
	old := debug.SetGCPercent(bigGCPercent)
	if old < bigGCPercent { // the program asked for less (or none): keep it
		debug.SetGCPercent(old)
	}
	return func() {
		debug.SetGCPercent(old)
		bigMu.Unlock()
	}
}

var setPool = sync.Pool{New: func() any { return new(stripedSet) }}

var memoPool = sync.Pool{New: func() any { return new(deflateMemo) }}

func (s *scan) hit(p []byte) bool { return s.hitIn(p, false) }

func (s *scan) hitIn(p []byte, anywhere bool) bool {
	if isASCIIBytes(p) {
		return s.m.hitIn(p, anywhere)
	}
	s.norm = appendNormalized(s.norm[:0], p, s.segs)
	return s.m.hitIn(s.norm, anywhere)
}

// charge spends n decoded bytes; past the budget the unit is exhausted.
func (c *closure) charge(n int) bool {
	if !c.ok() {
		return false
	}
	if c.budget.Add(-int64(n)) < 0 {
		c.stop(exhausted)
		return false
	}
	return true
}

// once reports whether decoder d has not yet read b in this unit, and
// records that it now has.
func (c *closure) once(d uint64, b []byte) bool {
	k := hash2(b)
	k[0] ^= d * 0x9E3779B97F4A7C15
	return c.done.add(k)
}

// onceUnless is once for decoder d, skipped (false) when decoder other has
// read b already.
func (c *closure) onceUnless(d, other uint64, b []byte) bool {
	k, o := hash2(b), hash2(b)
	k[0] ^= d * 0x9E3779B97F4A7C15
	o[0] ^= other * 0x9E3779B97F4A7C15
	return c.done.addUnless(k, o)
}

// stripedSet is a hashSet that workers share: 1024 stripes, each under
// its own lock, chosen by the second word of the key. Locking is skipped
// while one worker runs. A stripe is emptied when first used after init,
// so a check pays only for the stripes it touches.
type stripedSet struct {
	par     bool
	gen     uint32
	size    int // a stripe's starting size
	stripes [1 << 10]stripe
}

type stripe struct {
	sync.Mutex
	hashSet
	gen uint32
	_   [128 - 48]byte // one stripe per cache line
}

// init empties the set, sized for about n keys, reusing its memory.
func (t *stripedSet) init(n int) {
	t.gen++
	t.size = max(16, 1<<mbits.Len(uint(n/len(t.stripes))))
}

func (t *stripedSet) lock(k [2]uint64) *stripe {
	st := &t.stripes[k[1]&uint64(len(t.stripes)-1)]
	if t.par {
		st.Lock()
	}
	if st.gen != t.gen {
		st.gen, st.n = t.gen, 0
		if cap(st.keys) >= t.size {
			st.keys = st.keys[:t.size]
			clear(st.keys)
		} else {
			st.keys = make([][2]uint64, t.size)
		}
	}
	return st
}

func (t *stripedSet) unlock(st *stripe) {
	if t.par {
		st.Unlock()
	}
}

func (t *stripedSet) add(k [2]uint64) bool {
	st := t.lock(k)
	ok := st.add(k)
	t.unlock(st)
	return ok
}

// addUnless adds k unless the set has other, under one lock (both share
// the stripe word).
func (t *stripedSet) addUnless(k, other [2]uint64) bool {
	st := t.lock(k)
	ok := !st.has(other) && st.add(k)
	t.unlock(st)
	return ok
}

// hashSet is a set of 128-bit hashes: open addressing, probing from the
// first word, which is already uniform.
type hashSet struct {
	keys [][2]uint64 // the zero key marks an empty slot
	n    int
}

// has reports whether k is in the set.
func (h *hashSet) has(k [2]uint64) bool {
	if k == ([2]uint64{}) {
		k[1] = 1
	}
	if len(h.keys) == 0 {
		return false
	}
	mask := u64(len(h.keys) - 1)
	for i := k[0] & mask; ; i = (i + 1) & mask {
		switch h.keys[i] {
		case k:
			return true
		case [2]uint64{}:
			return false
		}
	}
}

// add reports whether k is new, and records it.
func (h *hashSet) add(k [2]uint64) bool {
	if k == ([2]uint64{}) {
		k[1] = 1
	}
	if 4*h.n >= 3*len(h.keys) {
		old := h.keys
		h.keys, h.n = make([][2]uint64, max(16, 2*len(old))), 0
		for _, o := range old {
			if o != ([2]uint64{}) {
				h.add(o)
			}
		}
	}
	mask := u64(len(h.keys) - 1)
	for i := k[0] & mask; ; i = (i + 1) & mask {
		switch h.keys[i] {
		case k:
			return false
		case [2]uint64{}:
			h.keys[i] = k
			h.n++
			return true
		}
	}
}

// normalize applies NFKC and case folding to each valid UTF-8 stretch of b,
// keeping every invalid byte as itself (contract 3: both sides, before
// comparing).
func normalize(b []byte) []byte {
	if isASCIIBytes(b) {
		if bytes.IndexFunc(b, func(r rune) bool { return r >= 'A' && r <= 'Z' }) < 0 {
			return b
		}
		return appendLower(nil, b)
	}
	return appendNormalized(nil, b, nil)
}

// appendNormalized appends b normalized. A character that normalizes to
// itself and cannot combine with a neighbour (inert) is copied; every other
// stretch goes through NFKC and case folding, from the character before it
// (a combining mark composes with it). segs, when not nil, memoizes the
// stretches.
func appendNormalized(out, b []byte, segs map[string]string) []byte {
	for i := 0; i < len(b); {
		c := b[i]
		if c < utf8.RuneSelf {
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			out = append(out, c)
			i++
			continue
		}
		if c < 0xC2 || c > 0xF4 { // never starts a UTF-8 character
			out = append(out, c)
			i++
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && sz == 1 {
			out = append(out, c)
			i++
			continue
		}
		if inert(r) {
			out = append(out, b[i:i+sz]...)
			i += sz
			continue
		}
		if f, ok := standalone()[r]; ok && (i+sz == len(b) || b[i+sz] < utf8.RuneSelf) {
			out = append(out, f.folded...)
			i += sz
			continue
		}
		start := i
		if i > 0 && b[i-1] < utf8.RuneSelf {
			start = i - 1
			out = out[:len(out)-1]
		}
		j := i + sz
		for j < len(b) {
			r, sz := utf8.DecodeRune(b[j:])
			if r < utf8.RuneSelf || r == utf8.RuneError && sz == 1 || inert(r) {
				break
			}
			j += sz
		}
		if n, ok := segs[string(b[start:j])]; ok {
			out = append(out, n...)
		} else if segs != nil {
			k := len(out)
			out = appendNormSegment(out, b[start:j])
			segs[string(b[start:j])] = string(out[k:])
		} else {
			out = appendNormSegment(out, b[start:j])
		}
		i = j
	}
	return out
}

var foldPool = sync.Pool{New: func() any { return cases.Fold() }}

func appendNormSegment(out, seg []byte) []byte {
	fold := foldPool.Get().(cases.Caser)
	fold.Reset()
	folded, _, _ := transform.Append(fold, nil, norm.NFKC.Bytes(seg))
	foldPool.Put(fold)
	if norm.NFKC.IsNormal(folded) {
		return append(out, folded...)
	}
	return norm.NFKC.Append(out, folded...)
}

// standalone maps each character that normalization changes and that does
// not combine with the character before it to its NFKC form, and to that
// form case folded: followed by ASCII (which nothing combines into), it
// normalizes on its own.
var standalone = sync.OnceValue(func() map[rune]forms {
	out := map[rune]forms{}
	for r, n := range compat() {
		if norm.NFKC.PropertiesString(string(r)).BoundaryBefore() {
			out[r] = forms{nfkc: norm.NFKC.String(string(r)), folded: n}
		}
	}
	return out
})

type forms struct{ nfkc, folded string }

// appendFold appends b case folded (contract 3's case-folding decoder; no
// NFKC), each valid UTF-8 stretch folded and every invalid byte kept.
func appendFold(out, b []byte) []byte {
	fold := foldPool.Get().(cases.Caser)
	defer foldPool.Put(fold)
	for len(b) > 0 {
		n := 0
		for n < len(b) {
			r, sz := utf8.DecodeRune(b[n:])
			if r == utf8.RuneError && sz == 1 {
				break
			}
			n += sz
		}
		fold.Reset()
		out, _, _ = transform.Append(fold, out, b[:n])
		if n < len(b) {
			out = append(out, b[n])
			n++
		}
		b = b[n:]
	}
	return out
}

// appendNFKC appends b in NFKC (contract 3's NFKC decoder; no folding).
func appendNFKC(out, b []byte) []byte {
	start := len(out)
	for i := 0; i < len(b); {
		c := b[i]
		if c < utf8.RuneSelf {
			out = append(out, c)
			i++
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		switch f, ok := standalone()[r]; {
		case r == utf8.RuneError && sz == 1 || inert(r):
			out = append(out, b[i:i+sz]...)
		case ok && (i+sz == len(b) || b[i+sz] < utf8.RuneSelf):
			out = append(out, f.nfkc...)
		default:
			return norm.NFKC.Append(out[:start], b...) // ponytail: the rare interacting case normalizes the whole text
		}
		i += sz
	}
	return out
}

// allInert reports whether normalization leaves b as it is but for ASCII
// case: every character is ASCII, inert, or not valid UTF-8.
func allInert(b []byte) bool {
	for i := 0; i < len(b); {
		if b[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if !(r == utf8.RuneError && sz == 1) && !inert(r) {
			return false
		}
		i += sz
	}
	return true
}

// inert reports whether r normalizes to itself and cannot combine with the
// character before or after it, so normalization copies it.
func inert(r rune) bool {
	if r < 0x10000 {
		t := inertBMP()
		return t[r>>6]&(1<<(r&63)) != 0
	}
	return inertRune(r)
}

var inertBMP = sync.OnceValue(func() *[1 << 10]uint64 {
	var t [1 << 10]uint64
	for r := rune(0x80); r < 0x10000; r++ {
		if inertRune(r) {
			t[r>>6] |= 1 << (r & 63)
		}
	}
	return &t
})

func inertRune(r rune) bool {
	if !utf8.ValidRune(r) {
		return false
	}
	var buf [4]byte
	s := buf[:utf8.EncodeRune(buf[:], r)]
	p := norm.NFKC.Properties(s)
	if !p.BoundaryBefore() || !p.BoundaryAfter() || !norm.NFKC.IsNormal(s) {
		return false
	}
	return !unicode.IsUpper(r) && !unicode.IsTitle(r) && unicode.SimpleFold(r) == r || foldsToItself(r)
}

func foldsToItself(r rune) bool {
	s := string(r)
	f, _, _ := transform.String(cases.Fold(), s)
	return f == s
}

func appendLower(out, b []byte) []byte {
	for _, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return out
}

// charStarts is the offset of every character of n, then len(n).
func charStarts(n []byte) []int {
	cs := make([]int, 0, len(n)+1)
	for i := 0; i < len(n); {
		cs = append(cs, i)
		_, sz := utf8.DecodeRune(n[i:])
		i += sz
	}
	return append(cs, len(n))
}

func byteStarts(n []byte) []int {
	cs := make([]int, len(n)+1)
	for i := range cs {
		cs[i] = i
	}
	return cs
}

func isASCIIBytes(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func allDigits(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// canonDigits is a digit string without its leading zeros.
func canonDigits(b []byte) string {
	i := 0
	for i < len(b)-1 && b[i] == '0' {
		i++
	}
	return string(b[i:])
}

// numberHit reports whether n spells a digit-only held value another way
// (contract 3, Numbers): with leading zeros added or removed, a leading plus
// sign, its digits in groups of any sizes with one separator between two
// groups (comma, full stop, underscore, apostrophe or space, mixed or not),
// or in scientific notation. A spelling of 8 or more characters counts
// anywhere, a shorter one only as a whole token, which a separator between
// two digits does not end. An 8-digit piece counts grouped. anywhere is
// hitIn's: in a reading every span or start offset of which is a candidate
// (a run decoder's output, a charset reading), a shorter spelling counts
// anywhere too.
func (m *matcher) numberHit(n []byte, anywhere bool) bool {
	var dig []byte
	var pos []int
	for i := 0; i < len(n); {
		if !isDigit(n[i]) {
			i++
			continue
		}
		// One run: digits, one separator between two of them.
		start := i
		dig, pos = dig[:0], pos[:0]
		for i < len(n) {
			if isDigit(n[i]) {
				dig, pos = append(dig, n[i]), append(pos, i)
			} else if !isGroupSep(n[i]) || i+1 == len(n) || !isDigit(n[i+1]) {
				break
			}
			i++
		}
		if m.groupedHit(n, start, i, dig, pos, anywhere) {
			return true
		}
	}
	return m.sciHit(n, anywhere)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isGroupSep(c byte) bool { return c == ',' || c == '.' || c == '_' || c == '\'' || c == ' ' }

// groupedHit reads one run, n[start:end], whose digits are dig at pos.
func (m *matcher) groupedHit(n []byte, start, end int, dig []byte, pos []int, anywhere bool) bool {
	plus := start > 0 && n[start-1] == '+'
	if end-start > len(dig) || plus {
		// Grouped: its 8-digit pieces are pieces of a held value (plain
		// digits are pieces as written, which the automaton reads).
		for k := 0; k+pieceLen <= len(dig); k++ {
			if _, ok := m.numWin[string(dig[k:k+pieceLen])]; ok {
				return true
			}
		}
	}
	// The left side is a view boundary at every offset (tokenAt): a
	// spelling may start at any digit of the run and must end it.
	token := end == len(n) || !isAlnum(n[end])
	for h := range m.numWhole {
		for off := 0; ; {
			k := bytes.Index(dig[off:], []byte(h))
			if k < 0 {
				break
			}
			k += off
			off = k + 1
			z := k // the zeros before it are leading zeros
			for z > 0 && dig[z-1] == '0' {
				z--
			}
			lo, hi := pos[z], pos[k+len(h)-1]+1
			if z == 0 && plus {
				lo--
			}
			if hi-lo >= pieceLen || anywhere || token && k+len(h) == len(dig) {
				return true
			}
		}
	}
	return false
}

// sciHit reports whether n holds a held value in scientific notation: a
// mantissa of digits with at most one decimal point, e or E, and an
// exponent with or without a sign, whose value is exactly that integer. A
// spelling is any substring of n that reads so, judged by its own length.
func (m *matcher) sciHit(n []byte, anywhere bool) bool {
	var dig []byte
	var pos []int
	for q := 1; q < len(n); q++ {
		if n[q] != 'e' && n[q] != 'E' {
			continue
		}
		x0, neg := q+1, false
		if x0 < len(n) && (n[x0] == '+' || n[x0] == '-') {
			x0, neg = x0+1, n[x0] == '-'
		}
		x1 := x0
		for x1 < len(n) && isDigit(n[x1]) {
			x1++
		}
		m0, pt := q, -1 // the mantissa's widest start, and its point
		for m0 > 0 && (isDigit(n[m0-1]) || n[m0-1] == '.' && pt < 0) {
			if m0--; n[m0] == '.' {
				pt = m0
			}
		}
		if x1 == x0 || m0 == q || m0 == pt && q == pt+1 {
			continue
		}
		dig, pos = dig[:0], pos[:0]
		before := 0 // digits before the point
		for j := m0; j < q; j++ {
			if isDigit(n[j]) {
				dig, pos = append(dig, n[j]), append(pos, j)
			} else {
				before = len(dig)
			}
		}
		if pt < 0 {
			before = len(dig)
		}
		e := len(dig) // after the last digit that is not 0
		for e > 0 && dig[e-1] == '0' {
			e--
		}
		// pm0 is the spelling's start, a leading plus sign included: the
		// plus is part of the spelling, so it counts toward its length
		// (contract 3, Numbers: "a leading plus sign"). Its left side is a
		// view boundary at every offset (tokenAt).
		pm0 := m0
		if m0 > 0 && n[m0-1] == '+' {
			pm0 = m0 - 1
		}
		// spelled reports whether the mantissa from lo, with the exponent
		// v, spells a held value at its own length.
		spelled := func(lo, v int) bool {
			a, t := max(v, -v), -1
			for j, val := x0, 0; j < x1 && val <= a; j++ {
				if val = min(val*10+int(n[j]-'0'), 1<<30); val == a && (v == 0 || neg == (v < 0)) {
					t = j + 1
				}
			}
			return t > 0 && (t-lo >= pieceLen || anywhere || t == x1 && (x1 == len(n) || !isAlnum(n[x1])))
		}
		for h := range m.numWhole {
			core := strings.TrimRight(h, "0")
			if core == "" { // zero: every all-zero mantissa, any exponent
				if e == 0 && (x1-pm0 >= pieceLen || anywhere || x1 == len(n) || !isAlnum(n[x1])) {
					return true
				}
				continue
			}
			c := e - len(core) // where the core starts, in dig
			if c < 0 || string(dig[c:e]) != core {
				continue
			}
			z := c // the zeros before the core lead
			for z > 0 && dig[z-1] == '0' {
				z--
			}
			lo := pos[z]
			if z == 0 {
				lo = pm0 // the widest start: a leading plus and point too
			}
			zh := len(h) - len(core)
			// Keeping the point: the mantissa starts at or before it.
			if pt >= 0 && lo <= pt && spelled(lo, zh+e-before) {
				return true
			}
			// Without one: it starts after the point, or there is none.
			if pt < 0 && spelled(lo, zh+e-len(dig)) {
				return true
			}
			if pt >= 0 && before <= c && spelled(pos[max(z, before)], zh+e-len(dig)) {
				return true
			}
		}
	}
	return false
}
