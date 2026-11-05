// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "config-reads-held-before-parse" (class secret-echo).

package cliutil

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"avanan-pp-cli/internal/credkit"
)

// heldNow reports whether the kit withholds v from a file write.
func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte("x "+v+" x"), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// TestHoldTOMLValuesHoldsEveryFormTheLoaderAccepts: every string a TOML file
// sets is held, in any form (case-folded keys, quoted and dotted keys, inline
// and commented header tables, multiline strings) and when the file does not
// parse; the values of the settings the caller names are not.
func TestHoldTOMLValuesHoldsEveryFormTheLoaderAccepts(t *testing.T) {
	setting := func(key string) bool { return key == "base_url" }
	for name, tc := range map[string]struct{ body, value string }{
		"upper-case key":         {`APP_ID = "toml-form-TF1"`, "toml-form-TF1"},
		"multiline basic":        {"app_id = \"\"\"\ntoml-form-TF2\"\"\"", "toml-form-TF2"},
		"multiline literal":      {"app_id = '''\ntoml-form-TF3'''", "toml-form-TF3"},
		"quoted key":             {`"app_id" = 'toml-form-TF4'`, "toml-form-TF4"},
		"dotted header":          {`headers.X-Key = "toml-form-TF5"`, "toml-form-TF5"},
		"inline header table":    {`headers = { X-Key = "toml-form-TF6" }`, "toml-form-TF6"},
		"commented header table": {"[headers] # upstream\nX-Key = \"toml-form-TF7\"", "toml-form-TF7"},
		"malformed multiline":    {"app_id = '''\ntoml-form-TF8'''\nbroken =", "toml-form-TF8"},
		"malformed inline table": {"headers = { X-Key = \"toml-form-TF9\" }\nbroken =", "toml-form-TF9"},
		"malformed bare value":   {"app_id = toml-form-TFA\nbroken =", "toml-form-TFA"},
		"malformed array":        {"keys = [\n  \"toml-form-TFB\",\n]\nbroken =", "toml-form-TFB"},
	} {
		url := "https://setting-" + hex.EncodeToString([]byte(name)) + ".example"
		HoldTOMLValues([]byte(`BASE_URL = "`+url+`"`+"\n"+tc.body+"\n"), setting)
		if !heldNow(t, tc.value) {
			t.Errorf("%s: %q is not held", name, tc.value)
		}
		if heldNow(t, url) {
			t.Errorf("%s: the base_url setting was held", name)
		}
	}
}

// TestHeldHeaderComponentsHaveNoShapeFilter: a part is dropped only when it is
// base64 padding, never for its shape: punctuation-only and short parts stay.
func TestHeldHeaderComponentsHaveNoShapeFilter(t *testing.T) {
	for header, want := range map[string]string{
		`sid="cookie-secret-Q1"; theme=dark`: "cookie-secret-Q1",
		`a=b1; sid='q2'`:                     "q2",
		`sid=!!!!`:                           "!!!!",
		`k=="`:                               "",
	} {
		got := HeldHeaderComponents(header)
		if want != "" && !slices.Contains(got, want) {
			t.Errorf("HeldHeaderComponents(%q) = %q, want %q among them", header, got, want)
		}
	}
	for _, c := range HeldHeaderComponents("Basic dXNlcjpwdzE=") {
		if c == "=" || c == "==" {
			t.Errorf("base64 padding %q is a component", c)
		}
	}
}

// TestTOMLUnescapeDecodesWhatTheLibraryDecodes: each escape, the line-ending
// backslash included; one the library refuses stays as written.
func TestTOMLUnescapeDecodesWhatTheLibraryDecodes(t *testing.T) {
	for in, want := range map[string]string{
		`\U00000073ecret`: "secret",
		`\u0073ecret`:     "secret",
		"sec\\\n   ret":   "secret",
		`a\tb`:            "a\tb",
		`a\qb`:            `a\qb`,
		`plain`:           "plain",
	} {
		if got := tomlUnescape(in); got != want {
			t.Errorf("tomlUnescape(%q) = %q, want %q", in, got, want)
		}
	}
}
