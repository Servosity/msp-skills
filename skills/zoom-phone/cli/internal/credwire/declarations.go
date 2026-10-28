// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
	"zoom-phone-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// The account ID is an identifier: `fleet` and `accounts` report it, one Zoom
// response carries it, and nothing authenticates with it alone. The client ID is
// held: it is the public half of the pair and no command prints it whole.
//
// The field names are the closed set the Zoom Phone spec uses for a credential
// (SIP password, desk-phone and port-order PIN, smartphone activation code, Team
// Chat channel token) and the OAuth token fields. The response redaction
// (zoom_secret_transport.go) still removes them from every JSON body, so a record
// stays readable; the kit sits beneath it and holds each value first.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"ZOOM_PHONE_CLIENT_ID",
		"ZOOM_PHONE_CLIENT_SECRET",
		"ZOOM_PHONE_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"ZOOM_PHONE_TOKEN_URL",
		"ZOOM_PHONE_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "phone_client_id", "phone_client_secret",
	},
	StoredRecords: []string{
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "phone_client_id", "phone_client_secret",
	},
	CredentialFlags: []string{
		"--client-id", "--client-secret", "--pin-code", "--desk-phone-pin-code", "--user-settings-desk-phone-pin-code",
	},
	StdinFlags: []string{"--client-secret-stdin"},
	CredentialFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret",
		"password", "pin_code", "desk_phone_pin_code", "pin", "activation_code", "token",
	},
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret",
		"password", "pin_code", "pin", "activation_code", "token",
	},
	MCPArguments: []string{
		"access_token", "refresh_token", "id_token", "client_secret",
		"password", "pin_code", "desk_phone_pin_code", "user_settings_desk_phone_pin_code", "pin", "activation_code", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "ZOOM_PHONE_BASE_URL",
		Default:   "https://api.zoom.us/v2",
	},
	Origins: []credkit.Origin{
		// A token, a PIN or any other held value goes only to the API base the
		// program will really use.
		{Source: "*", Allowed: []string{"base_url"}},
		// The token request: an app's client ID and secret and a refresh token also
		// go to Zoom's token endpoint, whose Basic header is held as sent.
		{Source: "header:authorization", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "basic-user", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "env:ZOOM_PHONE_CLIENT_ID", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "env:ZOOM_PHONE_CLIENT_SECRET", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "config:client_id", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "config:client_secret", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "config:phone_client_id", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "config:phone_client_secret", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "config:refresh_token", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "flag:--client-id", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "flag:--client-secret", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "response:refresh_token", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "field:refresh_token", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		{Source: "field:client_secret", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		// A secret piped to `accounts connect` or `auth set-token`.
		{Source: "stdin", Allowed: []string{"base_url", "https://zoom.us", "https://zoomgov.com"}},
		// The MCP HTTP bearers authenticate callers of this server; they are sent nowhere.
		{Source: "env:ZOOM_PHONE_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "cookie:_zm_chtaid", Reason: "Zoom tracking counter, a number from 0 to 999 set on every API and token response; it authenticates nothing and this connector sends no cookies (its client has no cookie jar)"},
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the OAuth token URL; a credential inside it is held through URLEnv and HoldConfigFile"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:phone_account_id", Reason: "the Zoom account ID, reported by fleet and accounts; nothing authenticates with it alone"},
		{Source: "config:zoom_mint_record", Reason: "a sha256 binding a saved token to its account and app, not the token"},
		{Source: "env:ZOOM_PHONE_ACCOUNT_ID", Reason: "the Zoom account ID, reported by fleet and accounts; nothing authenticates with it alone"},
		{Source: "env:ZOOM_PHONE_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:ZOOM_PHONE_TOKEN_URL", Reason: "the OAuth token URL; a credential inside it is held through URLEnv"},
		{Source: "env:ZOOM_PHONE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:ZOOM_PHONE_OAUTH_SCOPE", Reason: "OAuth scope names"},
		{Source: "env:ZOOM_PHONE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:ZOOM_PHONE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:ZOOM_PHONE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:ZOOM_PHONE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:ZOOM_PHONE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:ZOOM_PHONE_DATA_DIR", Reason: "directory path"},
		{Source: "env:ZOOM_PHONE_STATE_DIR", Reason: "directory path"},
		{Source: "env:ZOOM_PHONE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:ZOOM_PHONE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:ZOOM_PHONE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:ZOOM_PHONE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:ZOOM_PHONE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:ZOOM_PHONE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:ZOOM_PHONE_MCP_UPLOAD_ROOT", Reason: "directory path"},
		{Source: "env:ZOOM_PHONE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:ZOOM_PHONE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:ZOOM_PHONE_LEARN_SESSION", Reason: "session grouping label, hashed before it is written"},
		{Source: "env:ZOOM_PHONE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It names the files
// the run takes its settings from, and holds every other file a credential may be
// read from under any --config and --home the command line can mean: the default
// config and store, the legacy file, each client account's private config and
// store (fleet and accounts read them), and the run profile stores. Holding a file
// the run does not load is safe, missing the one it loads is not.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := append(credkit.FlagValues(args, "--config"), "")
	selectedHome, selectedConfig := lastFlag(args, "--home"), lastFlag(args, "--config")
	var out []string
	for _, home := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if consulted, err := config.ConsultedPaths(); err == nil {
			for _, f := range consulted {
				holdFile(f)
			}
		}
		for _, c := range configs {
			run := home == selectedHome && c == selectedConfig
			if files, err := config.ReadPaths(c); err == nil {
				for _, f := range files {
					if run && !slices.Contains(out, f) {
						out = append(out, f)
					} else if !run {
						holdFile(f)
					}
				}
			}
			if stores, err := config.StorePaths(c); err == nil {
				for _, f := range stores {
					holdFile(f) // a store decides nothing about this run's base URL
				}
			}
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			cliutil.HoldProfileStore(filepath.Join(dir, "profiles.json"))
			for _, pattern := range []string{"config.toml", filepath.Join("data", "credentials.toml")} {
				accounts, _ := filepath.Glob(filepath.Join(dir, "accounts", "*", pattern))
				for _, f := range accounts {
					holdFile(f)
				}
			}
		}
		restore()
	}
	cliutil.HoldLegacyProfileStore()
	return out
}

// lastFlag is the value the parser takes for a flag: the last before any "--".
func lastFlag(args []string, name string) string {
	if end := slices.Index(args, "--"); end >= 0 {
		args = args[:end]
	}
	values := credkit.FlagValues(args, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func holdFile(path string) {
	if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
		cliutil.HoldConfigFile(opened.Data)
	}
}

// HoldStdinBody reads a command's --stdin body before anything prints, holds the
// credentials in it with credkit.HoldBody and leaves it on os.Stdin for the
// command. It is a request document, not a credential: StdinFlags would hold the
// whole of it and each line, so a response repeating a field would lose its result
// and a pretty-printed body would refuse to start. Bytes that are no document are
// held whole. Call it straight after credkit.Start.
func HoldStdinBody() {
	asked := false
	for _, a := range os.Args[1:] { // a read only: argv reaches no call (tools/maintainer/mcp_file_flags)
		asked = asked || a == "--stdin" || strings.HasPrefix(a, "--stdin=")
	}
	if !asked {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err == nil {
		credkit.HoldBody(b)
		var r, w *os.File
		if r, w, err = os.Pipe(); err == nil {
			go func() {
				_, _ = w.Write(b)
				_ = w.Close()
			}()
			os.Stdin = r
			return
		}
	}
	fmt.Fprintln(os.Stderr, "reading stdin:", err)
	credkit.Exit(66)
}
