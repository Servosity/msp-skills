// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program creates or writes a file, except
// os.OpenFile, whose flags decide (writesFile). AtomicWriteCredentialFile is
// the one writer outside the credential kit (the declared exemption for the
// credential store and config.toml), so each of its callers is listed below.
// A credkit file writer is a write site too (it passes by kitMark), so the
// count below measures the tree, not only what is left outside the kit.
var fileWrite = regexp.MustCompile(`os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|AtomicWritePrivateFile\(|AtomicWriteCredentialFile\(|credkit\.(WriteFile|AppendFile|OpenFile|Create)\(`)

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

// kitMark is how a write site shows its data passes the shared credential kit
// (handfixes.json credkit-wiring) within a few lines of it, before or after:
// a credkit file writer, or AtomicWritePrivateFile, which writes through
// credkit.WriteFile.
var kitMark = regexp.MustCompile(`credkit\.(WriteFile|AppendFile|OpenFile|Create|NewWriter)\(|AtomicWritePrivateFile\(`)

// unfilteredWrites are the write sites that may skip the kit, with the
// reason; the key is the file and a fragment of the line. Anything else must
// pass the kit, so a writer a reprint adds fails this test until it is
// routed through a credkit writer or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/credentials.go|AtomicWriteCredentialFile(path, data":                "the 0600 credential store: a credential source (Declarations.Files), the declared exemption",
	"internal/config/config.go|AtomicWriteCredentialFile(c.Path, data":                    "config.toml: a credential source (Declarations.Files), the declared exemption",
	"internal/config/config.go|AtomicWriteCredentialFile(c.legacySourcePath":              "the legacy config scrubbed in place: a credential source (Declarations.Files), the declared exemption",
	"internal/config/config.go|AtomicWriteCredentialFile(snap.path":                       "restores the credential store or config.toml as it was: a credential source, the declared exemption",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":                         "restores the credential store or config.toml as it was: a credential source, the declared exemption",
	"internal/cliutil/paths.go|os.CreateTemp(dir":                                         "inside AtomicWritePrivateFile (credkit.WriteFile) and AtomicWriteCredentialFile; their callers are checked here",
	"internal/cliutil/filelock.go|":                                                       "lock file, no content",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                                     "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/profile.go|os.CreateTemp(dir":                                      "creates the empty temp file; writePrivateFile writes the data with credkit.WriteFile",
	"internal/platform/profile.go|os.O_EXCL":                                              "a random local HMAC key, no API or user data",
	"internal/cli/deliver.go|os.CreateTemp(dir":                                           "creates the empty temp file; the body is written with credkit.WriteFile",
	"internal/cli/store_adopt_linux.go|":                                                  "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":                                             "ownership marker holding a scope hash, no API or user data",
	"internal/store/store.go|journalPath":                                                 "pre-creates the empty SQLite journal at 0600",
	"internal/cli/export.go|os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)": "openExportOutput: creates the private file early (a bad path fails before any request); its one caller closes it and writes the export with credkit.WriteFile",
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

// TestEveryFileWritePassesTheKit: no file but the credential sources can
// receive data that skipped the credential kit (handfixes.json credkit-wiring,
// file sinks).
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
				return nil // the vendored kit: its writers are the sinks every site here routes through
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "func AtomicWritePrivateFile(") || strings.HasPrefix(code, "func AtomicWriteCredentialFile(") || !writesFile(lines, i) {
					continue
				}
				sites++
				window := strings.Join(lines[max(0, i-8):min(len(lines), i+9)], "\n")
				if kitMark.MatchString(window) || allowed(rel, code) {
					continue
				}
				t.Errorf("%s:%d writes a file without the credential kit: %s", rel, i+1, code)
			}
			return nil
		})
	}
	if sites < 20 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
