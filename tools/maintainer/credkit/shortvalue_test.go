// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Contract 2(a), amended 2026-11-27 (short held values): a held value of
// fewer than 8 characters is compared only in the raw unit and in the views
// substitution decoders make from it, and only as a whole token on both
// sides. Run and stream decoders' views, and every view decoded from one,
// compare no short value. Values of 8 or more characters are unchanged.

// shortUser is a 2-character user name, like the one a base URL's userinfo
// puts in the Basic auth of every request (the zammad case).
const shortUser = "op"

// holdShortUser holds shortUser as the user part of Basic auth, the way an
// outgoing request's credentials are held (contract 1), with a long
// password beside it.
func holdShortUser(t *testing.T) {
	t.Helper()
	resetForTest(Declarations{})
	Hold("header:authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(shortUser+":Pw-zq7Wsink-91")))
	if !isHeld(shortUser) {
		t.Fatalf("setup: %q not held", shortUser)
	}
}

func TestShortValueCleanOutputPasses(t *testing.T) {
	for name, hold := range map[string]func(*testing.T){
		"basic user": holdShortUser,
		"held alone": func(t *testing.T) { holding(t, shortUser) },
	} {
		t.Run(name, func(t *testing.T) {
			hold(t)
			for _, u := range []string{
				`{"jsonrpc":"2.0","id":42,"result":{}}`,
				`{"jsonrpc":"2.0","id":7,"error":{"code":-32603,"message":"internal error"}}`,
				"stop.",
				"options: none",
				"Laptop shipped; the customer asked us to stop.",
				`{"content":[{"type":"text","text":"Ticket 4512: Laptop shipped, customer asked us to stop. Desktop next."}],"isError":false}`,
				// Accepted trade-off: a short value carried only through a
				// run decoder (base64, hex) is out of scope.
				"b3A=",
				"6f70",
				"user 6f70 logged in",
				"111, 112",
				"[0x6f, 0x70]",
				// A stream decoder's reading (UTF-16LE) is out of scope too.
				"o\x00p\x00",
			} {
				if leaks(u) {
					t.Errorf("clean unit withheld with %q held: %q", shortUser, u)
				}
			}
		})
	}
}

func TestShortValueWholeTokenStillLeaks(t *testing.T) {
	holding(t, shortUser)
	for _, u := range []string{
		"user op logged in",
		`{"name":"op"}`,
		"op",
		"op.",
		"x=op",
		`{"user":"\u006fp"}`,          // a string escape: a substitution view
		"user=%6Fp&x=1",               // percent-encoding
		"<b>&#111;p</b>",              // a character reference
		"user bc logged in",           // ROT13
		"user \uff4f\uff50 logged in", // fullwidth: NFKC
	} {
		if !leaks(u) {
			t.Errorf("short value as a whole token not withheld: %q", u)
		}
	}
}

func TestShortValueNeedsBothBoundaries(t *testing.T) {
	holding(t, shortUser)
	// The left side is a real boundary now: a letter or digit before it
	// makes it part of a longer word, whatever charset reading starts there.
	for _, u := range []string{"stop", "stop.", "Laptop shipped", "2op ", "x%6Fp ", "\u00e9op "} {
		if leaks(u) {
			t.Errorf("short value inside a longer word withheld: %q", u)
		}
	}
}

func TestShortDigitValueRawAndSubstitutionOnly(t *testing.T) {
	holding(t, "4711")
	for _, u := range []string{"pin 4711", "pin 04711", "pin +4711", "pin 4,711", "pin 4.711e3", "pin=%34711"} {
		if !leaks(u) {
			t.Errorf("short digit value spelled as a whole token not withheld: %q", u)
		}
	}
	for _, u := range []string{"id 14711", "id 47110", "NDcxMQ==", hex.EncodeToString([]byte("4711"))} {
		if leaks(u) {
			t.Errorf("short digit value withheld outside raw and substitution views or inside a longer number: %q", u)
		}
	}
}

func TestLongValueUnchangedByShortRule(t *testing.T) {
	const long = "zq7WsinkPq"
	holding(t, long, shortUser)
	for _, u := range []string{
		base64.StdEncoding.EncodeToString([]byte(long)),
		hex.EncodeToString([]byte(long)),
		"piece zq7Wsink here",
		"xx" + long + "yy",
	} {
		if !leaks(u) {
			t.Errorf("long value or its piece not withheld: %q", u)
		}
	}
	holding(t, "12345678", shortUser)
	if !leaks(base64.StdEncoding.EncodeToString([]byte("12345678"))) {
		t.Error("long digit value through base64 not withheld")
	}
}

func TestShortValueRequestMatcherStillAnywhere(t *testing.T) {
	m := newRequestMatcher([]string{shortUser})
	for _, u := range []string{"stop", "Laptop", "b3A=", "6f70", "op"} {
		if m.check([]byte(u)) == clean {
			t.Errorf("contract 2(c): a short value in an outbound request not found: %q", u)
		}
	}
}

func TestShortValueWholeMatcherCollision(t *testing.T) {
	m := newWholeMatcher([]string{shortUser})
	if m.check([]byte("op")) == clean {
		t.Error("public-text collision: an entry that is the value whole not found")
	}
	for _, e := range []string{"Stop", "Not Found", "Content-Type"} {
		if m.check([]byte(e)) != clean {
			t.Errorf("public-text collision: %q found in an entry it is not a whole token of", e)
		}
	}
}

func TestShortValueMCPToolResultPassesUnchanged(t *testing.T) {
	holdShortUser(t)
	for _, text := range []string{
		"Laptop shipped; the customer asked us to stop.",
		`{"tickets":[{"id":4512,"title":"Desktop stops at boot","state":"open"}]}`,
	} {
		in := mcp.NewToolResultText(text)
		got, out := callTool(t, toolReturning(in))
		if got != in {
			t.Errorf("an ordinary tool result was rewritten or withheld: %s", out)
		}
	}
}

// A unit of parLevel bytes or more runs its decoders on several workers
// (match.go step): the no-short marks hold there too.
func TestShortValueLargeUnitParallelPath(t *testing.T) {
	holding(t, shortUser)
	var b strings.Builder
	for b.Len() < 3*parLevel {
		b.WriteString(`{"id":4512,"title":"Laptop shipped; customer asked us to stop","note":"b3A= 6f70 111, 112"}` + "\n")
	}
	clean := b.String()
	if leaks(clean) {
		t.Error("a large ordinary unit withheld with a short value held")
	}
	if !leaks(clean + `{"user":"op"}` + "\n") {
		t.Error("a short value as a whole token in a large unit released")
	}
	if !leaks(clean + `{"user":"%6Fp"}` + "\n") {
		t.Error("a percent-encoded short value as a whole token in a large unit released")
	}
}

// A short number's spellings (contract 3, Numbers) keep a left boundary at a
// plus sign or a point: punctuation, not a letter or digit (tokenAt). A
// letter before the spelling, ASCII or not, is no boundary.
func TestShortNumberSpellingBoundaries(t *testing.T) {
	for held, cases := range map[string]struct{ leak, clean []string }{
		"4711": {leak: []string{"pin+4,711", "+4,711", "id 4,711", "x.4711"}, clean: []string{"é4,711", "pin4,711", "4,711é"}},
		"1000": {leak: []string{"x+1e3", "x.1e3", "1e3", "+1E+3"}, clean: []string{"é1e3", "x1e3", "1e3é"}},
		"0000": {leak: []string{"x.0e3", "x.%30e3", "x+0e3", "0e3", "1.0e3", "1.%30e3", "x=0.e3"}, clean: []string{"é0e3", "x0e3", "10.e3"}},
		"0001": {leak: []string{"9.1000e01", "9.%31000e01", "x=.10e1", "1e0"}, clean: []string{"91e0", "é1e0", "x.10e1"}},
	} {
		holding(t, held)
		for _, u := range cases.leak {
			if !leaks(u) {
				t.Errorf("%q held: %q passed", held, u)
			}
		}
		for _, u := range cases.clean {
			if leaks(u) {
				t.Errorf("%q held: clean %q withheld", held, u)
			}
		}
	}
}

// sciOracle reports whether some substring of n spells held in scientific
// notation (contract 3, Numbers) and counts for a short value: 8 or more
// characters long, or a whole token (tokenAt).
func sciOracle(n []byte, held *big.Rat) bool {
	for i := range n {
		for j := i + 1; j <= len(n); j++ {
			s := string(n[i:j])
			k := strings.IndexAny(s, "eE")
			if k < 0 || !sciShape(s[:k], s[k+1:]) {
				continue
			}
			v, ok := new(big.Rat).SetString(strings.TrimPrefix(strings.Replace(s, "E", "e", 1), "+"))
			if ok && v.Cmp(held) == 0 && (j-i >= pieceLen || tokenAt(n, i, j)) {
				return true
			}
		}
	}
	return false
}

func sciShape(mant, exp string) bool {
	mant = strings.TrimPrefix(mant, "+")
	exp = strings.TrimLeft(exp, "+-")
	if len(exp) > 1 && strings.ContainsAny(exp[:1], "+-") || exp == "" || strings.Trim(exp, "0123456789") != "" {
		return false
	}
	digits := strings.Trim(mant, ".")
	return strings.Count(mant, ".") <= 1 && strings.Trim(mant, "0123456789.") == "" && strings.ContainsAny(mant, "0123456789") && digits != ""
}

// A short number's scientific spellings are found wherever the contract says
// they are, and only there: every substring is tried against an exact value.
func TestShortSciSpellingsMatchTheOracle(t *testing.T) {
	alphabet := []string{"0", "0", "1", "1", "9", ".", "+", "-", "e", "E", "x", "=", " ", "é"}
	r := rand.New(rand.NewPCG(7, 11))
	iters := 40000
	if testing.Short() {
		iters = 4000
	}
	for _, h := range []string{"0000", "0001", "1000", "4711", "0010", "9000"} {
		holding(t, h)
		m, _ := currentMatcher()
		if !isHeld(h) {
			t.Fatalf("setup: %q not held", h)
		}
		hv, _ := new(big.Rat).SetString(h)
		for it := 0; it < iters; it++ {
			var b strings.Builder
			for k := 1 + r.IntN(13); k > 0; k-- {
				b.WriteString(alphabet[r.IntN(len(alphabet))])
			}
			n := []byte(b.String())
			if want, got := sciOracle(n, hv), m.sciHit(n, false, false); want && !got {
				t.Fatalf("%q held: %q spells it and passed", h, n)
			}
		}
	}
}
