// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	"knowbe4-pp-cli/internal/cliutil"
	"knowbe4-pp-cli/internal/config"
	"knowbe4-pp-cli/internal/credkit"
	"knowbe4-pp-cli/internal/userevents"
)

// Declarations is what the connector holds and where each credential may go.
//
// Credentials: the Reporting API key (KNOWBE4_API_KEY, and config.toml
// api_key, auth_header, [headers] and the 0600 credentials.toml store), the
// User Event API key (KNOWBE4_USER_EVENT_API_KEY), the MCP HTTP bearer and
// the MCP server's TLS key. The Reporting API documents no response field
// that carries a credential and no MCP tool argument that is one, so
// ResponseFields and MCPArguments are empty. No flag carries a credential
// (CredentialFlags). auth set-token reads its key from stdin with no flag and
// holds it where it reads it; feedback --stdin is free text, never a
// credential, so StdinFlags is empty. The run profile store is a source too,
// held in credentialFiles.
var Declarations = credkit.Declarations{
	SecretEnv: []string{"KNOWBE4_API_KEY", "KNOWBE4_USER_EVENT_API_KEY", "KNOWBE4_MCP_HTTP_TOKEN"},
	Files:     credentialFiles,
	// The config and credential-store keys that carry a Reporting credential.
	ConfigFields:  []string{"api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "KNOWBE4_BASE_URL",
		Default:   "https://us.api.knowbe4.com/v1",
	},
	Origins: []credkit.Origin{
		// The Reporting credentials go only to a Reporting API host. The
		// program's base URL is https://{region}.api.knowbe4.com/v1 until its
		// region resolves, a template the kit cannot read as an origin, so the
		// five regional hosts are named; the connector's own origin pin
		// (internal/cli/knowbe4_guard.go) narrows every request to the one
		// origin the program resolved. Never the User Event host, a --deliver
		// webhook, the feedback endpoint or a redirect's host.
		{Source: "*", Allowed: []string{
			"base_url",
			"https://us.api.knowbe4.com",
			"https://eu.api.knowbe4.com",
			"https://ca.api.knowbe4.com",
			"https://uk.api.knowbe4.com",
			"https://de.api.knowbe4.com",
		}},
		// The User Event key goes only to the User Event host the events
		// client sends to, never to the Reporting host.
		{Source: "env:KNOWBE4_USER_EVENT_API_KEY", Allowed: userEventOrigins()},
		// The MCP HTTP bearer authenticates callers of this server, and the
		// TLS key is this server's own; neither is sent anywhere.
		{Source: "env:KNOWBE4_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the Reporting API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "config:template_vars", Reason: "the region the base URL template resolves with (us, eu, ca, uk, de)"},
		{Source: "env:KNOWBE4_BASE_URL", Reason: "the Reporting API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:KNOWBE4_USER_EVENT_BASE_URL", Reason: "the User Event API base URL; userevents.New refuses one with user info or a query"},
		{Source: "env:KNOWBE4_REGION", Reason: "a KnowBe4 region code (us, eu, ca, uk, de)"},
		{Source: "env:KNOWBE4_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:KNOWBE4_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:KNOWBE4_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:KNOWBE4_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:KNOWBE4_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:KNOWBE4_DATA_DIR", Reason: "directory path"},
		{Source: "env:KNOWBE4_STATE_DIR", Reason: "directory path"},
		{Source: "env:KNOWBE4_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:KNOWBE4_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:KNOWBE4_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:KNOWBE4_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:KNOWBE4_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:KNOWBE4_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:KNOWBE4_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:KNOWBE4_MCP_HTTP_ALLOWED_ORIGINS", Reason: "browser origins the MCP HTTP server accepts, not a credential"},
		{Source: "env:KNOWBE4_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:KNOWBE4_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; the outbound guard refuses any held value sent to it"},
		{Source: "env:KNOWBE4_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:KNOWBE4_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:KNOWBE4_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:KNOWBE4_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:KNOWBE4_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent session label the learn journal hashes"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// userEventOrigins is the one origin the User Event client sends its key to,
// resolved as userevents.New resolves it (KNOWBE4_USER_EVENT_BASE_URL, else the
// region's host). None when it does not resolve: New refuses then too.
func userEventOrigins() []string {
	base, err := userevents.ResolveBaseURL()
	if err != nil {
		return nil
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil
	}
	for _, b := range []byte(u.Host) {
		if b >= 0x80 { // the kit takes an origin's host in ASCII only
			return nil
		}
	}
	return []string{u.Scheme + "://" + u.Host}
}

// credentialFiles names every config and credentials file the command line
// and the environment could mean. The first is the config file the program
// reads when no --config is given (the kit takes the base URL from it; an
// explicit --config is read by the kit itself and wins), so a discovered
// file's base_url does not become the origin when the program reads another.
// Every other file is held: under the default home and each --home, the
// legacy path, and the credentials store beside each --config. Holding a file
// the program does not read is safe; missing the one it reads is not.
func credentialFiles(args []string) []string {
	var files []string
	add := func(p string, err error) {
		if err == nil && p != "" && !slices.Contains(files, p) {
			files = append(files, p)
		}
	}
	homes := credkit.FlagValues(args, "--home")
	last := ""
	if n := len(homes); n > 0 {
		last = homes[n-1]
	}
	if env := os.Getenv("KNOWBE4_CONFIG"); env != "" {
		add(env, nil)
	} else {
		underHome(last, func() { add(readConfigPath()) })
	}
	for _, h := range append([]string{""}, homes...) {
		underHome(h, func() {
			add(primaryConfigPath())
			add(config.LegacyConfigPath())
			add(cliutil.CredentialsFilePath())
			holdProfileStores()
		})
	}
	for _, c := range append(credkit.FlagValues(args, "--config"), os.Getenv("KNOWBE4_CONFIG")) {
		if c != "" {
			add(cliutil.CredentialsFilePathForConfig(c))
		}
	}
	return files
}

// underHome runs fn with the program's --home override set to h ("" is the
// default home); an invalid --home is one the program refuses before reading
// any file.
func underHome(h string, fn func()) {
	restore, err := cliutil.SetHomeOverride(h)
	if err != nil {
		return
	}
	defer restore()
	fn()
}

func primaryConfigPath() (string, error) {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// readConfigPath is the default config file the program reads: the primary
// one, else the legacy path when only that exists (config.Load).
func readConfigPath() (string, error) {
	p, err := primaryConfigPath()
	if err != nil {
		return "", err
	}
	if _, serr := os.Stat(p); serr != nil {
		if legacy, lerr := config.LegacyConfigPath(); lerr == nil {
			if _, e := os.Stat(legacy); e == nil {
				return legacy, nil
			}
		}
	}
	return p, nil
}

// holdProfileStores holds the run profile stores (profiles.json, current and
// legacy): every value under a declared credential name, and every value under
// base-url as a URL (item 8c).
func holdProfileStores() {
	var stores []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		stores = append(stores, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		stores = append(stores, filepath.Join(home, ".knowbe4-cli", "profiles.json"))
	}
	for _, p := range stores {
		data, err := fs.ReadFile(os.DirFS(filepath.Dir(p)), filepath.Base(p))
		if err != nil {
			continue
		}
		credkit.HoldBody(data)
		var store struct {
			Profiles map[string]struct {
				Values map[string]string `json:"values"`
			} `json:"profiles"`
		}
		if json.Unmarshal(data, &store) != nil {
			continue
		}
		for _, prof := range store.Profiles {
			if v := prof.Values["base-url"]; v != "" {
				credkit.HoldURL("profile:base-url", v)
			}
		}
	}
}
