// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Codex review verifyl4 round 1 (a new session over the whole branch, after
// the SSA rebuild): each input as Codex gave it, written before its fix and
// failing without it.

func init() { cases = append(cases, review9...) }

var review9 = []tcase{
	// Class interface-receiver-arguments: the followed value is also an argument.
	strict(tcase{name: "rv9 fail: an interface value handed to its own method",
		files: file("\t\"encoding/json\"\n\t\"net/http\"\n\n"+credkitImport, "type filler interface{ Fill(filler) }\n\ntype box struct{ C *http.Client }\n\nfunc (*box) Fill(x filler) {\n\t_ = json.Unmarshal([]byte(`{\"C\":{\"Transport\":null}}`), x)\n}\n\nfunc probe() {\n\tb := &box{C: &http.Client{Transport: credkit.Transport(nil)}}\n\tvar v filler = b\n\tv.Fill(v)\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n"),
		want:  []string{"rule 2|case.go|carrier|encoding/json.Unmarshal"}}),

	// Class inbound-callback-producers: a callback another package gets from
	// a function it is handed, then calls with a value it fills.
	{name: "rv9 fail: a callback factory handed to another package",
		files: with(dep("apply.go", "type Getter interface {\n\tGet(string) (*http.Response, error)\n}\n\nfunc Apply(factory func() func(Getter)) {\n\tfactory()(&http.Client{})\n}\n"),
			caseFile, file("\t\"example.com/httpx\"", "func factory() func(httpx.Getter) {\n\treturn func(c httpx.Getter) { _, _ = c.Get(\"https://outside.example\") }\n}\n\nfunc probe() { httpx.Apply(factory) }\n")[caseFile]),
		want: []string{"rule 2|case.go|inbound:net/http.Client|example.com/httpx.Apply"}},

	// The SSA side of the class: a sending interface holds a sink, so an
	// assertion to one fails even when the connector declares it.
	strict(tcase{name: "rv9 fail: an assertion to the connector's own sending interface",
		files: file("\t\"net/http\"", "type getter interface{ Get(string) (*http.Response, error) }\n\nfunc g(x any) {\n\tif c, ok := x.(getter); ok {\n\t\t_, _ = c.Get(\"https://outside.example\")\n\t}\n}\n"),
		want:  []string{"rule 2|case.go|type assertion", "rule 2|case.go|getter.Get|through an interface"}}),

	// Class decoder-keys (tag validity): encoding/json ignores a tag name it
	// rejects and reads the Go name.
	{name: "rv9 fail: a json tag name the decoder rejects",
		edits: [][3]string{
			{"internal/config/config.go", "\tAuth\n}", "\tShadow string `json:\"token\\t\"`\n\tAuth\n}"},
			{"internal/cli/wiring.go", `ConfigFields:    []string{"token", "client_secret"},`, `ConfigFields:    []string{"token", "client_secret", "token\t"},`}},
		want: []string{"rule 3|config.go|config:Shadow"}},
	{name: "rv9 pass: a json tag name the decoder accepts",
		edits: [][3]string{
			{"internal/config/config.go", "\tAuth\n}", "\tShadow string `json:\"tok-en.x\"`\n\tAuth\n}"},
			{"internal/cli/wiring.go", `ConfigFields:    []string{"token", "client_secret"},`, `ConfigFields:    []string{"token", "client_secret", "tok-en.x"},`}}},

	{name: "rv9 fail: a json tag \"-,\" is the key -",
		edits: [][3]string{{"internal/config/config.go", "\tAuth\n}", "\tDash string `json:\"-,\"`\n\tAuth\n}"}},
		want:  []string{"rule 3|config.go|config:-"}},

	// Class numeric-config-fields: numbers count (contract section 1).
	{name: "rv9 fail: a numeric config field not classified",
		edits: [][3]string{{"internal/config/config.go", "\tAuth\n}", "\tPIN int `json:\"pin\" toml:\"pin\"`\n\tAuth\n}"}},
		want:  []string{"rule 3|config.go|config:pin"}},
	{name: "rv9 pass: a numeric config field classified",
		edits: [][3]string{
			{"internal/config/config.go", "\tAuth\n}", "\tPIN int `json:\"pin\" toml:\"pin\"`\n\tAuth\n}"},
			{"internal/cli/wiring.go", `ConfigFields:    []string{"token", "client_secret"},`, `ConfigFields:    []string{"token", "client_secret", "pin"},`}}},

	// Round 2. Class foreign-generic-type-arguments: another package's
	// generic instantiated with a type that holds a sink.
	strict(tcase{name: "rv9 fail: another package's generic instantiated with a sink-taking visitor",
		files: with(dep("feed.go", "func Feed[T interface{ Use(*http.Client) }]() {\n\tvar v T\n\tv.Use(&http.Client{})\n}\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "type visitor struct{}\n\nfunc (visitor) Use(c *http.Client) { _, _ = c.Get(\"https://outside.example\") }\n\nfunc probe() { httpx.Feed[visitor]() }\n")[caseFile]),
		want: []string{"rule 2|case.go|generic:"}}),

	// Class opaque-scalars: a type that decodes itself holds what its text says.
	{name: "rv9 fail: a big.Int config field not classified",
		edits: [][3]string{
			{"internal/config/config.go", "\t\"encoding/json\"\n", "\t\"encoding/json\"\n\t\"math/big\"\n"},
			{"internal/config/config.go", "\tAuth\n}", "\tPIN big.Int `json:\"pin\" toml:\"pin\"`\n\tAuth\n}"}},
		want: []string{"rule 3|config.go|config:pin"}},
	{name: "rv9 pass: a big.Int config field classified",
		edits: [][3]string{
			{"internal/config/config.go", "\t\"encoding/json\"\n", "\t\"encoding/json\"\n\t\"math/big\"\n"},
			{"internal/config/config.go", "\tAuth\n}", "\tPIN big.Int `json:\"pin\" toml:\"pin\"`\n\tAuth\n}"},
			{"internal/cli/wiring.go", `ConfigFields:    []string{"token", "client_secret"},`, `ConfigFields:    []string{"token", "client_secret", "pin"},`}}},

	{name: "rv9 pass: an unexported self-decoding config field no decoder reaches",
		edits: [][3]string{
			{"internal/config/config.go", "\t\"encoding/json\"\n", "\t\"encoding/json\"\n\t\"regexp\"\n"},
			{"internal/config/config.go", "\tAuth\n}", "\tpatterns []*regexp.Regexp\n\tAuth\n}\n\nvar _ = Config{}.patterns"}}},

	// Class inline-maps: a map that takes every key no other field takes.
	{name: "rv9 fail: a yaml inline map in the config",
		edits: [][3]string{
			{"internal/config/config.go", "\tAuth\n}", "\tSecrets map[string]string `yaml:\",inline\"`\n\tAuth\n}"},
			{"internal/cli/wiring.go", `ConfigFields:    []string{"token", "client_secret"},`, `ConfigFields:    []string{"token", "client_secret", "Secrets"},`}},
		want: []string{"rule 3|config.go|config:Secrets,inline"}},
	{name: "rv9 fail: a mapstructure remain map in the config",
		edits: [][3]string{{"internal/config/config.go", "\tAuth\n}", "\tRest map[string]string `mapstructure:\",remain\"`\n\tAuth\n}"}},
		want:  []string{"rule 3|config.go|config:Rest,inline"}},

	// Round 3. Class opaque-scalars, unnamed: an unnamed struct gets the
	// decoding method of a type it embeds.
	{name: "rv9 fail: a pointer to an unnamed struct embedding big.Int",
		edits: [][3]string{
			{"internal/config/config.go", "\t\"encoding/json\"\n", "\t\"encoding/json\"\n\t\"math/big\"\n"},
			{"internal/config/config.go", "\tAuth\n}", "\tPIN *struct {\n\t\tbig.Int `json:\"token\"`\n\t} `json:\"pin\"`\n\tAuth\n}"}},
		want: []string{"rule 3|config.go|config:pin"}},

	// Class foreign-built-connector-types: another package that imports a
	// connector package can build its types.
	{name: "rv9 fail: a dependency builds a connector type",
		files: map[string]string{
			"model/model.go":               "package model\n\nimport \"net/http\"\n\ntype Box struct{ C *http.Client }\n",
			"third_party/httpx/go.mod":     "module example.com/httpx\n\ngo 1.26\n\nrequire acme-pp-cli v0.0.0\n",
			"third_party/httpx/makebox.go": "package httpx\n\nimport (\n\t\"net/http\"\n\n\t\"acme-pp-cli/model\"\n)\n\nfunc MakeBox() any { return &model.Box{C: &http.Client{}} }\n",
			caseFile:                       file("\t\"example.com/httpx\"\n\n\t\"acme-pp-cli/model\"", "func probe() {\n\tb := httpx.MakeBox().(*model.Box)\n\t_, _ = b.C.Get(\"https://outside.example\")\n}\n")[caseFile]},
		want: []string{"rule 2|example.com/httpx|imports the connector's package acme-pp-cli/model"}},
	{name: "rv9 pass: a connector package no dependency imports",
		files: map[string]string{
			"model/model.go": "package model\n\nimport \"net/http\"\n\ntype Box struct{ C *http.Client }\n",
			caseFile:         file("\t\"net/http\"\n\n\t\"acme-pp-cli/internal/credkit\"\n\t\"acme-pp-cli/model\"", "func probe() {\n\tb := &model.Box{C: &http.Client{Transport: credkit.Transport(nil)}}\n\t_, _ = b.C.Get(\"https://x\")\n}\n")[caseFile]}},
	// Pre-empted, class opaque-scalars, root: a decoded config type that
	// itself decodes (an embedded big.Int) is read whole, under no key.
	{name: "rv9 fail: a config type that decodes itself",
		edits: [][3]string{
			{"internal/config/config.go", "\t\"encoding/json\"\n", "\t\"encoding/json\"\n\t\"math/big\"\n"},
			{"internal/config/config.go", "\tAuth\n}", "\tbig.Int\n\tAuth\n}"}},
		want: []string{"rule 3|config.go|config type Config decodes itself"}},
	// Round 4. Class return-callers: a function whose value another package
	// can call has callers the call graph does not hold, so returning a
	// carrier from it is not proven by its visible callers.
	strict(tcase{name: "rv9 fail: a closure returning a carrier is also handed to another package",
		files: with(dep("run.go", "var _ = http.MethodGet\n\nfunc Run(value func() any) {\n\t_ = value().(interface{ Run() error }).Run()\n}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\nfunc probe(name string) {\n\tj := &job{exec.Command(name)}\n\tvalue := func() any { return j }\n\n\t_ = value()\n\thttpx.Run(value)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 fail: a named function returning a carrier is also handed to another package",
		files: with(dep("run.go", "var _ = http.MethodGet\n\nfunc Run(value func() any) {\n\t_ = value().(interface{ Run() error }).Run()\n}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\nfunc value() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = value()\n\thttpx.Run(value)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 pass: a closure returning a carrier only the connector calls",
		files: file("\t\"os/exec\"", "type job struct{ *exec.Cmd }\n\nfunc probe(name string) {\n\tj := &job{exec.Command(name)}\n\tvalue := func() any { return j }\n\tf := value\n\t_ = f()\n\t_ = value()\n}\n")}),
	strict(tcase{name: "rv9 fail: a method returning a carrier on a type handed to another package as an interface",
		files: with(dep("use.go", "var _ = http.MethodGet\n\nfunc Use(g interface{ Get() any }) {\n\t_ = g.Get().(interface{ Run() error }).Run()\n}\n\nfunc Feed2[T interface{ Get() any }]() {\n\tvar v T\n\t_ = v.Get().(interface{ Run() error }).Run()\n}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.Use(src{})\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 fail: a method returning a carrier on another package's generic type argument",
		files: with(dep("use.go", "var _ = http.MethodGet\n\nfunc Use(g interface{ Get() any }) {\n\t_ = g.Get().(interface{ Run() error }).Run()\n}\n\nfunc Feed2[T interface{ Get() any }]() {\n\tvar v T\n\t_ = v.Get().(interface{ Run() error }).Run()\n}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.Feed2[src]()\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	// Pre-empted: another package's generic type instantiated with a
	// connector type can call that type's methods from its own methods.
	strict(tcase{name: "rv9 fail: a method returning a carrier on another package's generic type's argument",
		files: with(dep("box.go", "var _ = http.MethodGet\n\ntype Box[T interface{ Get() any }] struct{ V T }\n\nfunc (b Box[T]) Go() { _ = b.V.Get().(interface{ Run() error }).Run() }\n\nfunc Take(r interface{ Go() }) { r.Go() }\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.Take(httpx.Box[src]{})\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	// Round 5. Class return-callers, generic function values: another
	// package's generic used as a value, not called.
	strict(tcase{name: "rv9 fail: a method returning a carrier on the type argument of another package's generic function value",
		files: with(dep("feedvalue.go", "var _ = http.MethodGet\n\nfunc Feed3[T interface{ Get(string) any }](name string) {\n\tvar v T\n\t_ = v.Get(name).(interface{ Run() error }).Run()\n}\n\nfunc Call(f func(string), name string) { f(name) }\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype source struct{}\n\nfunc (source) Get(name string) any { return &job{exec.Command(name)} }\n\nfunc probe(name string) {\n\t_ = source{}.Get(name)\n\thttpx.Call(httpx.Feed3[source], name)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	// Pre-empted: a method reached from another package without a type
	// argument of its own type: a bound method value or a method expression.
	strict(tcase{name: "rv9 fail: a method returning a carrier handed to another package as a bound method value",
		files: with(dep("runv.go", "var _ = http.MethodGet\n\nfunc RunV(value func() any) {\n\t_ = value().(interface{ Run() error }).Run()\n}\n\nfunc RunE[T any](f func(T) any) {\n\tvar t T\n\t_ = f(t).(interface{ Run() error }).Run()\n}\n\nfunc RunP(f func(*struct{}) any) {}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.RunV(src{}.Get)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 fail: a method returning a carrier handed to another package as a method expression",
		files: with(dep("runv.go", "var _ = http.MethodGet\n\nfunc RunV(value func() any) {\n\t_ = value().(interface{ Run() error }).Run()\n}\n\nfunc RunE[T any](f func(T) any) {\n\tvar t T\n\t_ = f(t).(interface{ Run() error }).Run()\n}\n\nfunc RunP(f func(*struct{}) any) {}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.RunE(src.Get)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 fail: a method returning a carrier handed to another package as a pointer method expression",
		files: with(dep("runv.go", "var _ = http.MethodGet\n\nfunc RunV(value func() any) {\n\t_ = value().(interface{ Run() error }).Run()\n}\n\nfunc RunE[T any](f func(T) any) {\n\tvar t T\n\t_ = f(t).(interface{ Run() error }).Run()\n}\n\nfunc RunP(f func(*struct{}) any) {}\n"),
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype src struct{}\n\nfunc (src) Get() any { return &job{exec.Command(\"x\")} }\n\nfunc probe() {\n\t_ = src{}.Get()\n\thttpx.RunE((*src).Get)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	// Round 6. Class return-callers, constituents: a type handed to another
	// package exposes the methods of every type inside it (reflection
	// reaches them), not only its own.
	strict(tcase{name: "rv9 fail: a method returning a carrier on the element of another package's generic type argument",
		files: with(map[string]string{"third_party/httpx/feed4.go": "package httpx\n\nimport \"reflect\"\n\nfunc Feed4[T any](name string) {\n\tv := reflect.Zero(reflect.TypeFor[T]().Elem())\n\tout := v.MethodByName(\"Get\").Call([]reflect.Value{reflect.ValueOf(name)})\n\t_ = out[0].Interface().(interface{ Run() error }).Run()\n}\n"},
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype source struct{}\n\nfunc (source) Get(name string) any { return &job{exec.Command(name)} }\n\nfunc probe(name string) {\n\t_ = source{}.Get(name)\n\thttpx.Feed4[[]source](name)\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
	strict(tcase{name: "rv9 fail: a method returning a carrier on a field of a value handed to another package as an interface",
		files: with(map[string]string{"third_party/httpx/use4.go": "package httpx\n\nimport \"reflect\"\n\nfunc Use4(x any) {\n\tv := reflect.ValueOf(x).Field(0)\n\tout := v.MethodByName(\"Get\").Call([]reflect.Value{reflect.ValueOf(\"x\")})\n\t_ = out[0].Interface().(interface{ Run() error }).Run()\n}\n"},
			caseFile, file("\t\"os/exec\"\n\n\t\"example.com/httpx\"", "type job struct{ *exec.Cmd }\n\ntype source struct{}\n\nfunc (source) Get(name string) any { return &job{exec.Command(name)} }\n\ntype holder struct{ S source }\n\nfunc probe(name string) {\n\t_ = source{}.Get(name)\n\thttpx.Use4(holder{})\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|returned by"}}),
}
