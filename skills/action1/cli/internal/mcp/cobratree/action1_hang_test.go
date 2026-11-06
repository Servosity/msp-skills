// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// destructiveCallDeadline bounds a mirrored destructive call in the tests. A
// child that waits for a confirmation it never gets runs until this deadline.
const destructiveCallDeadline = 30 * time.Second

// callHung reports whether a tool call ran into its context deadline, which
// is what a child blocked on a confirmation prompt does. Elapsed time is no
// measure of a hang: on PR #598 the Windows CI row took 2.57 s for the first
// exec of a freshly built probe that answers at once (hand-fix
// destructive-call-hang-by-deadline).
func callHung(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// buildProbe compiles a stand-in companion CLI from body (a main package).
func buildProbe(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, name+".go")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// purgeTool registers a destructive purge command mirrored onto bin.
func purgeTool(t *testing.T, bin string) server.ToolHandlerFunc {
	t.Helper()
	pinMCPReadOnlyOff(t)
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().Bool("yes", false, "skip confirmation")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.AddCommand(&cobra.Command{Use: "purge", Short: "Delete stored rows",
		RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	s := server.NewMCPServer("test", "0.0.0")
	RegisterAll(s, root, func() (string, error) { return bin, nil })
	entry := s.ListTools()["purge"]
	if entry == nil {
		t.Fatalf("purge tool missing")
	}
	return entry.Handler
}

// TestCallHungJudgesTheDeadlineNotTheClock proves the hang check both ways: a
// child that is slow to start but answers is not a hang (the wall-clock check
// it replaces failed on exactly this), and a child that never answers is.
func TestCallHungJudgesTheDeadlineNotTheClock(t *testing.T) {
	slow := buildProbe(t, "slowprobe", `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	time.Sleep(2500 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "refusing without confirmation")
	os.Exit(2)
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), destructiveCallDeadline)
	defer cancel()
	start := time.Now()
	result, err := purgeTool(t, slow)(ctx, mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{}}})
	if elapsed := time.Since(start); elapsed <= 2*time.Second {
		t.Fatalf("the slow probe answered in %s: it no longer reproduces the >2 s start the wall-clock check failed on", elapsed)
	}
	if callHung(ctx) {
		t.Fatalf("a slow child that answered was judged hung")
	}
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("slow child: want a refusal, got err=%v result=%s", err, toolResultText(result))
	}

	stuck := buildProbe(t, "stuckprobe", `package main

import "time"

func main() { time.Sleep(time.Hour) }
`)
	short, cancelShort := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelShort()
	_, _ = purgeTool(t, stuck)(short, mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{}}})
	if !callHung(short) {
		t.Fatalf("a child that never answered was not judged hung")
	}
}
