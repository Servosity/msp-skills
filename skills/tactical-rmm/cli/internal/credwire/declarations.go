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

	"tactical-rmm-pp-cli/internal/cliutil"
	"tactical-rmm-pp-cli/internal/config"
	"tactical-rmm-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Tactical RMM authenticates with one static API key, sent raw as the X-API-KEY
// header to the configured base URL (a self-hosted server, so there is no
// vendor default host). The key comes from TRMM_API_KEY, config.toml or the
// 0600 credentials.toml store (`auth set-token`). Nothing is minted.
//
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile), because the kit would withhold
// them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"TRMM_API_KEY",
		"TACTICAL_RMM_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"TACTICAL_RMM_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	// `auth set-token --stdin` reads the key; every generated write command
	// reads its JSON body with --stdin.
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"X-API-KEY"},
	// A user's password, the code-signing token and the MeshCentral token in a
	// request body (vendor serializers: accounts, core).
	CredentialFields: []string{"password", "token", "mesh_token"},
	MCPArguments:     []string{"password", "token", "mesh_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "TACTICAL_RMM_BASE_URL",
		Default:   "https://api.example.com",
	},
	Origins: []credkit.Origin{
		// Every API credential goes only to the configured Tactical RMM server.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:TACTICAL_RMM_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
		// A saved run profile's base-url is never used to send a request (no
		// command has a --base-url flag), so what it carries is sent nowhere.
		{Source: "profile:base-url", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "a generic OAuth client ID field Tactical RMM never uses; it names an app and authenticates nothing without client_secret"},
		{Source: "env:TACTICAL_RMM_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:TACTICAL_RMM_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:TACTICAL_RMM_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:TACTICAL_RMM_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:TACTICAL_RMM_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:TACTICAL_RMM_DATA_DIR", Reason: "directory path"},
		{Source: "env:TACTICAL_RMM_STATE_DIR", Reason: "directory path"},
		{Source: "env:TACTICAL_RMM_CACHE_DIR", Reason: "directory path"},
		{Source: "env:TACTICAL_RMM_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:TACTICAL_RMM_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:TACTICAL_RMM_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:TACTICAL_RMM_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:TACTICAL_RMM_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:TACTICAL_RMM_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:TACTICAL_RMM_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:TACTICAL_RMM_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:TACTICAL_RMM_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:TACTICAL_RMM_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:TACTICAL_RMM_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:TACTICAL_RMM_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:TACTICAL_RMM_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:TACTICAL_RMM_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:TACTICAL_RMM_USER_AGENT", Reason: "User-Agent string"},
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
// one: an explicit --config or TACTICAL_RMM_CONFIG wins, as it does in
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
		explicit = os.Getenv("TACTICAL_RMM_CONFIG")
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

// holdDirSources holds the run-profile store (current and legacy path): a
// profile value saved under a credential flag's name or under base-url is a
// credential source. Each is a JSON document the kit reads by its declared
// names (credkit.HoldBody); a file that does not parse is held whole, every
// word of it.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
		holdProfiles(filepath.Join(dir, "profiles.json")) // profile.go profileStorePath
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".tactical-rmm-cli", "profiles.json"))
		holdProfiles(filepath.Join(home, ".tactical-rmm-cli", "profiles.json")) // profile.go legacyProfileStorePath
	}
}

// holdProfiles holds the password and every query value of each saved profile
// value named base-url (any case, '-' or '_'), through the kit's own URL reader
// (credkit.HoldURL) under source profile:base-url, which Origins sends nowhere.
// The store is read member by member, so every duplicate of a key is held, not
// only the one encoding/json keeps.
func holdProfiles(path string) {
	data, err := readRegular(path)
	if err != nil {
		return
	}
	for _, top := range members(data) {
		if !strings.EqualFold(top.key, "profiles") {
			continue
		}
		for _, profile := range members(top.value) {
			for _, field := range members(profile.value) {
				if !strings.EqualFold(field.key, "values") {
					continue
				}
				for _, v := range members(field.value) {
					var s string
					if strings.EqualFold(strings.ReplaceAll(v.key, "_", "-"), "base-url") && json.Unmarshal(v.value, &s) == nil && s != "" {
						credkit.HoldURL("profile:base-url", s)
					}
				}
			}
		}
	}
}

type member struct {
	key   string
	value json.RawMessage
}

// members is every member of the JSON object b, in order, each duplicate key
// included, as far as b parses. Anything but an object has none.
func members(b []byte) []member {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out []member
	for dec.More() {
		k, err := dec.Token()
		key, ok := k.(string)
		var v json.RawMessage
		if err != nil || !ok || dec.Decode(&v) != nil {
			break
		}
		out = append(out, member{key, v})
	}
	return out
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
