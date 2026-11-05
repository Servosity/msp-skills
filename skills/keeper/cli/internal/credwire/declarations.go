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
	"slices"
	"strings"

	"keeper-pp-cli/internal/cliutil"
	"keeper-pp-cli/internal/config"
	"keeper-pp-cli/internal/credkit"
)

// No report this CLI reads carries a credential field: keeper/run.go refuses record
// content and projects each report to its allowlisted fields. ResponseFields is for the
// answer the projection has not read yet and an error envelope that repeats a request.
//
// Declarations is what the connector holds and where each credential may go.
// The Service Mode API key comes from KEEPER_API_KEY, config.toml or the 0600
// credential store, and goes only to the one Service Mode host: KEEPER_BASE_URL.
// The store and config.toml are credential SOURCES: their own writes go outside the
// kit (cliutil.AtomicWriteCredentialFile) and they are named in Files, because the
// kit would withhold them by contract.
//
// No StdinFlags: the one --stdin flag is feedback's, which reads a note written for
// the feedback ledger, not a credential or a request body. auth set-token reads its
// pipe in the command and holds it at once.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"KEEPER_API_KEY",
		"KEEPER_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"KEEPER_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "api_key", "client_id", "client_secret", "access_token", "refresh_token",
	},
	StoredRecords: []string{
		"auth_header", "api_key", "client_id", "client_secret", "access_token", "refresh_token",
	},
	CredentialHeaders: []string{"api-key"},
	CredentialFields: []string{
		"api_key", "api-key", "apikey", "access_token", "refresh_token", "client_secret",
		"password", "passphrase", "private_key", "secret", "token", "totp",
	},
	ResponseFields: []string{
		"api_key", "api-key", "apikey", "access_token", "refresh_token", "client_secret",
		"password", "passphrase", "private_key", "secret", "token", "totp",
	},
	MCPArguments: []string{
		"api_key", "api-key", "apikey", "access_token", "refresh_token", "client_secret",
		"password", "passphrase", "private_key", "secret", "token", "totp",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "KEEPER_BASE_URL",
		Default:   "https://localhost:9090",
	},
	Origins: []credkit.Origin{
		// The key goes only to the Service Mode base URL this run really uses. The
		// connector's own pin (api-origin-pinned) narrows every request to that origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearers authenticate callers of this server; they are sent nowhere.
		{Source: "env:KEEPER_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials.
		{Source: "config:base_url", Reason: "the Service Mode base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:skip_tls_verify", Reason: "a boolean setting"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "flag:--credential-ref", Reason: "a reference naming where a client profile's credential lives, not the credential"},
		{Source: "flag:--additional-credential-ref", Reason: "references naming where a client profile's credentials live, not the credentials"},
		{Source: "env:KEEPER_BASE_URL", Reason: "the Service Mode base URL; a credential inside it is held through BaseURL"},
		{Source: "env:KEEPER_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:KEEPER_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:KEEPER_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:KEEPER_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:KEEPER_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:KEEPER_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:KEEPER_DATA_DIR", Reason: "directory path"},
		{Source: "env:KEEPER_STATE_DIR", Reason: "directory path"},
		{Source: "env:KEEPER_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:KEEPER_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:KEEPER_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:KEEPER_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:KEEPER_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:KEEPER_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:KEEPER_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:KEEPER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:KEEPER_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:KEEPER_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:KEEPER_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:KEEPER_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:KEEPER_LEARN_SESSION", Reason: "session grouping label, hashed before it is written"},
		{Source: "env:KEEPER_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It names the files
// the run reads and holds every other file any --config or --home on the line can
// mean: holding a file the run does not load is safe, missing the one it loads is not.
func credentialFiles(args []string) []string {
	holdSetTokenArguments(args)
	return pathsUnder(args)
}

// setTokenArguments is what follows `auth set-token` on the line, when the line names
// that command. The two words are matched in order.
func setTokenArguments(args []string) ([]string, bool) {
	i := slices.Index(args, "auth")
	if i < 0 {
		return nil, false
	}
	j := slices.Index(args[i+1:], "set-token")
	if j < 0 {
		return nil, false
	}
	return args[i+1+j+1:], true
}

// holdSetTokenArguments holds a key pasted on the `auth set-token` line. The command
// takes no argument, and its parser repeats what it was given before the command
// runs. Only the command's own flags, and their values, are not a key.
func holdSetTokenArguments(args []string) {
	rest, ok := setTokenArguments(args)
	if !ok {
		return
	}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if a != name || hasValue { // a dashed word
			if takesValue, known := setTokenFlags[name]; known {
				if !hasValue && takesValue {
					i++ // the next word is this flag's value
				}
				continue
			}
			credkit.Hold("stdin", a, name)
			if hasValue {
				credkit.Hold("stdin", value)
			}
			continue
		}
		if strings.TrimSpace(a) != "" {
			credkit.Hold("stdin", a)
		}
	}
}

// setTokenFlags is every flag `auth set-token` declares and whether it takes a value.
// It is data, not a walk of the command tree: Files runs inside Start.
// TestSetTokenFlagsMatchTheCommand keeps it true.
var setTokenFlags = map[string]bool{
	"agent":           false,
	"audit-dir":       true,
	"ca-cert":         true,
	"client-profile":  true,
	"compact":         false,
	"config":          true,
	"csv":             false,
	"data-source":     true,
	"deliver":         true,
	"dry-run":         false,
	"h":               false,
	"help":            false,
	"home":            true,
	"human-friendly":  false,
	"json":            false,
	"max-age":         true,
	"no-cache":        false,
	"no-color":        false,
	"no-input":        false,
	"no-learn":        false,
	"plain":           false,
	"profile":         true,
	"quiet":           false,
	"rate-limit":      true,
	"receipt":         false,
	"receipt-file":    true,
	"select":          true,
	"timeout":         true,
	"tls-server-name": true,
	"yes":             false,
}

// pathsUnder is what the kit reads for the run. Files are listed for the --home the
// run will use, the last on the line; every other file under any --home or --config
// the line can mean is only held, along with the configs Load merely consults, the
// stores that record a base URL and the run profile stores. A listed file's base_url
// becomes the one origin credentials may go to (contract 2c), so nothing the run does
// not take its base URL from may be listed.
func pathsUnder(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := append(credkit.FlagValues(args, "--config"), "")
	selectedHome, selectedConfig := lastFlag(args, "--home"), lastFlag(args, "--config")
	var out []string
	listIf := func(run bool, f string) {
		if run && !slices.Contains(out, f) {
			out = append(out, f)
		} else if !run {
			holdFile(f)
		}
	}
	for _, home := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if consulted, err := config.ConsultedPaths(); err == nil {
			for _, f := range consulted {
				holdFile(f)
			}
		}
		for _, c := range configs {
			run := home == selectedHome && c == selectedConfig
			if files, err := config.ReadPaths(c); err == nil {
				for _, f := range files {
					listIf(run, f)
				}
			}
			if stores, err := config.StorePaths(c); err == nil {
				for _, f := range stores {
					// A store records the base URL its token was issued for, which
					// decides nothing about this run's base URL.
					if data, err := os.ReadFile(filepath.Clean(f)); err == nil && bytes.Contains(data, []byte("base_url")) {
						cliutil.HoldConfigFile(data)
					} else {
						listIf(run, f)
					}
				}
			}
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".keeper-cli", "profiles.json"))
	}
	return out
}

// lastFlag is the value the parser takes for a flag: the last before any "--".
func lastFlag(args []string, name string) string {
	if end := slices.Index(args, "--"); end >= 0 {
		args = args[:end]
	}
	values := credkit.FlagValues(args, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func holdFile(path string) {
	if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
		cliutil.HoldConfigFile(opened.Data)
	}
}
