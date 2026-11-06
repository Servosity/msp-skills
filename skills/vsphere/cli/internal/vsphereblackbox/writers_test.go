// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

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
// adds fails this test until it is routed through credkit or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|os.CreateTemp(dir":                           "atomicWritePrivate's temp file: its two callers are the kit's AtomicWritePrivateFile sink and the declared credential files",
	"internal/cliutil/filelock.go|":                                         "lock file, no content",
	"internal/learn/journal.go|journalOffsetFileName":                       "segment name and byte offset only",
	"internal/learn/journal.go|os.O_RDWR":                                   "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/ratelimit.go|":                                       "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":                                "a random local HMAC key, no API or user data",
	"internal/platform/profile.go|os.CreateTemp(dir, \".tmp-*\")":           "a temp file written through credkit in the next lines, or a declared credential file",
	"internal/cli/store_adopt_linux.go|":                                    "pins the existing store file, writes nothing",
	"internal/store/store.go|journalPath":                                   "pre-creates the empty SQLite journal at 0600",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)":       "an empty 0600 temp file; writeDownloadUnder writes the body with credkit.WriteFile",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":           "restores config.toml, a declared credential file, from its own snapshot",
	"internal/cli/teach.go|os.WriteFile(path, nil":                          "truncates to empty",
	"internal/cli/rvtools_export.go|os.CreateTemp(dir, \".rvtools-*.csv\")": "each row is written through credkit.NewWriter",
	"internal/cli/export.go|os.CreateTemp(dir, \".export-*.tmp\")":          "an empty 0600 temp file renamed over --output once the export finished; every byte reaches it through credkit.NewWriter",
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
// receive data that skipped the credential kit.
func TestEveryFileWritePassesTheKit(t *testing.T) {
	t.Parallel()
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
