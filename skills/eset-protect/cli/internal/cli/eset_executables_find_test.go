// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `executables find`: hash classification, matching, and the
// local-mirror search path end to end.
package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const (
	executablesFindTestSHA1   = "3395856ce81f2b7382dee72602f798b642f14140"
	executablesFindTestMD5    = "84c82835a5d21bbcf75a61706d8ab549"
	executablesFindTestSHA256 = "ed01ebfbc9eb5bbea545af4d01bf5f1071661840480439c6e5babe8e080e41aa"
)

func executablesFindTestRows() []map[string]any {
	return []map[string]any{
		{"uuid": "9a000001-0000-4000-8000-000000000001", "displayName": "tasksche.exe", "applicationDisplayName": "DiskPart", "versionName": "6.1.7601.17514",
			"hashSha1": strings.ToUpper(executablesFindTestSHA1), "hashMd5": executablesFindTestMD5, "hashSha2256": executablesFindTestSHA256,
			"isBlocked": true, "devicesCount": "3", "firstOccurTime": "2026-10-12T22:41:00Z",
			"signatures":       []map[string]any{{"signatureTrustLevel": "SIGNATURE_TRUST_LEVEL_NO_SIGNATURE"}},
			"liveGridFindings": map[string]any{"reputation": 0.05, "popularity": 0.001}},
		{"uuid": "9a000002-0000-4000-8000-000000000002", "displayName": "AnyDesk.exe", "applicationDisplayName": "AnyDesk", "applicationVersionName": "8.1.0",
			"hashSha1": "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", "isBlocked": false, "devicesCount": 41, "firstOccurTime": "2026-03-02T10:00:00Z",
			"signatures":       []map[string]any{{"signerName": ""}, {"signerName": "philandro Software GmbH", "signatureTrustLevel": "SIGNATURE_TRUST_LEVEL_OS_TRUSTED"}},
			"liveGridFindings": map[string]any{"reputation": "0.92"}},
		{"uuid": "9a000003-0000-4000-8000-000000000003", "displayName": "remote_helper.exe", "applicationDisplayName": "AnyDesk Portable",
			"hashSha1": "ffffffffffffffffffffffffffffffffffffffff", "devicesCount": "2"},
		{"uuid": "9a000004-0000-4000-8000-000000000004", "displayName": "chrome.exe", "applicationDisplayName": "Google Chrome",
			"hashSha1": "0000000000000000000000000000000000000001", "devicesCount": 120},
	}
}

func executablesFindTestDecode(t *testing.T, out string) executablesFindView {
	t.Helper()
	var v executablesFindView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not executables find JSON: %v\n%s", err, out)
	}
	return v
}

func TestExecutablesFindClassifyHash(t *testing.T) {
	cases := []struct {
		in, field string
		ok        bool
	}{
		{executablesFindTestMD5, "hashMd5", true},
		{strings.ToUpper(executablesFindTestSHA1), "hashSha1", true},
		{"  " + executablesFindTestSHA256 + "\n", "hashSha2256", true},
		{"abc123", "", false},                                           // wrong length
		{strings.Repeat("g", 40), "", false},                            // not hex
		{"", "", false},                                                 // empty
		{executablesFindTestSHA1[:39] + "-", "", false},                 // stray punctuation
		{executablesFindTestSHA256 + executablesFindTestMD5, "", false}, // too long
	}
	for _, tc := range cases {
		field, norm, err := executablesFindClassifyHash(tc.in)
		if tc.ok != (err == nil) {
			t.Errorf("classify(%q) err = %v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if tc.ok && (field != tc.field || norm != strings.ToLower(strings.TrimSpace(tc.in))) {
			t.Errorf("classify(%q) = %s %s, want %s", tc.in, field, norm, tc.field)
		}
	}
}

func TestExecutablesFindSearchPure(t *testing.T) {
	raw := make([]json.RawMessage, 0)
	for _, r := range executablesFindTestRows() {
		b, _ := json.Marshal(r)
		raw = append(raw, b)
	}
	raw = append(raw, json.RawMessage(`{"displayName":"no uuid AnyDesk"}`), json.RawMessage(`[]`))

	for _, h := range []string{executablesFindTestSHA1, executablesFindTestMD5, executablesFindTestSHA256} {
		field, norm, _ := executablesFindClassifyHash(h)
		var rej esetRejects
		got := executablesFindSearch(raw, executablesFindQuery{HashField: field, Hash: norm}, &rej)
		if len(got) != 1 || got[0].UUID != "9a000001-0000-4000-8000-000000000001" {
			t.Fatalf("%s lookup = %+v", field, got)
		}
		if rej.N != 2 {
			t.Fatalf("the no-uuid and non-object rows must be counted: %+v", rej)
		}
	}
	row := executablesFindSearch(raw, executablesFindQuery{HashField: "hashSha1", Hash: executablesFindTestSHA1}, nil)[0]
	if !row.IsBlocked || row.DevicesCount != 3 || row.SHA1 != executablesFindTestSHA1 || row.Signer != "" ||
		row.Reputation == nil || *row.Reputation != 0.05 || row.Version != "6.1.7601.17514" || row.FirstSeen != "2026-10-12T22:41:00Z" {
		t.Fatalf("hash row = %+v", row)
	}

	// Name matches displayName or applicationDisplayName, most widespread first.
	got := executablesFindSearch(raw, executablesFindQuery{Name: "anydesk"}, nil)
	if len(got) != 2 || got[0].DisplayName != "AnyDesk.exe" || got[1].DisplayName != "remote_helper.exe" {
		t.Fatalf("name search = %+v", got)
	}
	if got[0].Signer != "philandro Software GmbH" || got[0].Reputation == nil || *got[0].Reputation != 0.92 || got[0].Version != "8.1.0" {
		t.Fatalf("signer/reputation/version fallbacks = %+v", got[0])
	}
	if got[1].Reputation != nil {
		t.Fatalf("absent reputation must stay null, got %v", *got[1].Reputation)
	}
	// A hash on the wrong field must not match: the SHA-1 value as an MD5 query.
	if miss := executablesFindSearch(raw, executablesFindQuery{HashField: "hashMd5", Hash: executablesFindTestSHA1}, nil); len(miss) != 0 {
		t.Fatalf("cross-field hash matched: %+v", miss)
	}
	if empty := executablesFindSearch(nil, executablesFindQuery{Name: "x"}, nil); empty == nil || len(empty) != 0 {
		t.Fatal("empty input must give an empty, non-nil slice")
	}
	// A wrongly typed devicesCount hides the row from the search: it must be
	// counted, since it may be the very executable being looked for.
	var rej esetRejects
	typed := []json.RawMessage{json.RawMessage(`{"uuid":"9a0000ff-0000-4000-8000-0000000000ff","displayName":"AnyDesk.exe","devicesCount":"several"}`)}
	if got := executablesFindSearch(typed, executablesFindQuery{Name: "anydesk"}, &rej); len(got) != 0 || rej.N != 1 || !strings.Contains(rej.First, "executable row 0") {
		t.Fatalf("typed row = %+v / %+v", got, rej)
	}
}

func TestExecutablesFindLocalEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{"executables": executablesFindTestRows()})

	out, err := fleetStaleTestRun(t, "executables", "find", "--hash", strings.ToUpper(executablesFindTestSHA1), "--db", path, "--json")
	if err != nil {
		t.Fatalf("hash find: %v\n%s", err, out)
	}
	v := executablesFindTestDecode(t, out)
	if v.Source != "local" || v.ScannedExecutables != 4 || v.Matched != 1 || len(v.Items) != 1 || v.Truncated {
		t.Fatalf("hash view = %+v", v)
	}
	if it := v.Items[0]; it.UUID != "9a000001-0000-4000-8000-000000000001" || !it.IsBlocked || it.Application != "DiskPart" || it.MD5 != executablesFindTestMD5 {
		t.Fatalf("hash row = %+v", it)
	}

	out, err = fleetStaleTestRun(t, "executables", "find", "--name", "AnyDesk", "--limit", "1", "--db", path, "--json")
	if err != nil {
		t.Fatalf("name find: %v\n%s", err, out)
	}
	v = executablesFindTestDecode(t, out)
	if v.Matched != 2 || len(v.Items) != 1 || v.Items[0].DisplayName != "AnyDesk.exe" || !strings.Contains(v.Note, "showing 1 of 2") {
		t.Fatalf("limited name view = %+v", v)
	}

	out, err = fleetStaleTestRun(t, "executables", "find", "--name", "mimikatz", "--db", path, "--json")
	if err != nil {
		t.Fatalf("no-match find: %v\n%s", err, out)
	}
	v = executablesFindTestDecode(t, out)
	if v.Matched != 0 || len(v.Items) != 0 || !strings.Contains(out, `"items": []`) || !strings.Contains(v.Note, "--live") {
		t.Fatalf("no-match view = %+v\n%s", v, out)
	}

	for _, bad := range [][]string{
		{"--hash", "abc123"},
		{"--hash", executablesFindTestSHA1, "--name", "anydesk"},
		{"--db", path},
		{"--name", "anydesk", "--limit", "0"},
	} {
		if _, err := fleetStaleTestRun(t, append([]string{"executables", "find", "--json"}, bad...)...); err == nil {
			t.Fatalf("%v must be a usage error", bad)
		}
	}
}

// TestExecutablesFindLocalCountsRejectedRows: a stored executable that does
// not decode is reported in rejected_rows and the note, not silently skipped.
func TestExecutablesFindLocalCountsRejectedRows(t *testing.T) {
	testenv.Isolate(t)
	rows := executablesFindTestRows()
	rows = append(rows, map[string]any{"uuid": "9a0000ff-0000-4000-8000-0000000000ff", "displayName": "AnyDesk_old.exe", "devicesCount": "several"})
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{"executables": rows})
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"executables", "find", "--name", "anydesk", "--db", path, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run: %v\n%s", err, errOut.String())
	}
	v := executablesFindTestDecode(t, out.String())
	if v.ScannedExecutables != 5 || v.Matched != 2 || v.RejectedRows != 1 || !strings.Contains(v.Note, "1 executable row(s) could not be decoded") {
		t.Fatalf("view = %+v", v)
	}
	if strings.Count(errOut.String(), "could not be decoded") != 1 {
		t.Fatalf("want one stderr warning, got %q", errOut.String())
	}
	clean, err := fleetStaleTestRun(t, "executables", "find", "--name", "chrome", "--db", fleetStaleTestSeedDB(t, map[string][]map[string]any{"executables": executablesFindTestRows()}), "--json")
	if err != nil || !strings.Contains(clean, `"rejected_rows": 0`) {
		t.Fatalf("rejected_rows must always be present: %v\n%s", err, clean)
	}
}

func TestExecutablesFindLocalOnlyWithoutMirror(t *testing.T) {
	testenv.Isolate(t)
	out, err := fleetStaleTestRun(t, "executables", "find", "--name", "chrome", "--data-source", "local",
		"--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("local-only with no mirror must exit 0: %v", err)
	}
	v := executablesFindTestDecode(t, out)
	if v.Source != "local" || len(v.Items) != 0 || !strings.Contains(v.Note, "no local mirror") {
		t.Fatalf("view = %+v", v)
	}
	if !strings.Contains(out, `"items": []`) {
		t.Fatalf("want an empty items array:\n%s", out)
	}
}
