// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fleet-keys-one-declared-variable: PRTG_FLEET_KEYS is name=key entries
// separated by commas and/or newlines, and no error repeats key text.
func TestParseFleetKeys(t *testing.T) {
	const kA, kB, kC = "parseKeyAcme-7c1e9a3b", "parseKeyGlobex-4d5e6f==", "parseKeyBare-0a1b2c3d"
	good := []struct {
		name, raw string
		want      map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"only separators", " ,\n, ", map[string]string{}},
		{"one", "acme=" + kA, map[string]string{"acme": kA}},
		{"comma", "acme=" + kA + ",globex=" + kB, map[string]string{"acme": kA, "globex": kB}},
		{"whitespace", "  acme = " + kA + " ,\tglobex=\t" + kB + "\t, ", map[string]string{"acme": kA, "globex": kB}},
		{"newlines", "acme=" + kA + "\nglobex=" + kB + "\n", map[string]string{"acme": kA, "globex": kB}},
		{"crlf and mixed", "acme=" + kA + ",\r\n\r\nglobex=" + kB + "\r\n", map[string]string{"acme": kA, "globex": kB}},
		{"key keeps later =", "acme=a=b=c", map[string]string{"acme": "a=b=c"}},
	}
	for _, tc := range good {
		got, err := ParseKeys(tc.raw)
		if err != nil {
			t.Errorf("%s: ParseKeys error %v", tc.name, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: ParseKeys = %d entries, want %d", tc.name, len(got), len(tc.want))
		}
		for name, key := range tc.want {
			if got[name] != key {
				t.Errorf("%s: core %s key mismatch", tc.name, name)
			}
		}
	}
	bad := []struct {
		name, raw, want string
	}{
		{"missing =", "acme=" + kA + "\n" + kC, `entry 2 has no "="`},
		{"empty key", "acme=" + kA + ",globex=  ", `entry 2 (core "globex") has an empty key`},
		{"empty name", "=" + kA, "entry 1 has no core name"},
		{"bad name", "ACME=" + kA, "entry 1: the text before"},
		{"name is a key", kB + "=acme", "entry 1: the text before"},
		{"traversal name", "a..b=" + kA, "entry 1: the text before"},
		{"duplicate", "acme=" + kA + ",,acme=" + kB, `entry 2 (core "acme") repeats entry 1`},
		{"comma inside a key", "acme=" + kA[:6] + "," + kA[6:], `entry 2 has no "="`},
	}
	for _, tc := range bad {
		got, err := ParseKeys(tc.raw)
		if err == nil {
			t.Errorf("%s: ParseKeys accepted %d entries, want an error", tc.name, len(got))
			continue
		}
		if got != nil {
			t.Errorf("%s: an error must come with no keys at all, got %d", tc.name, len(got))
		}
		msg := err.Error()
		if !strings.Contains(msg, tc.want) || !strings.Contains(msg, "PRTG_FLEET_KEYS") {
			t.Errorf("%s: error %q, want it to contain %q", tc.name, msg, tc.want)
		}
		for _, secret := range []string{kA, kB, kC, kA[:6], kA[6:], strings.TrimRight(kB, "=")} {
			if strings.Contains(msg, secret) {
				t.Errorf("%s: error repeats key text %q: %q", tc.name, secret, msg)
			}
		}
		if tc.name == "bad name" && strings.Contains(msg, "ACME") {
			t.Errorf("%s: error repeats a name that failed validation: %q", tc.name, msg)
		}
	}
}

// A child pinned to one core sees that core's PRTG_FLEET_KEYS entry and no
// other core's key, whatever case the inherited variable is spelled in.
func TestScrubPassesOnlyThePinnedCoresKey(t *testing.T) {
	const kA, kB = "scrubKeyAcme-1a2b3c4d", "scrubKeyGlobex-5e6f7a8b"
	env := []string{"PATH=/bin", "PRTG_FLEET_KEYS=acme=" + kA + ",globex=" + kB, "prtg_fleet_keys=globex=" + kB, "PRTG_NO_LEARN=1"}
	got := Scrub(env, Core{Name: "acme"}, kA)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, kB) {
		t.Fatalf("the acme child sees globex's key:\n%s", joined)
	}
	var keys []string
	for _, kv := range got {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PRTG_FLEET_KEYS") {
			keys = append(keys, kv)
		}
	}
	if len(keys) != 1 || keys[0] != "PRTG_FLEET_KEYS=acme="+kA {
		t.Fatalf("child PRTG_FLEET_KEYS = %q, want exactly acme's entry", keys)
	}
	if !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "PRTG_NO_LEARN=1") {
		t.Fatalf("Scrub dropped a kept variable:\n%s", joined)
	}
	// A core with no key gets no PRTG_FLEET_KEYS at all.
	if none := strings.Join(Scrub(env, Core{Name: "initech"}, ""), "\n"); strings.Contains(strings.ToUpper(none), "PRTG_FLEET_KEYS") {
		t.Fatalf("a keyless child still sees PRTG_FLEET_KEYS:\n%s", none)
	}
}

// The same rule end to end: the process Runner starts for core A is handed
// A's entry only.
func TestRunnerChildSeesOnlyItsOwnCoresKey(t *testing.T) {
	const kA, kB = "runnerKeyAcme-9f8e7d6c", "runnerKeyGlobex-3c2b1a0f"
	t.Setenv(DirEnv, t.TempDir())
	t.Setenv("PRTG_FLEET_KEYS", "acme="+kA+"\nglobex="+kB)
	t.Setenv("PRTG_API_KEY", "runnerParentKey-77aa")
	out, err := testChild(t, "env", false).Run(t.Context(), Core{Name: "acme"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	env := string(out)
	if !strings.Contains(env, "PRTG_FLEET_KEYS=acme="+kA+"\n") {
		t.Fatalf("the acme child does not see its own key entry:\n%s", env)
	}
	for _, leak := range []string{kB, "globex", "runnerParentKey-77aa"} {
		if strings.Contains(env, leak) {
			t.Errorf("the acme child sees %q:\n%s", leak, env)
		}
	}
	if !strings.Contains(env, SelectEnv+"=acme\n") || !strings.Contains(env, DirEnv+"=") {
		t.Fatalf("the child lost its core selection or fleet directory:\n%s", env)
	}
}

// A fleet.json written by an earlier build names a key variable per core; it
// still loads, and the next save drops the field. The variables that build
// read are still held, as a defense, should one still be exported.
func TestLegacyKeyEnvFieldStillLoads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	t.Setenv("PRTG_KEY_LEGACYUNIT", "fleetHoldLegacyKey-7a8b9c")
	held := recordHolds(t)
	HoldCredentials()
	if !slices.Contains(*held, "fleetHoldLegacyKey-7a8b9c") {
		t.Error("a PRTG_KEY_* variable from an earlier build is not held")
	}
	legacy := `{"cores":[{"name":"acme","server":"a.example:1616","key_env":"PRTG_KEY_ACME","tags":["gold"]},` +
		`{"name":"globex","server":"g.example","key_env":"CUSTOM_GLOBEX_KEY"}]}`
	path := filepath.Join(dir, "fleet.json")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := Load()
	if err != nil {
		t.Fatalf("a legacy fleet file must still load: %v", err)
	}
	if c, ok := reg.Get("acme"); !ok || c.Server != "a.example:1616" || len(c.Tags) != 1 {
		t.Fatalf("legacy acme = %+v %v", c, ok)
	}
	if err := Update(func(*Registry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "key_env") || !strings.Contains(string(saved), `"globex"`) {
		t.Fatalf("saved fleet file:\n%s", saved)
	}
}
