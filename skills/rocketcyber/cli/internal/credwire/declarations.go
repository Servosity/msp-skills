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
	"strings"

	"rocketcyber-pp-cli/internal/cliutil"
	"rocketcyber-pp-cli/internal/config"
	"rocketcyber-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// The RocketCyber Customer API v3 authenticates with one bearer token
// (Authorization: Bearer <ROCKETCYBER_API_TOKEN>), from the environment,
// config.toml or the 0600 credentials.toml store. It is read-only (GET only)
// and answers no credential, so no response field or request body field is
// declared. The store and config.toml are credential SOURCES: their own writes
// go outside the kit (cliutil.AtomicWriteCredentialFile, a declared exemption:
// the kit withholds them by contract) and both are named in Files. No command
// reads a credential from a flag; the one --stdin flag (feedback) reads prose,
// so StdinFlags is empty, and auth set-token holds the token it reads from
// stdin itself (credkit.Hold("stdin", ...)).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"ROCKETCYBER_API_TOKEN",
		"ROCKETCYBER_MCP_HTTP_TOKEN",
	},
	// A token endpoint the generated config names; no command reads it, but its
	// userinfo password and query values are held so a typed-back one never prints.
	URLEnv:            []string{"ROCKETCYBER_TOKEN_URL"},
	Files:             credentialFiles,
	ConfigFields:      []string{"auth_header", "api_token", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:     []string{"auth_header", "api_token", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialHeaders: []string{"Authorization"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "ROCKETCYBER_BASE_URL",
		Default:   "https://api-us.rocketcyber.com/v3",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for:
		// the base URL's origin, nothing else (no regional host is added).
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:ROCKETCYBER_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Keys of config.toml and credentials.toml that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:ROCKETCYBER_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:ROCKETCYBER_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:ROCKETCYBER_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:ROCKETCYBER_CACHE_DIR", Reason: "directory path"},
		{Source: "env:ROCKETCYBER_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:ROCKETCYBER_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:ROCKETCYBER_DATA_DIR", Reason: "directory path"},
		{Source: "env:ROCKETCYBER_STATE_DIR", Reason: "directory path"},
		{Source: "env:ROCKETCYBER_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:ROCKETCYBER_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:ROCKETCYBER_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:ROCKETCYBER_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		// Not in URLEnv: its origin cannot be declared, so a held query value would refuse the feedback POST itself.
		{Source: "env:ROCKETCYBER_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a held credential sent to it is refused by the outbound guard"},
		{Source: "env:ROCKETCYBER_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:ROCKETCYBER_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:ROCKETCYBER_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:ROCKETCYBER_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:ROCKETCYBER_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:ROCKETCYBER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:ROCKETCYBER_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:ROCKETCYBER_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:ROCKETCYBER_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:ROCKETCYBER_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:ROCKETCYBER_USER_AGENT", Reason: "User-Agent string"},
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
// ROCKETCYBER_CONFIG wins, as it does in config.Load, and the last --home decides
// the default paths, as pflag does. Every other file any --config or --home could
// mean is only held (cliutil.HoldFile): its values cannot print, and its base_url
// cannot become an allowed origin. No flag arity is guessed: credkit.FlagValues
// counts every occurrence.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("ROCKETCYBER_CONFIG")
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
// was named and the default is absent, then the credential stores (the one beside
// an explicit config, and the default one).
func runFiles(explicit string) []string {
	var files []string
	if strings.TrimSpace(explicit) != "" {
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
	// The run profile store is a source too (credential leak contract section 1:
	// a value under a credential flag's name or under base-url), read here and not
	// in the returned list: it is nested JSON, which the kit's file reader answers
	// by holding every string in it, so a profile named "p" or a saved --limit 7
	// would refuse every later start. HoldFile reads it with the kit's HoldBody,
	// which holds the declared credential names at any depth.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldFile(filepath.Join(home, ".rocketcyber-cli", "profiles.json"))
	}
	return files
}
