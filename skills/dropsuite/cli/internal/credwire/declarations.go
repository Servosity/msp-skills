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

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
	"dropsuite-pp-cli/internal/config"
	"dropsuite-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Dropsuite sends two credentials on every call: the access token
// (X-Access-Token: the reseller's admin token, or one organization's
// authentication_token) and the reseller token (X-Reseller-Token), from the
// environment, config.toml or the credential store. The store and config.toml are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile) and they are named in Files.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"DROPSUITE_ACCESS_TOKEN",
		"DROPSUITE_RESELLER_TOKEN",
		"DROPSUITE_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"DROPSUITE_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	// The credential keys of config.toml, the legacy config and the 0600
	// credentials.toml store (config.Config and cliutil.Credentials).
	ConfigFields: []string{
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "reseller_token",
	},
	StoredRecords: []string{
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "reseller_token",
	},
	CredentialHeaders: []string{"X-Access-Token", "X-Reseller-Token"},
	// The vendor names a credential travels under. GET /users answers with each
	// organization's authentication_token, which --org and DROPSUITE_ORG routing
	// then send as that organization's X-Access-Token; the others are what a
	// token endpoint or a user record can carry. Every name is string-valued:
	// the kit holds every scalar under a declared name. The lists are written
	// out in each field so credkit_verify can read them.
	CredentialFields: []string{
		"authentication_token", "authenticationToken", "access_token", "refresh_token", "id_token",
		"reseller_token", "client_secret", "api_key", "password", "secret", "token",
	},
	ResponseFields: []string{
		"authentication_token", "authenticationToken", "access_token", "refresh_token", "id_token",
		"reseller_token", "client_secret", "api_key", "password", "secret", "token",
	},
	MCPArguments: []string{
		"authentication_token", "authenticationToken", "access_token", "refresh_token", "id_token",
		"reseller_token", "client_secret", "api_key", "password", "secret", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DROPSUITE_BASE_URL",
		// A template the kit cannot resolve, so it allows no origin unless
		// showBaseURL resolves the base the run really uses.
		Default: "https://{host}/api",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API base the program will really use.
		// The connector's own origin pin (api-origin-pinned) narrows every request,
		// redirect hops included, to that same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// Each organization's token from GET /users goes back to the same API only.
		{Source: "response:authentication_token", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of an MCP server; it is sent nowhere.
		{Source: "env:DROPSUITE_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "the API host name that fills {host} in the base URL"},
		{Source: "flag:--credential-ref", Reason: "a reference naming where a client profile's credential lives, not the credential"},
		{Source: "flag:--additional-credential-ref", Reason: "references naming where a client profile's credentials live, not the credentials"},
		{Source: "env:DROPSUITE_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DROPSUITE_HOST", Reason: "the API host name that fills {host} in the base URL"},
		{Source: "env:DROPSUITE_ORG", Reason: "an organization id, owner email or external_id that selects whose token is used, not the token"},
		{Source: "env:DROPSUITE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:DROPSUITE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DROPSUITE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DROPSUITE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DROPSUITE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:DROPSUITE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:DROPSUITE_DATA_DIR", Reason: "directory path"},
		{Source: "env:DROPSUITE_STATE_DIR", Reason: "directory path"},
		{Source: "env:DROPSUITE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:DROPSUITE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DROPSUITE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DROPSUITE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DROPSUITE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DROPSUITE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:DROPSUITE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DROPSUITE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DROPSUITE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DROPSUITE_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:DROPSUITE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:DROPSUITE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DROPSUITE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DROPSUITE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It shows the kit
// the base URL this run will use and names every file a credential may be read
// from under any --config, --home, DROPSUITE_CONFIG and DROPSUITE_HOME the command
// line can mean: config.toml under each config directory, the legacy config, each
// explicit config with the 0600 credentials.toml beside it, and the global store
// Load falls back to. Holding a file the run does not load is safe; missing the one
// it loads is not.
//
// No profiles.json value is a source here: the connector has no credential flag
// and no base-url flag, and profile save refuses every credential-like flag.
func credentialFiles(args []string) []string {
	homes, configs := credkit.FlagValues(args, "--home"), credkit.FlagValues(args, "--config")
	home, configPath := last(homes), last(configs)
	showBaseURL(home, configPath)
	if env := os.Getenv("DROPSUITE_CONFIG"); env != "" {
		configs = append(configs, env)
	}
	var out []string
	add := func(path string, err error) {
		if err == nil && path != "" && !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	for _, h := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(h)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			add(filepath.Join(dir, "config.toml"), nil)
		}
		add(cliutil.CredentialsFilePath())
		for _, c := range configs {
			add(c, nil)
			add(cliutil.CredentialsFilePathForConfig(c))
		}
		restore()
	}
	add(config.LegacyConfigPath())
	return out
}

// last is the value the flag parser takes for a repeated flag: the last one.
// The flag name stays a constant at each FlagValues call, so the MCP file-flag
// checker ties what it returns to that flag.
func last(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// savedBaseURL is DROPSUITE_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// default base is the template https://{host}/api, which the kit cannot resolve,
// so it resolves the base the way the client does: config.Load under this run's
// --home and --config, then client.ResolvedBaseURL over the loaded template
// variables (DROPSUITE_HOST, template_vars, then the default host). The kit reads
// DROPSUITE_BASE_URL right after Files returns; AfterStart puts the variable back.
// When Load refuses or a placeholder is left, the variable is left alone and the
// template allows no origin: a request carrying a credential is refused.
func showBaseURL(home, configPath string) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil {
		return
	}
	base := client.ResolvedBaseURL(strings.TrimRight(cfg.BaseURL, "/"), cfg.TemplateVars)
	if base == "" || strings.ContainsAny(base, "{}") {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("DROPSUITE_BASE_URL")
	baseURLShown = os.Setenv("DROPSUITE_BASE_URL", base) == nil
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// DROPSUITE_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("DROPSUITE_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("DROPSUITE_BASE_URL")
	}
}
