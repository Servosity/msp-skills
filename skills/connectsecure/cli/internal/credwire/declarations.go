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

	"connectsecure-pp-cli/internal/cliutil"
	"connectsecure-pp-cli/internal/config"
	"connectsecure-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// ConnectSecure mints a JWT: POST <pod>/w/authorize with Client-Auth-Token =
// base64(tenant + "+" + client_id + ":" + client_secret); the answer's
// data.access_token is the bearer and data.user_id goes in X-USER-ID
// (internal/cli/connectsecure_auth.go). A pre-minted JWT can come from
// CONNECTSECURE_ACCESS_TOKEN, config.toml or the 0600 credentials.toml store.
// Every credential goes to the configured pod only.
//
// The store, config.toml and the JWT token cache
// (<cache dir>/connectsecure-token-<hash>.json) are credential SOURCES: Start
// reads them, and their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile), because the kit would withhold them by
// contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CONNECTSECURE_CLIENT_SECRET",
		// The client id is half of the API key pair the portal issues (Settings >
		// Users > API Key) and is encoded into Client-Auth-Token with the secret.
		"CONNECTSECURE_CLIENT_ID",
		"CONNECTSECURE_ACCESS_TOKEN",
		"CONNECTSECURE_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"CONNECTSECURE_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	// session --client-auth-token is held by holdClientAuthTokenFlag, not
	// here: its "-" value means "read stdin".
	StdinFlags: []string{"--stdin"},
	// The mint header; Authorization and every [headers] value are held by the kit itself.
	CredentialHeaders: []string{"Client-Auth-Token"},
	// The /w/authorize answer's data.access_token is the minted JWT.
	ResponseFields: []string{"access_token"},
	MCPArguments:   []string{"client-auth-token", "client_auth_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CONNECTSECURE_BASE_URL",
		// A template, not an origin: every tenant has its own pod host, which
		// the config or CONNECTSECURE_BASE_URL names, so with neither set the
		// kit allows the credential to no host. The generated config's default
		// (https://your-pod.myconnectsecure.com, a host on the vendor's own
		// domain) is refused before any request by connectsecureClientHook
		// (connectsecureBaseURLProblem), whatever source supplied it.
		Default: "https://{pod}.myconnectsecure.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes to the configured pod.
		{Source: "*", Allowed: []string{"base_url"}},
		// The JWT minted by /w/authorize or read back from the token cache
		// (internal/cli/connectsecure_auth.go holds it).
		{Source: "minted:connectsecure-jwt", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CONNECTSECURE_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the pod API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:expires_at", Reason: "the token cache's expiry time"},
		{Source: "config:tenant", Reason: "the portal login tenant name; it authenticates nothing without the client id and secret, and the Client-Auth-Token that carries it is held whole"},
		{Source: "config:user_id", Reason: "the ConnectSecure user id sent as X-USER-ID: an identifier vendor records carry (created_by, assigned users), never accepted without the JWT"},
		{Source: "config:AuthSource", Reason: "names where the auth came from (env, config, mint), not a credential"},
		{Source: "config:CredentialSource", Reason: "names the credential source, not a credential"},
		{Source: "config:configOwner", Reason: "names the file a setting came from"},
		{Source: "config:legacySourcePath", Reason: "path of the legacy config file"},
		{Source: "config:Path", Reason: "path of the config file"},
		{Source: "config:envOverrides", Reason: "names of the environment variables that override settings"},
		{Source: "config:Source", Reason: "a provenance label"},
		{Source: "config:Err", Reason: "a load error, not a credential"},
		{Source: "config:variant", Reason: "a learn-loop flag spelling"},
		{Source: "config:canonical", Reason: "a learn-loop flag spelling"},
		{Source: "config:stopwords", Reason: "learn-loop search words"},
		{Source: "config:synonyms", Reason: "learn-loop search words"},
		{Source: "env:CONNECTSECURE_TENANT", Reason: "the portal login tenant name; it authenticates nothing without the client id and secret, and the Client-Auth-Token that carries it is held whole"},
		{Source: "env:CONNECTSECURE_USER_ID", Reason: "the ConnectSecure user id sent as X-USER-ID: an identifier vendor records carry, never accepted without the JWT"},
		{Source: "env:CONNECTSECURE_BASE_URL", Reason: "the pod API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CONNECTSECURE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CONNECTSECURE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CONNECTSECURE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CONNECTSECURE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CONNECTSECURE_DATA_DIR", Reason: "directory path"},
		{Source: "env:CONNECTSECURE_STATE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTSECURE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTSECURE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CONNECTSECURE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CONNECTSECURE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CONNECTSECURE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:CONNECTSECURE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:CONNECTSECURE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CONNECTSECURE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CONNECTSECURE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CONNECTSECURE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CONNECTSECURE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_MCP_UPLOAD_ROOT", Reason: "directory the MCP upload tools may read from"},
		{Source: "env:CONNECTSECURE_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CONNECTSECURE_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:CODEX_THREAD_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CODEX_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:HTTP_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:HTTPS_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:ALL_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:http_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:https_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:all_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or CONNECTSECURE_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	holdClientAuthTokenFlag(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("CONNECTSECURE_CONFIG")
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

// holdClientAuthTokenFlag holds every --client-auth-token value on the command
// line (session sends it to /w/authorize as Client-Auth-Token). The flag is no
// CredentialFlags or StdinFlags name because its value "-" means "read the
// token from stdin": as a CredentialFlags name the kit would hold "-" itself,
// which is public text and refuses the start, and as a StdinFlags name it
// would read stdin to its end for every value, which blocks on a pipe that
// stays open. session holds the token it reads from stdin the moment it reads
// it (internal/cli/promoted_session.go).
func holdClientAuthTokenFlag(args []string) {
	for _, v := range credkit.FlagValues(args, "--client-auth-token") {
		if v != "-" {
			credkit.Hold("flag:--client-auth-token", v)
		}
	}
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
// run-profile store (a profile value saved under a credential flag's name) and
// every JWT token cache file (connectsecure-token-<hash>.json, whose
// access_token is a minted JWT). Each is a JSON document the kit reads by its
// declared names (credkit.HoldBody); a file that does not parse is held whole,
// every word of it. They are not returned to the kit's config reader, so none
// of them can name the allowed origin.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".connectsecure-cli", "profiles.json"))
	}
	if dir, err := cliutil.CacheDir(); err == nil {
		caches, _ := filepath.Glob(filepath.Join(dir, "connectsecure-token-*.json"))
		for _, c := range caches {
			holdFile(c)
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
}

// readRegular reads path only when it is a regular file, so a FIFO or a device
// named as a source cannot stall the start.
func readRegular(path string) ([]byte, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(path) // #nosec G304 -- a credential source this program reads anyway
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
