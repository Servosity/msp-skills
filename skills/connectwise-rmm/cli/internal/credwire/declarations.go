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

	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/config"
	"connectwise-rmm-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// ConnectWise RMM (Asio) takes one bearer token, minted from a client ID and
// secret by a JSON POST to <regional host>/v1/token (client.mintClientCredentials,
// `auth login`, and `scopes`, which mints one token per scope; each holds its
// token the moment it exists), or the access_token / auth_header of config.toml
// or the 0600 credentials.toml store. The token endpoint answers access_token,
// so that response field is declared.
//
// The client ID is held, not an identifier: the manifest marks it sensitive,
// it is half of the credential pair, and nothing public shares an 8-character
// piece with it (its region prefix is 4 characters, the API hosts carry none of
// it), so holding it withholds no API host or ordinary output
// (TestClientIDDoesNotWithholdTheAPIHost).
//
// Origins: every credential goes to the configured base URL's origin. Asio
// runs three regional hosts and an EU or AU client ID moves an unconfigured run
// to its own host (config.applyAsioRegion), so each regional host has its own
// minted source, and config.HoldForAsioRegion binds the run's credentials to
// the one host the config really resolves for the API and the token URL.
//
// config.toml and credentials.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile, a declared exemption:
// the kit withholds them by contract). Every --stdin body is held
// (StdinFlags) except feedback's prose (compactStdinBody).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CONNECTWISE_RMM_CLIENT_ID",
		"CONNECTWISE_RMM_CLIENT_SECRET",
		"CONNECTWISE_RMM_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"CONNECTWISE_RMM_TOKEN_URL",
		"SPACEFAST_API_URL",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "rmm_client_id", "rmm_client_secret"},
	StoredRecords:    []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "rmm_client_id", "rmm_client_secret"},
	CredentialFlags:  []string{"--client-secret"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"client_secret", "refresh_token", "access_token", "password"},
	ResponseFields:   []string{"access_token", "refresh_token", "id_token", "client_secret"},
	MCPArguments:     []string{"client_secret", "refresh_token", "access_token", "password", "client-secret", "refresh-token", "access-token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CONNECTWISE_RMM_BASE_URL",
		Default:   "https://openapi.service.itsupport247.net",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for:
		// the base URL's origin (the default token URL is on that origin).
		{Source: "*", Allowed: []string{"base_url"}},
		// The regional host the config resolves (config.HoldForAsioRegion binds
		// a run's own credentials to exactly one of these, never all three).
		{Source: "minted:asio-region-na", Allowed: []string{"https://openapi.service.itsupport247.net"}},
		{Source: "minted:asio-region-eu", Allowed: []string{"https://openapi.service.euplatform.connectwise.com"}},
		{Source: "minted:asio-region-au", Allowed: []string{"https://openapi.service.auplatform.connectwise.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CONNECTWISE_RMM_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		// Keys of config.toml that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:CONNECTWISE_RMM_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CONNECTWISE_RMM_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:CONNECTWISE_RMM_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CONNECTWISE_RMM_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CONNECTWISE_RMM_OAUTH_SCOPE", Reason: "space-separated OAuth scope names, which the CLI prints itself"},
		{Source: "env:CONNECTWISE_RMM_CACHE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_RMM_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CONNECTWISE_RMM_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_RMM_DATA_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_RMM_STATE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_RMM_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CONNECTWISE_RMM_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CONNECTWISE_RMM_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CONNECTWISE_RMM_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		// Not in URLEnv: its origin cannot be declared, so a held query value would refuse the feedback POST itself.
		{Source: "env:CONNECTWISE_RMM_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a held credential sent to it is refused by the outbound guard"},
		{Source: "env:CONNECTWISE_RMM_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_RMM_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:CONNECTWISE_RMM_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CONNECTWISE_RMM_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CONNECTWISE_RMM_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CONNECTWISE_RMM_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CONNECTWISE_RMM_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_RMM_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_RMM_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_RMM_USER_AGENT", Reason: "User-Agent string"},
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
// files the kit reads as this run's own, the config the program will really use
// FIRST, because the kit takes the base URL (and with it every credential's
// allowed origin) from the first file naming one: an explicit --config or
// CONNECTWISE_RMM_CONFIG wins, as it does in config.Load, and the last --home
// decides the default paths, as pflag does. Every other file any --config or
// --home could mean is only held (cliutil.HoldFile): its values cannot print,
// and its base_url cannot become an allowed origin. No flag arity is guessed:
// credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("CONNECTWISE_RMM_CONFIG")
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
	// Held outside Files, whose first base_url would become the allowed origin. A
	// file the kit reads itself (Files and every --config) is left to its reader.
	for _, f := range held {
		if !slices.Contains(used, f) && !slices.Contains(configs, f) {
			cliutil.HoldFile(f)
		}
	}
	return used
}

// runFiles is what a run with this explicit config (or none) reads, in the
// order config.Load prefers it: the config file, then the legacy config when no
// explicit one was named and the default is absent.
//
// The credential stores (the one beside an explicit config, and the default
// one) are held here, not returned: each records the base_url its credential
// was issued for, which config.Load compares but never uses as the base URL,
// so it must not become the kit's allowed origin (google-workspace precedent).
// The run profile store is held the same way: it is nested JSON, which the
// kit's file reader answers by holding every string in it, so a profile named
// "p" would refuse every later start. HoldFile reads both with the kit's
// HoldBody, which holds the declared credential names at any depth.
func runFiles(explicit string) []string {
	var files []string
	if strings.TrimSpace(explicit) != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			cliutil.HoldFile(store)
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
		cliutil.HoldFile(store)
	}
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldFile(filepath.Join(home, ".connectwise-rmm-cli", "profiles.json"))
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
