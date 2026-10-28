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

	"kaseya-vsa-pp-cli/internal/cli"
	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/config"
	"kaseya-vsa-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
)

// Declarations is what the connector holds and where each credential may go.
// VSA 10 authenticates with HTTP Basic: a token id and a token secret, from
// the environment, config.toml or the 0600 credentials.toml store.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"KASEYA_VSA_TOKEN_ID",
		"KASEYA_VSA_TOKEN_SECRET",
		"KASEYA_VSA_MCP_HTTP_TOKEN",
	},
	// These name a server or endpoint, so the variable is an identifier below,
	// but a URL can carry a password or query value: those are held.
	URLEnv: []string{"KASEYA_VSA_SERVER", "KASEYA_VSA_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"vsa_token_id",
		"vsa_token_secret",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"vsa_token_id",
		"vsa_token_secret",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	// A webhook's custom headers authenticate it to its receiver. They arrive
	// on --headers, in the Headers field of a --stdin body or an MCP argument,
	// and back from VSA 10 when it returns the webhook. A body pasted from a
	// webhook read carries its SecretKey too. credkit.HoldBody holds these
	// from a body the startup read cannot reach (internal/cli/kaseya_stdin_hold.go).
	CredentialFlags:  []string{"--headers"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"Headers", "SecretKey"},
	// SecretKey is the webhook signing key VSA 10 returns, and Headers its custom
	// headers (the vendor response fields that authenticate anything).
	// Custom-field values and the Windows product key are the operator's own
	// data, not credentials: the store strips them (store-never-keeps-secrets)
	// and an explicit read returns them.
	ResponseFields: []string{"SecretKey", "Headers"},
	// The MCP mirror of auth set-credentials takes them as arguments, and
	// kaseya-vsa_execute takes a webhook's Headers.
	MCPArguments: []string{"vsa_token_id", "vsa_token_secret", "Headers", "SecretKey"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "KASEYA_VSA_BASE_URL",
		Default:   "https://{server}/api/v3",
	},
	Origins: []credkit.Origin{
		// The built-in base URL is a template the kit cannot resolve, so the
		// hosted domain is named here and KASEYA_VSA_BASE_URL (or a config
		// base_url) names any other server. The connector's own origin pin
		// (api-origin-pinned) narrows every request to the one configured
		// host, redirects included.
		{Source: "*", Allowed: []string{"base_url", "https://*.vsax.net"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:KASEYA_VSA_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Config keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:skip_tls_verify", Reason: "boolean switch"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:server", Reason: "the VSA server host name (template_vars.server), printed in every API URL"},
		{Source: "env:KASEYA_VSA_SERVER", Reason: "the VSA server host name, printed in every API URL"},
		{Source: "env:KASEYA_VSA_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:KASEYA_VSA_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:KASEYA_VSA_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:KASEYA_VSA_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:KASEYA_VSA_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:KASEYA_VSA_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_VSA_DATA_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_VSA_STATE_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_VSA_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:KASEYA_VSA_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:KASEYA_VSA_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:KASEYA_VSA_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:KASEYA_VSA_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:KASEYA_VSA_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:KASEYA_VSA_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:KASEYA_VSA_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:KASEYA_VSA_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:KASEYA_VSA_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside a URL is held by the outbound guard when sent"},
		{Source: "env:KASEYA_VSA_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:KASEYA_VSA_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:KASEYA_VSA_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:KASEYA_VSA_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:KASEYA_VSA_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
// reads is not.
func credentialFiles(args []string) []string {
	holdSetCredentialsArgs(args)
	holdURLInputs(args)
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

// holdURLInputs holds what the kit's URLEnv read cannot see: a URL with no
// scheme (ops:secret@host), which it reads as an opaque one, and --deliver,
// whose webhook a preview names and profile save keeps.
func holdURLInputs(args []string) {
	credkit.HoldURL("url:KASEYA_VSA_SERVER", os.Getenv("KASEYA_VSA_SERVER"))
	credkit.HoldURL("url:KASEYA_VSA_FEEDBACK_ENDPOINT", os.Getenv("KASEYA_VSA_FEEDBACK_ENDPOINT"))
	credkit.HoldURL("url:KASEYA_VSA_BASE_URL", os.Getenv("KASEYA_VSA_BASE_URL"))
	for _, v := range credkit.FlagValues(args, "--deliver") {
		cli.HoldDeliverURL(v)
	}
}

// holdSetCredentialsArgs holds every word after set-credentials on the command
// line but the flag names the command declares, flags' values and words of
// nothing but dashes included: an extra word held is safe, a token missed is not.
func holdSetCredentialsArgs(args []string) {
	i := slices.Index(args, "set-credentials")
	if i < 0 {
		return
	}
	root := cli.RootCmd()
	cmd, _, err := root.Find(args)
	if err == nil && cmd != nil {
		cmd.InitDefaultHelpFlag() // Cobra adds it only when it parses
	}
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
		credkit.Hold("config:vsa_token_secret", a)
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
// as the program sets it before it loads the config, and holds every value of
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
