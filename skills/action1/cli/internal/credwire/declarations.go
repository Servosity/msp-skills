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
	"slices"
	"strings"

	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/config"
	"action1-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Declarations is what the connector holds and where each credential may go.
// Action1 takes one bearer token: ACTION1_OAUTH2, or a token minted at
// <base URL>/oauth2/token from ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET
// (internal/client/oauth_cc.go holds it the moment it is minted), or the token /
// auth_header of config.toml or the 0600 credentials.toml store. /oauth2/token
// answers tokens and /2fa setup answers secret_key, so those response fields
// are declared. The store and config.toml are credential SOURCES: their own
// writes go outside the kit (cliutil.AtomicWriteCredentialFile, a declared
// exemption: the kit withholds them by contract) and both are named in Files.
// Every --stdin body is held (StdinFlags) except feedback's prose
// (compactStdinBody).
//
// The client ID is an identifier and only its unique part is held (holdClientID):
// Action1 issues it as api-key-<uuid>@action1.com, so holding it whole would
// withhold every line that names action1.com, the API host included, by the
// contract's 8-character piece rule.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"ACTION1_OAUTH2",
		"ACTION1_CLIENT_SECRET",
		"ACTION1_MCP_HTTP_TOKEN",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "oauth2", "access_token", "refresh_token", "client_secret"},
	StoredRecords:    []string{"auth_header", "oauth2", "access_token", "refresh_token", "client_secret"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"client_secret", "refresh_token", "access_token", "password", "secret_key", "mfa_code"},
	ResponseFields:   []string{"access_token", "refresh_token", "client_secret", "password", "secret_key", "mfa_code"},
	MCPArguments:     []string{"client_secret", "refresh_token", "access_token", "password", "secret_key", "mfa_code", "client-secret", "refresh-token", "access-token", "secret-key", "mfa-code"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "ACTION1_BASE_URL",
		Default:   "https://app.action1.com/api/3.0",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for:
		// the base URL's origin, nothing else (the token endpoint is on the API origin).
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:ACTION1_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Keys of config.toml and credentials.toml that are not credentials.
		{Source: "config:client_id", Reason: "the OAuth client ID, api-key-<uuid>@action1.com: it authenticates nothing without client_secret, and its unique part is held (holdClientID)"},
		{Source: "env:ACTION1_CLIENT_ID", Reason: "the OAuth client ID, api-key-<uuid>@action1.com: it authenticates nothing without client_secret, and its unique part is held (holdClientID)"},
		{Source: "config:tokenClientID", Reason: "an unexported ownership field no file loads: the client ID of the record that supplied the token"},
		{Source: "config:globalClientID", Reason: "an unexported ownership field no file loads: the client ID of the global store's record"},
		{Source: "env:ACTION1_ORG_ID", Reason: "default organization ID, a name the API returns in every listing"},
		{Source: "env:ACTION1_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:ACTION1_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:ACTION1_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:ACTION1_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:ACTION1_CACHE_DIR", Reason: "directory path"},
		{Source: "env:ACTION1_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:ACTION1_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:ACTION1_DATA_DIR", Reason: "directory path"},
		{Source: "env:ACTION1_STATE_DIR", Reason: "directory path"},
		{Source: "env:ACTION1_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:ACTION1_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:ACTION1_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:ACTION1_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		// Not in URLEnv: its origin cannot be declared, so a held query value would refuse the feedback POST itself.
		{Source: "env:ACTION1_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a held credential sent to it is refused by the outbound guard"},
		{Source: "env:ACTION1_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:ACTION1_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:ACTION1_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:ACTION1_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:ACTION1_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:ACTION1_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:ACTION1_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:ACTION1_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:ACTION1_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:ACTION1_USER_AGENT", Reason: "User-Agent string"},
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
// files the kit reads as this run's own, the config the program will really use
// FIRST, because the kit takes the base URL (and with it every credential's
// allowed origin) from the first file naming one: an explicit --config or
// ACTION1_CONFIG wins, as it does in config.Load, and the last --home decides
// the default paths, as pflag does. Every other file any --config or --home could
// mean is only held (cliutil.HoldFile): its values cannot print, and its base_url
// cannot become an allowed origin. No flag arity is guessed: credkit.FlagValues
// counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	holdClientID(cliutil.EnvOverride("ACTION1_CLIENT_ID"))
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("ACTION1_CONFIG")
	}
	var used, held []string
	if len(homes) == 0 {
		used = runFiles(explicit)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		held = append(held, runFiles("")...)
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			held = append(held, files...)
		}
	}
	held = append(held, runFiles("")...)
	for _, cfg := range configs {
		held = append(held, runFiles(cfg)...)
	}
	for _, f := range append(append(append([]string(nil), used...), held...), configs...) {
		holdFileClientID(f)
	}
	// Held outside Files, whose first base_url would become the allowed origin. A
	// file the kit reads itself (Files and every --config) is left to its reader.
	for _, f := range held {
		if !slices.Contains(used, f) && !slices.Contains(configs, f) {
			cliutil.HoldFile(f)
		}
	}
	return used
}

// holdClientID holds what is unique in a client ID. Action1's public text,
// api-key- and @action1.com, is no secret and sits in the API host's name.
func holdClientID(id string) {
	id = strings.TrimSpace(id)
	if at := strings.LastIndex(id, "@"); at >= 0 {
		id = id[:at]
	}
	if id = strings.TrimPrefix(id, "api-key-"); id != "" {
		cliutil.Hold("minted:client_id", id)
	}
}

// holdFileClientID is holdClientID for the client_id of a config or store file.
// A file that does not parse was held whole by the kit's token reader.
func holdFileClientID(path string) {
	data, err := os.ReadFile(filepath.Clean(path)) // the operator's own config or store, read to hold its values
	if err != nil {
		return
	}
	var parsed struct {
		ClientID string `toml:"client_id"`
	}
	if toml.Unmarshal(data, &parsed) == nil {
		holdClientID(parsed.ClientID)
	}
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, the legacy config when no explicit one
// was named and the default is absent, then the credential stores (the one beside
// an explicit config, and the default one).
func runFiles(explicit string) []string {
	var files []string
	if strings.TrimSpace(explicit) != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
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
		files = append(files, store)
	}
	// The run profile store is a source too (credential leak contract section 1:
	// a value under a credential flag's name or under base-url), read here and not
	// in the returned list: it is nested JSON, which the kit's file reader answers
	// by holding every string in it, so a profile named "p" or a saved --limit 7
	// would refuse every later start. HoldFile reads it with the kit's HoldBody,
	// which holds the declared credential names at any depth.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldFile(filepath.Join(home, ".action1-cli", "profiles.json"))
	}
	return files
}

// compactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came (google-workspace
// precedent).
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
