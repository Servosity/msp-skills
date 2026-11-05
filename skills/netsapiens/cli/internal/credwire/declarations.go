// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"netsapiens-pp-cli/internal/cli"
	"netsapiens-pp-cli/internal/cliutil"
	"netsapiens-pp-cli/internal/config"
	"netsapiens-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
)

// Declarations is what the connector holds and where each credential may go.
// NetSapiens authenticates with a bearer API key or JWT, from the environment,
// config.toml or the 0600 credentials.toml store.
//
// ResponseFields, CredentialFields and MCPArguments are the credential members
// spec.json documents (read off it with cliutil.IsSecretField, the walker the
// client uses for any other name). The PIN fields among them are Identifiers on
// the response side only: see cliutil.pinWords.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"NETSAPIENS_API_KEY",
		"NETSAPIENS_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"api_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	CredentialFlags: []string{
		"--access-token", "--client-secret", "--connection-sip-registration-password",
		"--device-provisioning-password", "--device-sip-registration-password",
		"--email-send-smtp-password", "--global-one-time-pass", "--idtoken",
		"--meeting-passcode", "--meeting-registration-id", "--passcode", "--password", "--voicemail-login-pin",
	},
	// Every generated write takes a JSON body on --stdin. A body pasted from a read
	// carries credentials; the kit holds it whole before anything can preview it.
	StdinFlags: []string{"--stdin"},
	CredentialFields: []string{
		"SipSipAuthKey", "access_token", "auth_pass", "authentication_key", "client-secret",
		"client_secret", "connection-sip-registration-password", "data_token",
		"device-provisioning-password", "device-sip-registration-password",
		"email-send-smtp-password", "global-one-time-pass", "idtoken", "key", "meeting-registration-id", "refresh", "leader_pin",
		"meeting-passcode", "participants_pin", "passcode", "password", "pin", "pwd_hash",
		"refresh_token", "smtp_pwd", "speaker_pin", "subscriber_pin", "token", "voicemail-login-pin",
	},
	ResponseFields: []string{
		"SipSipAuthKey", "access_token", "auth_pass", "authentication_key", "client-secret",
		"client_secret", "connection-sip-registration-password", "data_token",
		"device-provisioning-password", "device-sip-registration-password",
		"email-send-smtp-password", "global-one-time-pass", "idtoken", "key", "meeting-registration-id", "refresh", "password",
		"pwd_hash", "refresh_token", "smtp_pwd", "token",
	},
	MCPArguments: []string{
		"SipSipAuthKey", "access-token", "access_token", "auth_pass", "authentication_key", "client-secret",
		"client_secret", "connection-sip-registration-password", "data_token",
		"device-provisioning-password", "device-sip-registration-password",
		"email-send-smtp-password", "global-one-time-pass", "idtoken", "key", "meeting-registration-id", "refresh", "leader_pin",
		"meeting-passcode", "participants_pin", "passcode", "password", "pin", "pwd_hash",
		"refresh_token", "smtp_pwd", "speaker_pin", "subscriber_pin", "token", "voicemail-login-pin",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "NETSAPIENS_BASE_URL",
		Default:   "https://{server}/ns-api/v2",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the NetSapiens server this process is
		// configured for: a reseller's host is its own, so no hosted domain can be
		// named here. credentialFiles resolves it as the program will
		// (NETSAPIENS_BASE_URL, NETSAPIENS_SERVER, config) and shows it to the kit.
		// The connector's own origin pin (netsapiens-credential-goes-only-to-the-
		// configured-server) narrows every request and redirect hop to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:NETSAPIENS_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:server", Reason: "the NetSapiens server host name (template_vars.server), printed in every API URL"},
		{Source: "config:template_vars.server", Reason: "the NetSapiens server host name, printed in every API URL"},
		{Source: "response:leader_pin", Reason: "a short conference PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:participants_pin", Reason: "a short conference PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:speaker_pin", Reason: "a short conference PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:pin", Reason: "a short PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:subscriber_pin", Reason: "a short subscriber PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:voicemail-login-pin", Reason: "a short voicemail PIN, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:meeting-passcode", Reason: "a short meeting passcode, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "response:passcode", Reason: "a short passcode, replaced by <redacted> where named; held, it would withhold every unit with those digits as a token"},
		{Source: "env:NETSAPIENS_SERVER", Reason: "the NetSapiens server host name, printed in every API URL"},
		{Source: "env:NETSAPIENS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:NETSAPIENS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:NETSAPIENS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:NETSAPIENS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:NETSAPIENS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:NETSAPIENS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:NETSAPIENS_DATA_DIR", Reason: "directory path"},
		{Source: "env:NETSAPIENS_STATE_DIR", Reason: "directory path"},
		{Source: "env:NETSAPIENS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:NETSAPIENS_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:NETSAPIENS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:NETSAPIENS_RETRY_MAX", Reason: "a retry count"},
		{Source: "env:NETSAPIENS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:NETSAPIENS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:NETSAPIENS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:NETSAPIENS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:NETSAPIENS_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:NETSAPIENS_MCP_UPLOAD_ROOT", Reason: "directory path; no tool in this release takes a file"},
		{Source: "env:NETSAPIENS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:NETSAPIENS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:NETSAPIENS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:NETSAPIENS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:NETSAPIENS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:NETSAPIENS_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:NETSAPIENS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
	URLEnv: []string{"NETSAPIENS_FEEDBACK_ENDPOINT"},
}

// credentialFiles runs inside Start, before anything can print. It shows the kit
// this process's NetSapiens server, holds the configs Load only consults, and
// names every file a credential may be read from. The command line
// is read the way the program reads it: the CLI's own command tree finds the
// command and pflag parses its flags, so a value that looks like a flag
// (--notes --home=x) stays the value it is. When the command line does not
// parse (an unknown command or flag, the MCP server's own flags, a --home the
// program would refuse), every file it could mean is named, the environment's
// first: holding a file the program does not read is safe, missing the one it
// reads is not.
func credentialFiles(args []string) []string {
	configPath, home, parsed := cobraPathFlags(args)
	if !parsed {
		configPath, home = lastOf(credkit.FlagValues(args, "--config")), lastOf(credkit.FlagValues(args, "--home"))
	}
	showBaseURL(configPath, home)
	startHome = home
	if parsed {
		if files, ok := configFilesUnder(configPath, home); ok {
			return files
		}
	}
	var out []string
	add := func(files []string, _ bool) {
		for _, f := range files {
			if !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	add(configFilesUnder("", ""))
	for _, c := range credkit.FlagValues(args, "--config") {
		add(configFilesUnder(c, ""))
	}
	for _, h := range credkit.FlagValues(args, "--home") {
		add(configFilesUnder("", h))
	}
	return out
}

// savedBaseURL is NETSAPIENS_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
	startHome       string // the --home this command line means
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// kit reads Declarations.BaseURL, whose default is a template it cannot resolve,
// and a NetSapiens server can be any host, so credentials could go nowhere
// otherwise. It reads the variable right after Files returns; AfterStart puts the
// variable back, so nothing the program does later sees a change. When nothing
// resolves, the variable is left alone and the kit allows no origin: a request
// carrying a credential is refused.
func showBaseURL(configPath, home string) {
	base := configuredBase(configPath, home)
	if base == "" {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("NETSAPIENS_BASE_URL")
	baseURLShown = os.Setenv("NETSAPIENS_BASE_URL", base) == nil
}

// configuredBase is the base URL config.Load resolves under this command line,
// with its {placeholders} filled in, or "" when one is left or Load refuses.
func configuredBase(configPath, home string) string {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return ""
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil {
		return ""
	}
	base := cfg.BaseURL
	for k, v := range cfg.TemplateVars {
		base = strings.ReplaceAll(base, "{"+k+"}", v)
	}
	if strings.ContainsAny(base, "{}") {
		return ""
	}
	return base
}

// AfterStart is the statement after credkit.Start in both mains, before anything
// can print. It holds the request body a --body-json flag carries and the run
// profile stores with the kit's HoldBody, and puts NETSAPIENS_BASE_URL back as
// the process found it. The kit already holds the password and query values of
// the base URL it was shown. Held after Start, a value that occurs in the kit's
// public text is recorded and the program runs, rather than refusing to start.
func AfterStart() {
	for _, body := range credkit.FlagValues(os.Args[1:], "--body-json") {
		cliutil.HoldBody([]byte(body))
	}
	holdProfileStores(startHome)
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("NETSAPIENS_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("NETSAPIENS_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// cobraPathFlags is --config and --home as the CLI parses args: Cobra's
// Find (or Traverse) picks the command, then its flag set parses the rest,
// as ExecuteC does. ok is false when either step fails or the command
// leaves its flags unparsed.
func cobraPathFlags(args []string) (configPath, home string, ok bool) {
	root := cli.RootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	var cmd *cobra.Command
	var rest []string
	var err error
	if root.TraverseChildren {
		cmd, rest, err = root.Traverse(args)
	} else {
		cmd, rest, err = root.Find(args)
	}
	if err != nil || cmd == nil || cmd.DisableFlagParsing {
		return "", "", false
	}
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()
	if err := cmd.ParseFlags(rest); err != nil {
		return "", "", false
	}
	configPath, cerr := cmd.Flags().GetString("config")
	home, herr := cmd.Flags().GetString("home")
	return configPath, home, cerr == nil && herr == nil
}

// holdProfileStores holds every run profile store this run may read: a saved
// value under a credential flag's name is a source (cliutil.HoldProfileStoreFile).
// It is not in Files: the kit reads a file there as a config, and nested JSON as
// one it cannot read precisely, holding every string, a profile name included.
func holdProfileStores(home string) {
	if restore, err := cliutil.SetHomeOverride(home); err == nil {
		if dir, err := cliutil.ConfigDir(); err == nil {
			cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(userHome, ".netsapiens-cli", "profiles.json"))
	}
}

// configFilesUnder is config.ReadPaths(configPath) with --home set to home, as
// the program sets it before it loads the config, and holds every value of
// config.ConsultedPaths. ok is false when the program would refuse that --home
// or the path cannot be resolved.
func configFilesUnder(configPath, home string) ([]string, bool) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return nil, false
	}
	defer restore()
	if consulted, err := config.ConsultedPaths(); err == nil {
		for _, p := range consulted {
			if opened, err := cliutil.ReadOpenedConfig(p); err == nil { // the read Load does
				cliutil.HoldConfigFile(opened.Data)
			}
		}
	}
	files, err := config.ReadPaths(configPath)
	if err != nil {
		return nil, false
	}
	return files, true
}
