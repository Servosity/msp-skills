// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Resolves the named source addresses of an SSL-VPN portal to the set of
// addresses they admit, so "covers every address" is judged on the union of
// the objects (two /1 halves are as open as 0.0.0.0/0) and on address-group
// exclusions (Fortinet's addrgrp exclude-member subtracts from the members).

package fortios

import (
	"net/netip"
	"sort"
	"strings"
)

// AddressBook resolves firewall address and address-group names. Build one
// per family: cmdb firewall/address + addrgrp for IPv4, firewall/address6 +
// addrgrp6 for IPv6. A nil book knows only "all".
type AddressBook struct{ addrs, groups map[string]Obj }

func NewAddressBook(addrs, groups []Obj) *AddressBook {
	b := &AddressBook{addrs: map[string]Obj{}, groups: map[string]Obj{}}
	for _, a := range addrs {
		b.addrs[Str(a, "name")] = a
	}
	for _, g := range groups {
		b.groups[Str(g, "name")] = g
	}
	return b
}

type ipRange struct{ lo, hi netip.Addr }

// rangeSet is a sorted list of disjoint, non-adjacent ranges.
type rangeSet []ipRange

func fullRange(v6 bool) rangeSet {
	if v6 {
		return rangeSet{{netip.IPv6Unspecified(), netip.AddrFrom16([16]byte{0: 0xff, 1: 0xff, 2: 0xff, 3: 0xff, 4: 0xff, 5: 0xff, 6: 0xff, 7: 0xff, 8: 0xff, 9: 0xff, 10: 0xff, 11: 0xff, 12: 0xff, 13: 0xff, 14: 0xff, 15: 0xff})}}
	}
	return rangeSet{{netip.IPv4Unspecified(), netip.AddrFrom4([4]byte{255, 255, 255, 255})}}
}

func (s rangeSet) isFull(v6 bool) bool {
	f := fullRange(v6)[0]
	return len(s) == 1 && s[0].lo == f.lo && s[0].hi == f.hi
}

func union(sets ...rangeSet) rangeSet {
	var all []ipRange
	for _, s := range sets {
		all = append(all, s...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].lo.Less(all[j].lo) })
	var out rangeSet
	for _, r := range all {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if next := last.hi.Next(); !next.IsValid() || !next.Less(r.lo) {
				if last.hi.Less(r.hi) {
					last.hi = r.hi
				}
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// minus removes every address in b from a.
func minus(a, b rangeSet) rangeSet {
	out := rangeSet{}
	for _, r := range a {
		pieces := []ipRange{r}
		for _, x := range b {
			var next []ipRange
			for _, p := range pieces {
				if x.hi.Less(p.lo) || p.hi.Less(x.lo) {
					next = append(next, p)
					continue
				}
				if p.lo.Less(x.lo) {
					next = append(next, ipRange{p.lo, x.lo.Prev()})
				}
				if x.hi.Less(p.hi) {
					next = append(next, ipRange{x.hi.Next(), p.hi})
				}
			}
			pieces = next
		}
		out = append(out, pieces...)
	}
	return out
}

// cover returns the addresses name admits. more=true when the real set may be
// larger (a member could not be resolved), less=true when it may be smaller
// (an exclusion could not be resolved): a name not in the book, a type this
// CLI does not judge, or a group nested too deep.
func (b *AddressBook) cover(name string, v6 bool, depth int) (set rangeSet, more, less bool) {
	if strings.EqualFold(name, "all") {
		return fullRange(v6), false, false
	}
	if b == nil || depth > 8 {
		return nil, true, false
	}
	if a, ok := b.addrs[name]; ok {
		set, unknown := addressCover(a, v6)
		return set, unknown, false
	}
	g, ok := b.groups[name]
	if !ok {
		return nil, true, false
	}
	var in, out rangeSet
	for _, m := range Names(g, "member") {
		c, mo, le := b.cover(m, v6, depth+1)
		in, more, less = union(in, c), more || mo, less || le
	}
	if strings.EqualFold(Str(g, "exclude"), "enable") {
		for _, m := range Names(g, "exclude-member") {
			c, mo, le := b.cover(m, v6, depth+1)
			// An exclusion that may be larger makes the group possibly
			// smaller, and the other way round.
			out, less, more = union(out, c), less || mo, more || le
		}
	}
	return minus(in, out), more, less
}

func addressCover(a Obj, v6 bool) (rangeSet, bool) {
	switch strings.ToLower(Str(a, "type")) {
	case "", "ipmask", "ipprefix":
		raw := Str(a, "subnet")
		if v6 {
			raw = Str(a, "ip6")
		}
		p, ok := parsePrefix(raw)
		if !ok || p.Addr().Is6() != v6 {
			return nil, true
		}
		p = p.Masked()
		return rangeSet{{p.Addr(), lastAddr(p)}}, false
	case "iprange":
		lo, err1 := netip.ParseAddr(Str(a, "start-ip"))
		hi, err2 := netip.ParseAddr(Str(a, "end-ip"))
		if err1 != nil || err2 != nil || lo.Is6() != v6 || hi.Is6() != v6 || hi.Less(lo) {
			return nil, true
		}
		return rangeSet{{lo, hi}}, false
	case "fqdn", "geography", "mac", "interface-subnet", "dynamic":
		// Named hosts, countries, MACs, one interface's subnet or an SDN
		// feed: never judged as covering every address.
		return nil, false
	}
	return nil, true
}

// parsePrefix reads "10.0.0.0 255.0.0.0", "10.0.0.0/8" or "2001:db8::/32".
func parsePrefix(s string) (netip.Prefix, bool) {
	s = strings.TrimSpace(s)
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, true
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return netip.Prefix{}, false
	}
	addr, err := netip.ParseAddr(parts[0])
	n, ok := PrefixLen(s)
	if err != nil || !ok {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, n), true
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for i := range b {
		bit := i * 8
		switch {
		case bit >= p.Bits():
			b[i] = 0xff
		case bit+8 > p.Bits():
			b[i] |= 0xff >> (p.Bits() - bit)
		}
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// sourceAny folds a source list: open is true when the list surely admits
// every address of the family, uncertain when it might but part of it could
// not be resolved. negate inverts the list ("every source except these"):
// open unless the list itself covers every address.
func sourceAny(b *AddressBook, names []string, negate, v6 bool) (open, uncertain bool) {
	var set, sure rangeSet
	more, less := false, false
	for _, n := range names {
		c, mo, le := b.cover(n, v6, 0)
		set, more, less = union(set, c), more || mo, less || le
		if !le {
			// A name with no unresolved exclusion admits at least c, whatever
			// the other names turn out to be.
			sure = union(sure, c)
		}
	}
	full := set.isFull(v6)
	if negate {
		return !full && !more, (full && less) || (!full && more)
	}
	if sure.isFull(v6) {
		return true, false
	}
	return full && !less, (!full && more) || (full && less)
}
