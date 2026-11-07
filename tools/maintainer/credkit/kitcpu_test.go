// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The kitcpu bounds (2026-11-06, runzero's black box past CI's ten
// minutes): a held credential cost every process about half a CPU-second
// at startup (the public-text collision check) and about as much again on
// every store open (the decoding closure over each constant DDL statement).
// Both are fixed text: consttable.go reads their verdict from a table. The
// rest of this file holds the leak side where it was: a held value in
// public text, in a constant statement and in a bound value, in every
// encoding the kit decodes, is caught exactly as without the table.
//
// TestMain registers kitcpuTable for every test of the package, so every
// other test runs with the shortcut on.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/maphash"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

// kitcpuCreds is runzero's black box's held set: an API key and the MCP
// HTTP token.
var kitcpuCreds = []string{"CT0lyurIer8NpgQjFrecCy0ArZsoJGyx", "wBoIFpg9YLzVxopyW3mFzo3OjNVEGIQPqq"}

// kitcpuLeak is the value the leak-side tests hold; the table is built
// without it.
const kitcpuLeak = "Zq7HmP3kL9wXv2NsRt"

// kitcpuDDL is a press store's schema statements (runzero's, 2026-11-06).
var kitcpuDDL = []string{
	`CREATE TABLE IF NOT EXISTS resources (
			id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			data JSON NOT NULL,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (resource_type, id)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_resources_type ON resources(resource_type)`,
	`CREATE INDEX IF NOT EXISTS idx_resources_type_updated ON resources(resource_type, updated_at)`,
	`CREATE TABLE IF NOT EXISTS sync_state (
			resource_type TEXT PRIMARY KEY,
			last_cursor TEXT,
			last_synced_at DATETIME,
			total_count INTEGER DEFAULT 0,
			last_attempt_complete INTEGER NOT NULL DEFAULT 0,
			scope_key TEXT NOT NULL DEFAULT ''
		)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS resources_fts USING fts5(
	id, resource_type, content, tokenize='trigram'
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_learn_unique ON search_learnings(query_pattern, resource_id, action)`,
	`PRAGMA user_version`,
}

// kitcpuBound is a constant statement with a bound value.
const kitcpuBound = `INSERT INTO resources (id, resource_type, data) VALUES (?, 'asset', ?)`

// kitcpuEncodings is v in every encoding the kit decodes that a statement's
// text can carry, by name.
func kitcpuEncodings(v string) map[string]string {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	_, _ = w.Write([]byte(v))
	_ = w.Close()
	each := func(f string) string {
		var b strings.Builder
		for i := 0; i < len(v); i++ {
			fmt.Fprintf(&b, f, v[i])
		}
		return b.String()
	}
	var u16 []byte
	for _, c := range utf16.Encode([]rune(v)) {
		u16 = append(u16, byte(c), byte(c>>8))
	}
	var full []rune
	for _, c := range v {
		full = append(full, c+0xFEE0)
	}
	codes := make([]string, len(v))
	for i := 0; i < len(v); i++ {
		codes[i] = fmt.Sprint(v[i])
	}
	b64 := base64.StdEncoding.EncodeToString
	return map[string]string{
		"plain":         v,
		"upper":         strings.ToUpper(v),
		"rot13":         rot13s(v),
		"fullwidth":     string(full),
		"base64":        b64([]byte(v)),
		"base64url":     base64.RawURLEncoding.EncodeToString([]byte(v)),
		"base32":        base32.StdEncoding.EncodeToString([]byte(v)),
		"hex":           hex.EncodeToString([]byte(v)),
		"percent":       each("%%%02X"),
		"qp":            each("=%02X"),
		"html":          each("&#%d;"),
		"jsescape":      each(`\u%04x`),
		"mime":          "=?UTF-8?B?" + b64([]byte(v)) + "?=",
		"codes":         strings.Join(codes, ","),
		"utf16le":       string(u16),
		"gzip":          gz.String(),
		"base64-gzip":   b64(gz.Bytes()),
		"base64-base64": b64([]byte(b64([]byte(v)))),
		"hex-rot13":     hex.EncodeToString([]byte(rot13s(v))),
	}
}

// kitcpuStatement is a constant statement carrying enc.
func kitcpuStatement(enc string) string {
	return `INSERT INTO resources (id, resource_type, data) VALUES ('x', 'asset', '` + enc + `')`
}

// kitcpuUnits is every unit kitcpuTable covers besides public text.
func kitcpuUnits() []string {
	u := append(slices.Clone(kitcpuDDL), kitcpuBound)
	for _, e := range kitcpuEncodings(kitcpuLeak) {
		u = append(u, kitcpuStatement(e))
	}
	return u
}

var kitcpuTable = sync.OnceValue(func() string {
	t, err := ConstantTable(kitcpuUnits(), true)
	if err != nil {
		panic(err)
	}
	return t
})

// withoutTables runs f as the kit ran before the table: no table
// registered.
func withoutTables(f func()) {
	consts.Lock()
	saved := consts.tables
	consts.tables = nil
	consts.Unlock()
	defer func() {
		consts.Lock()
		consts.tables = saved
		consts.Unlock()
	}()
	f()
}

// cpuBound fails when f's CPU time, averaged over n runs after one warm-up
// run, exceeds ms milliseconds.
func cpuBound(t *testing.T, what string, ms float64, n int, f func()) {
	t.Helper()
	f()
	c0 := cpuTime()
	for range n {
		f()
	}
	if d := (cpuTime() - c0) / float64(n); d > ms {
		t.Errorf("%s: %.2f CPU-ms per run, want at most %.1f", what, d, ms)
	}
}

func startupVerdict(vals ...string) bool {
	resetForTest(Declarations{})
	atStartup(func() { Hold("env:K", vals...) })
	reg.Lock()
	defer reg.Unlock()
	_, refused := startupCollisionLocked()
	return refused
}

func TestKitcpuStartupCheckIsCheap(t *testing.T) {
	cpuBound(t, "startup public-text check", 10, 5, func() {
		if startupVerdict(kitcpuCreds...) {
			t.Fatal("refused")
		}
	})
}

// kitcpuDB is a database through SQLDriver over a recording driver.
func kitcpuDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlTestRuns++
	name := fmt.Sprintf("credkit-test-%d", sqlTestRuns)
	sql.Register(name, SQLDriver(&recDriver{}))
	db, err := sql.Open(name, "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestKitcpuConstantSQLIsCheap(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:RUNZERO_API_KEY", kitcpuCreds...)
	db := kitcpuDB(t)
	for _, q := range kitcpuDDL {
		if !constUnitClean([]byte(q)) {
			t.Errorf("the table did not decide %q", q)
		}
	}
	cpuBound(t, "a store's schema statements", 1, 10, func() {
		for _, q := range kitcpuDDL {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// TestKitcpuStartupVerdictUnchanged: a held value in public text, as
// written or as the kit decodes it, still refuses startup.
func TestKitcpuStartupVerdictUnchanged(t *testing.T) {
	// Every entry, and the forms of it the matcher reads as the entry: the
	// shortcut does not decide them, so the check runs as before.
	n := 0
	for _, e := range publicText() {
		var full []rune
		for _, c := range e {
			full = append(full, c+0xFEE0)
		}
		for _, v := range []string{e, strings.ToUpper(e), rot13s(e), string(full)} {
			m := newWholeMatcher([]string{v})
			if m == nil || m.prefixes == nil {
				continue // not of the class: checked as before
			}
			n++
			if m.constClean(true, publicUnits()...) {
				t.Errorf("held %q, in public text: the table called startup clean", v)
			}
		}
	}
	if n < 500 {
		t.Errorf("only %d held values of the class tried", n)
	}
	// What the closure decodes out of public text (not the text itself):
	// each window it decodes, held, is refused with the table as without.
	ref := newMatcher([]string{constRef})
	var mu sync.Mutex
	decoded := map[string]bool{}
	ref.rec = func(b []byte) {
		mu.Lock()
		defer mu.Unlock()
		run := 0
		for i := range b {
			if b[i] >= 'a' && b[i] <= 'z' || b[i] >= '0' && b[i] <= '9' {
				run++
			} else {
				run = 0
			}
			if run >= 10 {
				decoded[string(b[i+1-10:i+1])] = true
			}
		}
	}
	plain := strings.ToLower(strings.Join(publicText(), "\x00"))
	for _, e := range publicText() {
		ref.check([]byte(e))
	}
	var vals []string
	for v := range decoded {
		if !strings.Contains(plain, v) && !strings.Contains(plain, rot13s(v)) {
			vals = append(vals, v)
		}
	}
	slices.Sort(vals)
	if len(vals) == 0 {
		t.Fatal("no decoded window found")
	}
	for i, v := range vals {
		if m := newWholeMatcher([]string{v}); m.constClean(true, publicUnits()...) {
			t.Errorf("held %q, decoded from public text: the table called startup clean", v)
		}
		if i%max(1, len(vals)/4) == 0 { // a few the slow way too
			var before bool
			withoutTables(func() { before = startupVerdict(v) })
			if after := startupVerdict(v); !before || !after {
				t.Errorf("held %q, decoded from public text: refused %v without the table, %v with it", v, before, after)
			}
		}
	}
	// And a held set no public text carries is clean both ways.
	var before bool
	withoutTables(func() { before = startupVerdict(kitcpuCreds...) })
	if before || startupVerdict(kitcpuCreds...) {
		t.Error("runzero's held set refused")
	}
}

func publicUnits() [][]byte {
	var u [][]byte
	for _, e := range publicText() {
		u = append(u, []byte(e))
	}
	return u
}

// TestKitcpuSQLVerdictUnchanged: a held value in a constant statement or a
// bound value, in every encoding, is withheld with the table as without,
// and a value outside the table's class is checked as before.
func TestKitcpuSQLVerdictUnchanged(t *testing.T) {
	db := kitcpuDB(t)
	exec := func(q string, args ...any) (with, without bool) {
		_, err := db.Exec(q, args...)
		with = errors.Is(err, ErrWithheld)
		withoutTables(func() {
			_, err := db.Exec(q, args...)
			without = errors.Is(err, ErrWithheld)
		})
		return
	}
	resetForTest(Declarations{})
	Hold("env:K", kitcpuLeak)
	for name, enc := range kitcpuEncodings(kitcpuLeak) {
		q := kitcpuStatement(enc)
		if constUnitClean([]byte(q)) {
			t.Errorf("%s in a constant statement: the table called it clean", name)
		}
		if with, without := exec(q); !with || !without {
			t.Errorf("%s in a constant statement: withheld %v with the table, %v without", name, with, without)
		}
		if with, without := exec(kitcpuBound, "x", enc); !with || !without {
			t.Errorf("%s in a bound value: withheld %v with the table, %v without", name, with, without)
		}
	}
	for _, q := range kitcpuDDL {
		if with, without := exec(q); with || without {
			t.Errorf("%q withheld: %v with the table, %v without", q, with, without)
		}
	}
	if with, without := exec(kitcpuBound, "x", "clean data"); with || without {
		t.Errorf("a clean bound value withheld: %v with the table, %v without", with, without)
	}
	// Held values the statements carry, of the class and outside it (a
	// short token, digits, non-ASCII, a value the statement holds whole):
	// withheld as before.
	for _, v := range []string{"resource_type", "CURRENT_TIMESTAMP", "TEXT", "id", "12345678", "Größe-resources", "idx_learn_unique ON"} {
		resetForTest(Declarations{})
		Hold("env:K", v)
		for _, q := range kitcpuDDL {
			with, without := exec(q)
			if with != without {
				t.Errorf("held %q, %q: withheld %v with the table, %v without", v, q, with, without)
			}
		}
	}
}

// TestKitcpuClassDecodesOneViewSet checks consttable.go's first premise: every
// matcher of the class decodes the same views of a unit.
func TestKitcpuClassDecodesOneViewSet(t *testing.T) {
	seed := maphash.MakeSeed()
	views := func(build func([]string) *matcher, vals ...string) map[uint64]int {
		m := build(vals)
		if m == nil || m.prefixes == nil {
			t.Fatalf("%q: not of the class", vals)
		}
		var mu sync.Mutex
		out := map[uint64]int{}
		m.rec = func(b []byte) {
			mu.Lock()
			out[maphash.Bytes(seed, b)]++
			mu.Unlock()
		}
		for _, u := range append(slices.Clone(kitcpuDDL), publicText()[:200]...) {
			if m.check([]byte(u)) != clean {
				t.Fatalf("%q: a unit is not clean", vals)
			}
		}
		return out
	}
	want := views(newMatcher, constRef)
	for i, vals := range [][]string{
		kitcpuCreds,
		kitcpuCreds, // the startup check's matcher, below
		{"with spaces and punctuation: ~!@#$%^&*()_+{}|<>?"},
		{"ＱＺＸ７-fullwidth-zq9w"}, // NFKC reads it as ASCII
		{"tab\tseparated\tq7zw", "zq9w second vx8k"},
	} {
		build := newMatcher
		if i == 1 {
			build = newWholeMatcher
		}
		got := views(build, vals...)
		if len(got) != len(want) {
			t.Errorf("%q: %d distinct views, the reference %d", vals, len(got), len(want))
			continue
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				t.Errorf("%q: a view of the reference's is missing", vals)
				break
			}
		}
	}
}

// TestKitcpuTableHoldsOnlyWhereBuilt: a table for another kit, Go release,
// golang.org/x/text version or set of limits decides nothing.
func TestKitcpuTableHoldsOnlyWhereBuilt(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:K", kitcpuCreds...)
	if !constUnitClean([]byte(kitcpuDDL[0])) {
		t.Fatal("the registered table does not decide a covered statement")
	}
	withoutTables(func() {
		f := strings.Split(kitcpuTable(), "\n")
		for _, bad := range []struct {
			i int
			v string
		}{{0, "credkit-constants 0"}, {1, "0000 " + runtime.Version() + " v0.0.0"}, {1, kitSourceHash + " go0.0 v0.0.0"}, {2, "1 2 3 4 5 6 7"}, {3, "other"}, {4, "!!"}} {
			g := slices.Clone(f)
			g[bad.i] = bad.v
			RegisterConstants(strings.Join(g, "\n"))
			if constUnitClean([]byte(kitcpuDDL[0])) {
				t.Errorf("a table with field %d = %q decided a statement", bad.i, bad.v)
			}
		}
	})
	defer func(v int) { budgetBase = v }(budgetBase)
	budgetBase--
	if constUnitClean([]byte(kitcpuDDL[0])) {
		t.Error("a table built under other limits decided a statement")
	}
}

// TestKitcpuSourceHash: the hash a table is built for is the kit's.
func TestKitcpuSourceHash(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	h := sha256.New()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "constsource.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(f + "\x00"))
		h.Write(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))) // a Windows checkout has CRLF line endings
		h.Write([]byte("\x00"))
	}
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != kitSourceHash {
		t.Errorf("kitSourceHash is %s, the kit's source is %s: run python3 tools/maintainer/vendor_credkit.py --refresh-hash", kitSourceHash, got)
	}
}
