// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 6 (verifyl2-codex-r6, a new session over the whole
// branch): each input as Codex gave it, written before its fix and failing
// without it.

func init() { cases = append(cases, review6...) }

var review6 = []tcase{
	// Class range-writes.
	one("rv6 fail: a range clause sets a client's transport",
		"func probe() {\n\tc := &http.Client{Transport: credkit.Transport(nil)}\n\tfor _, c.Transport = range []http.RoundTripper{nil} {\n\t}\n\t_, _ = c.Get(\"https://outside.example\")\n}\n",
		"rule 2|case.go|net/http.Client.Transport|range clause"),

	// Class carrier-to-foreign.
	{name: "rv6 fail: a struct holding a client decoded into",
		files: file("\t\"encoding/json\"\n\t\"net/http\"", "type envelope struct{ C *http.Client }\n\nfunc probe() {\n\tvar b envelope\n\t_ = json.Unmarshal([]byte(`{\"C\":{}}`), &b)\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n"),
		want:  []string{"rule 2|case.go|carrier:net/http.Client|encoding/json.Unmarshal"}},
	{name: "rv6 pass: a struct without a sink decoded into",
		files: file("\t\"encoding/json\"", "type envelope struct{ N int }\n\nfunc probe() {\n\tvar b envelope\n\t_ = json.Unmarshal([]byte(`{\"N\":1}`), &b)\n}\n")},

	// Class func-value-callees.
	{name: "rv6 fail: a callback handed to another package's function variable",
		files: with(dep("each4.go", "type Getter interface{ Get(string) (*http.Response, error) }\n\nvar Each = func(f func(Getter)) { f(&http.Client{}) }\n"),
			caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\thttpx.Each(func(g httpx.Getter) { _, _ = g.Get(\"https://outside.example\") })\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|inbound:net/http.Client|example.com/httpx.Each"}},

	// Class tuple-writers.
	{name: "rv6 fail: a writer handed on in a tuple before Start",
		files: file("\t\"encoding/binary\"\n\t\"io\"\n\t\"os\"", "func outputArgs() (io.Writer, binary.ByteOrder, any) {\n\treturn os.Stdout, binary.LittleEndian, []byte(os.Getenv(\"ACME_TOKEN\"))\n}\n\nfunc init() { _ = binary.Write(outputArgs()) }\n"),
		want:  []string{"rule 2|case.go|writer:io.Writer|before credkit.Start"}},

	// Class decoder-keys.
	{name: "rv6 fail: a toml tag does not rename a field the JSON decoder reads",
		edits: [][3]string{{"internal/config/config.go", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n", "\tTimeout      int               `toml:\"timeout\" json:\"timeout\"`\n\tShadow       string            `toml:\"token\"`\n"}},
		want:  []string{"rule 3|config.go|config:Shadow"}},
	{name: "rv6 pass: toml tags on a TOML-decoded config",
		files: cfgFile("import (\n\t\"os\"\n\t\"path/filepath\"\n\n\t\"example.com/tomlx\"\n)\n\ntype Config struct {\n\tBaseURL string `toml:\"base_url\"`\n\tToken   string `toml:\"token\"`\n\tPath    string `toml:\"-\" json:\"-\"`\n\tProfile string `toml:\"profile\"`\n}\n\n" + pathFn + "\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar c Config\n\t_ = tomlx.Unmarshal(b, &c)\n\treturn &c, err\n}\n"),
		edits: [][3]string{{"go.mod", "example.com/httpx => ./third_party/httpx", "example.com/httpx => ./third_party/httpx\n\texample.com/tomlx => ./third_party/tomlx"}, {"go.mod", "\texample.com/httpx v0.0.0", "\texample.com/httpx v0.0.0\n\texample.com/tomlx v0.0.0"}}},
}

func init() {
	review6[len(review6)-1].files["third_party/tomlx/go.mod"] = "module example.com/tomlx\n\ngo 1.22\n"
	review6[len(review6)-1].files["third_party/tomlx/tomlx.go"] = "package tomlx\n\nfunc Unmarshal(b []byte, v any) error { return nil }\n"
}

// Self-review after round 6: tuples and the other destinations.
func init() {
	cases = append(cases,
		tcase{name: "rv6 fail: a struct holding a client decoded into through a tuple",
			files: file("\t\"encoding/json\"\n\t\"net/http\"", "type envelope struct{ C *http.Client }\n\nfunc args(b *envelope) ([]byte, *envelope) { return []byte(`{\"C\":{}}`), b }\n\nfunc probe() {\n\tvar b envelope\n\t_ = json.Unmarshal(args(&b))\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n"),
			want:  []string{"rule 2|case.go|carrier:net/http.Client|encoding/json.Unmarshal"}},
		tcase{name: "rv6 fail: a struct holding a client stored in another package's variable",
			files: with(dep("box.go", "var Box any\n\nvar _ = http.MethodGet\n"),
				caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"\n"+credkitImport, "type envelope struct{ C *http.Client }\n\nfunc probe() {\n\thttpx.Box = &envelope{C: &http.Client{Transport: credkit.Transport(nil)}}\n}\n")[caseFile]),
			want: []string{"rule 2|case.go|carrier:net/http.Client|example.com/httpx.Box"}},
	)
}

func init() {
	cases = append(cases, tcase{name: "rv6 pass: a connector router set as a client's transport",
		files: file("\t\"net/http\"\n\n"+credkitImport, "type router struct{ c *http.Client }\n\nfunc (r *router) RoundTrip(q *http.Request) (*http.Response, error) { return nil, nil }\n\nfunc g() *http.Client {\n\tr := &router{c: &http.Client{Transport: credkit.Transport(nil)}}\n\treturn &http.Client{Transport: credkit.Transport(r)}\n}\n")})
}
