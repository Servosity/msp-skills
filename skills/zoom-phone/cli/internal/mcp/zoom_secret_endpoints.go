// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "zoom-phone-pp-cli/internal/cliutil"

// GET endpoints whose responses carry secrets. They are credential-tier: the
// code-orchestration execute tool refuses them in MCP read-only mode, matching
// the CLI mirror tools (internal/cli/zoom_mcp_hardening.go drops their
// mcp:read-only annotation).
var zoomSecretEndpointIDs = map[string]string{
	"devices.get-adevice":                   "provision.sip_accounts[].password",
	"setting_templates.get":                 "user_settings.desk_phone.pin_code",
	"ported_numbers.get-details":            "original_billing_info.pin",
	"users.phone-settings":                  "desk_phone.pin_code",
	"common_areas.list-activation-codes":    "activation_code",
	"alert_settings.list-with-paging-query": "chat_channels[].token",
	"alert_settings.get-details":            "chat_channels[].token",
}

func zoomCodeOrchSecretBlocked(id string) bool {
	_, secret := zoomSecretEndpointIDs[id]
	return secret && cliutil.IsMCPReadOnlyEnv()
}
