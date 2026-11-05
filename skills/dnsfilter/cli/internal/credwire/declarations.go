// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/config"
	"dnsfilter-pp-cli/internal/credkit"
)

// ServerBinary is set by the MCP server's init. An environment marker cannot say this:
// any caller can set one.
var ServerBinary bool

// Declarations is what the connector holds and where each credential may go.
// DNSFilter authenticates with one API key, from DNSFILTER_API_KEY, config.toml
// or the 0600 credentials.toml store.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"DNSFILTER_API_KEY",
		"DNSFILTER_MCP_HTTP_TOKEN",
	},
	Files:         credentialFiles,
	ConfigFields:  cliutil.CredentialConfigKeys,
	StoredRecords: cliutil.CredentialConfigKeys,
	CredentialFlags: []string{
		"--new-password", "--options-client-secret", "--payment-token",
	},
	// A --stdin body is held whole and line by line, whatever it is: --dry-run, a flag
	// error and the answer that repeats part of it are all withheld.
	StdinFlags:       []string{"--stdin"},
	CredentialFields: cliutil.CredentialResponseFields,
	ResponseFields:   cliutil.CredentialResponseFields,
	MCPArguments:     cliutil.CredentialResponseFields,
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DNSFILTER_BASE_URL",
		Default:   "https://api.dnsfilter.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for.
		// The connector's redirect rule narrows every request to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:DNSFILTER_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the
		// per-read hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:logged_out", Reason: "boolean marker in the credential store"},
		{Source: "env:DNSFILTER_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DNSFILTER_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DNSFILTER_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DNSFILTER_CACHE_DIR", Reason: "directory path"},
		{Source: "env:DNSFILTER_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DNSFILTER_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:DNSFILTER_DATA_DIR", Reason: "directory path"},
		{Source: "env:DNSFILTER_STATE_DIR", Reason: "directory path"},
		{Source: "env:DNSFILTER_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:DNSFILTER_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DNSFILTER_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DNSFILTER_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DNSFILTER_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:DNSFILTER_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DNSFILTER_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:DNSFILTER_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:DNSFILTER_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DNSFILTER_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DNSFILTER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:DNSFILTER_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DNSFILTER_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DNSFILTER_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DNSFILTER_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles runs inside Start, before anything can print. It holds the
// credential auth set-token will read, and names every file a credential may be read
// from under any --config and --home the command line can mean: the DNSFILTER_CONFIG
// selection, config.toml under each config directory rung and credentials.toml under
// each data directory rung, the legacy config, and each explicit config with its sibling
// store. Load falls back to the global store from an explicit config, and that is always
// listed, so the list is a superset of what any run reads: holding a file the run does
// not load is safe, missing the one it loads is not. The kit takes the base URL from the
// first listed file that names one, so the list runs in Load's order; an explicit
// --config it reads itself and prefers.
// The HTTP server's --tls-key is listed too: a PEM file is no config, so the kit
// holds every string in it. The flag is the server's alone: a CLI's command line can
// come from a tool call, so the CLI never opens that path.
func credentialFiles(args []string) []string {
	cliutil.HoldStdinToken(args)
	cliutil.CompactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	var files []string
	add := func(path string) {
		if path != "" && !slices.Contains(files, path) {
			files = append(files, path)
		}
	}
	selected := os.Getenv("DNSFILTER_CONFIG")
	add(selected)
	for _, dir := range cliutil.StartupKindDirs(cliutil.PathKindConfig, homes) {
		add(filepath.Join(dir, "config.toml"))
		holdProfileStore(dir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".dnsfilter-cli")) // the legacy store
	}
	for _, dir := range cliutil.StartupKindDirs(cliutil.PathKindData, homes) {
		add(filepath.Join(dir, "credentials.toml"))
	}
	if legacy, err := config.LegacyConfigPath(); err == nil {
		add(legacy)
	}
	explicit := credkit.FlagValues(args, "--config")
	if selected != "" {
		explicit = append(explicit, selected)
	}
	for _, cfg := range explicit {
		add(cfg)
		if creds, err := cliutil.CredentialsFilePathForConfig(cfg); err == nil {
			add(creds)
		}
	}
	if ServerBinary {
		for _, key := range credkit.FlagValues(args, "--tls-key") {
			add(key)
		}
	}
	return files
}

// holdProfileStore holds what a run profile store carries. The kit is not given this
// file: it is nested JSON, so the kit would hold every string in it, and a profile's
// plain "true" in public text would refuse every start. Only a regular file is read:
// a pipe named profiles.json would hang startup.
func holdProfileStore(dir string) {
	store := os.DirFS(dir)
	if info, err := fs.Stat(store, "profiles.json"); err != nil || !info.Mode().IsRegular() {
		return
	}
	if data, err := fs.ReadFile(store, "profiles.json"); err == nil {
		cliutil.HoldProfileStore(data)
	}
}
