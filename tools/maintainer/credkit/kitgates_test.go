// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Contract 1 (argv pre-scan): "--" ends the flags Cobra parses, so a
// --config after it is a positional the program never opens and the kit
// must not open either (kit-findings item 3: version -- --config <fifo>
// hung startup). A "--" right after a flag that may take a value can be
// that value (pflag reads `--name --` as name="--"), so there the scan reads
// on: over-reading holds more, it never holds less.
func TestKitGatesFlagValuesStopAtTerminator(t *testing.T) {
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"version", "--", "--config", "x"}, nil},
		{[]string{"--", "--config=x"}, nil},
		{[]string{"--config", "a", "--", "--config", "b"}, []string{"a"}},
		{[]string{"--name=v", "--", "--config", "x"}, nil},
		{[]string{"--name", "--", "--config", "x"}, []string{"x"}},
		{[]string{"-n", "--", "--config", "x"}, []string{"x"}},
		{[]string{"--config", "--", "--config", "x"}, []string{"--", "x"}},
		{[]string{"get", "--config", "a"}, []string{"a"}},
	} {
		if got := FlagValues(c.args, "--config"); !slices.Equal(got, c.want) {
			t.Errorf("FlagValues(%q) = %q, want %q", c.args, got, c.want)
		}
	}
	// A credential flag is still held after "--": the program may echo the
	// positional, and holding more is never a leak.
	if got := flagValues([]string{"version", "--", "--token", "t0ken-after-terminator"}, "--token"); !slices.Equal(got, []string{"t0ken-after-terminator"}) {
		t.Errorf("a credential flag after \"--\" = %q, want it held", got)
	}
}

// Contract 1: a credential file is read through filepath.Clean (the path
// the security scanners can follow) only when the cleaned path is the same
// directory entry. "a//b" and "a/./b" are; the file is read and held.
func TestKitGatesCleanEquivalentPathIsRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte("api_token = 'clean-path-token-7171'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{dir + string(filepath.Separator) + string(filepath.Separator) + "config.toml",
		dir + string(filepath.Separator) + "." + string(filepath.Separator) + "config.toml"} {
		resetForTest(Declarations{ConfigFields: []string{"api_token"}})
		atStartup(func() { gatherFile(name) })
		if !leaks("clean-path-token-7171") || leaks("clean") {
			t.Errorf("%s: the credential was not held, or every unit was withheld", name)
		}
	}
	if err := WriteFile(dir+string(filepath.Separator)+"."+string(filepath.Separator)+"out.txt", []byte("ok\n"), 0o600); err != nil {
		t.Fatalf("WriteFile through a clean-equivalent path: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "out.txt")); err != nil || string(b) != "ok\n" {
		t.Fatalf("WriteFile wrote %q, %v", b, err)
	}
}

// FlagValues returns only what the flag parser could give that flag: the
// MCP file-flag check reads FlagValues(args, "--config") as the config
// flag's value. Tokens an MCP shell-out produces (--other=value, and
// positionals that do not start with "-") give it nothing.
func TestKitGatesFlagValuesOnlyThatFlag(t *testing.T) {
	mcpShaped := []string{"get", "--notes=--config", "--output=x.json", "--config-dir=/etc", "config", "/etc/passwd", "--confi=y"}
	if got := FlagValues(mcpShaped, "--config"); len(got) != 0 {
		t.Errorf("FlagValues over MCP-shaped argv = %q, want nothing", got)
	}
	if got := FlagValues([]string{"--config=a", "-config", "b"}, "--config"); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("FlagValues = %q, want [a b]", got)
	}
}

// A --config that names no file ("--config=", or a directory part that does
// not exist) is a file that does not exist, as os.ReadFile reports it: the
// kit holds nothing for it and does not withhold the program's output.
func TestKitGatesEmptyOrMissingConfigPathIsNotUnknown(t *testing.T) {
	for _, p := range []string{"", filepath.Join(t.TempDir(), "missing") + string(filepath.Separator)} {
		resetForTest(Declarations{})
		atStartup(func() { gather(Declarations{}, []string{"version", "--config=" + p}) })
		if isUnknown() {
			t.Errorf("--config=%q made every value unknown", p)
		}
	}
}
