// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): this connector's one credkit.Declarations
// literal. Pinned by this connector's handfixes.json under the "credkit-wiring"
// entry. The credential store (auth set-token, a token save, a config rewrite
// that keeps a [headers] table, the webhook secret) is a credential SOURCE: its
// writes go outside the kit, which withholds them by contract, and Files lists
// every one of those files.

package cli

import (
	"os"

	"zammad-pp-cli/internal/client"
	"zammad-pp-cli/internal/config"
	"zammad-pp-cli/internal/credkit"
)

// Declarations is what this connector declares about its credentials.
var Declarations = credkit.Declarations{
	SecretEnv:        []string{"ZAMMAD_API_TOKEN", "ZAMMAD_MCP_HTTP_TOKEN", "ZAMMAD_WEBHOOK_SECRET"},
	Files:            client.CredentialFiles,
	ConfigFields:     []string{"api_token", "client_secret"},
	StoredRecords:    []string{"access_token", "refresh_token"},
	CredentialFields: []string{"password", "signature_token"},
	MCPArguments:     []string{"password", "signature_token"},
	ResponseFields: []string{
		"token", "access_token", "refresh_token", "id_token", "client_secret",
		"signature_token", "bearer_token", "basic_auth_password", "password",
	},
	BaseURL: credkit.BaseURL{ConfigKey: "base_url", Env: "ZAMMAD_BASE_URL", Default: "https://your-instance.zammad.com/api/v1"},
	Origins: []credkit.Origin{{Source: "*", Allowed: []string{"base_url"}}},
	Identifiers: []credkit.Identifier{
		{Source: "config:ca_cert", Reason: "a path to a CA certificate file"},
		{Source: "config:tls_server_name", Reason: "a TLS host name"},
		{Source: "config:token_expiry", Reason: "a timestamp"},
		{Source: "config:client_id", Reason: "a public OAuth client id, sent in the clear by design"},
		{Source: "env:AGENT_ID", Reason: "an agent name, stored in a feedback entry"},
		{Source: "env:NO_COLOR", Reason: "a display switch"},
		{Source: "env:TERM", Reason: "a terminal type"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "the MCP transport name"},
		{Source: "env:PRESEED_DISABLED", Reason: "a switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "a profile name"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "a profile name"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "a test switch"},
		{Source: "env:XDG_CACHE_HOME", Reason: "a directory"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "a directory"},
		{Source: "env:XDG_DATA_HOME", Reason: "a directory"},
		{Source: "env:XDG_STATE_HOME", Reason: "a directory"},
		{Source: "env:ZAMMAD_CA_CERT", Reason: "a path to a CA certificate file"},
		{Source: "env:ZAMMAD_CLI_PATH", Reason: "a path to the companion CLI"},
		{Source: "env:ZAMMAD_CONFIG", Reason: "a path to the configuration file, which Files lists"},
		{Source: "env:ZAMMAD_DATA_SOURCE", Reason: "a mode name"},
		{Source: "env:ZAMMAD_FEEDBACK_AUTO_SEND", Reason: "a switch"},
		{Source: "env:ZAMMAD_FEEDBACK_ENDPOINT", Reason: "a URL; HoldStartupCredentials holds any credential inside it"},
		{Source: "env:ZAMMAD_HOME", Reason: "a directory"},
		{Source: "env:ZAMMAD_LEARN_NO_CAPTURE", Reason: "a switch"},
		{Source: "env:ZAMMAD_LEARN_SESSION", Reason: "a session name, stored only as a hash"},
		{Source: "env:ZAMMAD_LEARN_SURFACE", Reason: "a surface name"},
		{Source: "env:ZAMMAD_MAX_RESPONSE_BYTES", Reason: "a size"},
		{Source: "env:ZAMMAD_MCP_CHILD", Reason: "a marker this program sets on its own child"},
		{Source: "env:ZAMMAD_MCP_READ_ONLY", Reason: "a switch"},
		{Source: "env:ZAMMAD_NO_LEARN", Reason: "a switch"},
		{Source: "env:ZAMMAD_NO_LOCAL_DATA", Reason: "a switch"},
		{Source: "env:ZAMMAD_TLS_SERVER_NAME", Reason: "a TLS host name"},
		{Source: "env:ZAMMAD_URL", Reason: "the instance root; init below makes it ZAMMAD_BASE_URL, which the kit holds"},
		{Source: "env:ZAMMAD_USER_AGENT", Reason: "a User-Agent string"},
	},
}

// The kit reads one base-URL variable (Declarations.BaseURL.Env). ZAMMAD_URL is
// the documented per-instance form, so the value config.Load will use is made
// the one the kit reads, before main runs.
func init() {
	if v := config.InstanceBaseURLOverride(); v != "" {
		_ = os.Setenv("ZAMMAD_BASE_URL", v)
	} else {
		_ = os.Unsetenv("ZAMMAD_BASE_URL")
	}
}
