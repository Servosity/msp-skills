// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mspbots-pp-cli/internal/cliutil"
	"mspbots-pp-cli/internal/config"
	"mspbots-pp-cli/internal/credkit"
)

// mspbotsAPI is the one real MSPbots Public API host.
const mspbotsAPI = "https://api.mspbots.ai"

// Declarations is what the connector holds and where each credential may go.
// The MSPbots Public API authenticates with one API key (MSPBOTS_API_KEY,
// config.toml's api_key or auth_header, or the 0600 credentials.toml store that
// auth set-token writes), sent as the "apikey" request header to the base URL.
//
// StdinFlags is empty: the one --stdin is feedback's free text, never a
// credential, and the kit holds a declared stdin whole, which would withhold
// the feedback itself. auth set-token holds the token it reads where it reads
// it (readSecretFromStdin).
//
// CredentialFields names "token": a --deliver webhook URL whose query carries
// token=... is held as a credential and refused to any origin but the base URL
// (kit-findings row 54).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"MSPBOTS_API_KEY",
		"MSPBOTS_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"MSPBOTS_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	CredentialHeaders: []string{"apikey"},
	CredentialFields:  []string{"token"},
	MCPArguments:      []string{"api_key", "apikey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "MSPBOTS_BASE_URL",
		Default:   mspbotsAPI,
	},
	Origins: []credkit.Origin{
		// The API key and every other credential go only to the API base URL
		// this process uses (showBaseURL).
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:MSPBOTS_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		// Run-time fields of config.Config (toml:"-" or unexported): never read from a file.
		{Source: "config:AuthSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:CredentialSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:AgentcookieManaged", Reason: "a run-time switch, never read from a file"},
		{Source: "config:CredentialRefusals", Reason: "run-time notes of refused credential files, never read from a file"},
		{Source: "config:untrustedCredentialRoute", Reason: "a run-time switch, never read from a file"},
		{Source: "config:configOwner", Reason: "a run-time note of which file filled the config"},
		{Source: "config:legacySourcePath", Reason: "the legacy path this config was read from"},
		{Source: "config:Path", Reason: "the path this config was read from, never read from a file"},
		{Source: "config:envOverrides", Reason: "names of the variables that overrode a value"},
		{Source: "config:fileConfig", Reason: "a run-time copy of the file's own values, never read from a file"},
		// cliutil.CredentialRefusal: where a refused credential file was.
		{Source: "config:Source", Reason: "a run-time note of where a credential came from, never read from a file"},
		{Source: "config:Err", Reason: "a load error held in memory, never read from a file"},
		// learn/entities.Config: the entity extractor's word lists.
		{Source: "config:variant", Reason: "a learn-entity spelling from the learn config"},
		{Source: "config:canonical", Reason: "a learn-entity name from the learn config"},
		{Source: "config:stopwords", Reason: "learn-search stop words"},
		{Source: "config:synonyms", Reason: "learn-search synonyms"},
		{Source: "env:MSPBOTS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:MSPBOTS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; held through URLEnv, and a credential sent to it is refused"},
		{Source: "env:MSPBOTS_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:MSPBOTS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:MSPBOTS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:MSPBOTS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:MSPBOTS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:MSPBOTS_DATA_DIR", Reason: "directory path"},
		{Source: "env:MSPBOTS_STATE_DIR", Reason: "directory path"},
		{Source: "env:MSPBOTS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:MSPBOTS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:MSPBOTS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:MSPBOTS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:MSPBOTS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:MSPBOTS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:MSPBOTS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:MSPBOTS_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:MSPBOTS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:MSPBOTS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:MSPBOTS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:MSPBOTS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:MSPBOTS_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:MSPBOTS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CODEX_THREAD_ID", Reason: "agent harness session label; only a bounded hash is journaled"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent harness session label; only a bounded hash is journaled"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "agent harness session label; only a bounded hash is journaled"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "agent harness session label; only a bounded hash is journaled"},
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

// credentialFiles runs inside Start, before anything can print. It shows the
// kit this process's base URL, holds the saved profiles, and names every file
// a credential may be read from under any --config and --home the command line
// can mean: holding a file the run does not load is safe, missing the one it
// loads is not.
func credentialFiles(args []string) []string {
	showBaseURL(args)
	holdProfileStoresUnder(args)
	return pathsUnder(args)
}

// holdProfileStoresUnder holds what the saved profiles under any --home the
// command line can mean carry, the legacy store included.
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
		holdProfileStore(filepath.Join(home, ".mspbots-cli", "profiles.json"))
	}
}

// holdProfileStore holds each value saved under a credential flag's name or
// under base-url, read token by token so every member of a duplicate key and
// everything before a syntax error counts. profiles.json is not in Files: the
// kit would hold every string in it, its own keys included, and withhold any
// output naming them.
func holdProfileStore(path string) {
	data, _, err := cliutil.ReadFileWithLegacyFallback(path, "")
	if err != nil {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	holdProfileValue(dec, "")
}

// holdProfileValue reads one JSON value saved under key and reports whether
// the file can still be read.
func holdProfileValue(dec *json.Decoder, key string) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch v := tok.(type) {
	case json.Delim:
		for dec.More() {
			name := key
			if v == '{' {
				k, err := dec.Token()
				if err != nil {
					return false
				}
				name, _ = k.(string)
			}
			if !holdProfileValue(dec, name) {
				return false
			}
		}
		_, err := dec.Token()
		return err == nil
	case string:
		if credentialProfileKey(key) {
			credkit.Hold("profile:value", v)
		}
	case json.Number:
		if credentialProfileKey(key) {
			credkit.Hold("profile:value", v.String())
		}
	}
	return true
}

// profileCredentialFlags is Declarations.CredentialFlags (none: the API key
// is never a flag): the checker's rule 1 lets only credkit.Start read that
// variable (TestProfileFlagsMatchTheDeclaration keeps the two the same).
var profileCredentialFlags []string

// credentialProfileKey reports a profile value's name as a credential flag's,
// spelled as a flag or with underscores, or base-url. The markers are the ones
// profile save and apply refuse (credentialLikeRunProfileFlag).
func credentialProfileKey(key string) bool {
	name := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(strings.TrimSpace(key), "-"), "_", "-"))
	if name == "base-url" || slices.Contains(profileCredentialFlags, "--"+name) {
		return true
	}
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// savedBaseURL is MSPBOTS_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process:
// the base config.Load resolves under this command line (an explicit --config
// or MSPBOTS_CONFIG file, else config.toml or the legacy file, then
// MSPBOTS_BASE_URL), never a base_url a file the program does not trust for
// routing names. The kit reads the variable right after Files returns;
// AfterStart puts it back, so nothing the program does later sees a change.
func showBaseURL(args []string) {
	base := configuredBase(args)
	if base == "" {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("MSPBOTS_BASE_URL")
	baseURLShown = os.Setenv("MSPBOTS_BASE_URL", base) == nil
}

// configuredBase is the base URL config.Load resolves under this command
// line. When Load refuses, nothing is shown and the kit falls back to the
// files it holds; when an untrusted config names another server (the program
// then sends no credential), the default is shown, so no credential can go to
// that server.
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
	if cfg.CredentialRouteBlocked() {
		return mspbotsAPI
	}
	return strings.TrimSpace(cfg.BaseURL)
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// MSPBOTS_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("MSPBOTS_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("MSPBOTS_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// pathsUnder is every file config.Load may read a credential from under each
// --home and --config the command line can mean.
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
