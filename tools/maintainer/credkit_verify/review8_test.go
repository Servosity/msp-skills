// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 8 (verifyl2-codex-r8, a new session over the whole
// branch): each input as Codex gave it, written before its fix and failing
// without it.

func init() { cases = append(cases, review8...) }

var review8 = []tcase{
	// Class generic-carriers: a type argument holding a sink anywhere, named structs included.
	{name: "rv8 fail: a generic decodes into a struct holding a client",
		files: file("\t\"encoding/json\"\n\t\"net/http\"", "type envelope struct{ C *http.Client }\n\nfunc decode[T any](b []byte, dst *T) error {\n\treturn json.Unmarshal(b, dst)\n}\n\nfunc probe() {\n\tvar b envelope\n\t_ = decode([]byte(`{\"C\":{}}`), &b)\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n"),
		want:  []string{"rule 2|case.go|generic:net/http.Client"}},
	{name: "rv8 pass: a generic decodes into a struct without a sink",
		files: file("\t\"encoding/json\"", "type envelope struct{ N int }\n\nfunc decode[T any](b []byte, dst *T) error {\n\treturn json.Unmarshal(b, dst)\n}\n\nfunc probe() {\n\tvar b envelope\n\t_ = decode([]byte(`{\"N\":1}`), &b)\n}\n")},

	// Class carrier-conversions: converting a value that holds a sink to another type.
	{name: "rv8 fail: another package's struct converted to the connector's",
		files: with(dep("box.go", "type Box struct{ C *http.Client }\n\nfunc NewBox() Box {\n\treturn Box{C: &http.Client{}}\n}\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "type localBox struct{ C *http.Client }\n\nfunc probe() {\n\tb := localBox(httpx.NewBox())\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|carrier-convert:net/http.Client"}},
	{name: "rv8 pass: a struct without a sink converted",
		files: file("", "type a struct{ N int }\n\ntype b struct{ N int }\n\nfunc probe() int { return b(a{N: 1}).N }\n")},

	// Class range-conversions: every place a value is stored as an interface.
	one("rv8 fail: a range clause stores a value as an interface before Start",
		"type banner struct{}\n\nfunc (banner) String() string {\n\tfmt.Println(os.Getenv(\"ACME_TOKEN\"))\n\treturn \"\"\n}\n\nfunc init() {\n\tvar s fmt.Stringer\n\tfor _, s = range []banner{{}} {\n\t\t_ = s.String()\n\t}\n}\n",
		"rule 2|case.go|fmt.Println|before credkit.Start"),
	one("rv8 fail: a map key stores a value as an interface before Start",
		"type banner struct{}\n\nfunc (banner) String() string {\n\tfmt.Println(os.Getenv(\"ACME_TOKEN\"))\n\treturn \"\"\n}\n\nfunc init() {\n\tm := map[fmt.Stringer]int{}\n\tm[banner{}] = 1\n\tfor k := range m {\n\t\t_ = k.String()\n\t}\n}\n",
		"rule 2|case.go|fmt.Println|before credkit.Start"),
	one("rv8 fail: a range over an iterator stores a value as an interface before Start",
		"type banner struct{}\n\nfunc (banner) String() string {\n\tfmt.Println(os.Getenv(\"ACME_TOKEN\"))\n\treturn \"\"\n}\n\nfunc seq(yield func(banner) bool) { yield(banner{}) }\n\nfunc init() {\n\tvar s fmt.Stringer\n\tfor s = range seq {\n\t\t_ = s.String()\n\t}\n}\n",
		"rule 2|case.go|fmt.Println|before credkit.Start"),

	// Class decoder-keys (DecodeFile): every decode records its decoder's format.
	{name: "rv8 fail: DecodeFile reads the Go name a json tag hides",
		files: map[string]string{
			"../credkit-sites.json":      ledgerWith("example.com/tomlx.DecodeFile"),
			"internal/config/config.go":  "package config\n\nimport (\n\t\"os\"\n\t\"path/filepath\"\n\n\t\"example.com/tomlx\"\n)\n\ntype Config struct {\n\tBaseURL string `toml:\"base_url\"`\n\tToken   string `toml:\"token\"`\n\tPath    string `toml:\"-\" json:\"-\"`\n\tProfile string `toml:\"profile\"`\n\tShadow  string `json:\"token\"`\n}\n\n" + pathFn + "\nfunc Load() (*Config, error) {\n\tvar c Config\n\t_, err := tomlx.DecodeFile(Path(), &c)\n\treturn &c, err\n}\n",
			"third_party/tomlx/go.mod":   "module example.com/tomlx\n\ngo 1.22\n",
			"third_party/tomlx/tomlx.go": "package tomlx\n\nfunc DecodeFile(path string, v any) (struct{}, error) { return struct{}{}, nil }\n"},
		edits: [][3]string{{"go.mod", "example.com/httpx => ./third_party/httpx", "example.com/httpx => ./third_party/httpx\n\texample.com/tomlx => ./third_party/tomlx"}, {"go.mod", "\texample.com/httpx v0.0.0", "\texample.com/httpx v0.0.0\n\texample.com/tomlx v0.0.0"}},
		want:  []string{"rule 3|config.go|config:Shadow"}},
}
