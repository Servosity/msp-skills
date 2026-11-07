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

	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/config"
	"domain-cert-tracker-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// The connector has one credential file (config.toml: auth_header, the
// [headers] table, and any credential inside base_url) and one secret
// environment variable (the MCP HTTP bearer token). It has no credential
// store, no credential flags and no token it mints.
var Declarations = credkit.Declarations{
	// No StdinFlags: the one --stdin (feedback) reads the operator's free-text
	// note, no credential body, and holding each line would withhold the note.
	SecretEnv:    []string{"DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN"},
	Files:        credentialFiles,
	ConfigFields: []string{"auth_header"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DOMAIN_CERT_TRACKER_BASE_URL",
		Default:   "https://rdap.org",
	},
	Origins: []credkit.Origin{
		// auth_header, every [headers] value (their names are the
		// operator's, so they cannot be listed), and the credentials of the
		// config file and base_url go only to the configured base URL's
		// origin, never to crt.sh, IANA or an RDAP server a redirect names.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in config.NotCredentials, which the
		// per-read hold leaves unheld too (TestIdentifiersMatchConfigNotCredentials).
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "env:DOMAIN_CERT_TRACKER_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DOMAIN_CERT_TRACKER_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DOMAIN_CERT_TRACKER_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DOMAIN_CERT_TRACKER_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DOMAIN_CERT_TRACKER_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:DOMAIN_CERT_TRACKER_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:DOMAIN_CERT_TRACKER_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DOMAIN_CERT_TRACKER_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DOMAIN_CERT_TRACKER_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DOMAIN_CERT_TRACKER_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DOMAIN_CERT_TRACKER_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DOMAIN_CERT_TRACKER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:DOMAIN_CERT_TRACKER_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL, sent to itself; every message shows only its scheme and host"},
		{Source: "env:DOMAIN_CERT_TRACKER_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DOMAIN_CERT_TRACKER_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DOMAIN_CERT_TRACKER_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DOMAIN_CERT_TRACKER_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:DOMAIN_CERT_TRACKER_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles is every config file the command line could mean
// (handfixes.json credkit-startup-config-argv), resolved by config.Load's own
// rules (config.ReadPaths). A flag's value can look like a flag (--select
// --config=x), and holding a file the program does not read is safe while
// missing the one it reads is not, so every --config and --home value counts.
// The kit takes the base URL from the last --config; without one, from the first
// file named here, so the last --home comes first. With neither flag the
// program reads the environment's file or the default; a default the flags leave
// out is held when config.Load reads it, in the case a flag's value was only
// swallowed by another flag.
//
// The run profile store is a source too, but it is held here and not named: see
// config.HoldProfileStore.
func credentialFiles(args []string) []string {
	var out []string
	add := func(configPath, home string) {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil { // the program would refuse this --home too
			return
		}
		defer restore()
		if dir, err := cliutil.ConfigDir(); err == nil {
			config.HoldProfileStoreIn(dir)
		}
		files, err := config.ReadPaths(configPath)
		if err != nil {
			return
		}
		for _, f := range files {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	configs := credkit.FlagValues(args, "--config")
	homes := credkit.FlagValues(args, "--home")
	if len(configs)+len(homes) == 0 {
		add("", "")
	}
	for i := len(configs) - 1; i >= 0; i-- {
		add(configs[i], "")
	}
	for i := len(homes) - 1; i >= 0; i-- {
		add("", homes[i])
	}
	// The kit's URL reader takes every shape the program's errors can echo.
	config.HoldBaseURL(os.Getenv("DOMAIN_CERT_TRACKER_BASE_URL"))
	if home, err := os.UserHomeDir(); err == nil { // the legacy store loadProfileStore falls back to
		config.HoldProfileStoreIn(filepath.Join(home, ".domain-cert-tracker-cli"))
	}
	return out
}
