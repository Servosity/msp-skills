// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. The credential store and a config rewrite are credential
// SOURCES: they stay outside the kit (the kit withholds them by contract), and
// Files lists every one of those files. Hand-written; a reprint never
// regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"slices"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
var Declarations = credkit.Declarations{
	SecretEnv:       []string{"SUPEROPS_API_TOKEN", "SUPEROPS_MCP_HTTP_TOKEN"},
	Files:           credentialFiles,
	ConfigFields:    []string{"api_token", "access_token", "refresh_token", "client_id", "client_secret", "auth_header"},
	StoredRecords:   []string{"api_token", "access_token", "refresh_token", "client_id", "client_secret", "auth_header"},
	CredentialFlags: nil,
	// feedback --stdin is the one such flag, and it reads a free-text note. The
	// kit holds a declared body whole, which would refuse every note. raw's
	// --vars, from any source, goes to credkit.HoldBody before it prints.
	StdinFlags: nil,
	// The names credkit.HoldBody and the outbound guard hold a body's value under.
	CredentialFields: []string{"api_token", "access_token", "refresh_token", "id_token", "auth_token", "token", "client_id", "client_secret", "auth_header", "api_key", "apikey", "password", "passwd", "pwd", "secret", "private_key", "authorization"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SUPEROPS_BASE_URL",
		Default:   "https://api.superops.ai",
	},
	Origins: []credkit.Origin{
		// A credential goes to the one origin this run selected, which
		// selectKitBaseURL gives the kit with the region applied, never to the
		// other data center, a feedback endpoint, a --deliver webhook, a proxy
		// or a host a redirect names.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent
		// nowhere.
		{Source: "env:SUPEROPS_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "the token's expiry timestamp"},
		{Source: "config:subdomain", Reason: "the tenant name, sent as CustomerSubDomain and shown by doctor"},
		{Source: "config:region", Reason: "data center name (us or eu)"},
		{Source: "header:CustomerSubDomain", Reason: "the tenant name, not a secret to the operator or the vendor"},
		{Source: "header:Content-Type", Reason: "a media type"},
		{Source: "header:Accept", Reason: "a media type"},
		{Source: "header:User-Agent", Reason: "a client name"},
		{Source: "env:SUPEROPS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:SUPEROPS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SUPEROPS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:SUPEROPS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SUPEROPS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SUPEROPS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:SUPEROPS_DATA_DIR", Reason: "directory path"},
		{Source: "env:SUPEROPS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SUPEROPS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:SUPEROPS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL, a destination the operator chooses; a credential typed inside it is free text, which contract section 1 does not name"},
		{Source: "env:SUPEROPS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:SUPEROPS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SUPEROPS_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:SUPEROPS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SUPEROPS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SUPEROPS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:SUPEROPS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SUPEROPS_MCP_TOKEN_HELPER", Reason: "path of a helper program, not its output"},
		{Source: "env:SUPEROPS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SUPEROPS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SUPEROPS_REGION", Reason: "data center name (us or eu)"},
		{Source: "env:SUPEROPS_STATE_DIR", Reason: "directory path"},
		{Source: "env:SUPEROPS_SUBDOMAIN", Reason: "the tenant name, sent as CustomerSubDomain and shown by doctor"},
		{Source: "env:SUPEROPS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SUPEROPS_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles is every file a credential may be read from under any
// --config and --home the command line can mean. credkit.FlagValues reads past
// a "--" that pflag takes as a flag's value, and holding a file the run does
// not load is safe where missing the one it loads is not. The last value of a
// repeated flag comes first (pflag keeps the last), then the environment's
// choice, so the kit's base URL is the selected file's.
func credentialFiles(args []string) []string {
	var out []string
	profileStores = nil
	homes := append(lastFirst(credkit.FlagValues(args, "--home")), "")
	configs := append(lastFirst(credkit.FlagValues(args, "--config")), "")
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		for _, c := range configs {
			for _, f := range config.ReadPaths(c) {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		if i == 0 {
			selectKitBaseURL(configs[0])
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			profileStores = append(profileStores, filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	return out
}

// selectKitBaseURL makes the kit's base URL the one Load will route to. The kit
// reads its base from the first file that states one and from BaseURL.Env, so
// it sees neither SUPEROPS_REGION's data center nor that an explicit --config
// with no base_url means the default, not a discovered file's host. Start reads
// BaseURL.Env after Files, so the choice goes there for Start alone: HoldStartup
// restores the variable, and config.go's reads of it still mean the operator.
func selectKitBaseURL(configFlag string) {
	const env = "SUPEROPS_BASE_URL"
	want := config.SelectedBaseURL(configFlag)
	was, set := os.LookupEnv(env)
	if want == "" || was == want || os.Setenv(env, want) != nil {
		return
	}
	restoreBaseURLEnv = func() {
		if set {
			_ = os.Setenv(env, was)
		} else {
			_ = os.Unsetenv(env)
		}
	}
}

var (
	restoreBaseURLEnv = func() {}
	// profileStores is the run profile store under each --home Files read.
	profileStores []string
)

// holdProfileStores holds the saved run profile store before any output, whether
// or not this run opens it: credkit.HoldBody reads every value under a
// credential name, every member of a duplicate key included. It is not one of
// Files, which the kit holds string by string as a credential file, and that
// would withhold every profile name and saved flag value. A URL or note in a
// description or a non-credential flag is free text, which contract section 1
// does not name.
func holdProfileStores() {
	stores := profileStores
	if home, err := os.UserHomeDir(); err == nil {
		stores = append(stores, filepath.Join(home, ".superops-cli", "profiles.json"))
	}
	for _, p := range stores {
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue // a pipe here would block every run
		}
		if opened, err := cliutil.ReadOpenedConfig(p); err == nil {
			credkit.HoldBody(opened.Data)
		}
	}
}

func lastFirst(values []string) []string {
	slices.Reverse(values)
	return values
}

// HoldStartup runs as each main's second statement, before any output: it hands
// SUPEROPS_BASE_URL back, then holds what the kit does not read itself (a saved
// profile store, cliutil.HoldProcessCredentials).
func HoldStartup() {
	restoreBaseURLEnv()
	holdProfileStores()
	cliutil.HoldProcessCredentials()
}
