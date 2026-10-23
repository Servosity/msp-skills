// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import "math/bits"

// Tag numbering.
//
// Every tag comes in two forms. A LEAD tag marks a value that can supply the
// beginning of a filesystem path: the flag value itself, or the value with
// something appended. A COMPONENT tag marks a value that can only appear
// after a separator or after other text the program chose: the name in
// filepath.Join(dir, name), the "x" in dir+"/"+x or in "report-%s.html".
// Concatenation and joining turn the later parts' lead tags into component
// tags; nothing turns a component back into a lead.
//
// The first symbolicSlots indexes are function-local symbolic tags: P(i) is
// "derived from parameter i of the function being summarised" and FV(k) is
// "derived from free variable k of the closure being summarised", each in
// lead (0..63) and component (64..127) form. Every index from symbolicSlots
// up is a global source, two per source: lead at an even offset, component
// at the odd offset after it. Parameters past maxParamSlot share the last
// parameter slot and free variables past maxFreeVarSlot share the last free
// variable slot: merging slots only ever adds flows, never loses one.
const (
	maxParamSlot   = 31
	maxFreeVarSlot = 31
	compOffset     = 64
	symbolicSlots  = 128
)

func paramTag(i int) int {
	if i > maxParamSlot {
		i = maxParamSlot
	}
	return i
}

func freeVarTag(k int) int {
	if k > maxFreeVarSlot {
		k = maxFreeVarSlot
	}
	return 32 + k
}

// symLead maps a symbolic tag (lead or component) to its lead form.
func symLead(t int) int {
	if t >= compOffset && t < symbolicSlots {
		return t - compOffset
	}
	return t
}

func isParamTag(t int) bool   { return t >= 0 && t < symbolicSlots && symLead(t) < 32 }
func isFreeVarTag(t int) bool { return t >= 0 && t < symbolicSlots && symLead(t) >= 32 }

// symIndex is the parameter or free-variable index of a symbolic tag.
func symIndex(t int) int { return symLead(t) % 32 }

// isComponent reports whether t is the component form of its tag.
func isComponent(t int) bool {
	if t < symbolicSlots {
		return t >= compOffset
	}
	return (t-symbolicSlots)%2 == 1
}

// componentOf is the component form of t.
func componentOf(t int) int {
	if isComponent(t) {
		return t
	}
	if t < symbolicSlots {
		return t + compOffset
	}
	return t + 1
}

// sourceLead is the lead tag of the global source t belongs to.
func sourceLead(t int) int { return t - (t-symbolicSlots)%2 }

// tagSet is a growable bitset. The zero value is the empty set.
type tagSet struct {
	w []uint64
}

func (s *tagSet) add(t int) bool {
	i, b := t/64, uint(t%64)
	for len(s.w) <= i {
		s.w = append(s.w, 0)
	}
	if s.w[i]&(1<<b) != 0 {
		return false
	}
	s.w[i] |= 1 << b
	return true
}

func (s tagSet) has(t int) bool {
	i, b := t/64, uint(t%64)
	return i < len(s.w) && s.w[i]&(1<<b) != 0
}

// union adds every tag of o and reports whether s grew.
func (s *tagSet) union(o tagSet) bool {
	changed := false
	for len(s.w) < len(o.w) {
		s.w = append(s.w, 0)
	}
	for i, x := range o.w {
		if s.w[i]|x != s.w[i] {
			s.w[i] |= x
			changed = true
		}
	}
	return changed
}

// unionGlobal adds only the global tags of o.
func (s *tagSet) unionGlobal(o tagSet) bool {
	if len(o.w) <= symbolicWords {
		return false
	}
	changed := false
	for len(s.w) < len(o.w) {
		s.w = append(s.w, 0)
	}
	for i := symbolicWords; i < len(o.w); i++ {
		x := o.w[i]
		if s.w[i]|x != s.w[i] {
			s.w[i] |= x
			changed = true
		}
	}
	return changed
}

func (s tagSet) empty() bool {
	for _, x := range s.w {
		if x != 0 {
			return false
		}
	}
	return true
}

func (s tagSet) hasGlobal() bool {
	for i := symbolicWords; i < len(s.w); i++ {
		if s.w[i] != 0 {
			return true
		}
	}
	return false
}

// symbolicWords is how many bitset words hold the symbolic tags.
const symbolicWords = symbolicSlots / 64

// each calls f for every tag in ascending order.
func (s tagSet) each(f func(int)) {
	for i, x := range s.w {
		for x != 0 {
			b := bits.TrailingZeros64(x)
			f(i*64 + b)
			x &^= 1 << uint(b)
		}
	}
}

func (s tagSet) clone() tagSet {
	if len(s.w) == 0 {
		return tagSet{}
	}
	return tagSet{w: append([]uint64(nil), s.w...)}
}

func (s tagSet) count() int {
	n := 0
	for _, x := range s.w {
		n += bits.OnesCount64(x)
	}
	return n
}

// demoted is t with every lead tag replaced by its component form. Symbolic
// lead tags live in word 0 and their component forms in word 1; a global
// source's lead tag is at an even index and its component at the next odd
// index, always in the same word.
func (s tagSet) demoted() tagSet {
	if len(s.w) == 0 {
		return tagSet{}
	}
	out := tagSet{w: make([]uint64, len(s.w))}
	for i, x := range s.w {
		switch {
		case i == 0:
			if len(out.w) < 2 {
				out.w = append(out.w, 0)
			}
			out.w[1] |= x
		case i == 1:
			out.w[1] |= x
		default:
			const even = 0x5555555555555555
			out.w[i] |= (x &^ even) | ((x & even) << 1)
		}
	}
	return out
}

// asComponentIf returns t demoted when comp is set, else t unchanged.
func asComponentIf(t tagSet, comp bool) tagSet {
	if comp {
		return t.demoted()
	}
	return t
}

// eachSymbolic calls f for every symbolic tag (the first symbolicWords words).
func (s tagSet) eachSymbolic(f func(int)) {
	for i := 0; i < symbolicWords && i < len(s.w); i++ {
		x := s.w[i]
		for x != 0 {
			b := bits.TrailingZeros64(x)
			f(i*64 + b)
			x &^= 1 << uint(b)
		}
	}
}

// globalMinus returns the global tags of s that are not in o.
func (s tagSet) globalMinus(o tagSet) tagSet {
	var out tagSet
	for i := symbolicWords; i < len(s.w); i++ {
		x := s.w[i]
		if i < len(o.w) {
			x &^= o.w[i]
		}
		if x != 0 {
			for len(out.w) <= i {
				out.w = append(out.w, 0)
			}
			out.w[i] = x
		}
	}
	return out
}
