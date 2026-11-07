// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"xero-pp-cli/internal/cliutil"
	"xero-pp-cli/internal/config"
	"xero-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Xero authenticates with an OAuth 2.0 bearer. It comes from XERO_ACCESS_TOKEN,
// the legacy XERO_OAUTH2, config.toml or the 0600 credentials.toml store, or is
// minted: `auth login` exchanges an authorization code, and the client refreshes
// an expired bearer with the stored refresh token, both at the token endpoint
// (XERO_TOKEN_URL or token_url, default https://identity.xero.com/connect/token).
// The bearer goes to the configured base URL only.
//
// The client secret, the refresh token, the authorization code and the PKCE
// verifier go only to the token endpoint. The kit's Origins are plain data, so
// they cannot name a token URL chosen at run time: they allow Xero's identity
// origin and the configured base URL (a mock or proxy deployment that serves its
// token endpoint beside the API). A token URL override on any other origin is
// refused by the kit before the secret is sent (handfixes.json credkit-wiring).
//
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile), because the kit would withhold
// them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"XERO_ACCESS_TOKEN",
		"XERO_OAUTH2",
		"XERO_CLIENT_SECRET",
		"XERO_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"XERO_TOKEN_URL",
		"XERO_AUTHORIZATION_URL",
		"XERO_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_secret", "oauth2"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_secret", "oauth2"},
	// auth login --client-secret.
	CredentialFlags: []string{"--client-secret"},
	StdinFlags:      []string{"--stdin"},
	// The token-endpoint form of a login or a refresh.
	CredentialFields: []string{"client_secret", "refresh_token"},
	ResponseFields:   []string{"access_token", "refresh_token", "id_token"},
	// No API tool takes a credential; auth login, which does, is hidden from
	// MCP. These names are held wherever a call states them anyway.
	MCPArguments: []string{"client-secret", "client_secret", "access_token", "refresh_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "XERO_BASE_URL",
		Default:   "https://api.xero.com/api.xro/2.0",
	},
	Origins: []credkit.Origin{
		// Every API credential goes to the configured base URL.
		{Source: "*", Allowed: []string{"base_url"}},
		// The OAuth client secret, the refresh token, the authorization code and
		// the PKCE verifier go to the token endpoint: Xero's identity origin, or
		// a token URL on the base URL's origin.
		{Source: "env:XERO_CLIENT_SECRET", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "flag:--client-secret", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "config:client_secret", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "config:refresh_token", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "field:client_secret", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "field:refresh_token", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "response:refresh_token", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "minted:auth-code", Allowed: []string{"https://identity.xero.com", "base_url"}},
		{Source: "minted:pkce-verifier", Allowed: []string{"https://identity.xero.com", "base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:XERO_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		// Xero's edge sets these on API and identity responses to anyone who
		// connects, signed in or not. This connector sends no cookies (its HTTP
		// clients have no cookie jar), so none authenticates anything it does;
		// held, a 452-character ak_bmsc withheld ordinary output that shared any
		// 8-character piece with it (edge_cookie_test.go).
		{Source: "cookie:ak_bmsc", Reason: "Akamai Bot Manager cookie set by api.xero.com's edge on every response; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "cookie:bm_sv", Reason: "Akamai Bot Manager cookie set by Xero's edge; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "cookie:bm_sz", Reason: "Akamai Bot Manager cookie set by Xero's edge; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "cookie:bm_mi", Reason: "Akamai Bot Manager cookie set by Xero's edge; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "cookie:_abck", Reason: "Akamai Bot Manager cookie set by Xero's edge; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "cookie:Device", Reason: "device-recognition cookie identity.xero.com sets on every response, the token endpoint included, before any sign-in; authenticates nothing and this connector sends no cookies (no cookie jar)"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; auth login prints it in the authorize URL it opens"},
		{Source: "config:authorization_url", Reason: "the OAuth authorize endpoint URL, opened in a browser; it carries no credential"},
		{Source: "config:token_url", Reason: "the OAuth token endpoint URL; a credential sent to it is judged by the outbound guard"},
		{Source: "config:refresh_login_id", Reason: "a random label naming one login across refresh-token rotations; it authenticates nothing"},
		// Run-time fields of config.Config (toml:"-" or unexported): never read from a file.
		{Source: "config:AuthSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:CredentialSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:configOwner", Reason: "a run-time note of which file filled the config"},
		{Source: "config:legacySourcePath", Reason: "the legacy path this config was read from"},
		{Source: "config:refreshLoginID", Reason: "the in-memory copy of refresh_login_id, a random login label that authenticates nothing"},
		{Source: "config:Path", Reason: "the path this config was read from, never read from a file"},
		{Source: "config:envOverrides", Reason: "names of the variables that overrode a value"},
		// cliutil.CredentialRefusal: where a refused credential file was.
		{Source: "config:Source", Reason: "a run-time note of where a credential came from, never read from a file"},
		{Source: "config:Err", Reason: "a load error held in memory, never read from a file"},
		// learn/entities.Config: the entity extractor's word lists.
		{Source: "config:variant", Reason: "a learn-entity spelling from the learn config"},
		{Source: "config:canonical", Reason: "a learn-entity name from the learn config"},
		{Source: "config:stopwords", Reason: "learn-search stop words"},
		{Source: "config:synonyms", Reason: "learn-search synonyms"},
		{Source: "header:xero-tenant-id", Reason: "the Xero organisation (tenant) id: Xero requires it on every call and lists it in connection data; it authenticates nothing without the bearer"},
		{Source: "env:XERO_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:XERO_TOKEN_URL", Reason: "the OAuth token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:XERO_AUTHORIZATION_URL", Reason: "the OAuth authorize endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:XERO_CLIENT_ID", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; auth login prints it in the authorize URL it opens"},
		{Source: "env:XERO_TENANT_ID", Reason: "the Xero organisation (tenant) id sent as the Xero-Tenant-Id header; it authenticates nothing without the bearer"},
		{Source: "env:XERO_SCOPES", Reason: "the OAuth scope names auth login requests, printed in the authorize URL"},
		{Source: "env:XERO_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:XERO_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:XERO_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:XERO_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:XERO_DATA_DIR", Reason: "directory path"},
		{Source: "env:XERO_STATE_DIR", Reason: "directory path"},
		{Source: "env:XERO_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XERO_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:XERO_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:XERO_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:XERO_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:XERO_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:XERO_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:XERO_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:XERO_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:XERO_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:XERO_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:XERO_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:XERO_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:XERO_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:XERO_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:XERO_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:XERO_USER_AGENT", Reason: "User-Agent string"},
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
// one: an explicit --config or XERO_CONFIG wins, as it does in config.Load, and
// the last --home decides the default paths, as pflag does. Every other file
// any --config or --home could mean is only held. No flag arity is guessed:
// credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("XERO_CONFIG")
	}
	var used []string
	if len(homes) == 0 {
		used = runFiles(explicit)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		holdOnly(runFiles(""))
		holdProfileStores()
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	holdProfileStores()
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

// holdProfileStores holds the run-profile stores: a value saved under a
// credential flag's name (--client-secret) or under base-url. Each is a JSON
// document the kit reads by its declared names (credkit.HoldBody); a file that
// does not parse is held whole, every word of it.
func holdProfileStores() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".xero-cli", "profiles.json"))
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

// compactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came.
//
// `feedback --stdin` is the exception: feedback text is no credential
// (`feedback <text>` is never held), and held whole its local record was
// withheld while the command said it was recorded. Its body is taken off stdin
// here, so the kit reads an empty one, and RestoreFeedbackStdin hands it back
// after Start. Only a command line that starts with "feedback" qualifies: one
// with a flag in front stays held, which loses the record but never a secret.
func compactStdinBody(args []string) {
	body := false
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		body = body || name == "--stdin" || name == "-stdin"
	}
	if !body {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit's own read of what is left fails closed
	}
	if len(args) > 0 && args[0] == "feedback" {
		feedbackStdin, data = data, nil
	}
	var one bytes.Buffer
	if json.Compact(&one, data) == nil {
		data = one.Bytes()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent, so the command reads an empty body and refuses it
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}

// feedbackStdin is the `feedback --stdin` body compactStdinBody took off stdin.
var feedbackStdin []byte

// RestoreFeedbackStdin gives `feedback --stdin` its body back on os.Stdin. The
// CLI main calls it right after credkit.Start; without a feedback body it does
// nothing.
func RestoreFeedbackStdin() {
	if feedbackStdin == nil {
		return
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // the command reads an empty body and says so
	}
	go func(b []byte) {
		_, _ = w.Write(b)
		_ = w.Close()
	}(feedbackStdin)
	os.Stdin = r
}
