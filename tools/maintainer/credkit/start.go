// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1's timing and contract 4's install rule: the startup prologue.
// Start is the first statement of every main. It installs the output layer
// (or exits before writing anything), then holds every credential known at
// startup before any output is checked: the held set a parent passed down,
// the declared secret environment variables, every credential file
// (Declarations.Files and a --config path on the command line), the base
// URL's secrets and those of Declarations.URLEnv, a proxy variable's user and
// password, the values of declared credential flags (pre-scanned from
// argv, so a flag-parse error that echoes one is withheld), and stdin when
// a declared stdin flag is present and stdin is not a terminal. Then it
// returns and the program parses its flags as before.
//
//	func main() {
//		credkit.Start(declarations) // the wiring file's one Declarations literal
//		code := run()
//		credkit.Exit(code)
//	}
//
// A credential read from a terminal (a password prompt) cannot be read
// ahead: the program holds it with Hold("stdin", v) as soon as it reads it.

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
)

// started is closed when Start has held every startup credential; the
// stdout and stderr pumps check nothing before it.
var (
	started     = make(chan struct{})
	startedOnce sync.Once
)

// Start runs the startup prologue with the connector's declarations.
func Start(d Declarations) {
	if err := d.validate(); err != nil {
		fatal()
		return
	}
	reg.Lock()
	reg.decl = d
	reg.Unlock()
	if err := installStdio(); err != nil {
		fatal()
		return
	}
	reg.Lock()
	reg.startup = true
	reg.Unlock()
	gather(d, os.Args[1:])
	reg.Lock()
	src, refused := startupCollisionLocked()
	reg.startup = false
	reg.Unlock()
	currentMatcher() // built here, so the first unit is not held up building it
	startedOnce.Do(func() { close(started) })
	retargetLog()
	if refused {
		refuse(src)
	}
}

// gather holds every credential known at startup.
func gather(d Declarations, args []string) {
	readInherited()
	for _, name := range d.SecretEnv {
		if v, ok := os.LookupEnv(name); ok && !unfilledPlaceholder(v) {
			Hold("env:"+name, v)
		}
	}
	var files []string
	if d.Files != nil {
		files = d.Files(args)
	}
	base := ""
	for _, f := range files {
		if b := gatherFile(f); base == "" {
			base = b
		}
	}
	// An explicit --config is the file the program reads, the last of several
	// as a flag parser takes them: its base URL, or none, decides, so a
	// discovered default's host does not stay an allowed origin. Every file is
	// held, FlagValues' wider reading included; the choice stops at the first
	// "--", since a wrong choice here refuses the program's real sends.
	for _, f := range FlagValues(args, "--config") {
		gatherFile(f)
	}
	if n := len(flagValues(beforeEnd(args), "--config")); n > 0 {
		// What stands before the first "--" is a prefix of FlagValues, so this
		// is the last of those. An empty value is the flag unset.
		if f := FlagValues(args, "--config")[n-1]; f != "" {
			base = gatherFile(f)
		}
	}
	reg.Lock()
	if d.BaseURL.Env != "" {
		if v := os.Getenv(d.BaseURL.Env); v != "" && !unfilledPlaceholder(v) {
			holdBaseURLLocked(v)
			base = v
		}
	}
	if d.BaseURL.Default != "" {
		holdBaseURLLocked(d.BaseURL.Default)
	}
	if base == "" {
		base = d.BaseURL.Default
	}
	reg.baseURL = base
	for _, f := range d.CredentialFlags {
		holdLocked("flag:"+f, flagValues(args, f)...)
	}
	for _, name := range d.URLEnv {
		if v := os.Getenv(name); v != "" && !unfilledPlaceholder(v) {
			holdBaseURLLocked(v)
		}
	}
	reg.Unlock()
	holdProxyEnv()
	for _, f := range d.StdinFlags {
		if present(args, f) {
			readStdin()
			break
		}
	}
}

// proxyEnv is every variable a proxy with a password can be set in. Go's
// transport reads the first four; curl and others read the last two.
var proxyEnv = []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"}

func isProxyEnv(name string) bool {
	return slices.ContainsFunc(proxyEnv, func(p string) bool { return strings.EqualFold(p, name) })
}

// parseProxyEnv reads v as Go's proxy lookup does (golang.org/x/net/http/httpproxy):
// a URL, or, when that gives no scheme and host, the same text after "http://",
// so ops:secret@proxy.example:3128 is a user and password, not a scheme. It is
// an error when that second reading loses a user or password v has.
func parseProxyEnv(v string) (*url.URL, error) {
	if u, err := url.Parse(v); err == nil && u.Scheme != "" && u.Host != "" {
		return u, nil
	}
	u, err := url.Parse("http://" + v)
	if err == nil && u.User == nil && strings.Contains(v, "@") {
		return nil, errors.New("credkit: a proxy setting that does not parse")
	}
	return u, err
}

// rawUserinfo is the user and password written in a proxy setting no parser
// reads, which still gives them up.
func rawUserinfo(v string) (user, pass string, ok bool) {
	if _, rest, found := strings.Cut(v, "://"); found {
		v = rest
	}
	v = strings.TrimLeft(v, "/") // a scheme-relative //user:pass@host
	i := strings.LastIndexByte(v, '@')
	if i < 0 {
		return "", "", false
	}
	user, pass, _ = strings.Cut(v[:i], ":")
	return user, pass, true
}

// proxyHasUserinfo reports whether a proxy setting names a user or password.
func proxyHasUserinfo(v string) bool {
	if unfilledPlaceholder(v) {
		return false
	}
	if u, err := parseProxyEnv(v); err == nil {
		return u.User != nil
	}
	_, _, ok := rawUserinfo(v)
	return ok
}

// holdProxyEnv holds the user and password of each proxy variable from
// startup, whether or not a request ever uses the proxy: they are bound to that
// proxy's origin, as the transport binds them (holdProxyCreds).
func holdProxyEnv() {
	for _, name := range proxyEnv {
		v := os.Getenv(name)
		if v == "" || unfilledPlaceholder(v) {
			continue
		}
		if u, err := parseProxyEnv(v); err == nil {
			if u.User != nil {
				holdProxyCreds(u, nil)
			}
			continue
		}
		if user, pass, ok := rawUserinfo(v); ok {
			reg.Lock()
			holdAllowedLocked("proxy-user", nil, user)
			holdAllowedLocked("proxy-userinfo", nil, pass)
			if u, err := url.PathUnescape(user); err == nil {
				holdAllowedLocked("proxy-user", nil, u)
			}
			if p, err := url.PathUnescape(pass); err == nil {
				holdAllowedLocked("proxy-userinfo", nil, p)
			}
			reg.Unlock()
		}
	}
}

// unfilledPlaceholder reports whether v is the text an MCPB host leaves in an
// environment variable when the user left that optional setting empty. It is
// no value anyone gave the program, and the connector reads it as unset
// (cliutil.EnvOverride). Held, its pieces (the variable's own name, so the
// connector's) would withhold nearly everything the program says.
func unfilledPlaceholder(v string) bool {
	rest, ok := strings.CutPrefix(v, "${user_config.")
	return ok && strings.IndexByte(rest, '}') == len(rest)-1 && len(rest) > 1
}

// flagValues is every value given to flag on the command line, in each
// spelling a flag parser accepts: the name with one dash or two (Go's flag
// package reads either; pflag reads the declared one), "--flag v" and
// "--flag=v", and for a one-letter flag "-f v", "-f=v", "-fv" and the same
// inside a cluster of one-letter flags ("-vf v", "-vfv", "-vf=v"). Every
// place the letter stands in a cluster gives a value, so one that was
// another flag's value is held too.
func flagValues(args []string, flag string) []string {
	var out []string
	for i, a := range args {
		name, v, eq := strings.Cut(a, "=")
		switch {
		case isFlag(name, flag) && eq:
			out = append(out, v)
		case isFlag(a, flag):
			if i+1 < len(args) {
				out = append(out, args[i+1])
			}
		case isShort(flag) && isCluster(a):
			for j := 1; j < len(a); j++ {
				switch {
				case a[j] != flag[1]:
				case j+1 < len(a):
					out = append(out, strings.TrimPrefix(a[j+1:], "="))
				case i+1 < len(args):
					out = append(out, args[i+1])
				}
			}
		}
	}
	return out
}

// FlagValues is every value the program's flag parser could give the long
// flag ("--config") on the command line args, up to the "--" that ends the
// flags. The kit finds --config files with it, and a connector's
// Declarations.Files should too: tools/maintainer/mcp_file_flags reads a
// call with a constant flag name as that flag's value, so a file it names is
// tied to the flag the MCP server refuses. An argv scan of the connector's
// own is os.Args reaching a file, and fails that check.
func FlagValues(args []string, flag string) []string {
	return flagValues(flagArgs(args), flag)
}

// flagArgs is args up to the "--" that ends the flags Cobra parses: a
// --config after it is a positional the program never opens, so the kit
// does not open it either (a pipe there hung startup). Only file reads use
// it; credential flags are held from every argument, which holds more.
func flagArgs(args []string) []string {
	for i, a := range args {
		if a == "--" && (i == 0 || !mayTakeValue(args[i-1])) {
			return args[:i]
		}
	}
	return args
}

// mayTakeValue reports whether a could be a flag whose value is the next
// argument (pflag reads `--name --` as name="--"). The kit does not know
// which flags take values, so any flag without "=" may: a "--" after one
// does not end the scan, which then reads more, never less.
func mayTakeValue(a string) bool {
	return len(a) > 1 && a[0] == '-' && a != "--" && !strings.Contains(a, "=")
}

// beforeEnd is args up to the first "--".
func beforeEnd(args []string) []string {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i]
	}
	return args
}

// isFlag reports whether a names flag, with one dash or two.
func isFlag(a, flag string) bool {
	n := strings.TrimLeft(flag, "-")
	return n != "" && (a == "-"+n || a == "--"+n)
}

func isShort(flag string) bool { return len(flag) == 2 && flag[0] == '-' && flag[1] != '-' }

func isCluster(a string) bool { return len(a) > 2 && a[0] == '-' && a[1] != '-' }

func present(args []string, flag string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if isFlag(name, flag) || isShort(flag) && isCluster(a) && strings.IndexByte(a[1:], flag[1]) >= 0 {
			return true
		}
	}
	return false
}

// readStdin reads all of a non-terminal stdin, holds it whole and line by
// line, and gives the program the same bytes on os.Stdin. An unreadable
// stdin makes every value unknown (fail closed).
func readStdin() {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		markUnknown()
		return
	}
	vals := []string{string(b)}
	for _, l := range bytes.Split(b, []byte("\n")) {
		vals = append(vals, string(l))
	}
	Hold("stdin", vals...)
	r, w, err := os.Pipe()
	if err != nil {
		markUnknown()
		return
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	os.Stdin = r
}
