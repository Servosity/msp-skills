// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// comet-backup handfix (stderr-funnel-flushed-on-exit): the MCP server's
// stderr passes the output filter through a pipe, so a message written just
// before the process exits (the missing HTTP token, flag usage, a bad
// --transport) used to die with the filter goroutine: the operator saw an
// empty exit 1. Every startup exit must deliver its message, still filtered.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestMCPStartupMessagesReachStderr(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	noToken := make([]string, 0, len(e.env))
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "COMET_BACKUP_MCP_HTTP_TOKEN=") {
			noToken = append(noToken, kv)
		}
	}
	cases := []struct {
		name string
		env  []string
		args []string
		code int
		want string
	}{
		{"http without token", noToken, []string{"--transport", "http"}, 1, "COMET_BACKUP_MCP_HTTP_TOKEN must be set"},
		{"help", e.env, []string{"--help"}, 0, "-transport"},
		{"unknown flag", e.env, []string{"--bogus"}, 2, "flag provided but not defined: -bogus"},
		{"unknown transport", e.env, []string{"--transport", "carrier"}, 2, `unknown --transport "carrier"`},
		// Still filtered: the held password typed as the transport is withheld, not echoed.
		{"credential in the message", e.env, []string{"--transport", bbPassword}, 2, "credkit: withheld"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(bbMCP, c.args...)
			cmd.Env = c.env
			cmd.Dir = e.home
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			var err error
			select {
			case err = <-done:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatalf("%v: did not exit", c.args)
			}
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if code != c.code {
				t.Errorf("%v: exit %d, want %d", c.args, code, c.code)
			}
			if !strings.Contains(errb.String(), c.want) {
				t.Errorf("%v: stderr %q does not name %q", c.args, errb.String(), c.want)
			}
			if strings.Contains(out.String()+errb.String(), bbPassword) {
				t.Errorf("%v: the held password reached the output", c.args)
			}
		})
	}
}

// The MCP server's main ends through credkit.Exit; a bare os.Exit there would
// drop what the kit still holds in the stderr pipe.
func TestMCPMainHasNoBareExit(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("../../cmd/comet-backup-mcp/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`\bos\.Exit\(`).FindAllIndex(src, -1); len(m) > 0 {
		t.Fatalf("cmd/comet-backup-mcp/main.go calls os.Exit %d time(s); return from run", len(m))
	}
	if !bytes.Contains(src, []byte("credkit.Exit(run())")) {
		t.Fatal("cmd/comet-backup-mcp/main.go no longer exits through credkit.Exit")
	}
}
