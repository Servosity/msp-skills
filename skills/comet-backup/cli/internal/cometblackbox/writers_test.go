// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program creates or writes a file.
var fileWrite = regexp.MustCompile(`os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|os\.OpenFile\([^)]*O_(WRONLY|RDWR|APPEND|CREATE)`)

// chokepointMark is how a write site shows its data passes the one output
// credential kit (credkit.WriteFile / AppendFile / OpenFile / Create / NewWriter).
var chokepointMark = regexp.MustCompile(`credkit\.(WriteFile|AppendFile|OpenFile|Create|NewWriter)\(`)

// unfilteredWrites are the write sites that may skip the chokepoint, with the
// reason; the key is the file and a fragment of the line. Anything else must
// pass the chokepoint, so a writer a reprint adds fails this test until it is
// routed through the kit or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|os.CreateTemp(dir":     "inside atomicWritePrivate; AtomicWritePrivateFile writes through the kit and AtomicWriteCredentialFile is the declared exemption",
	"internal/learn/journal.go|journalOffsetFileName": "written through AtomicWritePrivateFile, which is the kit's",
	"internal/platform/profile.go|os.CreateTemp(dir":  "the temp name only; the data is written with credkit.WriteFile below",
	"internal/cli/deliver.go|os.CreateTemp(dir":       "the temp name only; the body is written with credkit.WriteFile below",
	"internal/cliutil/filelock.go|":                   "lock file, no content",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND": "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/ratelimit.go|":                 "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":          "a random local HMAC key, no API or user data",
	"internal/config/config.go|":                      "config.toml is a credential source named in Declarations.Files: its rewrites are the declared exemption (AtomicWriteCredentialFile)",
	"internal/cliutil/filelock.go|O_CREATE":           "lock file, no content",
	"internal/cli/store_adopt_linux.go|":              "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":         "empty ownership marker",
	"internal/store/store.go|journalPath":             "pre-creates the empty SQLite journal at 0600",
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

// TestEveryFileWritePassesTheChokepoint: no file but the credential store can
// receive data that skipped the chokepoint (comet-backup handfix: output-chokepoint,
// file sinks).
func TestEveryFileWritePassesTheChokepoint(t *testing.T) {
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
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(src), "\n")
			for i, line := range lines {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") || !fileWrite.MatchString(code) {
					continue
				}
				sites++
				window := strings.Join(lines[max(0, i-8):min(len(lines), i+9)], "\n")
				if chokepointMark.MatchString(window) || allowed(rel, code) {
					continue
				}
				t.Errorf("%s:%d writes a file without the chokepoint: %s", rel, i+1, code)
			}
			return nil
		})
	}
	if sites < 5 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
