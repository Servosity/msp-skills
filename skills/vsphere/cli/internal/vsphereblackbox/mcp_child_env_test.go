// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// envPrinter is a stand-in companion CLI: it prints the environment it was
// started with, so the test reads the child's environment as the child got it.
const envPrinter = `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() { fmt.Println(strings.Join(os.Environ(), "\n")) }
`

// A mirrored MCP tool runs the companion CLI as a child. With the password in
// the server's environment the tool still logs in (the kit hands the child its
// credentials through credkit.Start), and the child's environment carries no
// credential: no hand-written hand-off on stdin, and nothing held in it.
func TestMCPShellOutGetsCredentialsThroughTheKit(t *testing.T) {
	t.Parallel()
	t.Run("a mirrored tool logs in with the server's environment password", func(t *testing.T) {
		t.Parallel()
		sv := quietServer(t)
		e := newEnv(t, sv, true)
		out := e.mcpCalls(t, []map[string]any{{"name": "migration_blockers", "arguments": map[string]any{}}})
		logins := 0
		for _, r := range sv.requests() {
			if r == "POST /api/session" {
				logins++
			}
		}
		if !strings.Contains(out, "HQ-Record") || logins == 0 {
			t.Fatalf("the CLI child did not log in (logins=%d):\n%.2000s", logins, out)
		}
	})
	t.Run("the child's environment carries no credential", func(t *testing.T) {
		t.Parallel()
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		dir := t.TempDir()
		src := filepath.Join(t.TempDir(), "main.go")
		if err := os.WriteFile(src, []byte(envPrinter), 0o600); err != nil {
			t.Fatal(err)
		}
		fake := filepath.Join(dir, "vsphere-cli"+exe)
		if b, err := exec.Command("go", "build", "-o", fake, src).CombinedOutput(); err != nil {
			t.Fatalf("build the stand-in CLI: %v\n%s", err, b)
		}
		server := filepath.Join(dir, "vsphere-mcp"+exe)
		data, err := os.ReadFile(bbMCP)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(server, data, 0o700); err != nil {
			t.Fatal(err)
		}
		const feedbackToken = "FeedbackTok-8e2c41"
		e := newEnv(t, quietServer(t), true)
		e.env = append(withoutEnv(e.env, "VSPHERE_CLI_PATH"), "VSPHERE_FEEDBACK_ENDPOINT=https://feedback.example/?token="+feedbackToken)
		e.mcp = server
		out := e.mcpCalls(t, []map[string]any{{"name": "migration_blockers", "arguments": map[string]any{}}})
		if !strings.Contains(out, "VSPHERE_MCP_CHILD=1") || !strings.Contains(out, "CREDKIT_HELD=") {
			t.Fatalf("the stand-in CLI did not print its environment:\n%.3000s", out)
		}
		for _, name := range []string{"VSPHERE_PASSWORD=", "VSPHERE_FEEDBACK_ENDPOINT="} {
			if strings.Contains(out, name) {
				t.Errorf("the child's environment carries %s:\n%.3000s", name, out)
			}
		}
		for _, s := range []string{bbPassword, feedbackToken} {
			if strings.Contains(out, s) {
				t.Errorf("the child's environment carries a credential:\n%.3000s", out)
			}
		}
		if strings.Contains(out, withheldNotice) {
			t.Errorf("the child's environment was withheld, so it carried a held value:\n%.3000s", out)
		}
	})
}
