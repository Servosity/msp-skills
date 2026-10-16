// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var quotedCommandRE = regexp.MustCompile(`'watchguard-cli ([a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)*)`)

// hand-fix (handfixes.json hints-name-real-commands): error and doctor hints
// told users to run 'watchguard-cli auth setup', which does not exist, and
// labeled a support-contact page as the API docs. Every command a hint quotes
// must resolve in the command tree, and "See API docs" must point at the API
// documentation.
func TestHintsNameRealCommandsAndAPIDocs(t *testing.T) {
	root := RootCmd()
	var files []string
	for _, dir := range []string{".", "../mcp"} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, m := range matches {
			if !strings.HasSuffix(m, "_test.go") {
				files = append(files, m)
			}
		}
	}
	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range quotedCommandRE.FindAllStringSubmatch(src, -1) {
			words := strings.Fields(m[1])
			cmd, rest, err := root.Find(words)
			checked++
			if err != nil || cmd == root || len(rest) > 0 {
				t.Errorf("%s quotes 'watchguard-cli %s', which is not a command", filepath.Base(f), m[1])
			}
		}
		for _, line := range strings.Split(src, "\n") {
			if strings.Contains(line, "API docs") && strings.Contains(line, "https://") && !strings.Contains(line, "https://www.watchguard.com/help/docs/API/") {
				t.Errorf("%s labels a non-API-docs URL as API docs: %s", filepath.Base(f), strings.TrimSpace(line))
			}
		}
	}
	if checked < 10 {
		t.Fatalf("only %d quoted commands found; the scan is not reading the sources", checked)
	}
}
