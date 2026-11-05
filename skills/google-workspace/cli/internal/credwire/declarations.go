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

	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/config"
	"google-workspace-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Google Workspace authenticates two ways. A registered client tenant mints a
// bearer per call with a service-account JWT grant (the private key signs an
// assertion posted to the pinned https://oauth2.googleapis.com/token; the
// answer's access_token is the bearer). Without tenants, a bearer comes from
// GOOGLE_WORKSPACE_TOKEN, ADMIN_ACCESS_TOKEN, config.toml or the 0600
// credentials.toml store. The bearer goes to the configured base URL and, while
// that is Google's default, to the three other Google API hosts the commands
// name (reseller, licensing, Cloud Identity policies).
//
// The store, config.toml, tenants.toml and the copied service-account keys are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile), because the kit would withhold them by
// contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"GOOGLE_WORKSPACE_TOKEN",
		"ADMIN_ACCESS_TOKEN",
		"GOOGLE_WORKSPACE_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"GOOGLE_WORKSPACE_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "workspace_token", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"auth_header", "workspace_token", "access_token", "refresh_token", "client_secret", "private_key", "private_key_id"},
	// The users insert/patch/update password, a watch channel's token and a
	// reseller transfer's customerAuthToken.
	CredentialFlags: []string{"--password", "--token", "--customer-auth-token"},
	StdinFlags:      []string{"--stdin"},
	// assertion is the signed JWT of the service-account grant; the rest are
	// the same credentials in a request body or a token-endpoint form.
	CredentialFields: []string{"assertion", "password", "token", "customerAuthToken", "client_secret", "refresh_token"},
	ResponseFields:   []string{"access_token", "refresh_token", "id_token"},
	MCPArguments:     []string{"password", "token", "customer-auth-token", "customerAuthToken"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "GOOGLE_WORKSPACE_BASE_URL",
		Default:   "https://admin.googleapis.com",
	},
	Origins: []credkit.Origin{
		// Every API credential goes to the configured base URL. While that is
		// Google's default, the reseller, licensing and Cloud Identity commands
		// send it to their own Google hosts; an overridden base URL rebases them
		// onto it (client.rebaseAbsoluteURLOntoOverride).
		{Source: "*", Allowed: []string{"base_url", "https://reseller.googleapis.com", "https://licensing.googleapis.com", "https://cloudidentity.googleapis.com"}},
		{Source: "minted:tenant-token", Allowed: []string{"base_url", "https://reseller.googleapis.com", "https://licensing.googleapis.com", "https://cloudidentity.googleapis.com"}},
		// The service-account grant goes only to the pinned token endpoint
		// (internal/cli/gws_tenants.go googleTokenURL). The key id rides in the
		// assertion's header; the private key itself is sent nowhere.
		{Source: "field:assertion", Allowed: []string{"https://oauth2.googleapis.com"}},
		{Source: "field:private_key_id", Allowed: []string{"https://oauth2.googleapis.com"}},
		{Source: "field:private_key", Allowed: nil},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:GOOGLE_WORKSPACE_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret; Google prints it in every OAuth token and app listing"},
		{Source: "config:client_email", Reason: "a service account's email names the account (tenants add and tenants scopes print it for domain-wide delegation setup) and authenticates nothing without its private key"},
		{Source: "env:GOOGLE_WORKSPACE_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:GOOGLE_WORKSPACE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:GOOGLE_WORKSPACE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:GOOGLE_WORKSPACE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:GOOGLE_WORKSPACE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:GOOGLE_WORKSPACE_DATA_DIR", Reason: "directory path"},
		{Source: "env:GOOGLE_WORKSPACE_STATE_DIR", Reason: "directory path"},
		{Source: "env:GOOGLE_WORKSPACE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:GOOGLE_WORKSPACE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:GOOGLE_WORKSPACE_TENANT", Reason: "name of a registered client tenant, not a credential"},
		{Source: "env:GOOGLE_WORKSPACE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:GOOGLE_WORKSPACE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:GOOGLE_WORKSPACE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:GOOGLE_WORKSPACE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:GOOGLE_WORKSPACE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:GOOGLE_WORKSPACE_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:GOOGLE_WORKSPACE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:GOOGLE_WORKSPACE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:GOOGLE_WORKSPACE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:GOOGLE_WORKSPACE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:GOOGLE_WORKSPACE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:GOOGLE_WORKSPACE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:GOOGLE_WORKSPACE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:GOOGLE_WORKSPACE_USER_AGENT", Reason: "User-Agent string"},
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
// one: an explicit --config or GOOGLE_WORKSPACE_CONFIG wins, as it does in
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
		explicit = os.Getenv("GOOGLE_WORKSPACE_CONFIG")
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

// holdDirSources holds the credential sources under the config directory that
// are not config files: every service-account key (each copied into
// keys/<name>.json by tenants add, and any other key file tenants.toml names)
// and the run-profile store. Each is a JSON document the kit reads by its
// declared names (credkit.HoldBody): a key's private_key and private_key_id, a
// profile value saved under a credential flag's name. A file that does not
// parse is held whole, every word of it.
//
// They are not returned to the kit's own config reader, which holds every
// string of a document outside its canonical subset: a key's escaped PEM makes
// it one, and holding its URLs ("https://oauth2.googleapis.com/token") would
// withhold every later output that carries "https://". tenants.toml (names,
// domains, admin emails, scope URLs and key paths) carries no credential and
// is read here only for the key paths it names.
func holdDirSources() {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return
	}
	keys, _ := filepath.Glob(filepath.Join(dir, "keys", "*.json"))
	for _, k := range keys {
		holdFile(k)
	}
	for _, k := range registryKeyFiles(filepath.Join(dir, "tenants.toml")) {
		holdFile(k)
	}
	holdFile(filepath.Join(dir, "profiles.json"))
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".google-workspace-cli", "profiles.json"))
	}
}

// registryKeyFiles is every key_file tenants.toml names, "~/" expanded as
// expandTenantPath does. A registry that does not parse names none: the
// program refuses it before it reads any key.
func registryKeyFiles(path string) []string {
	data, err := readRegular(path)
	if err != nil {
		return nil
	}
	var reg struct {
		Tenants []struct {
			KeyFile string `toml:"key_file"`
		} `toml:"tenants"`
	}
	if toml.Unmarshal(data, &reg) != nil {
		return nil
	}
	var out []string
	for _, t := range reg.Tenants {
		p := t.KeyFile
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if p != "" {
			out = append(out, p)
		}
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
