// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 1 and 5 for child processes: the argv and environment of one
// child is one unit, and a child gets the held set or is not started. A
// child inherits the environment without the declared secret variables; its
// argv and environment are then checked, and a child that would carry a
// held value is not started. The held set reaches the child through an
// inherited pipe named by CREDKIT_HELD, never through argv or environment,
// and a child built on the kit holds it in Start.

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"
)

const heldEnv = "CREDKIT_HELD"

// V is the value when it is valid UTF-8 (JSON would rewrite any other
// byte as U+FFFD); B is the value byte for byte, always written and read
// first. A record with neither makes the child's set unknown.
type heldRecord struct {
	B       []byte   `json:"b,omitempty"`
	V       string   `json:"v,omitempty"`
	A       []string `json:"a,omitempty"`
	Unknown bool     `json:"unknown,omitempty"`
}

// StartCommand starts cmd under the rules above. Use it (or RunCommand) in
// place of cmd.Start, cmd.Run, cmd.Output and cmd.CombinedOutput.
func StartCommand(cmd *exec.Cmd) error {
	env := cmd.Env
	if env == nil {
		env = childEnv()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	// The unit is what the child gets: its argv (on Windows, the command
	// line) and its final environment, the kit's own addition and what
	// os/exec would add included. The kit sets both on cmd before the
	// check, so os/exec adds nothing after it.
	env = append(slices.Clip(env), heldEnv+"="+attachHeld(cmd, r))
	argv, env := childEnvelope(cmd, env, runtime.GOOS)
	cmd.Env = env
	if runtime.GOOS == "windows" {
		setCmdLine(cmd, argv[1])
	}
	e := emitBegin() // the barrier, to the start of the child (held.go heldMu)
	if withheld([]byte(strings.Join(argv, "\n") + "\n" + strings.Join(env, "\n"))) {
		emitEnd(e)
		_ = r.Close()
		_ = w.Close()
		return withheldError{SinkChild}
	}
	err = cmd.Start()
	emitEnd(e)
	if err != nil {
		_ = r.Close()
		_ = w.Close()
		return err
	}
	_ = r.Close()
	recs := heldRecords()
	go func() {
		_ = json.NewEncoder(w).Encode(recs)
		_ = w.Close()
	}()
	return nil
}

// childEnvelope is the argv and environment the child gets on goos, from
// cmd and the environment env, as os/exec and syscall build them: the
// environment with each variable's last setting kept (its name compared
// without case on Windows), SYSTEMROOT added on Windows when it is not
// set, and on Windows the program and one command line (SysProcAttr's, or
// the arguments quoted as syscall quotes them) in place of argv.
func childEnvelope(cmd *exec.Cmd, env []string, goos string) ([]string, []string) {
	env = dedupEnv(goos == "windows", env)
	if goos != "windows" {
		return append([]string{cmd.Path}, cmd.Args...), env
	}
	if !slices.ContainsFunc(env, func(kv string) bool {
		k, _, ok := strings.Cut(kv, "=")
		return ok && strings.EqualFold(k, "SYSTEMROOT")
	}) {
		env = append(env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}
	line := cmdLine(cmd)
	if line == nil {
		line = []string{makeCmdLine(cmd.Args)}
	}
	return []string{cmd.Path, line[0]}, env
}

// dedupEnv is os/exec's dedupEnvCase: the last setting of each name wins,
// order kept; an entry with a NUL byte is dropped (os/exec then refuses to
// start the child).
func dedupEnv(caseInsensitive bool, env []string) []string {
	out := make([]string, 0, len(env))
	saw := map[string]bool{}
	for n := len(env); n > 0; n-- {
		kv := env[n-1]
		if strings.IndexByte(kv, 0) >= 0 {
			continue
		}
		i := strings.Index(kv, "=")
		if i == 0 {
			i = strings.Index(kv[1:], "=") + 1
		}
		if i < 0 {
			if kv != "" {
				out = append(out, kv)
			}
			continue
		}
		k := kv[:i]
		if caseInsensitive {
			k = strings.ToLower(k)
		}
		if !saw[k] {
			saw[k] = true
			out = append(out, kv)
		}
	}
	slices.Reverse(out)
	return out
}

// makeCmdLine is syscall's on Windows: the arguments escaped and joined
// with spaces.
func makeCmdLine(args []string) string {
	var b []byte
	for _, v := range args {
		if len(b) > 0 {
			b = append(b, ' ')
		}
		b = appendEscapeArg(b, v)
	}
	return string(b)
}

// appendEscapeArg is syscall's on Windows.
func appendEscapeArg(b []byte, s string) []byte {
	if len(s) == 0 {
		return append(b, `""`...)
	}
	needsBackslash, hasSpace := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}
	if !needsBackslash && !hasSpace {
		return append(b, s...)
	}
	if !needsBackslash {
		return append(append(append(b, '"'), s...), '"')
	}
	if hasSpace {
		b = append(b, '"')
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		default:
			slashes = 0
		case '\\':
			slashes++
		case '"':
			for ; slashes > 0; slashes-- {
				b = append(b, '\\')
			}
			b = append(b, '\\')
		}
		b = append(b, c)
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b = append(b, '\\')
		}
		b = append(b, '"')
	}
	return b
}

// RunCommand is StartCommand, then cmd.Wait.
func RunCommand(cmd *exec.Cmd) error {
	if err := StartCommand(cmd); err != nil {
		return err
	}
	return cmd.Wait()
}

// childEnv is the environment without the declared secret variables and
// without this process's own CREDKIT_HELD.
func childEnv() []string {
	reg.Lock()
	secret := reg.decl.SecretEnv
	reg.Unlock()
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		drop := name == heldEnv
		for _, s := range secret {
			if name == s || runtime.GOOS == "windows" && strings.EqualFold(name, s) {
				drop = true
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

func heldRecords() []heldRecord {
	reg.Lock()
	defer reg.Unlock()
	if reg.unknown {
		return []heldRecord{{Unknown: true}}
	}
	out := make([]heldRecord, 0, len(reg.vals))
	for v, e := range reg.vals {
		r := heldRecord{B: []byte(v), A: e.allowed}
		if utf8.ValidString(v) {
			r.V = v
		}
		out = append(out, r)
	}
	return out
}

// readInherited holds the set a parent built on the kit passed down. A
// named pipe that cannot be read makes every value unknown (fail closed).
func readInherited() {
	spec, ok := os.LookupEnv(heldEnv)
	if !ok {
		return
	}
	os.Unsetenv(heldEnv)
	f := openHeld(spec)
	if f == nil {
		markUnknown()
		return
	}
	defer f.Close()
	holdInherited(f)
}

// holdInherited holds the records a parent wrote to r (heldRecords).
func holdInherited(r io.Reader) {
	var recs []heldRecord
	if err := json.NewDecoder(r).Decode(&recs); err != nil {
		markUnknown()
		return
	}
	reg.Lock()
	defer reg.Unlock()
	for _, r := range recs {
		v := r.V
		if r.B != nil {
			v = string(r.B)
		}
		if r.Unknown || v == "" {
			reg.unknown = true
			reg.gen++
			continue
		}
		e := reg.vals[v]
		if e == nil {
			e = &heldEntry{}
			reg.vals[v] = e
		}
		e.allowed = append(e.allowed, r.A...)
		reg.gen++
	}
}
