// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2,
// config-reload-hold. The MCP server reloads its config on every tool call;
// each read holds every value in the file before the file is parsed.

package credwire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The reviewer's input: start MCP with auth_header = "oldcredential999",
// replace that config with auth_header = 123456789012345678901234567890 (a
// number the TOML parser overflows on, so its diagnostic carries it), then
// call kaseya-vsa_read. The per-call reload must hold the
// new value before it parses the file.
func TestKitMCPConfigReloadHoldsBeforeParse(t *testing.T) {
	_, mcp := binaries(t)
	const secret = "123456789012345678901234567890"
	s := newSandbox(t)
	cfg := s.writePrivate(t, "rdap.toml", "auth_header = \"oldcredential999\"\n")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcp)
	cmd.Env = s.env("KASEYA_VSA_CONFIG=" + cfg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	var frames strings.Builder
	await := func(id string) {
		t.Helper()
		for sc.Scan() {
			line := sc.Text()
			frames.WriteString(line + "\n")
			var msg struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil && string(msg.ID) == id {
				return
			}
		}
		t.Fatalf("the MCP server never answered id %s; frames:\n%s\nstderr:\n%s", id, frames.String(), errb.String())
	}
	send := func(m string) {
		t.Helper()
		if _, err := io.WriteString(stdin, m+"\n"); err != nil {
			t.Fatalf("write to the MCP server: %v", err)
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"credkit-reload-test","version":"1"}}}`)
	await("1")
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// The operator (or anything else) rewrites the config while the server runs.
	if err := os.WriteFile(cfg, []byte("auth_header = "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kaseya-vsa_read","arguments":{"endpoint_id":"organizations.list"}}}`)
	await("2")
	if !strings.Contains(frames.String(), `"isError":true`) {
		t.Fatalf("kaseya-vsa_read did not fail on the unparseable config, so this test proved nothing; frames:\n%s", frames.String())
	}
	requireAbsent(t, "MCP kaseya-vsa_read frames", frames.String(), secret)
	requireAbsent(t, "MCP stderr", errb.String(), secret)
}
