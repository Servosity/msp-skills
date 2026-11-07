// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"os"
	"path/filepath"
	"slices"

	"microsoft-graph-pp-cli/internal/cli"
	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/config"
	"microsoft-graph-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
)

// entraLoginOrigin is the one host `auth login` posts the client credentials
// to (cli/auth_login.go entraLoginURLFormat).
const entraLoginOrigin = "https://login.microsoftonline.com"

// Declarations is what the connector holds and where each credential may go.
// Microsoft Graph takes a bearer token: minted outside this program
// (MICROSOFT_GRAPH_TOKEN, `auth set-token`) or by `auth login` from an Entra
// app registration's client id and secret. The 0600 credential store and
// config.toml are credential SOURCES: their own writes go outside the kit
// (cliutil.AtomicWriteCredentialFile) and they are named in Files, because the
// kit would withhold them by contract.
//
// feedback --stdin is deliberately not in StdinFlags: it reads a note for the
// feedback ledger, never a request body or a credential, and the kit holds
// each line of a declared stdin whole, so every ledger write would be
// withheld. `auth set-token` reads its token from stdin itself and holds it
// with credkit.Hold the moment it is read.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"MICROSOFT_GRAPH_TOKEN",
		"MICROSOFT_GRAPH_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"MICROSOFT_GRAPH_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "graph_token", "access_token", "refresh_token", "client_secret",
	},
	StoredRecords: []string{
		"auth_header", "graph_token", "access_token", "refresh_token", "client_secret",
	},
	// --client-id stays held as the old layer held it (graph-clientid-is-public):
	// only the response field clientId, an object id `apps consent` prints, is
	// public.
	CredentialFlags: []string{"--client-secret", "--client-id"},
	CredentialFields: []string{
		"client_secret", "password", "access_token", "refresh_token", "id_token", "secretText",
	},
	// What Graph or Entra can answer with that authenticates anything: a token
	// answer, an application password (secretText), a user's or a temporary
	// access pass, a BitLocker recovery key (key), a device's activation lock
	// bypass code. clientId is not here (graph-clientid-is-public).
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret", "accessToken", "refreshToken",
		"secretText", "password", "passwordBase64", "temporaryAccessPass", "key", "activationLockBypassCode",
	},
	// No tool declares one today; the generic names are listed so a reprint
	// that adds one is covered.
	MCPArguments: []string{"client_secret", "password", "access_token", "refresh_token", "id_token", "token", "secretText"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "MICROSOFT_GRAPH_BASE_URL",
		Default:   "https://graph.microsoft.com/v1.0",
	},
	Origins: []credkit.Origin{
		// A token goes only to the Graph base the program will really use. The
		// connector's own origin pin (api-origin-pinned) narrows every request,
		// redirects and nextLink pages included, to that same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// auth login: the client credentials go to the Entra token endpoint
		// only (auth-login-pinned-and-held), which refuses any redirect too.
		{Source: "flag:--client-secret", Allowed: []string{entraLoginOrigin}},
		{Source: "flag:--client-id", Allowed: []string{entraLoginOrigin}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:MICROSOFT_GRAPH_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the Graph base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "the app registration's client id, public: Graph answers and every consent URL repeat it (graph-clientid-is-public)"},
		{Source: "env:MICROSOFT_GRAPH_BASE_URL", Reason: "the Graph base URL; a credential inside it is held through BaseURL"},
		{Source: "env:MICROSOFT_GRAPH_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv and credkit.HoldURL"},
		{Source: "env:MICROSOFT_GRAPH_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:MICROSOFT_GRAPH_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:MICROSOFT_GRAPH_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:MICROSOFT_GRAPH_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:MICROSOFT_GRAPH_HOME_TENANT", Reason: "the MSP's own tenant id or domain, which transcripts compare a token's tid with"},
		{Source: "env:MICROSOFT_GRAPH_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:MICROSOFT_GRAPH_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:MICROSOFT_GRAPH_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:MICROSOFT_GRAPH_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:MICROSOFT_GRAPH_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:MICROSOFT_GRAPH_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:MICROSOFT_GRAPH_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:MICROSOFT_GRAPH_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:MICROSOFT_GRAPH_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio)"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
	},
}

// credentialFiles runs inside Start, before anything can print. It holds what
// the URL settings and each run-profile store carry, and names every file a
// credential may be read from under any --home and --config the command line
// can mean: holding a file the run does not load is safe, missing the one it
// loads is not. The run's own config files come first (as Cobra parses the
// command line; the environment's when it cannot), because the kit takes the
// first base_url it finds in a listed file as the one origin credentials may
// go to. A later file whose base_url differs can only make the kit refuse
// a send the program's own pin (api-origin-pinned) already confines to the
// run's base URL: it fails closed, never open.
func credentialFiles(args []string) []string {
	holdURLSettings()
	return pathsUnder(args)
}

// holdURLSettings holds the password and every query value of each URL setting
// with the kit's own reader, which also reads them in an address with a typo.
// The kit's BaseURL and URLEnv readers hold the well-formed ones; the program
// prints a malformed one back in a connection error and doctor's base_url.
func holdURLSettings() {
	for _, name := range []string{"MICROSOFT_GRAPH_BASE_URL", "MICROSOFT_GRAPH_FEEDBACK_ENDPOINT"} {
		if v := os.Getenv(name); v != "" {
			credkit.HoldURL("userinfo", v)
		}
	}
}

func pathsUnder(args []string) []string {
	homes := append([]string{""}, credkit.FlagValues(args, "--home")...)
	configs := append([]string{""}, credkit.FlagValues(args, "--config")...)
	var out []string
	add := func(files []string, err error) {
		if err != nil {
			return
		}
		for _, f := range files {
			if f != "" && !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	under := func(home string, each func()) {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			return // the run refuses this --home before it loads anything
		}
		defer restore()
		each()
	}
	if configPath, home, ok := cobraPathFlags(args); ok {
		under(home, func() { add(config.ReadPaths(configPath)) })
	}
	for _, home := range homes {
		under(home, func() {
			for _, c := range configs {
				add(config.ReadPaths(c))
			}
			add(config.ConsultedPaths())
		})
	}
	for _, home := range homes {
		under(home, func() {
			for _, c := range configs {
				add(config.StorePaths(c))
			}
			if dir, err := cliutil.ConfigDir(); err == nil {
				holdProfileStore(filepath.Join(dir, "profiles.json"))
			}
		})
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".microsoft-graph-cli", "profiles.json"))
	}
	return out
}

// cobraPathFlags is --config and --home as the CLI parses args: Cobra's
// Find (or Traverse) picks the command, then its flag set parses the rest,
// as ExecuteC does, so a flag's value that looks like --home is not one. ok is
// false when either step fails or the command leaves its flags unparsed (the
// MCP server's command line, a command line the CLI refuses): then the
// environment's own config comes first, as the MCP server reads it.
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

// holdProfileStore holds what a run-profile store carries
// (cliutil.HoldRunProfileStore). A store is read here and not named in Files:
// the kit would hold every string in a nested file, and saved flag values such
// as --select would then withhold ordinary output.
func holdProfileStore(path string) {
	if data, err := os.ReadFile(filepath.Clean(path)); err == nil {
		cliutil.HoldRunProfileStore(data)
	}
}
