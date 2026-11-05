// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"backup-radar-pp-cli/internal/cli"
	"backup-radar-pp-cli/internal/cliutil"
	"backup-radar-pp-cli/internal/config"
	"backup-radar-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Backup Radar takes two keys: the ScalePad v3 key (header x-api-key, from
// BACKUP_RADAR_API_KEY, config.toml or the 0600 credentials.toml store), sent
// only to the configured v3 base URL; and the Backup Radar v2 key (header
// ApiKey, from BACKUP_RADAR_V2_API_KEY), sent only to the fixed v2 origin. The
// store and config.toml are credential SOURCES: their own writes go outside the
// kit (cliutil.AtomicWriteCredentialFile, a declared exemption) and they are
// named in Files or held there. The spec has no credential flag, body field,
// response field or MCP argument: client_id, --client-id and the metadata key
// "key" are data. A saved run profile's base-url is a credential source too
// (holdProfiles), held from every profiles.json the run could read.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"BACKUP_RADAR_API_KEY",
		"BACKUP_RADAR_V2_API_KEY",
		"BACKUP_RADAR_MCP_HTTP_TOKEN",
	},
	Files:             credentialFiles,
	ConfigFields:      []string{"radar_api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords:     []string{"radar_api_key", "auth_header", "access_token", "refresh_token", "client_secret"},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"x-api-key", "ApiKey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "BACKUP_RADAR_BASE_URL",
		Default:   "https://api.scalepad.com/backup-radar/v3",
	},
	Origins: []credkit.Origin{
		// The v3 key, and every other held value, goes only to the v3 base URL
		// this process is configured with.
		{Source: "*", Allowed: []string{"base_url"}},
		// The v2 key goes only to the fixed v2 origin (client.baseURLForRequest).
		{Source: "env:BACKUP_RADAR_V2_API_KEY", Allowed: []string{"https://api.backupradar.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:BACKUP_RADAR_MCP_HTTP_TOKEN", Allowed: nil},
		// A saved run profile's base-url is never used to send a request (no
		// command has a --base-url flag), so what it carries is sent nowhere:
		// neither its own host nor the configured base URL is an origin for it.
		{Source: "profile:base-url", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:client_id", Reason: "an OAuth client identifier field the generated config carries; Backup Radar authenticates with keys only, and client_id is data here (a retire filter)"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:BACKUP_RADAR_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:BACKUP_RADAR_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:BACKUP_RADAR_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:BACKUP_RADAR_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:BACKUP_RADAR_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:BACKUP_RADAR_DATA_DIR", Reason: "directory path"},
		{Source: "env:BACKUP_RADAR_STATE_DIR", Reason: "directory path"},
		{Source: "env:BACKUP_RADAR_CACHE_DIR", Reason: "directory path"},
		{Source: "env:BACKUP_RADAR_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:BACKUP_RADAR_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:BACKUP_RADAR_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:BACKUP_RADAR_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a held value sent to it is refused by the kit's transport"},
		{Source: "env:BACKUP_RADAR_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash (journal-credential-hardening)"},
		{Source: "env:BACKUP_RADAR_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:BACKUP_RADAR_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:BACKUP_RADAR_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:BACKUP_RADAR_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:BACKUP_RADAR_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_NO_AUTO_REFRESH", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:BACKUP_RADAR_USER_AGENT", Reason: "User-Agent string"},
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
// config files this run reads, in config.Load's order, because the kit takes
// the base URL (and with it the v3 key's allowed origin) from the first file
// naming one. An explicit --config the kit reads itself and lets decide;
// BACKUP_RADAR_CONFIG comes next, as in config.Load; the last --home picks the
// default paths, as pflag does. Every other file a --home or --config could
// mean, and every credentials.toml store, is only held (credkit.HoldBody): its
// values cannot print, and its base_url cannot become an allowed origin
// (config.Load takes no routing from a store). Flag values come only from
// credkit.FlagValues, which counts every occurrence. The run profile store
// under each home, and the legacy one, is held too (holdProfiles), never
// returned.
func credentialFiles(args []string) []string {
	if feedbackStdin(args) {
		cliutil.StashFeedbackStdin()
	} else {
		cliutil.CompactStdinBody(args)
	}
	homes := credkit.FlagValues(args, "--home")
	var used []string
	for i, home := range append([]string{""}, homes...) {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles()
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfiles(filepath.Join(dir, "profiles.json")) // profile.go profileStorePath
		}
		restore()
		if i == len(homes) {
			used = files
			continue
		}
		for _, f := range files {
			holdFile(f)
		}
	}
	for _, cfg := range credkit.FlagValues(args, "--config") {
		holdStore(cliutil.CredentialsFilePathForConfig(cfg))
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfiles(filepath.Join(home, ".backup-radar-cli", "profiles.json")) // profile.go legacyProfileStorePath
	}
	return used
}

// runFiles is what config.Load reads under the current home: BACKUP_RADAR_CONFIG,
// else config.toml and, only when that is absent, the legacy config. The
// credential store is held, never returned.
func runFiles() []string {
	var files []string
	if explicit := os.Getenv("BACKUP_RADAR_CONFIG"); explicit != "" {
		files = append(files, explicit)
		holdStore(cliutil.CredentialsFilePathForConfig(explicit))
	} else if dir, err := cliutil.ConfigDir(); err == nil {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if legacy, err := config.LegacyConfigPath(); err == nil {
			if _, err := os.Stat(primary); err != nil {
				files = append(files, legacy)
			} else {
				holdFile(legacy)
			}
		}
	}
	holdStore(cliutil.CredentialsFilePath())
	return files
}

func holdStore(path string, err error) {
	if err == nil {
		holdFile(path)
	}
}

// holdFile holds what a credential or config file carries, through the kit's
// own reader.
func holdFile(path string) {
	if data, ok := readRegular(path); ok {
		credkit.HoldBody(data)
	}
}

// holdProfiles holds what a saved run profile store carries as a credential:
// the password and every query value of each value under base-url, through the
// kit's own URL reader (credkit.HoldURL), under source profile:base-url, which
// Origins sends nowhere. The store is read as profile.go's loadProfileStore
// reads it (encoding/json into profiles.<name>.values, struct keys matched in
// any case), but member by member, so every duplicate of a key is held, not
// only the member encoding/json keeps. What a store that stops parsing carries
// before the error is held; the program refuses such a store before it prints
// a value. The CLI has no credential flag (--credential-ref names a reference,
// not a secret), so base-url is the only profile value name held.
func holdProfiles(path string) {
	data, ok := readRegular(path)
	if !ok {
		return
	}
	for _, top := range members(data) {
		if !strings.EqualFold(top.key, "profiles") {
			continue
		}
		for _, profile := range members(top.value) {
			for _, field := range members(profile.value) {
				if !strings.EqualFold(field.key, "values") {
					continue
				}
				for _, v := range members(field.value) {
					var s string
					if baseURLName(v.key) && json.Unmarshal(v.value, &s) == nil && s != "" {
						credkit.HoldURL("profile:base-url", s)
					}
				}
			}
		}
	}
}

// baseURLName reports a profile value name that is base-url in any case, or
// spelled with an underscore: holding a URL's password costs nothing when the
// name only resembles the flag's.
func baseURLName(name string) bool {
	return strings.EqualFold(strings.ReplaceAll(name, "_", "-"), "base-url")
}

type member struct {
	key   string
	value json.RawMessage
}

// members is every member of the JSON object b, in order, each duplicate key
// included, as far as b parses. Anything but an object has none.
func members(b []byte) []member {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out []member
	for dec.More() {
		k, err := dec.Token()
		key, ok := k.(string)
		var v json.RawMessage
		if err != nil || !ok || dec.Decode(&v) != nil {
			break
		}
		out = append(out, member{key, v})
	}
	return out
}

// readRegular reads path only when it is a regular file, so a FIFO cannot hang
// startup. The path is one of the connector's own config, store and profile
// paths, read in its directory.
func readRegular(path string) ([]byte, bool) {
	dir, name := filepath.Split(filepath.Clean(path))
	fsys := os.DirFS(filepath.Clean(dir))
	info, err := fs.Stat(fsys, name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := fs.ReadFile(fsys, name)
	return data, err == nil
}

// feedbackStdin reports a run of `feedback --stdin`, as Cobra resolves the
// command line, so a flag value that reads "feedback" is not mistaken for it.
func feedbackStdin(args []string) bool {
	cmd, _, err := cli.RootCmd().Find(args)
	if err != nil || cmd.Name() != "feedback" || cmd.Parent() == nil || cmd.Parent().Parent() != nil {
		return false
	}
	if err := cmd.ParseFlags(args); err != nil {
		return false
	}
	on, err := cmd.Flags().GetBool("stdin")
	return err == nil && on
}
