// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"prtg-pp-cli/internal/cliutil/testenv"
	"prtg-pp-cli/internal/fleet"
)

// registerCores adds cores through 'core add', the way an operator does.
func registerCores(t *testing.T, names ...string) {
	t.Helper()
	dir := testenv.Isolate(t)
	t.Setenv(fleet.DirEnv, dir)
	for _, n := range names {
		if _, errb, err := runRootArgs(t, "core", "add", n, "--server", n+".example:1616", "--json"); err != nil {
			t.Fatalf("core add %s: %v\n%s", n, err, errb)
		}
	}
}

func coreListRows(t *testing.T) ([]map[string]any, string) {
	t.Helper()
	out, errb, err := runRootArgs(t, "core", "list", "--json")
	if err != nil {
		t.Fatalf("core list: %v\n%s", err, errb)
	}
	var rows []map[string]any
	if jerr := json.Unmarshal([]byte(out), &rows); jerr != nil {
		t.Fatalf("core list output is not a JSON array: %v\n%s", jerr, out)
	}
	return rows, out + "\n" + errb
}

func hasAll(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// core list reports key_set from PRTG_FLEET_KEYS; an unparseable value still
// lists every core (key_set false) and names the bad entry, never its text.
func TestCoreListReadsFleetKeys(t *testing.T) {
	const kA, kBare = "listKeyAcme-3e5f7a9b", "listKeyBare-2c4e6a8d"
	registerCores(t, "acme", "globex")
	t.Setenv("PRTG_FLEET_KEYS", "acme="+kA)
	rows, all := coreListRows(t)
	if len(rows) != 2 || rows[0]["name"] != "acme" || rows[0]["key_set"] != true || rows[1]["key_set"] != false {
		t.Fatalf("core list = %v", rows)
	}
	// A row names no key variable any more: these fields and no others.
	for _, r := range rows {
		if len(r) != 5 || !hasAll(r, "name", "server", "key_set", "insecure_tls", "tags") {
			t.Fatalf("core list row fields = %v", r)
		}
	}
	if strings.Contains(all, kA) {
		t.Fatalf("core list shows a key:\n%s", all)
	}
	t.Setenv("PRTG_FLEET_KEYS", "acme="+kA+"\n"+kBare)
	rows, all = coreListRows(t)
	if len(rows) != 2 || rows[0]["key_set"] != false || rows[1]["key_set"] != false {
		t.Fatalf("an unparseable PRTG_FLEET_KEYS must list every core without a key, got %v", rows)
	}
	if !strings.Contains(all, `PRTG_FLEET_KEYS entry 2 has no "="`) {
		t.Fatalf("core list does not report the bad entry:\n%s", all)
	}
	if strings.Contains(all, kA) || strings.Contains(all, kBare) {
		t.Fatalf("core list repeats key text:\n%s", all)
	}
}

// --core takes the key from the core's PRTG_FLEET_KEYS entry and leaves the
// pinned process holding that entry only.
func TestCoreSelectionTakesItsKeyFromFleetKeys(t *testing.T) {
	srv, auths := fakeCore(t, 1)
	pinCore(t, srv)
	if _, err := runRaw(t, "--req", "GET /sensors?limit=1"); err != nil {
		t.Fatal(err)
	}
	if len(*auths) == 0 || (*auths)[0] != "Bearer core-key" {
		t.Fatalf("the pinned core was sent %v, want its PRTG_FLEET_KEYS key", *auths)
	}
	if got := os.Getenv("PRTG_FLEET_KEYS"); got != "t1=core-key" {
		t.Fatalf("pinned process PRTG_FLEET_KEYS = %q, want only t1's entry", got)
	}
}

func TestCoreSelectionWithoutAKeyNamesFleetKeys(t *testing.T) {
	const kOther, kBare = "selectKeyOther-6b8d0f2a", "selectKeyBare-9e1c3a5f"
	srv, auths := fakeCore(t, 1)
	for _, tc := range []struct{ raw, want string }{
		{"other=" + kOther, "add t1=<API key> to PRTG_FLEET_KEYS"},
		{"t1=" + kBare + ",\n" + kOther, `PRTG_FLEET_KEYS entry 2 has no "="`},
	} {
		pinCore(t, srv)
		t.Setenv("PRTG_FLEET_KEYS", tc.raw)
		_, errb, err := runRootArgs(t, "--core", "t1", "fleet", "exec", "--raw", "--req", "GET /sensors", "--json")
		if err == nil {
			t.Fatalf("%q: a pinned core without a usable key must fail", tc.raw)
		}
		msg := err.Error() + "\n" + errb
		if !strings.Contains(msg, tc.want) {
			t.Fatalf("%q: error %q, want %q", tc.raw, msg, tc.want)
		}
		if strings.Contains(msg, kOther) || strings.Contains(msg, kBare) {
			t.Fatalf("%q: error repeats key text: %q", tc.raw, msg)
		}
	}
	if len(*auths) != 0 {
		t.Fatalf("a core without a usable key was still called: %v", *auths)
	}
}
