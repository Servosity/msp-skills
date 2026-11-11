// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"hook-security-pp-cli/internal/cliutil"
	"hook-security-pp-cli/internal/config"
	"hook-security-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Hook authenticates with one static API key (HOOK_SECURITY_API_KEY,
// config.toml's security_api_key or the 0600 credentials.toml store), sent as a
// bearer to the configured base URL and nowhere else. Nothing is minted or
// refreshed, and the API is read-only: no request body, flag or MCP argument
// carries a credential, and no response field is one.
//
// feedback --stdin is deliberately not in StdinFlags: it reads a note for the
// feedback ledger, never a request body or a credential, and the kit holds each
// line of a declared stdin whole, so every ledger write would be withheld.
//
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"HOOK_SECURITY_API_KEY",
		"HOOK_SECURITY_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"HOOK_SECURITY_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files: credentialFiles,
	// headers is the config's [headers] table: every value in it is sent on
	// each request, so each is held as a credential.
	ConfigFields:  []string{"security_api_key", "auth_header", "headers", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"security_api_key", "auth_header", "headers", "access_token", "refresh_token", "client_secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "HOOK_SECURITY_BASE_URL",
		Default:   "https://api.hooksecurity.co",
	},
	Origins: []credkit.Origin{
		// Every API credential goes only to the configured base URL.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:HOOK_SECURITY_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; Hook uses neither (static API key)"},
		{Source: "env:HOOK_SECURITY_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:HOOK_SECURITY_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:HOOK_SECURITY_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:HOOK_SECURITY_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:HOOK_SECURITY_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:HOOK_SECURITY_DATA_DIR", Reason: "directory path"},
		{Source: "env:HOOK_SECURITY_STATE_DIR", Reason: "directory path"},
		{Source: "env:HOOK_SECURITY_CACHE_DIR", Reason: "directory path"},
		{Source: "env:HOOK_SECURITY_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:HOOK_SECURITY_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:HOOK_SECURITY_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:HOOK_SECURITY_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:HOOK_SECURITY_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:HOOK_SECURITY_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:HOOK_SECURITY_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:HOOK_SECURITY_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:HOOK_SECURITY_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:HOOK_SECURITY_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:HOOK_SECURITY_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:HOOK_SECURITY_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:HOOK_SECURITY_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:HOOK_SECURITY_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:HOOK_SECURITY_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:SPACEFAST_API_URL", Reason: "Spacefast API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:SPACEFAST_TEAM", Reason: "Spacefast team id, not a credential"},
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
		{Source: "env:PP_FILELOCK_HOLDER", Reason: "file-lock test helper switch"},
		{Source: "env:PP_FILELOCK_PATH", Reason: "file-lock test helper path"},
		{Source: "env:PP_FILELOCK_READY", Reason: "file-lock test helper path"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or HOOK_SECURITY_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("HOOK_SECURITY_CONFIG")
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
		holdOnly(runFiles(""))
		holdDirSources()
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	holdDirSources()
	for _, cfg := range configs {
		holdOnly(runFiles(cfg))
	}
	return used
}

// startupNamedConfigOrHome records that the command line named a --config or
// --home, which Start read. Set once, inside Start, before main goes on.
var startupNamedConfigOrHome bool

// StartupNamedConfigOrHome reports whether the command line named a --config
// or --home that Start read. The MCP server has neither flag, so it refuses to
// run when Start took a config, and with it an allowed origin, from one.
func StartupNamedConfigOrHome() bool { return startupNamedConfigOrHome }

// runFiles is what a run with this explicit config (or none) reads, in the
// order config.Load prefers it: the config file, then the legacy config when no
// explicit one was named and the default is absent. The credential stores are
// held here, not returned: config.Load takes no base URL from a store, so one
// written there must not become an allowed origin.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			holdFile(store)
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
		holdFile(store)
	}
	return files
}

func holdOnly(files []string) {
	for _, f := range files {
		holdFile(f)
	}
}

// holdDirSources holds the run-profile stores (current and legacy): a
// profile value saved under a credential flag's name or under base-url is a
// credential source. Each is a JSON document the kit reads by its declared
// names (credkit.HoldBody); a file that does not parse is held whole, every
// word of it.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".hook-security-cli", "profiles.json"))
	}
}

// holdFile holds what one credential source carries, read by declared name.
// A missing file holds nothing.
func holdFile(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
}

// readRegular reads path only when it is a regular file, so a FIFO or a device
// named as a source cannot stall the start.
func readRegular(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(filepath.Clean(path)) // #nosec G304 -- a credential source this program reads anyway
}
