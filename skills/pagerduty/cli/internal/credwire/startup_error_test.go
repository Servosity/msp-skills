// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). pagerduty handfix startup-error-lost: under the
// credential kit stderr is a pipe the kit drains in credkit.Exit, so an exit
// that skips it (flag.Parse's own os.Exit) loses the reason a start was
// refused. Driven through the built MCP binary, the way a host starts it.

package credwire

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

var mcpBuilt struct {
	once sync.Once
	bin  string
	err  error
}

// mcpBinary builds the MCP server once per test run, next to the CLI.
func mcpBinary(t *testing.T) string {
	t.Helper()
	dir := filepath.Dir(nestedCLI(t))
	mcpBuilt.once.Do(func() {
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		out := filepath.Join(dir, "pagerduty-mcp"+exe)
		cmd := exec.Command("go", "build", "-o", out, "./cmd/pagerduty-mcp")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = buildEnv
		if b, err := cmd.CombinedOutput(); err != nil {
			mcpBuilt.err = fmt.Errorf("go build pagerduty-mcp: %v\n%s", err, b)
			return
		}
		mcpBuilt.bin = out
	})
	if mcpBuilt.err != nil {
		t.Fatal(mcpBuilt.err)
	}
	return mcpBuilt.bin
}

// runMCP starts the MCP server with args and no PagerDuty environment and
// returns its stderr and exit code.
func runMCP(t *testing.T, args ...string) (string, int) {
	t.Helper()
	bin := mcpBinary(t)
	home := testenv.Isolate(t)
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "PAGERDUTY_") || strings.HasPrefix(k, "PRINTING_PRESS_") || k == "PP_MCP_TRANSPORT" || k == "CREDKIT_HELD" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PAGERDUTY_HOME="+home, "PAGERDUTY_NO_LEARN=1")
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Dir = home
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("pagerduty-mcp %v timed out", args)
	}
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stderr.String(), code
}

// TestRefusedStartSaysWhy: an unknown flag and an HTTP start without its
// bearer token each exit non-zero and print why.
func TestRefusedStartSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--no-such-flag"}, 2, "flag provided but not defined: -no-such-flag"},
		{[]string{"--transport", "http"}, 1, "PAGERDUTY_MCP_HTTP_TOKEN must be set"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stderr, code := runMCP(t, tc.args...)
			if code != tc.code {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, tc.code, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("a refused start did not say why (want %q); stderr:\n%s", tc.want, stderr)
			}
		})
	}
}
