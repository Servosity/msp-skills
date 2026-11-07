// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/config"
	"servosity-msp-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Secret fields an API read returns on purpose (an encryption key, a stored
// credential's password, an MFA code, the token --reveal-token shows) are NOT
// response fields here: holding one would withhold the read a person asked
// for, at the CLI and at servosity-msp_execute's confirm: true. They are kept
// off disk and away from an MCP agent without a confirm by
// handfixes.json secret-family-never-persisted. A value this program also
// holds as its own credential stays withheld even then: --reveal-token with the
// token in use, or contracts get-by-token if the API echoes the --token typed.
//
// Written outside the kit, each for the reason named:
//   - The credentials store and the config file (AtomicWriteCredentialFile and
//     the config snapshot restore): credential sources, listed in Files. The kit
//     withholds a held value by contract, so a token could never be saved
//     through it.
//   - Lock files, the empty SQLite journal file and the .data-db-owner marker:
//     constant bytes or a hash of the store's scope, no value of ours.
//   - The platform fingerprint key: random bytes this program generates.
//   - The journal's torn-tail repair: one newline.
//   - os.Rename and os.Link of a file the kit just wrote.
//
// The credentials sent to the API, and the bearer that protects the MCP HTTP
// server, are bound by Origins below.
var Declarations = credkit.Declarations{
	SecretEnv:     []string{"SERVOSITY_MSP_TOKEN", "SERVOSITY_MSP_MCP_HTTP_TOKEN"},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_secret", "msp_token"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_secret", "msp_token"},
	CredentialFlags: []string{
		"--x-servosity-mfa", "--password", "--token", "--key", "--lan-password",
		"--application-settings-password", "--application-settings-encryption-password", "--shadowprotect-keys",
	},
	// The MCP server hands a mirrored tool's child the environment's token on
	// its stdin (cliutil.CredentialStdinFlag), never in argv or environment.
	StdinFlags:        []string{cliutil.CredentialStdinFlag},
	CredentialHeaders: []string{"X-Servosity-Mfa"},
	CredentialFields: []string{
		"password", "token", "key", "lan_password", "application_settings_password",
		"application_settings_encryption_password", "shadowprotect_keys", "client_secret", "refresh_token",
	},
	ResponseFields: []string{"access_token", "refresh_token", "client_secret"},
	MCPArguments: []string{
		"X-Servosity-Mfa", "x-servosity-mfa", "password", "token", "key", "lan_password", "lan-password",
		"application_settings_password", "application-settings-password",
		"application_settings_encryption_password", "application-settings-encryption-password",
		"shadowprotect_keys", "shadowprotect-keys",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SERVOSITY_MSP_BASE_URL",
		Default:   "https://api.servosity.com/api/v1",
	},
	Origins: []credkit.Origin{
		// The token, MFA codes, [headers] values and every other credential go
		// only to the configured API origin, never to a webhook --deliver
		// names, the feedback endpoint or a host a redirect names.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent
		// nowhere.
		{Source: "env:SERVOSITY_MSP_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the
		// per-read hold leaves unheld too (TestIdentifiersMatchNotCredentials).
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp"},
		{Source: "config:client_id", Reason: "the public half of an OAuth client pair"},
		{Source: "env:SERVOSITY_MSP_RESELLER_ID", Reason: "the numeric reseller id the partner portal shows"},
		{Source: "env:SERVOSITY_MSP_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SERVOSITY_MSP_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SERVOSITY_MSP_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SERVOSITY_MSP_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:SERVOSITY_MSP_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:SERVOSITY_MSP_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SERVOSITY_MSP_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SERVOSITY_MSP_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SERVOSITY_MSP_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SERVOSITY_MSP_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:SERVOSITY_MSP_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:SERVOSITY_MSP_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SERVOSITY_MSP_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside a URL is held by the outbound guard when sent"},
		{Source: "env:SERVOSITY_MSP_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:SERVOSITY_MSP_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SERVOSITY_MSP_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SERVOSITY_MSP_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:SERVOSITY_MSP_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:PP_FILELOCK_HOLDER", Reason: "test-helper switch for the file lock"},
		{Source: "env:PP_FILELOCK_PATH", Reason: "test-helper path for the file lock"},
		{Source: "env:PP_FILELOCK_READY", Reason: "test-helper path for the file lock"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:USERNAME", Reason: "account name for a Windows permission check"},
		{Source: "env:SYSTEMROOT", Reason: "Windows directory path"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles is every credential file the program could read given args:
// each config.ReadPaths under the environment's and defaults' resolution, each
// --config value and each --home. credkit.FlagValues reads every spelling at
// every position, so a value that only looks like one of these flags names one
// file too many. Holding a file the program does not read is safe; missing the
// one it reads is not. ponytail: no Cobra parse of argv, this over-approximates.
func credentialFiles(args []string) []string {
	var out []string
	add := func(configPath, home string) {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			return
		}
		defer restore()
		if files, err := config.ReadPaths(configPath); err == nil {
			out = append(out, files...)
		}
	}
	add("", "")
	for _, c := range credkit.FlagValues(args, "--config") {
		add(c, "")
	}
	for _, h := range credkit.FlagValues(args, "--home") {
		add("", h)
	}
	return out
}
