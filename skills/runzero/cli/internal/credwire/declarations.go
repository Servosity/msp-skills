// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"runzero-pp-cli/internal/cliutil"
	"runzero-pp-cli/internal/config"
	"runzero-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// runZero authenticates with one Bearer key, from RUNZERO_API_KEY, config.toml
// or the 0600 credentials.toml store. The store and config.toml are credential
// SOURCES: their own writes go outside the kit (cliutil.AtomicWriteCredentialFile)
// and they are named in Files, because the kit would withhold them by contract.
// The API also takes an OAuth client secret (POST /account/api/token and
// /account/api/rotate), an integration credential's secret (PUT
// /account/credentials), and returns API key, export and download tokens, an
// access token and a rotated API client secret. password and api_key are the
// spec's generic credential members (integration credential fields), held and
// masked by name wherever a request or a response carries one.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"RUNZERO_API_KEY",
		"RUNZERO_MCP_HTTP_TOKEN",
	},
	Files:            credentialFiles,
	ConfigFields:     []string{"auth_header", "api_key", "access_token", "refresh_token", "client_secret", "headers"},
	StoredRecords:    []string{"auth_header", "api_key", "access_token", "refresh_token", "client_secret"},
	CredentialFlags:  []string{"--client-secret", "--secret"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"client_secret", "secret", "password", "api_key"},
	ResponseFields:   []string{"token", "export_token", "download_token", "access_token", "api_client_secret", "password", "api_key"},
	MCPArguments:     []string{"client_secret", "client-secret", "secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "RUNZERO_BASE_URL",
		Default:   "https://console.runzero.com/api/v1.0",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for;
		// the origin pin (internal/cli/runzero_guard.go) narrows every request,
		// redirects included, to that origin too.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:RUNZERO_MCP_HTTP_TOKEN", Allowed: nil},
		// The MCP HTTP server's TLS private key is sent nowhere.
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:client_id", Reason: "the OAuth client's identifier, sent beside its secret; runZero repeats an account's client_id on every object it returns, so holding it would withhold every result"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:logged_out", Reason: "boolean marker in the credential store"},
		{Source: "env:RUNZERO_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:RUNZERO_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:RUNZERO_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:RUNZERO_CACHE_DIR", Reason: "directory path"},
		{Source: "env:RUNZERO_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:RUNZERO_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:RUNZERO_DATA_DIR", Reason: "directory path"},
		{Source: "env:RUNZERO_STATE_DIR", Reason: "directory path"},
		{Source: "env:RUNZERO_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:RUNZERO_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:RUNZERO_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:RUNZERO_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:RUNZERO_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:RUNZERO_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:RUNZERO_LEARN_SESSION", Reason: "session grouping label, stored only as its hash"},
		{Source: "env:RUNZERO_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:RUNZERO_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:RUNZERO_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:RUNZERO_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:RUNZERO_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:RUNZERO_MCP_UPLOAD_ROOT", Reason: "directory path"},
		{Source: "env:RUNZERO_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:RUNZERO_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:RUNZERO_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:CODEX_SESSION_ID", Reason: "harness session id, stored only as its hash"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// StartupConfigFlag and StartupHomeFlag are the --config and --home values
// startup read the files with: the last occurrence of each on the command line,
// or "". The kit took the allowed origin from that reading, so a run whose
// parser chose other files is refused (CheckStartupFlags).
var StartupConfigFlag, StartupHomeFlag string

var errStartupFlags = errors.New("--config or --home is also the value of another flag here, so it is not clear which file this run uses: write each once, as its own flag")

// credentialFiles runs inside Start, before anything can print. It returns the
// files this run reads, the config the program will really use FIRST, because
// the kit takes the base URL (and with it every credential's allowed origin)
// from the first file naming one: an explicit --config or RUNZERO_CONFIG wins,
// as it does in config.Load, and the last --home decides the default paths, as
// pflag does. Every other file any --config or --home could mean is only held
// (config.HoldFile): its values cannot print, and its base_url cannot become an
// allowed origin. No flag arity is guessed: credkit.FlagValues counts every
// occurrence.
func credentialFiles(args []string) []string {
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		holdTLSKey(keyFile)
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	StartupConfigFlag, StartupHomeFlag = explicit, ""
	if len(homes) > 0 {
		StartupHomeFlag = homes[len(homes)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("RUNZERO_CONFIG")
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
		defaults := runFiles("")
		restore()
		holdOnly(defaults)
		if i == len(homes)-1 {
			used = files
		} else {
			holdOnly(files)
		}
	}
	holdOnly(runFiles(""))
	for _, cfg := range configs {
		holdOnly(runFiles(cfg))
	}
	return used
}

func holdOnly(files []string) {
	for _, f := range files {
		config.HoldFile(f)
	}
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, then the legacy config when no
// explicit one was named and the default is absent. The credential stores and
// the run profile stores are held here, not returned.
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
	// config.Load takes no base URL from a credential store, so one written
	// there must not become an allowed origin: the stores are only held.
	if explicit != "" {
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			config.HoldFile(store)
		}
	}
	if store, err := cliutil.CredentialsFilePath(); err == nil {
		config.HoldFile(store)
	}
	// The run profile store is a source too. It is nested JSON, which the kit's
	// file reader answers by holding every string in it (a profile named "p" or a
	// saved --limit 7 would refuse every later start), so it is read with
	// credkit.HoldBody: the values under the credential flags (MCPArguments) are
	// held, every duplicate included.
	if dir, err := cliutil.ConfigDir(); err == nil {
		config.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		config.HoldFile(filepath.Join(home, ".runzero-cli", "profiles.json"))
	}
	return files
}

// holdTLSKey holds the private key the MCP server would serve TLS with: each PEM
// body line and the decoded key as hex, so neither its text nor its bytes can be
// printed. Armor and header lines are not the key, and holding them would
// withhold any PEM text. Its raw bytes are not held: the kit's startup check of
// a binary value is slow enough to stall the start.
func holdTLSKey(path string) {
	data, err := config.ReadRegularFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") && !strings.Contains(line, ":") {
			credkit.Hold("minted:tls-key", line)
		}
	}
	for rest := data; ; {
		block, more := pem.Decode(rest)
		if block == nil {
			return
		}
		credkit.Hold("minted:tls-key", hex.EncodeToString(block.Bytes))
		rest = more
	}
}

// CheckStartupFlags refuses a run whose parsed --config or --home is not what
// startup read. A flag that takes a value can swallow one (--select --config x),
// and then the allowed origin came from a file this run does not use.
func CheckStartupFlags(configPath, home string) error {
	if configPath != StartupConfigFlag || home != StartupHomeFlag {
		return errStartupFlags
	}
	return nil
}
