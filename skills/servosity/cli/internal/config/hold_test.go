// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json config-reads-held-before-parse):
// every config-file read, at startup or later (the MCP server reloads per
// call), holds every value in the file with the credential kit before the
// file is parsed. Codex publish pass 2 round 2: a per-call reload of a file
// rewritten to auth_header = 123456789012345678901234567890 returned the
// parser's overflow diagnostic, carrying the unheld number.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/credkit"
)

// heldNow reports whether the kit withholds v from a file write.
func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigReadHoldsEveryValueBeforeParse(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		parses  bool
		held    []string
		inError string // the parse diagnostic carries it
	}{
		{
			name: "an overflowing auth_header (the reviewer's input)",
			body: "auth_header = 123456789012345678901234567890\n", held: []string{"123456789012345678901234567890"},
			inError: "123456789012345678901234567890",
		},
		{
			name: "a [headers] value written with a TOML escape is held decoded too",
			body: "[headers]\nX-Rdap-Key = \"qrst\\U00000075vwx\"\n", parses: true, held: []string{"qrstuvwx", "qrst\\U00000075vwx"},
		},
		{
			name: "an inline headers table", body: "headers = { X-Api-Key = \"inlinehdr4Kp\", X-Other = \"inlinehdr5Kp\" }\n",
			parses: true, held: []string{"inlinehdr4Kp", "inlinehdr5Kp"},
		},
		{
			name: "a bare value under an unknown key", body: "zz_token = 98765432109876543210\n", parses: true,
			held: []string{"98765432109876543210"},
		},
		{
			name: "a multi-line string", body: "auth_header = \"\"\"\nmultilinecred42\n\"\"\"\n", parses: true,
			held: []string{"multilinecred42"},
		},
		{
			name: "an unterminated string in a file that does not parse", body: "auth_header = \"unterminatedcred77\nzz_other = \"anotherlongval12\"\n",
			held: []string{"unterminatedcred77", "anotherlongval12"},
		},
		{
			name: "a line the parser rejects", body: "garbage line with strayvalue61 in it\nauth_header = \"rejectedline99\"\n",
			held: []string{"strayvalue61", "rejectedline99"},
		},
		{
			name: "the secrets inside base_url", body: "base_url = \"https://user:basepass77@h.example/?key=querysecret55\"\n", parses: true,
			held: []string{"basepass77", "querysecret55"},
		},
		{
			name: "a value with no letter or digit under a key", body: "zz_token = \"?!?!\"\n", parses: true,
			held: []string{"?!?!"},
		},
		{
			name: "a bare value with no letter or digit, in a file that does not parse", body: "zz_token = %%^^ # note\n",
			held: []string{"%%^^"},
		},
		{
			name: "a base_url password with no letter or digit", body: "base_url = \"https://user:%3F%2B@h.example/\"\n", parses: true,
			held: []string{"?+"},
		},
		{
			name: "a value in a later table", body: "[unknown]\nnested = \"nestedcred31\"\n", parses: true,
			held: []string{"nestedcred31"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.held {
				if heldNow(t, v) {
					t.Fatalf("%q is held before the config was read, so this case proves nothing", v)
				}
			}
			cfg, err := Load(writeConfig(t, tc.body))
			if tc.parses && err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !tc.parses && err == nil {
				t.Fatalf("Load parsed a file it should reject: %+v", cfg)
			}
			if tc.inError != "" && (err == nil || !strings.Contains(err.Error(), tc.inError)) {
				t.Fatalf("the parse diagnostic no longer carries %q, so this case proves nothing: %v", tc.inError, err)
			}
			for _, v := range tc.held {
				if !heldNow(t, v) {
					t.Errorf("%q from the config file is not held after Load", v)
				}
			}
		})
	}
}

// Punctuation that is not a value stays unheld: a stray word on a line that
// assigns nothing, and a comment's marker, are in the text but name no value.
func TestConfigReadLeavesStrayPunctuationUnheld(t *testing.T) {
	if _, err := Load(writeConfig(t, "zz_timeout = 555 # ~~~~ note\n=+=+\n")); err == nil {
		t.Fatal("Load parsed a file it should reject")
	}
	for _, v := range []string{"#", "~~~~", "=+=+"} {
		if heldNow(t, v) {
			t.Errorf("%q, punctuation that names no value, is held", v)
		}
	}
}

// The keys that are not credentials stay unheld (the kit exempts the same
// keys through credwire's Identifiers), so ordinary output naming the base
// URL, the CA bundle or the TLS server name is not withheld.
func TestConfigReadLeavesNonCredentialKeysUnheld(t *testing.T) {
	vals := []string{"https://notheldhost31.example", "/tmp/notheldcapath77.pem", "notheldsni55.example"}
	body := "base_url = \"" + vals[0] + "\"\nca_cert = \"" + vals[1] + "\"\ntls_server_name = \"" + vals[2] + "\"\n"
	if _, err := Load(writeConfig(t, body)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, v := range vals {
		if heldNow(t, v) {
			t.Errorf("%q, a value of a key that is not a credential, is held", v)
		}
	}
}
