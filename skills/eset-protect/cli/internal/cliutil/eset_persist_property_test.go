// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
)

const credentialPropertySentinel = "PROPSECRET-7Q9Z"

func containsSentinel(b []byte) bool {
	return credvariants.Recoverable(b, credentialPropertySentinel)
}

// persistPropertyScopes are the scopes the property runs under: each
// credential-bearing store resource type and request path, a scope that only
// carries deviceToken, and the empty (fail-closed, every key) scope.
var persistPropertyScopes = []string{
	"installers",
	"https://{region}.installer-management.eset.systems/v1/installers/{installerUuid}",
	"gpo-sccm-files-generate",
	"mobile-devices-batch-get-enrollment-links",
	"https://{region}.mobile-device-management.eset.systems/v1/mobile-devices:batchGetEnrollmentLinks",
	"devices",
	"detections_v2",
	"",
}

// TestPersistFormPropertyNoSentinelEverPersisted is the round-3 class test
// for credential-tier-never-persisted: in every scope, for every key of that
// scope, in every spelling, wrapped in every valid shape and every
// malformed, truncated or concatenated variant, (1) PersistForm accepts a
// body exactly when it is one JSON value, (2) nothing it accepts holds the
// sentinel in any recoverable spelling, (3) what it returns is stable
// (persisting it again changes nothing), and (4) a body HoldsPersistCredential
// lets through unchanged (the response cache writes those as they are) holds
// no sentinel.
func TestPersistFormPropertyNoSentinelEverPersisted(t *testing.T) {
	for _, scope := range persistPropertyScopes {
		full := scope == "" || scope == "installers"
		corpus := credvariants.Corpus(PersistCredentialKeysFor(scope), credentialPropertySentinel, PersistRedactedValue, 20261019, full)
		valid, refused := 0, 0
		for _, v := range corpus {
			out, ok := PersistForm(scope, []byte(v))
			if want := json.Valid([]byte(v)); ok != want {
				t.Fatalf("%s: PersistForm(%q) ok = %v, want %v (one JSON value, then end of input)", scope, v, ok, want)
			}
			if !ok {
				refused++
				if !HoldsPersistCredential(scope, []byte(v)) {
					t.Fatalf("%s: a body PersistForm refuses must hold for the cache: %q", scope, v)
				}
				continue
			}
			valid++
			if containsSentinel(out) {
				t.Fatalf("%s: persisted bytes hold the sentinel:\n in %q\nout %q", scope, v, out)
			}
			if again, ok2 := PersistForm(scope, out); !ok2 || !bytes.Equal(again, out) {
				t.Fatalf("%s: the persisted form is not stable: %q -> %q", scope, out, again)
			}
			if !HoldsPersistCredential(scope, []byte(v)) && containsSentinel([]byte(v)) {
				t.Fatalf("%s: HoldsPersistCredential let a body with the sentinel through unchanged: %q", scope, v)
			}
		}
		if valid < 200 || refused < 2000 {
			t.Fatalf("%s: corpus too small to mean anything: %d valid, %d refused", scope, valid, refused)
		}
	}
}

// TestPersistFormPropertyKeepsOutOfScopeKeys is the other half of the
// schema scoping: in a scope whose schema does not carry them (a v2
// detection, a policy), every valid body built around uri, downloadUrl and
// gpoSccmFile in every spelling and shape is persisted byte for byte.
func TestPersistFormPropertyKeepsOutOfScopeKeys(t *testing.T) {
	for _, scope := range []string{"detections_v2", "policies", "detection-groups", "devices"} {
		corpus := credvariants.Corpus([]string{"uri", "downloadUrl", "gpoSccmFile"}, credentialPropertySentinel, PersistRedactedValue, 5, false)
		kept := 0
		for _, v := range corpus {
			if !json.Valid([]byte(v)) {
				continue
			}
			out, ok := PersistForm(scope, []byte(v))
			if !ok || !bytes.Equal(out, []byte(v)) {
				t.Fatalf("%s: an out-of-scope key was altered:\n in %q\nout %q", scope, v, out)
			}
			kept++
		}
		if kept < 200 {
			t.Fatalf("%s: corpus too small: %d", scope, kept)
		}
	}
}

// TestRedactJSONKeysMatchesReference checks the token-stream decision against
// an independent reference on random JSON documents with keys drawn from a
// small alphabet that includes every credential key spelling: the body comes
// back unchanged exactly when no key occurrence the reference sees (it walks
// a duplicate-preserving parse) holds a non-trivial value.
func TestRedactJSONKeysMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	keys := []string{"downloadUrl", "DOWNLOADURL", `download\u0055rl`, "uri", "Uri", "gpoSccmFile", "deviceToken", "deviceTo\u212aen", "name", "uuid", "url", "token"}
	vals := []string{`null`, `""`, `"[redacted]"`, `"v"`, `1`, `true`}
	var gen func(depth int) (string, bool)
	// gen returns a JSON text and whether a credential key in it holds a
	// non-trivial value (the reference verdict).
	gen = func(depth int) (string, bool) {
		if depth > 3 || r.Intn(3) == 0 {
			return vals[r.Intn(len(vals))], false
		}
		if r.Intn(2) == 0 {
			n := r.Intn(4)
			parts := make([]string, n)
			needs := false
			for i := range parts {
				var nd bool
				parts[i], nd = gen(depth + 1)
				needs = needs || nd
			}
			return "[" + strings.Join(parts, ",") + "]", needs
		}
		n := r.Intn(4)
		parts := make([]string, n)
		needs := false
		for i := range parts {
			k := keys[r.Intn(len(keys))]
			v, nd := gen(depth + 1)
			decoded := k
			var s string
			if err := json.Unmarshal([]byte(`"`+k+`"`), &s); err == nil {
				decoded = s
			}
			isCred := false
			for _, c := range persistCredentialKeys {
				if strings.EqualFold(decoded, c) {
					isCred = true
				}
			}
			if isCred {
				nd = !(v == `null` || v == `""` || v == `"[redacted]"`)
			}
			needs = needs || nd
			parts[i] = `"` + k + `":` + v
		}
		return "{" + strings.Join(parts, ",") + "}", needs
	}
	for i := 0; i < 20000; i++ {
		doc, needs := gen(0)
		out, ok := RedactJSONKeys([]byte(doc), persistCredentialKeys, PersistRedactedValue)
		if !ok {
			t.Fatalf("a valid document was refused: %s", doc)
		}
		if changed := !bytes.Equal(out, []byte(doc)); changed != needs {
			t.Fatalf("RedactJSONKeys changed = %v, reference needs redaction = %v:\n in %s\nout %s", changed, needs, doc, out)
		}
	}
}

// TestFoldKeyTextCoversEqualFold pins the two folding tables the any-encoding
// matcher relies on: the only non-ASCII runes strings.EqualFold maps onto an
// ASCII letter are U+212A (k) and U+017F (s), and FoldKeyText maps exactly
// those.
func TestFoldKeyTextCoversEqualFold(t *testing.T) {
	found := map[rune]rune{}
	for r := rune(0x80); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		for c := 'a'; c <= 'z'; c++ {
			if strings.EqualFold(string(r), string(c)) {
				found[r] = c
			}
		}
	}
	if len(found) != 2 || found[0x212A] != 'k' || found[0x017F] != 's' {
		t.Fatalf("non-ASCII runes that fold onto ASCII letters changed: %v; extend FoldKeyText and mayHoldQuotedKey", found)
	}
	for r, c := range found {
		if got := string(FoldKeyText([]byte(string(r)))); got != string(c) {
			t.Fatalf("FoldKeyText(%U) = %q, want %q", r, got, c)
		}
	}
}

// TestNamesKeyInAnyEncoding covers the matcher client.do uses to withhold a
// body that is not JSON: every non-JSON mention and every malformed variant
// that still holds the key is found, and bytes without the name are not.
func TestNamesKeyInAnyEncoding(t *testing.T) {
	for _, key := range PersistCredentialKeys() {
		for _, m := range credvariants.NonJSONMentions(key, credentialPropertySentinel) {
			if !NamesKeyInAnyEncoding([]byte(m), []string{key}) {
				t.Errorf("%s: missed %q", key, m)
			}
		}
		r := credvariants.NewRand(1)
		for _, k := range credvariants.KeySpellings(key, r) {
			b := `{"x":1}]{"` + k + `":"` + credentialPropertySentinel + `"`
			if !NamesKeyInAnyEncoding([]byte(b), []string{key}) {
				t.Errorf("%s: missed %q", key, b)
			}
		}
	}
	for _, b := range []string{`{"devices":[{"uuid":"a"}]}`, `not json`, `%zz\u12`, ``} {
		if NamesKeyInAnyEncoding([]byte(b), []string{"deviceToken"}) {
			t.Errorf("false mention in %q", b)
		}
	}
}
