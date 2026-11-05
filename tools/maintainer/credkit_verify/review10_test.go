// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Loop ruling 15b: a cheap visible marker of code built to get past the check
// fails closed unless the ledger names it with a reason. That is the whole
// defense against such code; a way around a marker is out of scope.

func init() { cases = append(cases, review10...) }

const elfHelper = "\x7fELF\x02\x01\x01\x00 helper"

func markerEntry(file, what string) string {
	return `{"file": "` + file + `", "func": "file", "what": "` + what + `", "count": 1, "kind": "exempt", "reason": "named by hand for this test"}`
}

var review10 = []tcase{
	strict(tcase{name: "rv10 fail: a binary in the connector's tree",
		files: map[string]string{"assets/helper": elfHelper},
		want:  []string{"rule 2|assets/helper|binary"}}),
	{name: "rv10 pass: a binary the ledger names",
		files: map[string]string{"assets/helper": elfHelper},
		sites: []string{markerEntry("assets/helper", "binary")}},
	strict(tcase{name: "rv10 fail: a Windows binary among test data",
		files: map[string]string{"internal/cli/testdata/h.exe": "MZ\x90\x00\x03\x00\x00\x00 stub"},
		want:  []string{"rule 2|internal/cli/testdata/h.exe|binary"}}),
	strict(tcase{name: "rv10 fail: a Mach-O binary",
		files: map[string]string{"h": "\xcf\xfa\xed\xfe\x07\x00\x00\x01"},
		want:  []string{"rule 2|h|binary"}}),
	strict(tcase{name: "rv10 fail: a Go archive",
		files: map[string]string{"x.a": "!<arch>\n__.PKGDEF"},
		want:  []string{"rule 2|x.a|binary"}}),
	{name: "rv10 pass: a text file that starts like a Windows binary, and ordinary data",
		files: map[string]string{"a.txt": "MZ is not a binary, this line is text\n", "b.png": "\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"}},
	{name: "rv10 pass: the vendored kit is not the connector's",
		files: map[string]string{"internal/credkit/h": elfHelper}},

	{name: "rv10 pass: assembly the ledger names",
		files: map[string]string{"internal/cli/x_amd64.s": "// nothing\n"},
		sites: []string{markerEntry("internal/cli/x_amd64.s", "assembly")}},
	{name: "rv10 pass: a go:linkname the ledger names",
		files: map[string]string{"internal/cli/ln.go": "package cli\n\nimport _ \"unsafe\"\n\n//go:linkname nanotime runtime.nanotime\nfunc nanotime() int64\n"},
		sites: []string{markerEntry("internal/cli/ln.go", "go:linkname")}},
	{name: "rv10 pass: cgo the ledger names",
		files: map[string]string{"internal/cli/c.go": "//go:build cgo\n\npackage cli\n\n// int one() { return 1; }\nimport \"C\"\n\nfunc one() int { return int(C.one()) }\n"},
		sites: []string{markerEntry("internal/cli/c.go", "cgo")}},
	// The kit's documented way to read --config. Start holds what it returns, so
	// a path function may call it. The kit's other functions stay refused.
	{name: "rv10 pass: a path function reads --config with credkit.FlagValues",
		files: map[string]string{"internal/config/config.go": flagConfig("if v := credkit.FlagValues(args, \"--config\"); len(v) > 0 {\n\t\treturn v[len(v)-1]\n\t}")},
		edits: argsWiring},
	strict(tcase{name: "rv10 fail: a path function calls another kit function",
		files: map[string]string{"internal/config/config.go": flagConfig("if credkit.MCPOut() == nil {\n\t\treturn args[0]\n\t}")},
		edits: argsWiring,
		want:  []string{"rule 4|via|MCPOut"}}),
	strict(tcase{name: "rv10 fail: FlagValues as a value is not called",
		files: map[string]string{"internal/config/config.go": flagConfig("f := credkit.FlagValues\n\tif v := f(args, \"--config\"); len(v) > 0 {\n\t\treturn v[0]\n\t}")},
		edits: argsWiring,
		want:  []string{"rule 4|via|function value"}}),
	strict(tcase{name: "rv10 fail: a marker entry with no reason is not a clearance",
		files: map[string]string{"internal/cli/x_amd64.s": "// nothing\n"},
		sites: []string{`{"file": "internal/cli/x_amd64.s", "func": "file", "what": "assembly", "count": 1, "kind": "exempt", "reason": " "}`},
		want:  []string{"rule 2|internal/cli/x_amd64.s|assembly", "ledger|has no reason"}}),
}

// flagConfig is the base config with a command-line path function whose body
// starts with stmt.
func flagConfig(stmt string) string {
	return "package config\n\nimport (\n\t\"encoding/json\"\n\t\"os\"\n\n\t\"acme-pp-cli/internal/credkit\"\n)\n\n" + baseConfigType +
		"\nfunc Path(args []string) string {\n\t" + stmt + "\n\treturn \"acme.toml\"\n}\n\n" +
		"func Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(os.Args[1:]))\n\tvar c Config\n\tif err == nil {\n\t\terr = json.Unmarshal(b, &c)\n\t}\n\treturn &c, err\n}\n"
}

// TestBinaryGitIgnored: a build leaves the connector's binary in the tree on a
// developer's machine. Git ignores it, so it is not in the tree; the same file
// once tracked is.
func TestBinaryGitIgnored(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	cli := filepath.Join(dir, "cli")
	copyTree(t, "testdata/base/cli", cli)
	must(t, os.WriteFile(filepath.Join(cli, ".gitignore"), []byte("/built\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(cli, "built"), []byte(elfHelper), 0o755))
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command(git, args...)
		c.Dir = dir
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, b)
		}
	}
	runGit("init", "-q")
	if got := binaries(cli); len(got) != 0 {
		t.Errorf("an ignored build output counted as in the tree: %v", got)
	}
	runGit("add", "-f", "cli/built")
	if got := binaries(cli); len(got) != 1 || got[0] != "built" {
		t.Errorf("a tracked binary not found: %v", got)
	}
}
