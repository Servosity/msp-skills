// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// gather chooses the base URL by position in what FlagValues returned: the
// values before the first "--" must be a prefix of them, in every spelling.
func TestFix2ChosenValuesArePrefixOfFlagValues(t *testing.T) {
	tokens := []string{"--config", "--config=a", "--config=", "-config", "-config=b", "--", "--json", "--name=x", "c", "", "-", "-c", "--config=--", "-vc", "--configure"}
	var walk func(args []string, left int)
	n := 0
	walk = func(args []string, left int) {
		n++
		narrow, wide := flagValues(beforeEnd(args), "--config"), FlagValues(args, "--config")
		if len(narrow) > len(wide) || !slices.Equal(narrow, wide[:len(narrow)]) {
			t.Errorf("%q: chosen from %q, not a prefix of %q", args, narrow, wide)
		}
		if left == 0 {
			return
		}
		for _, tok := range tokens {
			walk(append(slices.Clip(args), tok), left-1)
		}
	}
	walk(nil, 4)
	if n < 50000 {
		t.Fatalf("only %d argument lists", n)
	}
}

// An empty --config is the flag unset: the program falls back to the file it
// discovers, so that file's base URL stays the allowed origin.
func TestFix2EmptyConfigChoosesNothing(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	def := write("default.toml", "base_url = \"https://discovered.example.com\"\n")
	exp := write("explicit.toml", "base_url = \"https://chosen.example.com\"\n")
	d := Declarations{Files: func([]string) []string { return []string{def} }, BaseURL: BaseURL{Default: "https://default.example.com"}}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--config="}, "https://discovered.example.com"},
		{[]string{"--config", ""}, "https://discovered.example.com"},
		{[]string{"--config=" + exp, "--config="}, "https://discovered.example.com"},
		{[]string{"--config=", "--config=" + exp}, "https://chosen.example.com"},
		{[]string{"--config=" + exp}, "https://chosen.example.com"},
	} {
		resetForTest(d)
		atStartup(func() { gather(d, tc.args) })
		reg.Lock()
		got := reg.baseURL
		reg.Unlock()
		if got != tc.want {
			t.Errorf("%q: base URL %q, want %q", tc.args, got, tc.want)
		}
	}
}

// A vendored copy of the kit is the kit, not connector code: the fleet scans
// must not read it, or the first adopter on main turns this package's own
// tests red.
func TestFix2FleetScansSkipTheVendoredKit(t *testing.T) {
	for p, want := range map[string]bool{
		"../../../skills/hubspot/cli/internal/credkit/transport.go": true,
		"../../../skills/hubspot/cli/internal/client/client.go":     false,
		"../../../skills/credkit/cli/internal/client/client.go":     false,
	} {
		if got := vendoredKit(p); got != want {
			t.Errorf("vendoredKit(%q) = %v", p, got)
		}
	}
}
