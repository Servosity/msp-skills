// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json mcp-blocks-local-file-inputs.

package cobratree

import (
	"strings"

	"github.com/spf13/cobra"
)

// fileInputFlags are flags whose value is a local path the CLI reads. MCP runs
// as the server account, so a client choosing that path could read any file
// it can reach (credentials.toml, SSH keys) and get it back via recall or
// playbook_list, even with JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY=1. The inline
// twins (--playbook-json, --notes) stay available. cmdb reconcile --file is not
// listed: it is contained to the server's working directory (cmdb_reconcile.go).
// Keyed by command path without the root name.
// TestEveryFileInputFlagIsBlocked keeps this in step with internal/cli.
var fileInputFlags = map[string][]string{
	"teach":          {"playbook-file", "playbook-notes-file"},
	"teach-playbook": {"notes-file", "playbook-file"},
	"import":         {"input"},
	// An endpoint command, so execute (not a shell-out tool) serves it today;
	// listed so a future mirror of it can never take a client-chosen path.
	"servicedesk attach-temporary-file upload-temporary-file": {"file"},
}

func fileInputFlagNames(cmd *cobra.Command) []string {
	if cmd == nil {
		return nil
	}
	path := cmd.CommandPath()
	if i := strings.IndexByte(path, ' '); i >= 0 {
		return fileInputFlags[path[i+1:]]
	}
	return nil
}
