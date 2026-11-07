// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credvariants

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf16"
)

// TestRandIsDeterministicAndInRange pins the generator that replaced
// math/rand: the same seed gives the same sequence, every value is in
// [0, n), and small bounds see every value.
func TestRandIsDeterministicAndInRange(t *testing.T) {
	a, b := NewRand(31), NewRand(31)
	for _, n := range []int{1, 2, 3, 7, 10, 1000, 1 << 30} {
		seen := map[int]bool{}
		for i := 0; i < 4000; i++ {
			x, y := a.Intn(n), b.Intn(n)
			if x != y {
				t.Fatalf("seed 31 diverged at n=%d: %d vs %d", n, x, y)
			}
			if x < 0 || x >= n {
				t.Fatalf("Intn(%d) = %d", n, x)
			}
			seen[x] = true
		}
		if n <= 10 && len(seen) != n {
			t.Fatalf("Intn(%d) produced only %v", n, seen)
		}
	}
	if reflect.DeepEqual(Corpus([]string{"deviceToken"}, "S3NT", "[r]", 1, false), Corpus([]string{"deviceToken"}, "S3NT", "[r]", 2, false)) {
		t.Fatal("two seeds produced the same corpus")
	}
	if !reflect.DeepEqual(Corpus([]string{"deviceToken"}, "S3NT", "[r]", 5, false), Corpus([]string{"deviceToken"}, "S3NT", "[r]", 5, false)) {
		t.Fatal("one seed produced two corpora")
	}
}

// TestRecoverableDecodesEveryEscape checks the oracle's hand-rolled hex
// decoding against strconv for every \uXXXX code unit, every surrogate pair
// boundary and every %XX byte.
func TestRecoverableDecodesEveryEscape(t *testing.T) {
	for v := 0; v <= 0xFFFF; v++ {
		for _, f := range []string{"%04x", "%04X"} {
			esc := fmt.Sprintf(f, v)
			ref, _ := strconv.ParseUint(esc, 16, 32)
			if got := hexRune(esc); got != rune(ref) {
				t.Fatalf("hexRune(%q) = %#x, want %#x", esc, got, ref)
			}
		}
	}
	for _, pair := range [][2]rune{{0xD800, 0xDC00}, {0xDBFF, 0xDFFF}, {0xD83D, 0xDE00}} {
		hi, lo := fmt.Sprintf("%04x", pair[0]), fmt.Sprintf("%04X", pair[1])
		if got, want := utf16.DecodeRune(hexRune(hi), hexRune(lo)), utf16.DecodeRune(pair[0], pair[1]); got != want {
			t.Fatalf("pair %v: %#x, want %#x", pair, got, want)
		}
	}
	for v := 0; v <= 0xFF; v++ {
		for _, f := range []string{"%02x", "%02X"} {
			esc := fmt.Sprintf(f, v)
			if got := hexDigit(esc[0])<<4 | hexDigit(esc[1]); int(got) != v {
				t.Fatalf("%%%s decoded to %#x, want %#x", esc, got, v)
			}
		}
	}
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{`SECRET`, true},
		{`%53ECRET`, true},
		{`%2553ECRET`, true},
		{`%3ECRET`, false},
		{`SECRE`, false},
	} {
		if got := Recoverable([]byte(tc.in), "SECRET"); got != tc.want {
			t.Errorf("Recoverable(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestEncodedRecoverableFindsEveryEncodedForm proves the encoded-form oracle
// both ways: it recovers every EncodedForms spelling of random secrets (so a
// property test built on it can fail), alone and inside prose, and it does
// not report a secret in text that holds none.
func TestEncodedRecoverableFindsEveryEncodedForm(t *testing.T) {
	r := NewRand(5)
	for trial := 0; trial < 63; trial++ {
		secret := RandomSecret(r)
		if trial >= 60 {
			secret = []string{"s3cret!", "pw-7", "Ab1"}[trial-60]
		}
		for _, f := range EncodedForms(secret) {
			for _, text := range []string{f.Text, "Rejected " + f.Text + " for user"} {
				if !EncodedRecoverable([]byte(text), secret) {
					t.Fatalf("form %s of %q was not recovered from %q", f.Name, secret, text)
				}
			}
		}
		if EncodedRecoverable([]byte(`{"devices":[{"displayName":"FRONTDESK-01","uuid":"00000000-0000-0000-0000-000000000001"}]} Rejected [redacted] for user`), secret) {
			t.Fatalf("a text without %q was reported", secret)
		}
	}
}

// The URL-shaped value fixture is pinned here, in a _test.go file: the kit's
// fleet test (TestKit15FleetRequestsAreWhole) judges every query literal in a
// non-test file as one the connector sends, so credvariants.go composes it.
func TestValueSpellingsKeepTheURLFixture(t *testing.T) {
	const want = `"https://download.example/a?b=1\u0026c=SENTINEL9z"`
	for _, v := range ValueSpellings("SENTINEL9z") {
		if v == want {
			return
		}
	}
	t.Fatalf("ValueSpellings lost the URL fixture %s", want)
}
