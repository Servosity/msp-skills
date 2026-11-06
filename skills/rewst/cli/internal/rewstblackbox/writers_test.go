// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program creates or writes a file, except
// os.OpenFile, whose flags decide (writesFile).
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

// kitWrite is a file write the credential kit routes (credkit.WriteFile,
// AppendFile, OpenFile: handfixes.json credkit-wiring): the kit withholds a
// record that carries a held value.
var kitWrite = regexp.MustCompile(`credkit\.(WriteFile|AppendFile|OpenFile)\(`)

// unfilteredWrites are the write sites that go outside the kit, with the
// reason; the key is the file and a fragment of the line. Anything else must
// write through the kit, so a writer a reprint adds fails this test until it
// is routed through credkit or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/credentials.go|AtomicWritePrivateFile(path, data": "the 0600 credential store: a credential source, written outside the kit (publish-common item 7; credwire Files)",
	"internal/config/config.go|":                                        "config.toml and its snapshots: a credential source, written outside the kit (publish-common item 7; credwire Files)",
	"internal/cliutil/paths.go|os.CreateTemp(dir":                       "inside AtomicWritePrivateFile; its callers are checked here",
	"internal/cliutil/filelock.go|":                                     "lock file, no content",
	"internal/learn/journal.go|journalOffsetFileName":                   "segment name and byte offset only",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                   "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/ratelimit.go|":                                   "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":                            "a random local HMAC key, no API or user data",
	"internal/platform/profile.go|os.CreateTemp(dir":                    "press template writePrivateFile: client profiles, the rate-limit state and --receipt / --audit-dir receipts (argv as the press redacts it, digests and counts); the battery's receipt and audit runs are scanned by TestNoCredentialInAnyWrittenFile",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)":   "reserves the download's name, empty; the body is written by credkit.WriteFile",
	"internal/cli/store_adopt_linux.go|":                                "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":                           "empty ownership marker",
	"internal/store/store.go|journalPath":                               "pre-creates the empty SQLite journal at 0600",
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

// TestEveryFileWritePassesTheChokepoint: every file the program writes goes
// through the credential kit or is a reviewed exemption above (the
// credential sources, empty or content-free files), so no file but a
// credential source can receive a held value (leak contract section 5).
func TestEveryFileWritePassesTheChokepoint(t *testing.T) {
	budget(t)
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
				return nil // the kit itself (vendored, guard 11)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "func AtomicWritePrivateFile(") {
					continue
				}
				if kitWrite.MatchString(code) {
					sites++
					continue
				}
				if !writesFile(lines, i) {
					continue
				}
				sites++
				if !allowed(rel, code) {
					t.Errorf("%s:%d writes a file outside the credential kit: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 20 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
