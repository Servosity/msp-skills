// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"slices"
	"strings"

	"vsphere-pp-cli/internal/cli"
	"vsphere-pp-cli/internal/client"
	"vsphere-pp-cli/internal/cliutil"
	"vsphere-pp-cli/internal/config"
	"vsphere-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
)

// Declarations is what the connector holds and where each credential may go.
// vCenter authenticates with an SSO user and password, traded at login for a
// session id, from the environment, config.toml or the 0600 credentials.toml.
//
// The SSO user name is not a credential: vCenter's own answers repeat it. Every
// source of it is an Identifier below. One limit is the kit's, not a declaration:
// a Basic login holds the user:password pair whole, so output carrying 8 or more
// characters of the user name in a row is withheld after a login. That fails
// closed (.review kit notes; README Credential safety).
//
// feedback --stdin is deliberately not in StdinFlags: it reads a note for the
// feedback ledger, never a request body or a credential, and the kit holds each
// line of a declared stdin whole, so every ledger write would be withheld.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"VSPHERE_PASSWORD",
		"VSPHERE_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"password",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"password",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	// The session id on every data call, minted by the login.
	CredentialHeaders: []string{client.SessionHeader},
	// The VI/JSON login carries the password in its body. The rest name a
	// credential a --deliver or feedback URL could carry in its query.
	CredentialFields: []string{"password", "passwd", "token", "access_token", "refresh_token", "client_secret", "api_key", "apikey"},
	// What vCenter can answer with that authenticates anything or is licensed by
	// key: a session id is held where it is minted (client.mintSession), and a
	// licence key before licence exposure masks it.
	ResponseFields: []string{
		"password", "passwd", "secret", "secretAccessKey", "token", "access_token", "refresh_token",
		"client_secret", "clientSecret", "accessToken", "refreshToken", "apiKey", "api_key", "privateKey", "psk", "preSharedKey", "community", "licenseKey",
	},
	MCPArguments: []string{"password", "passwd", "token", "access_token", "refresh_token", "client_secret", "api_key", "apikey", "licenseKey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "VSPHERE_BASE_URL",
		Default:   "https://vcenter.example.com",
	},
	// The one other setting whose value is a URL: its password and query values
	// are held from startup, and it leaves a third-party child's environment.
	URLEnv: []string{"VSPHERE_FEEDBACK_ENDPOINT"},
	Origins: []credkit.Origin{
		// Every credential goes only to the one configured vCenter. The
		// connector's own origin pin (api-origin-pinned) narrows every request to
		// it too, redirects included.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer: callers send it to vsphere-mcp --transport http;
		// the program never sends it anywhere.
		{Source: "env:VSPHERE_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too (TestIdentifiersMatchConfigNotCredentials).
		{Source: "config:base_url", Reason: "the vCenter address; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:username", Reason: "the vCenter SSO account name, not a credential; vCenter's answers repeat it"},
		{Source: "basic-user", Reason: "the vCenter SSO account name, not a credential; vCenter's answers repeat it"},
		{Source: "env:VSPHERE_USERNAME", Reason: "the vCenter SSO account name, not a credential; vCenter's answers repeat it"},
		{Source: "env:VSPHERE_BASE_URL", Reason: "the vCenter address; a credential inside it is held through BaseURL"},
		{Source: "env:VSPHERE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:VSPHERE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:VSPHERE_TLS_FINGERPRINT", Reason: "the vCenter certificate's public fingerprint"},
		{Source: "env:VSPHERE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:VSPHERE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:VSPHERE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:VSPHERE_DATA_DIR", Reason: "directory path"},
		{Source: "env:VSPHERE_STATE_DIR", Reason: "directory path"},
		{Source: "env:VSPHERE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:VSPHERE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:VSPHERE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:VSPHERE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:VSPHERE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:VSPHERE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:VSPHERE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:VSPHERE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:VSPHERE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:VSPHERE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held at startup through URLEnv and credkit.HoldURL"},
		{Source: "env:VSPHERE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:VSPHERE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:VSPHERE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:VSPHERE_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:VSPHERE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio)"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles is every credential file the program will read, and holds
// the credentials of auth set-credentials, which arrive as positional
// arguments no flag scan sees. The command line is read the way the program
// reads it: the CLI's own command tree finds the command and pflag parses its
// flags, so a value that looks like a flag (--notes --home=x) stays the value
// it is. The files are then resolved by config.Load's own rules
// (config.ReadPaths) under that --home. When the command line does not parse
// (an unknown command or flag, the MCP server's own flags, a --home the
// program would refuse), every file it could mean is named, the environment's
// first: holding a file the program does not read is safe, missing the one it
// reads is not. It holds each run-profile store under each --home it names,
// and what the URL settings carry (holdURLSettings).
func credentialFiles(args []string) []string {
	holdURLSettings()
	holdSetCredentialsArgs(args)
	files := filesFor(args)
	config.HoldBaseURLs(files)
	return files
}

// holdURLSettings holds the password and every query value of each URL setting
// with the kit's own reader, credkit.HoldURL, which also reads them in an
// address with a typo (https//u:pw@h, u:pw@h). The kit's BaseURL and URLEnv
// readers hold the well-formed ones; the program prints a malformed one back
// in a connection error, a retry line and doctor's base_url.
func holdURLSettings() {
	if v := os.Getenv("VSPHERE_BASE_URL"); v != "" {
		credkit.HoldURL("userinfo", v)
	}
	if v := os.Getenv("VSPHERE_FEEDBACK_ENDPOINT"); v != "" {
		credkit.HoldURL("userinfo", v)
	}
}

func filesFor(args []string) []string {
	if configPath, home, ok := cobraPathFlags(args); ok {
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

// holdSetCredentialsArgs holds every word after set-credentials on the command
// line but the flag names the command declares, flags' values and leading
// dashes included: an extra word held is safe, a token missed is not.
func holdSetCredentialsArgs(args []string) {
	i := slices.Index(args, "set-credentials")
	if i < 0 {
		return
	}
	root := cli.RootCmd()
	cmd, _, err := root.Find(args)
	isFlag := func(a string) bool {
		if err != nil || cmd == nil {
			return false
		}
		if name, ok := strings.CutPrefix(a, "--"); ok {
			return cmd.Flag(name) != nil
		}
		return len(a) == 2 && (cmd.Flags().ShorthandLookup(a[1:]) != nil || cmd.InheritedFlags().ShorthandLookup(a[1:]) != nil)
	}
	words := args[i+1:]
	dashes := slices.Index(words, "--")
	for j, a := range words {
		if j == dashes {
			continue
		}
		if dashes < 0 || j < dashes {
			if name, v, ok := strings.Cut(a, "="); ok && isFlag(name) {
				a = v
			} else if isFlag(a) {
				continue
			}
		}
		if strings.Trim(a, "-") != "" {
			credkit.Hold("config:password", a)
		}
	}
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

// configFilesUnder is config.ReadPaths(configPath) with --home set to home,
// as the program sets it before it loads the config. ok is false when the
// program would refuse that --home or the path cannot be resolved.
func configFilesUnder(configPath, home string) ([]string, bool) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return nil, false
	}
	defer restore()
	holdRunProfiles()
	files, err := config.ReadPaths(configPath)
	if err != nil {
		return nil, false
	}
	return files, true
}

// holdRunProfiles holds what each run-profile store under the current --home can
// carry (cliutil.HoldRunProfileStore). A store is read here and not named in
// Files: the kit would hold every string in a nested file, and saved flag values
// such as --select would then withhold ordinary output. A value stays unheld only
// under a flag this CLI has and a run profile would apply.
func holdRunProfiles() {
	for _, path := range cli.RunProfileStorePaths() {
		data, _, err := cliutil.ReadFileWithLegacyFallback(path, "")
		if err == nil {
			cliutil.HoldRunProfileStore(data, cli.RunProfileFlagAllowed)
		}
	}
}
