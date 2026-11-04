// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Both directions, rule by rule: testdata/base is a connector wired
// completely (the checker reports nothing), and each case below changes one
// shape. A "fail" case must produce exactly the findings it names, so it
// fails when its rule is removed; its "pass" twin is the same shape routed or
// classified, so it fails when a rule over-reaches.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const caseFile = "internal/cli/case.go"

type tcase struct {
	name  string
	files map[string]string // cli-relative path: content ("" deletes)
	edits [][3]string       // {cli-relative path, old, new}
	sites []string          // ledger entries added to the base ledger (JSON objects)
	goos  []string          // "os/arch" targets; default: the host's
	want  []string          // each "a|b|c": one finding line holds every part; none: no findings
	// strict: every finding must be wanted. Without it a fail case (one with
	// wants) may also carry SSA findings (ssa.go reaches most failing shapes
	// on its own); a pass case never carries any finding.
	strict bool
}

// pkgCli wraps a case body in package cli with the imports cases use.
func pkgCli(body string) string {
	return `package cli

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"acme-pp-cli/internal/credkit"

	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/oauth2"
)

var (
	_ context.Context
	_ sql.Result
	_ fmt.Stringer
	_ net.Addr
	_ = http.StatusOK
	_ = os.ModePerm
	_ *exec.Cmd
	_ = syscall.EINVAL
	_ strings.Builder
	_ credkit.Origin
	_ *server.MCPServer
	_ oauth2.Token
)

` + body
}

func one(name, body string, want ...string) tcase {
	return tcase{name: name, files: map[string]string{caseFile: pkgCli(body)}, want: want}
}

var cases = []tcase{
	// Rule 1: the prologue.
	{name: "r1 pass: the base connector"},
	{name: "r1 fail: main does not start with Start",
		edits: [][3]string{{"cmd/acme-cli/main.go", "\tcredkit.Start(cli.Declarations)\n\tcredkit.Exit(cli.Execute())", "\tcode := cli.Execute()\n\tcredkit.Start(cli.Declarations)\n\tcredkit.Exit(code)"}},
		want:  []string{"rule 1|cmd/acme-cli/main.go|main's first statement", "rule 1|cmd/acme-cli/main.go|other than as the first statement"}},
	{name: "r1 pass: Start through an aliased import",
		edits: [][3]string{{"cmd/acme-cli/main.go", `"acme-pp-cli/internal/credkit"`, `ck "acme-pp-cli/internal/credkit"`}, {"cmd/acme-cli/main.go", "credkit.", "ck."}}},
	{name: "r1 fail: Start through an aliased import, not first",
		edits: [][3]string{{"cmd/acme-cli/main.go", `"acme-pp-cli/internal/credkit"`, `ck "acme-pp-cli/internal/credkit"`},
			{"cmd/acme-cli/main.go", "\tcredkit.Start(cli.Declarations)\n\tcredkit.Exit(cli.Execute())", "\t_ = cli.Execute\n\tck.Start(cli.Declarations)\n\tck.Exit(0)"}},
		want: []string{"rule 1|cmd/acme-cli/main.go|main's first statement", "rule 1|cmd/acme-cli/main.go|other than as the first statement"}},
	{name: "r1 fail: Start given a second Declarations literal",
		edits: [][3]string{{"cmd/acme-cli/main.go", "credkit.Start(cli.Declarations)", "credkit.Start(credkit.Declarations{})"}},
		// With two literals the checker reads neither, so nothing is classified: every field and variable is listed.
		want: []string{"rule 1|cmd/acme-cli/main.go|2 credkit.Declarations literals", "rule 3|is read but not classified", "rule 3|internal/config/config.go|is not classified", "rule 4|credkit-sites.json|Declarations.Files is not"}},
	{name: "r1 fail: the Declarations variable changed outside Start",
		files: map[string]string{caseFile: pkgCli("func init() { Declarations.SecretEnv = nil }\n")},
		want:  []string{"rule 1|internal/cli/case.go|other than as credkit.Start's argument"}},
	{name: "r1 fail: a declaration the checker cannot read",
		edits: [][3]string{{"internal/cli/wiring.go", `SecretEnv:       []string{"ACME_TOKEN"},`, `SecretEnv:       secretNames(),`}},
		files: map[string]string{caseFile: pkgCli("func secretNames() []string { return []string{\"ACME_TOKEN\"} }\n")},
		// ACME_TOKEN is then unclassified too: the checker read no SecretEnv.
		want: []string{"rule 1|internal/cli/wiring.go|cannot read", "rule 3|internal/cli/cli.go|ACME_TOKEN"}},

	// Rule 2: sinks.
	one("r2 fail: os.WriteFile", "func w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n", "rule 2|case.go|os.WriteFile|\"func\": \"w\""),
	one("r2 pass: credkit.WriteFile", "func w() { _ = credkit.WriteFile(\"x\", nil, 0o600) }\n"),
	{name: "r2 fail: os.WriteFile through an aliased import",
		files: map[string]string{caseFile: "package cli\n\nimport o \"os\"\n\nfunc w() { _ = o.WriteFile(\"x\", nil, 0o600) }\n"},
		want:  []string{"rule 2|case.go|os.WriteFile"}},
	{name: "r2 fail: os.WriteFile through a dot import",
		files: map[string]string{caseFile: "package cli\n\nimport . \"os\"\n\nfunc w() { _ = WriteFile(\"x\", nil, 0o600) }\n"},
		want:  []string{"rule 2|case.go|os.WriteFile"}},
	one("r2 fail: os.WriteFile as a method value", "func w() { f := os.WriteFile; _ = f(\"x\", nil, 0o600) }\n", "rule 2|case.go|os.WriteFile"),
	one("r2 fail: a file-writing helper", "func AtomicWritePrivateFile(p string, b []byte) error {\n\tf, err := os.CreateTemp(\"\", \"x\")\n\tif err != nil {\n\t\treturn err\n\t}\n\t_, err = f.Write(b)\n\treturn err\n}\n\nfunc save() { _ = AtomicWritePrivateFile(\"p\", nil) }\n",
		"rule 2|case.go|os.CreateTemp|\"func\": \"AtomicWritePrivateFile\""),
	one("r2 pass: a file-writing helper routed", "func AtomicWritePrivateFile(p string, b []byte) error { return credkit.WriteFile(p, b, 0o600) }\n\nfunc save() { _ = AtomicWritePrivateFile(\"p\", nil) }\n"),
	one("r2 fail: os.OpenFile for writing", "func w() { _, _ = os.OpenFile(\"x\", os.O_CREATE|os.O_WRONLY, 0o600) }\n", "rule 2|case.go|os.OpenFile|opened for writing"),
	one("r2 fail: os.OpenFile with a flag that is not a constant", "func w(fl int) { _, _ = os.OpenFile(\"x\", fl, 0o600) }\n", "rule 2|case.go|os.OpenFile|opened for writing"),
	one("r2 fail: an http.Client without the kit's transport", "var c = &http.Client{}\n", "rule 2|case.go|net/http.Client{}"),
	one("r2 fail: an http.Client with another transport", "var c = &http.Client{Transport: http.DefaultTransport}\n", "rule 2|case.go|net/http.Client{}", "rule 2|case.go|net/http.DefaultTransport"),
	one("r2 pass: an http.Client with the kit's transport", "var c = &http.Client{Transport: credkit.Transport(&http.Transport{})}\n"),
	one("r2 fail: http.Get", "func g() { _, _ = http.Get(\"https://x\") }\n", "rule 2|case.go|net/http.Get"),
	one("r2 fail: http.DefaultClient", "func g() { _, _ = http.DefaultClient.Do(nil) }\n", "rule 2|case.go|net/http.DefaultClient"),
	one("r2 fail: new(http.Client)", "var c = new(http.Client)\n", "rule 2|case.go|net/http.Client|by value", "rule 2|case.go|sink:net/http.Client|new makes a zero"),
	one("r2 fail: an embedded http.Client", "type api struct{ http.Client }\n", "rule 2|case.go|net/http.Client|by value"),
	one("r2 fail: an http.Client zero value", "func g() { var c http.Client; _, _ = c.Do(nil) }\n", "rule 2|case.go|net/http.Client|by value"),
	one("r2 fail: a client's transport replaced", "func g(c *http.Client) { c.Transport = http.DefaultTransport }\n", "rule 2|case.go|net/http.Client.Transport", "rule 2|case.go|net/http.DefaultTransport"),
	one("r2 pass: a client's transport set to the kit's", "func g(c *http.Client) { c.Transport = credkit.Transport(nil) }\n"),
	one("r2 fail: a transport converted outside the kit", "var rt http.RoundTripper = &http.Transport{}\n", "rule 2|case.go|sink:net/http.Transport|becomes a net/http.RoundTripper"),
	one("r2 fail: RoundTrip called directly", "func g(r *http.Request) { _, _ = http.DefaultTransport.RoundTrip(r) }\n", "rule 2|case.go|net/http.RoundTripper.RoundTrip", "rule 2|case.go|net/http.DefaultTransport"),
	one("r2 pass: RoundTrip on the kit's transport", "func g(r *http.Request) { _, _ = credkit.Transport(nil).RoundTrip(r) }\n"),
	one("r2 fail: a client built by another package", "var c = oauth2.NewClient(context.Background(), nil)\n", "rule 2|case.go|golang.org/x/oauth2.NewClient"),
	one("r2 fail: an outbound request the ledger does not list", "func g() { _, _ = http.NewRequest(\"GET\", \"https://x\", nil) }\n", "rule 2|case.go|net/http.NewRequest"),
	{name: "r2 pass: an outbound request the ledger lists",
		files: map[string]string{caseFile: pkgCli("func g() { _, _ = http.NewRequest(\"GET\", \"https://x\", nil) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "net/http.NewRequest", "count": 1, "kind": "exempt", "reason": "sent by the kit-routed client"}`}},
	one("r2 fail: sql.Open with a driver the kit does not wrap", "func g() { _, _ = sql.Open(\"sqlite\", \"x\") }\n", "rule 2|case.go|database/sql.Open"),
	one("r2 pass: sql.Open with the kit-wrapped driver", "func g() { _, _ = sql.Open(\"credkit-sqlite\", \"x\") }\n"),
	one("r2 fail: a driver registered without the kit", "func g() { sql.Register(\"raw\", fakeDriver{}) }\n", "rule 2|case.go|database/sql.Register"),
	one("r2 fail: sql.OpenDB", "func g() { _ = sql.OpenDB(nil) }\n", "rule 2|case.go|database/sql.OpenDB"),
	one("r2 fail: a child process run directly", "func g() { _ = exec.Command(\"true\").Run() }\n", "rule 2|case.go|os/exec.Cmd.Run"),
	one("r2 pass: a child process run through the kit", "func g() { _ = credkit.StartCommand(exec.Command(\"true\")) }\n"),
	one("r2 fail: Run promoted through an embedded field", "type runner struct{ *exec.Cmd }\n\nfunc g(r runner) { _ = r.Run() }\n", "rule 2|case.go|os/exec.Cmd.Run"),
	one("r2 fail: Run as a method value", "func g() { run := exec.Command(\"true\").Output; _, _ = run() }\n", "rule 2|case.go|os/exec.Cmd.Output"),
	one("r2 fail: a command converted to an interface", "func g() { var r interface{ Run() error } = exec.Command(\"true\"); _ = r }\n", "rule 2|case.go|sink:os/exec.Cmd|becomes a interface"),
	one("r2 fail: an embedding type converted to an interface", "type runner struct{ *exec.Cmd }\n\nfunc g(r runner) { var i interface{ CombinedOutput() ([]byte, error) } = r; _ = i }\n", "rule 2|case.go|convert:os/exec.Cmd.CombinedOutput"),
	one("r2 fail: a type assertion to a Run interface", "func g(x any) { _ = x.(interface{ Run() error }) }\n", "rule 2|case.go|assert:Run", "rule 2|case.go|assert:os/exec.Cmd"),
	one("r2 fail: ServeStdio", "func g(s *server.MCPServer) { _ = server.ServeStdio(s) }\n", "rule 2|case.go|server.ServeStdio"),
	one("r2 fail: an MCP server without the kit's middleware", "var s = server.NewMCPServer(\"a\", \"1\")\n", "rule 2|case.go|server.NewMCPServer"),
	one("r2 fail: stdio listening on unchecked streams", "func g(s *server.MCPServer) { _ = server.NewStdioServer(s).Listen(context.Background(), os.Stdin, os.Stdout) }\n", "rule 2|case.go|server.StdioServer.Listen"),
	one("r2 fail: a streamable HTTP server started directly", "func g(s *server.MCPServer) { _ = server.NewStreamableHTTPServer(s).Start(\":0\") }\n", "rule 2|case.go|server.StreamableHTTPServer.Start"),
	one("r2 fail: a streamable HTTP server served outside MCPHTTP", "func auth(h http.Handler) http.Handler { return h }\n\nfunc g(s *server.MCPServer) http.Handler { return auth(server.NewStreamableHTTPServer(s)) }\n", "rule 2|case.go|convert:github.com/mark3labs/mcp-go/server.StreamableHTTPServer.ServeHTTP"),
	one("r2 pass: a streamable HTTP server inside MCPHTTP, then a bearer wrapper", "func auth(h http.Handler) http.Handler { return h }\n\nfunc g(s *server.MCPServer) http.Handler { return auth(credkit.MCPHTTP(server.NewStreamableHTTPServer(s))) }\n"),
	one("r2 fail: output in init, before Start", "func init() { fmt.Println(\"hi\") }\n", "rule 2|case.go|fmt.Println|before credkit.Start"),
	one("r2 fail: output in a function init calls", "func init() { banner() }\n\nfunc banner() { fmt.Fprintln(os.Stderr, \"hi\") }\n", "rule 2|case.go|fmt.Fprintln|\"func\": \"banner\""),
	one("r2 fail: output in a package-level initializer", "var _ = func() int { println(\"hi\"); return 0 }()\n", "rule 2|case.go|builtin.println"),
	one("r2 pass: output after Start, and a stream captured before it", "var out = os.Stdout\n\nfunc later() { fmt.Fprintln(out, \"hi\") }\n"),
	one("r2 fail: output in init through a function value", "func init() { p := fmt.Println; _, _ = p(\"hi\") }\n", "rule 2|case.go|fmt.Println|before credkit.Start"),
	one("r2 fail: output in init through an io.Writer", "var out = os.Stdout\n\nfunc init() { var w interface{ Write([]byte) (int, error) } = out; _, _ = w.Write(nil) }\n", "rule 2|case.go|Write|before credkit.Start"),
	one("r2 pass: an in-memory buffer written in init", "var banner string\n\nfunc init() { var b strings.Builder; b.WriteString(\"hi\"); b.WriteByte('!'); banner = b.String() }\n"),
	one("r2 fail: a panic in init", "func init() { if len(os.Args) > 9 { panic(\"x\") } }\n", "rule 2|case.go|builtin.panic|before credkit.Start"),
	one("r2 fail: a raw network connection", "func g() { _, _ = net.Dial(\"tcp\", \"x:443\") }\n", "rule 2|case.go|net.Dial|raw network"),
	one("r2 fail: a SQL driver opened directly", "func g() { _, _ = fakeDriver{}.Open(\"x.db\") }\n", "rule 2|case.go|fakeDriver.Open|opened directly"),
	one("r2 fail: a client another package holds", "func g(c *oauth2.Holder) { _, _ = c.HTTPClient.Do(nil) }\n", "rule 2|case.go|golang.org/x/oauth2.HTTPClient|another package"),
	{name: "r4 fail: a source with no Files function",
		edits: [][3]string{{"internal/cli/wiring.go", "\tFiles:           func(args []string) []string { return []string{config.Path()} },\n", ""}, {"internal/cli/wiring.go", "\t\"acme-pp-cli/internal/config\"\n", ""}},
		want:  []string{"rule 1|wiring.go|no Files function", "rule 4|credkit-sites.json|Declarations.Files is not"}},
	one("r2 fail: a system call", "func g() { _, _ = syscall.Getenv(\"X\") }\n", "rule 2|case.go|syscall.Getenv"),
	{name: "r2 fail: a sink in a file only the windows build compiles", goos: []string{"linux/amd64", "windows/amd64"},
		files: map[string]string{"internal/cli/case_windows.go": "package cli\n\nimport \"os\"\n\nfunc w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"},
		want:  []string{"rule 2|case_windows.go|os.WriteFile"}},
	{name: "r2 pass: that file, checked for linux only", goos: []string{"linux/amd64"},
		files: map[string]string{"internal/cli/case_windows.go": "package cli\n\nimport \"os\"\n\nfunc w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"}},

	// Rule 3: config fields and environment variables.
	one("r3 fail: an unclassified variable", "func g() { _ = os.Getenv(\"ACME_REGION\") }\n", "rule 3|case.go|ACME_REGION is read but not classified"),
	one("r3 pass: a classified variable", "func g() { _ = os.Getenv(\"ACME_DEBUG\") }\n"),
	one("r3 fail: a variable read through a wrapper", "func g() { _ = env(\"ACME_REGION\") }\n", "rule 3|case.go|ACME_REGION is read but not classified"),
	one("r3 fail: a variable read through a wrapper of a wrapper", "func env2(n string) string { return env(n) }\n\nfunc g() { _ = env2(\"ACME_REGION\") }\n", "rule 3|case.go|ACME_REGION is read but not classified"),
	one("r3 fail: a wrapper used as a value", "func g() { f := env; _ = f(\"ACME_DEBUG\") }\n", "rule 3|case.go|acme-pp-cli/internal/cli.env|as a value"),
	one("r3 fail: a name that is not a constant", "func g(s string) { _ = os.Getenv(\"ACME_\" + s) }\n", "rule 3|case.go|os.Getenv|not a constant"),
	{name: "r3 pass: a computed name the ledger lists, classified",
		files: map[string]string{caseFile: pkgCli("func g(s string) { _ = os.Getenv(\"ACME_\" + s) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.Getenv", "count": 1, "kind": "exempt", "names": ["ACME_DEBUG"], "reason": "s is DEBUG only"}`}},
	{name: "r3 fail: a computed name the ledger lists, unclassified",
		files: map[string]string{caseFile: pkgCli("func g(s string) { _ = os.Getenv(\"ACME_\" + s) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.Getenv", "count": 1, "kind": "exempt", "names": ["ACME_REGION"], "reason": "s is REGION only"}`},
		want:  []string{"rule 3|credkit-sites.json|ACME_REGION"}},
	{name: "r3 fail: a computed name the ledger lists without names",
		files: map[string]string{caseFile: pkgCli("func g(s string) { _ = os.Getenv(\"ACME_\" + s) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.Getenv", "count": 1, "kind": "exempt", "reason": "no names"}`},
		want:  []string{"rule 0|credkit-sites.json|needs \"names\"", "rule 3|case.go|os.Getenv|not a constant"}},
	one("r3 fail: os.Environ", "func g() { _ = os.Environ() }\n", "rule 3|case.go|os.Environ"),
	one("r3 fail: os.ExpandEnv of a constant", "func g() { _ = os.ExpandEnv(\"$ACME_REGION/x\") }\n", "rule 3|case.go|ACME_REGION is read"),
	one("r3 fail: an env struct tag", "type opts struct {\n\tRegion string `env:\"ACME_REGION\"`\n}\n", "rule 3|case.go|ACME_REGION is read"),
	{name: "r3 fail: a new Config field",
		edits: [][3]string{{"internal/config/config.go", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n\tSecret       string            `toml:\"secret\" json:\"secret\"`"}},
		want:  []string{"rule 3|internal/config/config.go|config:secret"}},
	{name: "r3 fail: a new field in an embedded struct",
		edits: [][3]string{{"internal/config/config.go", "\tRefreshToken string `toml:\"refresh_token\" json:\"refresh_token\"`", "\tRefreshToken string `toml:\"refresh_token\" json:\"refresh_token\"`\n\tSessionKey   []byte `json:\"session_key\"`"}},
		want:  []string{"rule 3|internal/config/config.go|config:session_key"}},
	// Numbers count (contract section 1): this was a pass case while only
	// string-bearing fields were classified (Codex verifyl4 round 1).
	{name: "r3 fail: a new Config field that holds numbers",
		edits: [][3]string{{"internal/config/config.go", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n\tRetries      []int             `toml:\"retries\" json:\"retries\"`"}},
		want:  []string{"rule 3|config.go|config:retries"}},
	{name: "r3 pass: a new Config field that holds only bools",
		edits: [][3]string{{"internal/config/config.go", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n\tFlags        []bool            `toml:\"flags\" json:\"flags\"`"}}},
	{name: "r3 fail: a key declared both credential and identifier",
		edits: [][3]string{{"internal/cli/wiring.go", `{Source: "config:profile", Reason: "a profile name"},`, `{Source: "config:profile", Reason: "a profile name"}, {Source: "config:token", Reason: "x"},`}},
		want:  []string{"rule 1|wiring.go|config:token is declared both"}},
	// The type a source read decodes into is classified whatever its name.
	{name: "r3 pass: the decoded config type need not be named Config",
		edits: [][3]string{{"internal/config/config.go", "type Config struct", "type Settings struct"}, {"internal/config/config.go", "*Config, error", "*Settings, error"}, {"internal/config/config.go", "c := Config{", "c := Settings{"}}},
	{name: "r3 fail: a new field in the decoded type, whatever its name",
		edits: [][3]string{{"internal/config/config.go", "type Config struct {", "type Settings struct {\n\tNewKey string `toml:\"new_key\" json:\"new_key\"`\n}\n\ntype unused struct {"}, {"internal/config/config.go", "*Config, error", "*Settings, error"}, {"internal/config/config.go", "c := Config{Path: Path()}", "c := Settings{}"}},
		want:  []string{"rule 3|config.go|new_key"}},
	{name: "r3 fail: no Config type and nothing decoded",
		edits: [][3]string{{"internal/config/config.go", "type Config struct", "type Settings struct"}, {"internal/config/config.go", "*Config, error", "*Settings, error"}, {"internal/config/config.go", "c := Config{", "c := Settings{"}},
		files: map[string]string{"../credkit-sites.json": ""},
		want:  []string{"rule 3|Config|no Config type", "rule 4|config.go|os.ReadFile", "rule 2|cli.go|net/http.NewRequest", "rule 4|cli.go|os.OpenFile"}},

	// Rule 4: reads.
	one("r4 fail: a file read the ledger does not list", "func g() { _, _ = os.ReadFile(\"token.json\") }\n", "rule 4|case.go|os.ReadFile"),
	one("r4 fail: a read-only os.OpenFile the ledger does not list", "func g() { _, _ = os.OpenFile(\"t\", os.O_RDONLY, 0) }\n", "rule 4|case.go|os.OpenFile"),
	{name: "r4 fail: a second read in a listed function",
		files: map[string]string{caseFile: pkgCli("func g() { _, _ = os.ReadFile(\"a\"); _, _ = os.ReadFile(\"b\") }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.ReadFile", "count": 1, "kind": "source", "reason": "the token cache"}`},
		want:  []string{"rule 0|credkit-sites.json|count is 1 but the function has 2", "rule 4|case.go|os.ReadFile"}},
	{name: "r4 fail: a stale ledger entry",
		sites: []string{`{"file": "internal/cli/case.go", "func": "g", "what": "os.ReadFile", "count": 1, "kind": "source", "reason": "gone"}`},
		want:  []string{"rule 0|credkit-sites.json|stale"}},
	{name: "r4 fail: kind source on a write",
		files: map[string]string{caseFile: pkgCli("func w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "w", "what": "os.WriteFile", "count": 1, "kind": "source", "reason": "no"}`},
		want:  []string{"rule 0|credkit-sites.json|for a file read only", "rule 2|case.go|os.WriteFile"}},
	{name: "r4 fail: a ledger entry without a reason",
		files: map[string]string{caseFile: pkgCli("func w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n")},
		sites: []string{`{"file": "internal/cli/case.go", "func": "w", "what": "os.WriteFile", "count": 1, "kind": "exempt", "reason": " "}`},
		want:  []string{"rule 0|credkit-sites.json|no reason", "rule 2|case.go|os.WriteFile"}},

	// Rule 5: origins.
	{name: "r5 fail: a credential with no allowed origin",
		edits: [][3]string{{"internal/cli/wiring.go", `{{Source: "*", Allowed: []string{"base_url", "https://auth.example.com"}}}`, `{{Source: "env:ACME_TOKEN", Allowed: []string{"base_url"}}, {Source: "config:token", Allowed: []string{"base_url"}}, {Source: "config:client_secret", Allowed: []string{"https://auth.example.com"}}, {Source: "config:refresh_token", Allowed: []string{"https://auth.example.com"}}}`}},
		want:  []string{"rule 5|wiring.go|flag:--token has no allowed origin"}},
	{name: "r5 pass: every credential with its own origin",
		edits: [][3]string{{"internal/cli/wiring.go", `{{Source: "*", Allowed: []string{"base_url", "https://auth.example.com"}}}`, `{{Source: "env:ACME_TOKEN", Allowed: []string{"base_url"}}, {Source: "config:token", Allowed: []string{"base_url"}}, {Source: "config:client_secret", Allowed: []string{"https://auth.example.com"}}, {Source: "config:refresh_token", Allowed: []string{"https://auth.example.com"}}, {Source: "flag:--token", Allowed: []string{"base_url"}}}`}}},
	{name: "r5 fail: an origin that is not scheme://host",
		edits: [][3]string{{"internal/cli/wiring.go", `"https://auth.example.com"}}}`, `"auth.example.com"}}}`}},
		want: []string{"rule 1|wiring.go|is not scheme://host", "rule 5|wiring.go|env:ACME_TOKEN has no allowed origin", "rule 5|wiring.go|config:token has no", "rule 5|wiring.go|config:client_secret has no",
			"rule 5|wiring.go|config:refresh_token has no", "rule 5|wiring.go|flag:--token has no"}},
}

func TestRules(t *testing.T) {
	base, err := filepath.Abs("testdata/base")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			copyTree(t, base, dir)
			cli := filepath.Join(dir, "cli")
			for p, body := range c.files {
				if body == "" {
					os.Remove(filepath.Join(cli, p))
					continue
				}
				must(t, os.MkdirAll(filepath.Dir(filepath.Join(cli, p)), 0o755))
				must(t, os.WriteFile(filepath.Join(cli, p), []byte(body), 0o644))
			}
			for _, e := range c.edits {
				p := filepath.Join(cli, e[0])
				b, err := os.ReadFile(p)
				must(t, err)
				if !bytes.Contains(b, []byte(e[1])) {
					t.Fatalf("edit of %s: %q not found", e[0], e[1])
				}
				must(t, os.WriteFile(p, bytes.ReplaceAll(b, []byte(e[1]), []byte(e[2])), 0o644))
			}
			if len(c.sites) > 0 {
				lp := filepath.Join(dir, ledgerName)
				var l map[string][]json.RawMessage
				b, err := os.ReadFile(lp)
				must(t, err)
				must(t, json.Unmarshal(b, &l))
				for _, s := range c.sites {
					l["sites"] = append(l["sites"], json.RawMessage(s))
				}
				b, err = json.Marshal(l)
				must(t, err)
				must(t, os.WriteFile(lp, b, 0o644))
			}
			goos := c.goos
			if goos == nil {
				goos = []string{runtime.GOOS + "/" + runtime.GOARCH}
			}
			fs, err := verify(cli, goos)
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, f := range fs {
				if f.SSA && len(c.want) > 0 && !c.strict && !wanted(f.String(), c.want) {
					continue
				}
				lines = append(lines, f.String())
			}
			matches := func(line, want string) bool {
				for _, part := range strings.Split(want, "|") {
					if !strings.Contains(line, part) {
						return false
					}
				}
				return true
			}
			for _, w := range c.want {
				found := false
				for _, l := range lines {
					found = found || matches(l, w)
				}
				if !found {
					t.Errorf("want a finding with %q; got:\n%s", w, strings.Join(lines, "\n"))
				}
				if found && !c.strict {
					// A case written for the syntax-tree pass: that pass alone
					// must still report it, or removing one of its rules hides
					// behind the SSA pass's finding at the same site.
					ast := false
					for _, f := range fs {
						ast = ast || (!f.SSA && matches(f.String(), w))
					}
					if !ast {
						t.Errorf("want a syntax-tree finding with %q; only the SSA pass reports it", w)
					}
				}
			}
			for _, l := range lines {
				ok := false
				for _, w := range c.want {
					ok = ok || matches(l, w)
				}
				if !ok {
					t.Errorf("unexpected finding:\n%s", l)
				}
			}
		})
	}
}

// TestSkipsUnadopted: a connector without internal/credkit/ is skipped, exit 0.
func wanted(line string, want []string) bool {
	for _, w := range want {
		ok := true
		for _, part := range strings.Split(w, "|") {
			ok = ok && strings.Contains(line, part)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestSkipsUnadopted(t *testing.T) {
	dir := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(dir, "cli"), 0o755))
	var out bytes.Buffer
	if code := run(filepath.Join(dir, "cli"), []string{runtime.GOOS + "/" + runtime.GOARCH}, &out); code != 0 || !strings.Contains(out.String(), "skipped") {
		t.Fatalf("code %d, output %q", code, out.String())
	}
}

// TestExitCodes: findings exit 1, a clean connector exits 0.
func TestExitCodes(t *testing.T) {
	base, _ := filepath.Abs("testdata/base")
	dir := t.TempDir()
	copyTree(t, base, dir)
	var out bytes.Buffer
	if code := run(filepath.Join(dir, "cli"), []string{runtime.GOOS + "/" + runtime.GOARCH}, &out); code != 0 {
		t.Fatalf("clean: code %d\n%s", code, out.String())
	}
	must(t, os.WriteFile(filepath.Join(dir, "cli", caseFile), []byte(pkgCli("func w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n")), 0o644))
	out.Reset()
	if code := run(filepath.Join(dir, "cli"), []string{runtime.GOOS + "/" + runtime.GOARCH}, &out); code != 1 || !strings.Contains(out.String(), "1 unclassified site") {
		t.Fatalf("finding: code %d\n%s", code, out.String())
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	must(t, err)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
