// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package msgraphblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program creates or writes a file.
var fileWrite = regexp.MustCompile(`\bos\.WriteFile\(|\bos\.Create\(|\bos\.CreateTemp\(|\bos\.OpenFile\([^)]*O_(WRONLY|RDWR|APPEND|CREATE)`)

// unfilteredWrites are the write sites that do not pass the credential kit, with
// the reason; the key is the file and a fragment of the line. A writer a reprint
// adds fails this test until it is routed through credkit or reviewed here
// (handfixes.json credkit-every-file-write).
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|os.CreateTemp(dir":                                         "atomicWritePrivate's temp file: its two callers are the kit's AtomicWritePrivateFile sink and AtomicWriteCredentialFile for the declared credential files",
	"internal/cliutil/filelock.go|":                                                       "lock file, no content",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                                     "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/ratelimit.go|":                                                     "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":                                              "a random local HMAC key, no API or user data",
	"internal/platform/profile.go|os.CreateTemp(dir, \".tmp-*\")":                         "an empty 0600 temp file, closed at once; the data is written to it with credkit.WriteFile",
	"internal/cli/store_adopt_linux.go|":                                                  "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":                                             "ownership marker: a scope hash, no API or user data",
	"internal/store/store.go|journalPath":                                                 "pre-creates the empty SQLite journal at 0600",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)":                     "an empty 0600 temp file; the body reaches it with credkit.WriteFile",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":                         "restores the credential store, a declared credential file (Declarations.Files), from its own snapshot",
	"internal/cli/export.go|os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)": "openExportOutput returns it wrapped in credkit.NewWriter(f, credkit.SinkFile)",
}

func allowed(rel, line string) bool {
	for key := range unfilteredWrites {
		file, frag, _ := strings.Cut(key, "|")
		if file == rel && strings.Contains(line, frag) {
			return true
		}
	}
	return false
}

// TestEveryFileWritePassesTheKit: no file but the declared credential files can
// receive data that skipped the credential kit (handfixes.json
// credkit-every-file-write).
func TestEveryFileWritePassesTheKit(t *testing.T) {
	root := filepath.Join("..", "..")
	var sites int
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/credkit/") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || !fileWrite.MatchString(code) {
					continue
				}
				sites++
				if !allowed(rel, code) {
					t.Errorf("%s:%d writes a file without the credential kit: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 5 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
