// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"slices"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/config"
	"quickbooks-pp-cli/internal/credkit"
)

// intuitOAuth is the origin of Intuit's OAuth 2.0 token endpoint.
const intuitOAuth = "https://oauth.platform.intuit.com"

// Declarations is what the connector holds and where each credential may go.
// Credentials: the OAuth client, access and refresh tokens and the MCP bearer
// in the environment, the config file (auth_header, [headers], any credential
// inside base_url), and the 0600 credentials store. No tool declares an argument
// that carries a credential, but a write's params are free-form and its preview
// repeats them, so MCPArguments holds these names wherever a call states them. A
// token auth set-token reads from stdin and every minted auth header are held
// where they are read; credentialFiles holds the credentials in the endpoint
// variables and a saved profile's credential values.
var Declarations = credkit.Declarations{
	SecretEnv:        []string{"QUICKBOOKS_ACCESS_TOKEN", "QUICKBOOKS_CLIENT_ID", "QUICKBOOKS_CLIENT_SECRET", "QUICKBOOKS_REFRESH_TOKEN", "QUICKBOOKS_MCP_HTTP_TOKEN"},
	Files:            credentialFiles,
	StdinFlags:       []string{"--stdin"},
	ConfigFields:     []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:    []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialFields: []string{"refresh_token", "client_secret", "access_token"},
	ResponseFields:   []string{"access_token", "refresh_token", "id_token"},
	MCPArguments:     []string{"auth_header", "access_token", "refresh_token", "id_token", "client_id", "client_secret", "authorization", "token", "password", "api_key"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "QUICKBOOKS_BASE_URL",
		Default:   "https://quickbooks.api.intuit.com",
	},
	Origins: []credkit.Origin{
		// An access token goes to the base URL the program uses (selectBaseURL),
		// never to a --deliver webhook, the feedback endpoint or a redirect's host.
		{Source: "*", Allowed: []string{"base_url"}},
		// The OAuth client and the refresh token go to the token endpoint:
		// Intuit's, or QUICKBOOKS_TOKEN_URL on the base URL's origin.
		{Source: "env:QUICKBOOKS_CLIENT_ID", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "env:QUICKBOOKS_CLIENT_SECRET", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "env:QUICKBOOKS_REFRESH_TOKEN", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "config:client_id", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "config:client_secret", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "config:refresh_token", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "field:refresh_token", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "field:client_secret", Allowed: []string{"base_url", intuitOAuth}},
		{Source: "response:refresh_token", Allowed: []string{"base_url", intuitOAuth}},
		// Every other value of a token answer (cliutil.QBODecodeCredentialJSON).
		{Source: "minted:token-answer", Allowed: []string{"base_url", intuitOAuth}},
		// The MCP HTTP bearer authenticates callers of this server, and the
		// TLS key is this server's own; neither is sent anywhere.
		{Source: "env:QUICKBOOKS_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "config:supersedes_config", Reason: "SHA-256 of a config file a token refresh could not yet rewrite; the digest cannot be reversed to a credential the file holds, each of which is held on its own"},
		{Source: "env:QUICKBOOKS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:QUICKBOOKS_TOKEN_URL", Reason: "token endpoint URL; a credential inside it is held by QBOHoldEnvInputs"},
		{Source: "env:QUICKBOOKS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held by QBOHoldEnvInputs, and the outbound guard refuses any held value sent to it"},
		{Source: "env:QUICKBOOKS_REALM_ID", Reason: "the QuickBooks company id, part of every API path"},
		{Source: "env:QUICKBOOKS_ENVIRONMENT", Reason: "production or sandbox"},
		{Source: "env:QUICKBOOKS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:QUICKBOOKS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:QUICKBOOKS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:QUICKBOOKS_DB_PATH", Reason: "path of the local mirror"},
		{Source: "env:QUICKBOOKS_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:QUICKBOOKS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:QUICKBOOKS_DATA_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:QUICKBOOKS_STATE_DIR", Reason: "directory path"},
		{Source: "env:QUICKBOOKS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:QUICKBOOKS_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:QUICKBOOKS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:QUICKBOOKS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:QUICKBOOKS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:QUICKBOOKS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:QUICKBOOKS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:QUICKBOOKS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:QUICKBOOKS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:QUICKBOOKS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:QUICKBOOKS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:QUICKBOOKS_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:QUICKBOOKS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
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

// baseURLEnv is the one setting the kit takes over every file's base_url.
const baseURLEnv = "QUICKBOOKS_BASE_URL"

// restoreBaseURLEnv undoes selectBaseURL.
var restoreBaseURLEnv = func() {}

// RestoreBaseURLEnv puts the variable back after credkit.Start has read it, so
// the program sees what it was given. Both mains call it right after Start.
func RestoreBaseURLEnv() {
	restoreBaseURLEnv()
	restoreBaseURLEnv = func() {}
}

// selectBaseURL gives the kit the base URL the program will really use. The kit
// binds the environment's over any file's, and otherwise the first file naming
// one, which can be a store, an overridden --config or a default this run does
// not read. So for the kit's read alone, the variable carries the answer.
func selectBaseURL(configFlag, homeFlag string) {
	if cliutil.EnvOverride(baseURLEnv) != "" {
		return
	}
	base := ""
	underHome(homeFlag, func() { base = config.SelectedBaseURL(configFlag) })
	prior, had := os.LookupEnv(baseURLEnv)
	restoreBaseURLEnv = func() {
		if had {
			_ = os.Setenv(baseURLEnv, prior)
		} else {
			_ = os.Unsetenv(baseURLEnv)
		}
	}
	_ = os.Setenv(baseURLEnv, base)
}

// underHome runs f with --home set to home, when one was given and it is usable.
func underHome(home string, f func()) {
	if home != "" {
		if restore, err := cliutil.SetHomeOverride(home); err == nil {
			defer restore()
		}
	}
	f()
}

// lastOf is the value a flag parser keeps for a repeated flag.
func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// holdProfileStores holds what the saved profiles under the current home carry.
// profiles.json is not in Files: see cliutil.QBOHoldProfileStore.
func holdProfileStores() {
	var paths []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".quickbooks-cli", "profiles.json"))
	}
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if data, err := cliutil.ReadOpenedConfig(p); err == nil {
			cliutil.QBOHoldProfileStore(data.Data)
		}
	}
}

// credentialFiles is every config file and credentials store the command line
// could mean: under the environment and the default home, under each --config
// and each --home the command line may give. It names more than the program
// reads when a flag's value looks like a flag; holding a file the program does
// not read is safe, missing the one it reads is not. The origin a credential may
// go to does not come from this list: see selectBaseURL.
func credentialFiles(args []string) []string {
	cliutil.QBOHoldEnvInputs()
	configs := credkit.FlagValues(args, "--config")
	homes := credkit.FlagValues(args, "--home")
	selectBaseURL(lastOf(configs), lastOf(homes))
	var out []string
	add := func(files []string) {
		for _, f := range files {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	add(config.ReadPaths(""))
	holdProfileStores()
	for _, c := range configs {
		add(config.ReadPaths(c))
	}
	for _, h := range homes {
		underHome(h, func() {
			add(config.ReadPaths(""))
			holdProfileStores()
		})
	}
	return out
}
