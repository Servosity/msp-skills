// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// #612, second pass: after the first fixes the first sync still did not
// finish in a 1 GB, 1 CPU container. One large ordinary record's check
// still peaked near 2 GB (a 150 MB final part held for the next level, and
// every raw deflate attempt's output held to the end of its part), and each
// record was checked in full once per statement that carried it. A memo
// that let a later statement skip reads an earlier one made was removed
// after round 4 of review (kitmem3_test.go): the tests below that name it
// hold for any cross-statement state a matcher may keep.

import (
	"encoding/json"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// kitmemStatements is how a press store writes one synced record: the
// generic row, its search index (the record's string values, unescaped,
// joined by spaces), then the typed table's row.
func kitmemStatements(i int, rec []byte) [3][]byte {
	var v any
	if err := json.Unmarshal(rec, &v); err != nil {
		panic(err)
	}
	var vals []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		case string:
			vals = append(vals, x)
		}
	}
	walk(v)
	return [3][]byte{
		[]byte(fmt.Sprintf("INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, ?, ?, ?, ?)\n%d\nhubspot-calls-crm\n%s\n2026-11-06T20:00:00Z\n2026-11-06T20:00:00Z", i, rec)),
		[]byte(fmt.Sprintf("INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)\n%d\n%d\nhubspot-calls-crm\n%s", 7000000+i, i, strings.Join(vals, " "))),
		[]byte(fmt.Sprintf("INSERT INTO \"calls\" (\"id\", \"data\", \"synced_at\", \"title\") VALUES (?, ?, ?, ?)\n%d\n%s\n2026-11-06T20:00:00Z\nCall with Contoso about backups %d", i, rec, i)),
	}
}

// One large ordinary record's check holds memory proportional to the
// record, far below what it peaked at before (main and the first #612 fix:
// 1.8 to 2.0 GB over the start for this 198 KB record).
func TestKitmemLargeRecordPeak(t *testing.T) {
	if raceBuild || testing.Short() {
		t.Skip("a 198 KB check: skipped under -race and -short")
	}
	m := newMatcher([]string{kitsafeHeld})
	row := callRecord(3, 200_000)
	var finalParts, largest int
	readFinalHook = func(n int) { finalParts++; largest = max(largest, n) }
	defer func() { readFinalHook = nil }()
	base := heapLive()
	stop := heapPeak()
	v := m.check(row)
	over := int64(stop()) - int64(base)
	t.Logf("%d KB record: peak %d MB over the start; %d final parts read at the cap, the largest %d KB", len(row)>>10, over>>20, finalParts, largest>>10)
	if v != clean {
		t.Fatalf("an ordinary %d KB call record: verdict %d, want clean", len(row)>>10, v)
	}
	if bound := int64(1536*len(row) + 64<<20); over > bound {
		t.Errorf("a %d KB record's check peaked %d MB over the start, more than %d MB", len(row)>>10, over>>20, bound>>20)
	}
	if finalParts == 0 {
		t.Error("no final part reached the cap: the test no longer reaches readFinal")
	}
}

// The deflate memo holds only its latest outputs and decodes an older one
// again when an attempt joins it: verdicts and the work drawn from both
// budgets are the same as with every output held.
func TestKitmemMemoKeepsJoins(t *testing.T) {
	if raceBuild || testing.Short() {
		t.Skip("one worker, every unit checked twice: skipped under -race and -short")
	}
	size := 50_000
	held := fixedStreamWith(kitsafeHeld)
	units := map[string][]byte{
		"call record":                    callRecord(5, size),
		"SafeLinks emails":               safeLinksEmails(3),
		"held inside a broken stream":    held,
		"repeated broken stream, filler": []byte(strings.Repeat(string(fixedStreamWith(strings.Repeat("filler text ", 40))), 30)),
		"repeated broken stream, held":   []byte(strings.Repeat(string(fixedStreamWith(strings.Repeat("filler text ", 40))), 30) + string(held)),
	}
	type work struct {
		v         verdict
		dec, defl int64
	}
	// One worker: with several, which of two equal reads comes first varies
	// from run to run, and with it the work counted (the verdict does not).
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	run := func(kept int) map[string]work {
		defer func(k int) { keptBytes = k }(keptBytes)
		keptBytes = kept
		out := map[string]work{}
		for name, u := range units {
			v, _, dec, defl := newMatcher([]string{kitsafeHeld}).checkWork(u)
			out[name] = work{v, dec, defl}
		}
		return out
	}
	all, few := run(1<<40), run(1)
	for name, w := range all {
		if few[name] != w {
			t.Errorf("%s: with one output held %+v, with every output held %+v", name, few[name], w)
		}
	}
	if all["held inside a broken stream"].v == clean || all["repeated broken stream, held"].v == clean {
		t.Error("a held value inside a broken raw deflate stream was not found")
	}
	if all["call record"].v != clean || all["repeated broken stream, filler"].v != clean {
		t.Error("an ordinary unit was not clean")
	}
	// The window is exercised: decoding outputs again happens.
	defer func(k int) { keptBytes = k }(keptBytes)
	keptBytes = 1
	m := newMatcher([]string{kitsafeHeld})
	c := &closure{m: m}
	c.budget.Store(1 << 40)
	c.deflLeft.Store(1 << 40)
	c.done, c.seen = new(stripedSet), new(stripedSet)
	c.done.init(1 << 16)
	c.seen.init(1 << 12)
	s := c.worker(0)
	d := decoding{s: s, depth: 1}
	d.rawStreams(units["repeated broken stream, filler"])
	if s.defl.kept.redo == 0 {
		t.Error("with one output held, no output was decoded again: the test does not reach memoKept")
	}
}

// fixedStreamWith is a broken fixed-Huffman stream whose output starts with
// text: the attempt at its first byte yields text, then breaks.
func fixedStreamWith(text string) []byte {
	return newFixedStream().lit(text).bytes()
}

// Checks on one matcher at once, their workers running in parallel (go
// test -race): every verdict is what the same statement gets alone.
func TestKitmemMemoConcurrent(t *testing.T) {
	m := newMatcher([]string{kitsafeHeld})
	carrying := string(callRecord(21, 9_000))
	at := strings.Index(carrying, "<br>") + 4
	carrying = carrying[:at] + " " + kitsafeHeld + " " + carrying[at:]
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				for _, u := range kitmemStatements(20+g, callRecord(20+g, 9_000)) {
					if v := m.check(u); v != clean {
						errs <- fmt.Sprintf("goroutine %d: an ordinary statement: verdict %d", g, v)
					}
				}
				for _, u := range kitmemStatements(21, []byte(carrying)) {
					if v := m.check(u); v != leak {
						errs <- fmt.Sprintf("goroutine %d: a statement carrying the held value: verdict %d, want leak", g, v)
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
