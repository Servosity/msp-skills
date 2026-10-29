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

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/config"
	"pax8-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Pax8 authenticates with OAuth2 client credentials: the client ID and secret
// (PAX8_CLIENT_ID / PAX8_CLIENT_SECRET, config.toml, the 0600 credentials.toml
// store, or auth login --client-id / --client-secret) are POSTed in the form
// body (client.ClientCredentialsTokenForm) to Pax8's token manager, and the
// access token it answers is sent as a bearer to the Partner API. The client ID
// is held as a credential too (half of the pair, in no Partner API record).
//
// So the client pair may go only to Pax8's documented token host or the API
// base URL's origin (a PAX8_TOKEN_URL on any third host is refused, which fails
// closed; mimecast and cisco-umbrella bind theirs the same way), and every other
// credential only to the API base URL's origin. pax8_guard.go's origin pin (handfixes.json api-origin-pinned)
// is layered over the kit and narrows the token host to the one token POST.
//
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile, a declared exemption: the
// kit withholds them by contract) and both are named in Files. Every --stdin
// body is held whole before anything prints (the seven API writes and feedback
// share the flag name); a token auth set-token reads from stdin is held where
// it is read, and a minted token where it is minted.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"PAX8_CLIENT_ID",
		"PAX8_CLIENT_SECRET",
		// No code reads it; a cookie a Pax8 session could carry is declared so
		// that every 8-character piece of it (each cookie value inside
		// "sid=X; theme=dark") is held if an operator sets it.
		"PAX8_COOKIE",
		"PAX8_MCP_HTTP_TOKEN",
	},
	// These name an endpoint, so each variable is an identifier below, but a URL
	// can carry a password or query value: those are held.
	URLEnv:        []string{"PAX8_TOKEN_URL", "PAX8_FEEDBACK_ENDPOINT"},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	// auth login's flags. The MCP mirror never exposes auth (cobratree
	// classify.go), so no tool argument carries a credential.
	CredentialFlags: []string{"--client-id", "--client-secret"},
	StdinFlags:      []string{"--stdin"},
	// The token request's form fields, and the same names as a run profile
	// stores them (flag spelling), which HoldBody reads at startup.
	CredentialFields: []string{"client_id", "client_secret", "client-id", "client-secret"},
	// What the token endpoint answers. No Partner API record carries a
	// credential (research brief, spec.json).
	ResponseFields: []string{"access_token", "refresh_token", "id_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "PAX8_BASE_URL",
		Default:   "https://api.pax8.com/v1",
	},
	Origins: []credkit.Origin{
		// The bearer token and every other credential go only to the API base
		// URL's origin, never to a --deliver webhook, the feedback endpoint or
		// a redirect's host.
		{Source: "*", Allowed: []string{"base_url"}},
		// The client pair goes only to Pax8's token manager or the configured
		// API origin, wherever it came from (environment, config or store, auth
		// login's flags, the token form), never anywhere else.
		{Source: "env:PAX8_CLIENT_ID", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "env:PAX8_CLIENT_SECRET", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "config:client_id", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "config:client_secret", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "flag:--client-id", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "flag:--client-secret", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "field:client_id", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "field:client_secret", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "field:client-id", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		{Source: "field:client-secret", Allowed: []string{"base_url", "https://token-manager.pax8.com"}},
		// A cookie no code sends, and the MCP HTTP bearer, which authenticates
		// callers of this server: neither is sent anywhere.
		{Source: "env:PAX8_COOKIE", Allowed: nil},
		{Source: "env:PAX8_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Keys of config.toml and credentials.toml that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the OAuth2 token URL; a credential inside it is held through URLEnv's reading"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:PAX8_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:PAX8_TOKEN_URL", Reason: "the OAuth2 token URL; a credential inside it is held through URLEnv"},
		{Source: "env:PAX8_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and the outbound guard refuses any held value sent to it"},
		{Source: "env:PAX8_AUDIENCE", Reason: "the OAuth2 audience the token request names (api://p8p.client, from Pax8's documentation), not a credential"},
		{Source: "env:PAX8_OAUTH_SCOPE", Reason: "the OAuth2 scope the token request names, not a credential"},
		{Source: "env:PAX8_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:PAX8_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:PAX8_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:PAX8_CACHE_DIR", Reason: "directory path"},
		{Source: "env:PAX8_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:PAX8_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:PAX8_DATA_DIR", Reason: "directory path"},
		{Source: "env:PAX8_STATE_DIR", Reason: "directory path"},
		{Source: "env:PAX8_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:PAX8_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:PAX8_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:PAX8_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:PAX8_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:PAX8_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:PAX8_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:PAX8_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:PAX8_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:PAX8_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:PAX8_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:PAX8_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:PAX8_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:PAX8_USER_AGENT", Reason: "User-Agent string"},
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
// PAX8_CONFIG wins, as it does in config.Load, and the last --home decides the
// default paths, as pflag does. Every other file any --config or --home could
// mean is only held (cliutil.HoldFile): its values cannot print, and its
// base_url cannot become an allowed origin. No flag arity is guessed:
// credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("PAX8_CONFIG")
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
	// The run profile store is a source too (credential leak contract section 1,
	// orchestrator item 8c: a value under a credential flag's name or under
	// base-url), read here and not in the returned list: it is nested JSON, which
	// the kit's file reader answers by holding every string in it, so a profile
	// named "p" or a saved --limit 7 would refuse every later start.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldProfileFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileFile(filepath.Join(home, ".pax8-cli", "profiles.json"))
	}
	return files
}
