// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json config-reads-held-before-parse):
// every config-file read, at startup or later (the MCP server reloads per
// call), holds the file's credentials with the kit's own readers before the
// file is parsed: its declared fields, and every word of a file that does not
// parse. Codex publish pass 2 round 2: a per-call reload of a file
// rewritten to auth_header = 123456789012345678901234567890 returned the
// parser's overflow diagnostic, carrying the unheld number.

package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kaseya-vsa-pp-cli/internal/credkit"
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

// The keys that are not credentials stay unheld (the kit exempts the same
// keys through credwire's Identifiers), so ordinary output naming the base
// URL, the CA bundle or the TLS server name is not withheld.
// A credential is held whatever characters it is made of.
func TestConfigReadHoldsPunctuationOnlyCredential(t *testing.T) {
	const secret = "!!!!%%%%^^^^"
	// auth_header: the kit's built-in name, as no test here declares the rest.
	if _, err := Load(writeConfig(t, "auth_header = \""+secret+"\"\n")); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !heldNow(t, secret) {
		t.Errorf("%q was not held", secret)
	}
}

func TestConfigReadLeavesNonCredentialKeysUnheld(t *testing.T) {
	// A held value lasts for the process, and a file another test holds whole
	// (one that does not parse) covers every 8 characters of it: run alone.
	if os.Getenv("KASEYA_VSA_HOLD_TEST_CHILD") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConfigReadLeavesNonCredentialKeysUnheld$", "-test.count=1")
		cmd.Env = append(os.Environ(), "KASEYA_VSA_HOLD_TEST_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
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
