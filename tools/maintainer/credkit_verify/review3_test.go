// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 3 (verifyl2-codex-r3): each input as Codex gave it,
// written before its fix and failing without it, plus the cases that make each
// allow-list rule the only one that catches something (the mutation proof).

func init() { cases = append(cases, review3...) }

const credkitImport = "\t\"acme-pp-cli/internal/credkit\"\n"

// argsConfig: via reads the command line, Load reads via(os.Args[1:]).
func argsConfig(path string) map[string]string {
	return cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\n" + path + "\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path(os.Args[1:]))\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n")
}

const argsPath = "func Path(args []string) string {\n\tif len(args) > 0 {\n\t\treturn args[0]\n\t}\n\treturn \"acme.toml\"\n}\n"

var argsWiring = [][3]string{{"internal/cli/wiring.go", "[]string{config.Path()}", "[]string{config.Path(args)}"}}

func with(m map[string]string, kv ...string) map[string]string {
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

var review3 = []tcase{
	// Class sink-kinds: every sink kind an expression holds is checked.
	{name: "rv3 fail: a transport in a tuple exempts the clients beside it",
		files: map[string]string{
			"third_party/httpx/bundle.go": "package httpx\n\nimport \"net/http\"\n\nfunc Bundle() (*http.Transport, []*http.Client) {\n\treturn &http.Transport{}, []*http.Client{{}}\n}\n",
			caseFile:                      file("\t\"example.com/httpx\"", "func leak() {\n\t_, clients := httpx.Bundle()\n\t_, _ = clients[0].Get(\"https://outside.example\")\n}\n")[caseFile]},
		want: []string{"rule 2|case.go|example.com/httpx.Bundle"}},

	// Class named-carriers: a named type holds what its underlying type holds.
	{name: "rv3 fail: a named slice of clients from another package",
		files: map[string]string{
			"third_party/httpx/pool.go": "package httpx\n\nimport \"net/http\"\n\ntype Clients []*http.Client\n\nfunc Pool() Clients { return Clients{&http.Client{}} }\n",
			caseFile:                    file("\t\"example.com/httpx\"", "func leak() {\n\tclients := httpx.Pool()\n\t_, _ = clients[0].Get(\"https://outside.example\")\n}\n")[caseFile]},
		want: []string{"rule 2|case.go|example.com/httpx.Pool"}},
	{name: "rv3 pass: a connector's named slice of checked clients",
		files: file("\t\"net/http\"\n\n"+credkitImport, "type clients []*http.Client\n\nfunc g() {\n\tcs := clients{{Transport: credkit.Transport(nil)}}\n\t_, _ = cs[0].Get(\"https://x\")\n}\n")},
	{name: "rv3 fail: reflection clears a client inside a connector struct",
		files: file("\t\"net/http\"\n\t\"reflect\"\n\n"+credkitImport, "type box struct{ C *http.Client }\n\nfunc leak() {\n\tb := box{C: &http.Client{Transport: credkit.Transport(nil)}}\n\treflect.ValueOf(b).FieldByName(\"C\").Elem().SetZero()\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n"),
		want:  []string{"rule 2|case.go|reflect.ValueOf|holding a net/http.Client", "rule 2|case.go|reflect.Value.SetZero", "rule 2|case.go|reflect|a use of package reflect"}},
	{name: "rv3 fail: reflection writes through a value boxed in an interface",
		files: file("\t\"reflect\"", "func clear(x any) { reflect.ValueOf(x).Elem().Field(0).SetZero() }\n"),
		want:  []string{"rule 2|case.go|reflect.Value.SetZero", "rule 2|case.go|reflect|a use of package reflect"}},
	// Loop ruling 13g: every use of reflect or unsafe fails until the ledger
	// lists it with a reason, reads included.
	{name: "rv3 fail: reflection that only reads",
		files: file("\t\"reflect\"", "func isNil(x any) bool {\n\tv := reflect.ValueOf(x)\n\treturn v.Kind() == reflect.Ptr && v.IsNil()\n}\n"),
		want:  []string{"rule 2|case.go|reflect|a use of package reflect"}},
	{name: "rv3 pass: reflection that only reads, listed",
		files: file("\t\"reflect\"", "func isNil(x any) bool {\n\tv := reflect.ValueOf(x)\n\treturn v.Kind() == reflect.Ptr && v.IsNil()\n}\n"),
		sites: []string{`{"file": "internal/cli/case.go", "func": "isNil", "what": "reflect", "count": 4, "kind": "exempt", "reason": "reads a value's kind; never a command, client or transport"}`}},

	// The blanket finding listed, a value holding a sink is still never handed
	// to reflect or unsafe (proves reflectArgs, which the carrier rule shadows
	// for reflect.ValueOf).
	{name: "rv3 fail: an unsafe pointer to a struct holding a client, the unsafe use listed",
		files: file("\t\"net/http\"\n\t\"unsafe\"", "type box struct{ C *http.Client }\n\nfunc size(b *box) uintptr {\n\treturn uintptr(unsafe.Pointer(b))\n}\n"),
		sites: []string{`{"file": "internal/cli/case.go", "func": "size", "what": "unsafe", "count": 1, "kind": "exempt", "reason": "a pointer's address for a log line"}`},
		want:  []string{"rule 2|case.go|unsafe.Pointer|holding a net/http.Client", "rule 2|case.go|carrier-convert:net/http.Client"}},
	// Class method-expressions: a method expression is the method.
	{name: "rv3 fail: a slog handler's Handle through a method expression before Start",
		files: file("\t\"context\"\n\t\"log/slog\"\n\t\"os\"\n\t\"time\"", "func init() {\n\th := slog.NewTextHandler(os.Stdout, nil)\n\trecord := slog.NewRecord(time.Time{}, slog.LevelInfo, os.Getenv(\"ACME_TOKEN\"), 0)\n\t_ = (*slog.TextHandler).Handle(h, context.Background(), record)\n}\n"),
		want:  []string{"rule 2|case.go|Handle|before credkit.Start"}},
	{name: "rv3 pass: a builder's method expression before Start",
		files: file("\t\"strings\"", "var b strings.Builder\n\nfunc init() { _, _ = (*strings.Builder).WriteString(&b, \"x\") }\n")},

	// Class pure-callbacks: a path function passes no function as a value.
	{name: "rv3 fail: via hands a stateful callback to strings.Map",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"strings\"\n)\n\n" + baseConfigType + "\nvar count int\n\nfunc next(r rune) rune {\n\tcount++\n\tif count == 1 {\n\t\treturn 'a'\n\t}\n\treturn 'b'\n}\n\nfunc Path() string { return strings.Map(next, \"x\") + \".json\" }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|config.go|via|function as a value"}},
	{name: "rv3 pass: via hands strings.Map a function literal",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"strings\"\n)\n\n" + baseConfigType + "\nfunc Path() string {\n\treturn strings.Map(func(r rune) rune { return r + 1 }, \"`\") + \".json\"\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n")},

	// Class purity-memo: a function is proved again under each restriction.
	{name: "rv3 fail: a helper proved unrestricted, then handed the command line",
		edits: argsWiring,
		files: argsConfig("func edit(a []string) string {\n\ta = append(a[:0], a[0]+\"x\")\n\treturn a[0]\n}\n\nfunc Path(args []string) string {\n\t_ = edit([]string{\"probe\"})\n\treturn edit(args)\n}\n"),
		want:  []string{"rule 4|config.go|via|not a pure function"}},

	// Class argv-writes: parentheses and any non-via callee.
	{name: "rv3 fail: a parenthesized write to os.Args",
		edits: argsWiring,
		files: with(argsConfig(argsPath), caseFile, file("\t\"os\"", "func g() { (os.Args[1]) = \"other.json\" }\n")[caseFile]),
		want:  []string{"rule 4|case.go|os.Args|can change"}},
	{name: "rv3 fail: os.Args[1:] handed to a path-shaped function that is not via",
		edits: argsWiring,
		files: with(argsConfig(argsPath), caseFile, file("\t\"os\"", "func change(args []string) string {\n\targs[0] = \"other.json\"\n\treturn \"\"\n}\n\nfunc g() { _ = change(os.Args[1:]) }\n")[caseFile]),
		want:  []string{"rule 4|case.go|os.Args|can change"}},
	{name: "rv3 pass: os.Args[1:] handed to via itself",
		edits: argsWiring,
		files: with(argsConfig(argsPath), caseFile, file("\t\"os\"\n\n\t\"acme-pp-cli/internal/config\"", "func g() string { return config.Path(os.Args[1:]) }\n")[caseFile])},

	// Class working-directory: a source path is resolved against the directory Start ran in.
	one("rv3 fail: the working directory changes while a source is declared", "func g() { _ = os.Chdir(\"other\") }\n", "rule 4|case.go|os.Chdir|working directory"),

	// Class custom-decoding: the decoded type decodes itself.
	{name: "rv3 fail: a custom UnmarshalJSON decodes the real Config",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\ntype outer struct{}\n\nvar leaked string\n\nfunc (*outer) UnmarshalJSON(b []byte) error {\n\ttype Config struct {\n\t\tNewSecret string `json:\"new_secret\"`\n\t}\n\n\tvar c Config\n\terr := json.Unmarshal(b, &c)\n\tleaked = c.NewSecret\n\treturn err\n}\n\nfunc Load() (string, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c outer\n\tif err == nil {\n\t\terr = json.Unmarshal(b, &c)\n\t}\n\treturn leaked, err\n}\n"),
		want:  []string{"rule 3|config.go|UnmarshalJSON", "rule 3|config.go|config type outer decodes itself"}},
	// A time.Time decodes itself, so it is classified like a number (Codex verifyl4 round 2).
	{name: "rv3 pass: a decoded config holding a time.Time",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"time\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\ntype file struct {\n\tConfig\n\tSaved time.Time `json:\"saved\"`\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar f file\n\t_ = json.Unmarshal(b, &f)\n\treturn &f.Config, err\n}\n"),
		edits: [][3]string{{"internal/cli/wiring.go", `{Source: "config:profile", Reason: "a profile name"},`, `{Source: "config:profile", Reason: "a profile name"}, {Source: "config:saved", Reason: "when the file was saved"},`}}},
	{name: "rv3 fail: a decoded config holding a time.Time not classified",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"time\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\ntype file struct {\n\tConfig\n\tSaved time.Time `json:\"saved\"`\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar f file\n\t_ = json.Unmarshal(b, &f)\n\treturn &f.Config, err\n}\n"),
		want:  []string{"rule 3|config.go|config:saved"}},

	// Mutation proof: the case only one rule catches.
	one("rv3 fail: a command method outside the allowed list", "func g() { c := exec.Command(\"cat\"); _, _ = c.StdinPipe() }\n", "rule 2|case.go|os/exec.Cmd.StdinPipe"),
	one("rv3 fail: a no-op conversion of a client", "func g(c *http.Client) { d := (*http.Client)(c); _ = d }\n", "rule 2|case.go|sink:net/http.Client|a conversion makes this", "rule 2|case.go|sink:net/http.Client|converted to"),
	one("rv3 fail: a type assertion recovers a client", "func g(x any) { c := x.(*http.Client); _ = c }\n", "rule 2|case.go|sink:net/http.Client|type assertion"),
	{name: "rv3 fail: a struct identical to a client as a type argument",
		files: file("\t\"net/http\"\n\t\"time\"", "func id[T any](t T) T { return t }\n\nfunc g() {\n\t_ = id(struct {\n\t\tTransport     http.RoundTripper\n\t\tCheckRedirect func(*http.Request, []*http.Request) error\n\t\tJar           http.CookieJar\n\t\tTimeout       time.Duration\n\t}{})\n}\n"),
		want:  []string{"rule 2|case.go|generic:net/http.Client", "rule 2|case.go|sink:net/http.Client|type parameter of id", "rule 2|case.go|net/http.Client{}", "rule 2|case.go|sink:net/http.Client|becomes a T"}},
	{name: "rv3 fail: a cipher stream writer to stdout before Start",
		files: file("\t\"crypto/cipher\"\n\t\"io\"\n\t\"os\"", "func put(w io.Writer) {}\n\nfunc init() { put(cipher.StreamWriter{W: os.Stdout}) }\n"),
		want:  []string{"rule 2|case.go|writer:crypto/cipher.StreamWriter|before credkit.Start"}},
	one("rv3 fail: a registration after another call",
		"func skip() {}\n\nfunc init() {\n\tskip()\n\tsql.Register(\"credkit-c\", credkit.SQLDriver(fakeDriver{}))\n}\n\nfunc g() { _, _ = sql.Open(\"credkit-c\", \"x\") }\n",
		"rule 2|case.go|database/sql.Register|not among the leading", "rule 2|case.go|database/sql.Open|does not wrap"),
	{name: "rv3 fail: via appends into the command line",
		edits: argsWiring,
		files: argsConfig("func Path(args []string) string {\n\ta := append(args[:0], \"other.toml\")\n\treturn a[0]\n}\n"),
		want:  []string{"rule 4|config.go|via|command-line parameter"}},
	{name: "rv3 fail: via writes os.Args",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nfunc Path() string {\n\tos.Args[0] = \"x\"\n\treturn \"acme.toml\"\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		want:  []string{"rule 4|config.go|via|writes something other than its own local variable"}},
}

func init() {
	cases = append(cases, tcase{name: "rv3 fail: os.Args written by a range clause",
		edits: argsWiring,
		files: with(argsConfig(argsPath), caseFile, file("\t\"os\"", "func g() {\n\tfor _, os.Args[1] = range []string{\"other.json\"} {\n\t}\n}\n")[caseFile]),
		want:  []string{"rule 4|case.go|os.Args|can change"}})
}

func init() {
	cases = append(cases, tcase{name: "rv3 fail: a zero command held by value in a struct",
		files: file("\t\"os/exec\"\n\n"+credkitImport, "type box struct{ C exec.Cmd }\n\nfunc g() {\n\tvar b box\n\tb.C.Path = \"/bin/sh\"\n\t_ = credkit.RunCommand(&b.C)\n}\n"),
		want:  []string{"rule 2|case.go|os/exec.Cmd|held by value"}})
}

func init() {
	cases = append(cases, tcase{name: "rv3 fail: the working directory changes on Windows",
		goos:  []string{"windows/amd64"},
		files: file("\t\"syscall\"", "func g() { _ = syscall.SetCurrentDirectory(nil) }\n"),
		want:  []string{"rule 4|case.go|syscall.SetCurrentDirectory|working directory", "rule 2|case.go|syscall.SetCurrentDirectory|system call"}})
}

func init() {
	cases = append(cases,
		tcase{name: "rv3 fail: a uintptr made a pointer",
			files: file("\t\"unsafe\"", "func g(u uintptr) { _ = unsafe.Pointer(u) }\n"),
			want:  []string{"rule 2|case.go|unsafe|made from a uintptr"}},
		tcase{name: "rv3 fail: unsafe pointer arithmetic",
			files: file("\t\"unsafe\"", "type pair struct{ A, B int }\n\nfunc g(p *pair) { _ = unsafe.Add(unsafe.Pointer(&p.A), 8) }\n"),
			want:  []string{"rule 2|case.go|unsafe|unsafe.Add", "rule 2|case.go|unsafe|a use of package unsafe"}},
		tcase{name: "rv3 fail: an unsafe pointer reinterpreted",
			files: file("\t\"unsafe\"", "type pair struct{ A, B int }\n\nfunc g(x *int) { _ = (*pair)(unsafe.Pointer(x)) }\n"),
			want:  []string{"rule 2|case.go|unsafe|converted to *acme-pp-cli/internal/cli.pair", "rule 2|case.go|unsafe|a use of package unsafe"}},
		tcase{name: "rv3 fail: unsafe as the fleet uses it, for a system call",
			files: file("\t\"unsafe\"", "type info struct{ N int32 }\n\nfunc raw(p unsafe.Pointer, n int) uintptr { return uintptr(p) + uintptr(n) }\n\nfunc g() uintptr {\n\tvar i info\n\tbuf := make([]byte, 8)\n\t_ = raw(unsafe.Pointer(&buf[0]), len(buf))\n\treturn raw(unsafe.Pointer(&i), int(unsafe.Sizeof(i)+unsafe.Offsetof(i.N)))\n}\n"),
			want:  []string{"rule 2|case.go|unsafe|a use of package unsafe"}},
		tcase{name: "rv3 pass: unsafe as the fleet uses it, for a system call, listed",
			files: file("\t\"unsafe\"", "type info struct{ N int32 }\n\nfunc raw(p unsafe.Pointer, n int) uintptr { return uintptr(p) + uintptr(n) }\n\nfunc g() uintptr {\n\tvar i info\n\tbuf := make([]byte, 8)\n\t_ = raw(unsafe.Pointer(&buf[0]), len(buf))\n\treturn raw(unsafe.Pointer(&i), int(unsafe.Sizeof(i)+unsafe.Offsetof(i.N)))\n}\n"),
			sites: []string{`{"file": "internal/cli/case.go", "func": "raw", "what": "unsafe", "count": 1, "kind": "exempt", "reason": "a system call's pointer argument"}`, `{"file": "internal/cli/case.go", "func": "g", "what": "unsafe", "count": 4, "kind": "exempt", "reason": "pointers to local buffers for a system call"}`}},
	)
}

func init() {
	cases = append(cases, tcase{name: "rv3 fail: a dependency without a dot in its path decodes itself",
		files: with(cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n\n\t\"dotless/codec\"\n)\n\n"+baseConfigType+"\n"+pathFn+"\ntype file struct {\n\tConfig\n\tX codec.Self `json:\"x\"`\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar f file\n\t_ = json.Unmarshal(b, &f)\n\treturn &f.Config, err\n}\n"),
			"third_party/dotless/go.mod", "module dotless\n\ngo 1.22\n",
			"third_party/dotless/codec/codec.go", "package codec\n\ntype Self struct{}\n\nfunc (*Self) UnmarshalJSON([]byte) error { return nil }\n"),
		edits: [][3]string{{"go.mod", "example.com/httpx => ./third_party/httpx", "example.com/httpx => ./third_party/httpx\n\tdotless => ./third_party/dotless"}, {"go.mod", "\texample.com/httpx v0.0.0", "\texample.com/httpx v0.0.0\n\tdotless v0.0.0"}},
		want:  []string{"rule 3|config.go|dotless/codec.Self.UnmarshalJSON", "rule 3|config.go|config:x"}})
}

func init() {
	cases = append(cases, one("rv3 fail: the environment changed through syscall while via reads it",
		"func g() { _ = syscall.Setenv(\"TMPDIR\", \"/elsewhere\") }\n",
		"rule 4|case.go|syscall.Setenv|environment write", "rule 2|case.go|syscall.Setenv|system call"))
}

// Codex review round 4 (cut off by its provider's filter before a verdict):
// the probes it ran that the checker accepted, each failing first.
func init() {
	cases = append(cases,
		tcase{name: "rv3 fail: a client from another package recovered through reflect and an interface",
			files: map[string]string{
				"third_party/httpx/holder.go": "package httpx\n\nimport \"net/http\"\n\ntype Holder struct{ C *http.Client }\n\nfunc NewHolder() *Holder { return &Holder{C: &http.Client{}} }\n",
				caseFile:                      file("\t\"net/http\"\n\t\"reflect\"\n\n\t\"example.com/httpx\"", "type getter interface {\n\tGet(string) (*http.Response, error)\n}\n\nfunc leak() {\n\tvar opaque any = httpx.NewHolder()\n\tc := reflect.ValueOf(opaque).Elem().FieldByName(\"C\").Interface().(getter)\n\t_, _ = c.Get(\"https://outside.example\")\n}\n")[caseFile]},
			want: []string{"rule 2|case.go|assert:net/http.Client", "rule 2|case.go|reflect|a use of package reflect"}},
		tcase{name: "rv3 pass: assertions to interfaces no sink satisfies by sending",
			files: file("\t\"fmt\"\n\t\"io\"", "func g(x any) (string, bool) {\n\tif s, ok := x.(fmt.Stringer); ok {\n\t\treturn s.String(), true\n\t}\n\t_, ok := x.(io.Reader)\n\treturn \"\", ok\n}\n")},
		tcase{name: "rv3 fail: via ranges over a map",
			files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n)\n\n" + baseConfigType + "\nfunc Path() string {\n\tfor k := range map[string]bool{\"a.json\": true, \"b.json\": true} {\n\t\treturn k\n\t}\n\treturn \"a.json\"\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
			want:  []string{"rule 4|config.go|via|ranges over a map"}},
		tcase{name: "rv3 fail: a unicode table changed while via calls unicode",
			files: with(cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"unicode\"\n)\n\n"+baseConfigType+"\nfunc Path() string { return string(unicode.ToUpper('a')) + \".json\" }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
				caseFile, file("\t\"unicode\"", "func mutate() { unicode.CaseRanges = nil }\n")[caseFile]),
			want: []string{"rule 4|case.go|unicode.CaseRanges|path functions call"}},
		tcase{name: "rv3 pass: a unicode table as a unicode function's argument",
			files: file("\t\"unicode\"", "func g(r rune) bool { return unicode.Is(unicode.Cf, r) || unicode.In(r, unicode.Zl, unicode.Zp) }\n")},
		one("rv3 fail: another file renamed over a source path",
			"func g() { _ = os.Rename(\"other-token.json\", \"acme.toml\") }\n", "rule 2|case.go|os.Rename|moved or linked"),
	)
}

func init() {
	cases = append(cases,
		tcase{name: "rv3 fail: a generic asserts to an interface a client satisfies",
			files: file("\t\"net/http\"", "type getter interface {\n\tGet(string) (*http.Response, error)\n}\n\nfunc as[T any](x any) T { return x.(T) }\n\nfunc leak(x any) {\n\tc := as[getter](x)\n\t_, _ = c.Get(\"https://outside.example\")\n}\n"),
			want:  []string{"rule 2|case.go|generic:assert:net/http.Client"}},
		tcase{name: "rv3 pass: a generic over an interface no sink satisfies by sending",
			files: file("\t\"fmt\"", "func as[T any](x any) (T, bool) { t, ok := x.(T); return t, ok }\n\nfunc g(x any) string {\n\tif s, ok := as[fmt.Stringer](x); ok {\n\t\treturn s.String()\n\t}\n\treturn \"\"\n}\n")},
	)
}

func init() {
	cases = append(cases, tcase{name: "rv3 pass: via hands a pure standard-library function to strings.FieldsFunc",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"strings\"\n\t\"unicode\"\n)\n\n" + baseConfigType + "\nfunc Path() string { return strings.Join(strings.FieldsFunc(\" acme .json \", unicode.IsSpace), \"\") }\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = json.Unmarshal(b, &c)\n\treturn &c, err\n}\n")})
}
