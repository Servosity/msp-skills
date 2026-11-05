// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package appdirectblackbox

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

// chokepointMark is how a write site shows its data passes the one output
// chokepoint (cliutil.WithholdBytes / WithholdingFile / WithheldWriter)
// within a few lines of it, before or after.
var chokepointMark = regexp.MustCompile(`WithholdBytes\(|WithholdingFile\(|WithheldWriter\(`)

// unfilteredWrites are the write sites that may skip the chokepoint, with the
// reason; the key is the file and a fragment of the line. Anything else must
// pass the chokepoint, so a writer a reprint adds fails this test until it is
// routed through cliutil.WithholdBytes or reviewed here.
var unfilteredWrites = map[string]string{
	"internal/cliutil/credentials.go|AtomicWritePrivateFile(path, data":                   "the 0600 credential store: the one file allowed to hold a credential",
	"internal/cliutil/paths.go|os.CreateTemp(dir":                                         "inside AtomicWritePrivateFile; its callers are checked here",
	"internal/cliutil/filelock.go|":                                                       "lock file, no content",
	"internal/learn/journal.go|journalOffsetFileName":                                     "segment name and byte offset only",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                                     "opens a segment to trim a torn last line; writes nothing new",
	"internal/learn/journal.go|os.O_APPEND|os.O_CREATE|os.O_WRONLY":                       "learn journal: every field is withheld where the entry is built (root.go withheldJournalEntry, teach.go SetJournalLearnContext); journal.go stays byte-identical to the fleet so its reviewed #nosec keeps fleet precedent",
	"internal/cache/cache.go|":                                                            "package has no importer; the response cache is off on every client (appdirect_guard.go)",
	"internal/client/client.go|cacheFile":                                                 "response cache, gated by !c.NoCache; NoCache is set on every client (appdirect_guard.go)",
	"internal/platform/ratelimit.go|":                                                     "rate-limit lock file, no content",
	"internal/platform/profile.go|os.O_EXCL":                                              "a random local HMAC key, no API or user data",
	"internal/config/config.go|":                                                          "config.toml: credentials go to the credential store first (saveCredentialsFirst) and persisted() drops them; snapshots restore that same file",
	"internal/cli/deliver.go|":                                                            "Deliver withholds the body (cliutil.WithholdBytes) before any sink",
	"internal/cli/store_adopt_linux.go|":                                                  "pins the existing store file, writes nothing",
	"internal/cli/helpers.go|.data-db-owner-":                                             "empty ownership marker",
	"internal/store/store.go|journalPath":                                                 "pre-creates the empty SQLite journal at 0600",
	"internal/cli/teach.go|os.WriteFile(path, nil":                                        "truncates to empty",
	"internal/cli/export.go|os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)": "openExportOutput: its one caller wraps it in cliutil.WithholdingFile (tools/apply_credential_rules.py)",
	"internal/mcp/tools.go|os.WriteFile(path, data, 0o600)":                               "writeMCPUploadTemp: stages the operator's own upload file, read from the upload root, in a private temp dir for one multipart send to the pinned API, removed after the call; no API or credential data",
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
// receive data that skipped the chokepoint (appdirect handfix: output-chokepoint,
// file sinks).
func TestEveryFileWritePassesTheChokepoint(t *testing.T) {
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
				if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "func AtomicWritePrivateFile(") || !writesFile(lines, i) {
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
	if sites < 20 {
		t.Fatalf("found only %d write sites; the scan is not reading the tree", sites)
	}
}
