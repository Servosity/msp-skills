// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-auth-logout): auth logout removes only
// session files this CLI can prove it wrote.

package dsm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// managerRecord is the session manager's record (client.sessionRecord plus
// the fleet layer's device_ids) for baseURL, as `auth login` writes it.
func managerRecord(baseURL string) map[string]any {
	return map[string]any{
		"token": "SID-1", "account": "admin", "origin": SessionKey(baseURL), "synotoken": "ST-1",
		"device_id": "DID-1", "saved": time.Now().Format(time.RFC3339Nano),
		"cookies": []any{map[string]any{"url": "https://nas:5001/", "name": "id", "value": "C-1", "path": "/",
			"expires": "0001-01-01T00:00:00Z"}},
		"device_ids": map[string]any{"admin": "DID-1"},
	}
}

// legacyRecord is the origin-less session.json an early build wrote.
func legacyRecord() map[string]any {
	return map[string]any{"token": "SID-LEGACY", "synotoken": "TOK-LEGACY", "cookies": nil,
		"saved": time.Now().UTC().Format(time.RFC3339Nano)}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, string(b))
}

func writeRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func names(files []CachedSession) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(filepath.Dir(f.File))+"/"+filepath.Base(f.File))
	}
	sort.Strings(out)
	return out
}

func skippedNames(files []SkippedFile) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(filepath.Dir(f.File))+"/"+filepath.Base(f.File))
	}
	sort.Strings(out)
	return out
}

// TestSessionCacheFilesAreProvenBeforeRemoval is the review's cfgdir repro
// as a unit: `auth logout --all` deleted every file NAMED like a session
// cache in the config directory (a user's session.json, session-planning-
// 2026.json, .session-keep.tmp). A file is now this CLI's only when its name
// is exactly the CLI's scheme for where it is AND its content is the record
// the CLI writes under that name; everything else is listed as skipped, kept
// byte for byte, and a symbolic link is never followed.
func TestSessionCacheFilesAreProvenBeforeRemoval(t *testing.T) {
	root := t.TempDir()
	data, cfg, outside := filepath.Join(root, "data"), filepath.Join(root, "shared"), filepath.Join(root, "outside")
	for _, d := range []string{data, cfg, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	nas, other := "https://nas:5001", "https://other:5001"

	// This CLI's own files.
	if err := SaveDeviceID(data, nas, "admin", "DID-NAS"); err != nil { // the fleet layer's writer
		t.Fatal(err)
	}
	ownOther := SessionFilePath(data, other)
	rec, _ := json.Marshal(managerRecord(other))
	if err := WriteSessionRecord(ownOther, rec, "admin", "DID-1"); err != nil { // the session manager's writer
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(data, "."+SessionFileName(nas)+".4242.tmp"), managerRecord(nas)) // a crashed atomic write
	writeJSON(t, filepath.Join(data, "session.json"), legacyRecord())
	writeJSON(t, filepath.Join(cfg, ".session.json.77.tmp"), legacyRecord())

	// Files it did not write: kept whatever their name says.
	userFiles := map[string]string{
		filepath.Join(cfg, "session.json"):                                     "client notes\n",
		filepath.Join(cfg, "session-planning-2026.json"):                       "{}",
		filepath.Join(cfg, ".session-keep.tmp"):                                "keep",
		filepath.Join(cfg, SessionFileName(nas)):                               mustJSON(managerRecord(nas)), // a per-NAS name outside the data directory
		filepath.Join(data, SessionFileName("https://fake:5001")):              "{}",
		filepath.Join(data, SessionFileName("https://y:5001")):                 mustJSON(managerRecord("https://x:5001")), // records another NAS
		filepath.Join(data, SessionFileName("https://extra:5001")):             mustJSON(withField(managerRecord("https://extra:5001"), "notes", "mine")),
		filepath.Join(data, SessionFileName("https://types:5001")):             mustJSON(withField(managerRecord("https://types:5001"), "token", 7)),
		filepath.Join(data, "."+SessionFileName("https://torn:5001")+".9.tmp"): `{"token": "SID-TO`,
		filepath.Join(data, "session-nas_5001.json"):                           mustJSON(managerRecord(nas)), // an older file name for nas
		filepath.Join(cfg, "other.json"):                                       "keep",
	}
	for p, c := range userFiles {
		writeRaw(t, p, c)
	}
	if err := os.Mkdir(filepath.Join(data, "session-dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "victim.json")
	writeJSON(t, target, managerRecord("https://link:5001"))
	linked := filepath.Join(data, SessionFileName("https://link:5001"))
	symlinks := os.Symlink(target, linked) == nil // Windows without the privilege: not covered here
	if !symlinks {
		t.Log("symbolic links unavailable on this system; the link case is not exercised")
	}
	dirs := SessionDirs{Data: data, Config: cfg}

	files, skipped, err := CachedSessions("", dirs)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{"data/." + SessionFileName(nas) + ".4242.tmp", "data/" + SessionFileName(nas), "data/" + SessionFileName(other),
		"data/session.json", "shared/.session.json.77.tmp"}
	sort.Strings(wantFiles)
	if got := names(files); strings.Join(got, "|") != strings.Join(wantFiles, "|") {
		t.Fatalf("--all would remove %v, want exactly the CLI's own files %v", got, wantFiles)
	}
	wantSkipped := []string{"data/." + SessionFileName("https://torn:5001") + ".9.tmp", "data/" + SessionFileName("https://extra:5001"),
		"data/" + SessionFileName("https://fake:5001"), "data/" + SessionFileName("https://types:5001"), "data/" + SessionFileName("https://y:5001"),
		"data/session-dir.json", "data/session-nas_5001.json", "shared/.session-keep.tmp", "shared/" + SessionFileName(nas),
		"shared/session-planning-2026.json", "shared/session.json"}
	if symlinks {
		wantSkipped = append(wantSkipped, "data/"+SessionFileName("https://link:5001"))
	}
	sort.Strings(wantSkipped)
	if got := skippedNames(skipped); strings.Join(got, "|") != strings.Join(wantSkipped, "|") {
		t.Fatalf("skipped %v, want %v", got, wantSkipped)
	}
	for _, s := range skipped {
		if !strings.HasPrefix(s.Reason, NotWrittenByCLI+": ") {
			t.Fatalf("skip reason %q does not say the file is not the CLI's", s.Reason)
		}
	}

	// One NAS: its own file and temp file, plus the files that carry its name
	// or record it (listed as skipped, never removed).
	one, oneSkipped, err := CachedSessions(nas, dirs)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(one); strings.Join(got, "|") != "data/."+SessionFileName(nas)+".4242.tmp|data/"+SessionFileName(nas) {
		t.Fatalf("logout of %s would remove %v", nas, got)
	}
	if got := skippedNames(oneSkipped); strings.Join(got, "|") != "data/session-nas_5001.json|shared/"+SessionFileName(nas) {
		t.Fatalf("logout of %s lists as skipped %v", nas, got)
	}

	// A listed file replaced before removal is proven again and kept.
	writeRaw(t, ownOther, "replaced by the user")
	removed, changed, err := ForgetSessionFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != len(files)-1 || len(changed) != 1 || changed[0].File != ownOther {
		t.Fatalf("ForgetSessionFiles removed %v, kept as changed %+v", names(removed), changed)
	}
	for _, f := range removed {
		if _, err := os.Lstat(f.File); !os.IsNotExist(err) {
			t.Fatalf("%s was reported removed but exists", f.File)
		}
	}
	for p, c := range userFiles {
		if b, err := os.ReadFile(p); err != nil || string(b) != c {
			t.Fatalf("%s was not kept byte for byte: %q %v", p, b, err)
		}
	}
	if b, err := os.ReadFile(ownOther); err != nil || string(b) != "replaced by the user" {
		t.Fatalf("the replaced file was removed or changed: %q %v", b, err)
	}
	if fi, err := os.Stat(filepath.Join(data, "session-dir.json")); err != nil || !fi.IsDir() {
		t.Fatal("a directory named like a session file was removed")
	}
	if symlinks {
		if fi, err := os.Lstat(linked); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatal("the symbolic link was removed")
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatal("the symbolic link's target was removed")
		}
	}
}

// TestAtomicWriteTempNamesItsFile: the temp file writeAtomic creates (and a
// crash can leave) maps back to the session file it was written for, so
// auth logout removes it as that file's temp file and nothing else.
func TestAtomicWriteTempNamesItsFile(t *testing.T) {
	dir := t.TempDir()
	for _, base := range []string{"https://nas:5001", "HTTPS://Proxy.Lan/nas1/"} {
		path := SessionFilePath(dir, base)
		f, err := os.CreateTemp(dir, atomicTempPattern(path))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if owner := sessionOwner(filepath.Base(f.Name())); owner != filepath.Base(path) {
			t.Fatalf("writeAtomic's temp file %s maps to %q, want %q", filepath.Base(f.Name()), owner, filepath.Base(path))
		}
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func withField(m map[string]any, k string, v any) map[string]any {
	m[k] = v
	return m
}
