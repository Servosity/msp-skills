// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"slices"

	"pagerduty-pp-cli/internal/cli"
	"pagerduty-pp-cli/internal/cliutil"
	"pagerduty-pp-cli/internal/config"
	"pagerduty-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// PagerDuty authenticates with an API key sent as
// `Authorization: Token token=<key>`, from the environment, config.toml or the
// 0600 credentials.toml store.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"PAGERDUTY_API_KEY",
		"PAGERDUTY_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	// The generated config and credential store keep the API key under
	// api_key; auth_header, the OAuth fields and [headers] are the
	// template's. The kit holds auth_header and [headers] values whether or
	// not they are listed.
	ConfigFields: []string{
		"auth_header",
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	// Flags whose value is a credential: an OAuth client secret for a
	// webhook subscription, a workflow integration connection's secrets and
	// a webhook's custom headers, which authenticate it to its receiver.
	CredentialFlags: []string{
		"--oauth-client-client-secret",
		"--secrets",
		"--webhook-subscription-delivery-method-custom-headers",
	},
	// No StdinFlags: the kit holds a declared stdin body whole and line by
	// line, which would withhold every --stdin write preview. client.do holds
	// every request body with credkit.HoldBody before a preview prints or it
	// is sent, and what a handler prints before that (a JSON parse error) names
	// at most one character of the body.
	//
	// Request body fields that carry a credential: OAuth client secrets, the
	// webhook signing secret and custom headers, workflow connection
	// secrets, the runbook API key of an automation runner, and the Events
	// API routing, service and integration keys.
	CredentialFields: []string{
		"client_secret",
		"secret",
		"secrets",
		"custom_headers",
		"runbook_api_key",
		"routing_key",
		"routing_keys",
		"service_key",
		"integration_key",
	},
	// The vendor response fields that carry a credential, from the earlier
	// layer's PagerDuty name rule applied to the spec's property names
	// (internal/cliutil/testdata/pagerduty-spec-names.json) and the Events API
	// integration key it named. Not declared: client_id (the public half of
	// an OAuth pair, an identifier), "Authentication Headers" (in a response
	// it is a key of a workflow integration's secrets_schema, a definition
	// whose keywords would be held; the values themselves go out under
	// secrets), "Redacted Authentication Headers" (redacted by PagerDuty) and
	// headers (no PagerDuty response carries a credential under that name).
	ResponseFields: []string{
		"authorization",
		"client_secret",
		"secret",
		"secrets",
		"custom_headers",
		"runbook_api_key",
		"routing_key",
		"routing_keys",
		"service_key",
		"integration_key",
	},
	// MCP tools take the credential flags as arguments (endpoint tools) and
	// the body fields inside body_json (the raw and execute tools).
	MCPArguments: []string{
		"oauth-client-client-secret",
		"secrets",
		"webhook-subscription-delivery-method-custom-headers",
		"client_secret",
		"secret",
		"custom_headers",
		"runbook_api_key",
		"routing_key",
		"routing_keys",
		"service_key",
		"integration_key",
	},
	// EU accounts set PAGERDUTY_BASE_URL (or base_url) to
	// https://api.eu.pagerduty.com themselves; no other regional host is
	// allowed by default.
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "PAGERDUTY_BASE_URL",
		Default:   "https://api.pagerduty.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the configured API origin. The
		// connector's own origin pin (api-origin-pinned) narrows every
		// request to that one host, redirects included.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:PAGERDUTY_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's --tls-key material (cmd/pagerduty-mcp,
		// holdTLSKey) is sent nowhere either.
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Config keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:PAGERDUTY_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:PAGERDUTY_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:PAGERDUTY_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:PAGERDUTY_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:PAGERDUTY_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:PAGERDUTY_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:PAGERDUTY_DATA_DIR", Reason: "directory path"},
		{Source: "env:PAGERDUTY_STATE_DIR", Reason: "directory path"},
		{Source: "env:PAGERDUTY_CACHE_DIR", Reason: "directory path"},
		{Source: "env:PAGERDUTY_DB", Reason: "store path a selected client profile refuses to override"},
		{Source: "env:PAGERDUTY_DATABASE", Reason: "store path a selected client profile refuses to override"},
		{Source: "env:PAGERDUTY_DATA_FILE", Reason: "store path a selected client profile refuses to override"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:HOME", Reason: "the user's home directory"},
		{Source: "env:USERPROFILE", Reason: "the user's home directory on Windows"},
		{Source: "env:PAGERDUTY_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:PAGERDUTY_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:PAGERDUTY_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:PAGERDUTY_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:PAGERDUTY_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:PAGERDUTY_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:PAGERDUTY_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:PAGERDUTY_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:PAGERDUTY_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL the operator names; what is posted to it passes the kit's outbound guard"},
		{Source: "env:PAGERDUTY_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:PAGERDUTY_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:PAGERDUTY_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:PAGERDUTY_LEARN_SESSION", Reason: "session grouping label, stored only as a hash"},
		{Source: "env:PAGERDUTY_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CODEX_THREAD_ID", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "agent harness session id, stored only as a hash"},
		{Source: "env:HTTP_PROXY", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:HTTPS_PROXY", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:ALL_PROXY", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:http_proxy", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:https_proxy", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:all_proxy", Reason: "proxy URL; the client compares the dialed address with it"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
		{Source: "env:PATH", Reason: "executable search path"},
		{Source: "env:USERNAME", Reason: "the local account name, for a Windows file ACL"},
		{Source: "env:SYSTEMROOT", Reason: "the Windows directory"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// files this run reads, the one that decides the base URL first: the last --home
// decides the default paths and the last --config the explicit file, as pflag
// does, and config.ReadPaths resolves them by config.Load's own rules. Every other
// file the command line could mean is only held, so its values cannot print and
// its base_url cannot become an allowed origin: credkit.FlagValues reads a flag's
// value that looks like a flag (--notes --home=x) as that flag, and holding a file
// the program does not read is safe, missing the one it reads is not. A file the
// kit reads itself (these and every --config) is left to its reader. No command
// tree here: the MCP server's flags are not the CLI's, and what Start calls must
// stay a small function of the command line.
func credentialFiles(args []string) []string {
	configs := credkit.FlagValues(args, "--config")
	homes := credkit.FlagValues(args, "--home")
	explicit, home := lastOf(configs), lastOf(homes)
	used, ok := filesUnder(explicit, home)
	if !ok { // a --home the program refuses before it reads a file
		used, _ = filesUnder("", "")
	}
	hold := func(files []string, _ bool) {
		for _, f := range files {
			if !slices.Contains(used, f) && !slices.Contains(configs, f) {
				cliutil.HoldFile(f)
			}
		}
	}
	hold(filesUnder("", ""))
	for _, c := range configs {
		hold(filesUnder(c, ""))
	}
	for _, h := range homes {
		hold(filesUnder(explicit, h))
		hold(filesUnder("", h))
	}
	return used
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// filesUnder is config.ReadPaths(configPath) with --home set to home, as the
// program sets it before it loads the config. It holds every value of
// config.ConsultedPaths and the credentials of the run profile stores
// (cli.HoldProfileStores) under that home. ok is false when the program would
// refuse that --home or the path cannot be resolved.
func filesUnder(configPath, home string) ([]string, bool) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return nil, false
	}
	defer restore()
	if consulted, err := config.ConsultedPaths(); err == nil {
		for _, p := range consulted {
			cliutil.HoldFile(p)
		}
	}
	cli.HoldProfileStores()
	files, err := config.ReadPaths(configPath)
	if err != nil {
		return nil, false
	}
	return files, true
}
