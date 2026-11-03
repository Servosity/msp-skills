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

	"jira-service-management-pp-cli/internal/cliutil"
	"jira-service-management-pp-cli/internal/config"
	"jira-service-management-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Jira Service Management authenticates with HTTP Basic: the Atlassian
// account's email and an API token, from JIRA_SERVICE_MANAGEMENT_EMAIL and
// JIRA_SERVICE_MANAGEMENT_API_TOKEN, config.toml or the 0600 credentials.toml
// store (auth set-credentials writes it). The credential goes to the site the
// config names (https://<site>.atlassian.net) and, for the Assets commands, to
// https://api.atlassian.com.
//
// The base URL is a template ("https://{site}.atlassian.net/...") filled from
// the site at run time, so the kit cannot read the site's origin from it. Every
// credential may therefore go to any *.atlassian.net site and
// api.atlassian.com, and the connector's own pins narrow that to the one site:
// the site comes from the config, the Assets workspace and cloudId are checked
// against it (internal/client/jsm_request_guard.go), and a redirect off the
// origin drops Authorization and every config header. An overridden base URL
// (JIRA_SERVICE_MANAGEMENT_BASE_URL or a config base_url with no template) is
// the "base_url" origin.
//
// config.toml and credentials.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"JIRA_SERVICE_MANAGEMENT_API_TOKEN",
		"JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"service_management_api_token", "auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"service_management_api_token", "auth_header", "access_token", "refresh_token", "client_secret"},
	// No StdinFlags: no flag here reads a credential from stdin. --stdin reads
	// a request body, whose credential fields takeStdinBody holds.
	//
	// An Assets object's avatar carries a bearer JWT for Atlassian's media
	// API. jsm_scrub.go drops it by its literal key before any sink; the kit
	// holds it whatever the key's spelling.
	ResponseFields: []string{"mediaJwtToken"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "JIRA_SERVICE_MANAGEMENT_BASE_URL",
		Default:   "https://{site}.atlassian.net/rest/servicedeskapi",
	},
	Origins: []credkit.Origin{
		{Source: "*", Allowed: []string{"base_url", "https://*.atlassian.net", "https://api.atlassian.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "env:JIRA_SERVICE_MANAGEMENT_EMAIL", Reason: "the Atlassian account's email; Atlassian answers repeat it (reporter, author, emailAddress), and it authenticates nothing without the API token"},
		{Source: "config:service_management_email", Reason: "the Atlassian account's email; Atlassian answers repeat it (reporter, author, emailAddress), and it authenticates nothing without the API token"},
		{Source: "basic-user", Reason: "the user of the Basic credential is the account's email (see config:service_management_email)"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret"},
		{Source: "config:template_vars", Reason: "the site name, cloudId and Assets workspace id the base URL template is filled from; Atlassian prints each in its own answers"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_SITE", Reason: "the Atlassian site name, part of the host every request goes to"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CLOUD_ID", Reason: "the site's cloudId, which tenant_info publishes without authentication"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID", Reason: "the Assets workspace id, part of every Assets URL"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_DATA_DIR", Reason: "directory path"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_STATE_DIR", Reason: "directory path"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CACHE_DIR", Reason: "directory path"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_MCP_UPLOAD_ROOT", Reason: "directory path the MCP upload tools read under"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:JIRA_SERVICE_MANAGEMENT_USER_AGENT", Reason: "User-Agent string"},
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
// (and with it the "base_url" origin) from the first file naming one: an
// explicit --config or JIRA_SERVICE_MANAGEMENT_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// The credential stores that run reads follow the config. Every other file any
// --config or --home could mean is only held. No flag arity is guessed:
// credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	takeStdinBody(args)
	holdSetCredentialsArgs(args)
	holdURLInputs(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("JIRA_SERVICE_MANAGEMENT_CONFIG")
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
		holdProfileStore()
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	holdProfileStore()
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
// order config.Load reads it: the config file (the legacy one when no explicit
// config is named and the default is absent), then the credential store beside
// an explicit config, then the global store. A store comes after every config,
// so its base_url (the origin a saved credential is bound to) decides the
// allowed origin only when no config names one.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); err != nil {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	if explicit != "" {
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil && !slices.Contains(files, store) {
		files = append(files, store)
	}
	return files
}

func holdOnly(files []string) {
	for _, f := range files {
		if data, err := readRegular(f); err == nil && len(bytes.TrimSpace(data)) > 0 {
			credkit.HoldBody(data)
		}
	}
}

// holdSetCredentialsArgs holds every word after set-credentials on the command
// line, flag names and their values included, and the value of a
// --name=value word: an extra word held is safe, a credential missed is not,
// and a token mistyped as a flag (--<token>) is echoed by the flag parser.
// Only the help flags and "--" are left, so `auth set-credentials --help`
// still prints. The email and the API token are both held. The command tree is
// not built here to tell flag names apart: Files runs before the kit's output
// layer exists, and every command constructor would then run before it too.
func holdSetCredentialsArgs(args []string) {
	i := slices.Index(args, "set-credentials")
	if i < 0 {
		return
	}
	for _, a := range args[i+1:] {
		switch a {
		case "--", "-h", "--help":
			continue
		}
		credkit.Hold("config:service_management_api_token", a)
		if name, v, ok := strings.Cut(a, "="); ok && strings.HasPrefix(name, "-") {
			credkit.Hold("config:service_management_api_token", v)
		}
	}
}

// holdURLInputs holds what the kit's BaseURL read cannot see: a base URL with
// no scheme (ops:secret@host), which it reads as an opaque one, and
// --deliver, whose webhook a preview names and profile save keeps.
func holdURLInputs(args []string) {
	credkit.HoldURL("url:JIRA_SERVICE_MANAGEMENT_BASE_URL", os.Getenv("JIRA_SERVICE_MANAGEMENT_BASE_URL"))
	for _, v := range credkit.FlagValues(args, "--deliver") {
		if !strings.HasPrefix(v, "file:") && !strings.HasPrefix(v, "spacefast:") {
			credkit.HoldURL("url:deliver", strings.TrimPrefix(v, "webhook:"))
		}
	}
}

// holdProfileStore holds what the run-profile stores carry: every value of a
// credential-like name (the names internal/cli/profile.go
// credentialLikeRunProfileFlag refuses to save, which a hand-edited or older
// store can still hold) and of a declared one (credkit.HoldBody), every
// duplicate key included. The kit is not given the file: it is nested JSON, so
// the kit would hold every string in it, and a profile's plain "true" would
// refuse every start. Only a regular file is read: a pipe named profiles.json
// would hang startup.
func holdProfileStore() {
	var stores []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		stores = append(stores, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		stores = append(stores, filepath.Join(home, ".jira-service-management-cli", "profiles.json"))
	}
	for _, p := range stores {
		data, err := readRegular(p)
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		credkit.HoldBody(data) // values under a declared name; every word of a file that does not parse
		holdCredentialLikeValues(data)
	}
}

// holdCredentialLikeValues walks the JSON tokens, not a decoded tree, which
// would keep only the last member of a duplicate key. A file that does not
// parse is held up to where it stops here, and whole by HoldBody.
func holdCredentialLikeValues(data []byte) {
	type frame struct{ object, atKey bool }
	var stack []frame
	key := ""
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		top := len(stack) - 1
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				stack = append(stack, frame{object: d == '{', atKey: d == '{'})
				continue
			}
			stack = stack[:top]
			if top > 0 && stack[top-1].object {
				stack[top-1].atKey = true
			}
			continue
		}
		if top >= 0 && stack[top].atKey {
			key, _ = tok.(string)
			stack[top].atKey = false
			continue
		}
		if credentialLikeName(key) {
			switch v := tok.(type) {
			case string:
				credkit.Hold("profile:"+key, v)
			case json.Number:
				credkit.Hold("profile:"+key, v.String())
			}
		}
		if top >= 0 && stack[top].object {
			stack[top].atKey = true
		}
	}
}

// credentialLikeName is internal/cli/profile.go credentialLikeRunProfileFlag.
func credentialLikeName(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
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

// takeStdinBody takes a --stdin body off stdin inside Start, before anything
// prints, and holds the credential fields in it; RestoreStdinBody gives the
// program the same bytes back after Start.
//
// --stdin is no credential source: on every command but two it reads a JSON
// request body, and Atlassian's answers repeat the body, so holding it whole
// withheld the answer of every write and read made with one. What is held is
// the body's credential fields (credkit.HoldBody: the declared names and
// authorization, auth_header, proxy-authorization and cookie, at any depth,
// every duplicate), on every command that takes one, previews and --dry-run
// included. A body that is no document is held whole.
//
// `feedback --stdin` and `request note --stdin` are the exception: their body
// is free text for a person to read (feedback, an agent-only note), no
// document, and HoldBody would hold every word of it, withholding the
// command's own output or refusing the start on a line such as "OK". It is not
// held. Only a command line that starts with "feedback" or "request note"
// qualifies: free text with a flag in front is held as bytes that are no
// document are, every word, which refuses the start but never prints a secret.
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
	if !(len(args) > 0 && args[0] == "feedback") && !(len(args) > 1 && args[0] == "request" && args[1] == "note") {
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
