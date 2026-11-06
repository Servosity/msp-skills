// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

// Package winsrv holds the fixed PowerShell checks the CLI runs on a Windows
// server over WinRM, the profiles that say which servers exist, and the
// post-processing that turns script output into rows.
//
// Security model: the only scripts that can run are the constant scripts
// embedded here. Every argument is validated against a strict per-script
// schema, marshalled to JSON, and handed to the script through the WSPP_ARGS
// shell environment variable, where the script parses it as data and binds it
// to cmdlet parameters. No value is ever concatenated into script text, so the
// encoded command for a script is identical on every call.
package winsrv

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

//go:embed scripts/*.ps1
var scriptFS embed.FS

// ArgsEnv is the shell environment variable that carries the JSON arguments,
// base64 encoded: WinRS hands an environment value to the shell without
// decoding XML character references, so a raw quote would arrive as &#34;.
const ArgsEnv = "WSPP_ARGS"

// Param is one validated script argument.
type Param struct {
	Name string
	Kind string // "enum", "int", "bool", "name", "levels"
	Enum []string
	Min  int
	Max  int
}

// Script is one registered check or action.
type Script struct {
	Name     string
	Parts    []string // embedded files; the last is the body
	Params   []Param
	Mutating bool          // changes server state; needs a confirmed context
	Timeout  time.Duration // default per-run budget
}

var serviceNameRE = regexp.MustCompile(`^[A-Za-z0-9_.$-]{1,80}$`)

var registry = map[string]Script{}

func register(s Script) {
	if s.Timeout == 0 {
		s.Timeout = 90 * time.Second
	}
	registry[s.Name] = s
}

func init() {
	register(Script{Name: "system", Parts: []string{"prelude", "system"}})
	register(Script{Name: "volumes", Parts: []string{"prelude", "volumes"}})
	register(Script{Name: "services", Parts: []string{"prelude", "lib_services", "services"}})
	register(Script{Name: "service_get", Parts: []string{"prelude", "lib_services", "service_get"}, Params: []Param{{Name: "name", Kind: "name"}}})
	register(Script{Name: "eventlog", Parts: []string{"prelude", "eventlog"}, Params: []Param{
		{Name: "log", Kind: "enum", Enum: []string{"System", "Application"}},
		{Name: "hours", Kind: "int", Min: 1, Max: 720},
		{Name: "levels", Kind: "levels"},
		{Name: "max", Kind: "int", Min: 1, Max: 2000},
	}})
	register(Script{Name: "scm_events", Parts: []string{"prelude", "scm_events"}, Params: []Param{{Name: "days", Kind: "int", Min: 1, Max: 30}, {Name: "max", Kind: "int", Min: 1, Max: 2000}}})
	register(Script{Name: "installed_updates", Parts: []string{"prelude", "installed_updates"}})
	register(Script{Name: "pending_updates", Parts: []string{"prelude", "pending_updates"}, Timeout: 5 * time.Minute})
	register(Script{Name: "features", Parts: []string{"prelude", "features"}, Timeout: 3 * time.Minute})
	register(Script{Name: "local_admins", Parts: []string{"prelude", "lib_admins", "local_admins"}})
	register(Script{Name: "local_admin_changes", Parts: []string{"prelude", "lib_admins", "local_admin_changes"}, Params: []Param{{Name: "hours", Kind: "int", Min: 1, Max: 720}}})
	register(Script{Name: "shares", Parts: []string{"prelude", "shares"}})
	register(Script{Name: "scheduled_tasks", Parts: []string{"prelude", "scheduled_tasks"}, Params: []Param{{Name: "include_microsoft", Kind: "bool"}}, Timeout: 3 * time.Minute})
	register(Script{Name: "certificates", Parts: []string{"prelude", "certificates"}, Params: []Param{{Name: "store", Kind: "enum", Enum: []string{"My", "WebHosting", "Remote Desktop", "CA", "Root"}}}})
	register(Script{Name: "defender", Parts: []string{"prelude", "defender"}})
	register(Script{Name: "time_sync", Parts: []string{"prelude", "time_sync"}})
	register(Script{Name: "hyperv_vms", Parts: []string{"prelude", "hyperv_vms"}})
	register(Script{Name: "eventlog_groups", Parts: []string{"prelude", "eventlog_groups"}, Params: []Param{
		{Name: "window_hours", Kind: "int", Min: 1, Max: 168},
		{Name: "baseline_hours", Kind: "int", Min: 1, Max: 720},
	}, Timeout: 3 * time.Minute})
	actionParams := []Param{{Name: "name", Kind: "name"}, {Name: "action", Kind: "enum", Enum: []string{"restart", "start"}}}
	register(Script{Name: "service_plan", Parts: []string{"prelude", "lib_service_action", "service_plan"}, Params: actionParams})
	register(Script{Name: "service_action", Parts: []string{"prelude", "lib_service_action", "service_action"}, Params: actionParams, Mutating: true, Timeout: 2 * time.Minute})
}

// Lookup returns a registered script.
func Lookup(name string) (Script, bool) {
	s, ok := registry[name]
	return s, ok
}

// Names returns every registered script name, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Validate checks args against the script's schema and returns the typed
// values to send. Unknown keys, wrong types and out-of-range values are
// refused; nothing is coerced into a different meaning.
func (s Script) Validate(args map[string]any) (map[string]any, error) {
	known := map[string]Param{}
	for _, p := range s.Params {
		known[p.Name] = p
	}
	out := map[string]any{}
	for k, v := range args {
		p, ok := known[k]
		if !ok {
			return nil, fmt.Errorf("%s: unknown argument %q", s.Name, k)
		}
		val, err := p.check(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		out[k] = val
	}
	return out, nil
}

func (p Param) check(v any) (any, error) {
	switch p.Kind {
	case "name":
		s, ok := v.(string)
		if !ok || !serviceNameRE.MatchString(s) {
			return nil, fmt.Errorf("%s must be a service name (letters, digits, _ . $ -, at most 80)", p.Name)
		}
		return s, nil
	case "enum":
		s, ok := v.(string)
		if ok {
			for _, e := range p.Enum {
				if s == e {
					return s, nil
				}
			}
		}
		return nil, fmt.Errorf("%s must be one of %s", p.Name, strings.Join(p.Enum, ", "))
	case "int":
		n, ok := asInt(v)
		if !ok || n < p.Min || n > p.Max {
			return nil, fmt.Errorf("%s must be a whole number from %d to %d", p.Name, p.Min, p.Max)
		}
		return n, nil
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s must be true or false", p.Name)
		}
		return b, nil
	case "levels":
		list, ok := v.([]int)
		if !ok || len(list) == 0 || len(list) > 4 {
			return nil, fmt.Errorf("%s must list event levels 1 to 4", p.Name)
		}
		for _, n := range list {
			if n < 1 || n > 4 {
				return nil, fmt.Errorf("%s must list event levels 1 to 4", p.Name)
			}
		}
		return list, nil
	}
	return nil, fmt.Errorf("%s: unsupported parameter kind %q", p.Name, p.Kind)
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), int64(int(n)) == n
	}
	return 0, false
}

// Assemble returns the full constant script text for a registered script.
func Assemble(name string) (string, error) {
	s, ok := registry[name]
	if !ok {
		return "", fmt.Errorf("unknown script %q", name)
	}
	var b strings.Builder
	for i, part := range s.Parts {
		raw, err := scriptFS.ReadFile("scripts/" + part + ".ps1")
		if err != nil {
			return "", fmt.Errorf("script part %s: %w", part, err)
		}
		code := minify(string(raw))
		if i == len(s.Parts)-1 {
			b.WriteString("try {\n" + code + "\n} catch { Fail $_ }\n")
		} else {
			b.WriteString(code + "\n")
		}
	}
	return b.String(), nil
}

// ScriptEnv is the shell environment variable that carries the script text
// (base64 of its UTF-8 bytes). WinRS refuses a command line longer than 8191
// characters, and the encoded scripts are longer, so the command line is a
// short fixed bootstrap instead.
const ScriptEnv = "WSPP_SCRIPT"

// bootstrap runs the script in WSPP_SCRIPT only when its SHA-256 matches the
// one embedded here, so the command line pins exactly one constant script:
// a missing, altered or different script is refused before any of it runs.
const bootstrap = `$ErrorActionPreference='Stop'
try{$b=[Convert]::FromBase64String([string]$env:WSPP_SCRIPT)
$h=[BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($b)).Replace('-','')}catch{$h=''}
if($h -cne 'HASH'){[Console]::Out.Write('{"ok":false,"error":{"kind":"refused","message":"script integrity check failed"}}');exit 0}
& ([ScriptBlock]::Create([Text.Encoding]::UTF8.GetString($b)))
`

// CommandLine returns the executable, arguments and script payload for a
// registered script. All three are constant for a given script name; no
// argument value ever reaches them.
func CommandLine(name string) (exe string, argv []string, script string, err error) {
	text, err := Assemble(name)
	if err != nil {
		return "", nil, "", err
	}
	sum := sha256.Sum256([]byte(text))
	boot := strings.Replace(bootstrap, "HASH", strings.ToUpper(hex.EncodeToString(sum[:])), 1)
	return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", Encode(boot)},
		base64.StdEncoding.EncodeToString([]byte(text)), nil
}

// minify drops full-line comments, blank lines and indentation to keep the
// script payload small. The scripts use no here-strings or multi-line
// strings (a test enforces it), so no string spans a dropped line.
func minify(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		b.WriteString(t)
		b.WriteByte('\n')
	}
	return b.String()
}

// Encode returns the base64 UTF-16LE form PowerShell expects for -EncodedCommand.
func Encode(script string) string {
	u := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = binary.LittleEndian.AppendUint16(buf, c)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
