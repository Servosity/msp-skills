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

	"td-synnex-pp-cli/internal/cliutil"
	"td-synnex-pp-cli/internal/config"
	"td-synnex-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// StreamOne ION authenticates with a 2-hour bearer access token. It comes from
// TD_SYNNEX_ACCESS_TOKEN, a file-stored synnex_access_token in config.toml, or
// the 0600 credentials file `auth mint` and `auth set-token` save. `auth mint`
// posts a single-use refresh token (piped with --refresh-token-stdin, the one
// the last mint stored, or TD_SYNNEX_REFRESH_TOKEN, which internal/cli/auth_ion.go
// reads outside Config) to <base URL host>/oauth/token and gets a new access
// token and a rotated refresh token back; `auth check` posts the access token
// to <base URL host>/oauth/validateAccess. Both OAuth endpoints are on the
// configured base URL's origin, so every ION credential goes to that origin
// and nowhere else.
//
// The credentials file, config.toml and the refresh-token rescue file auth
// mint writes when a save fails are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"TD_SYNNEX_ACCESS_TOKEN",
		"TD_SYNNEX_REFRESH_TOKEN",
		"TD_SYNNEX_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"TD_SYNNEX_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "synnex_access_token", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"auth_header", "synnex_access_token", "access_token", "refresh_token", "client_secret"},
	// auth mint --refresh-token-stdin pipes the refresh token; --stdin is the
	// JSON request body every write command takes (ruling 8a). auth set-token
	// reads stdin without a flag: readSecretFromStdin holds that token the
	// moment it is read (cliutil.Hold("stdin")).
	StdinFlags: []string{"--refresh-token-stdin", "--stdin"},
	// The OAuth forms: refresh_token (auth mint) and access_token (auth
	// check). password is the customer-user password a --stdin body can carry.
	CredentialFields: []string{"refresh_token", "access_token", "password"},
	// The token exchange answers with access_token and refresh_token. ION
	// documents password as a credential in two responses (CustomerUser.password,
	// the generated initial password of a new customer's default user, and
	// ProvisionResponse.userCredentials.password, the generated tenant admin
	// password); the ion redaction transport also replaces both before the
	// program sees a response.
	ResponseFields: []string{"access_token", "refresh_token", "password"},
	// A password nested in an endpoint tool's body argument (provisionData,
	// orderItems): the confirm preview echoes the arguments.
	MCPArguments: []string{"password"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "TD_SYNNEX_BASE_URL",
		Default:   "https://ion.tdsynnex.com/api/v3",
	},
	Origins: []credkit.Origin{
		// Every ION credential goes only to the configured base URL's origin:
		// the API, and /oauth/token and /oauth/validateAccess on the same host
		// (internal/cli/auth_ion.go ionOAuthURL).
		{Source: "*", Allowed: []string{"base_url"}},
		// The tokens auth mint receives (held by auth_ion.go before anything
		// can print them).
		{Source: "minted:ion-access-token", Allowed: []string{"base_url"}},
		{Source: "minted:ion-refresh-token", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:TD_SYNNEX_MCP_HTTP_TOKEN", Allowed: nil},
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
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; ION's refresh-token grant does not use it"},
		{Source: "config:template_vars", Reason: "the ION account ID table (accountId), printed in every API path"},
		{Source: "env:TD_SYNNEX_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:TD_SYNNEX_ACCOUNT_ID", Reason: "the ION reseller account ID, printed in every API path"},
		{Source: "env:TD_SYNNEX_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:TD_SYNNEX_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:TD_SYNNEX_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:TD_SYNNEX_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:TD_SYNNEX_DATA_DIR", Reason: "directory path"},
		{Source: "env:TD_SYNNEX_STATE_DIR", Reason: "directory path"},
		{Source: "env:TD_SYNNEX_CACHE_DIR", Reason: "directory path"},
		{Source: "env:TD_SYNNEX_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:TD_SYNNEX_DB", Reason: "a local store path a selected client profile refuses (platform_client.go); never read as a value"},
		{Source: "env:TD_SYNNEX_DATABASE", Reason: "a local store path a selected client profile refuses (platform_client.go); never read as a value"},
		{Source: "env:TD_SYNNEX_DATA_FILE", Reason: "a local store path a selected client profile refuses (platform_client.go); never read as a value"},
		{Source: "env:TD_SYNNEX_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:TD_SYNNEX_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:TD_SYNNEX_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:TD_SYNNEX_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:TD_SYNNEX_LEARN_SESSION", Reason: "session grouping label; the journal appends through credkit.AppendFile, so a held credential set here is still withheld"},
		{Source: "env:TD_SYNNEX_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:TD_SYNNEX_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:TD_SYNNEX_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:TD_SYNNEX_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:TD_SYNNEX_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:TD_SYNNEX_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:TD_SYNNEX_TAIL_MAX_PAGES", Reason: "a page count"},
		{Source: "env:TD_SYNNEX_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:CODEX_THREAD_ID", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:CODEX_SESSION_ID", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "harness session id; the journal stores only a hash of it"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or TD_SYNNEX_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held (ruling 8b).
// No flag arity is guessed: credkit.FlagValues counts every occurrence.
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
		explicit = os.Getenv("TD_SYNNEX_CONFIG")
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
// run-profile store (ruling 8c: a value saved under a credential flag's name
// or under base-url, every duplicate included) and the refresh-token rescue
// files auth mint writes when it cannot save a rotated refresh token (one in
// the config directory, else one in the temp directory). Each is read by the
// kit's declared names (credkit.HoldBody); a rescue file is one bare token,
// which HoldBody holds whole as a file that does not parse.
func holdDirSources() {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return
	}
	holdFile(filepath.Join(dir, "profiles.json"))
	holdFile(filepath.Join(dir, cliutil.IONRescueFileName))
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".td-synnex-cli", "profiles.json"))
	}
	rescues, _ := filepath.Glob(filepath.Join(os.TempDir(), cliutil.IONRescueTempPattern))
	for _, r := range rescues {
		holdFile(r)
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
