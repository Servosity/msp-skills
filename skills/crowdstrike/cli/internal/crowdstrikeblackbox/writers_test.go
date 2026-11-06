// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program creates or writes a file outside
// the credential kit, except os.OpenFile, whose flags decide (writesFile).
var fileWrite = regexp.MustCompile(`os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|AtomicWritePrivateFile\(`)

// openFileCall, openFileWriteFlag, openFileReadOnly: an os.OpenFile is a write when a writing
// flag follows the call on its line or the next two. The path argument may
// itself be a call (os.OpenFile(pushesPath(dir), os.O_CREATE|...)), so the
// match must not stop at the first ")". A call whose flags the scan cannot
// read (a variable, no O_RDONLY in sight) counts as a write too.
var (
	openFileCall      = regexp.MustCompile(`os\.OpenFile\(`)
	openFileWriteFlag = regexp.MustCompile(`O_(WRONLY|RDWR|APPEND|CREATE)`)
	openFileReadOnly  = regexp.MustCompile(`O_RDONLY`)
)

// writesFile reports whether line i creates or writes a file.
func writesFile(lines []string, i int) bool {
	code := strings.TrimSpace(lines[i])
	if fileWrite.MatchString(code) {
		return true
	}
	loc := openFileCall.FindStringIndex(code)
	if loc == nil {
		return false
	}
	args := strings.Join(append([]string{code[loc[1]:]}, lines[i+1:min(len(lines), i+3)]...), "\n")
	return openFileWriteFlag.MatchString(args) || !openFileReadOnly.MatchString(args)
}

// unfilteredWrites are the write sites that do not pass the credential kit, with
// the reason; the key is the file and a fragment of the line. A writer a reprint
// adds fails this test until it is routed through credkit or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|os.CreateTemp(dir":                     "atomicWritePrivate's temp file: its two callers are the kit's AtomicWritePrivateFile sink and AtomicWriteCredentialFile, the declared credential files",
	"internal/cliutil/filelock.go|":                                   "lock file, no content",
	"internal/learn/journal.go|journalOffsetFileName":                 "segment name and byte offset only, through AtomicWritePrivateFile (the kit's WriteFile)",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                 "opens a segment to trim a torn last line; writes nothing new",
	"internal/cache/cache.go|":                                        "package has no importer; the response cache is off on every client (crowdstrike_guard.go)",
	"internal/client/client.go|cacheFile":                             "response cache, gated by !c.NoCache; NoCache is set on every client (crowdstrike_guard.go)",
	"internal/platform/ratelimit.go|":                                 "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":                          "a random local HMAC key, no API or user data",
	"internal/platform/profile.go|os.CreateTemp(dir, \".tmp-*\")":     "an empty 0600 temp file; writePrivateFile writes it with credkit.WriteFile",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":     "restores config.toml or credentials.toml, declared credential files (credwire Files), from its own snapshot",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)": "an empty 0600 temp file; writeDownloadUnder writes the body with credkit.WriteFile",
	"internal/cli/store_adopt_linux.go|":                              "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":                         "empty ownership marker",
	"internal/store/store.go|journalPath":                             "pre-creates the empty SQLite journal at 0600",
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
// receive data that skipped the credential kit (handfixes.json credkit-wiring).
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
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "func AtomicWritePrivateFile(") || !writesFile(lines, i) {
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
	if sites < 10 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
