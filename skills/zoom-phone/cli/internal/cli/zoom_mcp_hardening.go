// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// Runtime annotation fixes for generated commands (regen-durable: no generated
// file is edited).
//
//   - Flags that read an arbitrary local file are blocked from MCP callers via
//     mcp:write-flags (the walker omits and rejects any flag listed there), so a
//     prompt-injected agent cannot pull a local credentials file into the
//     learning store and read it back. Inline variants (--notes, --playbook-json)
//     stay available.
//   - GET endpoints whose responses carry secrets (device SIP passwords, desk
//     phone PINs, port-order PINs, smartphone activation codes, chat
//     channel tokens) lose mcp:read-only: they are credential-tier,
//     so MCP read-only mode and `fleet` (without --allow-writes) refuse them.
var zoomMCPFileFlags = map[string]string{
	"teach":          "playbook-file,playbook-notes-file",
	"teach-playbook": "playbook-file,notes-file",
	"import":         "input",
}

var zoomSecretReadCommands = []string{
	"devices get-adevice",                   // provision.sip_accounts[].password
	"setting-templates get",                 // user_settings.desk_phone.pin_code
	"ported-numbers get-details",            // original_billing_info.pin
	"users phone-settings",                  // desk_phone.pin_code
	"common-areas list-activation-codes",    // activation_code (smartphone login)
	"alert-settings list-with-paging-query", // chat_channels[].token
	"alert-settings get-details",            // chat_channels[].token
}

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		for path, flagNames := range zoomMCPFileFlags {
			if cmd := zoomFindExact(root, path); cmd != nil {
				zoomAppendAnnotation(cmd, "mcp:write-flags", flagNames)
			}
		}
		for _, path := range zoomSecretReadCommands {
			if cmd := zoomFindExact(root, path); cmd != nil {
				delete(cmd.Annotations, "mcp:read-only")
			}
		}
	})
}

func zoomFindExact(root *cobra.Command, path string) *cobra.Command {
	cmd, _, err := root.Find(strings.Fields(path))
	if err != nil || cmd == nil || cmd.CommandPath() != root.Name()+" "+path {
		return nil
	}
	return cmd
}

func zoomAppendAnnotation(cmd *cobra.Command, key, value string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	if existing := strings.TrimSpace(cmd.Annotations[key]); existing != "" {
		value = existing + "," + value
	}
	cmd.Annotations[key] = value
}
