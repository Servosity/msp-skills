// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/cliutil"
	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/credkit"
)

const (
	dellAPI    = "https://apigtwb2c.us.dell.com"
	lenovoAPI  = "https://supportapi.lenovo.com"
	appleOAuth = "https://account.apple.com"
	appleAPI   = "https://api-business.apple.com"
)

// Declarations is what the connector holds and where each credential may go.
// Three vendors, each credential bound to its own vendor's origins:
//   - Dell TechDirect: an OAuth2 client ID and secret (environment, config or
//     the 0600 credentials.toml) and the token minted from them.
//   - Lenovo eSupport: one ClientID token, sent as a header.
//   - Apple Business: a client ID, key ID and private key (PEM text or a path
//     in the environment), which sign an assertion for the token endpoint.
//
// WARRANTY_BASE_URL (or base_url in config) names another Dell API host and
// Dell's credentials may go there. WARRANTY_TOKEN_URL gets no binding of its
// own: a token URL off both Dell's origin and the base URL is refused by the
// kit, because it would receive the client secret.
//
// StdinFlags is empty: the one --stdin is feedback's free text, never a credential,
// and the kit holds a declared stdin whole, which would withhold the feedback itself.
// auth set-token holds the token it reads where it reads it.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"WARRANTY_DELL_CLIENT_ID",
		"WARRANTY_DELL_CLIENT_SECRET",
		"WARRANTY_LENOVO_CLIENT_KEY",
		"WARRANTY_APPLE_KEY_ID",
		"WARRANTY_MCP_HTTP_TOKEN",
	},
	// Each may carry a password or query value, held from startup. The Apple pair is
	// here too: client.HoldAppleCredentials holds the parts that are credentials, and
	// listing a variable here is what keeps it out of a third-party child (dsmemberutil),
	// where the kit would refuse a value it holds. The connector's own child gets it back.
	URLEnv: []string{"WARRANTY_TOKEN_URL", "WARRANTY_FEEDBACK_ENDPOINT", "WARRANTY_APPLE_CLIENT_ID", "WARRANTY_APPLE_PRIVATE_KEY"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
		"dell_client_id",
		"dell_client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
		"dell_client_id",
		"dell_client_secret",
	},
	CredentialFlags:   []string{"--client-id", "--client-secret"},
	CredentialHeaders: []string{"ClientID"},
	CredentialFields:  []string{"client_id", "client_secret", "refresh_token", "client_assertion"},
	// Both token endpoints answer with these. The transport holds them before
	// the program reads the body; each mint then binds its token to its vendor
	// (client.SourceDellMint, minted:apple-token).
	ResponseFields: []string{"access_token", "refresh_token", "id_token"},
	MCPArguments:   []string{"client_id", "client_secret", "client-id", "client-secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "WARRANTY_BASE_URL",
		Default:   dellAPI + "/PROD/sbil/eapi/v5",
	},
	Origins: []credkit.Origin{
		// The default is Dell: the config file and credential store are the
		// generated Dell credential, and every other vendor's value has its own
		// entry below.
		{Source: "*", Allowed: []string{"base_url", dellAPI}},
		{Source: client.SourceDellMint, Allowed: []string{"base_url", dellAPI}},
		{Source: "env:WARRANTY_LENOVO_CLIENT_KEY", Allowed: []string{lenovoAPI}},
		{Source: client.SourceAppleClientID, Allowed: []string{appleOAuth}},
		{Source: "env:WARRANTY_APPLE_KEY_ID", Allowed: []string{appleOAuth}},
		{Source: "minted:apple-assertion", Allowed: []string{appleOAuth}},
		{Source: "minted:apple-token", Allowed: []string{appleAPI}},
		// The key signs; it is sent nowhere.
		{Source: "minted:apple-key", Allowed: nil},
		// A response field gets no binding from being read: only the mint that
		// expected it binds it, to the vendor it asked.
		{Source: "response:access_token", Allowed: nil},
		{Source: "response:refresh_token", Allowed: nil},
		{Source: "response:id_token", Allowed: nil},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:WARRANTY_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "env:WARRANTY_APPLE_CLIENT_ID", Reason: "starts BUSINESSAPI., a word vendors' coverage names carry; client.HoldAppleCredentials holds the unique part as minted:apple-client-id"},
		{Source: "field:client_id", Reason: "the client ID as sent: Dell's is held from the environment or config, and the unique part of Apple's by client.HoldAppleCredentials"},
		{Source: "env:WARRANTY_APPLE_PRIVATE_KEY", Reason: "usually the key file's path, and every path under the same directory would be withheld with it; client.HoldAppleCredentials holds the key the value names, inline or in that file, as minted:apple-key"},
		// config:<key> for every key in cliutil.NotCredentials, which the
		// per-read hold leaves unheld too (TestIdentifiersMatchConfigNotCredentials).
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the Dell token endpoint URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:skip_tls_verify", Reason: "boolean switch"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:WARRANTY_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:WARRANTY_TOKEN_URL", Reason: "the Dell token endpoint URL, never a credential holder in itself"},
		{Source: "env:WARRANTY_OAUTH_SCOPE", Reason: "an OAuth scope name"},
		{Source: "env:WARRANTY_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:WARRANTY_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:WARRANTY_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:WARRANTY_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:WARRANTY_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:WARRANTY_DATA_DIR", Reason: "directory path"},
		{Source: "env:WARRANTY_STATE_DIR", Reason: "directory path"},
		{Source: "env:WARRANTY_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:WARRANTY_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:WARRANTY_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:WARRANTY_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:WARRANTY_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:WARRANTY_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:WARRANTY_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children, whether or not the call carried confirm: true"},
		{Source: "env:WARRANTY_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:WARRANTY_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:WARRANTY_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside a URL is held by the outbound guard when sent"},
		{Source: "env:WARRANTY_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:WARRANTY_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:WARRANTY_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:WARRANTY_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:WARRANTY_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles returns the config files the kit takes the base URL from: those
// of the last --home, or the discovered default. An explicit --config outranks them
// in the kit itself. Every other candidate file is only held, so a base_url in a
// file the program does not read never becomes an allowed origin. It also holds the
// Apple private key and the run profile stores, which are not config files.
func credentialFiles(args []string) []string {
	client.HoldAppleCredentials()
	homes := credkit.FlagValues(args, "--home")
	holdProfileStores("")
	holdFiles := func(files []string, _ bool) {
		for _, f := range files {
			cliutil.HoldConfigFilePath(f)
		}
	}
	for _, h := range homes {
		holdProfileStores(h)
		holdFiles(configFilesUnder("", h))
	}
	for _, c := range credkit.FlagValues(args, "--config") {
		holdFiles(configFilesUnder(c, ""))
	}
	last := ""
	if len(homes) > 0 {
		last = homes[len(homes)-1]
	}
	files, ok := configFilesUnder("", last)
	if !ok {
		files, _ = configFilesUnder("", "")
	} else if last != "" {
		holdFiles(configFilesUnder("", ""))
	}
	return files
}

// holdProfileStores holds the run profile store under home ("" is the default) and
// the legacy one. It is not a Files entry: the kit would hold every string in it.
func holdProfileStores(home string) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	if dir, err := cliutil.ConfigDir(); err == nil { // not profileStorePath, which creates the directory
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(h, ".warranty-cli", "profiles.json"))
	}
}

// configFilesUnder is config.ReadPaths(configPath) with --home set to home,
// as the program sets it before it loads the config. ok is false when the
// program would refuse that --home or the path cannot be resolved.
func configFilesUnder(configPath, home string) ([]string, bool) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return nil, false
	}
	defer restore()
	files, stores, err := config.ReadPaths(configPath)
	if err != nil {
		return nil, false
	}
	// A store is held, never offered as a config: its base_url is not an origin.
	for _, f := range stores {
		cliutil.HoldConfigFilePath(f)
	}
	return files, true
}
