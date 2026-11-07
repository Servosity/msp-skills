// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/config"
	"sonicwall-pp-cli/internal/credkit"
)

// mswAPI is the one MySonicWall API origin (internal/cli mswBase).
const mswAPI = "https://api.mysonicwall.com"

// Declarations is what the connector holds and where each credential may go.
//
// SonicWall has two vendor hosts with one credential each. The MySonicWall API
// key (SONICWALL_MSW_API_KEY or msw_api_key, sent as X-API-KEY) goes only to
// https://api.mysonicwall.com. The NSM session token (SONICWALL_NSM_TOKEN, or
// the token nsm-login mints and saves as access_token in the 0600
// credentials.toml store) goes only to the NSM base URL this run really uses:
// a template, https://nsm-{region}.sonicwall.com/api/manager, which
// showBaseURL resolves the way the client does. The token exchanges mint two
// more credentials, each held where the response is read
// (internal/cli/sonicwall_secret_redact.go holdExchangeCredentials): the
// access code MySonicWall returns, and the session token NSM returns.
//
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile), because the kit would withhold
// them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"SONICWALL_MSW_API_KEY",
		"SONICWALL_NSM_TOKEN",
		"SONICWALL_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"SONICWALL_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "nsm_token", "msw_api_key", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "nsm_token", "msw_api_key", "access_token", "refresh_token", "client_id", "client_secret"},
	// nsm-session sso takes the MySonicWall access code as --code, as the code
	// field of its request body (or of a --stdin body), and over MCP as the
	// nsm_session_sso tool's code argument.
	CredentialFlags:   []string{"--code"},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"X-API-KEY"},
	CredentialFields:  []string{"code"},
	MCPArguments:      []string{"code"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SONICWALL_BASE_URL",
		// A template the kit cannot resolve, so it allows no origin unless
		// showBaseURL resolves the base the run really uses.
		Default: "https://nsm-{region}.sonicwall.com/api/manager",
	},
	Origins: []credkit.Origin{
		// The NSM token, the access code and every other credential go only to
		// the NSM base URL this run uses.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MySonicWall key goes only to MySonicWall's API, never to NSM,
		// whatever SONICWALL_BASE_URL says.
		{Source: "env:SONICWALL_MSW_API_KEY", Allowed: []string{mswAPI}},
		{Source: "config:msw_api_key", Allowed: []string{mswAPI}},
		{Source: "header:X-API-KEY", Allowed: []string{mswAPI}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:SONICWALL_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API, with
		// SPACEFAST_TOKEN or an anonymous page's key (deliver_spacefast.go holds
		// it before an update sends it).
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "the NSM region that fills {region} in the base URL, printed in every NSM URL"},
		{Source: "env:SONICWALL_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:SONICWALL_REGION", Reason: "the NSM region that fills {region} in the base URL, printed in every NSM URL"},
		{Source: "env:SONICWALL_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SONICWALL_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SONICWALL_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SONICWALL_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:SONICWALL_DATA_DIR", Reason: "directory path"},
		{Source: "env:SONICWALL_STATE_DIR", Reason: "directory path"},
		{Source: "env:SONICWALL_CACHE_DIR", Reason: "directory path"},
		{Source: "env:SONICWALL_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:SONICWALL_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SONICWALL_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SONICWALL_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:SONICWALL_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:SONICWALL_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SONICWALL_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:SONICWALL_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SONICWALL_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SONICWALL_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:SONICWALL_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:SONICWALL_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SONICWALL_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SONICWALL_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SONICWALL_USER_AGENT", Reason: "User-Agent string"},
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

// credentialFiles runs inside Start, before anything can print. It shows the kit
// the base URL this run will use (showBaseURL), then returns every file a
// credential may be read from, the run's own first: the config an explicit
// --config or SONICWALL_CONFIG names (it wins, as in config.Load), else the
// default and legacy config, under the last --home (as pflag takes it), then
// the credential stores. Every file any other --config or --home could mean
// follows: holding a file the run does not read is safe, missing the one it
// reads is not, and the base URL the kit allows is the one showBaseURL set. No
// flag arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := last(configs)
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("SONICWALL_CONFIG")
	}
	showBaseURL(last(homes), explicit)
	var out []string
	add := func(files []string) {
		for _, f := range files {
			if f != "" && !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	underHome(last(homes), func() { add(runFiles(explicit)) })
	for _, home := range append(slices.Clone(homes), "") {
		underHome(home, func() {
			add(runFiles(""))
			for _, c := range configs {
				add(runFiles(c))
			}
			holdProfileStores()
		})
	}
	return out
}

// startupNamedConfigOrHome records that the command line named a --config or
// --home, which Start read. Set once, inside Start, before main goes on.
var startupNamedConfigOrHome bool

// StartupNamedConfigOrHome reports whether the command line named a --config
// or --home that Start read. The MCP server has neither flag, so it refuses to
// run when Start took a config, and with it an allowed origin, from one.
func StartupNamedConfigOrHome() bool { return startupNamedConfigOrHome }

// runFiles is what a run with this explicit config (or none) reads, in the
// order config.Load prefers it: the config file, the legacy config, then the
// credential stores (the one beside an explicit config, and the default one).
func runFiles(explicit string) []string {
	var files []string
	if strings.TrimSpace(explicit) != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
		}
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		files = append(files, filepath.Join(dir, "config.toml"))
		if legacy, err := config.LegacyConfigPath(); err == nil {
			files = append(files, legacy)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		files = append(files, store)
	}
	return files
}

// holdProfileStores holds what a run profile store carries under a credential
// name (the code flag), every duplicate key included, with the kit's
// credkit.HoldBody. The kit's file reader is not given the store: it is nested
// JSON, which that reader answers by holding every string in it, so a profile
// named "p" or a saved --limit 7 would withhold every later output. A store that
// does not parse is held whole, every word of it.
func holdProfileStores() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdBody(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdBody(filepath.Join(home, ".sonicwall-cli", "profiles.json"))
	}
}

func holdBody(path string) {
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

// underHome runs f with home as --home would set it. The run refuses a --home
// that does not apply before it reads a file, so f is skipped.
func underHome(home string, f func()) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	f()
}

// last is the value the flag parser takes for a repeated flag: the last one.
func last(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// SONICWALL_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// built-in base is the template https://nsm-{region}.sonicwall.com/api/manager,
// which the kit cannot resolve, so it resolves the base the way the client
// does: config.Load under this run's --home and --config (which applies
// SONICWALL_BASE_URL, the config's base_url when the file is trusted, and the
// region from SONICWALL_REGION, template_vars or the default), then
// client.ResolvedBaseURL over the loaded template variables. The kit reads
// SONICWALL_BASE_URL right after Files returns; AfterStart puts the variable
// back. When Load refuses or a placeholder stays unresolved, the variable is
// left alone and the template allows no origin: a request carrying a
// credential is refused.
func showBaseURL(home, configPath string) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil {
		return
	}
	base := client.ResolvedBaseURL(strings.TrimRight(cfg.BaseURL, "/"), cfg.TemplateVars)
	if base == "" || strings.ContainsAny(base, "{}") {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("SONICWALL_BASE_URL")
	baseURLShown = os.Setenv("SONICWALL_BASE_URL", base) == nil
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// SONICWALL_BASE_URL back as the process found it, and gives `feedback --stdin`
// its body back on os.Stdin (without a feedback body that part does nothing).
func AfterStart() {
	restoreFeedbackStdin()
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("SONICWALL_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("SONICWALL_BASE_URL")
	}
}

// compactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came.
//
// `feedback --stdin` is the exception: feedback text is no credential
// (`feedback <text>` is never held), and held whole its local record would be
// withheld while the command said it was recorded. Its body is taken off stdin
// here, so the kit reads an empty one, and AfterStart hands it back after
// Start. Only a command line that starts with "feedback" qualifies: one with a
// flag in front stays held, which loses the record but never a secret.
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

// restoreFeedbackStdin gives `feedback --stdin` its body back on os.Stdin.
func restoreFeedbackStdin() {
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
	feedbackStdin = nil
	os.Stdin = r
}
