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

	"datto-saas-pp-cli/internal/cliutil"
	"datto-saas-pp-cli/internal/config"
	"datto-saas-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Datto SaaS Protection authenticates with HTTP Basic: the public key is the user
// name and the secret key the password, from DATTO_SAAS_PUBLIC_KEY and
// DATTO_SAAS_SECRET_KEY, config.toml or the 0600 credentials.toml store. The store
// and config.toml are credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile), because the kit would withhold them by
// contract. config.toml is named in Files; the store is held by credentialFiles. The API takes and returns no other credential:
// its one write (bulkSeatChange) carries seat ids, and no documented response
// carries a secret, so ResponseFields stays empty. A --stdin body and an MCP tool
// call are input this program did not write: a member under one of these names, at
// any depth and every duplicate (an MCP frame's repeated arguments member too), is
// held by the kit (credkit.HoldBody when a body is read; MCPIn and MCPHTTP on the
// raw frame) before a preview or a send. Free text is not read (contract section 1).
// The kit folds case and nothing else, so each name is listed in its snake, kebab and
// joined spellings (camelCase folds to the joined one).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"DATTO_SAAS_PUBLIC_KEY",
		"DATTO_SAAS_SECRET_KEY",
		"DATTO_SAAS_MCP_HTTP_TOKEN",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "saas_public_key", "saas_secret_key", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:    []string{"auth_header", "saas_public_key", "saas_secret_key", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialFields: []string{"password", "passwd", "secret", "token", "bearer", "cookie", "credentials", "authorization", "client_id", "x-api-key", "x_api_key", "api_key", "api-key", "apikey", "client_secret", "client-secret", "clientsecret", "access_token", "access-token", "accesstoken", "refresh_token", "refresh-token", "refreshtoken", "id_token", "id-token", "idtoken", "session_token", "session-token", "sessiontoken", "private_key", "private-key", "privatekey", "public_key", "public-key", "publickey", "secret_key", "secret-key", "secretkey", "auth_header", "auth-header", "authheader", "saas_public_key", "saas-public-key", "saaspublickey", "saas_secret_key", "saas-secret-key", "saassecretkey"},
	MCPArguments:     []string{"password", "passwd", "secret", "token", "bearer", "cookie", "credentials", "authorization", "client_id", "x-api-key", "x_api_key", "api_key", "api-key", "apikey", "client_secret", "client-secret", "clientsecret", "access_token", "access-token", "accesstoken", "refresh_token", "refresh-token", "refreshtoken", "id_token", "id-token", "idtoken", "session_token", "session-token", "sessiontoken", "private_key", "private-key", "privatekey", "public_key", "public-key", "publickey", "secret_key", "secret-key", "secretkey", "auth_header", "auth-header", "authheader", "saas_public_key", "saas-public-key", "saaspublickey", "saas_secret_key", "saas-secret-key", "saassecretkey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DATTO_SAAS_BASE_URL",
		Default:   "https://api.datto.com/v1",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for. The
		// connector's redirect rule narrows every request to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:DATTO_SAAS_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's TLS private key is sent nowhere.
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:logged_out", Reason: "boolean marker in the credential store"},
		{Source: "env:DATTO_SAAS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DATTO_SAAS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DATTO_SAAS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DATTO_SAAS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:DATTO_SAAS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DATTO_SAAS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:DATTO_SAAS_DATA_DIR", Reason: "directory path"},
		{Source: "env:DATTO_SAAS_STATE_DIR", Reason: "directory path"},
		{Source: "env:DATTO_SAAS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:DATTO_SAAS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DATTO_SAAS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DATTO_SAAS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DATTO_SAAS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:DATTO_SAAS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DATTO_SAAS_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:DATTO_SAAS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:DATTO_SAAS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DATTO_SAAS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DATTO_SAAS_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:DATTO_SAAS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DATTO_SAAS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DATTO_SAAS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DATTO_SAAS_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles runs inside Start, before anything can print. It takes a
// parent's credential hand-off, then returns the files the kit reads as this
// run's own, the config the program will really use FIRST, because the kit takes
// the base URL (and with it every credential's allowed origin) from the first
// file naming one: an explicit --config or DATTO_SAAS_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held: its values
// cannot print, and its base_url cannot become an allowed origin. No flag arity is
// guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	cliutil.ReadCredentialHandoff()
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	} else {
		explicit = os.Getenv("DATTO_SAAS_CONFIG")
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

// HoldSetCredentialsArgs holds every word after set-credentials on the command
// line. The CLI's main calls it right after Start, not Files, so that argv never
// reaches a file read: it holds every word on the command
// line, dashes and flag names included, because no flag scan sees a positional
// credential and any word could be one: an extra word held is safe, a token
// missed is not. The one word left out is the first bare -- separator.
func HoldSetCredentialsArgs(args []string) {
	i := slices.Index(args, "set-credentials")
	if i < 0 {
		return
	}
	words := args[i+1:]
	dashes := slices.Index(words, "--")
	for j, a := range words {
		if j != dashes {
			cliutil.Hold("config:saas_secret_key", a)
		}
	}
}

func holdOnly(files []string) {
	for _, f := range files {
		cliutil.HoldConfigFilePath(f)
	}
}

// runFiles is the configuration a run with this explicit config (or none) reads, in
// the order config.Load prefers it: the config file, then the legacy config when no
// explicit one was named and the default is absent. Only these go back to the kit,
// which takes the allowed origin from the first file naming a base URL: config.Load
// never reads one from a credential store, so a store is only held, like every other
// file this run does not use.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			holdOnly([]string{store})
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
		holdOnly([]string{store})
	}
	// The run profile store is a source too, read here and not in the returned
	// list: it is nested JSON, which the kit's file reader answers by holding every
	// string in it, so a profile named "p" or a saved --limit 7 would refuse every
	// later start. Its fields are held by name and URL, and its values pass the
	// allow-list in loadProfileStore.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".datto-saas-cli", "profiles.json"))
	}
	return files
}
