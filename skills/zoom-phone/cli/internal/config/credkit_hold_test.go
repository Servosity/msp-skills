// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring):
// every config-file read, at startup or later (the MCP server reloads per
// call), holds every value in the file with the credential kit before the
// file is parsed: a per-call reload of a file rewritten to auth_header =
// 123456789012345678901234567890 returns the parser's overflow diagnostic,
// carrying the number.

package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
	"zoom-phone-pp-cli/internal/credkit"
)

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
			name: "a short punctuation value under [headers] (class 1)", body: "[headers]\nX-Short = \"!!\"\n", parses: true,
			held: []string{"!!"},
		},
		{
			name: "a client id, the public half of the pair", body: "client_id = \"cfgClientId-7Kq2\"\n", parses: true,
			held: []string{"cfgClientId-7Kq2"},
		},
		{
			name: "a stored API key and secret", body: "exo_api_key = \"cfgApiKey-2Wz9\"\nexo_api_secret = \"cfgApiSec-5Rn1\"\n", parses: true,
			held: []string{"cfgApiKey-2Wz9", "cfgApiSec-5Rn1"},
		},
		{
			name: "a value in a later table", body: "[unknown]\nnested = \"nestedcred31\"\n", parses: true,
			held: []string{"nestedcred31"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := holdRecorder(t)
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
				if !slices.Contains(*got, v) {
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
	got := holdRecorder(t)
	if _, err := Load(writeConfig(t, "client_secret = \""+secret+"\"\n")); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Contains(*got, secret) {
		t.Errorf("%q was not held", secret)
	}
}

func TestConfigReadLeavesNonCredentialKeysUnheld(t *testing.T) {
	vals := []string{"https://notheldhost31.example", "/tmp/notheldcapath77.pem", "notheldsni55.example"}
	body := "base_url = \"" + vals[0] + "\"\nca_cert = \"" + vals[1] + "\"\ntls_server_name = \"" + vals[2] + "\"\ntoken_expiry = 2026-11-03T20:40:19-05:00\n"
	// The seam, not the kit's set: another test in this process may have held a
	// short value that occurs inside one of these.
	got := holdRecorder(t)
	if _, err := Load(writeConfig(t, body)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, v := range vals {
		if slices.Contains(*got, v) {
			t.Errorf("%q, a value of a key that is not a credential, is held", v)
		}
	}
	// A saved token's expiry decodes to a time, not a string. Held, it would
	// withhold every record stamped with the same month.
	for _, v := range *got {
		if strings.Contains(v, "2026-11-03") {
			t.Errorf("token_expiry is held as %q", v)
		}
	}
}

// holdRecorder swaps the hold seam for a recorder: what reaches the kit's Hold,
// exactly as given, whitespace and all.
func holdRecorder(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prior := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { got = append(got, values...) }
	t.Cleanup(func() { cliutil.Hold = prior })
	return &got
}

// Class 1: no filter on a value's characters. A whitespace-only value under a
// named key and a short punctuation one reach the kit; a bare boolean under a key
// that names no credential does not (a setting, not text), and a boolean literal
// under a credential key does.
func TestConfigHoldHasNoCharacterFilter(t *testing.T) {
	got := holdRecorder(t)
	cliutil.HoldConfigFile([]byte("auth_header = \"   \"\n[headers]\nX-A = \"--\"\nX-B = true\n[other]\nmode = \" \"\nswitch = true\n"))
	for _, want := range []string{"   ", "--", "true", " "} {
		if !slices.Contains(*got, want) {
			t.Errorf("%q did not reach the kit: %q", want, *got)
		}
	}
	*got = nil
	cliutil.HoldConfigFile([]byte("unknown_switch = true\n"))
	if slices.Contains(*got, "true") {
		t.Errorf("a bare boolean setting was held: %q", *got)
	}
}

// Class 2: every file Load opens is one ReadPaths names, in every way a run can
// point it somewhere. A marker in each candidate file names the file; the hold
// seam records the marker when Load reads it.
func TestReadPathsCoversEveryFileLoadReads(t *testing.T) {
	t.Setenv("ZOOM_PHONE_API_KEY", "")
	t.Setenv("ZOOM_PHONE_API_SECRET", "")
	t.Setenv("ZOOM_PHONE_BASE_URL", "")
	t.Setenv("ZOOM_PHONE_TOKEN_URL", "")
	scenarios := map[string]struct {
		env        map[string]func(root string) string
		configPath func(root string) string
	}{
		"default":               {},
		"ZOOM_PHONE_CONFIG":     {env: map[string]func(string) string{"ZOOM_PHONE_CONFIG": func(r string) string { return filepath.Join(r, "envprofile", "config.toml") }}},
		"--config":              {configPath: func(r string) string { return filepath.Join(r, "flagprofile", "config.toml") }},
		"ZOOM_PHONE_HOME":       {env: map[string]func(string) string{"ZOOM_PHONE_HOME": func(r string) string { return filepath.Join(r, "home") }}},
		"ZOOM_PHONE_CONFIG_DIR": {env: map[string]func(string) string{"ZOOM_PHONE_CONFIG_DIR": func(r string) string { return filepath.Join(r, "cfgdir") }}},
		"ZOOM_PHONE_DATA_DIR":   {env: map[string]func(string) string{"ZOOM_PHONE_DATA_DIR": func(r string) string { return filepath.Join(r, "datadir") }}},
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			root := testenv.Isolate(t)
			for k, v := range sc.env {
				t.Setenv(k, v(root))
			}
			configPath := ""
			if sc.configPath != nil {
				configPath = sc.configPath(root)
			}
			paths, err := ReadPaths(configPath)
			if err != nil {
				t.Fatal(err)
			}
			stores, err := StorePaths(configPath)
			if err != nil {
				t.Fatal(err)
			}
			paths = append(paths, stores...)
			// Every candidate: what ReadPaths names, and each path Load could
			// reach. Each gets a marker; Load must never read one ReadPaths
			// did not name.
			candidates := append([]string{}, paths...)
			for _, c := range candidateFiles(t, root, configPath) {
				if !slices.Contains(candidates, c) {
					candidates = append(candidates, c)
				}
			}
			marker := map[string]string{}
			for i, c := range candidates {
				m := "tracemarker" + string(rune('a'+i)) + "zz"
				marker[m] = c
				if err := os.MkdirAll(filepath.Dir(c), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c, []byte("exo_api_key = \""+m+"\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got := holdRecorder(t)
			_, _ = Load(configPath)
			readAny := false
			for _, v := range *got {
				if c, ok := marker[v]; ok {
					readAny = true
					if !slices.Contains(paths, c) {
						t.Errorf("Load read %s, which ReadPaths does not name (%q)", c, paths)
					}
				}
			}
			if !readAny {
				t.Fatalf("Load read no marker, so this scenario proved nothing (%q)", *got)
			}
		})
	}
}

// candidateFiles are the paths Load could read a value from, spelled out apart
// from ReadPaths so a path it forgets is still written and traced.
func candidateFiles(t *testing.T, root, configPath string) []string {
	t.Helper()
	var out []string
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, filepath.Join(dir, "config.toml"))
	if legacy, err := LegacyConfigPath(); err == nil {
		out = append(out, legacy)
	}
	if global, err := cliutil.CredentialsFilePath(); err == nil {
		out = append(out, global)
	}
	if configPath != "" {
		out = append(out, configPath)
		if c, err := cliutil.CredentialsFilePathForConfig(configPath); err == nil {
			out = append(out, c)
		}
	}
	if env := os.Getenv("ZOOM_PHONE_CONFIG"); env != "" {
		out = append(out, env)
		if c, err := cliutil.CredentialsFilePathForConfig(env); err == nil {
			out = append(out, c)
		}
	}
	return out
}
