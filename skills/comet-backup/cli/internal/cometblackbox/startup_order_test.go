// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// comet-backup handfix (output-chokepoint, startup ordering): the output
// filter is installed before the configuration is loaded or any startup
// diagnostic can print, in the CLI and in the MCP server, and a config file
// that fails to parse still has every value in it held (the whole file is
// read, no size cutoff), at the default location and at the legacy one. The
// legacy file's parse warning used to print before the filter existed, and a
// malformed file held nothing at all, so its values reached stdout and stderr
// whenever anything repeated them.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	soPrimaryPw     = "MalformedPrimaryPw-3c8e1f7a" // backup_password in a malformed config
	soPrimaryPasted = "PastedPrimaryValue-5d2b9e"   // a secret pasted on the left of '=' (twice)
	soShortKey      = "Ab3dE9"                      // a 6-character backup_session_key
	soLegacyPw      = "MalformedLegacyPw-8a4c2e6b"  // backup_password in a malformed legacy config
	soLegacyPasted  = "PastedLegacyValue-1f7c3a"    // a pasted secret in the legacy config (twice)
	soBigPw         = "MalformedBigFilePw-6e0d4b9c" // a credential after 2 MiB of padding
)

func malformedConfig(pw, key string) string {
	return "backup_username = \"admin\"\n" +
		"backup_password = \"" + pw + "\"\n" +
		key + " = \"zqFirstMember-71\"\n" +
		key + " = \"zqOtherMember-72\"\n"
}

// isolatedEnv is newEnv with no server and the named variables replaced (or,
// with an empty value, removed).
func isolatedEnv(t *testing.T, set map[string]string) *bbEnv {
	t.Helper()
	e := newEnv(t, &server{url: "https://127.0.0.1:1", sinkURL: "http://127.0.0.1:1"}, false)
	env := make([]string, 0, len(e.env)+len(set))
	for _, kv := range e.env {
		name, _, _ := strings.Cut(kv, "=")
		if _, replaced := set[name]; !replaced {
			env = append(env, kv)
		}
	}
	for name, v := range set {
		if v != "" {
			env = append(env, name+"="+v)
		}
	}
	e.env = env
	return e
}

// startupEnv is an isolated home with no credential in the environment, so
// the config file is the only place the credentials live.
func startupEnv(t *testing.T) (*bbEnv, string, string) {
	t.Helper()
	e := isolatedEnv(t, map[string]string{"COMET_BACKUP_USERNAME": "", "COMET_BACKUP_PASSWORD": ""})
	primary := filepath.Join(e.home, "primary-config")
	legacy := filepath.Join(e.home, ".config", "comet-backup-cli")
	e.env = append(e.env, "COMET_BACKUP_CONFIG_DIR="+primary)
	for _, d := range []string{primary, legacy} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return e, filepath.Join(primary, "config.toml"), filepath.Join(legacy, "config.toml")
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runMCP starts the MCP server with args and returns everything it printed.
func runMCP(t *testing.T, e *bbEnv, args ...string) string {
	t.Helper()
	cmd := exec.Command(bbMCP, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("mcp %v: did not exit", args)
	}
	return out.String() + "\n" + errb.String()
}

func assertNotPrinted(t *testing.T, where, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s printed %q:\n%s", where, s, out)
		}
	}
}

func TestMalformedConfigCredentialsNeverPrinted(t *testing.T) {
	t.Parallel()
	e, primary, _ := startupEnv(t)
	writeConfig(t, primary, malformedConfig(soPrimaryPw, soPrimaryPasted)+"backup_session_key = \""+soShortKey+"\"\n")
	all := []string{soPrimaryPw, soPrimaryPasted, soShortKey}

	// A command that loads the config prints the parse error.
	out := e.run(t, "doctor", "--json")
	if !strings.Contains(out, "already defined") && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("control: doctor did not report the parse error:\n%s", out)
	}
	assertNotPrinted(t, "cli doctor", out, all...)

	// Any text that repeats a value of the malformed file is withheld.
	for _, s := range all {
		out := e.run(t, s)
		if !strings.Contains(out, "unknown command") && !strings.Contains(out, "credkit: withheld") {
			t.Fatalf("control: the CLI did not reject %q:\n%s", s, out)
		}
		assertNotPrinted(t, "cli unknown command", out, all...)

		out = runMCP(t, e, "--transport", s)
		if !strings.Contains(out, "unknown --transport") && !strings.Contains(out, "credkit: withheld") {
			t.Fatalf("control: the MCP server did not reject the transport:\n%s", out)
		}
		assertNotPrinted(t, "mcp startup", out, all...)
	}
}

func TestMalformedLegacyConfigCredentialsNeverPrinted(t *testing.T) {
	t.Parallel()
	e, _, legacy := startupEnv(t)
	writeConfig(t, legacy, malformedConfig(soLegacyPw, soLegacyPasted))
	all := []string{soLegacyPw, soLegacyPasted}

	// The legacy file's parse warning prints at startup, before any command.
	out := e.run(t, "version")
	if !strings.Contains(out, "legacy config parse skipped") && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("control: the legacy config was not read:\n%s", out)
	}
	assertNotPrinted(t, "cli startup", out, all...)

	out = runMCP(t, e, "--transport", soLegacyPw)
	if !strings.Contains(out, "unknown --transport") && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("control: the MCP server did not reject the transport:\n%s", out)
	}
	assertNotPrinted(t, "mcp startup", out, all...)

	out = e.run(t, soLegacyPw)
	assertNotPrinted(t, "cli unknown command", out, all...)
}

func TestMalformedConfigIsReadWhole(t *testing.T) {
	t.Parallel()
	e, primary, _ := startupEnv(t)
	writeConfig(t, primary, strings.Repeat("\n", 2<<20)+"backup_password = \""+soBigPw+"\"\nzqNotTomlLine\n")
	out := e.run(t, soBigPw)
	if !strings.Contains(out, "unknown command") && !strings.Contains(out, "credkit: withheld") {
		t.Fatalf("control: the CLI did not reject the argument:\n%s", out)
	}
	assertNotPrinted(t, "cli", out, soBigPw)
	assertNotPrinted(t, "mcp startup", runMCP(t, e, "--transport", soBigPw), soBigPw)
}

// A stray value of a malformed file that reads like public text ("= 1", "at"):
// the kit holds it, so it refuses to start, exits ExitRefused, and prints
// nothing that could carry the file's credentials.
func TestMalformedConfigCollidingWithPublicTextHoldsItsField(t *testing.T) {
	t.Parallel()
	e, primary, _ := startupEnv(t)
	writeConfig(t, primary, "backup_password = \""+soPrimaryPw+"\"\n"+soPrimaryPasted+" = 1\n"+soPrimaryPasted+" = 2\n")
	cmd := exec.Command(bbCLI, "doctor", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	out, _ := cmd.CombinedOutput()
	// The file's credential field is held whatever the parse says. A credential
	// pasted as a key is malformed input the kit reads (followups.md, P2).
	assertNotPrinted(t, "malformed config", string(out), soPrimaryPw)
}
