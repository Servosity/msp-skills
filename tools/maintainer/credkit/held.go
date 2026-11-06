// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1: the held set. Every value the program uses or receives as a
// secret is held through Hold (or a kit sink that calls it) under the source
// it came from. Nothing is exempt for its length, strength or type; a source
// is exempt only when Declarations.Identifiers names it with a reason; every
// value stays held until the process exits; and a value held from two
// sources is bound to both sources' origins.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type heldEntry struct {
	allowed []string // Origin.Allowed entries, unioned over every source that held it
	src     Source   // the first source that held it
}

// heldMu is reg's lock. When the held set changed while it was held,
// Unlock waits, after releasing it, for every unit already being put out
// (settle): once a call that holds a value returns, no sink puts out a unit
// checked without it (kit15rv rounds 2 and 3, held-set-update-race, blunt).
// Every sink takes the barrier (emitBegin) before its last check of the
// held set and leaves it (emitEnd) when its write or call returns.
type heldMu struct {
	mu   sync.Mutex
	gen  uint64
	gen0 uint64
}

func (h *heldMu) Lock() { h.mu.Lock(); h.gen0 = h.gen }

func (h *heldMu) Unlock() {
	changed := h.gen != h.gen0
	h.mu.Unlock()
	if changed {
		settle()
	}
}

// emit is every unit being put out, by id, with the epoch it began in.
// settle starts a new epoch and waits only for units of earlier ones, so a
// unit that starts while it waits (a kit Writer writing through another)
// neither waits for it nor holds it up. A unit may carry dirty, which
// reports whether the held set now withholds it: settle waits only for
// units whose dirty reports true, or that carry none. An outbound request
// through a program's own RoundTripper carries one, so a value held inside
// that RoundTripper (a token it mints) does not wait for the request it is
// part of, and an unrelated value does not wait for a long poll.
// ponytail: a sink whose write waits on a goroutine that is changing the
// held set (a program's io.Writer that calls Hold, a pipe whose reader
// holds a value before it reads) waits with it: nothing leaks.
type emitUnit struct {
	epoch uint64
	dirty func() bool
}

var emit = struct {
	sync.Mutex
	cond  *sync.Cond
	epoch uint64
	next  uint64
	units map[uint64]emitUnit
}{units: map[uint64]emitUnit{}}

func init() { emit.cond = sync.NewCond(&emit.Mutex) }

func emitBegin() uint64 { return emitBeginIf(nil) }

func emitBeginIf(dirty func() bool) uint64 {
	emit.Lock()
	defer emit.Unlock()
	emit.next++
	emit.units[emit.next] = emitUnit{emit.epoch, dirty}
	return emit.next
}

func emitEnd(id uint64) {
	emit.Lock()
	defer emit.Unlock()
	delete(emit.units, id)
	emit.cond.Broadcast()
}

func settle() {
	emit.Lock()
	old := emit.epoch
	emit.epoch++
	var wait []uint64
	var check []uint64
	var dirty []func() bool
	for id, u := range emit.units {
		switch {
		case u.epoch > old:
		case u.dirty == nil:
			wait = append(wait, id)
		default:
			check, dirty = append(check, id), append(dirty, u.dirty)
		}
	}
	emit.Unlock()
	for i, d := range dirty { // outside the lock: a check takes time
		if d() {
			wait = append(wait, check[i])
		}
	}
	emit.Lock()
	defer emit.Unlock()
	for _, id := range wait {
		for {
			if _, ok := emit.units[id]; !ok {
				break
			}
			emit.cond.Wait()
		}
	}
}

var reg = struct {
	heldMu
	decl     Declarations
	vals     map[string]*heldEntry
	unknown  bool // a declared credential file exists but could not be read
	m        *matcher
	mgen     uint64
	baseURL  string              // the configured base URL ("base_url" in Origin.Allowed)
	headers  map[string]bool     // header names a [headers] table declares, lowercased
	origins  map[string]*matcher // originMatcher's cache, for generation ogen
	ogen     uint64
	startup  bool     // Start is holding the startup credentials
	recorded []Source // values acquired while running not held: public text
}{heldMu: heldMu{gen: 1}, vals: map[string]*heldEntry{}, headers: map[string]bool{}}

// Hold holds each value under source from now until the process exits. A
// connector calls it for a credential it acquires outside the kit's own
// sources (a token it mints itself: Hold("minted:session", tok)); the kit's
// sinks and the startup prologue call it for everything else.
func Hold(source Source, values ...string) {
	reg.Lock()
	defer reg.Unlock()
	holdLocked(source, values...)
}

// sameSource reports whether a declared source names source: exactly,
// byte for byte (an environment variable or a config key differs by case
// or by a space on its own), except that a header name ignores case.
func sameSource(declared, source Source) bool {
	const h = "header:"
	if len(declared) > len(h) && len(source) > len(h) && strings.EqualFold(declared[:len(h)], h) && strings.EqualFold(source[:len(h)], h) {
		return strings.EqualFold(declared, source)
	}
	return declared == source
}

func exemptLocked(source Source) bool {
	for _, id := range reg.decl.Identifiers {
		if sameSource(id.Source, source) {
			return true
		}
	}
	return false
}

// allowedLocked is source's allowed origins, and whether source has a
// declaration of its own (else they are the "*" default, if any).
func allowedLocked(source Source) ([]string, bool) {
	var def []string
	for _, o := range reg.decl.Origins {
		switch {
		case sameSource(o.Source, source):
			return o.Allowed, true
		case o.Source == "*":
			def = o.Allowed
		}
	}
	return def, false
}

var (
	schemeRE = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)\s+(\S.*)$`)
	paramRE  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
)

func holdLocked(source Source, values ...string) { holdAllowedLocked(source, nil, values...) }

// holdAllowedLocked holds values under source, bound to the source's origins,
// to extra, and, when source has no origin declaration of its own, to the
// origins of any already-held value that is one of their parts: the parts
// of one credential ("Bearer x" and x) are one credential, so they share
// one binding. A declared binding is never widened by another source's.
func holdAllowedLocked(source Source, extra []string, values ...string) {
	if exemptLocked(source) {
		return
	}
	var parts []string
	var pairs []string // a Basic pair held only in part: its binding is still inherited
	part := func(v string) {
		if v != "" && !slices.Contains(parts, v) {
			parts = append(parts, v)
		}
	}
	for _, v := range values {
		part(v)
		t := strings.TrimFunc(v, unicode.IsSpace)
		part(t)
		// A scheme word in front ("Bearer x", "Basic x") is not the secret:
		// the rest is held on its own as well, each auth parameter's value,
		// and a Basic credential's decoded pair and both of its parts
		// (contract 1); with basic-user an Identifier, the user and the pair's
		// pieces inside it are not held (secretSide).
		if m := schemeRE.FindStringSubmatch(t); m != nil {
			rest := strings.TrimFunc(m[2], unicode.IsSpace)
			part(rest)
			// Auth parameters ("Token token=x", "k1=a, k2=\"b\""): each value.
			for _, p := range strings.Split(rest, ",") {
				k, val, ok := strings.Cut(p, "=")
				val = strings.Trim(strings.TrimSpace(val), `"`)
				if ok && paramRE.MatchString(strings.TrimSpace(k)) && strings.Trim(val, "=") != "" {
					part(val)
				}
			}
			if strings.EqualFold(m[1], "basic") {
				if pair, ok := decodeBasic(rest); ok {
					user, pass, _ := strings.Cut(pair, ":")
					part(pass)
					switch side := secretSide(pair, len(user), pass); {
					case !exemptLocked("basic-user"):
						part(pair)
						part(user)
					case side != pair && (reg.startup || !inPublicText(side)):
						part(side)
						pairs = append(pairs, pair)
					default:
						part(pair)
					}
				}
			}
		}
	}
	own, declared := allowedLocked(source)
	base := append(slices.Clone(own), extra...)
	// A source with no declaration of its own (a request header) lends a
	// value it first creates the binding of the parts it carries. It never
	// widens a value already held: an already-held full credential keeps
	// its declared origin and does not inherit a part's. A declared source
	// still unions its own origins into a value held before (two declared
	// sources bind to both).
	var inherited []string
	if !declared {
		for _, p := range slices.Concat(parts, pairs) {
			if e := reg.vals[p]; e != nil {
				inherited = append(inherited, e.allowed...)
			}
		}
	}
	add := func(e *heldEntry, origins []string) {
		for _, a := range origins {
			if !slices.Contains(e.allowed, a) {
				e.allowed = append(e.allowed, a)
				reg.gen++
			}
		}
	}
	for _, p := range parts {
		e := reg.vals[p]
		if e == nil {
			if !reg.startup && inPublicText(p) {
				recordLocked(source) // contract 2: public text acquired while running
				continue
			}
			e = &heldEntry{src: source}
			reg.vals[p] = e
			reg.gen++
			add(e, base)
			if !declared {
				add(e, inherited)
			}
			continue
		}
		if declared {
			add(e, base)
		}
	}
}

// secretSide is the part of a Basic pair (user ":" pass, the user declared an
// Identifier) whose 8-character pieces are the pair's pieces that overlap the
// password (KF-JSM-1): the colon and the password with the fewest characters
// of the user in front that still give every such piece, at least 6 as
// written and 6 normalized. A piece inside the user (its email domain,
// "acmemsp.") is the Identifier's, not a credential's: an answer naming the
// domain is ordinary output. The cut is taken only where the part, normalized,
// ends the normalized pair, so its pieces are pieces of the pair: never more
// than the whole pair holds. It is the pair, held whole as before, when no cut
// does that, when the pair is not valid UTF-8 (contract 2 then counts it by
// bytes) and when the password is under pieceLen characters ("key:x": the
// user half is the secret by its shape).
// ponytail: one normalize per candidate cut, capped at 4*pieceLen user
// characters (then the pair, whole); a user is a short account name.
func secretSide(pair string, userLen int, pass string) string {
	if !utf8.ValidString(pair) || utf8.RuneCountInString(pass) < pieceLen {
		return pair
	}
	n, np := normalize([]byte(pair)), normalize([]byte(pair[userLen:]))
	for i := userLen; i > 0; i-- {
		if !utf8.RuneStart(pair[i]) {
			continue
		}
		raw := utf8.RuneCountInString(pair[i:userLen])
		if raw > 4*pieceLen {
			break
		}
		ns := normalize([]byte(pair[i:]))
		if raw >= pieceLen-2 && utf8.RuneCount(ns)-utf8.RuneCount(np) >= pieceLen-2 && bytes.HasSuffix(n, ns) && bytes.HasSuffix(ns, np) {
			return pair[i:]
		}
	}
	return pair
}

// decodeBasic decodes a Basic credential in any base64 alphabet, padded or
// not, to its "user:password" pair. The pair is kept as bytes when it is not
// valid UTF-8: contract 2 counts such a credential by bytes.
func decodeBasic(s string) (string, bool) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && strings.Contains(string(b), ":") {
			return string(b), true
		}
	}
	return "", false
}

// markUnknown records that a credential source could not be read, so its
// values are unknown: every unit is withheld from now on (fail closed).
func markUnknown() {
	reg.Lock()
	reg.unknown = true
	reg.gen++
	reg.Unlock()
}

// heldValues is every held value.
func heldValues() []string {
	reg.Lock()
	defer reg.Unlock()
	out := make([]string, 0, len(reg.vals))
	for v := range reg.vals {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

// currentMatcher is the matcher for the held set as it is now; nil when
// nothing is held.
func currentMatcher() (*matcher, bool) {
	reg.Lock()
	defer reg.Unlock()
	if reg.unknown {
		return nil, true
	}
	if reg.m == nil || reg.mgen != reg.gen {
		vals := make([]string, 0, len(reg.vals))
		for v := range reg.vals {
			vals = append(vals, v)
		}
		reg.m = newMatcher(vals)
		reg.mgen = reg.gen
	}
	return reg.m, false
}

// originMatcher is the matcher over every held value that may not be sent
// to o (contract 2(c)): each value whose allowed origins do not include o,
// or every value when strict (plain http, or a redirect hop to a different
// origin).
func originMatcher(o origin, strict bool) (*matcher, bool) {
	reg.Lock()
	defer reg.Unlock()
	if reg.unknown {
		return nil, true
	}
	if reg.ogen != reg.gen {
		reg.origins = map[string]*matcher{}
		reg.ogen = reg.gen
	}
	key := fmt.Sprint(strict, o)
	if m, ok := reg.origins[key]; ok {
		return m, false
	}
	var base *origin
	if u, err := url.Parse(strings.TrimSpace(reg.baseURL)); err == nil && reg.baseURL != "" {
		if b, ok := originOf(u); ok {
			base = &b
		}
	}
	var vals []string
	for v, e := range reg.vals {
		if strict || !allowedAt(e.allowed, o, base) {
			vals = append(vals, v)
		}
	}
	m := newRequestMatcher(vals)
	reg.origins[key] = m
	return m, false
}

func allowedAt(allowed []string, o origin, base *origin) bool {
	for _, a := range allowed {
		if a == "base_url" {
			if base != nil && *base == o {
				return true
			}
			continue
		}
		if p, err := parseAllowed(a); err == nil && p.matches(o) {
			return true
		}
	}
	return false
}

// resetForTest empties the held set.
func resetForTest(d Declarations) {
	reg.Lock()
	defer reg.Unlock()
	reg.decl = d
	reg.vals = map[string]*heldEntry{}
	reg.unknown = false
	reg.gen++
	reg.m = nil
	reg.baseURL = ""
	reg.headers = map[string]bool{}
	reg.startup = false
	reg.recorded = nil
}
