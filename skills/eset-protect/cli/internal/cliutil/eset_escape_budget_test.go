// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// escapeBombs are bodies whose escapes unwind a few bytes per decoding pass,
// so a decoder that runs to a fixed point does n passes over n bytes (Codex
// round 5, P2: "%" then 500,000 "25" pairs is ~1 MB, ~500,000 allocations and
// ~250 GB scanned, outside the request deadline).
func escapeBombs(n int) map[string][]byte {
	return map[string][]byte{
		"percent 25 chain":  []byte("%" + strings.Repeat("25", n)),
		"percent 25 to key": []byte("%" + strings.Repeat("25", n) + "64eviceToken=x"),
		"json u005c chain":  []byte("\\u005c" + strings.Repeat("u005c", n) + "u0041"),
		"mixed chain":       []byte(strings.Repeat("%25", n/3) + "5Cu0025" + strings.Repeat("25", n/3)),
	}
}

// TestEscapeDecodingIsBounded: the work NamesKeyInAnyEncoding does is linear
// in the body. Allocations stay a small constant and bytes allocated a small
// multiple of the body, at 1x and 8x the size, and the 1 MB body from the
// review is answered (withheld) well inside a request deadline.
func TestEscapeDecodingIsBounded(t *testing.T) {
	// The ceiling is pinned here, not read from the package, so loosening
	// the budget fails this test instead of loosening it.
	const passCeiling = 16
	if maxEscapeDecodePasses > passCeiling {
		t.Fatalf("maxEscapeDecodePasses = %d, above the pinned ceiling %d", maxEscapeDecodePasses, passCeiling)
	}
	keys := []string{"deviceToken"}
	for name := range escapeBombs(8) {
		small, large := escapeBombs(25000)[name], escapeBombs(200000)[name]
		for _, body := range [][]byte{small, large} {
			allocs := testing.AllocsPerRun(1, func() { NamesKeyInAnyEncoding(body, keys) })
			if limit := float64(4*(passCeiling+2) + 8); allocs > limit {
				t.Fatalf("%s (%d bytes): %.0f allocations, want at most %.0f: the decoder is not bounded", name, len(body), allocs, limit)
			}
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			NamesKeyInAnyEncoding(body, keys)
			runtime.ReadMemStats(&after)
			if got, limit := after.TotalAlloc-before.TotalAlloc, uint64(8*(passCeiling+2)*len(body)+1<<20); got > limit {
				t.Fatalf("%s (%d bytes): %d bytes allocated, want at most %d", name, len(body), got, limit)
			}
		}
	}
	// The review's body, under a deadline: the old fixed-point loop takes
	// minutes here.
	bomb := []byte("%" + strings.Repeat("25", 500000))
	done := make(chan bool, 1)
	start := time.Now()
	go func() { done <- NamesKeyInAnyEncoding(bomb, keys) }()
	select {
	case named := <-done:
		if !named {
			t.Fatalf("a body still unwinding at the budget must be refused (named), got false")
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("1 MB escape bomb took %s", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("1 MB escape bomb did not finish in 10s: decoding is unbounded")
	}
	if _, ok := DecodeEscapesToFixedPoint(bomb); ok {
		t.Fatalf("DecodeEscapesToFixedPoint reported a fixed point for a body still unwinding at the budget")
	}
}

// TestEscapeDecodingBudgetVerdicts pins what the budget decides: every bomb
// is refused (withheld) whether or not it spells the key; a key nested up to
// the budget is still found and the same nesting without the key is still
// allowed; a literal key that a later pass would consume is found.
func TestEscapeDecodingBudgetVerdicts(t *testing.T) {
	keys := []string{"deviceToken"}
	for name, body := range escapeBombs(5000) {
		if !NamesKeyInAnyEncoding(body, keys) {
			t.Errorf("%s: an escape bomb was allowed", name)
		}
	}
	nest := func(s string, depth int) string {
		for i := 0; i < depth; i++ {
			s = strings.ReplaceAll(s, "%", "%25")
		}
		return s
	}
	for depth := 0; depth+1 < maxEscapeDecodePasses; depth++ {
		key := nest("%64evice%54oken=SECRET", depth)
		if !NamesKeyInAnyEncoding([]byte(key), keys) {
			t.Errorf("depth %d: a nested key was missed: %q", depth, key)
		}
		benign := nest("%64evice%4Dodel=X1", depth)
		if NamesKeyInAnyEncoding([]byte(benign), keys) {
			t.Errorf("depth %d: a nested body without the key was refused: %q", depth, benign)
		}
		if dec, ok := DecodeEscapesToFixedPoint([]byte(benign)); !ok || string(dec) != "deviceModel=X1" {
			t.Errorf("depth %d: DecodeEscapesToFixedPoint(%q) = %q, %v", depth, benign, dec, ok)
		}
	}
	for _, b := range []string{"%6deviceToken=SECRET", "%7" + "deviceToken", "\\u00" + "deviceToken"} {
		if !NamesKeyInAnyEncoding([]byte(b), keys) {
			t.Errorf("a literal key a later pass consumes was missed: %q", b)
		}
	}
	if NamesKeyInAnyEncoding([]byte(fmt.Sprintf("%%zz%s", "x")), keys) {
		t.Errorf("false mention")
	}
	_ = bytes.MinRead
}
