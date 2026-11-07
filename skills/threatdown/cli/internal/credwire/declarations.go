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

	"github.com/pelletier/go-toml/v2"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/config"
	"threatdown-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// ThreatDown OneView authenticates with an OAuth2 client pair
// (THREATDOWN_CLIENT_ID, THREATDOWN_CLIENT_SECRET) from the environment, auth
// login's flags, config.toml or the 0600 credentials.toml. The pair is sent as
// HTTP Basic client authentication to the token URL (config token_url,
// THREATDOWN_TOKEN_URL, else https://api.threatdown.com/oneview/oauth2/token),
// and the access_token the token endpoint answers with is the bearer for the
// API at the configured base URL. auth set-token reads a bearer on stdin
// instead.
//
// Every credential is bound to the API this run is configured for (base_url).
// The client pair, and a stored refresh token, are also bound to the default
// token URL's origin, https://api.threatdown.com: the program sends them there
// whenever no token URL is configured, also when the base URL is overridden. A
// token URL on any other origin than these two is refused by the kit (fail
// closed, .review/followups.md).
//
// The credential store, config.toml and the Spacefast delivery record are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile), because the kit would withhold them by
// contract.
//
// The vendor secret fields OneView returns inside ordinary rows
// (as_uninstall_password, secret, secret_hash, secret_token, custom_headers;
// installer and diag-log links) are not ResponseFields: governance.md documents
// the reads that return them as the Credential tier (policies secret-get and
// sites secret exist to return a tamper-protection password), and holding them
// would withhold the whole output of policies list, webhooks list and every
// export of those resources. They stay out of the local store, the response
// cache and tail's stream (handfixes.json secret-fields-never-persisted).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"THREATDOWN_CLIENT_ID",
		"THREATDOWN_CLIENT_SECRET",
		"THREATDOWN_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"THREATDOWN_TOKEN_URL",
		"THREATDOWN_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	// auth login's pair.
	CredentialFlags: []string{"--client-id", "--client-secret", "--secret", "--as-uninstall-password", "--secret-token"},
	// Every command that reads a request body (or feedback text) on stdin.
	StdinFlags: []string{"--stdin"},
	// The refresh grant's form (client_secret, refresh_token). client_id is not
	// listed: it is the first field of the sorted form, which the kit's
	// key-value reading takes as one line, holding the rest of the form as its
	// value. Its value is held from every source it comes from:
	// THREATDOWN_CLIENT_ID, --client-id and the client_id of config.toml and
	// credentials.toml.
	CredentialFields: []string{"client_secret", "refresh_token", "access_token"},
	// What the token endpoint answers with (the automatic mint, auth login and
	// the refresh grant).
	ResponseFields: []string{"access_token", "refresh_token", "id_token"},
	// auth login is hidden from MCP; a mirrored tool still never takes the
	// pair. The flag spellings also hold a profiles.json value saved under a
	// credential flag's name (credkit.HoldBody reads MCPArguments names).
	//
	// The rest are the request fields the OneView spec defines as credentials
	// (policy and site tamper-protection secrets, uninstall passwords, a
	// webhook subscription's HMAC signing token, an installation token), in
	// both spellings, so a value an MCP client passes is held before the
	// preview or the request carries it (Codex r1, mcp-request-credential).
	MCPArguments: []string{"client_id", "client_secret", "client-id", "client-secret", "access_token", "refresh_token",
		"secret", "password", "uninstall_password", "uninstall-password", "as_uninstall_password", "as-uninstall-password",
		"secret_token", "secret-token", "token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "THREATDOWN_BASE_URL",
		Default:   "https://api.threatdown.com/oneview",
	},
	Origins: []credkit.Origin{
		// The bearer (minted, saved, or set with auth set-token) goes only to
		// the API this run is configured for.
		{Source: "*", Allowed: []string{"base_url"}},
		// The client pair and a refresh token also go to the default token
		// URL's origin (client.effectiveTokenURL), which stays in use when only
		// the base URL is overridden.
		{Source: "env:THREATDOWN_CLIENT_ID", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "env:THREATDOWN_CLIENT_SECRET", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "flag:--client-id", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "flag:--client-secret", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "config:client_id", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "config:client_secret", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "config:refresh_token", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		{Source: "response:refresh_token", Allowed: []string{"base_url", "https://api.threatdown.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:THREATDOWN_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
		// A proxy's password goes to that proxy only: the kit's transport adds its origin.
		{Source: "proxy-userinfo", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the OneView API address; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held by holdURLSettings"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "config:token_binding", Reason: "a truncated SHA-256 fingerprint of client ID, scope and token URL (config.TokenBindingFor); it authenticates nothing (handfixes.json token-bound-to-client)"},
		{Source: "config:refresh_login_id", Reason: "a random label naming one login across refresh-token rotations; it authenticates nothing"},
		{Source: "env:THREATDOWN_BASE_URL", Reason: "the OneView API address; a credential inside it is held through BaseURL"},
		{Source: "env:THREATDOWN_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:THREATDOWN_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:THREATDOWN_OAUTH_SCOPE", Reason: "OAuth scope names (execute, read, write)"},
		{Source: "env:THREATDOWN_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:THREATDOWN_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:THREATDOWN_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:THREATDOWN_HOME", Reason: "root directory for config, data, state and cache; Files resolves it"},
		{Source: "env:THREATDOWN_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:THREATDOWN_DATA_DIR", Reason: "directory path"},
		{Source: "env:THREATDOWN_STATE_DIR", Reason: "directory path"},
		{Source: "env:THREATDOWN_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:THREATDOWN_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:THREATDOWN_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:THREATDOWN_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:THREATDOWN_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:THREATDOWN_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:THREATDOWN_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:THREATDOWN_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:THREATDOWN_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:THREATDOWN_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
		{Source: "env:HTTP_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:HTTPS_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:ALL_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:NO_PROXY", Reason: "a list of hosts"},
		{Source: "env:http_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:https_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:all_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:no_proxy", Reason: "a list of hosts"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or THREATDOWN_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	holdURLSettings()
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("THREATDOWN_CONFIG")
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
	for _, f := range used {
		holdConfigURLs(f)
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

// holdDirSources holds the run-profile stores (the config directory's
// profiles.json and the legacy ~/.threatdown-cli/profiles.json). A profile
// value saved under a credential flag's name (client-secret, client-id) is
// read by its declared name (credkit.HoldBody, MCPArguments), every duplicate
// included; a store that does not parse is held whole, every word of it. This
// CLI has no --base-url flag, so no profile value sets the base URL.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".threatdown-cli", "profiles.json"))
	}
}

// holdURLSettings holds the password and every query value of each URL setting
// in the environment with the kit's own reader, credkit.HoldURL, which also
// reads them in an address with a typo (https//u:pw@h, u:pw@h). The kit's
// BaseURL and URLEnv readers hold the well-formed ones; the program prints a
// malformed one back in a connection error and in doctor.
func holdURLSettings() {
	for _, name := range []string{"THREATDOWN_BASE_URL", "THREATDOWN_TOKEN_URL", "THREATDOWN_FEEDBACK_ENDPOINT", "SPACEFAST_API_URL"} {
		if v := os.Getenv(name); v != "" {
			credkit.HoldURL("userinfo", v)
		}
	}
}

// holdConfigURLs holds what the base_url and token_url of the file at path
// carry (a config file or a credential store), read with credkit.HoldURL, which
// also reads a malformed address the program would print back. It reads a
// regular file only, from the descriptor it judged, so a FIFO or a device named
// as a config cannot stall startup; a file that does not parse is held whole by
// the kit itself (Files) or by credkit.HoldBody (holdFile).
func holdConfigURLs(path string) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- a credential source this program reads anyway
	if err != nil {
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return
	}
	var urls struct {
		BaseURL  string `toml:"base_url"`
		TokenURL string `toml:"token_url"`
	}
	data, err := io.ReadAll(io.LimitReader(f, maxHeldConfigBytes))
	if err != nil || toml.Unmarshal(data, &urls) != nil {
		return
	}
	for _, v := range []string{urls.BaseURL, urls.TokenURL} {
		if v != "" {
			credkit.HoldURL("userinfo", v)
		}
	}
}

// maxHeldConfigBytes bounds what holdConfigURLs reads of one file; a config is
// a few hundred bytes.
const maxHeldConfigBytes = 8 << 20

// holdFile holds what one credential source carries, read by declared name.
// A missing file holds nothing.
func holdFile(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
	holdConfigURLs(path)
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
