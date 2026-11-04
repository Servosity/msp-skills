// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review round 5 (verifyl2-codex-r5): each input as Codex gave it,
// written before its fix and failing without it.

func init() { cases = append(cases, review5...) }

func dep(name, body string) map[string]string {
	return map[string]string{"third_party/httpx/" + name: "package httpx\n\nimport \"net/http\"\n\n" + body}
}

var review5 = []tcase{
	// Class func-value-producers: a call of a function value is checked through the value.
	{name: "rv5 fail: a client from another package's function variable",
		files: with(dep("factory.go", "var ClientFactory = func() *http.Client {\n\treturn &http.Client{}\n}\n"),
			caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\tc := httpx.ClientFactory()\n\t_, _ = c.Get(\"https://outside.example\")\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|example.com/httpx.ClientFactory"}},
	{name: "rv5 pass: a client from the connector's own function value",
		files: file("\t\"net/http\"\n\n"+credkitImport, "var factory = func() *http.Client { return &http.Client{Transport: credkit.Transport(nil)} }\n\nfunc g() {\n\tc := factory()\n\t_, _ = c.Get(\"https://x\")\n}\n")},

	// Class promoted-methods: an embedded field reached by promotion is a field read.
	{name: "rv5 fail: a client method promoted from another package's struct",
		files: with(dep("embedded.go", "type Embedded struct{ *http.Client }\n\nfunc NewEmbedded() *Embedded {\n\treturn &Embedded{&http.Client{}}\n}\n"),
			caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\t_, _ = httpx.NewEmbedded().Get(\"https://outside.example\")\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|example.com/httpx.Client|promotion"}},
	{name: "rv5 pass: a client method promoted from the connector's own struct",
		files: file("\t\"net/http\"\n\n"+credkitImport, "type api struct{ *http.Client }\n\nfunc g() {\n\ta := api{&http.Client{Transport: credkit.Transport(nil)}}\n\t_, _ = a.Get(\"https://x\")\n}\n")},

	// Class sending-interfaces: an interface a client or command satisfies by sending.
	{name: "rv5 fail: an assertion to a struct carrying a sending interface",
		files: with(dep("carrier.go", "func Carrier() any {\n\treturn struct {\n\t\tC interface{ Get(string) (*http.Response, error) }\n\t}{&http.Client{}}\n}\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "func probe() {\n\tv := httpx.Carrier().(struct {\n\t\tC interface{ Get(string) (*http.Response, error) }\n\t})\n\t_, _ = v.C.Get(\"https://outside.example\")\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|assert:net/http.Client"}},
	{name: "rv5 fail: another package returns a sending interface",
		files: with(dep("getter.go", "type Getter interface{ Get(string) (*http.Response, error) }\n\nfunc NewGetter() Getter { return &http.Client{} }\n"),
			caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\t_, _ = httpx.NewGetter().Get(\"https://outside.example\")\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|example.com/httpx.NewGetter|satisfies"}},
	{name: "rv5 pass: the connector's own sending interface over its own type",
		files: file("\t\"net/http\"", "type getter interface{ Get(string) (*http.Response, error) }\n\ntype fake struct{}\n\nfunc (fake) Get(string) (*http.Response, error) { return nil, nil }\n\nfunc g() {\n\tvar x getter = fake{}\n\t_, _ = x.Get(\"https://x\")\n}\n")},

	// Class decode-dynamic: an interface or a promoted decoding method in the decoded structure.
	{name: "rv5 fail: an anonymous decode target embeds json.Unmarshaler",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\ntype custom struct{}\n\nvar decodedSecret string\n\nfunc (*custom) UnmarshalJSON(b []byte) error {\n\tvar v struct {\n\t\tNewSecret string `json:\"new_secret\"`\n\t}\n\terr := json.Unmarshal(b, &v)\n\tdecodedSecret = v.NewSecret\n\treturn err\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tc := struct {\n\t\tjson.Unmarshaler `json:\"token\"`\n\t}{&custom{}}\n\tif err == nil {\n\t\terr = json.Unmarshal(b, &c)\n\t}\n\treturn &Config{Token: decodedSecret}, err\n}\n"),
		want:  []string{"rule 3|config.go|encoding/json.Unmarshaler", "rule 3|config.go|decodes itself"}},
	{name: "rv5 fail: a decoded struct holds an interface field",
		files: cfgFile("import (\n\t\"encoding/json\"\n\t\"os\"\n\t\"path/filepath\"\n)\n\n" + baseConfigType + "\n" + pathFn + "\ntype file struct {\n\tConfig\n\tExtra any `json:\"extra\"`\n}\n\nfunc Load() (*Config, error) {\n\tb, err := os.ReadFile(Path())\n\tvar f file\n\t_ = json.Unmarshal(b, &f)\n\treturn &f.Config, err\n}\n"),
		want:  []string{"rule 3|config.go|interface", "rule 3|config.go|config:extra"}},
}

func init() {
	cases = append(cases, tcase{name: "rv5 pass: a client's transport asserted back to the connector's own wrapper",
		files: file("\t\"net/http\"", "type wrap struct{ inner http.RoundTripper }\n\nfunc (w *wrap) RoundTrip(r *http.Request) (*http.Response, error) { return nil, nil }\n\nfunc g(c *http.Client) *wrap {\n\tw, _ := c.Transport.(*wrap)\n\treturn w\n}\n")})
}

// Self-review after round 5: the sending interface reaching the connector as
// a parameter another package fills.
func init() {
	cases = append(cases,
		tcase{name: "rv5 fail: a callback handed to another package receives a sending interface",
			files: with(dep("each.go", "type Getter interface{ Get(string) (*http.Response, error) }\n\nfunc Each(f func(Getter)) { f(&http.Client{}) }\n"),
				caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\thttpx.Each(func(g httpx.Getter) { _, _ = g.Get(\"https://outside.example\") })\n}\n")[caseFile]),
			want: []string{"rule 2|case.go|inbound:net/http.Client|example.com/httpx.Each"}},
		tcase{name: "rv5 fail: a visitor handed to another package receives a sending interface",
			files: with(dep("visit.go", "type Getter interface{ Get(string) (*http.Response, error) }\n\ntype Visitor interface{ Visit(Getter) }\n\nfunc Walk(v Visitor) { v.Visit(&http.Client{}) }\n"),
				caseFile, file("\t\"example.com/httpx\"", "type vis struct{}\n\nfunc (vis) Visit(g httpx.Getter) { _, _ = g.Get(\"https://outside.example\") }\n\nfunc probe() { httpx.Walk(vis{}) }\n")[caseFile]),
			want: []string{"rule 2|case.go|inbound:net/http.Client|example.com/httpx.Walk"}},
		tcase{name: "rv5 fail: a hook stored in another package's variable receives a sending interface",
			files: with(dep("hook.go", "type Getter interface{ Get(string) (*http.Response, error) }\n\nvar Hook func(Getter)\n"),
				caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\thttpx.Hook = func(g httpx.Getter) { _, _ = g.Get(\"https://outside.example\") }\n}\n")[caseFile]),
			want: []string{"rule 2|case.go|inbound:net/http.Client|example.com/httpx.Hook", "rule 2|case.go|example.com/httpx.Hook|another package holds"}},
		tcase{name: "rv5 pass: a callback handed to another package with ordinary parameters",
			files: with(dep("each3.go", "var _ = http.MethodGet\n\nfunc Each3(f func(string, int)) { f(\"x\", 1) }\n"),
				caseFile, file("\t\"example.com/httpx\"", "func probe() {\n\thttpx.Each3(func(s string, n int) { _, _ = s, n })\n}\n")[caseFile])},
	)
}
