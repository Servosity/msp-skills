// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"slices"

	"hubspot-pp-cli/internal/cli"
	"hubspot-pp-cli/internal/cliutil"
	"hubspot-pp-cli/internal/config"
	"hubspot-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Credentials: the access token and the MCP bearer in the environment, the
// config file (auth_header, [headers], any credential inside base_url), and
// the 0600 credentials store. press/tools-manifest.json has no tool argument
// or response field that carries a credential, so MCPArguments and
// ResponseFields are empty; a token auth set-token reads from stdin is held
// where it is read, and every minted auth header is held before the request.
// StdinFlags and CredentialFields are empty for the same reason: --stdin carries
// a CRM record on 155 commands, never a credential, and the kit holds a declared
// stdin whole, which would withhold every write's preview. The run profile store
// is a source too, held in credentialFiles under credential names and base-url.
var Declarations = credkit.Declarations{
	SecretEnv:     []string{"HUBSPOT_ACCESS_TOKEN", "HUBSPOT_MCP_HTTP_TOKEN"},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "HUBSPOT_BASE_URL",
		Default:   "https://api.hubapi.com",
	},
	Origins: []credkit.Origin{
		// Every HubSpot credential goes only to the configured base URL's
		// origin, never to a --deliver webhook, the feedback endpoint or a
		// redirect's host.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server, and the
		// TLS key is this server's own; neither is sent anywhere.
		{Source: "env:HUBSPOT_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "env:HUBSPOT_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:HUBSPOT_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:HUBSPOT_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:HUBSPOT_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:HUBSPOT_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:HUBSPOT_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:HUBSPOT_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:HUBSPOT_OWNER_EMAIL", Reason: "the owner's email address, a filter value"},
		{Source: "env:HUBSPOT_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:HUBSPOT_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:HUBSPOT_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:HUBSPOT_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:HUBSPOT_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:HUBSPOT_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:HUBSPOT_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; the outbound guard refuses any held value sent to it"},
		{Source: "env:HUBSPOT_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:HUBSPOT_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:HUBSPOT_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:HUBSPOT_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:HUBSPOT_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
	},
}

// credentialFiles names the one file the program discovers its base URL in, and
// the kit reads the base URL from the files it is given: named here, a stale
// file's base_url would become the origin credentials may go to. An explicit
// --config needs nothing here, since the kit takes the base URL from it over
// this file. Every other file the command line could mean is held directly:
// under the environment and the default home, under each --config and each
// --home the command line may give. That is more than the program reads when a
// flag's value looks like a flag; holding a file the program does not read is
// safe, missing the one it reads is not.
func credentialFiles(args []string) []string {
	base := baseURLFile(args)
	seen := []string{base}
	hold := func(files []string) {
		for _, f := range files {
			if !slices.Contains(seen, f) {
				seen = append(seen, f)
				config.HoldFile(f)
			}
		}
	}
	hold(config.ReadPaths(""))
	cli.HoldProfileStores()
	for _, c := range credkit.FlagValues(args, "--config") {
		hold(config.ReadPaths(c))
	}
	for _, h := range credkit.FlagValues(args, "--home") {
		if restore, err := cliutil.SetHomeOverride(h); err == nil {
			hold(config.ReadPaths(""))
			cli.HoldProfileStores()
			restore()
		}
	}
	if base == "" {
		return nil
	}
	return []string{base}
}

// baseURLFile reads --home the way the CLI does: its own command tree finds the
// command and pflag parses that command's flags, so a value that looks like a
// flag (--notes --home=x) stays the value it is. When that fails the CLI exits on
// the parse error before any request, and the MCP server's own flags fail it too,
// so what is left is the file the environment names.
func baseURLFile(args []string) string {
	restore, err := cliutil.SetHomeOverride(cobraHomeFlag(args))
	if err != nil {
		return config.BaseURLFile()
	}
	defer restore()
	return config.BaseURLFile()
}

func cobraHomeFlag(args []string) string {
	root := cli.RootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	cmd, rest, err := root.Find(args)
	if err != nil || cmd == nil || cmd.DisableFlagParsing {
		return ""
	}
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()
	if err := cmd.ParseFlags(rest); err != nil {
		return ""
	}
	home, _ := cmd.Flags().GetString("home")
	return home
}
