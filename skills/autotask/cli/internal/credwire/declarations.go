// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"strings"

	"autotask-pp-cli/internal/cliutil"
	"autotask-pp-cli/internal/config"
	"autotask-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Autotask authenticates every call with three headers: ApiIntegrationCode,
// UserName and Secret, from the environment, config.toml or the 0600
// credentials.toml store (auth set-token writes the integration code there).
// The integration code and the Secret are the credentials. The UserName is the
// API user's login name, an email address in the tenant's own domain: Autotask
// answers repeat it and its domain (Resources, Contacts), and it authenticates
// nothing without the Secret, so every source of it is an Identifier below
// (contract section 1). Held, it withheld every answer naming a colleague at
// that domain and failed the sync of such a record (.review kit finding 10).
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile) and they are named in Files,
// because the kit would withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"AUTOTASK_API_INTEGRATION_CODE",
		"AUTOTASK_PSA_SECRET",
		"AUTOTASK_MCP_HTTP_TOKEN",
	},
	// The feedback endpoint is a URL: a password or query value in it is held.
	URLEnv: []string{"AUTOTASK_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"api_integration_code",
		"psa_secret",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"api_integration_code",
		"psa_secret",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"ApiIntegrationCode", "Secret"},
	// The spec's credential members (internal/cliutil.MaskedResponseFields): a
	// webhook's signing secretKey, a client-portal user's password, an
	// integration vendor insight's or widget's secret.
	CredentialFields: []string{"secretKey", "password", "secret"},
	ResponseFields:   []string{"secretKey", "password", "secret"},
	// A caller that passes the integration code's or the Secret's header name
	// as a tool argument passes a credential (matched in any case, "secret"
	// covers Secret).
	MCPArguments: []string{"ApiIntegrationCode", "secretKey", "password", "secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "AUTOTASK_BASE_URL",
		Default:   "https://webservices5.autotask.net/atservicesrest/V1.0",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API base URL this process uses. The
		// connector's own origin pin (api-origin-pinned) narrows every request
		// to that one origin, redirects included.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:AUTOTASK_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// The UserName, from every source it is read from or sent through. zone
		// reads AUTOTASK_USERNAME and --user for the same login name.
		{Source: "env:AUTOTASK_PSA_USER_NAME", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "env:AUTOTASK_USERNAME", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "config:psa_user_name", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "header:UserName", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "flag:--user", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "mcp:user", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "mcp:UserName", Reason: "the Autotask API user's login name, an email address in the tenant's own domain; Autotask answers repeat it and its domain (Resources, Contacts), and it authenticates nothing without Secret"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:AUTOTASK_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:AUTOTASK_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:AUTOTASK_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:AUTOTASK_CACHE_DIR", Reason: "directory path"},
		{Source: "env:AUTOTASK_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:AUTOTASK_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:AUTOTASK_DATA_DIR", Reason: "directory path"},
		{Source: "env:AUTOTASK_STATE_DIR", Reason: "directory path"},
		{Source: "env:AUTOTASK_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:AUTOTASK_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:AUTOTASK_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:AUTOTASK_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:AUTOTASK_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; its password and query values are held through URLEnv"},
		{Source: "env:AUTOTASK_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:AUTOTASK_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:AUTOTASK_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:AUTOTASK_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:AUTOTASK_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:AUTOTASK_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:AUTOTASK_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:AUTOTASK_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:AUTOTASK_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:AUTOTASK_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles runs inside Start, before anything can print. It returns the
// files the kit reads as this run's own, the config the program will really use
// FIRST, because the kit takes the base URL (and with it every credential's
// allowed origin) from the first file naming one: an explicit --config or
// AUTOTASK_CONFIG wins, as it does in config.Load, and the last --home decides
// the default paths, as pflag does. Every other file any --config or --home
// could mean is only held: its values cannot print, and its base_url cannot
// become an allowed origin. No flag arity is guessed: credkit.FlagValues counts
// every occurrence.
func credentialFiles(args []string) []string {
	cliutil.CompactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	cliutil.StartupConfigFlag, cliutil.StartupHomeFlag = explicit, ""
	if len(homes) > 0 {
		cliutil.StartupHomeFlag = homes[len(homes)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("AUTOTASK_CONFIG")
	}
	var used []string
	if len(homes) == 0 {
		used = runFiles(explicit)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		defaults := runFiles("")
		restore()
		holdOnly(defaults)
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	for _, cfg := range configs {
		holdOnly(runFiles(cfg))
	}
	return used
}

func holdOnly(files []string) {
	for _, f := range files {
		cliutil.HoldConfigFilePath(f)
	}
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, the legacy config when no explicit one
// was named and the default is absent. The credential stores and the run
// profile stores are held here.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); err != nil {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	// The credential stores are only held: config.Load takes no base URL from a
	// store, so one written there must not become an allowed origin.
	if explicit != "" {
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			cliutil.HoldConfigFilePath(store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		cliutil.HoldConfigFilePath(store)
	}
	// The run profile store is a source too (contract item 8c): a profile value
	// under a credential flag's name is held by the kit's body reader, every
	// duplicate included. It is held here, not returned: the kit's file reader
	// answers nested JSON by holding every string in it.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".autotask-cli", "profiles.json"))
	}
	return files
}
