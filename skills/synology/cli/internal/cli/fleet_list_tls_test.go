// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFleetListTLSModes: a plain http:// NAS has no TLS at all and must not
// read as "verified"; https:// NAS units read verified, pinned or insecure.
func TestFleetListTLSModes(t *testing.T) {
	sweepIsolate(t)
	for _, args := range [][]string{
		{"fleet", "add", "plain", "--url", "http://10.0.0.20:5000"},
		{"fleet", "add", "checked", "--url", "https://nas.acme.invalid:5001"},
		{"fleet", "add", "pinned", "--url", "https://10.0.0.21:5001", "--tls-fingerprint", strings.Repeat("ab", 32)},
		{"fleet", "add", "skip", "--url", "https://10.0.0.22:5001", "--insecure"},
	} {
		if _, errText, code := runCLIErr(t, append(args, "--json")...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, errText)
		}
	}
	out, errText, code := runCLIErr(t, "fleet", "list", "--json")
	if code != 0 {
		t.Fatalf("fleet list: %d %s", code, errText)
	}
	var rows []struct {
		Name string `json:"name"`
		TLS  string `json:"tls"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := map[string]string{"plain": "none (plain http)", "checked": "verified", "pinned": "pinned", "skip": "insecure"}
	for _, r := range rows {
		if want[r.Name] != r.TLS {
			t.Fatalf("%s: tls = %q, want %q", r.Name, r.TLS, want[r.Name])
		}
		delete(want, r.Name)
	}
	if len(want) != 0 {
		t.Fatalf("rows missing: %v", want)
	}
}
