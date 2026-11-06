// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"domotz-pp-cli/internal/client"
	"domotz-pp-cli/internal/cliutil"
	"domotz-pp-cli/internal/config"
	"domotz-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// The Domotz Public API authenticates with one API key sent in the X-Api-Key
// header, from DOMOTZ_API_KEY, config.toml (api_key) or the 0600
// credentials.toml store that auth set-token writes; there is no token
// endpoint. The store and config.toml are credential SOURCES: their own writes
// go outside the kit (cliutil.AtomicWriteCredentialFile, orchestrator ruling 7)
// and they are named in Files, because the kit would withhold them by contract.
//
// The API also returns and accepts the credentials of the devices it monitors:
// the SNMP v1/v2 read and write communities (snmp_read_community and
// snmp_write_community on a device's details and SNMP authentication, and the
// bare read and write of DeviceSnmpCommunity), the SNMP v3 user,
// authentication_key and encryption_key, a device's or custom driver's
// credentials object with its username and password, and a team leader's
// password. These are exactly the spec names the one secret-name rule reads
// as credentials (cliutil/testdata/domotz-spec-names.json, pinned by
// TestDeclarationsMatchTheSpec), each a request field, a response field and an
// MCP argument. The MCP mirror names a tool argument after its command's flag,
// so the flag spellings (leader-password, credentials-password,
// authentication-key, ...) are MCP arguments and credential flags too.
//
// The base URL is a template, https://api-{region}.domotz.com/public-api/v1;
// the kit's allowed origin is the one host this run resolves it to
// (selectKitBaseURL), never every regional host.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"DOMOTZ_API_KEY",
		"DOMOTZ_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"DOMOTZ_FEEDBACK_ENDPOINT",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "api_key", "access_token", "refresh_token", "client_id", "client_secret", "headers"},
	StoredRecords: []string{"auth_header", "api_key", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialFlags: []string{
		"--read", "--write", "--snmp-read-community", "--snmp-write-community", "--username", "--password",
		"--authentication-key", "--encryption-key", "--credentials-username", "--credentials-password", "--leader-password",
	},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"X-Api-Key"},
	CredentialFields:  []string{"authentication_key", "credentials", "encryption_key", "password", "read", "snmp_read_community", "snmp_write_community", "username", "write"},
	ResponseFields:    []string{"authentication_key", "credentials", "encryption_key", "password", "read", "snmp_read_community", "snmp_write_community", "username", "write"},
	MCPArguments: []string{
		"authentication_key", "credentials", "encryption_key", "password", "read", "snmp_read_community", "snmp_write_community", "username", "write",
		"authentication-key", "credentials-password", "credentials-username", "encryption-key", "leader-password", "snmp-read-community", "snmp-write-community",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DOMOTZ_BASE_URL",
		// The program's own default, a template: alone it parses to no origin,
		// so a run whose {region} does not resolve to one DNS label sends a held
		// value nowhere (the origin pin, api-origin-pinned, refuses it too).
		Default: "https://api-{region}.domotz.com/public-api/v1",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API host this process is configured
		// for. The connector's origin pin (internal/cli/domotz_guard.go) narrows
		// every request and redirect to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:DOMOTZ_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's TLS private key is sent nowhere.
		{Source: "minted:tls-key", Allowed: nil},
		// A saved run profile's base-url is never used to send a request (no
		// command has a --base-url flag), so what it carries is sent nowhere.
		{Source: "profile:base-url", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "the base URL's template values ({region}), each a DNS label"},
		{Source: "config:region", Reason: "the Domotz region label in the API host name (template_vars.region)"},
		{Source: "env:DOMOTZ_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DOMOTZ_REGION", Reason: "the Domotz region label in the API host name"},
		{Source: "env:DOMOTZ_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DOMOTZ_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DOMOTZ_CACHE_DIR", Reason: "directory path"},
		{Source: "env:DOMOTZ_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DOMOTZ_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:DOMOTZ_DATA_DIR", Reason: "directory path"},
		{Source: "env:DOMOTZ_STATE_DIR", Reason: "directory path"},
		{Source: "env:DOMOTZ_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:DOMOTZ_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DOMOTZ_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DOMOTZ_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:DOMOTZ_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:DOMOTZ_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:DOMOTZ_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DOMOTZ_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DOMOTZ_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:DOMOTZ_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DOMOTZ_USER_AGENT", Reason: "User-Agent string"},
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
// DOMOTZ_CONFIG wins, as it does in config.Load, and the last --home decides the
// default paths, as pflag does. The credential stores that config reads follow
// it. Every other file any --config or --home could mean is only held: its
// values cannot print, and its base_url cannot become an allowed origin. No
// flag arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	cliutil.CompactStdinBody(args)
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	cliutil.StartupConfigFlag, cliutil.StartupHomeFlag = explicit, ""
	if len(homes) > 0 {
		cliutil.StartupHomeFlag = homes[len(homes)-1]
	}
	flagConfig := explicit
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("DOMOTZ_CONFIG")
	}
	var used []string
	if len(homes) == 0 {
		used = runFiles(explicit)
		selectKitBaseURL(flagConfig)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		defaults := runFiles("")
		if i == len(homes)-1 {
			selectKitBaseURL(flagConfig)
		}
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

func holdOnly(files []string) {
	for _, f := range files {
		cliutil.HoldConfigFilePath(f)
	}
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, the legacy config when no explicit one
// was named and the default is absent, then the credential stores it reads
// (the explicit config's own store, then the global one). The run profile
// stores are held here and not returned.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); err != nil {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	if explicit != "" {
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		files = append(files, store)
	}
	// The run profile store is nested JSON, which the kit's file reader answers
	// by holding every string in it, so a profile named "p" would refuse every
	// later start. Its credential flags and fields are held by name, and the
	// password and query values of a base-url value through the kit's URL reader.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".domotz-cli", "profiles.json"))
	}
	return files
}

// regionLabel is one DNS label: the only form a Domotz region
// (us-east-1-cell-1, eu-west-1-cell-1) takes in the API host. The origin pin
// (internal/cli/domotz_guard.go pinnedOrigin) applies the same rule.
var regionLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

var basePlaceholder = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// restoreBaseURLEnv undoes selectKitBaseURL; RestoreStartupEnv runs it.
var restoreBaseURLEnv = func() {}

// selectKitBaseURL makes the kit's base_url the one API host this run sends to:
// the base URL config.Load resolves (an explicit --config, DOMOTZ_CONFIG, the
// discovered default, DOMOTZ_BASE_URL over all of them), with its {region}
// filled from DOMOTZ_REGION or the config's template_vars, as the origin pin
// fills it. The kit reads an explicit --config and BaseURL.Env, so it sees
// neither {region} filled nor what --home selects; Start reads BaseURL.Env
// after Files, so the choice goes there for Start alone, and RestoreStartupEnv
// puts the variable back before the program reads it. A {region} that is not
// one DNS label selects nothing: the template stays, which parses to no origin.
// Nothing is held while config.Load runs here: this runs inside Start's
// public-text check, and the real Load holds every value after it.
func selectKitBaseURL(configFlag string) {
	const env = "DOMOTZ_BASE_URL"
	hold, holdBody, holdURL := cliutil.Hold, cliutil.HoldBody, cliutil.HoldURL
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.HoldBody = func([]byte) {}
	cliutil.HoldURL = func(credkit.Source, string) {}
	cfg, err := config.Load(configFlag)
	cliutil.Hold, cliutil.HoldBody, cliutil.HoldURL = hold, holdBody, holdURL
	if err != nil || cfg == nil {
		return
	}
	for _, m := range basePlaceholder.FindAllStringSubmatch(cfg.BaseURL, -1) {
		if !regionLabel.MatchString(cfg.TemplateVars[m[1]]) {
			return
		}
	}
	want := strings.TrimRight(client.ResolvedBaseURL(cfg.BaseURL, cfg.TemplateVars), "/")
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

// RestoreStartupEnv runs right after credkit.Start in each main: it puts back
// the DOMOTZ_BASE_URL that selectKitBaseURL set for Start alone.
func RestoreStartupEnv() {
	restoreBaseURLEnv()
	restoreBaseURLEnv = func() {}
}
