// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json mcp-blocks-local-file-inputs.

package cobratree

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
)

func TestFileInputFlagsAreBlockedArgs(t *testing.T) {
	root := &cobra.Command{Use: "jira-service-management-cli"}
	cmd := &cobra.Command{Use: "teach-playbook", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().String("notes-file", "", "")
	cmd.Flags().String("notes", "", "")
	root.AddCommand(cmd)
	blocked := blockedStructuredArgsForCommand(cmd)
	if !blocked["notes-file"] || blocked["notes"] {
		t.Fatalf("notes-file must be blocked and inline notes allowed: %v", blocked)
	}
}

// TestEveryFileInputFlagIsBlocked scans internal/cli for flags whose variable
// becomes a multipart file part or is opened directly, so a reprint that adds
// one fails here instead of exposing a local-file read over MCP.
func TestEveryFileInputFlagIsBlocked(t *testing.T) {
	listed := map[string]bool{}
	for _, names := range fileInputFlags {
		for _, n := range names {
			listed[n] = true
		}
	}
	files, _ := filepath.Glob("../../cli/*.go")
	if len(files) == 0 {
		t.Fatal("no internal/cli sources found")
	}
	bind := regexp.MustCompile(`StringVarP?\(&(\w+), "([\w-]+)"`)
	use := regexp.MustCompile(`fileFields\["[^"]+"\] = (\w+)|os\.(?:Open|ReadFile|OpenFile)\((?:filepath\.Clean\()?(\w+)`)
	found := 0
	for _, f := range files {
		if filepath.Ext(f) != ".go" || regexp.MustCompile(`_test\.go$`).MatchString(f) {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		flagOf := map[string]string{}
		for _, m := range bind.FindAllStringSubmatch(string(src), -1) {
			flagOf[m[1]] = m[2]
		}
		for _, m := range use.FindAllStringSubmatch(string(src), -1) {
			v := m[1] + m[2]
			if flag, ok := flagOf[v]; ok {
				found++
				if !listed[flag] {
					t.Errorf("%s: --%s is read as a local file but is not in fileInputFlags", filepath.Base(f), flag)
				}
			}
		}
	}
	if found < 3 { // upload-temporary-file --file, teach-playbook --notes-file, import --input
		t.Fatalf("scan found only %d file-input flags; the patterns no longer match the generator", found)
	}
}
