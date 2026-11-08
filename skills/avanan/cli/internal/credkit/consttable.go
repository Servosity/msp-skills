// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Fixed text, checked again in every process: the public-text collision
// check at startup (public.go) and a program's constant SQL statements (a
// store's schema, run on every open). Their decoding closure is the same
// in every process, so a table built once from it (ConstantTable, by
// vendor_credkit.py into a file of the connector's) lets a process read
// their verdict instead of decoding them again. The shortcut is exact:
//
//  1. A matcher of the class (constPrefixes: every pattern 8 or more
//     printable ASCII bytes, no short and no digit-only value, no NUL)
//     has the fixed relevance of newRelevance with nothing added, so its
//     closure of a unit decodes exactly the views of any other matcher of
//     the class: the closure reads the matcher only through rel (and the
//     tables, masks and multi-byte index built from it), binary, heldNUL,
//     short and numWhole (TestKitcpuClassDecodesOneViewSet checks it).
//  2. Such a matcher finds a pattern only through hitIn's automaton or
//     word set: the pattern as a substring of a view, ASCII upper case
//     read as lower.
//     So a pattern found has its first eight bytes, folded, as a window
//     of a view.
//  3. The table holds every such window of every view of each unit it
//     covers, except the windows of the unit itself and of its ROT13,
//     which a process reads from the unit (windowHit). ConstantTable
//     records them from a reference matcher of the class whose closure
//     ran to the end, clean, using under a quarter of each budget (the
//     limits.go notes measure at most twice the variation by schedule),
//     so the same closure under any matcher of the class is not cut by a
//     budget either.
//
// A unit is clean when no pattern's first eight bytes is a window the
// table or the unit holds. Anything else (another class, a window found,
// a unit the table does not cover, another kit, Go, golang.org/x/text or
// limits than the table was built with) checks the unit as before. A
// table holds only text the program carries anyway and windows of its
// decodings: nothing derived from a held value, and it is never written
// at run time.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
)

const constMagic = "credkit-constants 1"

// constRef is the reference matcher's value: of the class, and in no
// covered unit (a unit it occurs in is not covered).
const constRef = "q8Vz3kX1mWj7Lp2Rt9Ys4Nb6Hc0Fd5Gx"

type constTable struct {
	public bool   // covers every entry of publicText
	limits string // limitsKey when built
	units  map[[32]byte]struct{}
	extras map[uint64]struct{}
}

var consts struct {
	sync.Mutex
	tables []*constTable
}

// constEnv is what a table holds only for: this kit's source, the Go
// release and golang.org/x/text version the program is built with.
var constEnv = sync.OnceValue(func() string {
	x := "none"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "golang.org/x/text" {
				x = d.Version
				if d.Replace != nil {
					x = d.Replace.Path + "@" + d.Replace.Version
				}
			}
		}
	}
	return kitSourceHash + " " + runtime.Version() + " " + x
})

func limitsKey() string {
	return fmt.Sprint(maxDepth, budgetPerByte, budgetBase, deflPerByte, deflBase, deflMax, maxUnit)
}

// constPrefixes is the folded first eight bytes of every pattern, or nil
// when the matcher is not of the class.
func constPrefixes(m *matcher, pats map[string]bool) map[uint64]struct{} {
	if m.short || len(m.numWhole) > 0 || m.heldNUL {
		return nil
	}
	for _, n := range m.norms { // the patterns are among them
		for _, c := range n {
			if !asciiRelevant(c) {
				return nil
			}
		}
	}
	out := map[uint64]struct{}{}
	for p := range pats {
		if len(p) < pieceLen { // exact: the class's patterns are 8 bytes or more (a shorter one is not of the class)
			return nil
		}
		out[packWindow([]byte(p))] = struct{}{}
	}
	return out
}

// packWindow is b's first eight bytes, ASCII upper case folded.
func packWindow(b []byte) uint64 {
	var w [pieceLen]byte
	for i := range w {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		w[i] = c
	}
	return binary.LittleEndian.Uint64(w[:])
}

// addWindows adds every eight-byte window of b, folded, that is printable
// ASCII or whitespace: no pattern of the class has any other byte.
func addWindows(set map[uint64]struct{}, b []byte) {
	run := 0
	for i := range b {
		if asciiRelevant(b[i]) {
			run++
		} else {
			run = 0
		}
		if run >= pieceLen {
			set[packWindow(b[i+1-pieceLen:])] = struct{}{}
		}
	}
}

// windowHit reports whether a window of u or of its ROT13 is one of m's
// prefixes.
func (m *matcher) windowHit(u []byte) bool {
	r := []byte(rot13s(string(u)))
	for i := 0; i+pieceLen <= len(u); i++ {
		if _, ok := m.prefixes[packWindow(u[i:])]; ok {
			return true
		}
		if _, ok := m.prefixes[packWindow(r[i:])]; ok {
			return true
		}
	}
	return false
}

// constClean reports whether each of units is clean for m by a table that
// covers them all (public: publicText's entries, else by their hashes).
func (m *matcher) constClean(public bool, units ...[]byte) bool {
	if m == nil || m.prefixes == nil {
		return false
	}
	lim := limitsKey()
	consts.Lock()
	tables := consts.tables
	consts.Unlock()
next:
	for _, t := range tables {
		if t.limits != lim || public && !t.public {
			continue
		}
		if !public {
			for _, u := range units {
				if _, ok := t.units[sha256.Sum256(u)]; !ok {
					continue next
				}
			}
		}
		for p := range m.prefixes {
			if _, ok := t.extras[p]; ok {
				return false
			}
		}
		for _, u := range units {
			if m.windowHit(u) {
				return false
			}
		}
		return true
	}
	return false
}

// constUnitClean is constClean for one unit under the current held set.
func constUnitClean(u []byte) bool {
	m, unknown := currentMatcher()
	return !unknown && m.constClean(false, u)
}

// ConstantTable builds a table of units, fixed text the program checks
// each as a whole unit (its constant SQL statements), and of publicText
// when public: the argument to RegisterConstants. A unit the reference
// matcher's closure does not finish clean within a quarter of each budget
// is left out; a public text entry left out is an error.
func ConstantTable(units []string, public bool) (string, error) {
	ref := newMatcher([]string{constRef})
	if ref == nil || ref.prefixes == nil {
		return "", errors.New("credkit: the reference value is not of the table's class")
	}
	var mu sync.Mutex
	seen := map[uint64]struct{}{}
	ref.rec = func(n []byte) {
		mu.Lock()
		addWindows(seen, n)
		mu.Unlock()
	}
	extras := map[uint64]struct{}{}
	add := func(u string) bool {
		clear(seen)
		v, _, dec, defl := ref.checkWork([]byte(u))
		budget := int64(budgetPerByte*len(u) + budgetBase)
		deflAllow := int64(min(deflPerByte*len(u)+deflBase, deflMax))
		if v != clean || 4*dec > budget || 4*defl > deflAllow {
			return false
		}
		own := map[uint64]struct{}{}
		addWindows(own, []byte(u))
		addWindows(own, []byte(rot13s(u)))
		for w := range seen {
			if _, ok := own[w]; !ok {
				extras[w] = struct{}{}
			}
		}
		return true
	}
	if public {
		for _, e := range publicText() {
			if !add(e) {
				return "", fmt.Errorf("credkit: public text entry %q is not covered", e)
			}
		}
	}
	var hashes [][32]byte
	for _, u := range units {
		if add(u) {
			hashes = append(hashes, sha256.Sum256([]byte(u)))
		}
	}
	slices.SortFunc(hashes, func(a, b [32]byte) int { return slices.Compare(a[:], b[:]) })
	hashes = slices.Compact(hashes)
	ws := make([]uint64, 0, len(extras))
	for w := range extras {
		ws = append(ws, w)
	}
	slices.Sort(ws)
	// the counts are uint32 fields; gosec G115 accepts a guard on a variable, not on len()
	nh, nw := len(hashes), len(ws)
	if nh > math.MaxUint32 || nw > math.MaxUint32 {
		return "", fmt.Errorf("credkit: a constants table holds at most %d records of each kind", uint32(math.MaxUint32))
	}
	var p []byte
	p = binary.LittleEndian.AppendUint32(p, uint32(nh))
	for _, h := range hashes {
		p = append(p, h[:]...)
	}
	p = binary.LittleEndian.AppendUint32(p, uint32(nw))
	for _, w := range ws {
		p = binary.LittleEndian.AppendUint64(p, w)
	}
	kind := "units"
	if public {
		kind = "public"
	}
	return strings.Join([]string{constMagic, constEnv(), limitsKey(), kind, base64.StdEncoding.EncodeToString(p)}, "\n"), nil
}

// RegisterConstants adds a table ConstantTable built: a connector's
// generated file calls it from init. A table built for another kit, Go
// release or golang.org/x/text version, or that does not parse, is ignored.
func RegisterConstants(table string) {
	f := strings.Split(table, "\n")
	if len(f) != 5 || f[0] != constMagic || f[1] != constEnv() || f[3] != "public" && f[3] != "units" {
		return
	}
	p, err := base64.StdEncoding.DecodeString(f[4])
	if err != nil {
		return
	}
	t := &constTable{public: f[3] == "public", limits: f[2], units: map[[32]byte]struct{}{}, extras: map[uint64]struct{}{}}
	n, p, ok := takeCount(p, 32)
	if !ok {
		return
	}
	for range n {
		t.units[[32]byte(p[:32])] = struct{}{}
		p = p[32:]
	}
	n, p, ok = takeCount(p, 8)
	if !ok || len(p) != 8*n {
		return
	}
	for range n {
		t.extras[binary.LittleEndian.Uint64(p)] = struct{}{}
		p = p[8:]
	}
	consts.Lock()
	consts.tables = append(consts.tables, t)
	consts.Unlock()
}

// takeCount reads a count of records of size bytes and checks they follow.
func takeCount(p []byte, size int) (int, []byte, bool) {
	if len(p) < 4 { // exact: a table's count field, not a unit
		return 0, nil, false
	}
	n := int(binary.LittleEndian.Uint32(p))
	p = p[4:]
	return n, p, n <= len(p)/size
}
