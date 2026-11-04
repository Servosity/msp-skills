// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The first adopters' class A (axcient kit-findings item 1): a connector's
// MCP server shells out to its own companion CLI with cmd.Env set to its
// whole environment, and a user's API key is in that environment. The kit
// refused the child, so every shell-out tool answered "withheld from child
// process" for every user who authenticates by environment.
//
// The tests run as copies of the test binary named like a connector's two
// programs, so that a process's own name and directory are real.

const fix1Val = "fix1-env-" + "0123456789abcdef"

// fix1Copy copies the test binary to dir/name (with .exe on Windows).
func fix1Copy(t *testing.T, dir, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// fix1Report is what a parent copy says about the child it tried to start.
type fix1Report struct {
	Refused bool   // StartCommand returned ErrWithheld
	Other   string // any other error
	Child   string // what the child printed
}

// fix1Parent starts parent, which starts child (resolved as the case says),
// with the secret in the parent's environment.
func fix1Parent(t *testing.T, parent, how, child, path string, extra ...string) fix1Report {
	t.Helper()
	cmd := exec.Command(parent, "-test.run=^$")
	cmd.Env = append(os.Environ(), childModeEnv+"=fix1-parent", "FIX1_HOW="+how, "FIX1_CHILD="+child, "CK_TOKEN="+fix1Val, "FIX1_ARGV="+strings.Join(extra, "\x1f"))
	if path != "" {
		cmd.Env = append(cmd.Env, "PATH="+path)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("parent: %v\n%s", err, out)
	}
	var r fix1Report
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("parent said %q: %v", out, err)
	}
	return r
}

// fix1ParentMain is the parent copy: it holds the declared secret as Start
// would, and starts the child, with an explicit whole environment (as the
// press's mcpChildEnviron does) unless told otherwise.
func fix1ParentMain() {
	d := Declarations{SecretEnv: []string{"CK_TOKEN"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}}
	resetForTest(d)
	Hold("env:CK_TOKEN", os.Getenv("CK_TOKEN"))
	var args []string
	if a := os.Getenv("FIX1_ARGV"); a != "" {
		args = strings.Split(a, "\x1f")
	}
	child := os.Getenv("FIX1_CHILD")
	how := os.Getenv("FIX1_HOW")
	if how == "self" {
		child, _ = os.Executable()
	}
	cmd := exec.Command(child, args...)
	if how != "nil-env" {
		cmd.Env = append(os.Environ(), childModeEnv+"=fix1-child")
	} else {
		os.Setenv(childModeEnv, "fix1-child")
	}
	var buf strings.Builder
	cmd.Stdout = &buf
	err := RunCommand(cmd)
	r := fix1Report{Refused: errors.Is(err, ErrWithheld), Child: buf.String()}
	if err != nil && !r.Refused {
		r.Other = err.Error()
	}
	b, _ := json.Marshal(r)
	fmt.Println(string(b))
}

// fix1ChildMain is the child copy: what its environment held when it
// started, then what Start gives the program.
func fix1ChildMain() {
	before := os.Getenv("CK_TOKEN")
	readInherited()
	fmt.Printf("before=%v after=%v held=%v", before == fix1Val, os.Getenv("CK_TOKEN") == fix1Val, isHeld(fix1Val))
}

const fix1Handed = "before=false after=true held=true"

func TestFix1ACompanionGetsTheKeyByHandOver(t *testing.T) {
	if testing.Short() {
		t.Skip("copies the test binary")
	}
	dir := t.TempDir()
	mcp, cli := fix1Copy(t, dir, "acme-mcp"), fix1Copy(t, dir, "acme-cli")
	for name, r := range map[string]fix1Report{
		"its own binary":              fix1Parent(t, mcp, "self", "", ""),
		"its companion, by path":      fix1Parent(t, mcp, "path", cli, ""),
		"its companion, through PATH": fix1Parent(t, mcp, "path", filepath.Base(cli), dir),
		"the CLI's server":            fix1Parent(t, cli, "path", mcp, ""),
		"its companion, Env left nil": fix1Parent(t, mcp, "nil-env", cli, ""),
	} {
		if r.Refused || r.Other != "" || r.Child != fix1Handed {
			t.Errorf("%s: %+v, want the child to run with the key handed over and none in its environment", name, r)
		}
	}
}

func TestFix1ADifferentProgramIsStillRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("copies the test binary")
	}
	dir, other := t.TempDir(), t.TempDir()
	mcp := fix1Copy(t, dir, "acme-mcp")
	cases := map[string]fix1Report{
		"a same-named program elsewhere, through PATH": fix1Parent(t, mcp, "path", filepath.Base(fix1Copy(t, other, "acme-cli")), other),
		"a same-named program elsewhere, by path":      fix1Parent(t, mcp, "path", filepath.Join(other, filepath.Base(fix1Copy(t, other, "acme-cli"))), ""),
		"a sibling of another connector":               fix1Parent(t, mcp, "path", fix1Copy(t, dir, "other-cli"), ""),
		"a sibling with another role":                  fix1Parent(t, mcp, "path", fix1Copy(t, dir, "acme-tool"), ""),
		"a name that only starts like it":              fix1Parent(t, mcp, "path", fix1Copy(t, dir, "acme-mcp-two"), ""),
		"another of its own role beside it":            fix1Parent(t, mcp, "path", fix1Copy(t, dir, "acme-mcp-"+runtime.GOOS), ""),
	}
	for name, r := range cases {
		if !r.Refused || r.Child != "" {
			t.Errorf("%s: %+v, want it refused and not started", name, r)
		}
	}
	// Its own binary still does not take a credential in argv: argv cannot
	// be handed over.
	if r := fix1Parent(t, mcp, "self", "", "", "-test.run=^$", fix1Val); !r.Refused {
		t.Errorf("a credential in its own binary's argv: %+v, want refused", r)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// A symbolic link is judged by what it runs.
	foreign := fix1Copy(t, other, "not-ours")
	for _, link := range []string{filepath.Join(dir, "acme-cli"), filepath.Join(other, "acme-cli-link")} {
		if err := os.Symlink(foreign, link); err != nil {
			t.Fatal(err)
		}
		if r := fix1Parent(t, mcp, "path", link, ""); !r.Refused || r.Child != "" {
			t.Errorf("a link %s to a foreign program: %+v, want refused", link, r)
		}
		os.Remove(link)
	}
	// A link in PATH to the real companion is the companion.
	cli := fix1Copy(t, dir, "acme-cli")
	bin := t.TempDir()
	if err := os.Symlink(cli, filepath.Join(bin, "acme-cli")); err != nil {
		t.Fatal(err)
	}
	if r := fix1Parent(t, mcp, "path", "acme-cli", bin); r.Refused || r.Child != fix1Handed {
		t.Errorf("a link in PATH to the companion: %+v, want handed over", r)
	}
}

// A relative path means whatever the working directory holds.
func TestFix1ARelativePathIsNotOwn(t *testing.T) {
	cmd := &exec.Cmd{Path: "acme-cli", Args: []string{"acme-cli"}}
	if ownProgram(cmd) {
		t.Error("a relative path counted as the connector's own program")
	}
}

func TestFix1AProgramNames(t *testing.T) {
	for _, tc := range []struct {
		name, stem, role string
		goos, goarch     string
	}{
		{"acme-mcp", "acme", "mcp", "linux", "amd64"},
		{"acme-cli", "acme", "cli", "linux", "amd64"},
		{"acme-pp-mcp", "acme-pp", "mcp", "linux", "amd64"},
		{"acme-cli.exe", "acme", "cli", "windows", "amd64"},
		{"ACME-MCP.EXE", "acme", "mcp", "windows", "amd64"},
		{"acme-cli-darwin-arm64", "acme", "cli", "darwin", "arm64"},
		{"acme-mcp-darwin", "acme", "mcp", "darwin", "arm64"},
		{"acme-cli.exe", "", "", "linux", "amd64"},
		{"acme-cli-linux-amd64", "", "", "darwin", "arm64"},
		{"acme", "", "", "linux", "amd64"},
		{"-cli", "", "", "linux", "amd64"},
		{"sh", "", "", "linux", "amd64"},
	} {
		if s, r := programName(tc.name, tc.goos, tc.goarch); s != tc.stem || r != tc.role {
			t.Errorf("programName(%q, %s, %s) = %q, %q; want %q, %q", tc.name, tc.goos, tc.goarch, s, r, tc.stem, tc.role)
		}
	}
}

// Class C (axcient kit-findings item 3, domain-cert-tracker section 1): a
// second writer appends to a file this process appends to.
func TestFix1CAppendToleratesAnotherWriter(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	if err := AppendFile(p, []byte("{\"entry\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{\"from\":\"another process\"}\n")
	if err := AppendFile(p, []byte("{\"entry\":2}\n"), 0o600); err != nil {
		t.Errorf("an append after another writer's: %v", err)
	}
	f.WriteString("{\"from\":\"it again\"}\n")
	f.Close()
	if err := AppendFile(p, []byte("{\"entry\":3}\n"), 0o600); err != nil {
		t.Errorf("the next one: %v", err)
	}
	b, _ := os.ReadFile(p)
	want := "{\"entry\":1}\n{\"from\":\"another process\"}\n{\"entry\":2}\n{\"from\":\"it again\"}\n{\"entry\":3}\n"
	if string(b) != want {
		t.Errorf("the file is %q, want %q", b, want)
	}
}

// The same with one long-lived Writer, as a server's log is.
func TestFix1CLongLivedWriterToleratesAnotherWriter(t *testing.T) {
	resetForTest(Declarations{})
	p := filepath.Join(t.TempDir(), "log")
	w, err := OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := range 3 {
		if _, err := w.Write(fmt.Appendf(nil, "server %d\n", i)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
		fmt.Fprintf(f, "child %d\n", i)
		f.Close()
	}
	b, _ := os.ReadFile(p)
	if want := "server 0\nchild 0\nserver 1\nchild 1\nserver 2\nchild 2\n"; string(b) != want {
		t.Errorf("the file is %q, want %q", b, want)
	}
}

// What the kit keeps: this process's own writes are still checked as one
// stream, a held value in a write is still withheld, and nothing it wrote
// or anyone else wrote is overwritten.
func TestFix1CStillChecksWhatItWrites(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	p := filepath.Join(t.TempDir(), "f")
	AppendFile(p, []byte("secr"), 0o600)
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	f.WriteString("X")
	f.Close()
	if err := AppendFile(p, []byte("et!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "et!!") {
		t.Errorf("the rest of a credential this process began writing landed: %q", b)
	}
	if !strings.HasPrefix(string(b), "secrX") {
		t.Errorf("another writer's byte was overwritten: %q", b)
	}
	if err := AppendFile(p, []byte("a clean line\n"), 0o600); err != nil {
		t.Errorf("a clean append after a withheld one: %v", err)
	}
	if err := AppendFile(p, []byte("another secret!! here\n"), 0o600); err != nil {
		t.Errorf("a withheld append: %v", err)
	}
	if b, _ := os.ReadFile(p); strings.Contains(string(b), "secret!!") {
		t.Errorf("a held value reached the file: %q", b)
	}
}

// Two processes appending as fast as they can lose nothing and overwrite
// nothing (macOS pwrite at a stale end would).
func TestFix1CTwoProcessesAppending(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child")
	}
	p := filepath.Join(t.TempDir(), "shared")
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), childModeEnv+"=fix1-appender", "FIX1_FILE="+p, "FIX1_TAG=child")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fix1Appender(p, "parent")
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2*fix1Appends {
		t.Errorf("%d lines, want %d", len(lines), 2*fix1Appends)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "parent ") && !strings.HasPrefix(l, "child ") || !strings.HasSuffix(l, " end") {
			t.Fatalf("a torn or withheld line: %q", l)
		}
	}
}

const fix1Appends = 300

func fix1Appender(p, tag string) {
	resetForTest(Declarations{})
	Hold("minted:t", "secret!!")
	for i := range fix1Appends {
		if err := AppendFile(p, fmt.Appendf(nil, "%s %d end\n", tag, i), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, tag, err)
			os.Exit(1)
		}
	}
}

// Class D (avanan kit-findings item b): a command whose stdout never
// arrived does not report success.
func TestFix1DLostStdoutFailsTheCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child")
	}
	fix1ReleaseSleepers(t)
	var stderr *os.File
	run := func(mode string, stdout *os.File) int {
		t.Helper()
		exe, _ := os.Executable()
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), childModeEnv+"="+mode)
		cmd.Stdout = stdout
		if stderr != nil {
			cmd.Stderr = stderr
		}
		err := cmd.Run()
		var ee *exec.ExitError
		switch {
		case err == nil:
			return 0
		case errors.As(err, &ee):
			return ee.ExitCode()
		}
		t.Fatal(err)
		return -1
	}
	// A stdout that refuses every write: opened for reading only.
	p := filepath.Join(t.TempDir(), "readonly")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ro, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if code := run("fix1-print", ro); code != ExitOutputLost {
		t.Errorf("a lost write, program status 0: exit %d, want %d", code, ExitOutputLost)
	}
	if code := run("fix1-print-3", ro); code != 3 {
		t.Errorf("a lost write, program status 3: exit %d, want 3", code)
	}
	// Codex r3: an MCP response goes to the stream itself, past the pump.
	if code := run("fix1-mcp-out", ro); code != ExitOutputLost {
		t.Errorf("a lost MCP response: exit %d, want %d", code, ExitOutputLost)
	}
	// Codex r1: the loss is remembered when Exit gives up on a stream a child
	// keeps open.
	if code := run("fix1-keeps-stream", ro); code != ExitOutputLost {
		t.Errorf("a lost write, a child holding the stream: exit %d, want %d", code, ExitOutputLost)
	}
	// A good stdout: 0 stays 0, also when nothing was written.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if code := run("fix1-print", null); code != 0 {
		t.Errorf("a stdout that took it: exit %d, want 0", code)
	}
	if code := run("fix1-mcp-out", null); code != 0 {
		t.Errorf("an MCP response that arrived: exit %d, want 0", code)
	}
	stderr = ro
	if code := run("fix1-stderr", null); code != 0 {
		t.Errorf("only stderr refused: exit %d, want 0", code)
	}
	stderr = nil
	if code := run("fix1-quiet", ro); code != 0 {
		t.Errorf("nothing written to a stdout that refuses: exit %d, want 0", code)
	}
	// A notice that arrived did tell the reader: the program's status stands.
	if code := run("fix1-withheld", null); code != 0 {
		t.Errorf("a withheld unit with its notice: exit %d, want 0", code)
	}
	// A unit dropped with its notice said nothing to anyone.
	if code := run("fix1-dropped", null); code != ExitOutputLost {
		t.Errorf("a unit dropped with its notice: exit %d, want %d", code, ExitOutputLost)
	}
}

// Exit does not cut off output still being checked, however long the check
// takes. It gives up on a reader that takes nothing, which is lost output, and
// on a stream a child keeps open, which is not.
func TestFix1DExitWaitsForACheckInProgress(t *testing.T) {
	prev := exitWait
	exitWait = 100 * time.Millisecond
	defer func() { exitWait = prev }()
	p := &pump{done: make(chan struct{})}
	p.checking.Store(true)
	start := time.Now()
	time.AfterFunc(600*time.Millisecond, func() { close(p.done) })
	drainPumps([]*pump{p})
	if d := time.Since(start); d < 500*time.Millisecond {
		t.Errorf("Exit gave up on a pump that was checking after %v", d)
	}
	if p.lostOutput() {
		t.Error("a pump that finished clean lost output")
	}
	stuck := &pump{done: make(chan struct{})}
	stuck.writing.Store(true)
	drainPumps([]*pump{stuck})
	if !stuck.lostOutput() {
		t.Error("a reader that took nothing lost no output")
	}
	open := &pump{done: make(chan struct{})}
	drainPumps([]*pump{open})
	if open.lostOutput() {
		t.Error("a stream a child kept open lost output")
	}
}

// fix1ReleaseSleepers ends a test's fix1-sleeper grandchildren with it: on
// Windows, go test cannot delete a test binary a process is still running.
func fix1ReleaseSleepers(t *testing.T) {
	t.Helper()
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("FIX1_RELEASE", release)
	t.Cleanup(func() {
		if err := os.WriteFile(release, nil, 0o600); err != nil {
			t.Error(err)
		}
		time.Sleep(fix1SleeperPoll * 4)
	})
}

const fix1SleeperPoll = 50 * time.Millisecond

// fix1Sleeper outlives its parent until the test that started it ends.
func fix1Sleeper() {
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(fix1SleeperPoll) {
		if _, err := os.Stat(os.Getenv("FIX1_RELEASE")); err == nil {
			return
		}
	}
}

// A program ends though a child it started still holds its stdout open.
func TestFix1DExitDoesNotWaitForAChildThatKeepsTheStream(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child")
	}
	fix1ReleaseSleepers(t)
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), childModeEnv+"=fix1-keeps-stream")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("exit: %v, want 0", err)
		}
	case <-time.After(45 * time.Second):
		cmd.Process.Kill()
		t.Fatal("the program did not end")
	}
}

// Class E (domain-cert-tracker kit-findings section 2): an MCPB host leaves
// ${user_config.x} in a variable the user left empty. It is not a credential.
func TestFix1EUnfilledPlaceholderIsNotHeld(t *testing.T) {
	const ph = "${user_config.domain_cert_tracker_mcp_http_token}"
	resetForTest(Declarations{})
	t.Setenv("CK_TOKEN", ph)
	t.Setenv("CK_URL", "${user_config.domain_cert_tracker_base_url}")
	gather(Declarations{SecretEnv: []string{"CK_TOKEN"}, BaseURL: BaseURL{Env: "CK_URL", Default: "https://api.example.com"}}, nil)
	if isHeld(ph) {
		t.Error("the placeholder was held")
	}
	if withheld([]byte("domain_cert_tracker doctor: http_token not set")) {
		t.Error("a line naming the connector was withheld")
	}
	reg.Lock()
	base := reg.baseURL
	reg.Unlock()
	if base != "https://api.example.com" {
		t.Errorf("the base URL is %q, want the default", base)
	}
	for _, v := range []string{"${user_config.a}b", "x${user_config.a}", "${user_config.a}}", "${user_config.}", "${other.a}", "${user_config.a"} {
		if unfilledPlaceholder(v) {
			t.Errorf("%q read as an unfilled placeholder", v)
		}
	}
	// Anything else in that variable is the user's value, whatever it looks like.
	resetForTest(Declarations{})
	t.Setenv("CK_TOKEN", "${secret-value-of-theirs}")
	gather(Declarations{SecretEnv: []string{"CK_TOKEN"}}, nil)
	if !isHeld("${secret-value-of-theirs}") {
		t.Error("a real value in the same shape was not held")
	}
}

// fix1Sends reports whether the kit lets a request with a held token go to
// url: the dial fails either way, so ErrWithheld alone says it was refused.
func fix1Sends(t *testing.T, url string) bool {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer fix1-token-Q7vXk2Lp9RwZ")
	base := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("no network in this test")
	}}
	resp, err := (&http.Client{Transport: Transport(base)}).Do(req)
	if err == nil {
		resp.Body.Close()
	}
	return !errors.Is(err, ErrWithheld)
}

// An explicit --config names the file the program reads, so its base URL is
// the one the program sends to (hubspot, quickbooks): the discovered default's
// host must not stay the allowed origin.
func TestFix1GExplicitConfigChoosesTheBaseURL(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	def := write("default.toml", "base_url = \"https://stale.example.com\"\napi_key = \"default-file-key-Zm4Ht8Jc\"\n")
	exp := write("explicit.toml", "base_url = \"https://chosen.example.com\"\n")
	bare := write("bare.toml", "api_key = \"bare-file-key-Hb3wQ7vX\"\n")
	exp2 := write("explicit2.toml", "base_url = \"https://second.example.com\"\n")
	origins := []Origin{{Source: "*", Allowed: []string{"base_url"}}}
	run := func(files func([]string) []string, env string, args ...string) string {
		t.Helper()
		d := Declarations{Files: files, ConfigFields: []string{"api_key"}, Origins: origins, BaseURL: BaseURL{Env: "FIX1_BASE", Default: "https://default.example.com"}}
		t.Setenv("FIX1_BASE", env)
		resetForTest(d)
		atStartup(func() { gather(d, args) })
		reg.Lock()
		defer reg.Unlock()
		return reg.baseURL
	}
	discovered := func([]string) []string { return []string{def} }
	// The coordinator's test: default A, explicit B.
	if got := run(discovered, "", "--config", exp); got != "https://chosen.example.com" {
		t.Errorf("explicit --config: base URL %q", got)
	}
	if !fix1Sends(t, "https://chosen.example.com/v1") {
		t.Error("a send to the explicit file's host was refused")
	}
	if fix1Sends(t, "https://stale.example.com/v1") {
		t.Error("a token could go to the unused default's host")
	}
	if !isHeld("default-file-key-Zm4Ht8Jc") {
		t.Error("the default file was not held")
	}
	// A Files that names the flag's file itself, in either place.
	if got := run(func(a []string) []string { return append([]string{def}, FlagValues(a, "--config")...) }, "", "--config", exp); got != "https://chosen.example.com" {
		t.Errorf("Files names it after the default: base URL %q", got)
	}
	// The flag parser takes the last of several.
	if got := run(discovered, "", "--config", exp, "--config="+exp2); got != "https://second.example.com" {
		t.Errorf("two flags: base URL %q", got)
	}
	// The program reads the explicit file alone, so one with no base URL means
	// the default, not the file it did not read. Its values are held.
	if got := run(discovered, "", "--config", bare); got != "https://default.example.com" {
		t.Errorf("an explicit file without one: base URL %q", got)
	}
	if !isHeld("bare-file-key-Hb3wQ7vX") {
		t.Error("the explicit file was not held")
	}
	// A flag after the "--" that ends the flags is no flag (Codex r4), however
	// the flag before it reads: the program reads the default.
	if got := run(discovered, "", "--json", "--", "--config="+exp); got != "https://stale.example.com" {
		t.Errorf("--config after --: base URL %q", got)
	}
	if !isHeld("default-file-key-Zm4Ht8Jc") {
		t.Error("the default file was not held")
	}
	// The environment still overrides, and with no flag nothing changes.
	if got := run(discovered, "https://env.example.com", "--config", exp); got != "https://env.example.com" {
		t.Errorf("the env override: base URL %q", got)
	}
	if got := run(discovered, ""); got != "https://stale.example.com" {
		t.Errorf("no flag: base URL %q", got)
	}
}

func TestFix1FURLEnvIsHeldAtStartup(t *testing.T) {
	t.Setenv("FIX1_TOKEN_URL", "https://ops:token-url-pass-Q7vX@auth.example.com/t?client_secret=token-url-query-Zm4H")
	t.Setenv("FIX1_FEEDBACK", "${user_config.fix1_feedback_endpoint}")
	d := Declarations{URLEnv: []string{"FIX1_TOKEN_URL", "FIX1_FEEDBACK", "FIX1_UNSET"}}
	resetForTest(d)
	atStartup(func() { gather(d, nil) })
	for _, v := range []string{"token-url-pass-Q7vX", "token-url-query-Zm4H"} {
		if !isHeld(v) {
			t.Errorf("%q was not held", v)
		}
	}
	if isHeld("${user_config.fix1_feedback_endpoint}") || withheld([]byte("fix1_feedback_endpoint is not set")) {
		t.Error("an unfilled placeholder was held")
	}
	// Such a variable does not go to a child that is not the connector's.
	for _, kv := range childEnv() {
		if strings.HasPrefix(kv, "FIX1_TOKEN_URL=") {
			t.Error("a URLEnv variable was passed to a child")
		}
	}
}

func TestFix1FProxyUserinfoIsHeldAtStartup(t *testing.T) {
	// The schemeless form reads as Go reads it, so the password is bound to it.
	if u, err := parseProxyEnv("ops:secret@proxy.example:3128"); err != nil || u.Host != "proxy.example:3128" || u.User == nil {
		t.Errorf("schemeless: %v, %v", u, err)
	}
	names := []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}
	clear := func() {
		for _, n := range names {
			t.Setenv(n, "")
		}
	}
	for _, name := range names {
		if runtime.GOOS == "windows" && name == strings.ToLower(name) {
			continue // the same variable there
		}
		for form, v := range map[string]string{
			"with a scheme":                     "http://opsuser:proxy-pass-Q7vXk2@proxy.example:3128",
			"schemeless":                        "opsuser:proxy-pass-Q7vXk2@proxy.example:3128",
			"socks":                             "socks5://opsuser:proxy-pass-Q7vXk2@proxy.example:1080",
			"a host Go cannot parse":            "opsuser:proxy-pass-Q7vXk2@proxy example:bad",
			"a port Go cannot parse (Codex r4)": "http://opsuser:proxy-pass-Q7vXk2@proxy.example:bad",
			"scheme-relative (Codex r5)":        "//opsuser:proxy-pass-Q7vXk2@proxy.example:3128",
		} {
			clear()
			t.Setenv(name, v)
			d := Declarations{}
			resetForTest(d)
			atStartup(func() { gather(d, nil) })
			if !isHeld("proxy-pass-Q7vXk2") {
				t.Errorf("%s, %s: the password was not held", name, form)
			}
			if !isHeld("opsuser") {
				t.Errorf("%s, %s: the user was not held", name, form)
			}
			for _, kv := range childEnv() {
				if k, _, _ := strings.Cut(kv, "="); k == name {
					t.Errorf("%s, %s: passed to a child", name, form)
				}
			}
		}
	}
	// Codex r6: the user as a program reads it, not only as it is written.
	clear()
	t.Setenv("HTTPS_PROXY", "//ops%40user:proxy-pass-Q7vXk2@proxy.example:3128")
	resetForTest(Declarations{})
	atStartup(func() { gather(Declarations{}, nil) })
	if !isHeld("ops@user") || !isHeld("ops%40user") {
		t.Error("the fallback reading did not hold the user both ways")
	}
	// A proxy without userinfo holds nothing and stays in a child's environment.
	clear()
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	t.Setenv("HTTP_PROXY", "proxy.example:3128")
	d := Declarations{}
	resetForTest(d)
	atStartup(func() { gather(d, nil) })
	if withheld([]byte("proxy.example:3128 http://proxy.example:3128")) {
		t.Error("a proxy without userinfo withheld output")
	}
	if !slices.ContainsFunc(childEnv(), func(kv string) bool { return strings.HasPrefix(kv, "HTTPS_PROXY=http://proxy.example") }) {
		t.Error("a proxy without userinfo was taken from a child's environment")
	}
	// The password goes to that proxy and nowhere else (holdProxyCreds).
	clear()
	t.Setenv("HTTPS_PROXY", "https://opsuser:proxy-pass-Q7vXk2@proxy.example:3128")
	d = Declarations{SecretEnv: []string{"FIX1_KEY"}, Origins: []Origin{{Source: "env:FIX1_KEY", Allowed: []string{"https://api.example.com"}}}}
	resetForTest(d)
	atStartup(func() { gather(d, nil) })
	req, _ := http.NewRequest("GET", "https://api.example.com/proxy-pass-Q7vXk2", nil)
	_, err := (&http.Client{Transport: Transport(&http.Transport{})}).Do(req)
	if !errors.Is(err, ErrWithheld) {
		t.Errorf("the proxy's password to the vendor: %v, want refused", err)
	}
}
