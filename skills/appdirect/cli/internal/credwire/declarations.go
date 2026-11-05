// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): this connector's one credkit.Declarations
// literal, which both mains pass to credkit.Start. Pinned by this connector's
// handfixes.json under the "credkit-wiring" entry. The SQLite driver every
// read-write store opens through is internal/kitsql.
//
// The credential store (credentials.toml, 0600) and config.toml are credential
// SOURCES: their writes (a token save, auth set-token, auth login, auth logout,
// a config rewrite that keeps a [headers] table) go outside the kit, which
// withholds them by contract, and credentialFiles names every one of them.

// Package credwire wires the shared credential kit for this connector.
package credwire

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/pelletier/go-toml/v2"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/config"
	"appdirect-pp-cli/internal/credkit"
)

// Declarations is what this connector declares about its credentials.
// AppDirect authenticates with an OAuth client ID and secret, exchanged at the
// marketplace's /oauth2/token for a bearer token.
var Declarations = credkit.Declarations{
	SecretEnv: []string{"APPDIRECT_CLIENT_ID", "APPDIRECT_CLIENT_SECRET", "APPDIRECT_MCP_HTTP_TOKEN"},
	// These name an endpoint; a password or query value inside one is held.
	URLEnv:        []string{"APPDIRECT_TOKEN_URL", "APPDIRECT_FEEDBACK_ENDPOINT"},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	// auth login takes the pair as flags; user and payment-method commands
	// take a password, a temporary password or a payment token.
	CredentialFlags: []string{"--client-id", "--client-secret", "--password", "--temporary-password", "--token"},
	// Every generated write command takes its JSON body on --stdin.
	StdinFlags: []string{"--stdin"},
	// Request body fields: the token exchange's form, and the spec's password
	// and payment-token fields.
	CredentialFields: []string{"client_id", "client_secret", "password", "temporaryPassword", "marketplacePassword", "token"},
	// Vendor response fields: the token response and the spec's credential
	// fields. "secret" is a boolean in the spec, so it is not one.
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "accessToken", "refreshToken",
		"client_secret", "clientSecret", "password", "temporaryPassword", "marketplacePassword", "token",
	},
	// MCP arguments: the endpoint tools' body fields, and the flag names the
	// command mirror and a saved run profile (profiles.json) use.
	MCPArguments: []string{
		"password", "temporaryPassword", "marketplacePassword", "token", "client_secret", "clientSecret",
		"client-id", "client-secret", "temporary-password",
	},
	BaseURL: credkit.BaseURL{ConfigKey: "base_url", Env: "APPDIRECT_BASE_URL", Default: "https://marketplace.appdirect.com/api"},
	Origins: []credkit.Origin{
		// Every credential goes only to the marketplace the program is
		// configured for. Its token URL is /oauth2/token on that origin
		// (client.ResolveAppDirectTokenURL); AppDirect's own marketplace is
		// not an extra origin when another one is configured.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:APPDIRECT_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "config:token_url", Reason: "the token URL; credentialFiles holds a credential inside it"},
		{Source: "config:ca_cert", Reason: "a path to a CA certificate file"},
		{Source: "config:tls_server_name", Reason: "a TLS host name"},
		{Source: "config:token_expiry", Reason: "a timestamp"},
		{Source: "env:AGENT_ID", Reason: "an agent name, stored in a feedback entry"},
		{Source: "env:APPDIRECT_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:APPDIRECT_CA_CERT", Reason: "a path to a CA certificate file"},
		{Source: "env:APPDIRECT_CLI_PATH", Reason: "a path to the companion CLI"},
		{Source: "env:APPDIRECT_CONFIG", Reason: "a path to the configuration file, which Files lists"},
		{Source: "env:APPDIRECT_CONFIG_DIR", Reason: "a directory"},
		{Source: "env:APPDIRECT_DATA_DIR", Reason: "a directory"},
		{Source: "env:APPDIRECT_STATE_DIR", Reason: "a directory"},
		{Source: "env:APPDIRECT_CACHE_DIR", Reason: "a directory"},
		{Source: "env:APPDIRECT_DATA_SOURCE", Reason: "a mode name"},
		{Source: "env:APPDIRECT_FEEDBACK_AUTO_SEND", Reason: "a switch"},
		{Source: "env:APPDIRECT_FEEDBACK_ENDPOINT", Reason: "a URL; URLEnv holds a credential inside it"},
		{Source: "env:APPDIRECT_HOME", Reason: "a directory"},
		{Source: "env:APPDIRECT_LEARN_NO_CAPTURE", Reason: "a switch"},
		{Source: "env:APPDIRECT_LEARN_SESSION", Reason: "a session name, stored only as a hash"},
		{Source: "env:APPDIRECT_LEARN_SURFACE", Reason: "a surface name"},
		{Source: "env:APPDIRECT_MAX_RESPONSE_BYTES", Reason: "a size"},
		{Source: "env:APPDIRECT_MCP_CHILD", Reason: "a marker this program sets on its own child"},
		{Source: "env:APPDIRECT_MCP_HTTP_ALLOWED_ORIGINS", Reason: "browser origins allowed to call the MCP HTTP server"},
		{Source: "env:APPDIRECT_MCP_READ_ONLY", Reason: "a switch"},
		{Source: "env:APPDIRECT_MCP_UPLOAD_ROOT", Reason: "a directory"},
		{Source: "env:APPDIRECT_NO_CONFIG_WRITE", Reason: "a switch"},
		{Source: "env:APPDIRECT_NO_LEARN", Reason: "a switch"},
		{Source: "env:APPDIRECT_NO_LOCAL_DATA", Reason: "a switch"},
		{Source: "env:APPDIRECT_OAUTH_SCOPE", Reason: "OAuth scope names"},
		{Source: "env:APPDIRECT_TLS_SERVER_NAME", Reason: "a TLS host name"},
		{Source: "env:APPDIRECT_TOKEN_URL", Reason: "a URL; URLEnv holds a credential inside it"},
		{Source: "env:APPDIRECT_USER_AGENT", Reason: "a User-Agent string"},
		{Source: "env:NO_COLOR", Reason: "a display switch"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "the MCP transport name"},
		{Source: "env:PRESEED_DISABLED", Reason: "a switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "a profile name"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "a profile name"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "a test switch"},
		{Source: "env:TERM", Reason: "a terminal type"},
		{Source: "env:XDG_CACHE_HOME", Reason: "a directory"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "a directory"},
		{Source: "env:XDG_DATA_HOME", Reason: "a directory"},
		{Source: "env:XDG_STATE_HOME", Reason: "a directory"},
	},
}

// baseURLEnv is the variable the kit reads the configured base URL from.
const baseURLEnv = "APPDIRECT_BASE_URL"

// pinnedBase marks baseURLEnv as set by credentialFiles, and original is
// what it was before (had says whether it was set at all).
var (
	pinnedBase  bool
	original    string
	hadOriginal bool
)

// credentialFiles is every credential file the command line and environment
// can name, under every --home and --config they carry: the config file (an
// explicit one, else config.toml and its legacy path), the credentials.toml
// store beside an explicit config and the default one. Holding a file the
// program does not read is safe; missing the one it reads is not. It also
// holds what no declared key names (a credential inside config.toml's
// token_url; run-profile values under a credential flag's name) and makes the
// base URL the kit binds credentials to the one config.Load will use: a blank
// APPDIRECT_BASE_URL names no marketplace, and a config file the
// environment selects decides as --config does (Started restores it).
func credentialFiles(args []string) []string {
	homes, configs := credkit.FlagValues(args, "--home"), credkit.FlagValues(args, "--config")
	home, configFlag := last(homes), last(configs)
	if restore, err := cliutil.SetHomeOverride(home); err == nil {
		base := config.StartupBaseURL(configFlag)
		restore()
		if v, ok := os.LookupEnv(baseURLEnv); v != base {
			original, hadOriginal = v, ok
			pinnedBase = os.Setenv(baseURLEnv, base) == nil
		}
	}
	var files []string
	add := func(p string, err error) {
		if err == nil && p != "" && !slices.Contains(files, p) {
			files = append(files, p)
		}
	}
	for _, h := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(h)
		if err != nil {
			continue
		}
		for _, c := range append(configs, "") {
			path, explicit, err := config.ResolveConfigPath(c)
			add(path, err)
			if explicit && err == nil {
				add(cliutil.CredentialsFilePathForConfig(path))
			}
		}
		add(cliutil.CredentialsFilePath())
		add(config.LegacyConfigPath())
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfiles(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		holdProfiles(filepath.Join(userHome, ".appdirect-cli", "profiles.json"))
	}
	for _, f := range files {
		holdTokenURL(f)
	}
	return files
}

// Started undoes credentialFiles' base URL pin. Each main calls it right after
// credkit.Start, so config.Load reads the environment as the operator set it.
func Started() {
	if !pinnedBase {
		return
	}
	pinnedBase = false
	if hadOriginal {
		_ = os.Setenv(baseURLEnv, original)
	} else {
		_ = os.Unsetenv(baseURLEnv)
	}
}

// last is the value the flag parser settles on among what FlagValues found
// for a flag: the last one, as the kit chooses an explicit --config. Each call
// site names its flag as a constant, which ties the value to that flag for
// tools/maintainer/mcp_file_flags.
// ponytail: a value FlagValues over-reads ("--select --home x") can be picked;
// the pinned base then names no host the program sends to, so the kit refuses
// (fail closed). zammad's parsedValue is the precise form if that ever bites.
func last(found []string) string {
	if len(found) > 0 {
		return found[len(found)-1]
	}
	return ""
}

// holdProfiles holds a run profile store's values under every credential
// name Declarations declares (credkit.HoldBody reads every duplicate).
func holdProfiles(path string) {
	if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
		credkit.HoldBody(opened.Data)
	}
}

// holdTokenURL holds the password and query values of a config file's
// token_url: the kit reads the base URL's, and this URL is printed by a
// --dry-run of auth login.
func holdTokenURL(path string) {
	opened, err := cliutil.ReadOpenedConfig(path)
	if err != nil {
		return
	}
	var file struct {
		TokenURL string `toml:"token_url"`
	}
	if toml.Unmarshal(opened.Data, &file) == nil && file.TokenURL != "" {
		credkit.HoldURL("url:config:token_url", file.TokenURL)
	}
}
