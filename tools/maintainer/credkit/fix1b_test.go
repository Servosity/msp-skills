// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The first adopters' class B: ordinary SQL-shaped units ran out of the
// decoded-bytes budget and were withheld, one of 189 bytes (axcient
// kit-findings item 2), one of 16 (zammad), one of 622 (kaseya-vsa), about 3%
// of units of the shape avanan measured.

var fix1Held = []string{
	"x360-fixture-api-key-0123456789abcdef",
	"Zq7f3a9c0e5b1d42e8f6a7b3c1d9e0f2",
	"nbt-Q7vXk2Lp9RwZm4Ht8JcY3bNf6GsD1aEu",
	"R8v52W4o1n7Q6u3s",
}

const fix1LearnInsert = "INSERT INTO learn_candidates\n\t\t(class, payload, derivation_signature, sightings, status, query_family, command_path, created_at, updated_at, last_seen_at)\n\t\tVALUES (?, ?, ?, 1, 'open', ?, ?, ?, ?, ?)\n\t\tON CONFLICT(derivation_signature) DO UPDATE SET\n\t\t\tsightings = sightings + 1,\n\t\t\tstatus = CASE WHEN learn_candidates.status = 'expired' THEN 'open' ELSE learn_candidates.status END,\n\t\t\tupdated_at = excluded.updated_at,\n\t\t\tlast_seen_at = excluded.last_seen_at\n\t\tWHERE learn_candidates.status != 'rejected'"

// fix1Reported is every unit an adopter reported.
var fix1Reported = []string{
	"flag_alias\n{\"failed_flag\":\"--kind\",\"corrected_flag\":\"--type\",\"command_path\":\"items list\"}\nsig-rej-fa\nfam-norollback",
	fix1LearnInsert + "\nflag_alias\n{\"failed_flag\":\"--kind\",\"corrected_flag\":\"--type\",\"command_path\":\"items list\"}\nsig-rej-fa\nfam-norollback",
	"coveredtaught200",
	"already covered\ntaught\n2026-11-03T20:47:45.006114Z",
	"already covered\n2026-11-03T20:47:45.006114Z",
	"degraded list\ntaught\n2026-11-03T09:12:59.648910Z",
}

// fix1SQLUnits is SQL-shaped units: statement text and bound values, as the
// SQLite sink checks them. The first shape is the one that ran out most often.
func fix1SQLUnits(n int, seed uint64) [][]byte {
	r := rand.New(rand.NewPCG(seed, 7))
	stmts := []string{
		fix1LearnInsert,
		"INSERT OR REPLACE INTO teach_notes (query, notes, source, created_at) VALUES (?, ?, ?, ?)",
		"UPDATE sync_state SET cursor = ?, updated_at = ? WHERE resource = ?",
		"INSERT INTO resources (id, resource_type, data, synced_at) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data",
		"SELECT id, data FROM resources WHERE resource_type = ? ORDER BY synced_at DESC LIMIT ?",
		"",
	}
	words := strings.Fields("already covered taught flag_alias sig-rej-fa fam-norollback items list tickets devices kind type open closed pending backup failed printer offline widget note device agent status healthy degraded customer invoice alert policy firewall")
	flags := strings.Fields("--kind --type --json --limit --status --since --org --site")
	w := func() string { return words[r.IntN(len(words))] }
	var out [][]byte
	for i := range n {
		var b strings.Builder
		if i%2 == 0 {
			fmt.Fprintf(&b, "%s %s\n%s\n2026-11-03T%02d:%02d:%02d.%06dZ", w(), w(), w(), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1000000))
			out = append(out, []byte(b.String()))
			continue
		}
		b.WriteString(stmts[r.IntN(len(stmts))] + "\n")
		for range 1 + r.IntN(4) {
			switch r.IntN(5) {
			case 0:
				fmt.Fprintf(&b, "%s %s\n", w(), w())
			case 1:
				fmt.Fprintf(&b, "2026-%02d-%02dT%02d:%02d:%02d.%06dZ\n", 1+r.IntN(12), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1000000))
			case 2:
				fmt.Fprintf(&b, "{\"failed_flag\":%q,\"corrected_flag\":%q,\"command_path\":\"%s %s\"}\n", flags[r.IntN(len(flags))], flags[r.IntN(len(flags))], w(), w())
			case 3:
				fmt.Fprintf(&b, "{\"id\":%d,\"name\":\"%s-%02d\",\"status\":%q,\"updated_at\":\"2026-10-%02dT%02d:%02d:%02dZ\"}\n", r.IntN(1000000), w(), r.IntN(99), w(), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60))
			case 4:
				b.WriteString(w() + "\n")
			}
		}
		out = append(out, []byte(b.String()))
	}
	return out
}

// TestFix1BReportedUnitsAreClean is every input an adopter reported, with
// every value they held, alone and together.
func TestFix1BReportedUnitsAreClean(t *testing.T) {
	for _, held := range [][]string{fix1Held[:1], fix1Held[1:2], fix1Held[2:3], fix1Held[3:4], fix1Held} {
		resetForTest(Declarations{})
		Hold("minted:x", held...)
		m, _ := currentMatcher()
		for _, u := range fix1Reported {
			if v := m.check([]byte(u)); v != clean {
				t.Errorf("held %d values: %.60q: verdict %d, want clean", len(held), u, v)
			}
		}
	}
}

// TestFix1BOrdinarySQLNeverExhausts fails if ordinary SQL-shaped input ever
// exhausts the budget again.
func TestFix1BOrdinarySQLNeverExhausts(t *testing.T) {
	n := 400 // the race job runs this too
	if testing.Short() {
		n = 100
	}
	resetForTest(Declarations{})
	Hold("minted:x", fix1Held[0])
	m, _ := currentMatcher()
	for _, u := range fix1SQLUnits(n, 1) {
		if v := m.check(u); v != clean {
			t.Errorf("%q: verdict %d, want clean", u, v)
		}
	}
	// A store's connection name, a unit with a random temp directory in it
	// (axcient: one test in four runs could not open its store).
	r := rand.New(rand.NewPCG(1, 1))
	for i := range n / 2 {
		u := fmt.Sprintf("/var/folders/3c/v7xfshz979z_pkc18bwn767c0000gn/T/TestLearningsStats_PrunesBeforeReporting%d/%03d/data.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate", r.IntN(1<<31), 1+i%3)
		if v := m.check([]byte(u)); v != clean {
			t.Errorf("%q: verdict %d, want clean", u, v)
		}
	}
	// The units that ran out before the limits were raised.
	if testing.Short() {
		return
	}
	for _, u := range []string{
		"/var/folders/3c/v7xfshz979z_pkc18bwn767c0000gn/T/TestLearningsStats_PrunesBeforeReporting652517341/001/data.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate",
		"/var/folders/3c/v7xfshz979z_pkc18bwn767c0000gn/T/TestLearningsStats_PrunesBeforeReporting696977885/001/data.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate",
	} {
		if v := m.check([]byte(u)); v != clean {
			t.Errorf("%q: verdict %d, want clean", u, v)
		}
	}
}

// fixedStream writes a raw deflate stream of fixed-Huffman symbols by hand,
// one block that is never ended, so a reader that reaches its end calls it
// broken: its bytes are a final view.
type fixedStream struct {
	buf []byte
	n   uint
	acc uint64
}

func newFixedStream() *fixedStream {
	f := &fixedStream{}
	f.bits(0b010, 3) // not final, fixed Huffman
	return f
}

func (f *fixedStream) bits(v uint64, n uint) {
	f.acc |= v << f.n
	f.n += n
	for f.n >= 8 {
		f.buf = append(f.buf, byte(f.acc))
		f.acc >>= 8
		f.n -= 8
	}
}

// code writes a Huffman code, most significant bit first.
func (f *fixedStream) code(v uint64, n uint) {
	for i := int(n) - 1; i >= 0; i-- {
		f.bits(v>>uint(i)&1, 1)
	}
}

func (f *fixedStream) sym(s int) {
	switch {
	case s < 144:
		f.code(uint64(0b00110000+s), 8)
	case s < 256:
		f.code(uint64(0b110010000+s-144), 9)
	case s < 280:
		f.code(uint64(s-256), 7)
	default:
		f.code(uint64(0b11000000+s-280), 8)
	}
}

func (f *fixedStream) lit(s string) *fixedStream {
	for i := range len(s) {
		f.sym(int(s[i]))
	}
	return f
}

// run writes n more copies of the last period bytes (period 1 to 4), in
// copies of 258.
func (f *fixedStream) run(n, period int) *fixedStream {
	for ; n >= 258; n -= 258 {
		f.sym(285)
		f.code(uint64(period-1), 5)
	}
	for ; n >= 3; n -= 3 {
		f.sym(257)
		f.code(uint64(period-1), 5)
	}
	return f.lit(strings.Repeat("?", n))
}

func (f *fixedStream) bytes() []byte {
	out := append([]byte(nil), f.buf...)
	if f.n > 0 {
		out = append(out, byte(f.acc))
	}
	return out
}

// fix1Tail is a tail that does not repeat, for streams to share.
func fix1Tail() string {
	r := rand.New(rand.NewPCG(11, 12))
	var b strings.Builder
	for b.Len() < 1500 {
		fmt.Fprintf(&b, "%c", 'a'+r.IntN(26))
		fmt.Fprintf(&b, "%d", r.IntN(10))
	}
	return b.String()
}

// fix1Across puts a head before a tail another output in the same view also
// ends in, so the second is cut. Everything is in one part: the streams run
// out where the part does, so both are read to its end, and it is the same
// tail by construction.
func fix1Across(head string) []byte {
	tail := fix1Tail()
	first := newFixedStream().lit("some first head of its own that the second does not have ").lit(tail).bytes()
	second := newFixedStream().lit(head).lit(tail).bytes()
	return append(append(first, 0xFF, 0xFF, 0xFF, 0xFF), second...)
}

func fix1Gzip(t *testing.T, payload string) string {
	t.Helper()
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	w.Write([]byte(payload))
	w.Close()
	return b.String()
}

func fix1RandBytes(n int) string {
	r := rand.New(rand.NewPCG(31, 32))
	b := make([]byte, n)
	for i := range b {
		b[i] = "abcdefghijklmnopqrstuvwxyz0123456789"[r.IntN(36)]
	}
	return string(b)
}

func fix1RandHex(n int) string {
	r := rand.New(rand.NewPCG(33, 34))
	b := make([]byte, n)
	for i := range b {
		b[i] = "0123456789abcdef"[r.IntN(16)]
	}
	return string(b)
}

// fix1Sharing is two broken streams in one part that end in one tail, the
// first with a head of its own and the second with head: the shape the cut
// applies to.
func fix1Sharing(first, head, tail string) []byte {
	a := newFixedStream().lit(first).lit(tail).bytes()
	b := newFixedStream().lit(head).lit(tail).bytes()
	return append(append(a, 0xFF, 0xFF, 0xFF, 0xFF), b...)
}

// fix1AcrossAt puts the first k bytes of s in the second stream's head and
// the rest in the tail both share, so only a read across the cut sees it whole.
func fix1AcrossAt(t *testing.T, s string, k int) []byte {
	t.Helper()
	return fix1Sharing("the first stream's own head", s[:k], s[k:]+fix1Tail())
}

// fix1CodexR1 is Codex round 1's input: eight named references of 33 bytes
// each, which decode to 8 characters of the credential, read only from the
// second stream's head across the shared tail. A window of 264 bytes.
func fix1CodexR1() []byte {
	s := strings.Repeat("&CounterClockwiseContourIntegral;", 8)
	return fix1Sharing("head", "&", s[1:]+strings.Repeat("!", 1400))
}

// fix1AcrossDoc is a JSON document open across the cut: it starts in the
// second head, and its values, which only together carry the credential, are
// deep in the shared tail.
func fix1AcrossDoc() []byte {
	tail := strings.Repeat("padding ", 100) + `secret!","b":"!value"}` + fix1Tail()
	return fix1Sharing("the first stream's own head", `{"a":"`, tail)
}

// TestFix1BCutsHideNothing is the other direction: each credential stands
// where the shortcuts of class B could lose it, and each is still found, with
// the shortcuts on and off.
func TestFix1BCutsHideNothing(t *testing.T) {
	const secret = "secret!!value"
	b64 := base64.StdEncoding.EncodeToString([]byte(secret))
	hx := hex.EncodeToString([]byte(secret))
	var spaced strings.Builder
	for i := 0; i < len(hx); i += 2 {
		spaced.WriteString(hx[i:i+2] + strings.Repeat(" ", 100))
	}
	fill := strings.Repeat("abcdefgh", 100)
	cases := map[string][]byte{
		"plain, in the head":                   fix1Across("head " + secret + " head"),
		"plain, at the head's very end":        fix1Across(secret),
		"base64, in the head":                  fix1Across(b64),
		"base64, across the cut":               fix1Sharing("the first stream's own head", b64[:6], b64[6:]+fix1Tail()),
		"plain, across the join":               fix1Sharing("the first stream's own head", secret[:6], secret[6:]+fix1Tail()),
		"hex with long separators in the head": fix1Across(spaced.String()),
		"percent, in the head":                 fix1Across(strings.ReplaceAll(fmt.Sprintf("%x", secret), "", "")),
		"gzip stream, whole in the head":       fix1Across(fix1Gzip(t, fill+secret)),
		"gzip stream, across the cut":          fix1AcrossAt(t, fix1Gzip(t, fix1RandBytes(600)+secret+fix1RandBytes(300)), 12),
		"JSON document across the cut":         fix1AcrossDoc(),
		"MIME word in the head":                fix1Across("=?utf-8?B?" + b64 + "?="),
		"hex with long separators across the cut": fix1Sharing("the first stream's own head", hex.EncodeToString([]byte(secret[:6])),
			strings.Repeat(" ", 1000)+hex.EncodeToString([]byte(secret[6:]))+fix1Tail()),
		"character references, 33 bytes each (Codex r1)":        fix1CodexR1(),
		"a numeric reference longer than the margin (Codex r2)": fix1Sharing("head", "&", "#x"+fix1RandHex(600)+"00000073;ecret!!"+fix1Tail()),
		"MIME word across the cut": fix1Sharing("the first stream's own head", "=?utf-16le?B?",
			base64.StdEncoding.EncodeToString(utf16LE(strings.Repeat("padding ", 200)+secret))+"?= "+fix1Tail()),
		"a credential after a long run":      newFixedStream().lit("x").run(100000, 1).lit(secret).bytes(),
		"a credential before a long run":     newFixedStream().lit(secret).lit("x").run(100000, 1).bytes(),
		"a credential between periodic runs": newFixedStream().lit("abc").run(50000, 3).lit(secret).lit("def").run(50000, 3).bytes(),
		"base64 run of it after a long run":  newFixedStream().lit("-").run(5000, 1).lit(b64).bytes(),
	}
	defer func() { trimOff = false }()
	held := map[string]string{
		"character references, 33 bytes each (Codex r1)":        strings.Repeat("\u2233", 8),
		"a numeric reference longer than the margin (Codex r2)": "secret!!",
	}
	if fix1CutCount(cases["JSON document across the cut"]) == 0 {
		t.Fatal("fixture: nothing in the shared tail was cut")
	}
	for name, u := range cases {
		for _, off := range []bool{false, true} {
			trimOff = off
			resetForTest(Declarations{})
			h := secret
			if v, ok := held[name]; ok {
				h = v
			}
			Hold("minted:x", h)
			m, _ := currentMatcher()
			if bytes.Contains(u, []byte(h)) {
				t.Fatalf("%s: the fixture carries it as written", name)
			}
			if v := m.check(u); v != leak {
				t.Errorf("%s (shortcut off: %v): verdict %d, want it found", name, off, v)
			}
		}
	}
}

// fix1CutCount runs unit and reports how many bytes of final views the cut
// spared.
func fix1CutCount(u []byte) int64 {
	resetForTest(Declarations{})
	Hold("minted:x", "unrelated-held-value-0123")
	m, _ := currentMatcher()
	trimOff = true
	m.check(u)
	off := spentOn(m, u)
	trimOff = false
	return off - spentOn(m, u)
}

func spentOn(m *matcher, u []byte) int64 {
	m.prepare()
	c := &closure{m: m}
	c.budget.Store(1 << 40)
	c.deflLeft.Store(1 << 40)
	c.done, c.seen = setPool.Get().(*stripedSet), setPool.Get().(*stripedSet)
	c.done.init(2 * len(u))
	c.seen.init(len(u) / 16)
	c.fresh(u, false, 0)
	level := []view{{parts: [][]byte{u}, tail: true}}
	for depth := 1; len(level) > 0 && c.ok(); depth++ {
		level = c.step(level, depth)
	}
	c.release()
	return 1<<40 - c.budget.Load()
}

// TestFix1BCutsChangeNoVerdict: over units built from the shape the cut
// applies to, with the credential spelled and placed at random, the cut never
// turns a found credential into a clean verdict.
func TestFix1BCutsChangeNoVerdict(t *testing.T) {
	defer func() { trimOff = false }()
	const secret = "s3cr3t-Value-77"
	spell := []func(string) string{
		func(s string) string { return s },
		func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		func(s string) string { return hex.EncodeToString([]byte(s)) },
		func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) },
		func(s string) string {
			var b strings.Builder
			for _, c := range []byte(s) {
				fmt.Fprintf(&b, "%%%02X", c)
			}
			return b.String()
		},
	}
	r := rand.New(rand.NewPCG(21, 22))
	cuts := 0
	n := 150
	if testing.Short() {
		n = 30
	}
	for i := range n {
		pre := fix1RandText(r, r.IntN(400))
		post := fix1RandText(r, r.IntN(400))
		sec := spell[r.IntN(len(spell))](secret)
		u := fix1Across(pre + sec + post)
		resetForTest(Declarations{})
		Hold("minted:x", secret)
		m, _ := currentMatcher()
		trimOff = false
		on := m.check(u)
		trimOff = true
		off := m.check(u)
		if on != leak && off == leak {
			t.Errorf("unit %d: found with the cut off, %d with it on", i, on)
		}
		cuts++
	}
	if cuts == 0 {
		t.Fatal("no unit ran")
	}
}

func fix1RandText(r *rand.Rand, n int) string {
	b := make([]byte, n)
	const a = "abcdefghijklmnopqrstuvwxyz ,.:;{}[]\"'0123456789\n"
	for i := range b {
		b[i] = a[r.IntN(len(a))]
	}
	return string(b)
}

// TestFix1BNULRunsDecodeToNothing proves the all-NUL shortcut is exact: a run
// of NUL bytes as a view gives no decoder anything but NUL bytes, with the
// shortcut bypassed by a held value that has a NUL. A value with one still
// finds them.
func TestFix1BNULRunsDecodeToNothing(t *testing.T) {
	for _, n := range []int{4, 5, 6, 7, 8, 9, 16, 17, 256, 4096} {
		unit := make([]byte, n)
		resetForTest(Declarations{})
		Hold("minted:x", "value\x00with-a-NUL") // heldNUL: the views are read
		m, _ := currentMatcher()
		m.prepare()
		if !m.heldNUL {
			t.Fatal("the matcher did not see the NUL")
		}
		c := &closure{m: m}
		c.budget.Store(1 << 30)
		c.deflLeft.Store(1 << 30)
		c.done, c.seen = setPool.Get().(*stripedSet), setPool.Get().(*stripedSet)
		c.done.init(2 * n)
		c.seen.init(n / 16)
		c.fresh(unit, false, 0)
		// What every decoder makes of the run itself. Later levels read the
		// separators the kit puts between pieces too, and those are not the run's.
		for _, v := range c.step([]view{{parts: [][]byte{unit}}}, 1) {
			for _, p := range v.parts {
				for _, b := range p {
					if b != 0 && b != sep {
						t.Fatalf("%d NUL bytes gave a view with %#x: %.40q", n, b, p)
					}
				}
			}
		}
		c.release()
	}
	// A held value that has a NUL is still found in a decoder's output.
	resetForTest(Declarations{})
	Hold("minted:x", "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	if !withheld([]byte(base64.StdEncoding.EncodeToString(make([]byte, 12)))) {
		t.Error("a held run of NUL bytes was not found through base64")
	}
}

// fixedStream writes what a reader reads: a check on the fixture itself.
func TestFix1BFixedStreamFixture(t *testing.T) {
	out, end, _ := inflateFixed(newFixedStream().lit("abc").run(300, 3).lit("def").bytes(), nil, 1<<20, -1)
	if end != inflateBroken || !bytes.HasPrefix(out, []byte("abcabcabc")) || !bytes.HasSuffix(out, []byte("def")) || len(out) != 306 {
		t.Fatalf("end %d, %d bytes: %.20q...%.20q", end, len(out), out, out[max(0, len(out)-20):])
	}
}

// TestFix1BSharedTailIsWhatTheAttemptDecodes checks the proof in fixedCopies
// on real bytes: every attempt the kit adds without decoding it, decoded, is
// the output the kit said it was.
func TestFix1BSharedTailIsWhatTheAttemptDecodes(t *testing.T) {
	defer func() { sharedTailHook = nil }()
	var mu sync.Mutex
	seen := 0
	sharedTailHook = func(stream, pre, rest []byte) {
		out, end, _ := inflateFixed(stream, nil, 1<<24, -1)
		mu.Lock()
		defer mu.Unlock()
		seen++
		if end != inflateBroken || !bytes.Equal(out, append(slices.Clip(pre), rest...)) {
			t.Errorf("an attempt added as %d bytes then %d of another's decodes to %d bytes (end %d)", len(pre), len(rest), len(out), end)
		}
	}
	resetForTest(Declarations{})
	Hold("minted:x", fix1Held[0])
	m, _ := currentMatcher()
	for _, u := range fix1SQLUnits(150, 2) {
		m.check(u)
	}
	for _, u := range fix1Reported {
		m.check([]byte(u))
	}
	// Periodic input with random interruptions, the shape that makes attempts
	// join, whose copies reach all over their output.
	r := rand.New(rand.NewPCG(41, 42))
	for range 200 {
		var b []byte
		for range 2 + r.IntN(6) {
			period := make([]byte, 1+r.IntN(16))
			for i := range period {
				period[i] = byte(r.IntN(256))
			}
			b = append(b, bytes.Repeat(period, 50+r.IntN(400))...)
			b = append(b, 0xFF)
		}
		m.check(b)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen < 100 {
		t.Errorf("only %d attempts went through the shared tail: this proves little", seen)
	}
	t.Logf("%d attempts checked", seen)
}

// fix1Shared drives sharedTail on an attempt with output pre, joined to one
// with output o at ja, whose copies read nothing before ja, and returns
// whether it added the attempt without decoding it, and what it added.
func fix1Shared(t *testing.T, pre, o []byte, ja int) (bool, []byte) {
	t.Helper()
	m, _ := currentMatcher()
	m.prepare()
	c := &closure{m: m}
	c.budget.Store(1 << 30)
	c.deflLeft.Store(1 << 30)
	c.done, c.seen = setPool.Get().(*stripedSet), setPool.Get().(*stripedSet)
	c.done.init(1 << 10)
	c.seen.init(1 << 10)
	defer c.release()
	s := c.worker(0)
	s.defl.reset()
	s.defl.base = append(s.defl.base, 0)
	s.defl.reads = append(s.defl.reads, copyReads{done: true})
	s.defl.from = 0
	d := &decoding{s: s}
	ok := d.sharedTail(nil, pre, o, ja, 0)
	return ok, bytes.Clone(d.bk[0].bjunk)
}

// sharedTail's own guards and margin, on outputs built to need them.
func TestFix1BSharedTailGuards(t *testing.T) {
	const secret = "secret!!value"
	resetForTest(Declarations{})
	Hold("minted:x", secret)
	tail := fix1Tail()
	// It keeps the join: a credential across it is in what it adds.
	ok, added := fix1Shared(t, []byte("some bytes then secret"), []byte("head that differs!!value"+tail), len("head that differs"))
	if !ok || !bytes.Contains(added, []byte(secret)) {
		t.Errorf("a credential across the join: added %v, %.60q", ok, added)
	}
	if ok && bytes.Contains(added, []byte(tail[600:640])) {
		t.Error("the shared tail was added whole")
	}
	// A MIME word before the cut, or filler where the cut would be: not cut.
	if ok, _ := fix1Shared(t, []byte("=?utf-16le?B?"), []byte("other"+tail), len("other")); ok {
		t.Error("a MIME word was left open at a cut")
	}
	if ok, _ := fix1Shared(t, []byte("0a0b"), []byte("other"+strings.Repeat(" ", 1000)+tail), len("other")); ok {
		t.Error("a window of filler was cut")
	}
	// Too little to cut, or nothing the attempt can be proved to be.
	if ok, _ := fix1Shared(t, []byte("pre"), []byte("other"+tail[:100]), len("other")); ok {
		t.Error("a short tail was cut")
	}
	if ok, _ := fix1Shared(t, []byte("pre"), nil, 0); ok {
		t.Error("an attempt with no output to share was cut")
	}
}
