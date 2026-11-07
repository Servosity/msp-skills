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

	"betterstack-pp-cli/internal/cliutil"
	"betterstack-pp-cli/internal/config"
	"betterstack-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Better Stack Uptime authenticates with one API token sent as a Bearer
// Authorization header (always held by the kit), from BETTERSTACK_API_TOKEN,
// config.toml or the 0600 credentials.toml store; there is no token endpoint.
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile) and they are named in
// Files, because the kit would withhold them by contract.
//
// The API also returns and accepts credentials of the sites a monitor probes:
// a monitor's auth_username and auth_password (HTTP basic auth for the probed
// URL) and its request_headers (name and value pairs, an Authorization or
// X-Api-Key header among them, so the whole list is held), and an escalation
// policy's incident_token (the token that opens incidents through the
// policy's integrations). The kit compares a field name ignoring case only,
// so each separator spelling of the monitor's credential pair is listed
// (authUser is AUTHUSER and authuser). The spec pin is
// TestDeclarationsMatchTheSpec. A heartbeat's url is the ingest address the
// operator configures a job with, and the same field name is every monitor's
// probed address and every status page's address, so it is data, not a
// declared credential (see .review/followups.md).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"BETTERSTACK_API_TOKEN",
		"BETTERSTACK_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"BETTERSTACK_FEEDBACK_ENDPOINT",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "api_token", "access_token", "refresh_token", "client_id", "client_secret", "headers"},
	StoredRecords:    []string{"auth_header", "api_token", "access_token", "refresh_token", "client_id", "client_secret"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"auth_username", "authUsername", "auth_user", "authUser", "auth_password", "authPassword", "request_headers", "incident_token"},
	ResponseFields:   []string{"auth_username", "authUsername", "auth_user", "authUser", "auth_password", "authPassword", "request_headers", "incident_token"},
	MCPArguments:     []string{"auth_username", "authUsername", "auth_user", "authUser", "auth_password", "authPassword", "request_headers", "incident_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "BETTERSTACK_BASE_URL",
		Default:   "https://uptime.betterstack.com/api/v2",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for.
		// The connector's origin pin (internal/cli/betterstack_guard.go) narrows
		// every request and redirect to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:BETTERSTACK_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's TLS private key is sent nowhere.
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:BETTERSTACK_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:BETTERSTACK_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:BETTERSTACK_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:BETTERSTACK_CACHE_DIR", Reason: "directory path"},
		{Source: "env:BETTERSTACK_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:BETTERSTACK_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:BETTERSTACK_DATA_DIR", Reason: "directory path"},
		{Source: "env:BETTERSTACK_STATE_DIR", Reason: "directory path"},
		{Source: "env:BETTERSTACK_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:BETTERSTACK_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:BETTERSTACK_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:BETTERSTACK_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:BETTERSTACK_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:BETTERSTACK_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:BETTERSTACK_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:BETTERSTACK_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:BETTERSTACK_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:BETTERSTACK_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:BETTERSTACK_USER_AGENT", Reason: "User-Agent string"},
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
// BETTERSTACK_CONFIG wins, as it does in config.Load, and the last --home decides the
// default paths, as pflag does. Every other file any --config or --home could
// mean is only held: its values cannot print, and its base_url cannot become an
// allowed origin. No flag arity is guessed: credkit.FlagValues counts every
// occurrence.
func credentialFiles(args []string) []string {
	cliutil.CompactStdinBody(args)
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
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
		explicit = os.Getenv("BETTERSTACK_CONFIG")
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
// was named and the default is absent. The credential stores and the run profile
// stores are held here and not returned.
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
	// config.Load takes no base URL from a store, so one written there must not
	// become an allowed origin.
	if explicit != "" {
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			cliutil.HoldConfigFilePath(store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		cliutil.HoldConfigFilePath(store)
	}
	// The run profile store is nested JSON, which the kit's file reader answers
	// by holding every string in it, so a profile named "p" would refuse every
	// later start. Its credential flags and fields are held by name.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".betterstack-cli", "profiles.json"))
	}
	return files
}
