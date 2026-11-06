// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"slices"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/config"
	"eset-protect-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// ESET Connect authenticates with an API user's email and password, which mint a
// one-hour bearer token; or with a bearer token the operator supplies. Each comes
// from the environment, config.toml or the 0600 credentials.toml store. The store and config.toml are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile) and they are named in Files, because the kit
// would withhold them by contract. The only credential a response carries besides the
// token grant's tokens is a device's identity token (deviceToken).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"ESET_PROTECT_PASSWORD",
		"ESET_PROTECT_TOKEN",
		"ESET_PROTECT_MCP_HTTP_TOKEN",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "protect_password", "protect_token", "access_token", "refresh_token", "client_secret"},
	StoredRecords:    []string{"auth_header", "protect_password", "protect_token", "access_token", "refresh_token", "client_secret"},
	CredentialFields: []string{"password", "refresh_token"},
	ResponseFields:   []string{"access_token", "refresh_token", "deviceToken"},
	MCPArguments:     []string{"access_token", "refresh_token", "deviceToken", "password", "client_secret", "protect_password", "protect_token", "auth_header"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "ESET_PROTECT_BASE_URL",
		Default:   "https://{region}.device-management.eset.systems",
	},
	Origins: []credkit.Origin{
		// ESET Connect has one host per service in each region
		// (<region>.<service>.eset.systems, the token service under
		// business-account.iam), so the base URL alone cannot name them and this
		// entry must name the domain. The connector's own pin narrows every
		// credentialed request to the configured region's hosts
		// (cli.esetHostAllower, handfixes.json credential-stays-in-its-region), and
		// its redirect rule to the same origin. An explicit base URL replaces them.
		{Source: "*", Allowed: []string{"base_url", "https://*.eset.systems"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:ESET_PROTECT_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's TLS private key is sent nowhere.
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "env:ESET_PROTECT_USERNAME", Reason: "the API user's email: it names a user and authenticates nothing without the password, and ESET lists it in its own answers"},
		{Source: "config:protect_username", Reason: "the API user's email, as ESET_PROTECT_USERNAME"},
		{Source: "config:client_id", Reason: "the stored token's stamp: region and a hash of the API user, not a credential"},
		{Source: "config:region", Reason: "the ESET data center's short name"},
		{Source: "config:template_vars.region", Reason: "the ESET data center's short name, as documented in governance.md"},
		{Source: "env:ESET_PROTECT_REGION", Reason: "the ESET data center's short name"},
		// Keys of config.toml and credentials.toml that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:logged_out", Reason: "boolean marker in the credential store"},
		{Source: "env:ESET_PROTECT_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:ESET_PROTECT_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:ESET_PROTECT_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:ESET_PROTECT_CACHE_DIR", Reason: "directory path"},
		{Source: "env:ESET_PROTECT_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:ESET_PROTECT_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:ESET_PROTECT_DATA_DIR", Reason: "directory path"},
		{Source: "env:ESET_PROTECT_STATE_DIR", Reason: "directory path"},
		{Source: "env:ESET_PROTECT_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:ESET_PROTECT_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:ESET_PROTECT_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:ESET_PROTECT_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		// Not in URLEnv: its origin cannot be declared, so a held query value would refuse the feedback POST itself.
		{Source: "env:ESET_PROTECT_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:ESET_PROTECT_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:ESET_PROTECT_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:ESET_PROTECT_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:ESET_PROTECT_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:ESET_PROTECT_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:ESET_PROTECT_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:ESET_PROTECT_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:ESET_PROTECT_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:ESET_PROTECT_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:ESET_PROTECT_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles runs inside Start, before anything can print. It returns the files the kit reads as this
// run's own, the config the program will really use FIRST, because the kit takes
// the base URL (and with it every credential's allowed origin) from the first
// file naming one: an explicit --config or ESET_PROTECT_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held: its values
// cannot print, and its base_url cannot become an allowed origin. No flag arity is
// guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	} else {
		explicit = os.Getenv("ESET_PROTECT_CONFIG")
	}
	var used, held []string
	if len(homes) == 0 {
		used = runFiles(explicit)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		held = append(held, runFiles("")...)
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			held = append(held, files...)
		}
	}
	held = append(held, runFiles("")...)
	for _, cfg := range configs {
		held = append(held, runFiles(cfg)...)
	}
	// Held outside Files, whose first base_url would become the allowed origin. A
	// file the kit reads itself (Files and every --config) is left to its reader.
	for _, f := range held {
		if !slices.Contains(used, f) && !slices.Contains(configs, f) {
			cliutil.HoldFile(f)
		}
	}
	return used
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, the legacy config when no explicit one
// was named and the default is absent, then the credential stores.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
		}
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); err != nil {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		files = append(files, store)
	}
	// The run profile store is a source too, read here and not in the returned
	// list: it is nested JSON, which the kit's file reader answers by holding every
	// string in it, so a profile named "p" or a saved --limit 7 would refuse every
	// later start. HoldFile holds its declared fields with the kit's HoldBody, and
	// its values pass the allow-list in loadProfileStore.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldFile(filepath.Join(home, ".eset-protect-cli", "profiles.json"))
	}
	return files
}
