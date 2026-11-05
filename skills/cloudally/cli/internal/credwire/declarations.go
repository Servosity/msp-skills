// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
	"cloudally-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// CloudAlly authenticates every request with the API application's client-id
// and client-secret headers plus a bearer access token. The token comes from
// CLOUDALLY_ACCESS_TOKEN, config.toml or the 0600 credentials.toml store, or is
// minted at run time by auth login (POST /v2/auth, /v2/auth/partner, with an
// email and a password), auth refresh (POST /v2/auth/refresh, with a refresh
// token) and auth use-account (POST .../partner-access-token). The use-account
// flow keeps the partner session aside in the state directory's
// auth-context.json. config.toml, credentials.toml and auth-context.json are
// credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWritePrivateFile) and they are named in Files, because the kit
// would withhold them by contract.
//
// The login emails are identifiers, not credentials: the manifest marks them
// non-sensitive, the program prints them masked, and the partner's own address
// and domain run through ordinary API results, which holding them would withhold.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CLOUDALLY_CLIENT_ID",
		"CLOUDALLY_CLIENT_SECRET",
		"CLOUDALLY_ACCESS_TOKEN",
		"CLOUDALLY_PASSWORD",
		"CLOUDALLY_PARTNER_PASSWORD",
		"CLOUDALLY_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		// config.toml and credentials.toml
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "password", "partner_password",
		// auth-context.json
		"partner_access_token", "partner_refresh_token",
		// profiles.json: a saved value under a credential flag's name (contract 1, item 8(c))
		"new-password", "partner-password", "reseller-password", "refresh-token", "storage-key", "storage-secret-key", "acc-cred", "base-url",
	},
	StoredRecords: []string{
		"auth_header", "access_token", "refresh_token", "client_id", "client_secret", "password", "partner_password",
		"partner_access_token", "partner_refresh_token",
	},
	CredentialFlags: []string{
		"--password", "--new-password", "--partner-password", "--reseller-password",
		"--refresh-token", "--storage-key", "--storage-secret-key", "--acc-cred",
	},
	StdinFlags:        []string{"--stdin", "--password-stdin"},
	CredentialHeaders: []string{"client-id", "client-secret"},
	CredentialFields: []string{
		"password", "newPassword", "partnerPassword", "resellerPassword", "refreshToken",
		"storageKey", "storageSecretKey", "accCred", "s3Key", "s3Secret",
	},
	ResponseFields: []string{
		"accessToken", "refreshToken", "password", "partnerPassword",
		"storageKey", "storageSecretKey", "s3Key", "s3Secret",
	},
	MCPArguments: []string{
		// mirrored CLI flags
		"password", "new-password", "partner-password", "reseller-password", "refresh-token",
		"storage-key", "storage-secret-key", "acc-cred",
		// cloudally_execute params, by their wire names
		"newPassword", "partnerPassword", "resellerPassword", "refreshToken", "accessToken",
		"storageKey", "storageSecretKey", "accCred", "s3Key", "s3Secret",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CLOUDALLY_BASE_URL",
		Default:   "https://api.cloudally.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API this process is configured for:
		// the login, refresh and partner-access-token calls are on that origin too.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:CLOUDALLY_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "env:CLOUDALLY_EMAIL", Reason: "the company account's login email: a user name the manifest marks non-sensitive and the program prints masked"},
		{Source: "env:CLOUDALLY_PARTNER_EMAIL", Reason: "the partner portal's login email: a user name the manifest marks non-sensitive and the program prints masked"},
		{Source: "config:email", Reason: "the company account's login email, a user name"},
		{Source: "config:partner_email", Reason: "the partner portal's login email, a user name"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:scope", Reason: "auth-context.json: which token is active, partner or account"},
		{Source: "config:account_id", Reason: "auth-context.json: the customer account id the active token acts in"},
		{Source: "config:partner_token_expiry", Reason: "auth-context.json: an expiry time"},
		{Source: "config:refresh_expires_at", Reason: "auth-context.json: an expiry time"},
		{Source: "config:updated_at", Reason: "auth-context.json: a time stamp"},
		{Source: "env:CLOUDALLY_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CLOUDALLY_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CLOUDALLY_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CLOUDALLY_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CLOUDALLY_DATA_DIR", Reason: "directory path"},
		{Source: "env:CLOUDALLY_STATE_DIR", Reason: "directory path"},
		{Source: "env:CLOUDALLY_CACHE_DIR", Reason: "directory path"},
		{Source: "env:CLOUDALLY_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CLOUDALLY_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CLOUDALLY_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:CLOUDALLY_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CLOUDALLY_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:CLOUDALLY_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CLOUDALLY_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CLOUDALLY_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CLOUDALLY_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CLOUDALLY_USER_AGENT", Reason: "User-Agent string"},
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
		{Source: "env:USERNAME", Reason: "the Windows account name, compared with a file's owner"},
		{Source: "env:SYSTEMROOT", Reason: "the Windows system directory"},
		{Source: "env:GODEBUG", Reason: "Go runtime settings"},
	},
}

// credentialFiles runs inside Start, before anything can print. It returns the
// files this run reads as its own, the config the program will really use FIRST,
// because the kit takes the base URL (and with it every credential's allowed
// origin) from the first file naming one: an explicit --config or
// CLOUDALLY_CONFIG wins, as it does in config.Load, and the last --home decides
// the default paths, as pflag does. Every other file any --config or --home could
// mean, and the run profiles, are only held (credkit.HoldBody): their values
// cannot print, and their base_url cannot become an allowed origin. No flag
// arity is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("CLOUDALLY_CONFIG")
	}
	home := ""
	if len(homes) > 0 {
		home = homes[len(homes)-1]
	}
	var used, other []string
	underHome(home, func() { used = runFiles(explicit) })
	for _, h := range append([]string{""}, homes...) {
		underHome(h, func() {
			other = append(other, runFiles(explicit)...)
			other = append(other, runFiles("")...)
			for _, c := range configs {
				other = append(other, runFiles(c)...)
			}
			other = append(other, profileStores()...)
		})
	}
	return append(used, holdOnly(other, used)...)
}

// underHome runs fn with the path resolver relocated to home, as the root
// command's --home does; an empty home is the resolver's own default.
func underHome(home string, fn func()) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return // the run refuses this --home before it reads a file
	}
	defer restore()
	fn()
}

// runFiles is every credential file a run with this explicit config (or none)
// reads, in the order config.Load prefers them: the config file (the legacy
// config when no explicit one was named and the default is absent), the
// credential stores, the use-account sidecar and the run profile stores.
func runFiles(explicit string) []string {
	var files []string
	if explicit != "" {
		files = append(files, explicit)
		if store, err := cliutil.CredentialsFilePathForConfig(explicit); err == nil {
			files = append(files, store)
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
		files = append(files, store)
	}
	if dir, err := cliutil.StateDir(); err == nil {
		files = append(files, filepath.Join(dir, "auth-context.json"))
	}
	return files
}

// holdOnly holds every file in files that is not one of used, with
// credkit.HoldBody, and returns the ones it could not read, for the kit to read
// as credential files (one it cannot read withholds everything, as contract 1
// requires). The run profile stores come through here, never as a used file:
// profiles.json is nested JSON, which the kit's file reader answers by holding
// every string in it, so a profile named "p" or a saved --json true would refuse
// every later start. HoldBody reads it by name instead: every value under a
// credential flag's name or base-url, every duplicate included.
func holdOnly(files, used []string) []string {
	var unreadable []string
	seen := map[string]bool{}
	for _, f := range used {
		seen[filepath.Clean(f)] = true
	}
	for _, f := range files {
		if f == "" || seen[filepath.Clean(f)] {
			continue
		}
		seen[filepath.Clean(f)] = true
		b, err := readRegular(f)
		switch {
		case err == nil:
			credkit.HoldBody(b)
		case !errors.Is(err, fs.ErrNotExist):
			unreadable = append(unreadable, f)
		}
	}
	return unreadable
}

// profileStores is the run profile store under the current path resolution,
// and the legacy one.
func profileStores() []string {
	var out []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".cloudally-cli", "profiles.json"))
	}
	return out
}

// readRegular reads a regular file; anything else (a directory, a device, a
// pipe that would block) is an error the caller treats as unreadable.
func readRegular(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	return os.ReadFile(filepath.Clean(path))
}

// compactStdinBody puts a --stdin JSON body on one line before the kit reads it.
// Declarations.StdinFlags holds a body whole and line by line, and a line of "{"
// alone collides with public text and refuses the start, so a body from `jq .`
// would end the run. The credentials stay held: StdinFlags does that. Anything
// that is not one JSON document stays as it came.
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
