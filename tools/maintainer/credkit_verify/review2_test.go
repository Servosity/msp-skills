// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 2 (verify-codex-r2): each input as Codex gave it, written
// before the allow-list rules (loop ruling 12) and failing without them.

import (
	"strings"
	"testing"
)

func init() { cases = append(cases, review2...) }

// cfgFile replaces the base config package.
func cfgFile(body string) map[string]string {
	return map[string]string{"internal/config/config.go": "package config\n\n" + body}
}

const baseConfigType = "type Auth struct {\n\tRefreshToken string `toml:\"refresh_token\" json:\"refresh_token\"`\n}\n\ntype Config struct {\n\tBaseURL      string            `toml:\"base_url\" json:\"base_url\"`\n\tToken        string            `toml:\"token\" json:\"token\"`\n\tClientSecret string            `toml:\"client_secret\" json:\"client_secret\"`\n\tProfile      string            `toml:\"profile\" json:\"profile\"`\n\tHeaders      map[string]string `toml:\"headers\" json:\"headers\"`\n\tPath         string            `toml:\"-\" json:\"-\"`\n\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n\tAuth\n}\n"

var review2 = []tcase{
	// Class source-via-proof: Files must return via's path on every return,
	// and via's path cannot depend on what a caller passes.
	{name: "rv2 fail: Files calls via without returning its path",
		edits: [][3]string{{"internal/cli/wiring.go", "func(args []string) []string { return []string{config.Path()} }", "func(args []string) []string { _ = config.Path(); return nil }"}},
		want:  []string{"rule 4|credkit-sites.json|Files"}},
	{name: "rv2 fail: Files returns via's path only on an unreachable path",
		edits: [][3]string{{"internal/cli/wiring.go", "func(args []string) []string { return []string{config.Path()} }", "func(args []string) []string {\n\t\tif false {\n\t\t\treturn []string{config.Path()}\n\t\t}\n\t\treturn nil\n\t}"}},
		want:  []string{"rule 4|credkit-sites.json|Files"}},
	{name: "rv2 fail: via takes the file name from its caller",
		edits: [][3]string{{"internal/cli/wiring.go", "[]string{config.Path()}", "[]string{config.Path(\"startup.toml\")}"}},
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nfunc Path(name string) string { return name }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(\"token-cache.json\"))\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|credkit-sites.json|internal/config.Path"}},
	{name: "rv2 fail: via returns a different path on each call",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nvar n string\n\nfunc Path() string { n += \"x\"; return n }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|config.go|internal/config.Path"}},

	// Class config-type-discovery: the type a source read decodes into.
	{name: "rv2 fail: a function-local Config is what the file decodes into",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\nfunc Path() string { return filepath.Join(os.TempDir(), \"acme.toml\") }\n\nfunc Load() (any, error) {\n\ttype Config struct {\n\t\tNewSecret string `json:\"new_secret\"`\n\t}\n\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 3|config.go|new_secret"}},
	{name: "rv2 fail: a source read's bytes go somewhere the checker cannot follow",
		files: cfgFile("import (\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\nfunc Path() string { return filepath.Join(os.TempDir(), \"acme.toml\") }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\treturn &Config{Path: string(b)}, err\n}\n"),
		want:  []string{"rule 3|config.go|cannot follow"}},

	// Class tuple-conversions.
	one("rv2 fail: a command reaches an interface through a tuple assignment",
		"func makeCmd() (*exec.Cmd, error) { return exec.Command(\"echo\", \"secret\"), nil }\n\nfunc leak() {\n\tvar r interface{ Run() error }\n\tr, _ = makeCmd()\n\t_ = r.Run()\n}\n",
		"rule 2|case.go|os/exec.Cmd"),
	one("rv2 fail: a command reaches an interface through tuple arguments",
		"func makeCmd() (*exec.Cmd, error) { return exec.Command(\"echo\", \"secret\"), nil }\n\nfunc run(r interface{ Run() error }, err error) { _ = r.Run() }\n\nfunc leak() { run(makeCmd()) }\n",
		"rule 2|case.go|os/exec.Cmd"),
	one("rv2 fail: an environment wrapper reaches an interface through a tuple",
		"type reader struct{}\n\nfunc (reader) Read(n string) string { return os.Getenv(n) }\n\nfunc mk() (reader, error) { return reader{}, nil }\n\nfunc leak() {\n\tvar r interface{ Read(string) string }\n\tr, _ = mk()\n\t_ = r.Read(\"ACME_NEW_SECRET\")\n}\n",
		"rule 3|case.go|wrapper"),

	// Class generic-bodies: no generic is instantiated with a sink type.
	{name: "rv2 fail: a generic assertion recovers a writable stdin",
		files: file(`"io"
	"os"`, "func cast[T any](x any) T { return x.(T) }\n\nfunc leak() {\n\tvar r io.Reader = os.Stdin\n\tw := cast[*os.File](r)\n\t_, _ = w.WriteString(\"secret\")\n}\n"),
		want: []string{"rule 2|case.go|generic"}},
	one("rv2 fail: a generic assertion recovers a command",
		"func cast[T any](x any) T { return x.(T) }\n\nfunc leak() {\n\tvar x any = exec.Command(\"echo\", \"secret\")\n\tr := cast[interface{ Run() error }](x)\n\t_ = r.Run()\n}\n",
		"rule 2|case.go|os/exec.Cmd"),
	one("rv2 fail: a generic zeroes a client",
		"func reset[T any](p *T) {\n\tvar zero T\n\t*p = zero\n}\n\nfunc leak() {\n\tc := &http.Client{Transport: credkit.Transport(nil)}\n\treset(c)\n\t_, _ = c.Get(\"https://outside.example\")\n}\n",
		"rule 2|case.go|generic:net/http.Client", "rule 2|case.go|sink:net/http.Client|becomes a *T"),
	one("rv2 fail: a generic allocates a zero client",
		"func fresh[P interface{ *E }, E any]() P { return P(new(E)) }\n\nfunc leak() {\n\tc := fresh[*http.Client]()\n\t_, _ = c.Get(\"https://outside.example\")\n}\n",
		"rule 2|case.go|generic:net/http.Client", "rule 2|case.go|sink:net/http.Client|type parameter of fresh"),

	// Class client-construction.
	{name: "rv2 fail: a pointer conversion builds a client",
		files: file(`"net/http"
	"time"`, "func leak() {\n\tc := (*http.Client)(&struct {\n\t\tTransport     http.RoundTripper\n\t\tCheckRedirect func(*http.Request, []*http.Request) error\n\t\tJar           http.CookieJar\n\t\tTimeout       time.Duration\n\t}{})\n\t_, _ = c.Get(\"https://outside.example\")\n}\n"),
		want: []string{"rule 2|case.go|net/http.Client"}},

	// Class sql-registration-execution.
	one("rv2 fail: a registration an early return can skip",
		"func init() {\n\tif len(os.Args) > 1 {\n\t\treturn\n\t}\n\tsql.Register(\"credkit-sqlite2\", credkit.SQLDriver(fakeDriver{}))\n}\n\nfunc leak() { _, _ = sql.Open(\"credkit-sqlite2\", \"out.db\") }\n",
		"rule 2|case.go|database/sql.Register|not among the leading", "rule 2|case.go|database/sql.Open|does not wrap"),
	one("rv2 fail: a database opened in a package initializer",
		"var db, _ = sql.Open(\"credkit-sqlite\", \"out.db\")\n",
		"rule 2|case.go|database/sql.Open|before credkit.Start"),

	// Class premain-output.
	{name: "rv2 fail: a slog handler's Handle writes before Start",
		files: file(`"context"
	"log/slog"
	"os"
	"time"`, "func init() {\n\th := slog.NewTextHandler(os.Stdout, nil)\n\trecord := slog.NewRecord(time.Time{}, slog.LevelInfo, os.Getenv(\"ACME_TOKEN\"), 0)\n\t_ = h.Handle(context.Background(), record)\n}\n"),
		want: []string{"rule 2|case.go|before credkit.Start"}},
	{name: "rv2 fail: a crypto stream writer on stdout before Start",
		files: file(`"crypto/cipher"
	"os"`, "type plain struct{}\n\nfunc (plain) XORKeyStream(dst, src []byte) { copy(dst, src) }\n\nvar out = cipher.StreamWriter{S: plain{}, W: os.Stdout}\n\nfunc init() { _, _ = out.Write([]byte(os.Getenv(\"ACME_TOKEN\"))) }\n"),
		want: []string{"rule 2|case.go|before credkit.Start"}},

	// Class fixture-default-targets: the default list, as CI runs it.
	{name: "rv2 fail: a sink only arm64 compiles, under the default targets", goos: defaultTargetList(),
		files: map[string]string{"internal/cli/case_arm64.go": "package cli\n\nimport \"os\"\n\nfunc w() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"},
		want:  []string{"rule 2|case_arm64.go|os.WriteFile"}},

	// P2: a Config type that points at itself.
	{name: "rv2 fail: a Config pointer cycle is reported, not looped",
		files: file(`"os"`, "type Config *Config\n\nvar _ = os.Args\n"),
		want:  []string{"rule 3|case.go|not a struct"}},
}

// TestDefaultTargets: the default list is every pair the release builds, so
// dropping one fails here (and the arm64 case above runs under this list).
func TestDefaultTargets(t *testing.T) {
	want := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"}
	if got := defaultTargetList(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("default targets %v, want %v", got, want)
	}
}

// The allow-lists' pass twins: the ordinary shapes correct wiring takes, and
// one fail case per rule the allow-lists added.
var ledgerWith = func(what string) string {
	return `{"sites": [
    {"file": "internal/config/config.go", "func": "Load", "what": "` + what + `", "count": 1, "kind": "source", "via": "internal/config.Path", "reason": "the config file"},
    {"file": "internal/cli/cli.go", "func": "Execute", "what": "net/http.NewRequest", "count": 1, "kind": "exempt", "reason": "sent by the kit's client"},
    {"file": "internal/cli/cli.go", "func": "Execute", "what": "os.OpenFile", "count": 1, "kind": "exempt", "reason": "the request body file"}]}`
}

const pathFn = "func Path() string { return filepath.Join(os.TempDir(), \"acme.toml\") }\n"

var review2b = []tcase{
	one("rv2 pass: a command built by a helper, kept in a struct and a slice, run through the kit",
		"type job struct{ cmd *exec.Cmd }\n\nfunc build() (*exec.Cmd, error) { return exec.CommandContext(context.Background(), \"true\"), nil }\n\nfunc g() error {\n\tc, err := build()\n\tif err != nil || c == nil {\n\t\treturn err\n\t}\n\tj := job{cmd: c}\n\tcmds := []*exec.Cmd{j.cmd}\n\tcmds = append(cmds, c)\n\tfor _, x := range cmds {\n\t\tx.Env = []string{}\n\t\tout, _ := x.StdoutPipe()\n\t\t_ = out\n\t\tif err := credkit.StartCommand(x); err != nil {\n\t\t\treturn err\n\t\t}\n\t\t_ = x.Wait()\n\t}\n\treturn nil\n}\n"),
	one("rv2 pass: a client built once and handed on as its own type",
		"type api struct{ hc *http.Client }\n\nfunc newClient() *http.Client {\n\treturn &http.Client{Transport: credkit.Transport(http.DefaultTransport.(*http.Transport).Clone())}\n}\n\nfunc newAPI(hc *http.Client) *api { return &api{hc: hc} }\n\nfunc g() {\n\ta := newAPI(newClient())\n\t_, _ = a.hc.Get(\"https://x\")\n\tclients := map[string]*http.Client{\"a\": a.hc}\n\tif clients[\"a\"] != a.hc {\n\t\ta.hc.CloseIdleConnections()\n\t}\n}\n"),
	one("rv2 pass: a transport recovered from a client, cloned and wrapped",
		"func tune(base *http.Client) *http.Client {\n\tc := *base\n\tif t, ok := base.Transport.(*http.Transport); ok {\n\t\ttr := t.Clone()\n\t\ttr.MaxIdleConns = 4\n\t\tc.Transport = credkit.Transport(tr)\n\t}\n\treturn &c\n}\n"),
	one("rv2 fail: a recovered transport used to send", "func g(base *http.Client, r *http.Request) {\n\tif t, ok := base.Transport.(*http.Transport); ok {\n\t\t_, _ = t.RoundTrip(r)\n\t}\n}\n", "rule 2|case.go|net/http.Transport.RoundTrip"),
	{name: "rv2 fail: a print method of a type that is not a writer, before Start",
		files: file(`"example.com/httpx"`, "func init() { httpx.Printer{}.Println(\"x\") }\n"),
		want:  []string{"rule 2|case.go|example.com/httpx.Printer.Println|before credkit.Start"}},
	one("rv2 fail: a Config alias of a pointer that nothing decodes",
		"type Config = *settingsX\n\ntype settingsX struct{ Secret string }\n", "rule 3|case.go|config:Secret"),
	one("rv2 pass: generics over ordinary types",
		"func first[T any](xs []T) T { return xs[0] }\n\nfunc g() string { return first([]string{\"a\"}) + strings.ToUpper(first[string]([]string{\"b\"})) }\n"),
	one("rv2 pass: two leading kit registrations",
		"func init() {\n\tsql.Register(\"credkit-a\", credkit.SQLDriver(fakeDriver{}))\n\tsql.Register(\"credkit-b\", credkit.SQLDriver(fakeDriver{}))\n}\n\nfunc g() { _, _ = sql.Open(\"credkit-b\", \"x\") }\n"),
	one("rv2 fail: an exec.Cmd literal", "func g() { _ = credkit.RunCommand(&exec.Cmd{Path: \"/bin/true\"}) }\n", "rule 2|case.go|sink:os/exec.Cmd|literal", "rule 2|case.go|os/exec.Cmd|held by value"),
	one("rv2 fail: a type switch recovers a command",
		"func g(x any) {\n\tswitch c := x.(type) {\n\tcase *exec.Cmd:\n\t\t_ = credkit.RunCommand(c)\n\t}\n}\n", "rule 2|case.go|sink:os/exec.Cmd|type switch"),
	one("rv2 fail: a command handed to a method through an interface",
		"type user interface{ Use(*exec.Cmd) }\n\nfunc g(u user) { u.Use(exec.Command(\"true\")) }\n", "rule 2|case.go|sink:os/exec.Cmd|through an interface"),
	one("rv2 fail: a command handed to another package", "func g() { fmt.Println(exec.Command(\"true\")) }\n", "rule 2|case.go|sink:os/exec.Cmd|fmt.Println"),
	one("rv2 fail: a transport's RoundTrip on a value", "func g(r *http.Request) { t := &http.Transport{}; _, _ = t.RoundTrip(r) }\n", "rule 2|case.go|net/http.Transport.RoundTrip"),
	{name: "rv2 fail: a hash written before Start",
		files: file(`"crypto/sha256"`, "var h = sha256.New()\n\nfunc init() { _, _ = h.Write([]byte(\"x\")) }\n"),
		want:  []string{"rule 2|case.go|before credkit.Start"}},
	one("rv2 fail: a request sent before Start",
		"var c = &http.Client{Transport: credkit.Transport(nil)}\n\nfunc init() { _, _ = c.Get(\"https://x\") }\n", "rule 2|case.go|net/http.Client.Get|before credkit.Start"),

	// Sources: the command-line form of via, decoders, and what breaks the proof.
	{name: "rv2 pass: via reads the command line, Files and the read pass the same one",
		edits: [][3]string{{"internal/cli/wiring.go", "[]string{config.Path()}", "[]string{config.Path(args)}"}},
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"strings\"\n)\n\n" + baseConfigType + "\nfunc Path(args []string) string {\n\tfor i, a := range args {\n\t\tif a == \"--config\" && i+1 < len(args) {\n\t\t\treturn args[i+1]\n\t\t}\n\t\tif v, ok := strings.CutPrefix(a, \"--config=\"); ok {\n\t\t\treturn v\n\t\t}\n\t}\n\treturn filepath.Join(os.TempDir(), \"acme.toml\")\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(os.Args[1:]))\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n")},
	{name: "rv2 fail: os.Args changed while via reads the command line",
		edits: [][3]string{{"internal/cli/wiring.go", "[]string{config.Path()}", "[]string{config.Path(args)}"}},
		files: map[string]string{
			"internal/config/config.go": "package config\n\nimport (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nfunc Path(args []string) string {\n\tif len(args) > 0 {\n\t\treturn args[0]\n\t}\n\treturn \"acme.toml\"\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(os.Args[1:]))\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n",
			caseFile:                    file(`"os"`, "func g() { os.Args = append(os.Args, \"other.toml\") }\n")[caseFile]},
		want: []string{"rule 4|case.go|os.Args|can change"}},
	{name: "rv2 fail: via changes the command line it reads",
		edits: [][3]string{{"internal/cli/wiring.go", "[]string{config.Path()}", "[]string{config.Path(args)}"}},
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nfunc Path(args []string) string {\n\targs[0] = \"other.toml\"\n\treturn args[0]\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(os.Args[1:]))\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|config.go|via|not a pure function"}},
	{name: "rv2 fail: via calls something that is not a pure path function",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\nfunc Path() string { d, _ := os.Getwd(); return filepath.Join(d, \"acme.toml\") }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|config.go|via|os.Getwd"}},
	one("rv2 fail: the environment changed while via reads it", "func g() { _ = os.Setenv(\"TMPDIR\", \"/elsewhere\") }\n", "rule 4|case.go|os.Setenv|environment write"),
	{name: "rv2 fail: Files reads package state",
		edits: [][3]string{{"internal/cli/wiring.go", "func(args []string) []string { return []string{config.Path()} }", "func(args []string) []string { return []string{config.Path(), legacy} }"}},
		files: file(`"os"`, "var legacy = os.Args[0]\n"),
		want:  []string{"rule 4|wiring.go|Files|package-level variable"}},
	{name: "rv2 pass: a source read through a decoder",
		files: map[string]string{
			"../credkit-sites.json":     ledgerWith("os.Open"),
			"internal/config/config.go": "package config\n\nimport (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\nfunc Load() (*Config, error) {\n\tf, err := os.Open(Path())\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tdefer f.Close()\n\tvar c Config\n\terr = json.NewDecoder(f).Decode(&c)\n\treturn &c, err\n}\n"}},
	{name: "rv2 pass: a source read through io.ReadAll",
		files: map[string]string{
			"../credkit-sites.json":     ledgerWith("os.Open"),
			"internal/config/config.go": "package config\n\nimport (\n\t\"encoding/json\"\n\t\"io\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\nfunc Load() (*Config, error) {\n\tf, err := os.Open(Path())\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tb, err := io.ReadAll(f)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\tvar c Config\n\terr = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"}},
	{name: "rv2 fail: a source read's bytes handed to a reader",
		files: cfgFile("import (\n\t\"bytes\"\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.NewDecoder(bytes.NewReader(b)).Decode(&c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 3|config.go|cannot follow"}},
	{name: "rv2 fail: a source read decoded into a map",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\nfunc Load() (map[string]string, error) {\n\tb, err := os.ReadFile(Path())\n\tm := map[string]string{}\n\t_ = json.Unmarshal(b, &m)\n\treturn m, err\n}\n"),
		want:  []string{"rule 3|config.go|not a struct"}},
}

func init() { cases = append(cases, review2b...) }
