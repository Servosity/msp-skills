// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 1 (verify-codex-r1): one pair of cases per class, each
// written before its fix and failing on the checker without it.

func init() { cases = append(cases, review1...) }

// file is a case file with its own imports.
func file(imports, body string) map[string]string {
	return map[string]string{caseFile: "package cli\n\nimport (\n" + imports + "\n)\n\n" + body}
}

var review1 = []tcase{
	// Class: the ledger may not waive the prologue.
	{name: "rv1 fail: a ledger entry does not waive a missing Start",
		edits: [][3]string{{"cmd/acme-cli/main.go", "\tcredkit.Start(cli.Declarations)\n", ""}},
		sites: []string{`{"file": "cmd/acme-cli/main.go", "func": "main", "what": "credkit.Start", "count": 1, "kind": "exempt", "reason": "delegates startup"}`},
		want:  []string{"rule 1|cmd/acme-cli/main.go|main's first statement", "rule 0|credkit-sites.json|stale"}},

	{name: "rv1 fail: a ledger entry does not waive a Start that is not first",
		edits: [][3]string{{"cmd/acme-cli/main.go", "\tcredkit.Start(cli.Declarations)\n\tcredkit.Exit(cli.Execute())", "\tcode := cli.Execute()\n\tcredkit.Start(cli.Declarations)\n\tcredkit.Exit(code)"}},
		sites: []string{`{"file": "cmd/acme-cli/main.go", "func": "main", "what": "credkit.Start", "count": 1, "kind": "exempt", "reason": "delegates startup"}`},
		want:  []string{"rule 1|cmd/acme-cli/main.go|main's first statement", "rule 1|cmd/acme-cli/main.go|other than as the first statement", "rule 0|credkit-sites.json|stale"}},

	// Class: output before Start through any writer, and through a local
	// type's methods reached by an interface.
	{name: "rv1 fail: an encoder writes to stdout in init",
		files: file(`"encoding/json"
	"os"`, "func init() { _ = json.NewEncoder(os.Stdout).Encode(\"x\") }\n"),
		want: []string{"rule 2|case.go|before credkit.Start"}},
	{name: "rv1 pass: an encoder writes to memory in init",
		files: file(`"bytes"
	"encoding/json"`, "var buf bytes.Buffer\n\nfunc init() { _ = json.NewEncoder(&buf).Encode(\"x\") }\n")},
	{name: "rv1 fail: output in a method init reaches through an interface",
		files: file(`"fmt"`, "type banner struct{}\n\nfunc (banner) String() string { fmt.Println(\"hi\"); return \"\" }\n\nfunc init() { var s fmt.Stringer = banner{}; _ = s.String() }\n"),
		want:  []string{"rule 2|case.go|fmt.Println|before credkit.Start"}},

	// Class: os.Stdin is the one writable *os.File the kit does not cover.
	{name: "rv1 fail: a write to os.Stdin",
		files: file(`"os"`, "func leak() { _, _ = os.Stdin.WriteString(\"x\") }\n"),
		want:  []string{"rule 2|case.go|os.Stdin"}},
	{name: "rv1 fail: os.Stdin handed on as an *os.File",
		files: file(`"os"`, "func save(f *os.File) { _, _ = f.WriteString(\"x\") }\n\nfunc leak() { save(os.Stdin) }\n"),
		want:  []string{"rule 2|case.go|os.Stdin"}},
	{name: "rv1 fail: a reader asserted back to a writer",
		files: file(`"io"
	"os"`, "func leak(r io.Reader) { _ = r.(*os.File) }\n"),
		want: []string{"rule 2|case.go|assert:writer"}},
	{name: "rv1 pass: os.Stdin read",
		files: file(`"io"
	"os"`, "func g() { _, _ = io.ReadAll(os.Stdin); var r io.Reader = os.Stdin; _ = r }\n")},

	// Class: an http.Client value made without a checked literal.
	one("rv1 fail: an array of client values", "func leak() { var cs [1]http.Client; _, _ = cs[0].Get(\"x\") }\n", "rule 2|case.go|net/http.Client|by value"),
	one("rv1 fail: a slice of client values made", "func leak() { cs := make([]http.Client, 1); _, _ = cs[0].Get(\"x\") }\n", "rule 2|case.go|net/http.Client|by value"),
	one("rv1 pass: a slice of client pointers", "func g() { cs := make([]*http.Client, 1); _ = cs }\n"),

	// Class: parentheses around a checked shape.
	one("rv1 fail: the transport's address taken in parentheses", "func g(c *http.Client) { p := &(c.Transport); _ = p }\n", "rule 2|case.go|net/http.Client.Transport|address"),
	one("rv1 fail: the transport assigned in parentheses", "func g(c *http.Client) { (c.Transport) = nil }\n", "rule 2|case.go|net/http.Client.Transport"),

	// Class: a generic type argument is a conversion to its constraint.
	one("rv1 fail: a command passed as a generic type argument", "func run[T interface{ Run() error }](x T) { _ = x.Run() }\n\nfunc g() { run(exec.Command(\"true\")) }\n", "rule 2|case.go|generic:os/exec.Cmd", "rule 2|case.go|sink:os/exec.Cmd|becomes a T"),

	// Class: a SQL registration must run before any open.
	one("rv1 fail: a wrapped registration that never runs", "func neverCalled() { sql.Register(\"other\", credkit.SQLDriver(fakeDriver{})) }\n\nfunc g() { _, _ = sql.Open(\"other\", \"x\") }\n", "rule 2|case.go|database/sql.Open", "rule 2|case.go|database/sql.Register|not among the leading statements"),
	{name: "rv1 fail: a wrapped registration in a package the opener does not import",
		files: map[string]string{
			"internal/other/other.go": "package other\n\nimport (\n\t\"database/sql\"\n\t\"database/sql/driver\"\n\n\t\"acme-pp-cli/internal/credkit\"\n)\n\ntype d struct{}\n\nfunc (d) Open(string) (driver.Conn, error) { return nil, nil }\n\nfunc init() { sql.Register(\"other\", credkit.SQLDriver(d{})) }\n",
			caseFile:                  pkgCli("func g() { _, _ = sql.Open(\"other\", \"x\") }\n")},
		want: []string{"rule 2|case.go|database/sql.Open"}},
	one("rv1 pass: a wrapped registration in init", "func init() { sql.Register(\"other\", credkit.SQLDriver(fakeDriver{})) }\n\nfunc g() { _, _ = sql.Open(\"other\", \"x\") }\n"),

	// Class: an environment wrapper must read its parameter unchanged and be
	// reached only by calls the checker sees.
	one("rv1 fail: a wrapper that overwrites its parameter", "func env3(n string) string { n = \"ACME_NEW\"; return os.Getenv(n) }\n\nfunc g() { _ = env3(\"ACME_DEBUG\") }\n", "rule 3|case.go|os.Getenv|not a constant"),
	one("rv1 fail: a wrapper method reached through an interface", "type rd struct{}\n\nfunc (rd) Read(n string) string { return os.Getenv(n) }\n\nfunc g() { var r interface{ Read(string) string } = rd{}; _ = r.Read(\"ACME_NEW\") }\n", "rule 3|case.go|rd.Read|interface"),

	// Class: a Config shape the checker does not read is a finding.
	{name: "rv1 fail: a Config alias of a pointer, with a new field",
		edits: [][3]string{{"internal/config/config.go", "type Config struct {", "type Config = *settings\n\ntype settings struct {\n\tSecret string `toml:\"secret\" json:\"secret\"`"}, {"internal/config/config.go", "(*Config, error)", "(Config, error)"}, {"internal/config/config.go", "c := Config{", "c := settings{"}},
		want:  []string{"rule 3|internal/config/config.go|config:secret"}},
	{name: "rv1 fail: a Config that is not a struct",
		files: map[string]string{"internal/other/other.go": "package other\n\ntype Config map[string]string\n"},
		want:  []string{"rule 3|internal/other/other.go|not a struct"}},
	{name: "rv1 fail: a scalar headers field",
		edits: [][3]string{{"internal/config/config.go", "Headers      map[string]string `toml:\"headers\" json:\"headers\"`", "Headers      string            `toml:\"headers\" json:\"headers\"`"}},
		want:  []string{"rule 3|internal/config/config.go|config:headers"}},

	// Class: a source read proves its path comes from what Files calls.
	{name: "rv1 fail: a source with a Files function that calls nothing",
		edits: [][3]string{{"internal/cli/wiring.go", "return []string{config.Path()}", "return nil"}, {"internal/cli/wiring.go", "\t\"acme-pp-cli/internal/config\"\n", ""}},
		want:  []string{"rule 4|credkit-sites.json|Declarations.Files returns|without Path()"}},
	{name: "rv1 fail: a source read whose path is not the via function's",
		files: map[string]string{caseFile: pkgCli("func g() { _, _ = os.ReadFile(\"token.json\") }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.ReadFile", "count": 1, "kind": "source", "via": "internal/config.Path", "reason": "the token cache"}`},
		want:  []string{"rule 4|case.go|os.ReadFile|is not Path()"}},
	{name: "rv1 fail: a source without via",
		files: map[string]string{caseFile: pkgCli("func g() { _, _ = os.ReadFile(\"token.json\") }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.ReadFile", "count": 1, "kind": "source", "reason": "the token cache"}`},
		want:  []string{"rule 0|credkit-sites.json|needs \"via\"", "rule 4|case.go|os.ReadFile"}},
	{name: "rv1 pass: a source read through a parameter every caller fills from via",
		files: map[string]string{caseFile: "package cli\n\nimport (\n\t\"os\"\n\n\t\"acme-pp-cli/internal/config\"\n)\n\nfunc load(p string) { _, _ = os.ReadFile(p) }\n\nfunc g() { load(config.Path()) }\n"},
		sites: []string{`{"file": "internal/cli/case.go", "func": "load", "what": "os.ReadFile", "count": 1, "kind": "source", "via": "internal/config.Path", "reason": "the config file"}`}},
	{name: "rv1 fail: a source read through a parameter one caller fills otherwise",
		files: map[string]string{caseFile: "package cli\n\nimport (\n\t\"os\"\n\n\t\"acme-pp-cli/internal/config\"\n)\n\nfunc load(p string) { _, _ = os.ReadFile(p) }\n\nfunc g() { load(config.Path()); load(\"token.json\") }\n"},
		sites: []string{`{"file": "internal/cli/case.go", "func": "load", "what": "os.ReadFile", "count": 1, "kind": "source", "via": "internal/config.Path", "reason": "the config file"}`},
		want:  []string{"rule 4|case.go|os.ReadFile|is not Path()"}},

	// Class: a client or transport any other package builds.
	{name: "rv1 fail: a client another package builds",
		files: file(`"example.com/httpx"`, "var c = httpx.NewClient()\n"),
		want:  []string{"rule 2|case.go|example.com/httpx.NewClient|another package"}},
	{name: "rv1 fail: a transport another package builds",
		files: file(`"example.com/httpx"`, "var rt = httpx.NewTransport()\n"),
		want:  []string{"rule 2|case.go|example.com/httpx.NewTransport|another package"}},

	// Class: every released architecture is loaded.
	{name: "rv1 fail: a sink in a file only arm64 compiles", goos: []string{"linux/amd64", "linux/arm64"},
		files: map[string]string{"internal/cli/case_arm64.go": "package cli\n\nimport \"os\"\n\nfunc w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"},
		want:  []string{"rule 2|case_arm64.go|os.WriteFile"}},
	{name: "rv1 pass: that file, checked for amd64 only", goos: []string{"linux/amd64"},
		files: map[string]string{"internal/cli/case_arm64.go": "package cli\n\nimport \"os\"\n\nfunc w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"}},

	// P2: a credential's own Origins entry wins over the "*" default, as in the kit.
	{name: "rv1 fail: an empty own origin is not saved by the default",
		edits: [][3]string{{"internal/cli/wiring.go", `[]credkit.Origin{{Source: "*",`, `[]credkit.Origin{{Source: "env:ACME_TOKEN", Allowed: nil}, {Source: "*",`}},
		want:  []string{"rule 5|wiring.go|env:ACME_TOKEN has no allowed origin"}},
}
