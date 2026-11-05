// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"liongard-pp-cli/internal/client"
	"liongard-pp-cli/internal/cliutil"
	"liongard-pp-cli/internal/config"
	"liongard-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Liongard authenticates with X-ROAR-API-KEY: LIONGARD_API_KEY, or
// base64(id:secret) composed from LIONGARD_ACCESS_KEY_ID and
// LIONGARD_ACCESS_KEY_SECRET (config.Load holds the composed forms where it
// derives them). The optional session token LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH
// goes in X-Auth-Token. Stored credentials live in config.toml and the 0600
// credentials.toml store, both credential sources (credentialFiles).
//
// The body, response and argument names are the credential members of the
// Liongard spec: log-in Password and verify-token Token (requests), a new access
// key's AccessKeySecret, the log-in token, a launchpoint's SecureConfig (an
// object whose member names vary per inspector: the kit holds a declared
// container's whole subtree) and a launchpoint log's Secrets; accessToken is
// the generic name a session token answer uses (held as well). secure-config,
// password and token are the CLI flag names a mirrored MCP tool and a saved
// profile carry. NumericPassword and tokens are the factory review's recorded
// inputs (.review/factory/codex-r2.md, kit-findings rows for liongard), kept so
// those verdicts stay true.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"LIONGARD_API_KEY",
		// The access key ID is held as well: the old layer held it, and holding it
		// only withholds a listing that repeats the operator's own key ID.
		"LIONGARD_ACCESS_KEY_ID",
		"LIONGARD_ACCESS_KEY_SECRET",
		"LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH",
		"LIONGARD_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "api_key", "endpoints_session_token_auth",
		"access_token", "refresh_token", "client_id", "client_secret",
	},
	StoredRecords: []string{
		"auth_header", "api_key", "endpoints_session_token_auth",
		"access_token", "refresh_token", "client_id", "client_secret",
	},
	CredentialFlags:   []string{"--password", "--token", "--secure-config"},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"X-ROAR-API-KEY", "X-Auth-Token"},
	CredentialFields:  []string{"Password", "Token", "SecureConfig", "AccessKeySecret", "secure-config", "NumericPassword"},
	ResponseFields:    []string{"AccessKeySecret", "Token", "SecureConfig", "Secrets", "Password", "accessToken", "tokens"},
	MCPArguments:      []string{"Password", "Token", "SecureConfig", "AccessKeySecret", "secure-config", "NumericPassword"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "LIONGARD_BASE_URL",
		Default:   shippedBaseURL,
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the one Liongard instance this process
		// resolves (showBaseURL). The connector's origin pin (api-origin-pinned)
		// narrows every request and redirect, credentialed or not, to it too.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:LIONGARD_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "template variables for the base URL ({instance}, the tenant's subdomain), printed in every API URL"},
		{Source: "config:AuthSource", Reason: "never persisted (toml:\"-\"): the name of the source the credential came from, not its value"},
		{Source: "config:CredentialSource", Reason: "never persisted (toml:\"-\"): the name of the source the credential came from, not its value"},
		{Source: "config:configOwner", Reason: "unexported, never persisted: the path of the file this config was read from"},
		{Source: "config:legacySourcePath", Reason: "unexported, never persisted: the legacy config path Load fell back to"},
		{Source: "config:Path", Reason: "never persisted (toml:\"-\"): the config file's path"},
		{Source: "config:envOverrides", Reason: "unexported, never persisted: which fields an environment variable set (booleans by field name)"},
		{Source: "config:Source", Reason: "cliutil.CredentialRefusal: the name of a credential source that was refused, not its value"},
		{Source: "config:Err", Reason: "cliutil.CredentialRefusal: why a credential source was refused"},
		{Source: "config:variant", Reason: "learn entity extractor: a synonym's variant words, program text"},
		{Source: "config:canonical", Reason: "learn entity extractor: a synonym's canonical words, program text"},
		{Source: "config:stopwords", Reason: "learn entity extractor: registered stopwords, program text"},
		{Source: "config:synonyms", Reason: "learn entity extractor: registered synonyms, program text"},
		{Source: "env:LIONGARD_INSTANCE", Reason: "the tenant's subdomain, printed in every API URL"},
		{Source: "env:LIONGARD_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:LIONGARD_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:LIONGARD_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:LIONGARD_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:LIONGARD_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:LIONGARD_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:LIONGARD_DATA_DIR", Reason: "directory path"},
		{Source: "env:LIONGARD_STATE_DIR", Reason: "directory path"},
		{Source: "env:LIONGARD_CACHE_DIR", Reason: "directory path"},
		{Source: "env:LIONGARD_DB", Reason: "path of the local database"},
		{Source: "env:LIONGARD_DATABASE", Reason: "path of the local database"},
		{Source: "env:LIONGARD_DATA_FILE", Reason: "path of a local data file"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:LIONGARD_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:LIONGARD_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:LIONGARD_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:LIONGARD_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:LIONGARD_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:LIONGARD_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:LIONGARD_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:LIONGARD_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:LIONGARD_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the kit's transport"},
		{Source: "env:LIONGARD_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:LIONGARD_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:LIONGARD_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:LIONGARD_LEARN_SESSION", Reason: "session grouping label, stored only as a hash"},
		{Source: "env:LIONGARD_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:PP_FILELOCK_HOLDER", Reason: "test-helper marker for the file-lock probe"},
		{Source: "env:PP_FILELOCK_PATH", Reason: "test-helper path for the file-lock probe"},
		{Source: "env:PP_FILELOCK_READY", Reason: "test-helper path for the file-lock probe"},
		{Source: "env:PP_STORE_HARDEN_DB", Reason: "test-helper database path"},
		{Source: "env:PP_STORE_HARDEN_ID", Reason: "test-helper writer id"},
		{Source: "env:PP_STORE_HARDEN_LOCK_PROBE", Reason: "test-helper switch"},
		{Source: "env:PP_STORE_HARDEN_ROUNDS", Reason: "test-helper count"},
		{Source: "env:PP_STORE_HARDEN_WRITER", Reason: "test-helper switch"},
		{Source: "env:PP_STORE_RO_DB", Reason: "test-helper database path"},
		{Source: "env:PP_STORE_RO_HELPER", Reason: "test-helper switch"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "agent session label, stored only as a hash"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "agent session label, stored only as a hash"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent session label, stored only as a hash"},
		{Source: "env:CODEX_THREAD_ID", Reason: "agent session label, stored only as a hash"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "agent session label, stored only as a hash"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "agent session label, stored only as a hash"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
		{Source: "env:GODEBUG", Reason: "Go runtime settings"},
		{Source: "env:PATH", Reason: "executable search path"},
		{Source: "env:SYSTEMROOT", Reason: "Windows system directory"},
		{Source: "env:USERNAME", Reason: "the local account name"},
	},
}

// credentialFiles runs inside Start, before anything can print. It holds the
// saved profiles, shows the kit this process's base URL, and names every file a
// credential may be read from under any --config and --home the command line
// can mean: holding a file the run does not load is safe, missing the one it
// loads is not.
func credentialFiles(args []string) []string {
	showBaseURL(args)
	holdProfileStoresUnder(args)
	return pathsUnder(args)
}

// shippedBaseURL is the spec's templated base URL, which does not parse as a URL.
const shippedBaseURL = "https://{instance}.app.liongard.com/api/v1"

// savedBaseURL is LIONGARD_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process.
// The kit reads Declarations.BaseURL, whose default is a template it cannot
// resolve ({instance}), so it is shown the URL the client really resolves:
// config.Load under this command line, then client.ResolvedBaseURL with its
// template variables, the same call the origin pin makes. It sets
// LIONGARD_BASE_URL, which the kit reads right after Files returns; AfterStart
// puts the variable back. When nothing resolves, the variable is set to the
// shipped template, which does not parse as a URL, so no file's base_url
// becomes an origin and every credentialed request is refused.
func showBaseURL(args []string) {
	base := configuredBase(args)
	if base == "" {
		base = shippedBaseURL
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("LIONGARD_BASE_URL")
	baseURLShown = os.Setenv("LIONGARD_BASE_URL", base) == nil
}

// configuredBase is the base URL the client resolves under this command line,
// or "" when a {placeholder} is left, the instance is still config.Load's
// placeholder default ("example", no real tenant), or config.Load refuses.
func configuredBase(args []string) string {
	home, configPath := lastOf(credkit.FlagValues(args, "--home")), lastOf(credkit.FlagValues(args, "--config"))
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return ""
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil {
		return ""
	}
	base := strings.TrimRight(client.ResolvedBaseURL(strings.TrimRight(cfg.BaseURL, "/"), cfg.TemplateVars), "/")
	if strings.ContainsAny(base, "{}") || strings.EqualFold(base, placeholderBaseURL()) {
		return ""
	}
	return base
}

// placeholderBaseURL is the shipped template with config.Load's default
// instance, the same placeholder doctor refuses to dial.
func placeholderBaseURL() string {
	return client.ResolvedBaseURL(strings.TrimRight(shippedBaseURL, "/"), map[string]string{"instance": "example"})
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// LIONGARD_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("LIONGARD_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("LIONGARD_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// holdProfileStoresUnder holds what the saved profiles under any --home the
// command line can mean carry: a value under a credential flag's name, every
// duplicate included (credkit.HoldBody reads the declared names).
func holdProfileStoresUnder(args []string) {
	for _, home := range append(credkit.FlagValues(args, "--home"), "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".liongard-cli", "profiles.json"))
	}
}

func holdProfileStore(path string) {
	if data, err := os.ReadFile(filepath.Clean(path)); err == nil { // the profile store path under a --home
		credkit.HoldBody(data)
		holdProfileURLs(data)
	}
}

// holdProfileURLs holds the password and every query value of each saved
// base-url, in every profile: HoldBody reads credential names, and a URL is not
// one. A store that does not parse was already held word by word by HoldBody.
func holdProfileURLs(data []byte) {
	var doc any
	if json.Unmarshal(data, &doc) != nil {
		return
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if s, ok := val.(string); ok && (k == "base-url" || k == "base_url") {
					credkit.HoldURL("profile:base-url", s)
				}
				walk(val)
			}
		case []any:
			for _, val := range x {
				walk(val)
			}
		}
	}
	walk(doc)
}

func pathsUnder(args []string) []string {
	homes := append(credkit.FlagValues(args, "--home"), "")
	configs := append(credkit.FlagValues(args, "--config"), "")
	var out []string
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		for _, c := range configs {
			files, err := config.ReadPaths(c)
			if err != nil {
				continue
			}
			for _, f := range files {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		restore()
	}
	return out
}
