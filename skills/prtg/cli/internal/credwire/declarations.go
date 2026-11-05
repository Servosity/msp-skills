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

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/config"
	"prtg-pp-cli/internal/credkit"
	"prtg-pp-cli/internal/fleet"
)

// Declarations is what the connector holds and where each credential may go. A
// PRTG core authenticates with one API key, from PRTG_API_KEY, config.toml, the
// 0600 credentials.toml store, or a per-core entry in PRTG_FLEET_KEYS (held in
// credentialFiles, because the whole value names the cores beside their keys).
//
// ResponseFields is what the spec's responses carry unmasked. PRTG answers every
// credential setting in an include section (windowsloginpassword,
// linuxloginpassword, snmpauthpass, esxpassword, ...) with a "****" mask, and the
// kit would hold a declared field's mask: holding it would withhold every later
// listing. Those names are left to the shared walker
// (cliutil.HoldCredentialValues), which skips a mask, and every response has them
// replaced by name (cliutil.RedactBody).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"PRTG_API_KEY",
		"PRTG_MCP_HTTP_TOKEN",
		"PRTG_FLEET_KEYS",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"api_key",
		"client_secret",
		"access_token",
		"refresh_token",
	},
	StoredRecords: []string{
		"auth_header",
		"api_key",
		"client_secret",
		"access_token",
		"refresh_token",
	},
	CredentialFlags:   []string{"--key", "--password", "--token"},
	CredentialHeaders: nil,
	StdinFlags:        []string{"--stdin"},
	CredentialFields:  []string{"api_key", "apikey", "key", "passhash", "password", "token"},
	ResponseFields:    []string{"api_key", "key", "token"},
	MCPArguments:      []string{"api_key", "apikey", "key", "passhash", "password", "token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "PRTG_BASE_URL",
		Default:   "https://{server}/api/v2",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the core this process is configured for: a
		// core's host is the operator's, so no hosted domain can be named here.
		// credentialFiles resolves it as the program will (--core, PRTG_CORE,
		// PRTG_SERVER, config) and shows it to the kit. The connector's own origin
		// pin (api-origin-pinned) narrows every request to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:PRTG_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:server", Reason: "the core's host name, printed in every API URL"},
		{Source: "config:template_vars.server", Reason: "the core's host name, printed in every API URL"},
		{Source: "env:PRTG_FLEET_KEYS", Reason: "the whole list names each core beside its key; credentialFiles holds each key on its own (minted:fleet-key), and SecretEnv still keeps the variable from other programs"},
		{Source: "env:PRTG_SERVER", Reason: "the core's host name, printed in every API URL"},
		{Source: "env:PRTG_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:PRTG_CORE", Reason: "name of a registered core"},
		{Source: "env:PP_PRTG_FLEET_DIR", Reason: "directory path"},
		{Source: "env:PP_PRTG_FLEET_CHILD", Reason: "marker a fleet parent sets on its children"},
		{Source: "env:PRTG_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:PRTG_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:PRTG_SKIP_TLS_VERIFY", Reason: "boolean switch"},
		{Source: "env:PRTG_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:PRTG_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:PRTG_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:PRTG_DATA_DIR", Reason: "directory path"},
		{Source: "env:PRTG_STATE_DIR", Reason: "directory path"},
		{Source: "env:PRTG_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:PRTG_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:PRTG_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:PRTG_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:PRTG_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:PRTG_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:PRTG_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:PRTG_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:PRTG_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:PRTG_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:PRTG_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:PRTG_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:PRTG_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:PRTG_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:PRTG_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It holds each
// fleet key and the saved profiles, shows the kit this process's core, and names
// every file a credential may be read from under any --config and --home the
// command line can mean: holding a file the run does not load is safe, missing the
// one it loads is not. The fleet registry is not one of them: it holds host names,
// and the kit holds every value in a file it is given.
func credentialFiles(args []string) []string {
	fleet.HoldCredentials()
	selected, selectedOK := selectedCore(args)
	showBaseURL(args, selected, selectedOK)
	out := pathsUnder(args)
	holdProfileStoresUnder(args)
	reg, err := fleet.Load()
	if err != nil {
		return out
	}
	base, err := fleet.BaseDir()
	if err != nil {
		return out
	}
	for _, c := range reg.Cores {
		restore, err := cliutil.SetHomeOverride(c.Home(base))
		if err != nil {
			continue
		}
		if files, err := config.ReadPaths(""); err == nil {
			for _, f := range files {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		// A selection moves the home after Start, so its profiles are held here.
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	return out
}

// holdProfileStoresUnder holds what the saved profiles under any --home the
// command line can mean carry.
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
		holdProfileStore(filepath.Join(home, ".prtg-cli", "profiles.json"))
	}
}

// holdProfileStore holds each value saved under a credential flag's name or under
// base-url, read token by token so every member of a duplicate key and everything
// before a syntax error counts. profiles.json is not in Files: the kit would hold
// every string in it, its own keys included, and withhold any output naming them.
func holdProfileStore(path string) {
	data, _, err := cliutil.ReadFileWithLegacyFallback(path, "")
	if err != nil {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	holdProfileValue(dec, "")
}

// holdProfileValue reads one JSON value saved under key and reports whether the
// file can still be read.
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

// profileCredentialFlags is Declarations.CredentialFlags: the checker's rule 1
// lets only credkit.Start read that variable (TestProfileFlagsMatchTheDeclaration
// keeps the two the same).
var profileCredentialFlags = []string{"--key", "--password", "--token"}

// credentialProfileKey reports a profile value's name as a credential flag's,
// spelled as a flag or with underscores, or base-url. The markers are the ones
// profile save and apply refuse (credentialLikeRunProfileFlag).
func credentialProfileKey(key string) bool {
	name := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(strings.TrimSpace(key), "-"), "_", "-"))
	if name == "base-url" || slices.Contains(profileCredentialFlags, "--"+name) {
		return true
	}
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// selectedCore is the registered core --core or PRTG_CORE names, as
// applyCoreSelection reads them: the flag's last value, then the variable.
func selectedCore(args []string) (fleet.Core, bool) {
	name := ""
	if vals := credkit.FlagValues(args, "--core"); len(vals) > 0 {
		name = strings.TrimSpace(vals[len(vals)-1])
	}
	if name == "" {
		name = strings.TrimSpace(os.Getenv("PRTG_CORE"))
	}
	if name == "" {
		return fleet.Core{}, false
	}
	reg, err := fleet.Load()
	if err != nil {
		return fleet.Core{}, false
	}
	return reg.Get(name)
}

// savedBaseURL is PRTG_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// kit reads Declarations.BaseURL, whose default is a template it cannot resolve,
// and a core's host can be anything, so credentials could go nowhere otherwise.
// It reads the variable right after Files returns; AfterStart puts the variable
// back, so nothing the program does later sees a change. When nothing resolves,
// the variable is left alone and the kit allows no origin: a request carrying a
// credential is refused.
func showBaseURL(args []string, selected fleet.Core, selectedOK bool) {
	base := ""
	if selectedOK {
		base = selected.BaseURL()
	} else {
		base = configuredBase(args)
	}
	if base == "" {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("PRTG_BASE_URL")
	baseURLShown = os.Setenv("PRTG_BASE_URL", base) == nil
}

// configuredBase is the base URL config.Load resolves under this command line,
// with its {placeholders} filled in, or "" when one is left or Load refuses.
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
	base := cfg.BaseURL
	for k, v := range cfg.TemplateVars {
		base = strings.ReplaceAll(base, "{"+k+"}", v)
	}
	if strings.ContainsAny(base, "{}") {
		return ""
	}
	return base
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// PRTG_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("PRTG_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("PRTG_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
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
