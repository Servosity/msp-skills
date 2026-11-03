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

	"watchguard-pp-cli/internal/cliutil"
	"watchguard-pp-cli/internal/config"
	"watchguard-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// WatchGuard Cloud authenticates with three values from Managed Access: the
// Access ID (WATCHGUARD_CLIENT_ID), its API password (WATCHGUARD_CLIENT_SECRET)
// and the API key (WATCHGUARD_API_KEY, sent as the WatchGuard-API-Key header).
// They come from the environment, auth login's flags, config.toml or the 0600
// credentials.toml store. The Access ID and password are sent as HTTP Basic to
// the token URL for a one-hour bearer (client.mintClientCredentials, auth
// login); a Service Provider also mints one bearer per managed account's
// audience (client.ScopedBearer). Every bearer is held as soon as it exists.
//
// Every credential goes to the one WatchGuard Cloud region this run is
// configured for (base_url). The documented token URL is that region's host
// plus /oauth/token (README, guide.md), so it is on the same origin and no
// other origin is declared: a token URL on another host is refused by the kit
// (fail closed). The Access ID is declared secret so it never prints either,
// auth login --dry-run included.
//
// The store, config.toml and the Spacefast delivery record are credential
// SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile and the explicit-profile store writer),
// because the kit would withhold them by contract.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"WATCHGUARD_CLIENT_ID",
		"WATCHGUARD_CLIENT_SECRET",
		"WATCHGUARD_API_KEY",
		"WATCHGUARD_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"WATCHGUARD_TOKEN_URL",
		"WATCHGUARD_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "api_key"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "api_key"},
	// auth login's Access ID and API password, and the request-body flags that
	// carry a secret: AuthPoint's password, one-time password, Forgot Token
	// activation and verification codes and QR code response, and a BOVPN
	// tunnel's pre-shared key.
	CredentialFlags: []string{
		"--client-id", "--client-secret",
		"--password", "--otp", "--activation-code", "--verification-code", "--qr-code-response",
		"--psk",
	},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"WatchGuard-API-Key"},
	// The same secrets in a request body (the generated commands' body keys).
	CredentialFields: []string{"password", "otp", "activationCode", "verificationCode", "qrCodeResponse", "psk", "client_secret", "refresh_token"},
	// What the token endpoint answers with, and the credentials WatchGuard
	// responses carry (the spec's schema properties): BOVPN pre-shared keys,
	// certificate private keys and PKCS#12 bundles, Wi-Fi passphrases, user,
	// PPPoE and Dynamic DNS passwords, DHCP auth tokens, product activation and
	// license keys, and AuthPoint's codes.
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "auth_token",
		"password", "passphrase", "psk", "pvt_key", "pfx",
		"activationKey", "initialServiceLicenseKey", "parentLicenseKey",
		"activationCode", "verificationCode", "qrCodeResponse", "otp",
	},
	// auth login is hidden from MCP; a mirrored tool still never takes the
	// pair. watchguard_execute's params carry body fields at any depth.
	MCPArguments: []string{
		"client_id", "client_secret", "client-id", "client-secret", "access_token", "refresh_token", "api_key",
		"password", "otp", "activation-code", "activationCode", "verification-code", "verificationCode",
		"qr-code-response", "qrCodeResponse", "psk", "passphrase", "pvt_key", "pfx",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "WATCHGUARD_BASE_URL",
		Default:   "https://api.usa.cloud.watchguard.com",
	},
	Origins: []credkit.Origin{
		// Every API credential, the Access ID and password, the API key and every
		// minted bearer go only to the configured region (base_url): its API and
		// its /oauth/token. Stored credentials are applied only to the server
		// they were recorded for (config.allowsGlobalCredential).
		{Source: "*", Allowed: []string{"base_url"}},
		{Source: "minted:token", Allowed: []string{"base_url"}},
		{Source: "minted:scoped-token", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:WATCHGUARD_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key (deliver_spacefast.go holds it before
		// an update sends it as the bearer) goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held by credwire.holdConfigURLs"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:token_binding", Reason: "a SHA-256 fingerprint of the Access ID, scope and token URL (config.TokenBindingFor); it authenticates nothing"},
		{Source: "config:refresh_login_id", Reason: "a SHA-256 of a refresh token (config.oauthRefreshLoginID); it authenticates nothing"},
		{Source: "config:template_vars", Reason: "path template values such as accountId, not credentials"},
		{Source: "env:WATCHGUARD_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:WATCHGUARD_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:WATCHGUARD_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:WATCHGUARD_ACCOUNT_ID", Reason: "a WatchGuard Cloud account id (ACC-... or WGC-...), printed in every account listing"},
		{Source: "env:WATCHGUARD_AUDIENCE", Reason: "a managed account's audience value, which selects the account a minted bearer is scoped to and authenticates nothing"},
		{Source: "env:WATCHGUARD_OAUTH_SCOPE", Reason: "an OAuth scope name"},
		{Source: "env:WATCHGUARD_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:WATCHGUARD_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:WATCHGUARD_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:WATCHGUARD_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:WATCHGUARD_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:WATCHGUARD_DATA_DIR", Reason: "directory path"},
		{Source: "env:WATCHGUARD_STATE_DIR", Reason: "directory path"},
		{Source: "env:WATCHGUARD_CACHE_DIR", Reason: "directory path"},
		{Source: "env:WATCHGUARD_HOME", Reason: "root directory for config, data, state and cache; Files resolves it"},
		{Source: "env:WATCHGUARD_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:WATCHGUARD_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:WATCHGUARD_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:WATCHGUARD_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:WATCHGUARD_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:WATCHGUARD_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:WATCHGUARD_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:WATCHGUARD_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:WATCHGUARD_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:WATCHGUARD_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:WATCHGUARD_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:WATCHGUARD_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:WATCHGUARD_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SPACEFAST_API_URL", Reason: "Spacefast API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:SPACEFAST_TEAM", Reason: "Spacefast team id, not a credential"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:APPDATA", Reason: "directory path (Windows)"},
		{Source: "env:LOCALAPPDATA", Reason: "directory path (Windows)"},
		{Source: "env:USERNAME", Reason: "the operating system account name (Windows ACL checks)"},
		{Source: "env:SYSTEMROOT", Reason: "the Windows directory path"},
		{Source: "env:CODEX_SESSION_ID", Reason: "an agent harness session id; the journal stores only its hash"},
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
		{Source: "env:GODEBUG", Reason: "Go runtime debug settings"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// config the program will really use FIRST, because the kit takes the base URL
// (and with it every credential's allowed origin) from the first file naming
// one: an explicit --config or WATCHGUARD_CONFIG wins, as it does in
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
		explicit = os.Getenv("WATCHGUARD_CONFIG")
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
	holdURLSettings()
	holdConfigURLs(used)
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

// holdDirSources holds the run-profile stores (profiles.json under the config
// directory and the legacy ~/.watchguard-cli one) as sources of contract
// section 1: a value saved under a credential flag's name, every member of a
// duplicate key included, is held by its declared name (credkit.HoldBody), and
// whatever a value saved under base-url carries is held by credkit.HoldURL. A
// store that does not parse is held whole, every word of it. Free text (a
// description, a --deliver URL) is no source and is not read.
//
// A store is read here and not named in Files: the kit's own config reader
// would hold every string in a nested file, and saved flag values such as
// --select would then withhold ordinary output.
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdProfileStore(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".watchguard-cli", "profiles.json"))
	}
}

// holdProfileStore holds one run-profile store (see holdDirSources).
func holdProfileStore(path string) {
	data, err := readRegular(path)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}
	credkit.HoldBody(data)
	eachValueUnder(data, isBaseURLKey, func(v string) {
		if strings.TrimSpace(v) != "" {
			credkit.HoldURL("userinfo", v)
		}
	})
}

// isBaseURLKey reports whether a profile key names the base URL, in any
// spelling a flag or a config key gives it.
func isBaseURLKey(k string) bool {
	k = strings.ReplaceAll(strings.ToLower(strings.TrimLeft(k, "-")), "_", "-")
	return k == "base-url" || k == "baseurl"
}

// eachValueUnder calls f with every string value whose own key satisfies
// match, at any depth, every member of a duplicate key included: json.Unmarshal
// keeps only the last of each.
func eachValueUnder(data []byte, match func(key string) bool, f func(value string)) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(key string)
	walk = func(key string) {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch tok {
		case json.Delim('{'):
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return
				}
				name, _ := k.(string)
				walk(name)
			}
			_, _ = dec.Token()
		case json.Delim('['):
			for dec.More() {
				walk(key)
			}
			_, _ = dec.Token()
		default:
			if s, ok := tok.(string); ok && match(key) {
				f(s)
			}
		}
	}
	walk("")
}

// holdURLSettings holds the password and every query value of each URL setting
// in the environment with the kit's own reader, credkit.HoldURL, which also
// reads them in an address with a typo (https//u:pw@h, u:pw@h). The kit's
// BaseURL and URLEnv readers hold the well-formed ones; the program prints a
// malformed one back in a connection error and in doctor.
func holdURLSettings() {
	for _, name := range []string{"WATCHGUARD_BASE_URL", "WATCHGUARD_TOKEN_URL", "WATCHGUARD_FEEDBACK_ENDPOINT"} {
		if v := os.Getenv(name); strings.TrimSpace(v) != "" {
			credkit.HoldURL("userinfo", v)
		}
	}
}

// holdConfigURLs holds what the base_url and token_url of each file in paths
// carry, read with credkit.HoldURL. A file that does not parse is held whole by
// the kit itself.
func holdConfigURLs(paths []string) {
	for _, path := range paths {
		data, err := readRegular(path)
		if err != nil {
			continue
		}
		var urls struct {
			BaseURL  string `toml:"base_url"`
			TokenURL string `toml:"token_url"`
		}
		if toml.Unmarshal(data, &urls) == nil {
			for _, v := range []string{urls.BaseURL, urls.TokenURL} {
				if strings.TrimSpace(v) != "" {
					credkit.HoldURL("userinfo", v)
				}
			}
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

// maxHeldSourceBytes bounds what one source read takes; a config or store is a
// few hundred bytes.
const maxHeldSourceBytes = 8 << 20

// readRegular reads path only when it is a regular file, so a FIFO or a device
// named as a source cannot stall the start.
func readRegular(path string) ([]byte, error) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 -- a credential source this program reads anyway
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	return io.ReadAll(io.LimitReader(f, maxHeldSourceBytes))
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
