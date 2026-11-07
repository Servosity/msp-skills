// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"cisco-meraki-pp-cli/internal/cliutil"
	"cisco-meraki-pp-cli/internal/config"
	"cisco-meraki-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Credentials: the Dashboard API key (MERAKI_DASHBOARD_API_KEY, config.toml
// dashboard_api_key, auth_header, [headers] and the 0600 credentials.toml
// store), sent as the X-Cisco-Meraki-API-Key header to the configured base
// URL only; the MCP HTTP bearer; and SPACEFAST_TOKEN for --deliver
// spacefast:. The key is static: nothing is minted or refreshed. No flag
// carries a credential (CredentialFlags). auth set-token reads its key from
// stdin with no flag and holds it where it reads it; a --stdin body is a
// request body or feedback text, never a credential source, so StdinFlags is
// empty. The run profile store is a source too, held in credentialFiles.
//
// ResponseFields are the Dashboard API's documented secret fields (spec.json
// property names: SSID PSKs, RADIUS, VPN and webhook shared secrets, SNMP
// communities and passwords, claim keys and tokens; the same names
// internal/store/meraki_secrets.go keeps out of the local mirror). A read
// that returns one is held as it streams, so an output carrying the value is
// withheld; --select without the field prints the rest.
var Declarations = credkit.Declarations{
	SecretEnv: []string{"MERAKI_DASHBOARD_API_KEY", "CISCO_MERAKI_MCP_HTTP_TOKEN", "SPACEFAST_TOKEN"},
	URLEnv:    []string{"CISCO_MERAKI_FEEDBACK_ENDPOINT", "SPACEFAST_API_URL"},
	Files:     credentialFiles,
	// The config and credential-store keys that carry the Dashboard API key.
	ConfigFields:      []string{"dashboard_api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:     []string{"dashboard_api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialHeaders: []string{"X-Cisco-Meraki-API-Key"},
	ResponseFields: []string{
		"apiKey", "claimKey", "communityString", "contentToken", "hashedToken", "identityPassphrase",
		"md5AuthenticationKey", "parsedToken", "passphrase", "password", "psk", "secret", "sharedSecret",
		"token", "v2CommunityString", "v3AuthPass", "v3PrivPass", "vppServiceToken",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CISCO_MERAKI_BASE_URL",
		Default:   "https://api.meraki.com/api/v1",
	},
	Origins: []credkit.Origin{
		// The Dashboard API key goes only to the configured base URL, never to
		// a --deliver webhook, the feedback endpoint or a redirect's host.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CISCO_MERAKI_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the Dashboard API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "env:CISCO_MERAKI_BASE_URL", Reason: "the Dashboard API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CISCO_MERAKI_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CISCO_MERAKI_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CISCO_MERAKI_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CISCO_MERAKI_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CISCO_MERAKI_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:CISCO_MERAKI_DATA_DIR", Reason: "directory path"},
		{Source: "env:CISCO_MERAKI_STATE_DIR", Reason: "directory path"},
		{Source: "env:CISCO_MERAKI_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:CISCO_MERAKI_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CISCO_MERAKI_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CISCO_MERAKI_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CISCO_MERAKI_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CISCO_MERAKI_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CISCO_MERAKI_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CISCO_MERAKI_MCP_HTTP_ALLOWED_ORIGINS", Reason: "browser origins the MCP HTTP server accepts, not a credential"},
		{Source: "env:CISCO_MERAKI_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CISCO_MERAKI_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and the outbound guard refuses any held value sent to it"},
		{Source: "env:CISCO_MERAKI_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CISCO_MERAKI_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CISCO_MERAKI_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CISCO_MERAKI_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:CISCO_MERAKI_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SPACEFAST_API_URL", Reason: "Spacefast API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:SPACEFAST_TEAM", Reason: "Spacefast team id, not a credential"},
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

// credentialFiles names every config and credentials file the command line
// and the environment could mean. The first is the config file the program
// reads when no --config is given (the kit takes the base URL from it; an
// explicit --config is read by the kit itself and wins), so a discovered
// file's base_url does not become the origin when the program reads another.
// Every other file is held: under the default home and each --home, the
// legacy path, and the credentials store beside each --config. Holding a file
// the program does not read is safe; missing the one it reads is not.
func credentialFiles(args []string) []string {
	var files []string
	add := func(p string, err error) {
		if err == nil && p != "" && !slices.Contains(files, p) {
			files = append(files, p)
		}
	}
	homes := credkit.FlagValues(args, "--home")
	startupNamedConfigOrHome = len(homes) > 0 || len(credkit.FlagValues(args, "--config")) > 0
	last := ""
	if n := len(homes); n > 0 {
		last = homes[n-1]
	}
	if env := os.Getenv("CISCO_MERAKI_CONFIG"); env != "" {
		add(env, nil)
	} else {
		underHome(last, func() { add(readConfigPath()) })
	}
	for _, h := range append([]string{""}, homes...) {
		underHome(h, func() {
			add(primaryConfigPath())
			add(config.LegacyConfigPath())
			add(cliutil.CredentialsFilePath())
			holdProfileStores()
		})
	}
	for _, c := range append(credkit.FlagValues(args, "--config"), os.Getenv("CISCO_MERAKI_CONFIG")) {
		if c != "" {
			add(cliutil.CredentialsFilePathForConfig(c))
		}
	}
	return files
}

// startupNamedConfigOrHome records that the command line named a --config or
// --home, which Start read. Set once, inside Start, before main goes on.
var startupNamedConfigOrHome bool

// StartupNamedConfigOrHome reports whether the command line named a --config
// or --home that Start read. The MCP server has neither flag, so it refuses to
// run when Start took a config, and with it an allowed origin, from one.
func StartupNamedConfigOrHome() bool { return startupNamedConfigOrHome }

// underHome runs fn with the program's --home override set to h ("" is the
// default home); an invalid --home is one the program refuses before reading
// any file.
func underHome(h string, fn func()) {
	restore, err := cliutil.SetHomeOverride(h)
	if err != nil {
		return
	}
	defer restore()
	fn()
}

func primaryConfigPath() (string, error) {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// readConfigPath is the default config file the program reads: the primary
// one, else the legacy path when only that exists (config.Load).
func readConfigPath() (string, error) {
	p, err := primaryConfigPath()
	if err != nil {
		return "", err
	}
	if _, serr := os.Stat(p); serr != nil {
		if legacy, lerr := config.LegacyConfigPath(); lerr == nil {
			if _, e := os.Stat(legacy); e == nil {
				return legacy, nil
			}
		}
	}
	return p, nil
}

// holdProfileStores holds the run profile stores (profiles.json, current and
// legacy): every value under a declared credential name, and every value under
// base-url as a URL (item 8c).
func holdProfileStores() {
	var stores []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		stores = append(stores, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		stores = append(stores, filepath.Join(home, ".cisco-meraki-cli", "profiles.json"))
	}
	for _, p := range stores {
		data, err := fs.ReadFile(os.DirFS(filepath.Dir(p)), filepath.Base(p))
		if err != nil {
			continue
		}
		credkit.HoldBody(data)
		var store struct {
			Profiles map[string]struct {
				Values map[string]string `json:"values"`
			} `json:"profiles"`
		}
		if json.Unmarshal(data, &store) != nil {
			continue
		}
		for _, prof := range store.Profiles {
			if v := prof.Values["base-url"]; v != "" {
				credkit.HoldURL("profile:base-url", v)
			}
		}
	}
}
