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

	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/config"
	"sophos-central-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Sophos Central authenticates with an API credential's client ID and secret
// (form fields on the client_credentials grant at the token URL, by default
// https://id.sophos.com/api/v2/oauth2/token) and a minted bearer on every API
// call. The pair comes from SOPHOS_CENTRAL_CLIENT_ID /
// SOPHOS_CENTRAL_CLIENT_SECRET, `auth login --client-id --client-secret`,
// config.toml or the 0600 credentials.toml store; a bearer can also be saved
// with `auth set-token` (stdin).
//
// Where the bearer goes: the global API (https://api.central.sophos.com, the
// default base URL) and the tenant's regional apiHost
// (https://api-<region>.central.sophos.com), which the program learns at run
// time from GET /whoami/v1 and the partner or organization tenant lookup. The
// kit's Origins are fixed data and cannot name a host learned at run time, so
// the Sophos Central API domain is named instead; the connector's own pin
// (internal/cli/sophos_routing.go isSophosAPIHost, handfixes.json
// sophos-whoami-apihost-routing) narrows every routed request to
// api.central.sophos.com or an api-*.central.sophos.com host the vendor
// returned, and a base URL that is not a Sophos host is never rewritten.
//
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract. So is the Spacefast delivery record, which keeps
// an anonymous page key.
//
// Vendor response credentials (handfixes.json
// secret-responses-never-persisted): an endpoint's tamper protection password
// (password, and each previousPasswords[].password beneath it) and a
// migration job's token. The connector already drops them from the store,
// the response cache, export and tail; the kit holds them as the response
// streams, so any other sink that would carry one is withheld.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"SOPHOS_CENTRAL_CLIENT_ID",
		"SOPHOS_CENTRAL_CLIENT_SECRET",
		"SOPHOS_CENTRAL_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"SOPHOS_CENTRAL_TOKEN_URL",
		"SOPHOS_CENTRAL_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token",
		"central_client_id", "central_client_secret",
	},
	StoredRecords: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token",
		"central_client_id", "central_client_secret",
	},
	// auth login's pair; the archive password of endpoints
	// collect-forensic-log / collect-memory-dump; the migration job token
	// migrations create-sending-job sends. Every other flag of the command tree
	// is a name, an ID, a path, a reference or a filter.
	CredentialFlags: []string{"--client-id", "--client-secret", "--password", "--token"},
	// No StdinFlags: no flag here reads a credential from stdin. --stdin reads
	// a request body, whose credential fields takeStdinBody holds.
	// The token endpoint's form fields (the client pair; the refresh grant also
	// sends the refresh token), and the request body fields named above.
	CredentialFields: []string{"client_id", "client_secret", "refresh_token", "password", "token"},
	// The token response's tokens; the tamper protection password; the
	// migration job token; a client secret a response could carry.
	ResponseFields: []string{"access_token", "refresh_token", "password", "token", "clientSecret", "client_secret"},
	// Both spellings: an MCP argument is named as its flag (client-secret),
	// and a run profile (profiles.json, read by HoldBody) keys a value by the
	// flag's name.
	MCPArguments: []string{
		"client-id", "client-secret", "client_id", "client_secret", "access_token", "refresh_token",
		"password", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SOPHOS_CENTRAL_BASE_URL",
		Default:   "https://api.central.sophos.com",
	},
	Origins: []credkit.Origin{
		// Every API credential goes to the configured base URL and to a Sophos
		// Central API host: the global host and the regional apiHosts the
		// vendor returns, all under central.sophos.com (see above), and to
		// the host that serves the default token URL.
		{Source: "*", Allowed: []string{"base_url", "https://*.central.sophos.com", "https://id.sophos.com"}},
		// The client secret and the refresh token are only ever posted to the
		// token endpoint: the host of the default token URL, or the base URL's
		// origin (a token URL on the API host). Never a regional API host.
		{Source: "env:SOPHOS_CENTRAL_CLIENT_SECRET", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "flag:--client-secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "config:client_secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "config:central_client_secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "config:refresh_token", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "field:client_secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "field:client-secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "field:refresh_token", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "mcp:client-secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "mcp:client_secret", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "mcp:refresh_token", Allowed: []string{"base_url", "https://id.sophos.com"}},
		{Source: "minted:refresh_token", Allowed: []string{"base_url", "https://id.sophos.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:SOPHOS_CENTRAL_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a third origin is refused by Origins"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:refresh_login_id", Reason: "a SHA-256 hash that names a stored login, never the token it hashes"},
		{Source: "env:SOPHOS_CENTRAL_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:SOPHOS_CENTRAL_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv, and a third origin is refused by Origins"},
		{Source: "env:SOPHOS_CENTRAL_OAUTH_SCOPE", Reason: "OAuth scope name (token by default)"},
		{Source: "env:SOPHOS_CENTRAL_TENANT_ID", Reason: "a Sophos tenant ID, sent as the X-Tenant-ID header; it names a tenant and authenticates nothing"},
		{Source: "env:SOPHOS_CENTRAL_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SOPHOS_CENTRAL_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SOPHOS_CENTRAL_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SOPHOS_CENTRAL_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:SOPHOS_CENTRAL_DATA_DIR", Reason: "directory path"},
		{Source: "env:SOPHOS_CENTRAL_STATE_DIR", Reason: "directory path"},
		{Source: "env:SOPHOS_CENTRAL_CACHE_DIR", Reason: "directory path"},
		{Source: "env:SOPHOS_CENTRAL_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:SOPHOS_CENTRAL_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SOPHOS_CENTRAL_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SOPHOS_CENTRAL_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:SOPHOS_CENTRAL_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:SOPHOS_CENTRAL_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SOPHOS_CENTRAL_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SOPHOS_CENTRAL_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:SOPHOS_CENTRAL_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:SOPHOS_CENTRAL_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SOPHOS_CENTRAL_USER_AGENT", Reason: "User-Agent string"},
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
// one: an explicit --config or SOPHOS_CENTRAL_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	holdPipedToken(args)
	takeStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("SOPHOS_CENTRAL_CONFIG")
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

// startupNamedConfigOrHome records that the command line named a --config
// or --home, which Start read. Set once, inside Start, before main goes on.
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

// holdDirSources holds the run-profile store (profiles.json, and its legacy
// copy under ~/.sophos-central-cli), a credential source that is not a config
// file: a profile value saved under a credential flag's name is held by
// HoldBody (every member of a duplicate key included), and the userinfo and
// query values of one saved under base-url by HoldURL. A store that does not
// parse is held whole, every word of it.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdProfiles(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfiles(filepath.Join(home, ".sophos-central-cli", "profiles.json"))
	}
}

func holdProfiles(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
	holdProfileBaseURLs(data)
}

// holdProfileBaseURLs passes every string value of a base-url member, at any
// depth and every duplicate included, to credkit.HoldURL. It walks the JSON
// tokens rather than decoding into a map, which would keep only the last of
// two duplicate keys.
func holdProfileBaseURLs(data []byte) {
	type frame struct{ object, wantKey bool }
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var stack []frame
	key := ""
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return // the end, or a store that does not parse (HoldBody held it whole)
		}
		top := len(stack) - 1
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				stack = append(stack, frame{object: true, wantKey: true})
			case '[':
				stack = append(stack, frame{})
			default:
				stack = stack[:top]
				valueDone()
			}
		case string:
			if top >= 0 && stack[top].object && stack[top].wantKey {
				key = v
				stack[top].wantKey = false
				continue
			}
			if key == "base-url" {
				credkit.HoldURL("config:base_url", v)
			}
			valueDone()
		default:
			valueDone()
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
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(filepath.Clean(path)) // #nosec G304 -- a credential source this program reads anyway
}

// stdinHoldLimit is how much of a piped token is read ahead: past the size
// `auth set-token` accepts, so the command still sees an oversized pipe and
// refuses it, and bounded, so a pipe that never ends cannot fill memory first.
const stdinHoldLimit = 128 << 10

// holdPipedToken holds what `auth set-token` will read before anything can print
// (contract section 1, stdin included): the parser reads the command line before the
// command reads the pipe, and its errors repeat what it was given. It does for this
// subcommand what the kit's StdinFlags do for a dashed flag: holds the pipe whole and
// line by line, and gives the program the same bytes on a new os.Stdin. Blank input
// is the command's own refusal, not a value. The word is matched anywhere on the
// line, which holds more, never less. A pipe that stays open with nothing written
// waits here, as the command itself would.
func holdPipedToken(args []string) {
	if !slices.Contains(args, "set-token") {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return // a terminal: the command reads it itself and holds what it read
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
	if r := pipeOf(b); r != nil {
		os.Stdin = r
	} // else os.Stdin is spent: the command reads nothing and refuses
}

// takeStdinBody takes a --stdin body off stdin inside Start, before anything
// prints, and holds the credential fields in it; RestoreStdinBody gives the
// program the same bytes back after Start.
//
// --stdin is no credential source: on every command but feedback it reads a
// JSON request body, and Sophos's answers repeat the body, so holding it whole
// (as Declarations.StdinFlags would) withheld the answer of every write and
// read made with one and failed the learn store migration. What is held is the
// body's credential fields (credkit.HoldBody: the declared names, password and
// token among them, and authorization, auth_header, proxy-authorization and
// cookie, at any depth, every duplicate), on every command that takes one,
// previews and --dry-run included. A body that is no document is held whole.
//
// `feedback --stdin` is the exception: its body is free text for a person to
// read, no credential and no document, and HoldBody would hold every word of
// it, withholding the command's own record or refusing the start on a line
// such as "OK". It is not held. Only a command line that starts with
// "feedback" qualifies: free text with a flag in front is held as bytes that
// are no document are, every word, which refuses the start but never prints a
// secret.
//
// A terminal is not read, as the kit reads none. When stdin fails part way,
// the part read is what the program gets, held as any body is: the rest is
// never read.
func takeStdinBody(args []string) {
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
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	stdinBody = append([]byte{}, data...)
	if !(len(args) > 0 && args[0] == "feedback") {
		credkit.HoldBody(stdinBody)
	}
}

// stdinBody is the --stdin body takeStdinBody took off stdin; nil when none was.
var stdinBody []byte

// RestoreStdinBody gives a --stdin command its body back on os.Stdin. The CLI
// main calls it right after credkit.Start; without a body it does nothing.
// When no pipe can be made, os.Stdin is nil, so the command's read fails and
// sends nothing.
func RestoreStdinBody() {
	if stdinBody == nil {
		return
	}
	os.Stdin = pipeOf(stdinBody)
}

// pipeOf is a pipe that yields b, or nil when no pipe can be made.
func pipeOf(b []byte) *os.File {
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	return r
}
