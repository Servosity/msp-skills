// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"datto-rmm-pp-cli/internal/cli"
	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/config"
	"datto-rmm-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Datto RMM takes an API key and API secret key (DATTO_RMM_API_KEY,
// DATTO_RMM_API_SECRET_KEY, config.toml or the 0600 credentials.toml store),
// exchanged at <API host>/auth/oauth/token for a bearer token, or a pre-minted
// DATTO_RMM_TOKEN. Every one of them, and a minted token, goes only to the API
// base URL this process uses: DATTO_RMM_BASE_URL, DATTO_RMM_API_URL or
// DATTO_RMM_PLATFORM (showBaseURL), else the file, else the default platform.
// The store and config.toml are credential SOURCES: their own writes go outside
// the kit (cliutil.AtomicWriteCredentialFile, a declared exemption) and they are
// named in Files or held there. --password and --proxy-settings-password carry
// a site's proxy password, sent as the body field password. The token response's
// access_token, a site's proxy password and the apiAccessKey / apiSecretKey pair
// POST /v2/user/resetApiKeys issues are response fields (LEDGER-NOTES: they
// print withheld; rotate keys in the Datto RMM web UI). A saved run profile's
// base-url and credential-flag values are sources too (holdProfiles).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"DATTO_RMM_API_KEY",
		"DATTO_RMM_API_SECRET_KEY",
		"DATTO_RMM_TOKEN",
		"DATTO_RMM_MCP_HTTP_TOKEN",
	},
	URLEnv:           []string{"DATTO_RMM_API_URL", "DATTO_RMM_FEEDBACK_ENDPOINT"},
	Files:            credentialFiles,
	ConfigFields:     []string{"rmm_api_key", "rmm_api_secret_key", "rmm_token", "auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords:    []string{"rmm_api_key", "rmm_api_secret_key", "rmm_token", "auth_header", "access_token", "refresh_token", "client_secret"},
	CredentialFlags:  []string{"--password", "--proxy-settings-password"},
	CredentialFields: []string{"password"},
	ResponseFields:   []string{"access_token", "refresh_token", "password", "apiAccessKey", "apiSecretKey"},
	MCPArguments:     []string{"password", "proxy-settings-password", "proxy_settings_password"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "DATTO_RMM_BASE_URL",
		Default:   dattoDefaultBase,
	},
	Origins: []credkit.Origin{
		// Every held value goes only to the API base URL this process uses.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:DATTO_RMM_MCP_HTTP_TOKEN", Allowed: nil},
		// A saved run profile's values are never used to send a request (no
		// command has a --base-url flag), so what they carry is sent nowhere.
		{Source: "profile:base-url", Allowed: nil},
		{Source: "profile:value", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:client_id", Reason: "a stored token's binding: 32 hex digits, fingerprints of the API host and key (config.DattoTokenBinding), never the key"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:DATTO_RMM_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:DATTO_RMM_API_URL", Reason: "the API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:DATTO_RMM_PLATFORM", Reason: "name of the Datto RMM regional platform"},
		{Source: "env:DATTO_RMM_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:DATTO_RMM_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:DATTO_RMM_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:DATTO_RMM_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:DATTO_RMM_DATA_DIR", Reason: "directory path"},
		{Source: "env:DATTO_RMM_STATE_DIR", Reason: "directory path"},
		{Source: "env:DATTO_RMM_CACHE_DIR", Reason: "directory path"},
		{Source: "env:DATTO_RMM_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:DATTO_RMM_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:DATTO_RMM_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:DATTO_RMM_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; held through URLEnv, and a held value sent to it is refused by the kit's transport"},
		{Source: "env:DATTO_RMM_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash (journal-credential-hardening)"},
		{Source: "env:DATTO_RMM_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:DATTO_RMM_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:DATTO_RMM_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:DATTO_RMM_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:DATTO_RMM_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:DATTO_RMM_USER_AGENT", Reason: "User-Agent string"},
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

// dattoDefaultBase is the API base URL when nothing names one (config.go).
const dattoDefaultBase = "https://merlot-api.centrastage.net/api"

// credentialFiles runs inside Start, before anything can print. It returns the
// config files this run reads, in config.Load's order, because the kit takes
// the base URL (and with it every credential's allowed origin) from the first
// file naming one. An explicit --config the kit reads itself and lets decide;
// DATTO_RMM_CONFIG comes next, as in config.Load; the last --home picks the
// default paths, as pflag does. Every other file a --home or --config could
// mean, and every credentials.toml store, is only held (credkit.HoldBody): its
// values cannot print, and its base_url cannot become an allowed origin
// (config.Load takes no routing from a store). Flag values come only from
// credkit.FlagValues, which counts every occurrence. The run profile store
// under each home, and the legacy one, is held too (holdProfiles), never
// returned.
func credentialFiles(args []string) []string {
	showBaseURL()
	holdPipedToken(args)
	holdStdinBody(args)
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
		holdProfiles(filepath.Join(home, ".datto-rmm-cli", "profiles.json")) // profile.go legacyProfileStorePath
	}
	return used
}

// runFiles is what config.Load reads under the current home: DATTO_RMM_CONFIG,
// else config.toml and, only when that is absent, the legacy config. The
// credential store is held, never returned.
func runFiles() []string {
	var files []string
	if explicit := os.Getenv("DATTO_RMM_CONFIG"); explicit != "" {
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
// a value. --credential-ref names a reference,
// not a secret); --password and --proxy-settings-password are held under
// profile:value.
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
					if json.Unmarshal(v.value, &s) != nil || s == "" {
						continue
					}
					switch {
					case baseURLName(v.key):
						credkit.HoldURL("profile:base-url", s)
					case slices.Contains(profileCredentialFlags, "--"+strings.ReplaceAll(strings.ToLower(v.key), "_", "-")):
						credkit.Hold("profile:value", s)
					}
				}
			}
		}
	}
}

// profileCredentialFlags is Declarations.CredentialFlags: the checker's rule 1
// lets only credkit.Start read that variable (TestDeclarationCopiesMatch
// keeps the two the same).
var profileCredentialFlags = []string{"--password", "--proxy-settings-password"}

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

// baseURLShown records that showBaseURL set the variable.
var baseURLShown bool

// showBaseURL tells the kit which origin "base_url" means when the environment
// chose the host by platform: config.Load treats DATTO_RMM_API_URL and
// DATTO_RMM_PLATFORM as an explicit base URL (handfix
// platform-routes-every-request), and the kit reads only BaseURL.Env. The kit
// reads the variable right after Files returns; AfterStart clears it, so nothing
// the program does later sees a change. An unknown platform shows nothing: every
// command refuses it (config.Load).
func showBaseURL() {
	if os.Getenv("DATTO_RMM_BASE_URL") != "" {
		return
	}
	if base, err := config.DattoPlatformBaseURL(); err == nil && base != "" {
		baseURLShown = os.Setenv("DATTO_RMM_BASE_URL", base) == nil
	}
}

// AfterStart undoes showBaseURL. Both mains call it right after credkit.Start.
func AfterStart() {
	if baseURLShown {
		baseURLShown = false
		_ = os.Unsetenv("DATTO_RMM_BASE_URL")
	}
}

// holdPipedToken holds what `auth set-token` will read before anything can print
// (contract section 1, stdin included): the parser reads the command line before
// the command reads the pipe, and its errors repeat what it was given. The pipe
// is held whole and line by line, and the program gets the same bytes on a new
// os.Stdin. The word is matched anywhere on the line, which holds more, never less.
func holdPipedToken(args []string) {
	if !slices.Contains(args, "set-token") {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return // a terminal: the command refuses it and reads nothing
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, stdinHoldLimit))
	for _, v := range append(strings.Split(string(b), "\n"), string(b)) {
		if strings.TrimSpace(v) != "" {
			credkit.Hold("stdin", v)
		}
	}
	if err != nil {
		b = nil // a partial read is no token: the command refuses an empty one
	}
	refillStdin(b)
}

// stdinHoldLimit is larger than any token auth set-token accepts, so the command
// still sees an oversized pipe and refuses it, and bounded, so a pipe that never
// ends cannot fill memory first.
const stdinHoldLimit = 128 << 10

// holdStdinBody holds the credential fields of a --stdin request body before
// anything prints, previews and --dry-run included, and gives the program the
// same bytes on a new os.Stdin. It is not StdinFlags: those hold every line of a
// body, so a pretty-printed body stops the program (a line that is only "{"
// collides with public text, exit 78 with no message) and a one-line body
// withholds its own preview and any response that shares a piece of it. A body is
// a request, not a credential; its credential fields are, and credkit.HoldBody
// holds every string of one that is not a document, whose error may repeat it.
// feedback --stdin reads a note, not a body; import reads records as a body too.
func holdStdinBody(args []string) {
	if !hasStdinFlag(args) || feedbackStdin(args) {
		return
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		b = nil // a partial read is no body: the command refuses an empty one
	}
	credkit.HoldBody(b)
	refillStdin(b)
}

func hasStdinFlag(args []string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if name == "--stdin" || name == "-stdin" {
			return true
		}
	}
	return false
}

// refillStdin gives the program the bytes startup read on a new os.Stdin. If the
// pipe cannot be made os.Stdin is spent, and the command reads nothing and refuses.
func refillStdin(b []byte) {
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	os.Stdin = r
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
