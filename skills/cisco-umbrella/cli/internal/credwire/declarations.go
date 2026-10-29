// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"io"
	"os"
	"slices"
	"strings"

	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/config"
	"cisco-umbrella-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Umbrella authenticates with an API key and secret (Basic auth at the token
// URL, a minted bearer everywhere else), from the environment, config.toml or
// the 0600 credential store. The store and config.toml are credential SOURCES:
// their own writes go outside the kit (cliutil.AtomicWriteCredentialFile) and
// they are named in Files, because the kit would withhold them by contract.
//
// ResponseFields, MCPArguments, CredentialFields and CredentialFlags are the
// string-valued credential names in the Umbrella spec and the CLI's flags
// (TestDeclaredCredentialFlagsMatchTheCommands and
// TestDeclaredFieldsMatchTheSpec keep them complete). One name is left out of
// ResponseFields on purpose: key. The spec's only "key" is the object wrapping a
// whole API key record (KeyResponse {key: {id, name, clientId, ...}}), and the
// kit holds every scalar inside a declared key, so an API key's name, id and
// dates would be held and every listing naming them withheld. The shared walker
// (cliutil.HoldCredentialValues) holds a string-valued key and walks an
// object-valued one by name. Ordinary identifiers (an API key's id, keyId,
// creatorKeyId, an org ID) are not held.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CISCO_UMBRELLA_API_KEY",
		"CISCO_UMBRELLA_API_SECRET",
		"CISCO_UMBRELLA_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	// The keys of config.toml and the credential store that hold a credential;
	// cliutil.CredentialKeys is the per-read hold's copy (TestConfigKeyListsAgree).
	ConfigFields: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token", "umbrella_api_key", "umbrella_api_secret",
	},
	StoredRecords: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token", "umbrella_api_key", "umbrella_api_secret",
	},
	CredentialFlags: []string{
		"--authentication-parameters-secret", "--client-id", "--client-secret", "--password", "--psk-secret", "--token",
	},
	// What the operator's own requests and tool arguments hold: ResponseFields and
	// key, since no request carries the wrapper object.
	CredentialFields: []string{
		"accessKeyId", "clientId", "clientSecret", "currentKeyId", "oldKeyId", "password", "psk", "secret", "secretAccessKey", "secretKey", "token",
		"access_token", "client_id", "client_secret", "refresh_token", "key",
	},
	// The string-valued credential members of the Umbrella API (an API key's clientId
	// and clientSecret, the Cisco-managed S3 bucket key's three members, a tunnel's
	// psk and secret, a user's password, a logo's token, which the spec calls a
	// random number but names a token) and of the OAuth exchange. secretKey and
	// accessKeyId are the same pair under names other Cisco endpoints use. A declared
	// name matches exactly; the walker catches other spellings.
	ResponseFields: []string{
		"accessKeyId", "clientId", "clientSecret", "currentKeyId", "oldKeyId", "password", "psk", "secret", "secretAccessKey", "secretKey", "token",
		"access_token", "client_id", "client_secret", "refresh_token",
	},
	MCPArguments: []string{
		"accessKeyId", "clientId", "clientSecret", "currentKeyId", "oldKeyId", "password", "psk", "secret", "secretAccessKey", "secretKey", "token",
		"access_token", "client_id", "client_secret", "refresh_token", "key",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CISCO_UMBRELLA_BASE_URL",
		Default:   "https://api.umbrella.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API base the operator configured or to
		// Cisco's own host, which serves the API and the default token URL. A
		// token URL on any third origin is refused: the Basic header carries the
		// key and the secret. The connector's own origin pin (api-origin-pinned)
		// narrows every request, redirects included, to the resolved base and
		// token URLs.
		{Source: "*", Allowed: []string{"base_url", "https://api.umbrella.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CISCO_UMBRELLA_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a third origin is refused by Origins"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:CISCO_UMBRELLA_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CISCO_UMBRELLA_TOKEN_URL", Reason: "the token endpoint URL; a third origin is refused by Origins"},
		{Source: "env:CISCO_UMBRELLA_OAUTH_SCOPE", Reason: "OAuth scope name"},
		{Source: "env:CISCO_UMBRELLA_ORG_ID", Reason: "a child organization ID, an ordinary number in every Umbrella listing"},
		{Source: "env:CISCO_UMBRELLA_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CISCO_UMBRELLA_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CISCO_UMBRELLA_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CISCO_UMBRELLA_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CISCO_UMBRELLA_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CISCO_UMBRELLA_DATA_DIR", Reason: "directory path"},
		{Source: "env:CISCO_UMBRELLA_STATE_DIR", Reason: "directory path"},
		{Source: "env:CISCO_UMBRELLA_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:CISCO_UMBRELLA_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CISCO_UMBRELLA_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CISCO_UMBRELLA_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CISCO_UMBRELLA_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CISCO_UMBRELLA_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CISCO_UMBRELLA_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CISCO_UMBRELLA_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CISCO_UMBRELLA_MCP_UPLOAD_ROOT", Reason: "directory MCP uploads are read from"},
		{Source: "env:CISCO_UMBRELLA_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CISCO_UMBRELLA_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:CISCO_UMBRELLA_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CISCO_UMBRELLA_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CISCO_UMBRELLA_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CISCO_UMBRELLA_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:CISCO_UMBRELLA_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It names every file a credential may be read
// from under any --config and --home the command line can mean: holding a file
// the run does not load is safe, missing the one it loads is not.
func credentialFiles(args []string) []string {
	holdPipedToken(args)
	return pathsUnder(args)
}

// stdinHoldLimit is how much of a piped token is read ahead: past the 64 KiB
// `auth set-token` accepts, so the command still sees an oversized pipe and refuses
// it, and bounded, so a pipe that never ends cannot fill memory first.
const stdinHoldLimit = 128 << 10

// holdPipedToken holds what `auth set-token` will read before anything can print
// (contract section 1, stdin included): the parser reads the command line before the
// command reads the pipe, and its errors repeat what it was given. It does for this
// subcommand what the kit's StdinFlags do for a dashed flag: holds the pipe whole and
// line by line, and gives the program the same bytes on a new os.Stdin. Not --stdin:
// those bodies are JSON requests, whose lines would withhold every later echo of the
// record. Blank input is the command's own refusal, not a value. The word is matched
// anywhere on the line, which holds more, never less. A pipe that stays open with
// nothing written waits here, as the command itself would.
func holdPipedToken(args []string) {
	if !slices.Contains(args, "set-token") {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return // a terminal: the command refuses it and reads nothing
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, stdinHoldLimit))
	for _, v := range append(strings.Split(string(b), "\n"), string(b)) {
		if strings.TrimSpace(v) != "" {
			credkit.Hold("stdin", v)
		}
	}
	if err != nil {
		b = nil // a partial read is no token: the command refuses an empty one
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent: the command reads nothing and refuses
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	os.Stdin = r
}

func pathsUnder(args []string) []string {
	homes := append(credkit.FlagValues(args, "--home"), "")
	configs := append(credkit.FlagValues(args, "--config"), "")
	var out []string
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		for _, c := range configs {
			files, err := config.ReadPaths(c)
			if err != nil {
				continue
			}
			for _, f := range files {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		restore()
	}
	return out
}
