// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"io"
	"os"
	"path/filepath"
	"slices"

	"crowdstrike-pp-cli/internal/cli"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/config"
	"crowdstrike-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

// Declarations is what the connector holds and where each credential may go.
// CrowdStrike authenticates with an API client pair (FALCON_CLIENT_ID,
// FALCON_CLIENT_SECRET) from the environment, auth login's flags, config.toml
// or the 0600 credentials.toml, traded at the Falcon cloud's /oauth2/token for
// a bearer token; fleet sync trades the same pair for one token per child CID
// (member_cid). auth set-token reads a bearer token on stdin.
//
// Every credential is bound to the one Falcon cloud this run sends to
// (base_url). The token URL the program derives (client.ResolveCrowdStrikeTokenURL:
// the base URL's scheme and host plus /oauth2/token) is on that same origin, so
// no other origin is declared: a CROWDSTRIKE_TOKEN_URL on another host is
// refused by the kit (fail closed, .review/followups.md).
//
// --stdin is in StdinFlags for the endpoint commands that read a request body
// on stdin. The kit holds each line of a declared stdin whole, so feedback
// --stdin, which reads a note for the local ledger, has its ledger write
// withheld (.review/followups.md).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"FALCON_CLIENT_ID",
		"FALCON_CLIENT_SECRET",
		"CROWDSTRIKE_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{"CROWDSTRIKE_TOKEN_URL", "CROWDSTRIKE_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	CredentialFlags: []string{"--client-id", "--client-secret"},
	StdinFlags:      []string{"--stdin"},
	// The token request form (client.ClientCredentialsTokenForm, the refresh
	// grant). The rest name a credential a --deliver or feedback URL could carry.
	// client_id is not listed: it is the first field of the sorted form, which
	// the kit's key-value reading takes as one line, holding the rest of the form
	// (grant_type=client_credentials) as its value (.review/kit-findings.md). Its
	// value is held from every source it comes from: FALCON_CLIENT_ID, --client-id
	// and the client_id of config.toml and credentials.toml.
	CredentialFields: []string{"client_secret", "refresh_token", "access_token", "token", "password", "api_key", "apikey"},
	// What the token endpoint answers with (the automatic mint, the refresh
	// grant, auth login and fleet sync's member_cid mint), and the credential
	// fields any other Falcon response can carry, in either spelling.
	ResponseFields: []string{
		"access_token", "accessToken", "refresh_token", "refreshToken", "id_token", "client_secret", "clientSecret",
		"password", "passwd", "secret", "api_key", "apiKey", "private_key", "privateKey", "uninstall_token",
	},
	// auth login is hidden from MCP; a mirrored tool still never takes the pair.
	MCPArguments: []string{"client_id", "client_secret", "client-id", "client-secret", "access_token", "refresh_token", "token", "password"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CROWDSTRIKE_BASE_URL",
		Default:   "https://api.crowdstrike.com",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the Falcon cloud this run is configured
		// for: its API and its /oauth2/token. The connector's own origin pin
		// (api-origin-pinned) narrows every request to it too, redirects included,
		// and stored credentials are applied only to the cloud they were recorded
		// for (stored-credential-bound-to-its-origin).
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server, and the TLS
		// key is this server's own; neither is sent anywhere.
		{Source: "env:CROWDSTRIKE_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
		// A proxy's password goes to that proxy only: the kit's transport adds its origin.
		{Source: "proxy-userinfo", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the Falcon cloud's API address; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held by config.holdURLSettings"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "env:CROWDSTRIKE_BASE_URL", Reason: "the Falcon cloud's API address; a credential inside it is held through BaseURL"},
		{Source: "env:CROWDSTRIKE_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:CROWDSTRIKE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:CROWDSTRIKE_OAUTH_SCOPE", Reason: "an OAuth scope name"},
		{Source: "env:CROWDSTRIKE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CROWDSTRIKE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CROWDSTRIKE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CROWDSTRIKE_HOME", Reason: "root directory for config, data, state and cache; Files resolves it"},
		{Source: "env:CROWDSTRIKE_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:CROWDSTRIKE_DATA_DIR", Reason: "directory path"},
		{Source: "env:CROWDSTRIKE_STATE_DIR", Reason: "directory path"},
		{Source: "env:CROWDSTRIKE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:CROWDSTRIKE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CROWDSTRIKE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CROWDSTRIKE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CROWDSTRIKE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CROWDSTRIKE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CROWDSTRIKE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CROWDSTRIKE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CROWDSTRIKE_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash (handfix journal-credential-hardening)"},
		{Source: "env:CROWDSTRIKE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
		{Source: "env:HTTP_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:HTTPS_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:ALL_PROXY", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:NO_PROXY", Reason: "a list of hosts"},
		{Source: "env:http_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:https_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:all_proxy", Reason: "a proxy URL; the kit holds a credential inside it"},
		{Source: "env:no_proxy", Reason: "a list of hosts"},
	},
}

// credentialFiles is every credential file the program will read. The command
// line is read the way the program reads it: the CLI's own command tree finds
// the command and pflag parses its flags, so a value that looks like a flag
// stays the value it is. The files are then resolved by config.Load's own rules
// (config.ReadPaths) under that --home. When the command line does not parse
// (an unknown command or flag, the MCP server's own flags, a --home the
// program would refuse), every file it could mean is named, the environment's
// first: holding a file the program does not read is safe, missing the one it
// reads is not. It also holds each run-profile store under each --home it
// names, and what each URL setting carries (holdURLSettings).
func credentialFiles(args []string) []string {
	holdURLSettings()
	files := filesFor(args)
	holdConfigURLs(files)
	return files
}

// holdURLSettings holds the password and every query value of each URL setting
// in the environment with the kit's own reader, credkit.HoldURL, which also
// reads them in an address with a typo (https//u:pw@h, u:pw@h). The kit's
// BaseURL and URLEnv readers hold the well-formed ones; the program prints a
// malformed one back in a connection error and in doctor.
func holdURLSettings() {
	for _, v := range []string{os.Getenv("CROWDSTRIKE_BASE_URL"), os.Getenv("CROWDSTRIKE_TOKEN_URL"), os.Getenv("CROWDSTRIKE_FEEDBACK_ENDPOINT")} {
		if v != "" {
			credkit.HoldURL("userinfo", v)
		}
	}
}

// holdConfigURLs holds what the base_url and token_url of each file in paths
// carry, read with credkit.HoldURL. It reads a regular file only, from the
// descriptor it judged, so a FIFO or a device named as a config cannot stall
// startup; a file that does not parse is held whole by the kit itself.
func holdConfigURLs(paths []string) {
	for _, path := range paths {
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			continue
		}
		if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
			var urls struct {
				BaseURL  string `toml:"base_url"`
				TokenURL string `toml:"token_url"`
			}
			if data, err := io.ReadAll(io.LimitReader(f, maxHeldConfigBytes)); err == nil && toml.Unmarshal(data, &urls) == nil {
				for _, v := range []string{urls.BaseURL, urls.TokenURL} {
					if v != "" {
						credkit.HoldURL("userinfo", v)
					}
				}
			}
		}
		_ = f.Close()
	}
}

// maxHeldConfigBytes bounds what holdConfigURLs reads of one file; a config is
// a few hundred bytes.
const maxHeldConfigBytes = 8 << 20

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
