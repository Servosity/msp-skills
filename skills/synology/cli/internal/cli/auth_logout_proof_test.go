// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-auth-logout): logout removes only files it
// can prove it wrote, and its preview and run print one shape.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

type logoutProofJSON struct {
	SessionFiles []dsm.CachedSession `json:"session_files"`
	Skipped      []dsm.SkippedFile   `json:"skipped"`
	Cleared      bool                `json:"cleared"`
	Result       string              `json:"result"`
}

func runLogoutProof(t *testing.T, args ...string) (logoutProofJSON, string) {
	t.Helper()
	out, errText, code := runCLIErr(t, append([]string{"auth", "logout", "--json"}, args...)...)
	var rep logoutProofJSON
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil {
		t.Fatalf("auth logout %v: exit %d\n%s%s", args, code, out, errText)
	}
	return rep, out
}

func baseNames(paths []string) string {
	var out []string
	for _, p := range paths {
		out = append(out, filepath.Base(p))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func sessionFileNames(files []dsm.CachedSession) string {
	var p []string
	for _, f := range files {
		p = append(p, f.File)
	}
	return baseNames(p)
}

func skippedFileNames(files []dsm.SkippedFile) string {
	var p []string
	for _, f := range files {
		p = append(p, f.File)
	}
	return baseNames(p)
}

// TestAuthLogoutKeepsFilesItDidNotWrite is the review's cfgdir and lg
// repros: with the config in a shared or per-client folder
// (SYNOLOGY_CONFIG_DIR, --config), `auth logout --all` deleted a user's
// session.json, session-planning-2026.json and .session-keep.tmp because of
// their names. It now removes the real login's session file (data
// directory) and the legacy session.json only when its content is the
// CLI's record, and lists the user's files as skipped, kept byte for byte.
func TestAuthLogoutKeepsFilesItDidNotWrite(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	nas := dsmtest.NewTwoFactorNAS(t, "")
	loginWithCode(t, nas)
	if files := sessionFilesOnDisk(t); len(files) != 1 {
		t.Fatalf("auth login cached %v", files)
	}
	freshShell(t)

	shared := t.TempDir()
	user := map[string]string{
		filepath.Join(shared, "session.json"):               "client notes\n",
		filepath.Join(shared, "session-planning-2026.json"): "{}",
		filepath.Join(shared, ".session-keep.tmp"):          "keep",
		filepath.Join(shared, "other.json"):                 "keep",
	}
	for p, c := range user {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SYNOLOGY_CONFIG_DIR", shared)
	help, _, _ := runCLIErr(t, "auth", "logout", "--help")
	for _, want := range []string{"Forget every DSM session file this CLI wrote in its data directory", "any other file is kept",
		"Only files this CLI can prove it wrote are removed", "never followed"} {
		if !strings.Contains(strings.Join(strings.Fields(help), " "), want) {
			t.Fatalf("auth logout --help must say %q:\n%s", want, help)
		}
	}
	const wantSkipped = ".session-keep.tmp,session-planning-2026.json,session.json"

	rep, text := runLogoutProof(t, "--all", "--dry-run")
	if len(rep.SessionFiles) != 1 || skippedFileNames(rep.Skipped) != wantSkipped || !strings.Contains(rep.Result, "would be kept") {
		t.Fatalf("--all --dry-run with a shared config folder:\n%s", text)
	}
	rep, text = runLogoutProof(t, "--all")
	if len(rep.SessionFiles) != 1 || !rep.Cleared || skippedFileNames(rep.Skipped) != wantSkipped {
		t.Fatalf("--all with a shared config folder:\n%s", text)
	}
	for _, s := range rep.Skipped {
		if !strings.HasPrefix(s.Reason, dsm.NotWrittenByCLI) {
			t.Fatalf("skip reason %q", s.Reason)
		}
	}
	if files := sessionFilesOnDisk(t); len(files) != 0 {
		t.Fatalf("the real login's session file survived --all: %v", files)
	}
	for p, c := range user {
		if b, err := os.ReadFile(p); err != nil || string(b) != c {
			t.Fatalf("auth logout --all changed %s: %q %v", p, b, err)
		}
	}
	out, errText, code := runCLIErr(t, "auth", "logout", "--all")
	if code != 0 || !strings.Contains(out, "skipped "+filepath.Join(shared, "session.json")+" ("+dsm.NotWrittenByCLI) || !strings.Contains(out, "; kept)") {
		t.Fatalf("human auth logout --all must list the kept files: exit %d\n%s%s", code, out, errText)
	}

	// --config <dir>/app.json: the same for a per-client folder, and an early
	// build's session.json there IS removed when its content is the CLI's
	// legacy record.
	t.Setenv("SYNOLOGY_CONFIG_DIR", "")
	userdir, legacydir := t.TempDir(), t.TempDir()
	for name, c := range map[string]string{"session.json": "notes", "session-2026-q3-notes.json": `{"q3": 1}`, ".session-keep.tmp": "x"} {
		if err := os.WriteFile(filepath.Join(userdir, name), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacy, _ := json.Marshal(map[string]any{"token": "SID-LEGACY", "synotoken": "TOK-LEGACY", "saved": time.Now().UTC().Format(time.RFC3339Nano),
		"cookies": []any{map[string]any{"url": "https://127.0.0.1/", "name": "id", "value": "C", "path": "/", "expires": "0001-01-01T00:00:00Z"}}})
	if err := os.WriteFile(filepath.Join(legacydir, "session.json"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	rep, text = runLogoutProof(t, "--all", "--config", filepath.Join(userdir, "app.json"))
	if len(rep.SessionFiles) != 0 || skippedFileNames(rep.Skipped) != ".session-keep.tmp,session-2026-q3-notes.json,session.json" {
		t.Fatalf("--all --config <per-client folder>:\n%s", text)
	}
	if entries, _ := os.ReadDir(userdir); len(entries) != 3 {
		t.Fatalf("--all --config removed files from the per-client folder: %v", entries)
	}
	rep, text = runLogoutProof(t, "--all", "--config", filepath.Join(legacydir, "app.json"))
	if sessionFileNames(rep.SessionFiles) != "session.json" || !rep.SessionFiles[0].SessionID || len(rep.Skipped) != 0 {
		t.Fatalf("--all must remove the CLI's own legacy session.json:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(legacydir, "session.json")); !os.IsNotExist(err) {
		t.Fatal("the legacy session.json survived")
	}
}

// TestAuthLogoutPreviewAndRunHaveOneShape: with nothing cached the dry-run
// printed "session_files": null while the run printed []. Every list is now
// [] in both, for one NAS and for --all, and the two carry the same keys.
func TestAuthLogoutPreviewAndRunHaveOneShape(t *testing.T) {
	sweepIsolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "https://nas1.client.lan:5001")
	for _, scope := range [][]string{nil, {"--all"}} {
		shapes := map[bool]map[string]any{}
		for _, dry := range []bool{true, false} {
			args := append([]string{"auth", "logout", "--json"}, scope...)
			if dry {
				args = append(args, "--dry-run")
			}
			out, errText, code := runCLIErr(t, args...)
			m := map[string]any{}
			if code != 0 || json.Unmarshal([]byte(out), &m) != nil {
				t.Fatalf("%v: exit %d\n%s%s", args, code, out, errText)
			}
			for k, v := range m {
				if v == nil {
					t.Fatalf("%v: %q is null:\n%s", args, k, out)
				}
			}
			for _, k := range []string{"session_files", "skipped", "stored_credentials", "env_still_set"} {
				if l, ok := m[k].([]any); !ok || len(l) != 0 {
					t.Fatalf("%v: %q = %#v, want []:\n%s", args, k, m[k], out)
				}
			}
			delete(m, "dry_run")
			shapes[dry] = m
		}
		for k := range shapes[true] {
			if _, ok := shapes[false][k]; !ok {
				t.Fatalf("auth logout %v: the preview has %q, the run does not", scope, k)
			}
		}
		for k := range shapes[false] {
			if _, ok := shapes[true][k]; !ok {
				t.Fatalf("auth logout %v: the run has %q, the preview does not", scope, k)
			}
		}
	}
}
