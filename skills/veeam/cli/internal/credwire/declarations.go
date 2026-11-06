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

	"veeam-pp-cli/internal/cliutil"
	"veeam-pp-cli/internal/config"
	"veeam-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Veeam Service Provider Console authenticates with one bearer token
// (VEEAM_TOKEN, config.toml's token or the 0600 credentials store), sent to
// the configured appliance (base_url) and nowhere else. The CLI never mints
// or refreshes it: the operator supplies it.
//
// VSPC itself manages credentials, so request and response bodies carry them:
// the credential records of backup servers, management agents, discovery
// rules and public cloud sites, generated key pairs and TOTP secrets, new API
// key logins, Microsoft 365 organization secrets and SMTP OAuth tokens. Their
// field names (from the VSPC 3.6.2 OpenAPI document: every property in format
// "password", plus the key, secret and token properties) are declared below.
// A declared response field is held before the program reads the response,
// so it never reaches the store, the cache, the journal or the logs, and an
// output that carries it is withheld (the kit's notice stands in its place).
//
// The store and config.toml are credential SOURCES: their own writes go
// outside the kit (cliutil.AtomicWriteCredentialFile), because the kit would
// withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"VEEAM_TOKEN",
		"VEEAM_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"VEEAM_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files: credentialFiles,
	// headers is the config's [headers] table: every value in it is sent on
	// each request, so each is held as a credential. client_id is held too:
	// VSPC uses no OAuth client, so a value put there is never needed in output.
	ConfigFields:  []string{"token", "auth_header", "headers", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"token", "auth_header", "headers", "access_token", "refresh_token", "client_id", "client_secret"},
	// Request-body and query credentials as command flags (each flag's own
	// value is the credential). JSON-valued object flags that carry a
	// credential inside (--credentials and the like) are held field by field
	// when the request is sent (CredentialFields).
	CredentialFlags: []string{
		"--password",
		"--passphrase",
		"--private-key",
		"--root-password",
		"--new-password",
		"--secret",
		"--secret-key",
		"--secret-url",
		"--api-key",
		"--mfa-token",
		"--refresh-token",
		"--json-configuration",
		"--certificate-content",
		"--access-account-password",
		"--admin-credentials-password",
		"--configuration-admin-credentials-password",
		"--cloud-repository-connection-settings-password",
		"--configuration-credentials-tenant-password",
		"--credentials-tenant-password",
		"--configuration-distribution-password",
		"--distribution-password",
		"--credentials-password",
		"--job-configuration-backup-storage-password",
		"--owner-credentials-password",
		"--password-credential-password",
		"--service-account-password",
		"--client-settings-client-secret",
		"--o-auth2-credential-client-settings-client-secret",
		"--o-auth2-credential-access-token",
		"--o-auth2-credential-refresh-token",
	},
	// --stdin reads a request body: its credential fields are held whole and
	// line by line before anything prints.
	StdinFlags: []string{"--stdin"},
	// Request body and query credential fields, by wire name: every VSPC
	// property in format "password" and the key, secret and token properties
	// beside them. The token endpoint's form (POST /token) carries password,
	// refresh_token and mfa_token.
	CredentialFields: []string{
		"password", "passphrase", "privateKey", "privateKeyContent", "rootPassword", "sshPrivateKey",
		"newPassword", "tenantPassword", "hashedPassword",
		"secretKey", "secret", "secretUrl", "apiKey", "jsonConfiguration", "certificateContent",
		"applicationCertificate", "applicationCertificatePassword", "applicationSecret",
		"clientSecret", "accessToken", "refreshToken", "refresh_token", "mfa_token", "token",
	},
	// Response fields that carry a credential, by wire name. parameters is
	// UserLogin's (the only schema with it): VSPC returns a new API key login's
	// key there.
	ResponseFields: []string{
		"password", "passphrase", "privateKey", "privateKeyContent", "rootPassword", "sshPrivateKey",
		"secretKey", "secret", "secretUrl", "parameters", "hashedPassword", "jsonConfiguration",
		"applicationCertificate", "applicationCertificatePassword", "applicationSecret",
		"clientSecret", "accessToken", "refreshToken", "access_token", "refresh_token", "mfa_token",
		"registrationToken", "deviceCodeToken", "token",
	},
	// The endpoint tools (veeam_execute, veeam_read) take a request body or
	// query as params; these names (CredentialFields') are held anywhere
	// beneath it.
	MCPArguments: []string{
		"password", "passphrase", "privateKey", "privateKeyContent", "rootPassword", "sshPrivateKey",
		"newPassword", "tenantPassword", "hashedPassword",
		"secretKey", "secret", "secretUrl", "apiKey", "jsonConfiguration", "certificateContent",
		"applicationCertificate", "applicationCertificatePassword", "applicationSecret",
		"clientSecret", "accessToken", "refreshToken", "refresh_token", "mfa_token", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "VEEAM_BASE_URL",
		Default:   "https://your-vspc-host:1280/api/v3",
	},
	Origins: []credkit.Origin{
		// Every API credential goes only to the configured VSPC appliance.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:VEEAM_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the VSPC appliance URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:VEEAM_BASE_URL", Reason: "the VSPC appliance URL; a credential inside it is held through BaseURL"},
		{Source: "env:VEEAM_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:VEEAM_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:VEEAM_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:VEEAM_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:VEEAM_DATA_DIR", Reason: "directory path"},
		{Source: "env:VEEAM_STATE_DIR", Reason: "directory path"},
		{Source: "env:VEEAM_CACHE_DIR", Reason: "directory path"},
		{Source: "env:VEEAM_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:VEEAM_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:VEEAM_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:VEEAM_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:VEEAM_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:VEEAM_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:VEEAM_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:VEEAM_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:VEEAM_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:VEEAM_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:VEEAM_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:VEEAM_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:VEEAM_MCP_UPLOAD_ROOT", Reason: "directory an operator opts in for MCP file uploads"},
		{Source: "env:VEEAM_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:VEEAM_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:VEEAM_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:PP_FILELOCK_HOLDER", Reason: "file-lock test helper switch"},
		{Source: "env:PP_FILELOCK_PATH", Reason: "file-lock test helper path"},
		{Source: "env:PP_FILELOCK_READY", Reason: "file-lock test helper path"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or VEEAM_CONFIG wins, as it does in
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
		explicit = os.Getenv("VEEAM_CONFIG")
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

// holdDirSources holds the run-profile stores (current and legacy): a
// profile value saved under a credential flag's name or under base-url is a
// credential source. Each is a JSON document the kit reads by its declared
// names (credkit.HoldBody); a file that does not parse is held whole, every
// word of it.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".veeam-cli", "profiles.json"))
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

// HoldJSONArguments holds the credential fields of every JSON document on the
// command line: a JSON-valued flag such as --credentials
// '[{"userName":"u","password":"..."}]', or a positional body. Such a flag is
// no CredentialFlag, since holding its whole value would withhold every output
// that shares a field name with it; credkit.HoldBody holds only the values
// under the declared field names, before a --dry-run preview can print them.
// Text that is not valid JSON is skipped (HoldBody would hold every string in
// it). The CLI main calls it right after credkit.Start.
func HoldJSONArguments(args []string) {
	for _, a := range args {
		v := a
		if strings.HasPrefix(a, "-") {
			_, after, ok := strings.Cut(a, "=")
			if !ok {
				continue
			}
			v = after
		}
		v = strings.TrimSpace(v)
		if (strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[")) && json.Valid([]byte(v)) {
			credkit.HoldBody([]byte(v))
		}
	}
}
