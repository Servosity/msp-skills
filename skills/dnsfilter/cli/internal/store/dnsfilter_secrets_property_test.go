// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: secrets-never-persisted): a
// brute-force property check of ScrubPersistedSecrets with an oracle that
// does not depend on the implementation.

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// secretGen builds small JSON documents by hand (so objects can repeat a
// key) in which every sentinel value sits under a secret-named key, or in a
// secret name=value pair inside a string. No other value ever contains the
// sentinel prefix, so any sentinel in the scrubbed output is a leak, whatever
// the scrubber's internals.
type secretGen struct {
	r *rand.Rand
	n int
}

const sentinelPrefix = "SNTNL"

var genKeys = []string{"name", "token", "secret_key", "pin", "id", "meta"}

func (g *secretGen) sentinel() string {
	g.n++
	return fmt.Sprintf("%s%d", sentinelPrefix, g.n)
}

func (g *secretGen) quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// plainValue is any JSON value that carries no secret.
func (g *secretGen) plainValue(depth int) string {
	switch k := g.r.Intn(9); {
	case k == 0:
		return "null"
	case k == 1:
		return []string{"true", "false"}[g.r.Intn(2)]
	case k == 2:
		return fmt.Sprintf("%d", g.r.Intn(1000))
	case k == 3 && depth < 4:
		return g.object(depth + 1)
	case k == 4 && depth < 4:
		return g.array(depth + 1)
	case k == 5 && depth < 4:
		return g.quote(g.textValue(depth + 1))
	default:
		return g.quote([]string{"ok", "HQ", "a b", "x:1", "Top Secret, Finance", "{name}", "[1,2", "token: null"}[g.r.Intn(8)])
	}
}

// secretValue is the value of a secret-named key: a sentinel string,
// number-like string, object or array holding a sentinel, or a harmless
// null / bool flag.
func (g *secretGen) secretValue(depth int) string {
	switch g.r.Intn(6) {
	case 0:
		return "null"
	case 1:
		return "true"
	case 2:
		return `{"v":` + g.quote(g.sentinel()) + `}`
	case 3:
		return `[` + g.quote(g.sentinel()) + `]`
	default:
		return g.quote(g.sentinel())
	}
}

func (g *secretGen) value(key string, depth int) string {
	if IsSecretKey(key) {
		return g.secretValue(depth)
	}
	return g.plainValue(depth)
}

func (g *secretGen) object(depth int) string {
	n := g.r.Intn(4)
	parts := make([]string, 0, n+1)
	for i := 0; i < n; i++ {
		key := genKeys[g.r.Intn(len(genKeys))]
		parts = append(parts, g.quote(key)+":"+g.value(key, depth))
	}
	if g.r.Intn(3) == 0 && n > 0 {
		// Repeat a key, often with the secret first and a null after it.
		key := []string{"token", "secret_key", "pin", "name"}[g.r.Intn(4)]
		parts = append(parts, g.quote(key)+":"+g.value(key, depth), g.quote(key)+":null")
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (g *secretGen) array(depth int) string {
	n := g.r.Intn(3)
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, g.plainValue(depth))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// textValue is the content of a string value: JSON in a string, JSON with
// trailing or leading text, a secret name=value pair, or plain text.
func (g *secretGen) textValue(depth int) string {
	pair := func() string {
		return []string{"token=", "pin: ", "secret_key=", "Token:"}[g.r.Intn(4)] + g.sentinel()
	}
	switch g.r.Intn(7) {
	case 0:
		return g.object(depth)
	case 1:
		return g.object(depth) + "} " + pair()
	case 2:
		return "note " + g.object(depth) + " end"
	case 3:
		return g.object(depth) + " " + g.object(depth)
	case 4:
		return "x " + pair() + " y"
	case 5:
		return g.quote(g.object(depth)) // JSON string holding JSON
	default:
		return "plain text"
	}
}

func TestScrubPersistedSecretsProperty(t *testing.T) {
	for _, seed := range []int64{20261020, 1, 7, 42} {
		checkScrubProperty(t, seed, 20000)
	}
}

func checkScrubProperty(t *testing.T, seed int64, cases int) {
	t.Helper()
	g := &secretGen{r: rand.New(rand.NewSource(seed))}
	for i := 0; i < cases; i++ {
		var doc string
		switch i % 3 {
		case 0:
			doc = g.object(0)
		case 1:
			doc = `{"id":"1","attributes":` + g.object(1) + `,"meta":` + g.quote(g.textValue(1)) + `}`
		default:
			doc = `[` + g.object(0) + `,` + g.object(0) + `]`
		}
		if !json.Valid([]byte(doc)) {
			t.Fatalf("generator produced invalid JSON: %s", doc)
		}
		out := ScrubPersistedSecrets(json.RawMessage(doc))
		if bytes.Contains(out, []byte(sentinelPrefix)) {
			t.Fatalf("seed %d case %d: a secret survived the scrub\n in  %s\n out %s", seed, i, doc, out)
		}
		if !json.Valid(out) {
			t.Fatalf("seed %d case %d: scrubbed output is not JSON\n in  %s\n out %s", seed, i, doc, out)
		}
	}
}

// The duplicate-key class, pinned case by case: the decoded value keeps only
// the last occurrence, so the first one's secret must not be carried by the
// original bytes, at the top level, inside JSON in a string, inside JSON
// embedded in text, and with text after an embedded value.
func TestScrubDuplicateKeysAndTrailingText(t *testing.T) {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	for name, in := range map[string]string{
		"top-level duplicate":        `{"id":"a1","meta":{"token":"SNTNL-1","token":null}}`,
		"duplicate inside array":     `[{"secret_key":"SNTNL-2","secret_key":false}]`,
		"duplicate in json string":   `{"meta":` + q(`{"token":"SNTNL-3","token":null}`) + `}`,
		"duplicate in embedded json": `{"meta":` + q(`note {"pin":"SNTNL-4","pin":true} end`) + `}`,
		"text after embedded value":  `{"meta":` + q(`{"ok":1}} token=SNTNL-5`) + `}`,
		"second value in string":     `{"meta":` + q(`{"ok":1} {"token":"SNTNL-6"}`) + `}`,
	} {
		out := ScrubPersistedSecrets(json.RawMessage(in))
		if bytes.Contains(out, []byte(sentinelPrefix)) || !json.Valid(out) {
			t.Errorf("%s:\n in  %s\n out %s", name, in, out)
		}
	}
	// A duplicate of a non-secret key is collapsed by re-encoding (the raw
	// bytes are never trusted once an object repeats a key).
	in := `{"name":"a","name":"b"}`
	if out := string(ScrubPersistedSecrets(json.RawMessage(in))); out != `{"name":"b"}` {
		t.Errorf("duplicate non-secret key: %s", out)
	}
}
