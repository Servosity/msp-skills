// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the last pass's
// two classes, from that review's failing inputs. A preview never prints a
// credential field of the body, however the body arrived. A read-only MCP child
// writes nothing on any path, a parse failure included.

package credwire

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kaseya-vsa-pp-cli/internal/cli"
	"kaseya-vsa-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

const typingPause = 300 * time.Millisecond

// onTerminal runs bin with its stdin on a pseudo-terminal, where the kit's
// startup read of a --stdin body does not reach, typing body and then EOF.
func onTerminal(t *testing.T, bin string, env []string, body string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dct-tty-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	outPath, errPath := filepath.Join(dir, "out"), filepath.Join(dir, "err")
	inner := []string{"/bin/sh", "-c", `out=$1; err=$2; shift 2; exec "$@" >"$out" 2>"$err"`, "sh", outPath, errPath, bin}
	inner = append(inner, args...)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("script", append([]string{"-q", "/dev/null"}, inner...)...)
	case "linux":
		quoted := make([]string, len(inner))
		for i, a := range inner {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		cmd = exec.Command("script", "-qec", strings.Join(quoted, " "), "/dev/null")
	default:
		t.Skip("needs script(1)")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("needs script(1)")
	}
	cmd.Env = env
	// Typed as a person types: script(1) passes on what it reads as one write,
	// and an end-of-file in the same write as the line loses the line.
	in, typing := io.Pipe()
	defer typing.Close()
	cmd.Stdin = in
	go func() {
		time.Sleep(typingPause)
		_, _ = io.WriteString(typing, body+"\n")
		time.Sleep(typingPause)
		_, _ = io.WriteString(typing, "\x04")
		time.Sleep(typingPause)
		_ = typing.Close()
	}()
	var ptyOut bytes.Buffer
	cmd.Stdout = &ptyOut
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("timed out; terminal said %q", ptyOut.String())
	}
	o, _ := os.ReadFile(outPath)
	e, _ := os.ReadFile(errPath)
	return string(o), string(e), code
}

func TestKitPreviewNeverPrintsCredentialField(t *testing.T) {
	cliBin, _ := binaries(t)
	const shown = "PreviewNameShown"
	create := []string{"webhooks", "create", "--stdin", "--dry-run"}
	update := []string{"webhooks", "update", "7", "--stdin", "--dry-run"}
	flagged := []string{"webhooks", "create", "--dry-run", "--name", shown, "--priorities", "Low", "--url", "https://hook.example.test/x"}
	cases := []struct {
		name     string
		terminal bool
		body     string
		args     []string
		secret   string
	}{
		{"terminal create", true, `{"Name":"` + shown + `","Headers":{"Authorization":"Bearer TerminalSecretQ4821"}}`, create, "TerminalSecretQ4821"},
		{"terminal update", true, `{"Name":"` + shown + `","Headers":{"X-Hook":"UpdateSecretW9317"}}`, update, "UpdateSecretW9317"},
		{"terminal other case", true, `{"Name":"` + shown + `","headers":{"X-Hook":"CaseSecretE5526"}}`, create, "CaseSecretE5526"},
		{"terminal nested", true, `{"Name":"` + shown + `","Outer":[{"HEADERS":{"X-Hook":"NestedSecretR7741"}}]}`, create, "NestedSecretR7741"},
		{"terminal scalar", true, `{"Name":"` + shown + `","Headers":"ScalarSecretT2298"}`, create, "ScalarSecretT2298"},
		{"terminal signing key", true, `{"Name":"` + shown + `","SecretKey":"` + "SigningSecretY6603" + `"}`, update, "SigningSecretY6603"},
		{"terminal another command", true, `{"Name":"` + shown + `","Headers":{"X-Hook":"OtherSecretU1185"}}`, []string{"organizations", "create", "--stdin", "--dry-run"}, "OtherSecretU1185"},
		{"piped create", false, `{"Name":"` + shown + `","Headers":{"Authorization":"Bearer PipedSecretI3350"}}`, create, "PipedSecretI3350"},
		{"flag", false, "", append(slices.Clone(flagged), "--headers", `{"Authorization":"Bearer FlagSecretO8842"}`), "FlagSecretO8842"},
		{"flag with escapes", false, "", append(slices.Clone(flagged), "--headers", `{"Auth":"\u0045\u0073\u0063\u0061\u0070\u0065\u0053\u0065\u0063\u0072\u0065\u0074\u0034\u0038\u0032\u0031"}`), "EscapeSecret4821"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t)
			env := s.env("KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3")
			var stdout, stderr string
			var code int
			if c.terminal {
				stdout, stderr, code = onTerminal(t, cliBin, env, c.body, c.args...)
			} else {
				cmd := exec.Command(cliBin, c.args...)
				cmd.Env = env
				cmd.Stdin = strings.NewReader(c.body)
				var o, e bytes.Buffer
				cmd.Stdout, cmd.Stderr = &o, &e
				err := cmd.Run()
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else if err != nil {
					t.Fatal(err)
				}
				stdout, stderr = o.String(), e.String()
			}
			if code != 0 {
				t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
			}
			if strings.Contains(stdout+stderr, c.secret) {
				t.Errorf("printed %s:\nstdout %q\nstderr %q", c.secret, stdout, stderr)
			}
			// The kit holds a piped body whole at startup, so its preview is
			// withheld whole: safe, and less useful than the others.
			if c.name != "piped create" && (!strings.Contains(stderr, shown) || !strings.Contains(stderr, "no request sent")) {
				t.Errorf("the preview lost its other fields:\n%q", stderr)
			}
		})
	}
}

// TestKitReadOnlyChildParseFailureWritesNothing runs every command in an MCP
// child with a flag it rejects: parsing fails before any command runs, so what
// the command may do cannot decide what is refused.
func TestKitReadOnlyChildParseFailureWritesNothing(t *testing.T) {
	cliBin, _ := binaries(t)
	var runs [][]string
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		runs = append(runs, append(slices.Clone(path), "--no-such-flag-zq"))
		for _, sub := range c.Commands() {
			walk(sub, append(slices.Clone(path), sub.Name()))
		}
	}
	walk(cli.RootCmd(), nil)
	if len(runs) < 50 {
		t.Fatalf("only %d commands found", len(runs))
	}
	runs = append(runs, []string{"analytics", "--max-age", "oops"}, []string{"no-such-command-zq"})
	for _, seeded := range []bool{false, true} {
		s := newSandbox(t)
		env := s.env("KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3", "KASEYA_VSA_MCP_CHILD=1")
		if seeded {
			seedLooseLearnStore(t, s.home)
		}
		before := snapshot(t, s.home)
		for _, args := range runs {
			if r := run(t, cliBin, env, args...); r.code == 0 {
				t.Errorf("%v exited 0", args)
			}
		}
		if changed := snapshotChanges(before, snapshot(t, s.home)); len(changed) > 0 {
			t.Errorf("seeded=%v: changed %v", seeded, changed)
		}
	}
}

func TestPreviewCredentialFieldsAreMCPArguments(t *testing.T) {
	for _, f := range client.PreviewCredentialFields {
		if !slices.Contains(Declarations.MCPArguments, f) || !slices.Contains(Declarations.CredentialFields, f) {
			t.Errorf("%s masked in the preview, not declared", f)
		}
	}
	for _, f := range Declarations.CredentialFields {
		if !slices.Contains(client.PreviewCredentialFields, f) {
			t.Errorf("%s declared, not masked in the preview", f)
		}
	}
}

// A credential in the MCP server's environment reaches a mirrored tool's child:
// the kit hands it over, and the child's request carries it.
func TestKitMirroredToolRunsWithEnvCredential(t *testing.T) {
	_, mcp := binaries(t)
	const tokenID, secret = "EnvTokenIdH4417", "EnvTokenSecretJ9023"
	s := newSandbox(t)
	srv := newRDAPServer(t, s, http.StatusOK, `{"Data":[]}`)
	env := s.env("KASEYA_VSA_BASE_URL="+srv.URL, "KASEYA_VSA_CA_CERT="+srv.caFile,
		"KASEYA_VSA_TOKEN_ID="+tokenID, "KASEYA_VSA_TOKEN_SECRET="+secret)
	frames, stderr := mcpCall(t, mcp, env, "sync", map[string]any{"confirm": true})
	if strings.Contains(frames, "withheld from child process") {
		t.Fatalf("the child was refused: %s\nstderr %s", frames, stderr)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(tokenID+":"+secret))
	if !srv.sent("Authorization", want) {
		t.Fatalf("no request carried the credential (%d requests); frames %s\nstderr %s", srv.requests(), frames, stderr)
	}
	requireAbsent(t, "frames", frames, secret)
	requireAbsent(t, "stderr", stderr, secret)
}

// A number the decoder rejects is named in its error, and that error comes
// before the request is built. Terminal stdin and import's stdin are the two
// bodies the kit's startup read does not reach.
func TestKitBodyParseErrorNeverPrintsCredentialField(t *testing.T) {
	cliBin, _ := binaries(t)
	const body = `{"Headers":{"X-Key":1e309}}`
	// The second Headers member replaces the first in a parse that keeps one.
	const duplicate = `{"Headers":{"X-Key":1e309},"Headers":{}}`
	for _, typed := range []string{body, duplicate} {
		for _, args := range [][]string{
			{"webhooks", "create", "--stdin", "--dry-run", "--no-learn"},
			{"webhooks", "update", "7", "--stdin", "--dry-run", "--no-learn"},
		} {
			s := newSandbox(t)
			stdout, stderr, _ := onTerminal(t, cliBin, s.env("KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3"), typed, args...)
			requireAbsent(t, "stdout", stdout, "1e309")
			requireAbsent(t, "stderr", stderr, "1e309")
			if stderr == "" {
				t.Errorf("%v said nothing", args)
			}
		}
	}
	// import skips a comment line, so a record after one is still read.
	for _, input := range []string{body + "\n", duplicate + "\n", "# comment\n" + body + "\n"} {
		s := newSandbox(t)
		cmd := exec.Command(cliBin, "import", "webhooks", "--input", "-", "--dry-run", "--no-learn")
		cmd.Env = s.env("KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3")
		cmd.Stdin = strings.NewReader(input)
		out, _ := cmd.CombinedOutput()
		requireAbsent(t, "import "+strings.ReplaceAll(input, "\n", " | "), string(out), "1e309")
	}
}

// A password or query value inside a declared URL source (URLEnv, the base URL,
// --deliver, which a profile keeps) is held, so no preview, diagnostic or saved
// profile carries it. A URL in a non-credential field is free text, which
// contract section 1 does not name.
func TestKitURLInputsNeverPrintTheirCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	base := "KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3"
	list := []string{"devices", "list", "--dry-run", "--data-source", "live"}
	cases := []struct {
		name, config, secret string
		env, args            []string
	}{
		{"feedback endpoint", "", "FeedbackValue9z", []string{base, "KASEYA_VSA_FEEDBACK_ENDPOINT=https://ops:FeedbackValue9z@feedback.example:bad/"}, []string{"feedback", "safe", "--send"}},
		{"server", "", "ServerValue9z", []string{"KASEYA_VSA_SERVER=https://ops:ServerValue9z@tenant.vsax.net"}, list},
		{"server without a scheme", "", "BareValue9z", []string{"KASEYA_VSA_SERVER=ops:BareValue9z@tenant.vsax.net"}, list},
		{"server query", "", "QueryValue9z", []string{"KASEYA_VSA_SERVER=tenant.vsax.net/?token=QueryValue9z"}, list},
		{"base url without a scheme", "", "BaseBareValue9z", []string{"KASEYA_VSA_BASE_URL=ops:BaseBareValue9z@tenant.vsax.net/api/v3"}, list},
		{"deliver", "", "DeliveryValue9z", []string{base}, []string{"version", "--deliver", "webhook:https://ops:DeliveryValue9z@feedback.example/", "--dry-run"}},
		{"deliver query", "", "DeliverQueryValue9z", []string{base}, []string{"version", "--deliver", "webhook:https://feedback.example/?token=DeliverQueryValue9z", "--dry-run"}},
		{"deliver malformed", "", "MalformedValue9z", []string{base}, []string{"version", "--deliver", "webhook:ops:MalformedValue9z@feedback.example"}},
		{"profile save", "", "ProfileValue9z", []string{base}, []string{"profile", "save", "kept", "--deliver", "webhook:https://ops:ProfileValue9z@feedback.example/"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t)
			env := c.env
			if c.config != "" {
				env = append(slices.Clone(env), "KASEYA_VSA_CONFIG="+s.writePrivate(t, "cfg.toml", c.config))
			}
			res := run(t, cliBin, s.env(env...), c.args...)
			requireAbsent(t, "stdout", res.stdout, c.secret)
			requireAbsent(t, "stderr", res.stderr, c.secret)
			if hits := filesHolding(t, s.home, c.secret); len(hits) > 0 && c.config == "" {
				t.Errorf("persisted to %v", hits)
			}
		})
	}
}

// The MCP write preview shows a body the caller sent, before any request.
func TestKitMCPPreviewNeverPrintsCredentialField(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	const shown = "PreviewNameShown"
	secrets := []string{"McpHeaderValue9z", "McpSigningValue9z"}
	frames, stderr := mcpCall(t, mcp, s.env("KASEYA_VSA_BASE_URL=https://kit.vsax.net/api/v3"), "kaseya-vsa_execute", map[string]any{
		"endpoint_id": "webhooks.create",
		"params": map[string]any{
			"Name": shown, "Priorities": []string{"Low"},
			"Url":       "https://hook.example.test/x",
			"Headers":   map[string]any{"X-Hook": secrets[0]},
			"SecretKey": secrets[1],
		},
	})
	for _, secret := range secrets {
		requireAbsent(t, "frames", frames, secret)
		requireAbsent(t, "stderr", stderr, secret)
	}
	if !strings.Contains(frames, shown) || !strings.Contains(frames, "confirmation_required") {
		t.Errorf("no preview with the other fields: %s", frames)
	}
}
