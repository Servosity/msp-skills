// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/ascii85"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

// output-units-reassembled (Codex r1 4.34.2 class credential split across
// output members): a held value whose parts sit in separate output units, in
// any order, with anything between them, is withheld in every shape.
func TestHeldValueSplitAcrossUnitsWithGapsIsWithheld(t *testing.T) {
	const key = "Kq7RmT2wZx9VbN4s"
	const a, b = "Kq7RmT2w", "Zx9VbN4s"
	FunnelHold(key)
	buf := make([]byte, ascii85.MaxEncodedLen(len(key)))
	a85 := string(buf[:ascii85.Encode(buf, []byte(key))])
	if strings.ContainsAny(a85, `"\`) {
		t.Fatalf("pick a key whose ASCII85 form fits a JSON string: %s", a85)
	}
	for _, text := range []string{
		`{"a":"` + a + `","gap":"x","b":"` + b + `"}`,             // Codex r1 input
		`{"b":"` + b + `","gap":"x","a":"` + a + `"}`,             // reversed
		`{"a":"xx` + a + `","gap":"x","b":"` + b + `yy"}`,         // tail and head edges
		`{"a":"Kq7R","n":1,"b":"mT2wZx","c":"note","d":"9VbN4s"}`, // three parts, a gap each
		`a=` + a + `&g=x&b=` + b,                                  // query string
		a + `,x,` + b,                                             // CSV cells
		`{"a":"` + base64.StdEncoding.EncodeToString([]byte(a)) + `","g":"x","b":"` + base64.StdEncoding.EncodeToString([]byte(b)) + `"}`, // each part encoded
		`{"a":"` + url.QueryEscape(a) + `","g":"x","b":"` + strings.ReplaceAll(url.QueryEscape(b), "Z", "%5A") + `"}`,
		`{"outer":"{\"a\":\"` + a + `\",\"g\":\"x\",\"b\":\"` + b + `\"}"}`, // JSON inside a JSON string
		`{"a":"` + a + `x","g":"y","b":"z` + b + `"}`,                       // head of one, tail of the other
		`{"a":"q-` + a + `-q","g":"y","b":"r.` + b + `.r"}`,                 // inside units
		`{"a":"` + a85[:9] + `","g":"y","b":"` + a85[9:] + `"}`,             // an encoded form split
	} {
		got := WithholdText(text)
		if got == text || strings.Contains(got, a) && strings.Contains(got, b) || strings.Contains(got, "Kq7R") && strings.Contains(got, "9VbN4s") {
			t.Errorf("%s: the split value printed: %s", text, got)
		}
	}
	pretty := "{\n  \"a\": \"" + a + "\",\n  \"gap\": \"x\",\n  \"b\": \"" + b + "\"\n}\n"
	if got := string(WithholdBytes([]byte(pretty))); strings.Contains(got, a) && strings.Contains(got, b) {
		t.Errorf("pretty JSON: the split value printed: %s", got)
	}
}

// No length floor on a part: a small output that spells the value one or
// three characters per member, out of order and with gaps, is withheld.
func TestHeldValueSplitIntoShortPartsIsWithheld(t *testing.T) {
	const key = "Hy4tNc8RwQ2mLp6V"
	FunnelHold(key)
	ones := []string{}
	for i := len(key) - 1; i >= 0; i-- {
		ones = append(ones, fmt.Sprintf(`"k%d":"%c","g%d":"x"`, i, key[i], i))
	}
	threes := []string{}
	for i := 0; i < len(key); i += 3 {
		threes = append(threes, fmt.Sprintf(`"k%d":"%s","g%d":"note"`, i, key[i:min(i+3, len(key))], i))
	}
	// A letters-then-digits key one character per member, in order.
	const plain = "QWERTYUI98765432"
	FunnelHold(plain)
	inOrder := []string{}
	for i := 0; i < len(plain); i++ {
		inOrder = append(inOrder, fmt.Sprintf(`"k%d":"%c","g%d":"x"`, i, plain[i], i))
	}
	for _, text := range []string{"{" + strings.Join(ones, ",") + "}", "{" + strings.Join(threes, ",") + "}", "{" + strings.Join(inOrder, ",") + "}"} {
		if got := WithholdText(text); got == text {
			t.Errorf("the value spelled in short parts printed: %s", got)
		}
	}
}

// Parts hidden far apart inside a large listing are still found: long parts
// are rare in any output.
func TestHeldValueSplitInsideALargeListingIsWithheld(t *testing.T) {
	FunnelHold("Rb6XeJ1uYc3WqT9z")
	text := accountListing(3000, map[int]string{120: "Rb6XeJ1u", 2400: "Yc3WqT9z"})
	if !scatteredEchoes(text, heldValues()) {
		t.Error("two eight-character parts inside a 3000-row listing were not found")
	}
}

// accountListing is a AppDirect-shaped account list of n rows; notes puts an
// extra member in the rows it names.
func accountListing(n int, notes map[int]string) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		note := ""
		if v, ok := notes[i]; ok {
			note = `,"note":"` + v + `"`
		}
		fmt.Fprintf(&sb, `{"id":"%d","name":"Acme %d Dental","description":"Backup for site %c, plan B, 10 seats","mappingId":"5f9f1c1b9b9b%012x","serviceId":"550e8400-e29b-41d4-a716-%012x","isMapped":%t,"unitCount":%d%s}`,
			100000+i, i, 'A'+i%26, i*7919, i*104729, i%3 == 0, i%50, note)
	}
	sb.WriteString("]")
	return sb.String()
}

// A short held value (below heldMatchFloor) split into whole units with a
// gap is withheld too.
func TestShortHeldValueSplitAcrossUnitsIsWithheld(t *testing.T) {
	FunnelHold("Pz8kLm")
	if got := WithholdText(`{"a":"Pz8","g":"x","b":"kLm"}`); strings.Contains(got, "Pz8") && strings.Contains(got, "kLm") {
		t.Errorf("the short split value printed: %s", got)
	}
}

// Ordinary output passes: a 3000-row account list (ids, UUIDs, names,
// counts), and a 20000-row one, with realistic credentials and short ones held
// (a short value's hex form is mostly digits), print unchanged, and quickly.
func TestScatteredCheckLeavesOrdinaryOutputAlone(t *testing.T) {
	FunnelHold("3f9a0c2e7b1d4f6a8c0e2b4d6f8a1c3e", "gV7pQ2xL9mZ4rT6wB8nK1sD3fH5jY0cA", "Vk3q", "u7Tx")
	FunnelHold(base64.StdEncoding.EncodeToString([]byte("3f9a0c2e7b1d4f6a8c0e2b4d6f8a1c3e:gV7pQ2xL9mZ4rT6wB8nK1sD3fH5jY0cA")))
	for _, rows := range []int{3000, 20000} {
		text := accountListing(rows, nil)
		start := time.Now()
		if scatteredEchoes(text, heldValues()) {
			t.Fatalf("an ordinary %d-row account list reads as a scattered credential", rows)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("the scattered check took %s on %d bytes", d, len(text))
		}
	}
}
