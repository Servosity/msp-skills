// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package secrets_test

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/credvariants"
	"eset-protect-pp-cli/internal/secrets"
)

// TestMaskNeverLeaksARegisteredSecret is the registry's property test
// (handfixes.json: secret-registry-masks-every-boundary): random secrets
// (mixed-alphabet passwords with specials, spaces and non-ASCII runes, and
// JWT-shaped bearers) are registered, then echoed back in every spelling a
// server could use, alone, in prose and inside a JSON error body. No masked
// text may let the independent oracle recover the secret or a ten-byte run
// of it. A spelling the registry masks in place must come back masked in
// place ("[redacted]", the rest of the text kept), never withheld whole.
func TestMaskNeverLeaksARegisteredSecret(t *testing.T) {
	t.Cleanup(secrets.Reset)
	for _, seed := range []int64{seedForTest(6), 1, 2} {
		maskProperty(t, seed, trialsForTest(700))
	}
}

func maskProperty(t *testing.T, seed int64, trials int) {
	t.Helper()
	r := credvariants.NewRand(seed)
	for trial := 0; trial < trials; trial++ {
		secrets.Reset()
		secret := credvariants.RandomSecret(r)
		others := []string{credvariants.RandomSecret(r), credvariants.RandomSecret(r)}
		secrets.Register(append(others, secret)...)
		for _, f := range credvariants.SecretForms(secret) {
			before, after := credvariants.RandomContext(r), credvariants.RandomContext(r)
			texts := []string{
				f.Text,
				before + " " + f.Text + " " + after,
				`{"code":16,"message":"` + before + " " + f.Text + `","details":[]}`,
				f.Text + f.Text,
			}
			for ti, text := range texts {
				out := secrets.Mask(text)
				if credvariants.SecretRecoverable([]byte(out), secret) {
					t.Fatalf("seed %d, trial %d, form %s: the secret survived masking\nsecret %q\n  in %q\n out %q", seed, trial, f.Name, secret, text, out)
				}
				if f.Fixed && (out == secrets.Withheld || !strings.Contains(out, secrets.Redacted)) {
					t.Fatalf("trial %d, form %s: an in-place form was not masked in place\nsecret %q\n  in %q\n out %q", trial, f.Name, secret, text, out)
				}
				if f.Fixed && (ti == 1 || ti == 2) && before != "" && !strings.Contains(out, before) {
					t.Fatalf("trial %d, form %s: masking lost the surrounding text %q: %q (in %q, secret %q)", trial, f.Name, before, out, text, secret)
				}
			}
		}
	}
}

// TestMaskLeavesOrdinaryTextAlone: text that holds no registered value is
// returned byte for byte, escapes and all.
func TestMaskLeavesOrdinaryTextAlone(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	r := credvariants.NewRand(9)
	for i := 0; i < 20; i++ {
		secrets.Register(credvariants.RandomSecret(r))
	}
	for _, text := range []string{
		`{"devices":[{"displayName":"FRONTDESK-01","uuid":"00000000-0000-0000-0000-000000000001"}]}`,
		"GET https://eu.device-management.eset.systems/v1/devices?pageSize=100&pageToken=abc%3D%3D returned HTTP 404",
		`C:\\Users\\kiosk\\AppData\\Local\\Temp\\sample.exe`,
		"plain words, with 100% coverage &amp; a + sign",
	} {
		if got := secrets.Mask(text); got != text {
			t.Fatalf("ordinary text changed:\n in %q\nout %q", text, got)
		}
	}
}

// TestShortValuesAreIgnored: a value under MinLen characters is never
// registered, so ordinary words are not masked; at MinLen it is.
func TestShortValuesAreIgnored(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("abc1234", "")
	if got := secrets.Mask("abc1234 failed"); got != "abc1234 failed" {
		t.Fatalf("a 7-character value was masked: %q", got)
	}
	secrets.Register("abcd1234")
	if got := secrets.Mask("abcd1234 failed"); got != secrets.Redacted+" failed" {
		t.Fatalf("an 8-character value was not masked: %q", got)
	}
	secrets.Register("\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9")
	if got := secrets.Mask("x \u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9\u00e9 y"); got != "x "+secrets.Redacted+" y" {
		t.Fatalf("eight non-ASCII characters were not masked: %q", got)
	}
}

// TestRegisterSplitsAnAuthorizationHeader: "Bearer <token>" registers the
// token alone, so an echo of the bare token is masked.
func TestRegisterSplitsAnAuthorizationHeader(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("Bearer T2-refreshed-bearer-0001")
	if got := secrets.Mask(`{"message":"Rejected credential T2-refreshed-bearer-0001"}`); strings.Contains(got, "T2-refreshed") {
		t.Fatalf("the bare token of a registered header leaked: %q", got)
	}
}

// TestOverlappingSecretsAreHiddenWhole: two secrets that overlap in the text
// are masked as one run, so no tail of either is shown.
func TestOverlappingSecretsAreHiddenWhole(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("alpha-beta-gamma", "gamma-delta-epsilon")
	got := secrets.Mask("x alpha-beta-gamma-delta-epsilon y")
	if got != "x "+secrets.Redacted+" y" {
		t.Fatalf("overlapping secrets left a tail: %q", got)
	}
}

// TestMaskJSONWithholdsWhatItCannotMaskInPlace: a secret spelled as a JSON
// number cannot be replaced without breaking the document, so the body is
// withheld; one inside a string is masked in place and stays valid JSON.
func TestMaskJSONWithholdsWhatItCannotMaskInPlace(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("918273645546", "pw-in-a-string-01")
	if _, ok := secrets.MaskJSON([]byte(`{"pin":918273645546}`)); ok {
		t.Fatal("a body whose secret is a JSON number was not withheld")
	}
	out, ok := secrets.MaskJSON([]byte(`{"message":"bad pw-in-a-string-01"}`))
	if !ok || string(out) != `{"message":"bad [redacted]"}` {
		t.Fatalf("a string secret was not masked in place: %q %v", out, ok)
	}
	same := []byte(`{"ok":true}`)
	if out, ok := secrets.MaskJSON(same); !ok || &out[0] != &same[0] {
		t.Fatal("a body without a secret was not returned as is")
	}
}

// TestDecodingBudgetFailsClosed: text still decoding after the pass budget
// is withheld, whatever it holds.
func TestDecodingBudgetFailsClosed(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("budget-secret-01")
	nested := "budget-secret-01"
	for i := 0; i < 20; i++ {
		nested = strings.ReplaceAll(nested, "%", "%25")
		if i == 0 {
			nested = strings.ReplaceAll(nested, "-", "%2D")
		}
	}
	if got := secrets.Mask("x " + nested); got != secrets.Withheld {
		t.Fatalf("a deeply nested encoding was not withheld: %q", got)
	}
}

// TestRegistryIsConcurrencySafe registers and masks from many goroutines at
// once (run with -race to check the publication).
func TestRegistryIsConcurrencySafe(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				v := fmt.Sprintf("concurrent-secret-%02d-%03d", g, i)
				secrets.Register(v)
				if got := secrets.Mask("echo " + v); strings.Contains(got, v) {
					t.Errorf("a value registered by this goroutine leaked: %q", got)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestHoldBackKeepsALineBrokenSecretWhole: a secret whose raw form holds a
// line break is never cut at a newline inside it.
func TestHoldBackKeepsALineBrokenSecretWhole(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	secrets.Register("first-line\nsecond-line")
	b := []byte("ok\nleak first-line\n")
	if got := secrets.HoldBack(b, len(b)); got != len("ok\nleak ") {
		t.Fatalf("HoldBack = %d, want the cut before the secret's first line (%d)", got, len("ok\nleak "))
	}
}

// seedForTest and trialsForTest let a long soak run override the property
// test's seed and size: ESET_SECRETS_SEED / ESET_SECRETS_TRIALS.
func seedForTest(def int64) int64 {
	if v, err := strconv.ParseInt(os.Getenv("ESET_SECRETS_SEED"), 10, 64); err == nil {
		return v
	}
	return def
}

func trialsForTest(def int) int {
	if v, err := strconv.Atoi(os.Getenv("ESET_SECRETS_TRIALS")); err == nil && v > 0 {
		return v
	}
	return def
}

// TestMaskingOneSecretNeverHidesAnother: when a registered value sits inside
// an encoded occurrence of another, masking the inner one in place must not
// leave the outer one's encoded head visible; the outer one is judged on the
// text with only its own matches cut out.
func TestMaskingOneSecretNeverHidesAnother(t *testing.T) {
	t.Cleanup(secrets.Reset)
	secrets.Reset()
	outer, inner := "secretpart-one secretpart-two-xyz", "secretpart-two-xyz"
	secrets.Register(outer, inner)
	// The head has one letter percent-encoded, so no fixed form of outer
	// matches, while inner is masked in place.
	text := "x secretpart-o%6Ee " + inner
	if url.PathEscape(outer) == text[2:] || url.QueryEscape(outer) == text[2:] {
		t.Fatal("the fixture must not be a fixed form of outer")
	}
	if out := secrets.Mask(text); credvariants.SecretRecoverable([]byte(out), outer) {
		t.Fatalf("masking %q in place exposed the encoded head of %q: %q", inner, outer, out)
	}
}
