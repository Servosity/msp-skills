// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/config"
	"fortinet-pp-cli/internal/credkit"
	"fortinet-pp-cli/internal/fgtcloud"
)

// Declarations is what the connector holds and where each credential may go.
//
// Fortinet reaches four kinds of host, each with its own credential:
//   - The FortiGate at the configured base URL: a REST API token from
//     FORTINET_API_TOKEN, config.toml (api_token) or the 0600 credentials store,
//     sent as a bearer to that origin only.
//   - Fleet devices (fleet commands, --devices / FORTINET_DEVICES): one token
//     per device, read from that device's token_file and held as
//     minted:device-token the moment it is read (resolveToken). Each goes only
//     to its own device URL. Those URLs come from the operator's devices file,
//     and the kit's origins are fixed at build time, so the per-device clients
//     send outside credkit.Transport with the pinning they already carry (https
//     only, no redirect off the device's origin, no other configured credential
//     on the request: handfixes.json plain-http-refused-every-client,
//     redirects-stay-on-origin, fleet-device-client-carries-no-configured-credential).
//     Bound here to no origin, the token is refused by every kit-guarded client.
//   - FortiManager (fmg commands): FORTINET_FMG_API_KEY, sent by internal/fortimgr
//     to the operator's FORTINET_FMG_URL only, https only, no redirects; outside
//     the kit for the same reason, and bound here to no origin.
//   - FortiGate Cloud (cloud commands): FORTINET_CLOUD_API_PASSWORD goes only to
//     the FortiCloud IAM token endpoint; the minted access token only to the
//     regional API hosts; the refresh token only back to the token endpoint.
//     These hosts are fixed, so the Cloud client sends through credkit.Transport.
//
// The credentials store, config.toml and the FortiGate Cloud token cache are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile), because the kit would withhold them by
// contract, and each is held at the next start.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"FORTINET_API_TOKEN",
		"FORTINET_FMG_API_KEY",
		"FORTINET_CLOUD_API_PASSWORD",
		"FORTINET_MCP_HTTP_TOKEN",
		"SPACEFAST_TOKEN",
	},
	URLEnv: []string{
		"FORTINET_FMG_URL",
		"FORTINET_FEEDBACK_ENDPOINT",
		"SPACEFAST_API_URL",
	},
	Files:         credentialFiles,
	ConfigFields:  []string{"auth_header", "api_token", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"auth_header", "api_token", "access_token", "refresh_token", "client_secret"},
	// The FortiGate Cloud password and refresh grants (fgtcloud.Client.grant).
	CredentialFields: []string{"password", "refresh_token"},
	// The FortiGate Cloud token endpoint's answer.
	ResponseFields: []string{"access_token", "refresh_token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "FORTINET_BASE_URL",
	},
	Origins: []credkit.Origin{
		// Every FortiGate credential goes only to the configured base URL.
		{Source: "*", Allowed: []string{"base_url"}},
		// Sent outside the kit, each to its own operator-configured host (above);
		// no kit-guarded client may send either anywhere.
		{Source: "minted:device-token", Allowed: nil},
		{Source: "env:FORTINET_FMG_API_KEY", Allowed: nil},
		// FortiGate Cloud.
		{Source: "env:FORTINET_CLOUD_API_PASSWORD", Allowed: []string{"https://customerapiauth.fortinet.com"}},
		{Source: "response:access_token", Allowed: []string{"https://api.fortigate.forticloud.com", "https://usapi.fortigate.forticloud.com", "https://euapi.fortigate.forticloud.com", "https://jpapi.fortigate.forticloud.com"}},
		{Source: "minted:fgtcloud-access", Allowed: []string{"https://api.fortigate.forticloud.com", "https://usapi.fortigate.forticloud.com", "https://euapi.fortigate.forticloud.com", "https://jpapi.fortigate.forticloud.com"}},
		{Source: "response:refresh_token", Allowed: []string{"https://customerapiauth.fortinet.com"}},
		{Source: "minted:fgtcloud-refresh", Allowed: []string{"https://customerapiauth.fortinet.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:FORTINET_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "an OAuth client ID names an app and authenticates nothing without client_secret"},
		{Source: "env:FORTINET_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:FORTINET_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:FORTINET_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:FORTINET_SKIP_TLS_VERIFY", Reason: "boolean switch"},
		{Source: "env:FORTINET_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:FORTINET_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:FORTINET_DATA_DIR", Reason: "directory path"},
		{Source: "env:FORTINET_STATE_DIR", Reason: "directory path"},
		{Source: "env:FORTINET_CACHE_DIR", Reason: "directory path"},
		{Source: "env:FORTINET_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:FORTINET_DEVICES", Reason: "path of the devices file; each device's token is read from its token_file and held as minted:device-token"},
		{Source: "env:FORTINET_FMG_URL", Reason: "the FortiManager URL; a credential inside it is held through URLEnv"},
		{Source: "env:FORTINET_FMG_INSECURE", Reason: "boolean switch"},
		{Source: "env:FORTINET_CLOUD_API_USER", Reason: "the FortiCloud IAM API user ID, a user name the manifest marks non-sensitive; it authenticates nothing without the password"},
		{Source: "env:FORTINET_CLOUD_ACCOUNT_ID", Reason: "a FortiCloud account number, an identifier"},
		{Source: "env:FORTINET_CLOUD_REGION", Reason: "region name (global, us, eu, jp)"},
		{Source: "env:FORTINET_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:FORTINET_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:FORTINET_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:FORTINET_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and a credential sent to it is refused by the outbound guard"},
		{Source: "env:FORTINET_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:FORTINET_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:FORTINET_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:FORTINET_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:FORTINET_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:FORTINET_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:FORTINET_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:FORTINET_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:FORTINET_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:FORTINET_USER_AGENT", Reason: "User-Agent string"},
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
// one: an explicit --config or FORTINET_CONFIG wins, as it does in
// config.Load, and the last --home decides the default paths, as pflag does.
// Every other file any --config or --home could mean is only held. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("FORTINET_CONFIG")
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

// holdDirSources holds the credential sources that are not config files: the
// run-profile stores (a value saved under a declared name, read by
// credkit.HoldBody) and the FortiGate Cloud token cache, which keeps the minted
// access and refresh tokens (fgtcloud.HoldCache binds each to the FortiCloud
// origins it may go to; a cache that does not parse is held whole).
func holdDirSources() {
	if dir, err := cliutil.ConfigDir(); err == nil {
		holdFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdFile(filepath.Join(home, ".fortinet-cli", "profiles.json"))
	}
	if p := fgtcloud.CachePath(); p != "" {
		fgtcloud.HoldCache(p)
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
