// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"riverside-fm-pp-cli/internal/cliutil"
	"riverside-fm-pp-cli/internal/config"
	"riverside-fm-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// The credential is a Riverside browser session: the riverside_auth_access and
// riverside_auth_refresh cookies, imported by auth login or set by auth
// set-token, kept in config.toml, the 0600 credentials.toml store and the 0600
// cookies.json jar, and sent only to the jar's domain. The session slides, so
// every response can rotate it; the kit's transport holds each Set-Cookie value.
//
// Those three files are credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile, client.cookieJar), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"RIVERSIDE_FM_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"RIVERSIDE_FM_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files: credentialFiles,
	// access_token is not listed: it holds the whole Cookie header, names
	// included, and a held "riverside_auth_access=..." would withhold every output
	// that says "riverside". holdSession holds what it really carries.
	ConfigFields:  []string{"auth_header", "refresh_token", "client_secret"},
	StoredRecords: []string{"auth_header", "refresh_token", "client_secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "RIVERSIDE_FM_BASE_URL",
		Default:   "https://riverside.com",
	},
	Origins: []credkit.Origin{
		// The session goes to the configured base URL. The jar is seeded for
		// riverside.com only, so an overridden base URL receives no cookie.
		{Source: "*", Allowed: []string{"base_url"}},
		{Source: "minted:browser-cookie", Allowed: []string{"https://riverside.com"}},
		{Source: "stored:cookie", Allowed: []string{"https://riverside.com"}},
		{Source: "config:access_token", Allowed: []string{"base_url"}},
		// A studio join secret is removed from every response and sent nowhere.
		{Source: "response:join-secret", Allowed: nil},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:RIVERSIDE_FM_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret"},
		{Source: "config:credential_domain", Reason: "the domain a browser session was captured from, a host name"},
		{Source: "env:RIVERSIDE_FM_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:RIVERSIDE_FM_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:RIVERSIDE_FM_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:RIVERSIDE_FM_SKIP_TLS_VERIFY", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:RIVERSIDE_FM_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:RIVERSIDE_FM_DATA_DIR", Reason: "directory path"},
		{Source: "env:RIVERSIDE_FM_STATE_DIR", Reason: "directory path"},
		{Source: "env:RIVERSIDE_FM_CACHE_DIR", Reason: "directory path"},
		{Source: "env:RIVERSIDE_FM_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:RIVERSIDE_FM_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:RIVERSIDE_FM_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:RIVERSIDE_FM_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:RIVERSIDE_FM_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:RIVERSIDE_FM_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:RIVERSIDE_FM_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:RIVERSIDE_FM_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:RIVERSIDE_FM_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:RIVERSIDE_FM_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:RIVERSIDE_FM_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:SPACEFAST_API_URL", Reason: "Spacefast API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:SPACEFAST_TEAM", Reason: "Spacefast team id, not a credential"},
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
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or RIVERSIDE_FM_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("RIVERSIDE_FM_CONFIG")
	}
	var used []string
	if len(homes) == 0 {
		used = runFiles(explicit)
		holdSessions(used)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		holdSessions(files)
		holdOnly(runFiles(""))
		holdDirSources()
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	holdDirSources()
	for _, cfg := range configs {
		holdOnly(runFiles(cfg))
	}
	return used
}

// startupNamedConfigOrHome records that the command line named a --config or
// --home, which Start read. Set once, inside Start, before main goes on.
var startupNamedConfigOrHome bool

// StartupNamedConfigOrHome reports whether the command line named a --config
// or --home that Start read. The MCP server has neither flag, so it refuses to
// run when Start took a config, and with it an allowed origin, from one.
func StartupNamedConfigOrHome() bool { return startupNamedConfigOrHome }

// runFiles is what a run with this explicit config (or none) reads, in the
// order config.Load prefers it: the config file, then the legacy config when no
// explicit one was named and the default is absent. The credential stores are
// held here, not returned: config.Load takes no base URL from a store, so one
// written there must not become an allowed origin.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			holdFile(store)
		}
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); err != nil {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		holdFile(store)
	}
	return files
}

func holdOnly(files []string) {
	for _, f := range files {
		holdFile(f)
	}
}

// holdDirSources holds the credential sources that are not config files: the
// persistent cookie jar and the run-profile store.
func holdDirSources() {
	if dir, err := cliutil.DataDir(); err == nil {
		holdCookieJar(filepath.Join(dir, "cookies.json"))
	}
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".riverside-fm-cli", "profiles.json"))
	}
}

// holdCookieJar holds every cookie value in the jar, and not its names or
// domains: "riverside.com" is ordinary output. A jar that does not parse is
// held whole, every word of it.
func holdCookieJar(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	var jar []struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(data, &jar) != nil {
		credkit.HoldBody(data)
		return
	}
	for _, c := range jar {
		if c.Value != "" {
			credkit.Hold("stored:cookie", c.Value)
		}
	}
}

// holdFile holds what one credential source carries, read by declared name.
// A missing file holds nothing.
func holdFile(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
	holdSession(data)
}

func holdSessions(files []string) {
	for _, f := range files {
		if data, err := readRegular(f); err == nil {
			holdSession(data)
		}
	}
}

// holdSession holds the access_token of a config or credential store. Auth
// login stores the whole Cookie header there: its values are held and its
// names are not. Auth set-token stores a bare JWT, held whole. A file that does
// not parse has already been held whole, every word of it.
func holdSession(data []byte) {
	var doc struct {
		AccessToken string `toml:"access_token"`
	}
	if toml.Unmarshal(data, &doc) != nil || doc.AccessToken == "" {
		return
	}
	first, _, _ := strings.Cut(doc.AccessToken, ";")
	name, _, isPair := strings.Cut(first, "=")
	// A base64-padded JWT also has an "=", but a dot in its first name.
	if !isPair || strings.ContainsAny(name, "./") {
		credkit.Hold("config:access_token", doc.AccessToken)
		return
	}
	for _, part := range strings.Split(doc.AccessToken, ";") {
		_, value, ok := strings.Cut(part, "=")
		if !ok {
			value = part
		}
		if value = strings.TrimSpace(value); value != "" {
			credkit.Hold("stored:cookie", value)
		}
	}
}

// readRegular reads path only when it is a regular file, so a FIFO or a device
// named as a source cannot stall the start.
func readRegular(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(filepath.Clean(path)) // #nosec G304 -- a credential source this program reads anyway
}
