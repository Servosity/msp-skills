// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

// Codex round 1 (rocketcyber 4.34.2 reprint) [P1] classes, each driven
// through the built binaries with the reviewer's failing input and the
// variants of its class.

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// noArgEnv is a clean environment against the fake with no token in it.
func noArgEnv(t *testing.T) *bbEnv {
	t.Helper()
	e := newEnv(t, echoServer(t), credEnv)
	e.env = withoutEnv(e.env, "ROCKETCYBER_API_TOKEN")
	return e
}

func write0600(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Class holder-incomplete (store unread beside a complete config): startup
// held what the configuration's credential selection read, and a config with
// complete credential fields never reads the credential store, so a stored
// token printed in an unknown-command error.
func TestStoredTokenIsHeldBesideACompleteConfig(t *testing.T) {
	const cfgTok, stored = "ConfigKey7c9e31a8", "StoredKey4b2f60d5"
	e := noArgEnv(t)
	home := filepath.Join(e.home, "rc")
	write0600(t, filepath.Join(home, "config", "config.toml"), "api_token=\""+cfgTok+"\"\n")
	write0600(t, filepath.Join(home, "data", "credentials.toml"), "api_token=\""+stored+"\"\n")
	e.env = append(e.env, "ROCKETCYBER_HOME="+home)
	for _, args := range [][]string{{stored, "--no-learn"}, {"agents", "--bogus-" + stored}, {"version", "--home", home, stored}} {
		if out := e.run(t, args...); strings.Contains(out, stored) {
			t.Errorf("%v printed the stored token beside a complete config:\n%s", args, out)
		}
	}
	// A config that does not parse returned before the store was read.
	write0600(t, filepath.Join(home, "config", "config.toml"), "api_token = [\n")
	if out := e.run(t, stored, "--no-learn"); strings.Contains(out, stored) {
		t.Errorf("the stored token printed beside a config that does not parse:\n%s", out)
	}
}

// Class holder-incomplete (malformed config): a config file TOML rejects was
// held a line at a time, so a value spanning lines (or under a key an earlier
// break swallowed) was never held.
func TestMalformedConfigHoldsEveryValueInIt(t *testing.T) {
	const tok = "RcyToken42VeryPrivate"
	for name, body := range map[string]string{
		"multiline literal":     "api_token = '''\n" + tok + "\n'''\nbad = [\n",
		"multiline basic":       "api_token = \"\"\"\n" + tok + "\"\"\"\nbad = [\n",
		"after an open array":   "bad = [\napi_token = \"\"\"\n" + tok + "\"\"\"\n",
		"in a broken table":     "[headers\nX-Trace = \"" + tok + "\"\n",
		"escaped in basic":      "api_token = \"" + strings.Replace(tok, "T", `T`, 1) + "\"\nbad = [\n",
		"array under a generic": "values = [\n  \"" + tok + "\",\n",
	} {
		t.Run(name, func(t *testing.T) {
			e := noArgEnv(t)
			e.writeConfig(t, body)
			if out := e.run(t, tok, "--no-learn"); strings.Contains(out, tok) {
				t.Errorf("a malformed config's value printed (%s):\n%s", name, out)
			}
		})
	}
}

// Class secret-in-encoded-output (compressed under nested carriers): a
// compression signature was looked for in recursively decoded views only when
// the text held an escape, so a Zstandard frame of the token under two
// carriers printed (the reviewer's input: base64 of base64 of the frame).
func TestCompressedTokenUnderNestedCarriersIsWithheld(t *testing.T) {
	const tok = "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"
	frame, _ := hex.DecodeString("28b52ffd20200301005a") // Zstandard: one RLE block, 32 x 'Z'
	inner := base64.StdEncoding.EncodeToString(frame)
	e := noArgEnv(t)
	e.env = append(e.env, "ROCKETCYBER_API_TOKEN="+tok)
	for name, carrier := range map[string]string{
		"base64(base64)": base64.StdEncoding.EncodeToString([]byte(inner)),
		"hex(base64)":    hex.EncodeToString([]byte(inner)),
		"base32(base64)": base32.StdEncoding.EncodeToString([]byte(inner)),
		"base64(hex)":    base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(frame))),
	} {
		if out := e.run(t, carrier, "--no-learn"); strings.Contains(out, carrier) {
			t.Errorf("%s of a Zstandard frame of the token printed:\n%s", name, out)
		}
	}
}

// Class credential-split-across-processes: each process remembered only its
// own deliveries, so two runs whose output reaches the same reader (a log
// both append to; one MCP client across server restarts) delivered the two
// halves of a held value.
func TestHalvesAcrossProcessesAreWithheld(t *testing.T) {
	const tok = "QQQQQQQQQQQQQQQQZZZZZZZZZZZZZZZZ"
	a, b := halves(tok)
	e := noArgEnv(t)
	e.env = append(e.env, "ROCKETCYBER_API_TOKEN="+tok)
	out := e.run(t, a, "--no-learn") + e.run(t, b, "--no-learn")
	if strings.Contains(out, a) && strings.Contains(out, b) {
		t.Errorf("two CLI runs delivered both halves of the token:\n%s", out)
	}
	mcp := e.mcpCalls(t, []map[string]any{tool(a, map[string]any{})}) + e.mcpCalls(t, []map[string]any{tool(b, map[string]any{})})
	if strings.Contains(mcp, a) && strings.Contains(mcp, b) {
		t.Errorf("two MCP sessions delivered both halves of the token:\n%.3000s", mcp)
	}
}
