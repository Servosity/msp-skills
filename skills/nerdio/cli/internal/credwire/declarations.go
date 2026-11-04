// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/config"
	"nerdio-pp-cli/internal/credkit"
)

// credentialFlags are the flags whose value is a credential. A run profile
// keeps flag values by name, so the Files hook holds these from profiles.json.
var credentialFlags = []string{"--client-id", "--client-secret", "--azure-access-token", "--graph-access-token"}

// Declarations is what the connector holds and where each credential may go.
// Nerdio Manager for MSP takes a bearer token minted by the OAuth2 client
// credentials grant at NERDIO_TOKEN_URL (Microsoft Entra) from the client ID and
// secret, from the environment, config.toml or the 0600 credentials.toml store.
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile) and they are named in Files,
// because the kit would withhold them by contract. provisioning link-tenant
// sends an Azure and a Graph access token in its body.
//
// The client pair and a refresh token may go to base_url and Entra's sign-in
// hosts, public and sovereign, and nowhere else. The connector's pin
// (handfixes.json api-origin-pinned) narrows that further: the pair leaves only
// in the POST to NERDIO_TOKEN_URL's path at its origin. The kit's origins are
// data and cannot name the token URL; base_url is in the list because the
// verify and class harnesses serve the token endpoint on the API's origin, and
// the client ID because the minted token, sent to base_url, names it.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"NERDIO_CLIENT_ID",
		"NERDIO_CLIENT_SECRET",
		"NERDIO_MCP_HTTP_TOKEN",
	},
	URLEnv:           []string{"NERDIO_TOKEN_URL", "NERDIO_FEEDBACK_ENDPOINT"},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:    []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialFlags:  []string{"--client-id", "--client-secret", "--azure-access-token", "--graph-access-token"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"client_id", "client_secret", "refresh_token", "azureAccessToken", "graphAccessToken"},
	// Token fields only (an AVD session's sessionToken included): a LAPS
	// password, a BitLocker key and a secure
	// variable's value are tenant secrets the caller asked to see
	// (handfixes.json tenant-secrets-shown-never-stored), so they are shown on
	// stdout and in MCP results and kept out of files, the store and argv by
	// the connector, not withheld everywhere by the kit.
	ResponseFields: []string{"access_token", "refresh_token", "id_token", "accessToken", "refreshToken", "idToken", "sessionToken"},
	MCPArguments:   []string{"azure-access-token", "graph-access-token", "azureAccessToken", "graphAccessToken"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "NERDIO_BASE_URL",
		Default:   "https://nmm.example.com",
	},
	Origins: []credkit.Origin{
		// Every other credential goes only to the API this run is configured for
		// (selectKitBaseURL).
		{Source: "*", Allowed: []string{"base_url"}},
		{Source: "env:NERDIO_CLIENT_ID", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "env:NERDIO_CLIENT_SECRET", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "config:client_id", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "config:client_secret", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "flag:--client-id", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "flag:--client-secret", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "field:client_id", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "field:client_secret", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "basic-user", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "minted:client-pair", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "minted:client-profile", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "config:refresh_token", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "field:refresh_token", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "response:refresh_token", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		{Source: "minted:refresh_token", Allowed: []string{"base_url", "https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}},
		// The MCP HTTP bearer authenticates callers of this server, and the TLS
		// key is this server's own; neither is sent anywhere.
		{Source: "env:NERDIO_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
		// A proxy's password goes to that proxy only: the kit's transport adds its origin.
		{Source: "proxy-userinfo", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held as userinfo"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "env:NERDIO_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:NERDIO_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:NERDIO_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and the kit's transport refuses any held value sent to it"},
		{Source: "env:NERDIO_OAUTH_SCOPE", Reason: "an OAuth scope name"},
		{Source: "env:NERDIO_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:NERDIO_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:NERDIO_CACHE_DIR", Reason: "directory path"},
		{Source: "env:NERDIO_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:NERDIO_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:NERDIO_DATA_DIR", Reason: "directory path"},
		{Source: "env:NERDIO_STATE_DIR", Reason: "directory path"},
		{Source: "env:NERDIO_HOME", Reason: "root directory for config, data, state and cache; Files resolves it"},
		{Source: "env:NERDIO_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:NERDIO_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:NERDIO_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:NERDIO_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:NERDIO_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:NERDIO_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:NERDIO_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:NERDIO_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:NERDIO_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:NERDIO_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:NERDIO_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:NERDIO_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:NERDIO_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:NERDIO_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:NERDIO_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:USERNAME", Reason: "the local account name, recorded in no request"},
		{Source: "env:GODEBUG", Reason: "Go runtime settings"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns
// every config and credential store file any --config or --home on the command
// line (and none) could mean, which the kit reads and holds, and holds the run
// profile stores and a --tls-key itself. Then selectKitBaseURL makes the kit's
// base_url the one this run will use. No flag arity is guessed:
// credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	cliutil.CompactStdinBody(args)
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
	configs := credkit.FlagValues(args, "--config")
	homes := append([]string{""}, credkit.FlagValues(args, "--home")...)
	var files []string
	for _, home := range homes {
		underHome(home, func() { files = append(files, runFiles(configs)...) })
	}
	underHome(homes[len(homes)-1], func() { selectKitBaseURL(lastOf(configs)) })
	return files
}

// runFiles is every file a run under the current home reads a credential from:
// each --config, NERDIO_CONFIG, the default config and its legacy path, and the
// credential store beside each and the global one.
func runFiles(configs []string) []string {
	explicit := append(append([]string(nil), configs...), os.Getenv("NERDIO_CONFIG"))
	var files []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		files = append(files, filepath.Join(dir, "config.toml"))
		cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"), credentialFlags)
	}
	if legacy, err := config.LegacyConfigPath(); err == nil {
		files = append(files, legacy)
	}
	for _, c := range explicit {
		if c == "" {
			continue
		}
		files = append(files, c)
		if store, err := cliutil.CredentialsFilePathForConfig(c); err == nil {
			files = append(files, store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		files = append(files, store)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".nerdio-cli", "profiles.json"), credentialFlags)
	}
	return files
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// underHome runs f with home as --home would set it ("" is no --home). The run
// refuses a --home that does not apply before it loads anything, so f is
// skipped.
func underHome(home string, f func()) {
	if home == "" {
		f()
		return
	}
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	f()
}

// restoreBaseURLEnv undoes selectKitBaseURL; AfterStart runs it.
var restoreBaseURLEnv = func() {}

// selectKitBaseURL makes the kit's base_url the one config.Load will route to.
// The kit takes base_url from the first file naming one, an explicit --config,
// then BaseURL.Env; it cannot see which file Load prefers, nor what --home
// selects, and the credential store names a base_url too. Start reads
// BaseURL.Env after Files, so the choice goes there for Start alone.
func selectKitBaseURL(configFlag string) {
	const env = "NERDIO_BASE_URL"
	want := config.SelectedBaseURL(configFlag)
	was, set := os.LookupEnv(env)
	if want == "" || (set && was == want) || os.Setenv(env, want) != nil {
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

// AfterStart restores what the Files hook changed for Start alone. Each main
// calls it right after credkit.Start.
func AfterStart() { restoreBaseURLEnv() }
