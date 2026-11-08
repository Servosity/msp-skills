// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// #612, round 4: a memo kept across statements let a read made in one
// statement stand for a read in a later one, though what follows a read
// depends on the view it was made in (text or bytes), the budget left and
// the depth, not on the bytes alone. A UTF-16LE primer's text-view read of
// a fullwidth spelling skipped a later byte view's read of the same
// spelling, and with it NFKC's byte output and its deflate decode. Every
// statement's verdict must be the one a cold check of it alone gives.

import (
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/width"
)

func TestMemoTextRunCannotAuthorizeByteRun(t *testing.T) {
	const secret = "cdefgabc"
	wide := "ＪＮＩＭＫＯＬＪＮ"
	primer := append(utf16LE(wide), make([]byte, 8<<10)...)
	payload := append([]byte{0xff}, []byte(wide)...)

	out, _, _ := inflateFixed([]byte("JNIMKOLJN"), nil, 64, -1)
	if string(out) != secret {
		t.Fatalf("invalid construction: %q", out)
	}
	if newMatcher([]string{secret}).check(payload) == clean {
		t.Fatal("cold matcher missed the payload")
	}

	holding(t, secret)
	db, rec := kitmemDB(t)
	const query = "INSERT INTO t(v) VALUES (?)"
	if _, err := db.Exec(query, primer); err != nil {
		t.Fatalf("primer: %v", err)
	}
	_, err := db.Exec(query, payload)
	if !errors.Is(err, ErrWithheld) || len(rec.stmts) != 1 {
		t.Fatalf("payload reached driver: err=%v statements=%d",
			err, len(rec.stmts))
	}
}

// The class: sequences of statements through the store-write check, each
// statement's verdict equal to a cold matcher's on that statement alone.
// Cores are the held value in every spelling the decode set reads (raw,
// fixed-Huffman deflate as text and as bytes, fullwidth for NFKC, percent,
// base64 inside JSON) and near-miss decoys; each is written in a view kind
// the matcher builds (UTF-8, a byte view, UTF-16LE/BE, Shift-JIS and EBCDIC
// text views, a JSON string), padded to either side of the 8 KB unit size
// the memo kept reads from, and statements of one sequence share cores so a
// read in one meets the same bytes in another.
func TestKitmemMemoDifferential(t *testing.T) {
	n := 2000
	if raceBuild || testing.Short() {
		n = 100
	}
	const secret = "cdefgabc"
	const deflText = "JNIMKOLJN" // fixed-Huffman bytes that decode to secret
	if out, _, _ := inflateFixed([]byte(deflText), nil, 64, -1); string(out) != secret {
		t.Fatalf("invalid construction: %q", out)
	}
	wide := func(s string) string { return width.Widen.String(s) }
	var pct strings.Builder
	for i := 0; i < len(secret); i++ {
		fmt.Fprintf(&pct, "%%%02X", secret[i])
	}
	cores := []string{
		secret, deflText, wide(secret), wide(deflText), pct.String(),
		`{"sdata":"` + base64.StdEncoding.EncodeToString([]byte(secret)) + `"}`,
		`{"sdata":"` + base64.StdEncoding.EncodeToString([]byte(deflText)) + `"}`,
		string(fixedStreamWith(secret)),
		// decoys
		"JNIMKOLJM", wide("JNIMKOLJM"), "ordinary call notes about backups", wide("cdefgab"),
	}
	utf16x := func(s string, be bool) []byte {
		var b []byte
		for _, u := range utf16.Encode([]rune(s)) {
			if be {
				b = append(b, byte(u>>8), byte(u))
			} else {
				b = append(b, byte(u), byte(u>>8))
			}
		}
		return b
	}
	wrappers := []func(string) []byte{
		func(s string) []byte { return []byte(s) },
		func(s string) []byte { return append([]byte{0xff}, s...) },
		func(s string) []byte { return utf16x(s, false) },
		func(s string) []byte { return utf16x(s, true) },
		func(s string) []byte {
			if b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(s)); err == nil {
				return b
			}
			return []byte(s)
		},
		func(s string) []byte {
			if b, err := charmap.CodePage037.NewEncoder().Bytes([]byte(s)); err == nil {
				return b
			}
			return []byte(s)
		},
		func(s string) []byte { return []byte(`{"notes":"` + strings.ReplaceAll(s, `"`, `\"`) + `"}`) },
	}
	filler := strings.Repeat("Call with Contoso about backups. ", 300)
	pad := func(r *rand.Rand, b []byte) []byte {
		target := []int{0, 100, 8<<10 - 64, 8 << 10, 8<<10 + 64, 9000}[r.IntN(6)]
		if len(b) >= target {
			return b
		}
		var p []byte
		if r.IntN(2) == 0 {
			p = make([]byte, target-len(b))
		} else {
			p = []byte(filler[:target-len(b)])
		}
		if r.IntN(2) == 0 {
			return append(b, p...)
		}
		return append(p, b...)
	}
	const query = "INSERT INTO t(v) VALUES (?)"
	db, _ := kitmemDB(t)
	r := rand.New(rand.NewPCG(612, 4))
	start, stmts, held, diffs, seq := time.Now(), 0, 0, 0, 0
	for ; seq < n && diffs < 5; seq++ {
		holding(t, secret) // a fresh matcher, as a new run's
		use := []string{cores[r.IntN(len(cores))], cores[r.IntN(len(cores))]}
		k := 2 + r.IntN(3)
		for i := 0; i < k; i++ {
			payload := pad(r, wrappers[r.IntN(len(wrappers))](use[r.IntN(2)]))
			_, err := db.Exec(query, payload)
			warm := errors.Is(err, ErrWithheld)
			if err != nil && !warm {
				t.Fatalf("sequence %d statement %d: %v", seq, i, err)
			}
			cold := newMatcher([]string{secret}).check(sqlUnit(query, []driver.NamedValue{{Ordinal: 1, Value: payload}})) != clean
			stmts++
			if cold {
				held++
			}
			if warm != cold {
				diffs++
				t.Errorf("sequence %d statement %d (%d bytes, %q...): withheld %v after the sequence's earlier statements, %v alone",
					seq, i, len(payload), payload[:min(len(payload), 24)], warm, cold)
			}
		}
	}
	t.Logf("%d sequences, %d statements (%d withheld cold) in %v", seq, stmts, held, time.Since(start).Round(time.Millisecond))
	if held == 0 || held == stmts {
		t.Errorf("%d of %d statements withheld: the generator does not reach both verdicts", held, stmts)
	}
}
