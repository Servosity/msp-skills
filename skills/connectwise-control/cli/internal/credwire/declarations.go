// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"connectwise-control-pp-cli/internal/cli"
	"connectwise-control-pp-cli/internal/cliutil"
	"connectwise-control-pp-cli/internal/config"
	"connectwise-control-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
)

// Declarations is what the connector holds and where each credential may go.
// ConnectWise Control authenticates with HTTP Basic: a user name and a
// password, from the environment, config.toml or the 0600 credentials.toml
// store.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"CONNECTWISE_CONTROL_USERNAME",
		"CONNECTWISE_CONTROL_PASSWORD",
		"CONNECTWISE_CONTROL_MCP_HTTP_TOKEN",
	},
	// A feedback endpoint names a server, so the variable is an identifier
	// below, but a URL can carry a password or query value: those are held.
	URLEnv: []string{"CONNECTWISE_CONTROL_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"control_username",
		"control_password",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"control_username",
		"control_password",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	// security save-user sets a user's password with --password, in the
	// password field of a --stdin body or as an MCP argument.
	CredentialFlags:  []string{"--password"},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"password"},
	// The access grant: sessions get-access-token answers with accessToken,
	// and the join url repeats it. Both stay withheld everywhere.
	ResponseFields: []string{"accessToken", "url"},
	// The MCP mirror of auth set-credentials takes the user name and password
	// as arguments, and security save-user takes a password.
	MCPArguments: []string{"control_username", "control_password", "password"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "CONNECTWISE_CONTROL_BASE_URL",
		// A template, not an origin: every instance has its own host, which
		// the config or CONNECTWISE_CONTROL_BASE_URL names, so with neither set
		// the kit allows the credential to no host (kaseya-vsa ships the same
		// shape). The generated config's placeholder default is refused by the
		// client itself (no-credential-to-placeholder-host), and the origin
		// pin (api-origin-pinned) narrows every request to the configured host,
		// redirects included.
		Default: "https://{instance}.screenconnect.com",
	},
	Origins: []credkit.Origin{
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:CONNECTWISE_CONTROL_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Config keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "env:CONNECTWISE_CONTROL_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:CONNECTWISE_CONTROL_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:CONNECTWISE_CONTROL_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:CONNECTWISE_CONTROL_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:CONNECTWISE_CONTROL_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:CONNECTWISE_CONTROL_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_CONTROL_DATA_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_CONTROL_STATE_DIR", Reason: "directory path"},
		{Source: "env:CONNECTWISE_CONTROL_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:CONNECTWISE_CONTROL_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:CONNECTWISE_CONTROL_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:CONNECTWISE_CONTROL_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:CONNECTWISE_CONTROL_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_CONTROL_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_CONTROL_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:CONNECTWISE_CONTROL_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:CONNECTWISE_CONTROL_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:CONNECTWISE_CONTROL_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:CONNECTWISE_CONTROL_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_CONTROL_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_CONTROL_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:CONNECTWISE_CONTROL_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:CONNECTWISE_CONTROL_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
// what no file read sees: the auth set-credentials positionals, a --deliver
// webhook and the run profile stores. The command line is read the way the
// program reads it: the CLI's own command tree finds the command and pflag
// parses its flags, so a value that looks like a flag stays the value it is.
// The files are then resolved by config.Load's own rules (config.ReadPaths)
// under that --home, in Load's order, so the kit takes the base URL from the
// file the program uses. When the command line does not parse (an unknown
// command or flag, the MCP server's own flags, a --home the program would
// refuse), every file it could mean is named, the environment's first:
// holding a file the program does not read is safe, missing the one it reads
// is not.
func credentialFiles(args []string) []string {
	holdSetCredentialsArgs(args)
	holdURLInputs(args)
	holdProfileStores(args)
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

// holdURLInputs holds what the kit's BaseURL read cannot see: a base URL with
// no scheme (ops:secret@host), which it reads as an opaque one, and
// --deliver, whose webhook a preview names and profile save keeps.
func holdURLInputs(args []string) {
	credkit.HoldURL("url:CONNECTWISE_CONTROL_BASE_URL", os.Getenv("CONNECTWISE_CONTROL_BASE_URL"))
	for _, v := range credkit.FlagValues(args, "--deliver") {
		if !strings.HasPrefix(v, "file:") {
			credkit.HoldURL("url:deliver", strings.TrimPrefix(v, "webhook:"))
		}
	}
}

// holdSetCredentialsArgs holds every word after set-credentials on the command
// line but the flag names the command declares, flags' values and words of
// nothing but dashes included: an extra word held is safe, a credential
// missed is not. The user name and the password are both held.
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
		credkit.Hold("config:control_password", a)
	}
}

// holdProfileStores holds what a run profile store carries under a credential
// name or base-url, every duplicate key included. The kit is not given the
// file: it is nested JSON, so the kit would hold every string in it, and a
// profile's plain "true" would refuse every start. Each store the program
// can read is held: the config directory's under every --home the command
// line names and under none, and the legacy one. Only a regular file is
// read: a pipe named profiles.json would hang startup.
func holdProfileStores(args []string) {
	homes := append([]string{""}, credkit.FlagValues(args, "--home")...)
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(dir)
		}
		restore()
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".connectwise-control-cli"))
	}
}

func holdProfileStore(dir string) {
	store := os.DirFS(dir)
	if info, err := fs.Stat(store, "profiles.json"); err != nil || !info.Mode().IsRegular() {
		return
	}
	data, err := fs.ReadFile(store, "profiles.json")
	if err != nil {
		return
	}
	credkit.HoldBody(data) // values under a credential name, every duplicate; every word of a file that does not parse
	// Tokens, not a decoded tree, which keeps only the last member of a
	// duplicate key. A file that does not parse is held up to where it stops
	// here, and whole by HoldBody above.
	type frame struct{ object, atKey bool }
	var stack []frame
	key := ""
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		top := len(stack) - 1
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				stack = append(stack, frame{object: d == '{', atKey: d == '{'})
				continue
			}
			stack = stack[:top]
			if top > 0 && stack[top-1].object {
				stack[top-1].atKey = true
			}
			continue
		}
		if top >= 0 && stack[top].atKey {
			key, _ = tok.(string)
			stack[top].atKey = false
			continue
		}
		if s, ok := tok.(string); ok && (key == "base-url" || key == "base_url") {
			credkit.HoldURL("profile:base-url", s)
		}
		if top >= 0 && stack[top].object {
			stack[top].atKey = true
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
