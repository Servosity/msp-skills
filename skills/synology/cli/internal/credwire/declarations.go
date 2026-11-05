// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// DSM takes an account and password (with a one-time code or trusted-device
// token) in a login form and answers with a session id and SynoToken.
//
// device_id is deliberately not a declared request or response field: Active
// Backup answers and parameters use that name for a backup device's number, and
// a held number withholds every line that carries it. A trusted-device token is
// held where the login reads or mints it (dsm.Login, parseLogin).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"SYNOLOGY_PASSWORD",
		"SYNOLOGY_OTP_CODE",
		"SYNOLOGY_DEVICE_ID",
		"SYNOLOGY_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	// feedback --stdin is the one command that reads a body. The kit holds it whole and
	// line by line, so feedback refuses it (internal/cli/feedback.go).
	StdinFlags: []string{"--stdin"},
	ConfigFields: []string{
		"password", "otp_code", "device_id", "auth_header", "access_token", "refresh_token", "client_secret",
	},
	StoredRecords: []string{
		"password", "otp_code", "device_id", "auth_header", "access_token", "refresh_token", "client_secret",
		"token", "synotoken", "sid",
	},
	CredentialHeaders: []string{"X-SYNO-TOKEN"},
	// _sid and SynoToken are not fields here. Every read carries them first in its
	// query, and the kit reads a declared first field as a config line, so it would
	// hold the whole query and with it its 8-character pieces ("_devices" in
	// load_devices=true), which then withholds "DELETE FROM abb_devices" from SQLite.
	// Both are held already where they are minted or read: the login response
	// (ResponseFields), the session cache and the config.
	CredentialFields: []string{"passwd", "otp_code"},
	ResponseFields:   []string{"sid", "synotoken", "did", "passwd", "password", "access_token", "refresh_token", "client_secret"},
	MCPArguments: []string{
		"passwd", "password", "otp_code", "sid", "_sid", "synotoken", "token",
		"access_token", "refresh_token", "client_secret",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SYNOLOGY_BASE_URL",
		Default:   "https://nas.example.com:5001",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the NAS this process is bound to. A run
		// over several NAS units is one process per NAS (internal/cli
		// nas_process.go), so each NAS's password can only reach its own NAS.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server and is sent nowhere:
		// .invalid never resolves (RFC 2606). credkit_verify rule 5 reads Allowed: nil as
		// a credential with no origin and no ledger entry can waive it (.review/kit-findings.md).
		{Source: "env:SYNOLOGY_MCP_HTTP_TOKEN", Allowed: []string{"https://mcp-http-token.invalid"}},
		// A registered NAS's credential is known to every process, so output can
		// withhold it, and is sent nowhere from a process started for another NAS...
		{Source: "minted:other-nas", Allowed: nil},
		// ...and only to the NAS this process was started for, when the command logs
		// in to it (cli.dsmClient). The kit unions two declared sources.
		{Source: "minted:this-nas", Allowed: []string{"base_url"}},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the NAS address; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:account", Reason: "the DSM account name, not a credential; it is printed by auth status"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:logged_out", Reason: "boolean marker in the credential store"},
		{Source: "env:PP_NAS_PROCESS", Reason: "a marker the parent sets: this process was started for one NAS and starts no other"},
		{Source: "config:credentialsBaseURL", Reason: "the address credentials.toml was saved for, read and never written; not a credential"},
		{Source: "cookie:stay_login", Reason: "DSM's remember-me flag, 0 or 1 (cliutil.RememberMeCookie)"},
		{Source: "config:client_id", Reason: "an OAuth client id, public by design; the generated config carries it and DSM does not use it"},
		{Source: "env:SYNOLOGY_ACCOUNT", Reason: "the DSM account name, not a credential"},
		{Source: "env:SYNOLOGY_BASE_URL", Reason: "the NAS address; a credential inside it is held through BaseURL"},
		{Source: "env:SYNOLOGY_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SYNOLOGY_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SYNOLOGY_TLS_FINGERPRINT", Reason: "the NAS certificate's public fingerprint"},
		{Source: "env:SYNOLOGY_SKIP_TLS_VERIFY", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_CACHE_DIR", Reason: "directory path"},
		{Source: "env:SYNOLOGY_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SYNOLOGY_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:SYNOLOGY_DATA_DIR", Reason: "directory path"},
		{Source: "env:SYNOLOGY_STATE_DIR", Reason: "directory path"},
		{Source: "env:SYNOLOGY_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:SYNOLOGY_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SYNOLOGY_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SYNOLOGY_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; credentialFiles holds a password or credential query value in it, sent nowhere"},
		{Source: "env:SYNOLOGY_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_LEARN_SESSION", Reason: "session grouping label, stored only hashed"},
		{Source: "env:SYNOLOGY_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SYNOLOGY_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SYNOLOGY_MCP_CHILD", Reason: "marker a parent sets on a CLI child"},
		{Source: "env:SYNOLOGY_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:SYNOLOGY_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SYNOLOGY_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CODEX_THREAD_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CODEX_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "an agent-harness session label, stored only hashed"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:HTTP_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:HTTPS_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:ALL_PROXY", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:http_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:https_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "env:all_proxy", Reason: "proxy address; the kit holds its user and password, bound to that proxy"},
		{Source: "config:Source", Reason: "a run-time note of where a credential came from, never read from a file"},
		{Source: "config:Path", Reason: "the path this config was read from, never read from a file"},
		{Source: "config:Err", Reason: "a load error held in memory, never read from a file"},
		{Source: "config:AuthSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:CredentialSource", Reason: "a run-time label of where the credentials came from"},
		{Source: "config:configOwner", Reason: "a run-time note of the file owner"},
		{Source: "config:legacySourcePath", Reason: "the legacy path this config was read from"},
		{Source: "config:envOverrides", Reason: "names of the variables that overrode a value"},
		{Source: "config:variant", Reason: "a learn-entity spelling from the learn config"},
		{Source: "config:canonical", Reason: "a learn-entity name from the learn config"},
		{Source: "config:stopwords", Reason: "learn-search stop words"},
		{Source: "config:synonyms", Reason: "learn-search synonyms"},
	},
}

// credentialFiles runs inside Start, before anything can print. It binds this
// process to the NAS the command line names, and holds every registered NAS's credential variables and every session
// cache and run profile's credentials. It returns only the files this run really
// reads, because the kit takes the base URL (and with it every credential's allowed
// origin) from the first file naming one: the last --config, else SYNOLOGY_CONFIG,
// else the defaults under the last --home, as config.Load and pflag decide. Every
// other file any --config or --home could mean is only held: its values cannot
// print, and its base_url cannot become an allowed origin.
func credentialFiles(args []string) []string {
	for _, keyFile := range credkit.FlagValues(args, "--tls-key") {
		cliutil.HoldTLSKeyFile(keyFile)
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	envConfig := os.Getenv("SYNOLOGY_CONFIG")
	explicit, home := envConfig, ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if len(homes) > 0 {
		home = homes[len(homes)-1]
	}
	configDirs := cliutil.StartupKindDirs(cliutil.PathKindConfig, homes)
	dataDirs := cliutil.StartupKindDirs(cliutil.PathKindData, homes)
	var heldOnly []string
	for _, dir := range configDirs {
		heldOnly = append(heldOnly, filepath.Join(dir, "config.toml"))
	}
	for _, dir := range dataDirs {
		heldOnly = append(heldOnly, filepath.Join(dir, "credentials.toml"))
	}
	if legacy, err := config.LegacyConfigPath(); err == nil {
		heldOnly = append(heldOnly, legacy)
	}
	sessionDirs := append(slices.Clone(dataDirs), configDirs...)
	for _, cfg := range append(slices.Clone(configs), envConfig) {
		if cfg != "" {
			heldOnly = append(heldOnly, explicitFiles(cfg)...)
			sessionDirs = append(sessionDirs, filepath.Dir(cfg))
		}
	}
	for _, path := range heldOnly {
		cliutil.HoldConfigFilePath(path)
	}
	for _, dir := range sessionDirs {
		for _, path := range sessionFiles(dir) {
			holdJSONStore(path)
		}
	}
	for _, path := range profileStores(configDirs) {
		if data, err := cliutil.ReadRegularFile(path); err == nil {
			cliutil.HoldProfileStore(data)
		}
	}
	// credkit_verify does not read URLEnv yet (.review/kit-findings.md 10).
	cliutil.HoldURLCredentials(cliutil.SentNowhere, os.Getenv("SYNOLOGY_FEEDBACK_ENDPOINT"))
	nas := registeredNAS(homes)
	bindProcessToNAS(args, nas)
	for _, n := range nas {
		for _, suffix := range []string{"_PASSWORD", "_OTP_CODE", "_DEVICE_ID"} {
			cliutil.Hold("minted:other-nas", os.Getenv(n.EnvPrefix+suffix))
		}
	}
	if explicit != "" {
		return explicitFiles(explicit)
	}
	return defaultFiles(home)
}

// explicitFiles is what a run with this explicit config reads, in config.Load's order.
func explicitFiles(cfg string) []string {
	files := []string{cfg}
	if creds, err := cliutil.CredentialsFilePathForConfig(cfg); err == nil {
		files = append(files, creds)
	}
	return files
}

// defaultFiles is what a run with no explicit config reads under home ("" for no
// --home), in config.Load's order: the legacy config only when the default is absent.
func defaultFiles(home string) []string {
	var files []string
	if dir := cliutil.StartupKindDir(cliutil.PathKindConfig, home); dir != "" {
		primary := filepath.Join(dir, "config.toml")
		files = append(files, primary)
		if _, err := os.Stat(primary); os.IsNotExist(err) {
			if legacy, err := config.LegacyConfigPath(); err == nil {
				files = append(files, legacy)
			}
		}
	}
	if dir := cliutil.StartupKindDir(cliutil.PathKindData, home); dir != "" {
		files = append(files, filepath.Join(dir, "credentials.toml"))
	}
	return files
}

// profileStores is every run profile store a run can read (internal/cli/profile.go).
func profileStores(configDirs []string) []string {
	var out []string
	for _, dir := range configDirs {
		out = append(out, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".synology-cli", "profiles.json"))
	}
	return out
}

// registeredNAS reads fleet.json under each config directory: the one the run resolves
// to first, so it decides what a name binds to, and every other so a prefix any of them
// names has its variables held.
func registeredNAS(homes []string) []registered {
	winner := ""
	if len(homes) > 0 {
		winner = homes[len(homes)-1]
	}
	dirs := []string{cliutil.StartupKindDir(cliutil.PathKindConfig, winner)}
	for _, dir := range cliutil.StartupKindDirs(cliutil.PathKindConfig, homes) {
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	var out []registered
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, "fleet.json")) // #nosec G304 -- this CLI's own registry under its config dirs
		if err != nil {
			continue
		}
		var reg struct {
			NAS []registered `json:"nas"`
		}
		if json.Unmarshal(data, &reg) != nil {
			continue
		}
		for _, n := range reg.NAS {
			cliutil.HoldURL(cliutil.SentNowhere, n.URL) // fleet add refuses a password in the URL; a hand edit does not
			if n.EnvPrefix != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

type registered struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	EnvPrefix string `json:"env_prefix"`
}

// bindProcessToNAS makes the kit's base URL the NAS that fleet add names. It runs before
// the kit reads SYNOLOGY_BASE_URL, so it reads argv with no parser, and a command it
// misreads would send the environment's password to that address. So it acts only on
// the command line that starts with fleet add, whose one positional is a name and which
// sends only the prefix credentials of the NAS it names. Every other command with --nas
// starts a process for that NAS after its flags are parsed (cli.runForNAS).
func bindProcessToNAS(args []string, nas []registered) {
	args = afterPathFlags(args)
	if len(args) < 2 || args[0] != "fleet" || args[1] != "add" {
		return
	}
	if urls := credkit.FlagValues(args, "--url"); len(urls) > 0 && urls[len(urls)-1] != "" {
		_ = os.Setenv("SYNOLOGY_BASE_URL", urls[len(urls)-1])
		return
	}
	for _, a := range args[2:] { // fleet add <name> --check on a registered NAS
		if strings.HasPrefix(a, "-") {
			continue
		}
		for _, n := range nas {
			if strings.EqualFold(n.Name, a) && n.URL != "" {
				_ = os.Setenv("SYNOLOGY_BASE_URL", n.URL)
				return
			}
		}
	}
}

// afterPathFlags is args without the leading --home and --config, the root flags the
// documented lines put before a command. Anything else in front means no command is read.
func afterPathFlags(args []string) []string {
	for len(args) > 0 {
		name, _, inline := strings.Cut(args[0], "=")
		switch {
		case name != "--home" && name != "--config":
			return args
		case inline:
			args = args[1:]
		case len(args) < 2:
			return nil
		default:
			args = args[2:]
		}
	}
	return args
}

// sessionFileRE is this CLI's session cache names: dsm.SessionFileName, the legacy
// session.json, and the temp file a crash leaves of either. A --config can sit in any
// folder, so a file that only has "session" in its name is not one of them.
var sessionFileRE = regexp.MustCompile(`^\.?session(-.*-[0-9a-f]{8})?\.json(\.[0-9]+\.tmp)?$`)

// sessionFiles is dir's DSM session caches, a temp file a crash left included.
func sessionFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if sessionFileRE.MatchString(strings.ToLower(e.Name())) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// holdJSONStore holds a session cache at startup. The kit is not
// given these files: a nested file is outside its canonical subset, so it would hold
// every string in it, and a normal cache's account name in public text would refuse
// every start. The directory is shared by every NAS, so nothing here is sent anywhere
// until the session manager finds a record's origin is this NAS (client.loadFromDisk).
func holdJSONStore(path string) {
	if data, err := cliutil.ReadRegularFile(path); err == nil {
		cliutil.HoldJSONStore(data)
	}
}
