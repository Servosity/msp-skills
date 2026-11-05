// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"fmt"
	"io"
	"os"

	"zoom-phone-pp-cli/internal/cli"
	"zoom-phone-pp-cli/internal/credkit"
)

// The MCP server's search and sql tools open the local store in-process,
// without the CLI pre-run or an API client, so the account-isolation check
// (no links in a selected account's storage tree, profile/data-dir pairing)
// runs once at server startup, before any tool is registered or served. A
// refused environment stops the server with the CLI's exit code (10). API
// tools are additionally checked per call by the CLI client hook, and mirrored
// CLI commands by the CLI's own pre-run.
//
// main calls ZoomAcctStartupCheck right after credkit.Start, so a refusal naming a
// path passes the kit.

// zoomAcctIsolationExit is credkit.Exit; tests replace it.
var zoomAcctIsolationExit = credkit.Exit

// ZoomAcctStartupCheck stops the server when the account storage is refused.
func ZoomAcctStartupCheck() bool {
	return zoomAcctEnforceIsolation(os.Stderr)
}

// zoomAcctEnforceIsolation reports whether the process may serve; on refusal
// it writes the reason to w and exits.
func zoomAcctEnforceIsolation(w io.Writer) bool {
	if err := cli.ZoomCheckAccountIsolation(); err != nil {
		fmt.Fprintf(w, "zoom-phone-mcp: refusing to start: %v\n", err)
		zoomAcctIsolationExit(cli.ExitCode(err))
		return false
	}
	return true
}
