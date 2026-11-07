// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 1 and 5 for child processes: the argv and environment of one
// child is one unit, and a child gets the held set or is not started. A
// child inherits the environment without the declared secret variables; its
// argv and environment are then checked, and a child that would carry a
// held value is not started. The held set reaches the child through an
// inherited pipe named by CREDKIT_HELD, never through argv or environment,
// and a child built on the kit holds it in Start.
//
// The connector's own program is the one exception to "not started": its own
// binary, or its companion beside it (ownProgram). A variable that would
// carry a held value goes down that same pipe instead of into the child's
// environment, and Start in the child sets it again before the program reads
// it. So a credential the user set in the environment still reaches the CLI
// an MCP server shells out to, with no wiring, and the unit the child gets
// is still clean. Any other program is refused as before.

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	C       bool     `json:"c,omitempty"` // userCut: a Basic pair held with its user cut
	// E: not a held value, but the environment variable E the parent handed
	// over with this value (handOver). The values that make it one to hand
	// over come down as held records of their own.
	E string `json:"e,omitempty"`
}

// StartCommand starts cmd under the rules above. Use it (or RunCommand) in
// place of cmd.Start, cmd.Run, cmd.Output and cmd.CombinedOutput.
func StartCommand(cmd *exec.Cmd) error {
	own := ownProgram(cmd)
	env := cmd.Env
	if env == nil {
		env = childEnv()
		if own {
			env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, heldEnv+"=") })
		}
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
	if runtime.GOOS == "windows" {
		setCmdLine(cmd, argv[1])
	}
	e := emitBegin() // the barrier, to the start of the child (held.go heldMu)
	var moved []heldRecord
	unit := func() []byte { return []byte(strings.Join(argv, "\n") + "\n" + strings.Join(env, "\n")) }
	refuse := withheld(unit())
	if refuse && own {
		env, moved = handOver(env)
		refuse = withheld(unit())
	}
	cmd.Env = env
	if refuse {
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
	recs := append(heldRecords(), moved...)
	go func() {
		_ = json.NewEncoder(w).Encode(recs)
		_ = w.Close()
	}()
	return nil
}

// handOver takes out of env each variable that would be withheld on its own
// and returns what is left and a record for each one taken. CREDKIT_HELD
// stays, since it is how the child finds the records, and so does SYSTEMROOT,
// which a Windows child needs before any of its code runs.
func handOver(env []string) ([]string, []heldRecord) {
	var left []string
	var moved []heldRecord
	for _, kv := range env {
		name, v, ok := strings.Cut(kv, "=")
		if !ok || name == "" || name == heldEnv || strings.EqualFold(name, "SYSTEMROOT") || !withheld([]byte(kv)) {
			left = append(left, kv)
			continue
		}
		rec := heldRecord{B: []byte(v), E: name}
		if utf8.ValidString(v) {
			rec.V = v
		}
		moved = append(moved, rec)
	}
	return left, moved
}

// ownProgram reports whether cmd runs this connector's own program: this
// very binary, or the companion a connector's MCP server and CLI each are to
// the other, beside it. Only an absolute path counts: a relative one means
// whatever the working directory holds, and exec.Command leaves one only for
// a PATH that names a relative directory.
func ownProgram(cmd *exec.Cmd) bool {
	if cmd.Err != nil || !filepath.IsAbs(cmd.Path) {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return sameProgram(exe, cmd.Path, runtime.GOOS, runtime.GOARCH)
}

// sameProgram is ownProgram's test, every name judged by the file it
// resolves to, so a symbolic link is what it runs. A companion is in the same
// directory as exe, and only the -mcp and -cli of one name differ: the
// fleet's two programs are <x>-mcp and <x>-cli, the release lays them out
// side by side, and cli_path.go looks for the CLI there first
// (tools/maintainer/mcpb_bundle.py). A file of that name anywhere else, a
// PATH entry included, could be anything, so it is a different program.
func sameProgram(exe, path, goos, goarch string) bool {
	re, err1 := filepath.EvalSymlinks(exe)
	rp, err2 := filepath.EvalSymlinks(path)
	if err1 != nil || err2 != nil {
		return false
	}
	fe, err1 := os.Stat(re)
	fp, err2 := os.Stat(rp)
	if err1 != nil || err2 != nil || !fp.Mode().IsRegular() {
		return false
	}
	if os.SameFile(fe, fp) {
		return true
	}
	de, err1 := os.Stat(filepath.Dir(re))
	dp, err2 := os.Stat(filepath.Dir(rp))
	if err1 != nil || err2 != nil || !os.SameFile(de, dp) {
		return false
	}
	se, roleE := programName(filepath.Base(re), goos, goarch)
	sp, roleP := programName(filepath.Base(rp), goos, goarch)
	return se != "" && se == sp && roleE != roleP
}

// programName splits a file name into a connector's name and its role, mcp
// or cli; both are "" for any other name. It takes off what a release adds:
// .exe on Windows, and the -<goos> or -<goos>-<goarch> of a bundle's thin
// binaries (cli_path.go siblingCLICandidates). Windows names ignore case.
func programName(name, goos, goarch string) (stem, role string) {
	if goos == "windows" {
		name = strings.ToLower(name)
		name = strings.TrimSuffix(name, ".exe")
	}
	name = strings.TrimSuffix(name, "-"+goos+"-"+goarch)
	name = strings.TrimSuffix(name, "-"+goos)
	for _, r := range []string{"mcp", "cli"} {
		if s, ok := strings.CutSuffix(name, "-"+r); ok && s != "" {
			return s, r
		}
	}
	return "", ""
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

// childEnv is the environment without the declared secret and URL variables,
// a proxy variable that names a user or password, and this process's own
// CREDKIT_HELD.
func childEnv() []string {
	reg.Lock()
	secret := slices.Concat(reg.decl.SecretEnv, reg.decl.URLEnv)
	reg.Unlock()
	var out []string
	for _, kv := range os.Environ() {
		name, v, _ := strings.Cut(kv, "=")
		drop := name == heldEnv || isProxyEnv(name) && proxyHasUserinfo(v)
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
		r := heldRecord{B: []byte(v), A: e.allowed, C: e.userCut}
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
		if r.E != "" {
			_ = os.Setenv(r.E, v) // handOver: a name it refuses never reached the program either
			continue
		}
		if r.Unknown || v == "" {
			reg.unknown = true
			reg.gen++
			continue
		}
		e := reg.vals[v]
		if e == nil {
			e = &heldEntry{userCut: r.C}
			reg.vals[v] = e
		}
		e.userCut = e.userCut && r.C // held whole from either side: whole
		e.allowed = append(e.allowed, r.A...)
		reg.gen++
	}
}
