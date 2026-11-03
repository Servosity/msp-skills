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

	"connectwise-automate-pp-cli/internal/cliutil"
	"connectwise-automate-pp-cli/internal/config"
	"connectwise-automate-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// ConnectWise Automate authenticates with a bearer token (from
// CONNECTWISE_AUTOMATE_TOKEN, config.toml or the 0600 credentials.toml store)
// and, on v2020.11 and later, the integration's clientId header
// (CONNECTWISE_AUTOMATE_CLIENT_ID, injected into Config.Headers by
// config.Load). Both go to the configured Automate server only. A bearer token
// is minted by `apitoken mint` (user name, password, two-factor passcode) or
// renewed by `apitoken refresh` (the current token); the answer's AccessToken
// is held before the program reads it.
//
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile), because the kit would withhold
// them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CONNECTWISE_AUTOMATE_TOKEN",
		// The integration GUID rides as the clientId header on every request; it
		// is treated as a credential (redirect_credentials.go strips it off the
		// origin too).
		"CONNECTWISE_AUTOMATE_CLIENT_ID",
		"CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	// A URL can carry a password or query value: those are held.
	URLEnv: []string{
		"CONNECTWISE_AUTOMATE_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"automate_token", "access_token", "refresh_token", "client_id", "client_secret", "auth_header"},
	StoredRecords: []string{"automate_token", "access_token", "refresh_token", "client_id", "client_secret", "auth_header"},
	// apitoken mint takes --password and --two-factor, apitoken refresh takes
	// the current --token.
	CredentialFlags: []string{"--password", "--two-factor", "--token"},
	StdinFlags:      []string{"--stdin"},
	// The request body fields of POST /apitoken and POST /apitoken/refresh.
	CredentialFields: []string{"Password", "TwoFactorPasscode", "Token"},
	// The TokenResult both answer with.
	ResponseFields: []string{"AccessToken"},
	// The typed MCP tools apitoken_mint and apitoken_refresh, and the body
	// field spellings.
	MCPArguments: []string{"password", "two-factor", "token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CONNECTWISE_AUTOMATE_BASE_URL",
		// A template, not an origin: {server} is the operator's own Automate
		// host (CONNECTWISE_AUTOMATE_SERVER or template_vars.server). The kit
		// cannot resolve it, so showBaseURL resolves it as config.Load and the
		// client do and shows the kit the result; with nothing resolved (or the
		// shipped YOUR_SERVER placeholder) the kit allows the credentials no
		// host and refuses every credentialed request.
		Default: "https://{server}/cwa/api/v1",
	},
	Origins: []credkit.Origin{
		// The bearer token, the clientId header, the credential flags and
		// fields and a minted token go only to the configured Automate server.
		// The connector's own pin (base-url-pinned-at-construction) narrows
		// every request to the same base, redirects included.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		// Config keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "the Automate server host name that fills {server} in the base URL, printed in every API URL"},
		{Source: "env:CONNECTWISE_AUTOMATE_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CONNECTWISE_AUTOMATE_SERVER", Reason: "the Automate server host name that fills {server} in the base URL, printed in every API URL"},
		{Source: "env:CONNECTWISE_AUTOMATE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CONNECTWISE_AUTOMATE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CONNECTWISE_AUTOMATE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CONNECTWISE_AUTOMATE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CONNECTWISE_AUTOMATE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_AUTOMATE_DATA_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_AUTOMATE_STATE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_AUTOMATE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:CONNECTWISE_AUTOMATE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CONNECTWISE_AUTOMATE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CONNECTWISE_AUTOMATE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CONNECTWISE_AUTOMATE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_AUTOMATE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_AUTOMATE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CONNECTWISE_AUTOMATE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CONNECTWISE_AUTOMATE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CONNECTWISE_AUTOMATE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:CONNECTWISE_AUTOMATE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_AUTOMATE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_AUTOMATE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_AUTOMATE_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:CONNECTWISE_AUTOMATE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
		{Source: "env:CLAUDE_SESSION_ID", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:CODEX_SESSION_ID", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:CODEX_THREAD_ID", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "an agent harness session id; the learn journal stores only its hash"},
		{Source: "env:HTTP_PROXY", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:HTTPS_PROXY", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:ALL_PROXY", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:http_proxy", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:https_proxy", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:all_proxy", Reason: "a proxy URL; the kit holds its user and password itself, bound to that proxy"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or CONNECTWISE_AUTOMATE_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
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
		explicit = os.Getenv("CONNECTWISE_AUTOMATE_CONFIG")
	}
	lastHome := ""
	if len(homes) > 0 {
		lastHome = homes[len(homes)-1]
	}
	showBaseURL(lastHome, explicit)
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

// holdProfileStores holds what a run profile store carries under a credential
// flag's name (password, two-factor, token), every duplicate key included: the
// config directory's profiles.json and the legacy one. credkit.HoldBody reads a
// file that does not parse as every word of it.
func holdProfileStores() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".connectwise-automate-cli", "profiles.json"))
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

// shippedServerPlaceholder is the {server} value config.Load falls back to
// when neither CONNECTWISE_AUTOMATE_SERVER nor template_vars.server is set.
// It names no operator's server, so it is never shown to the kit.
const shippedServerPlaceholder = "YOUR_SERVER.hostedrmm.com"

// savedBaseURL is CONNECTWISE_AUTOMATE_BASE_URL as it was before showBaseURL
// set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// default base is the template https://{server}/cwa/api/v1, which the kit
// cannot resolve, so it resolves the base the way the program does: config.Load
// under this run's --home and --config, then every {placeholder} filled from
// the loaded template variables (CONNECTWISE_AUTOMATE_SERVER, template_vars,
// then the shipped placeholder), as the client's buildURL does. The kit reads
// CONNECTWISE_AUTOMATE_BASE_URL right after Files returns; AfterStart puts the
// variable back. When Load refuses, the config's credential route is blocked
// (an untrusted config names another server), a placeholder is left, or the
// server is the shipped placeholder, the variable is left alone and the
// template allows no origin: a request carrying a credential is refused.
func showBaseURL(home, configPath string) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil || cfg.CredentialRouteBlocked() {
		return
	}
	if strings.EqualFold(strings.TrimSpace(cfg.TemplateVars["server"]), shippedServerPlaceholder) && strings.Contains(cfg.BaseURL, "{server}") {
		return
	}
	base := strings.TrimSpace(cfg.BaseURL)
	for k, v := range cfg.TemplateVars {
		if v != "" {
			base = strings.ReplaceAll(base, "{"+k+"}", v)
		}
	}
	if base == "" || strings.ContainsAny(base, "{}") {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("CONNECTWISE_AUTOMATE_BASE_URL")
	baseURLShown = os.Setenv("CONNECTWISE_AUTOMATE_BASE_URL", base) == nil
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// CONNECTWISE_AUTOMATE_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("CONNECTWISE_AUTOMATE_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("CONNECTWISE_AUTOMATE_BASE_URL")
	}
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
