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

	"bitdefender-pp-cli/internal/cliutil"
	"bitdefender-pp-cli/internal/config"
	"bitdefender-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Declarations is what the connector holds and where each credential may go.
//
// GravityZone authenticates every Public API call with one API key, sent as
// HTTP Basic (base64 of "<key>:") to the configured Control Center access URL.
// The key comes from BITDEFENDER_API_KEY, config.toml or the 0600
// credentials.toml store (auth set-token). The API is JSON-RPC 2.0: the
// envelope (internal/cli/jsonrpc_transport.go) sits above the kit's outbound
// guard, so the kit sees every request and response as it crosses the wire.
//
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"BITDEFENDER_API_KEY",
		"BITDEFENDER_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"BITDEFENDER_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	// accounts create/update and companies get-by-user take a user's
	// password, quarantine restore-exchange an Exchange user's; license
	// set-key, add-product-key and remove-product-key take a license key.
	CredentialFlags: []string{"--password", "--license-key"},
	StdinFlags:      []string{"--stdin"},
	// The same credentials in a JSON-RPC request's params: a user's
	// password (accounts, companies get-by-user, quarantine restore-exchange,
	// a vCenter integration's specifics, a gather-logs network share), a
	// license key, a package's uninstall and proxy passwords, the Event Push
	// receiver's Authorization value and a gather-logs S3 secret key.
	CredentialFields: []string{"password", "licenseKey", "uninstallPassword", "proxyPassword", "authorization", "secretKey"},
	// Results that carry one: license get (licenseKey), push get
	// (serviceSettings.authorization), policies get (powerUser and proxy
	// passwords), and any integration, package or inventory item that
	// echoes a credential field it was configured with.
	ResponseFields: []string{"licenseKey", "authorization", "password", "uninstallPassword", "proxyPassword", "secretKey"},
	// The same names as MCP arguments: bitdefender_execute and
	// bitdefender_read params (held at any depth) and the CLI-mirror tools'
	// flag names.
	MCPArguments: []string{"password", "license-key", "licenseKey", "uninstallPassword", "proxyPassword", "authorization", "secretKey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "BITDEFENDER_BASE_URL",
		Default:   "https://cloud.gravityzone.bitdefender.com/api",
	},
	Origins: []credkit.Origin{
		// Every API credential goes only to the Control Center access URL the
		// run really uses (an explicit --config or BITDEFENDER_CONFIG wins
		// over a discovered default; BITDEFENDER_BASE_URL overrides both).
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:BITDEFENDER_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key goes only to Spacefast's API.
		// deliver_spacefast.go holds it the moment a publish receipt carries
		// it, and again when an update reads it back to send as the bearer.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the Control Center access URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; GravityZone uses only the API key"},
		{Source: "response:value", Reason: "too generic to hold: policies get returns passwordConfig.value beside hundreds of non-credential value members, and holding them all would withhold every later result; the policy's other password fields are held as response:password (kit note in .review/kit-findings.md)"},
		{Source: "response:key", Reason: "too generic to hold: policies get returns securityTelemetry.config.key beside other key members; holding them all would withhold every later result (kit note in .review/kit-findings.md)"},
		{Source: "field:accessKey", Reason: "a gather-logs S3 access key id names the bucket's key pair and authenticates nothing without secretKey, which is held"},
		{Source: "env:BITDEFENDER_BASE_URL", Reason: "the Control Center access URL; a credential inside it is held through BaseURL"},
		{Source: "env:BITDEFENDER_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:BITDEFENDER_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:BITDEFENDER_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:BITDEFENDER_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:BITDEFENDER_DATA_DIR", Reason: "directory path"},
		{Source: "env:BITDEFENDER_STATE_DIR", Reason: "directory path"},
		{Source: "env:BITDEFENDER_CACHE_DIR", Reason: "directory path"},
		{Source: "env:BITDEFENDER_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:BITDEFENDER_DB", Reason: "presence check only: a selected client profile refuses this override"},
		{Source: "env:BITDEFENDER_DATABASE", Reason: "presence check only: a selected client profile refuses this override"},
		{Source: "env:BITDEFENDER_DATA_FILE", Reason: "presence check only: a selected client profile refuses this override"},
		{Source: "env:BITDEFENDER_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:BITDEFENDER_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:BITDEFENDER_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:BITDEFENDER_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:BITDEFENDER_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:BITDEFENDER_LEARN_SESSION", Reason: "session grouping label; a held credential inside it is still withheld from the journal, which appends through the kit"},
		{Source: "env:BITDEFENDER_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:BITDEFENDER_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:BITDEFENDER_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:BITDEFENDER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:BITDEFENDER_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:BITDEFENDER_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:BITDEFENDER_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:BITDEFENDER_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:CODEX_THREAD_ID", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "agent harness session id; the journal records only a hash of it"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or BITDEFENDER_CONFIG wins, as it does in
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
		explicit = os.Getenv("BITDEFENDER_CONFIG")
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

// holdDirSources holds the run-profile store (ruling 8c): a profile value
// saved under a credential flag's name or under base-url is a source, every
// duplicate included. It is a JSON document the kit reads by its declared
// names (credkit.HoldBody), and each URL under base-url by the kit's URL reader
// (holdURLKeys); one that does not parse is held whole, every word of it. The
// legacy ~/.bitdefender-cli/profiles.json is read the same way.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".bitdefender-cli", "profiles.json"))
	}
}

// holdFile holds what one credential source carries: its credential fields,
// read by declared name, and each URL it saves under a base URL key, read as a
// URL (holdURLKeys). A missing file holds nothing.
func holdFile(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
	holdURLKeys(data)
}

// holdURLKeys holds the password and every query value of each string a
// credential source saves under a base URL key (base-url or base_url, in any
// case), at any depth, through the kit's URL reader (credkit.HoldURL). These
// are the run-profile stores (a profile value under base-url, ruling 8c), the
// credential store (the base_url a key was saved for, which a refusal names)
// and every config file this program only holds; the config it uses has its
// base_url held by the kit itself. Contract 1 calls such a value the userinfo
// or query of a base URL, so it is held under "userinfo", bound to the
// configured base URL's origin by the "*" Origins entry. HoldBody reads the
// same file by declared name and is unchanged; a URL there is no declared name.
func holdURLKeys(data []byte) {
	holdJSONURLKeys(data)
	var doc map[string]any
	if toml.Unmarshal(data, &doc) == nil {
		holdTOMLURLKeys(doc, false)
	}
}

// holdJSONURLKeys reads a JSON document token by token, so every member of a
// duplicate key is seen (json.Unmarshal keeps only the last). A value under a
// base URL key is held, and so is every string nested inside one. Whatever
// parses before an error is held; the rest of a file that does not parse is
// held whole by credkit.HoldBody.
func holdJSONURLKeys(data []byte) {
	type frame struct {
		object    bool // an object, else an array
		key       bool // in an object: the next token is a member name
		under     bool // this container is inside a base URL key's value
		nextUnder bool // in an object: the value about to be read is under one
	}
	var stack []*frame
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		var top *frame
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].key = true
			}
			continue
		}
		if top != nil && top.object && top.key {
			name, _ := tok.(string)
			top.key, top.nextUnder = false, top.under || baseURLKey(name)
			continue
		}
		under := false
		if top != nil {
			under = top.under
			if top.object {
				under = top.nextUnder
			}
		}
		switch v := tok.(type) {
		case json.Delim: // '{' or '['; the parent's next name follows its close
			stack = append(stack, &frame{object: v == '{', key: v == '{', under: under})
			continue
		case string:
			if under && v != "" {
				credkit.HoldURL("userinfo", v)
			}
		}
		if top != nil && top.object {
			top.key = true
		}
	}
}

// holdTOMLURLKeys holds each string under a base URL key in a TOML document
// (a key appears once there; a duplicate does not parse, and HoldBody holds
// such a file whole).
func holdTOMLURLKeys(v any, under bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			holdTOMLURLKeys(child, under || baseURLKey(k))
		}
	case []any:
		for _, child := range x {
			holdTOMLURLKeys(child, under)
		}
	case []map[string]any:
		for _, child := range x {
			holdTOMLURLKeys(child, under)
		}
	case string:
		if under && x != "" {
			credkit.HoldURL("userinfo", x)
		}
	}
}

// baseURLKey reports a member name that is base-url or base_url, in any case:
// holding a URL's password costs nothing when the name only resembles one.
func baseURLKey(name string) bool {
	return strings.EqualFold(strings.ReplaceAll(name, "_", "-"), "base-url")
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
