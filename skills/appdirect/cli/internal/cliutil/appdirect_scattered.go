// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// appdirect handfix (output-units-reassembled; Codex r1 4.34.2 class credential
// split across output members): the joined views (joinedEchoes) find a held
// value split across neighbouring values only when nothing stands between the
// parts; `{"a":"ABCDEFGH","gap":"x","b":"12345678"}` printed a held
// ABCDEFGH12345678 whole. scatteredEchoes reads the output as a bag of units
// (every JSON string value, every word, every delimiter-separated field, and
// each unit's decoded forms) and reports a held value that units spell when
// put together in ANY order, with anything between them: the tail of one unit,
// then whole units, then the head of another (the outbound check's rule for a
// request's parts, coveredByParts). There is no length floor on a part.
// Instead every part is weighed by how unlikely the output is to hold it by
// chance (its length and alphabet against the number of units of that shape
// the output holds), and a spelling is a leak when its parts, net of the ways
// to cut the value into that many parts (spelledBits), are less likely than
// one in 2^scatterLeakBits, or carry most of the value's own entropy for a
// short value. A small output that spells a held value even in one-character
// parts is withheld; a 20000-row listing whose numbers and letters happen to
// spell a long encoded form of a held value is not, because such parts are
// everywhere in it. A part may stand anywhere inside a unit (any substring,
// weighed against the number of places it could stand); a whole unit, or a
// unit's tail or head, weighs more. Every held form is spelled, the
// alignment encodings holdEncodedCores adds included (with scatterEncodedExtra
// more evidence, for how many there are); a part encoded on its own is also
// decoded as a unit (unitViews).

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"html"
	"index/suffixarray"
	"math"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// scatterLeakBits is the evidence (in bits against chance) a spelling needs:
// with thousands of held forms and many spellings each, chance stays far
// below one in a million.
const scatterLeakBits = 42

// scatterShortShare is the share of a held value's own entropy its parts must
// carry when that entropy is below scatterLeakBits/scatterShortShare.
const scatterShortShare = 0.8

// scatterEncodedExtra is the extra evidence an encoded form (holdEncodedCores:
// every alignment of every encoding of every held value, about a hundred per
// credential) needs, for the extra chances so many forms give.
const scatterEncodedExtra = 8

type scatterPos struct{ target, at int }

// scatterIndex indexes the held values (targets) by every two-character run
// (where a unit may sit inside a target) and by their first two characters
// (where a unit's tail may begin a target).
type scatterIndex struct {
	targets []string
	need    []float64 // bits a spelling of each target needs
	pos2    map[string][]scatterPos
	head2   map[string][]int
}

var scatterCache = struct {
	sync.Mutex
	count int
	last  string
	idx   *scatterIndex
}{}

// scatterIndexFor returns the index of held (held only grows, so its length
// and last value name one set).
func scatterIndexFor(held []string) *scatterIndex {
	last := ""
	if len(held) > 0 {
		last = held[len(held)-1]
	}
	scatterCache.Lock()
	defer scatterCache.Unlock()
	if scatterCache.idx != nil && scatterCache.count == len(held) && scatterCache.last == last {
		return scatterCache.idx
	}
	idx := &scatterIndex{pos2: map[string][]scatterPos{}, head2: map[string][]int{}}
	seen := map[string]bool{}
	for _, h := range held {
		if len(h) < shortMinLen || seen[h] {
			continue
		}
		seen[h] = true
		i := len(idx.targets)
		idx.targets = append(idx.targets, h)
		need := math.Min(scatterLeakBits, scatterShortShare*shapeBits(h))
		if isEncodedForm(h) {
			need += scatterEncodedExtra
		}
		idx.need = append(idx.need, need)
		for p := 0; p+2 <= len(h); p++ {
			idx.pos2[h[p:p+2]] = append(idx.pos2[h[p:p+2]], scatterPos{i, p})
		}
		idx.head2[h[:2]] = append(idx.head2[h[:2]], i)
	}
	scatterCache.count, scatterCache.last, scatterCache.idx = len(held), last, idx
	return idx
}

// shapeClass is the set of character classes s draws from.
func shapeClass(s string) int {
	c := 0
	for i := 0; i < len(s); i++ {
		switch b := s[i]; {
		case '0' <= b && b <= '9':
			c |= 1
		case 'a' <= b && b <= 'z':
			c |= 2
		case 'A' <= b && b <= 'Z':
			c |= 4
		default:
			c |= 8
		}
	}
	return c
}

// shapeBits is log2 of the number of strings of s's length drawn from its
// classes (digits 10, lower 26, upper 26, anything else 33).
func shapeBits(s string) float64 {
	c, n := shapeClass(s), 0.0
	if c&1 != 0 {
		n += 10
	}
	if c&2 != 0 {
		n += 26
	}
	if c&4 != 0 {
		n += 26
	}
	if c&8 != 0 {
		n += 33
	}
	if n == 0 {
		return 0
	}
	return float64(len(s)) * math.Log2(n)
}

type scatterShape struct{ n, class int }

// scatterEdge is one part: target bytes [from, to) and its weight in bits.
type scatterEdge struct {
	from, to int
	bits     float64
}

// scatteredEchoes reports a held value spelled by the units of text in any
// order (see the file comment).
func scatteredEchoes(text string, held []string) bool {
	idx := scatterIndexFor(held)
	if len(idx.targets) == 0 || text == "" {
		return false
	}
	units := outputUnits(text)
	if len(units) == 0 {
		return false
	}
	shapes := map[scatterShape]int{}
	ones, firsts, lasts := map[byte]bool{}, map[byte]bool{}, map[byte]bool{}
	for _, u := range units {
		shapes[scatterShape{len(u), shapeClass(u)}]++
		firsts[u[0]], lasts[u[len(u)-1]] = true, true
		if len(u) == 1 {
			ones[u[0]] = true
		}
	}
	perUnit := math.Log2(float64(len(units)))
	whole := func(f string) float64 {
		return math.Max(0, shapeBits(f)-math.Log2(float64(max(1, shapes[scatterShape{len(f), shapeClass(f)}]))))
	}
	edge := func(f string) float64 { return math.Max(0, shapeBits(f)-perUnit) }

	// Parts of two characters or more.
	edges := map[int][]scatterEdge{}
	for _, u := range units {
		if len(u) >= 2 && idx.unitEdges(u, edges, whole(u), edge) {
			return true
		}
	}
	// A target every part of which may be one character: its inside is
	// one-character units, its first character ends a unit, its last starts
	// one.
	if len(ones) > 0 {
		for i, h := range idx.targets {
			if _, ok := edges[i]; ok || !(ones[h[0]] || lasts[h[0]]) || !(ones[h[len(h)-1]] || firsts[h[len(h)-1]]) {
				continue
			}
			inside := true
			for p := 1; p < len(h)-1 && inside; p++ {
				inside = ones[h[p]]
			}
			if inside {
				edges[i] = nil
			}
		}
	}
	// One-character parts of every target in play: whole one-character
	// units, and a unit that ends with its first character or starts with its
	// last.
	for i := range edges {
		h := idx.targets[i]
		for p := 0; p < len(h); p++ {
			if ones[h[p]] {
				edges[i] = append(edges[i], scatterEdge{p, p + 1, whole(h[p : p+1])})
			}
		}
		if lasts[h[0]] {
			edges[i] = append(edges[i], scatterEdge{0, 1, edge(h[:1])})
		}
		if firsts[h[len(h)-1]] {
			edges[i] = append(edges[i], scatterEdge{len(h) - 1, len(h), edge(h[len(h)-1:])})
		}
	}
	order := fieldOffsets(text)
	// Parts anywhere inside a unit: any substring of any unit, weighed against
	// the number of places in the units such a part could stand by chance.
	// Only a target whose substrings in the units could carry enough evidence
	// is searched (bigram runs bound it from above).
	joined := strings.Join(units, "\x00")
	perWindow := math.Log2(float64(len(joined)))
	sub := func(f string) float64 { return math.Max(0, shapeBits(f)-perWindow) }
	hasBigram := bigramSet(joined)
	var sa *suffixarray.Index
	for i, h := range idx.targets {
		bound := scatterEdgeBound(h, edges[i], order)
		for a := 0; a+2 <= len(h); {
			if !hasBigram(h[a], h[a+1]) {
				a++
				continue
			}
			b := a + 2
			for b < len(h) && hasBigram(h[b-1], h[b]) {
				b++
			}
			bound += sub(h[a:b])
			a = b - 1
		}
		if bound < idx.need[i] {
			continue
		}
		if sa == nil {
			sa = suffixarray.New([]byte(joined))
		}
		longest := 0
		for p := 0; p < len(h); p++ {
			longest = longestIn(sa, h[p:], max(0, longest-1))
			if p == 0 && longest == len(h) {
				return true // the whole target stands inside a unit
			}
			if longest >= 1 {
				edges[i] = append(edges[i], scatterEdge{p, p + 1, 0})
			}
			for l := 2; l <= longest; l++ {
				if w := sub(h[p : p+l]); w > 0 {
					edges[i] = append(edges[i], scatterEdge{p, p + l, w})
				}
			}
		}
	}
	for i, es := range edges {
		if spelledBits(idx.targets[i], es, idx.need[i], order) >= idx.need[i] {
			return true
		}
	}
	return false
}

// scatterEdgeBound bounds the evidence edges give any spelling of h: the
// strongest edge from each position, plus the most order could add.
func scatterEdgeBound(h string, es []scatterEdge, order map[string]int) float64 {
	if len(es) == 0 {
		return 0
	}
	top := make(map[int]float64, len(es))
	unique := map[string]bool{}
	for _, e := range es {
		top[e.from] = math.Max(top[e.from], e.bits)
		if o, ok := order[h[e.from:e.to]]; ok && o >= 0 {
			unique[h[e.from:e.to]] = true
		}
	}
	sum := 0.0
	for _, b := range top {
		sum += b
	}
	if len(unique) >= 3 {
		sum += lgamma(float64(len(unique)+1))/math.Ln2 - 1
	}
	return sum
}

// longestIn is the length of the longest prefix of s that stands in the
// indexed text, at least atLeast (a prefix known to stand there).
func longestIn(sa *suffixarray.Index, s string, atLeast int) int {
	lo, hi := atLeast, len(s)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if len(sa.Lookup([]byte(s[:mid]), 1)) > 0 {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// bigramSet reports the two-byte runs of s.
func bigramSet(s string) func(a, b byte) bool {
	if len(s) < 4096 {
		m := make(map[uint16]bool, len(s))
		for i := 0; i+1 < len(s); i++ {
			m[uint16(s[i])<<8|uint16(s[i+1])] = true
		}
		return func(a, b byte) bool { return m[uint16(a)<<8|uint16(b)] }
	}
	set := make([]bool, 1<<16)
	for i := 0; i+1 < len(s); i++ {
		set[uint16(s[i])<<8|uint16(s[i+1])] = true
	}
	return func(a, b byte) bool { return set[uint16(a)<<8|uint16(b)] }
}

// fieldOffsets maps each delimiter-separated field of text to where it
// stands, or -1 for a field that stands in more than one place (its order
// tells nothing).
func fieldOffsets(text string) map[string]int {
	out := map[string]int{}
	start := -1
	for i, r := range text + " " {
		if unicode.IsSpace(r) || strings.ContainsRune(scatterDelims, r) {
			if start >= 0 {
				f := text[start:i]
				if _, ok := out[f]; ok {
					out[f] = -1
				} else {
					out[f] = start
				}
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

// orderBits is the evidence that parts (in the held value's order) stand in
// the output in that order or its reverse: log2(m!/2) for m parts that each
// stand in one place, the chance of a random arrangement being monotone
// either way being 2/m!. A part repeated in the spelling, or standing in more
// than one place, carries no order and is left out.
func orderBits(parts []string, order map[string]int) float64 {
	uses := map[string]int{}
	for _, p := range parts {
		uses[p]++
	}
	var offs []int
	for _, p := range parts {
		if o, ok := order[p]; ok && o >= 0 && uses[p] == 1 {
			offs = append(offs, o)
		}
	}
	if len(offs) < 3 {
		return 0
	}
	up, down := true, true
	for i := 1; i < len(offs); i++ {
		up = up && offs[i] > offs[i-1]
		down = down && offs[i] < offs[i-1]
	}
	if !up && !down {
		return 0
	}
	return lgamma(float64(len(offs)+1))/math.Ln2 - 1
}

// unitEdges records where unit u (two characters or more, worth wu bits as a
// whole part) can stand in each target; true when u holds a whole target.
func (idx *scatterIndex) unitEdges(u string, edges map[int][]scatterEdge, wu float64, edge func(string) float64) bool {
	// u inside a target (a whole part), or u's head ending a target.
	for _, tp := range idx.pos2[u[:2]] {
		h := idx.targets[tp.target]
		rest := h[tp.at:]
		switch {
		case strings.HasPrefix(rest, u):
			edges[tp.target] = append(edges[tp.target], scatterEdge{tp.at, tp.at + len(u), wu})
		case strings.HasPrefix(u, rest):
			if tp.at == 0 {
				return true // u holds the whole target
			}
			edges[tp.target] = append(edges[tp.target], scatterEdge{tp.at, len(h), edge(rest)})
		}
	}
	// u's tail beginning a target (two characters or more; one is checked
	// per target).
	for q := 1; q+2 <= len(u); q++ {
		for _, i := range idx.head2[u[q:q+2]] {
			h, s := idx.targets[i], u[q:]
			if strings.HasPrefix(s, h) {
				return true
			}
			if strings.HasPrefix(h, s) {
				edges[i] = append(edges[i], scatterEdge{0, len(s), edge(s)})
			}
		}
	}
	return false
}

// spelledBits is the evidence (bits against chance) of the strongest
// spelling of h by the edges, -1 when none spells it. A large output must not
// spell a long held form by coincidence, so:
//   - a part that repeats counts once (the same unit standing at many places
//     in h is one coincidence, not many);
//   - a spelling in k parts is charged log2 C(n-1, k-1) bits, the number of
//     ways to cut h into k parts (a 100-character form cut into 40 short
//     parts, each present in a 20000-row listing, is no evidence);
//   - parts that stand in the output in h's order, or its reverse, earn
//     orderBits: a reader can put them back together.
func spelledBits(h string, es []scatterEdge, need float64, order map[string]int) float64 {
	n := len(h)
	start, end := false, false
	unique := map[string]bool{}
	for _, e := range es {
		start, end = start || e.from == 0, end || e.to == n
		if o, ok := order[h[e.from:e.to]]; ok && o >= 0 {
			unique[h[e.from:e.to]] = true
		}
	}
	if !start || !end {
		return -1
	}
	orderBound := 0.0
	if len(unique) >= 3 {
		orderBound = lgamma(float64(min(len(unique), n)+1))/math.Ln2 - 1
	}
	from := make([][]scatterEdge, n)
	for _, e := range es {
		from[e.from] = append(from[e.from], e)
	}
	// The strongest spelling without the corrections, plus the most order
	// could add, bounds the answer from above: most targets stop here.
	best := make([]float64, n+1)
	for i := range best {
		best[i] = -1
	}
	best[0] = 0
	for p := 0; p < n; p++ {
		if best[p] < 0 {
			continue
		}
		for _, e := range from[p] {
			best[e.to] = math.Max(best[e.to], best[p]+e.bits)
		}
	}
	if best[n] < 0 || best[n]+orderBound < need {
		return best[n]
	}
	// The strongest spelling in exactly k parts, for every k.
	kmax := min(n, 256)
	type cell struct {
		bits float64
		e    scatterEdge
	}
	byK := make([][]cell, kmax+1)
	for k := range byK {
		byK[k] = make([]cell, n+1)
		for p := range byK[k] {
			byK[k][p].bits = -1
		}
	}
	byK[0][0].bits = 0
	for k := 0; k < kmax; k++ {
		for p := 0; p < n; p++ {
			if byK[k][p].bits < 0 {
				continue
			}
			for _, e := range from[p] {
				if b := byK[k][p].bits + e.bits; b > byK[k+1][e.to].bits {
					byK[k+1][e.to] = cell{b, e}
				}
			}
		}
	}
	top := -1.0
	for k := 1; k <= kmax; k++ {
		if byK[k][n].bits < 0 {
			continue
		}
		charge := (lgamma(float64(n)) - lgamma(float64(k)) - lgamma(float64(n-k+1))) / math.Ln2
		if byK[k][n].bits-charge+orderBound <= top {
			continue
		}
		path := make([]string, k)
		bits := map[string]float64{}
		for j, p := k, n; j > 0; j-- {
			e := byK[j][p].e
			path[j-1] = h[e.from:e.to]
			bits[path[j-1]] = math.Max(bits[path[j-1]], e.bits)
			p = e.from
		}
		total := orderBits(path, order) - charge
		for _, b := range bits {
			total += b
		}
		top = math.Max(top, total)
	}
	return top
}

func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// scatterDelims separate fields inside a word: CSV and table cells, query
// strings, cookie and parameter lists, key=value and key:value pairs, and the
// punctuation around a JSON or markup value. Characters that occur inside
// credentials and identifiers (- _ . / + ~ % \ * @ $ ^) are not delimiters.
const scatterDelims = "\"'`,;:&|=()[]{}<>?#!"

func scatterFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(scatterDelims, r) })
}

// outputUnits is every unit a reader takes from text: each whitespace word
// (as written and trimmed of quotes and table punctuation), each
// delimiter-separated field, each JSON string value (and the values of a
// value that is itself JSON) with its words and fields, and each unit's
// decoded forms (unitViews).
func outputUnits(text string) []string {
	seen := map[string]bool{}
	var out []string
	var add func(s string, depth int)
	add = func(s string, depth int) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
		if depth < 2 {
			for _, v := range unitViews(s) {
				add(v, depth+1)
				for _, w := range scatterFields(v) {
					add(w, depth+1)
				}
			}
		}
	}
	pieces := func(s string) {
		for _, w := range strings.Fields(s) {
			add(w, 0)
		}
		for _, w := range words(s) {
			add(w, 0)
		}
		for _, w := range scatterFields(s) {
			add(w, 0)
		}
	}
	pieces(text)
	var leaves func(s string, depth int)
	leaves = func(s string, depth int) {
		for _, leaf := range jsonStringValueList(s) {
			add(leaf, 0)
			pieces(leaf)
			if t := strings.TrimSpace(leaf); depth < 3 && t != "" && (t[0] == '{' || t[0] == '[') {
				leaves(t, depth+1)
			}
		}
	}
	leaves(text, 0)
	return out
}

var scatterB32 = []*base32.Encoding{
	base32.StdEncoding, base32.StdEncoding.WithPadding(base32.NoPadding),
	base32.HexEncoding, base32.HexEncoding.WithPadding(base32.NoPadding),
}

// unitViews decodes one unit the ways the funnel decodes text (escapes,
// entities, markup) and as a whole base64 (four alphabets), base32 (two
// alphabets, either case) or hex value, keeping printable results of two
// characters or more: a part encoded on its own is read too. A decoded single
// character is dropped: nearly every short word decodes to one printable byte
// in some alphabet, so a large output would hold every character that way.
func unitViews(u string) []string {
	var out []string
	keep := func(v string) {
		if v != u && len(v) >= 2 && utf8.ValidString(v) && strings.IndexFunc(v, unicode.IsControl) < 0 {
			out = append(out, v)
		}
	}
	for _, decode := range []func(string) string{
		funnelDropUnprintable, funnelJSONUnescape, funnelIdentityUnescape, funnelPercentDecode, html.UnescapeString, funnelStripTagsText,
	} {
		keep(decode(u))
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(u); err == nil {
			keep(string(b))
		}
	}
	for _, enc := range scatterB32 {
		if b, err := enc.DecodeString(strings.ToUpper(u)); err == nil {
			keep(string(b))
		}
	}
	if len(u)%2 == 0 {
		if b, err := hex.DecodeString(u); err == nil {
			keep(string(b))
		}
	}
	return out
}
