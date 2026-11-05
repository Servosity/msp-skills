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

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/config"
	"apple-business-pp-cli/internal/credkit"
)

// appleAccountOrigin serves Apple's OAuth token endpoint.
const appleAccountOrigin = "https://account.apple.com"

// credentialFields are the form and JSON request fields that carry a credential.
var credentialFields = []string{
	"client_assertion", "client_secret", "access_token", "refresh_token", "id_token", "password", "passcode", "pin", "secret", "token",
}

// Declarations is what the connector holds and where each credential may go.
// Apple Business signs a short-lived ES256 assertion with the API account's
// private key, posts it to the token endpoint and sends the minted bearer to the
// API, from the environment, config.toml or the 0600 credential store. The store
// and config.toml are credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile) and they are named in Files, because the kit
// would withhold them by contract.
//
// APPLE_BUSINESS_PRIVATE_KEY, config business_private_key, config client_secret
// and --client-secret hold PEM text or a path to the .pem file, so they are not
// SecretEnv, ConfigFields or CredentialFlags, which hold the value as written.
// credentialFiles holds what each one really is (cliutil.HoldPrivateKeyRef): the
// key's body, its decoded forms and the contents of the file it names, under
// minted:private-key, which may be sent nowhere. Held as written, 8 characters of
// a path (the home directory) would withhold every printed path, and the PEM armor
// would withhold all text saying "private key".
//
// The Client ID and Key ID name the API account: auth status prints them, the
// store is scoped by the Client ID, and neither proves anything without the key.
//
// No Apple Business response in the vendor's spec carries a credential. These
// ResponseFields are the names a token endpoint answers with and the names of a
// credential anywhere else, matched exactly (an audit event's eventData* attributes
// and keyId are descriptions, not credentials).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"APPLE_BUSINESS_ACCESS_TOKEN",
		"APPLE_BUSINESS_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"APPLE_BUSINESS_TOKEN_URL",
		"APPLE_BUSINESS_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "access_token", "refresh_token",
	},
	StoredRecords: []string{
		"auth_header", "access_token", "refresh_token",
	},
	CredentialFields: credentialFields,
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret", "client_assertion",
		"password", "passwd", "pwd", "passcode", "passphrase", "pin", "otp", "secret", "token",
		"private_key", "privateKey", "api_key", "apiKey", "credential", "credentials",
	},
	MCPArguments: []string{
		"client-secret", "client_secret", "client_assertion", "access_token", "refresh_token", "id_token", "password", "passcode", "pin", "secret", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "APPLE_BUSINESS_BASE_URL",
		Default:   "https://api-business.apple.com/v1",
	},
	Origins: []credkit.Origin{
		// A bearer goes only to the API base the operator configured.
		{Source: "*", Allowed: []string{"base_url"}},
		// The signed assertion and a refresh token go to the token endpoint:
		// Apple's, or one on the configured API origin. A token URL on any third
		// origin is refused. The connector's own pin (apple-business-host-pin)
		// narrows every request, redirects included, to the resolved base and
		// token URLs.
		{Source: "minted:client-assertion", Allowed: []string{appleAccountOrigin, "base_url"}},
		{Source: "field:client_assertion", Allowed: []string{appleAccountOrigin, "base_url"}},
		{Source: "config:refresh_token", Allowed: []string{appleAccountOrigin, "base_url"}},
		{Source: "field:refresh_token", Allowed: []string{appleAccountOrigin, "base_url"}},
		{Source: "response:refresh_token", Allowed: []string{appleAccountOrigin, "base_url"}},
		// The private key signs and is never sent.
		{Source: cliutil.PrivateKeySource, Allowed: nil},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:APPLE_BUSINESS_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials and
		// cliutil.PrivateKeyFields, which the per-read hold treats the same way
		// (TestIdentifiersMatchTheHold).
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a third origin is refused by Origins"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "the API account's Client ID, its public name"},
		{Source: "config:business_client_id", Reason: "the API account's Client ID, its public name"},
		{Source: "config:business_key_id", Reason: "the Key ID, the public name of the account's key (the JWT kid)"},
		{Source: "config:client_secret", Reason: "PEM text or a key file path; cliutil.HoldPrivateKeyRef holds the key under minted:private-key"},
		{Source: "config:business_private_key", Reason: "PEM text or a key file path; cliutil.HoldPrivateKeyRef holds the key under minted:private-key"},
		{Source: "env:APPLE_BUSINESS_PRIVATE_KEY", Reason: "PEM text or a key file path; cliutil.HoldPrivateKeyRef holds the key under minted:private-key"},
		{Source: "env:APPLE_BUSINESS_CLIENT_ID", Reason: "the API account's Client ID, its public name"},
		{Source: "env:APPLE_BUSINESS_KEY_ID", Reason: "the Key ID, the public name of the account's key (the JWT kid)"},
		{Source: "env:APPLE_BUSINESS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:APPLE_BUSINESS_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:APPLE_BUSINESS_OAUTH_SCOPE", Reason: "OAuth scope name"},
		{Source: "env:APPLE_BUSINESS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:APPLE_BUSINESS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:APPLE_BUSINESS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:APPLE_BUSINESS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:APPLE_BUSINESS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:APPLE_BUSINESS_DATA_DIR", Reason: "directory path"},
		{Source: "env:APPLE_BUSINESS_STATE_DIR", Reason: "directory path"},
		{Source: "env:APPLE_BUSINESS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:APPLE_BUSINESS_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:APPLE_BUSINESS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:APPLE_BUSINESS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:APPLE_BUSINESS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:APPLE_BUSINESS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:APPLE_BUSINESS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:APPLE_BUSINESS_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:APPLE_BUSINESS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:APPLE_BUSINESS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:APPLE_BUSINESS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:APPLE_BUSINESS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:APPLE_BUSINESS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:APPLE_BUSINESS_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:APPLE_BUSINESS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It holds what the
// private-key settings really are and every value of a file the program consults,
// holds what `auth set-token` will read, and names every file a credential may be
// read from under any --config and --home the command line can mean: holding a
// file the run does not load is safe, missing the one it loads is not.
func credentialFiles(args []string) []string {
	cliutil.HoldPrivateKeyRef(cliutil.EnvOverride("APPLE_BUSINESS_PRIVATE_KEY"))
	for _, v := range credkit.FlagValues(args, "--client-secret") {
		cliutil.HoldPrivateKeyRef(v)
	}
	holdPipedToken(args)
	holdStdinBody(args)
	holdArgBodies(args)
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
// line by line, and gives the program the same bytes on a new os.Stdin. The word is
// matched anywhere on the line, which holds more, never less.
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

// holdStdinBody holds the credential fields of a --stdin request body before anything
// prints, previews and --dry-run included, and gives the program the same bytes on a
// new os.Stdin. It is not StdinFlags: those hold every line of a body, so a pretty-printed
// body stops the program (a line that is only "{" collides with public text, exit 78 with
// no message) and a one-line body withholds its own preview. A body is a request, not a
// credential; its credential fields are. One that is not JSON is refused by the command,
// whose error may repeat it, so its lines are held instead. feedback --stdin reads a
// note, not a body; only the first argument names it, since any later one may be a
// value. With a global flag before feedback the note is held, which withholds its
// preview and leaks nothing.
func holdStdinBody(args []string) {
	if (len(args) > 0 && args[0] == "feedback") || !hasStdinFlag(args) {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		b = nil // a partial read is no body: the command refuses an empty one
	}
	if !holdBodyFields(b) {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Trim(line, " \t\r{}[],:") != "" {
				credkit.Hold("stdin", line)
			}
		}
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

// holdArgBodies holds the credential fields of a JSON document on the command line,
// from the bytes the operator gave: a parser later keeps only the last member of a
// duplicate key, and the first one is a credential too. What is not JSON holds nothing.
func holdArgBodies(args []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			_, v, ok := strings.Cut(a, "=")
			if !ok {
				continue
			}
			a = v
		}
		holdBodyFields([]byte(a))
	}
}

func holdStringBodies(strs []string) {
	for _, s := range strs {
		holdBodyFields([]byte(s))
	}
}

// hasStdinFlag is a plain loop, not slices.ContainsFunc: check_mcp_file_flags ties the
// kit's FlagValues to its flag only while every read of argv is one it can follow.
func hasStdinFlag(args []string) bool {
	for _, a := range args {
		if a == "--stdin" || strings.HasPrefix(a, "--stdin=") {
			return true
		}
	}
	return false
}

// HoldRequest holds the credential fields of one request, before a preview or an error
// can print them: a JSON body however it was built (a body flag, --stdin, import) and
// its query parameters. The kit's Transport holds them too, but only once a request is
// on its way, and a dry run never gets there.
func HoldRequest(params map[string]string, body []byte) {
	for name, value := range params {
		if value != "" && isCredentialField(name) {
			credkit.Hold("stdin", value)
		}
	}
	holdBodyFields(body)
}

func isCredentialField(name string) bool {
	return slices.ContainsFunc(credentialFields, func(f string) bool { return strings.EqualFold(f, name) })
}

// holdBodyFields holds every scalar under a credential field of one JSON document, every
// member of a duplicate key included, and reports whether b was one.
func holdBodyFields(b []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if holdJSONValue(dec, false) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

func holdJSONValue(dec *json.Decoder, held bool) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		for dec.More() {
			under := held
			if t == '{' {
				key, err := dec.Token()
				if err != nil {
					return err
				}
				under = held || isCredentialField(key.(string))
			}
			if err := holdJSONValue(dec, under); err != nil {
				return err
			}
		}
		_, err = dec.Token() // the closing delimiter
		return err
	case string:
		if held {
			credkit.Hold("stdin", t)
		}
	case json.Number:
		if held {
			credkit.Hold("stdin", t.String())
		}
	}
	return nil
}

// pathsUnder is every config and credential file Load may read under each --home
// and --config the command line can mean. The default and legacy configs are only
// consulted under an explicit selection (the Apple account's provenance rules read
// them), so their values are held and they stay out of the list, or the first
// base_url found would become the base URL the kit allows credentials to. Each
// run profile store is held the same way.
func pathsUnder(args []string) []string {
	homes := append(credkit.FlagValues(args, "--home"), "")
	configs := append(credkit.FlagValues(args, "--config"), "")
	var out []string
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if consulted, err := config.ConsultedPaths(); err == nil {
			for _, p := range consulted {
				if opened, err := cliutil.ReadOpenedConfig(p); err == nil {
					cliutil.HoldConfigFile(opened.Data)
				}
			}
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdStringBodies(cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json")))
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
	if home, err := os.UserHomeDir(); err == nil {
		holdStringBodies(cliutil.HoldProfileStoreFile(filepath.Join(home, ".apple-business-cli", "profiles.json")))
	}
	return out
}
