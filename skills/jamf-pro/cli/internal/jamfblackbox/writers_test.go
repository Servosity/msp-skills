// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWrite matches every way this program writes a file past the credential
// kit. A write through credkit.WriteFile, AppendFile, OpenFile, Create or a
// credkit.Writer is checked by the kit, so it does not match.
var fileWrite = regexp.MustCompile(`os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|AtomicWriteCredentialFile\(|os\.OpenFile\([^)]*O_(WRONLY|RDWR|APPEND|CREATE)`)

// unfilteredWrites are the write sites that may skip the kit, with the reason;
// the key is the file and a fragment of the line. Anything else fails this test,
// so a writer a reprint adds fails until it is routed through the kit or reviewed.
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|": "the private-file writer's own temp file, and AtomicWriteCredentialFile, the declared exemption",
	"internal/cliutil/credentials.go|AtomicWriteCredentialFile(path, data": "the 0600 credential store: the one file allowed to hold a credential",
	"internal/config/config.go|AtomicWriteCredentialFile(":                 "config.toml: named in Declarations.Files, so whatever it holds is held at startup",
	"internal/cli/jamf_token_auth.go|os.CreateTemp(filepath.Dir(marker)":   "an empty temp file; its content goes through credkit.WriteFile",
	"internal/cliutil/filelock.go|":                                        "lock file, no content",
	"internal/cli/store_adopt_linux.go|":                                   "pins the existing store file, writes nothing",
	"internal/store/store.go|journalPath":                                  "pre-creates the empty SQLite journal at 0600",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)":      "an empty temp file; its content goes through credkit.WriteFile",
	"internal/cli/helpers.go|.data-db-owner-":                              "empty ownership marker",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":          "puts config.toml's own earlier bytes back after a failed save",
	"internal/learn/journal.go|os.O_RDWR|os.O_APPEND":                      "opens a segment to trim a torn last line; writes nothing new",
	"internal/platform/profile.go|os.CreateTemp(dir, \".tmp-*\")":          "the profile writer's temp file: profile.go, checked by the kit before it is written",
	"internal/platform/profile.go|os.O_EXCL":                               "a random local HMAC key, no API or user data",
	"internal/platform/ratelimit.go|":                                      "rate-limit lock file, no content",
	"internal/cli/teach.go|os.WriteFile(path, nil":                         "truncates to empty",
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

// TestEveryFileWriteGoesThroughTheKit: no file but the credential store and the
// config file its exemption names can receive data the kit did not check
// (handfixes.json credkit-wiring).
func TestEveryFileWriteGoesThroughTheKit(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	kitWrites := regexp.MustCompile(`credkit\.(WriteFile|AppendFile|OpenFile|Create|NewWriter)\(`)
	var sites, routed int
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "credkit" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				code := strings.TrimSpace(line)
				if strings.HasPrefix(code, "//") {
					continue
				}
				if kitWrites.MatchString(code) {
					routed++
				}
				if !fileWrite.MatchString(code) {
					continue
				}
				sites++
				if !allowed(rel, code) {
					t.Errorf("%s:%d writes a file past the kit: %s", rel, i+1, code)
				}
			}
			return nil
		})
	}
	if sites < 3 || routed < 5 {
		t.Fatalf("found %d raw and %d routed write sites; the scan is not reading the tree", sites, routed)
	}
}
