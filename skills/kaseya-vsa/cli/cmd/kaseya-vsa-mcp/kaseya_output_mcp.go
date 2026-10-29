// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// kaseya-vsa handfix (output-chokepoint): the MCP server's output paths pass
// the one chokepoint (cliutil.WithholdText): every tool result and error
// through withholdToolResults (outermost middleware, stdio and HTTP alike),
// stderr and the standard logger through InstallStderrFunnel from init.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

func init() {
	id := cliutil.EnvOverride("KASEYA_VSA_TOKEN_ID")
	secret := cliutil.EnvOverride("KASEYA_VSA_TOKEN_SECRET")
	cliutil.FunnelHold(secret, os.Getenv(httpTokenEnvVar))
	if id != "" && secret != "" {
		cliutil.FunnelHold("Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	}
	if err := cliutil.InstallStderrFunnel(); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot start the output filter:", err)
		os.Exit(1)
	}
	filteredStderr = os.Stderr
}

// filteredStderr is the pipe InstallStderrFunnel put in place of os.Stderr.
var filteredStderr *os.File

// flushAndExit is the MCP server's only way out once the stderr filter is
// installed. os.Exit alone ends the process before the filter goroutine has
// written what was just printed, so a startup refusal (no HTTP token, bad
// --addr) reached the operator as an empty stderr and exit 1. Closing the pipe
// lets the filter drain to the real stderr; the wait is bounded.
func flushAndExit(code int) {
	if filteredStderr != nil {
		_ = filteredStderr.Close()
	}
	done := make(chan struct{})
	go func() { cliutil.WaitOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	os.Exit(code)
}

func withholdToolResults(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			if msg := cliutil.WithholdText(err.Error()); msg != err.Error() {
				err = errors.New(msg)
			}
		}
		if res == nil {
			return res, err
		}
		for i, c := range res.Content {
			switch tc := c.(type) {
			case mcplib.TextContent:
				tc.Text = cliutil.WithholdText(tc.Text)
				res.Content[i] = tc
			case *mcplib.TextContent:
				tc.Text = cliutil.WithholdText(tc.Text)
			case mcplib.EmbeddedResource:
				res.Content[i] = withholdEmbedded(tc)
			case *mcplib.EmbeddedResource:
				*tc = withholdEmbedded(*tc)
			}
		}
		if res.StructuredContent != nil {
			b, mErr := json.Marshal(res.StructuredContent)
			if mErr != nil {
				res.StructuredContent = nil
			} else if w := cliutil.WithholdText(string(b)); w != string(b) {
				var v any
				if json.Unmarshal([]byte(w), &v) != nil {
					v = nil
				}
				res.StructuredContent = v
			}
		}
		return res, err
	}
}

func withholdEmbedded(e mcplib.EmbeddedResource) mcplib.EmbeddedResource {
	switch r := e.Resource.(type) {
	case mcplib.TextResourceContents:
		r.Text = cliutil.WithholdText(r.Text)
		e.Resource = r
	case *mcplib.TextResourceContents:
		r.Text = cliutil.WithholdText(r.Text)
	}
	return e
}
