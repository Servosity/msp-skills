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
	"strings"

	"avanan-pp-cli/internal/avanansig"
	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/config"
	"avanan-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Its credentials are the application id and client secret or access key
// (env, config, credentials store, --app-id, --secret), the session token
// either handshake mints (config, store, auth set-token, x-av-token, the
// handshake answers), the [headers] table, and the MCP HTTP bearer.
var Declarations = credkit.Declarations{
	SecretEnv: []string{"AVANAN_APP_ID", "AVANAN_CLIENT_SECRET", "AVANAN_TOKEN", "AVANAN_MCP_HTTP_TOKEN"},
	Files:     credentialFiles,
	// app-id and secret: the credential flags as a run profile names them.
	ConfigFields:      []string{"app_id", "client_secret", "client_id", "token", "access_token", "refresh_token", "auth_header", "app-id", "secret"},
	StoredRecords:     []string{"app_id", "client_secret", "client_id", "token", "access_token", "refresh_token", "auth_header"},
	CredentialFlags:   []string{"--app-id", "--secret"},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"x-av-token", "x-av-app-id", "x-av-sig", "cloudinfra-external-client-id"},
	CredentialFields:  []string{"accessKey"},
	ResponseFields:    []string{"token"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "AVANAN_BASE_URL",
		Default:   "https://smart-api-production-1-us.avanan.net",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the base URL this run really uses, which
		// selectBaseURL gives the kit. It never reaches another regional host, a
		// feedback endpoint, a --deliver webhook or a host a redirect names.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:AVANAN_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "the session token's expiry timestamp"},
		{Source: "env:AVANAN_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:AVANAN_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:AVANAN_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:AVANAN_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:AVANAN_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:AVANAN_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:AVANAN_DATA_DIR", Reason: "directory path"},
		{Source: "env:AVANAN_STATE_DIR", Reason: "directory path"},
		{Source: "env:AVANAN_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:AVANAN_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:AVANAN_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:AVANAN_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:AVANAN_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:AVANAN_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:AVANAN_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:AVANAN_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:AVANAN_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:AVANAN_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:AVANAN_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:AVANAN_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:AVANAN_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:AVANAN_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles is every file a credential may be read from under any
// --config and --home the command line can mean. credkit.FlagValues reads
// past a "--" that pflag takes as a flag's value, and holding a file the run
// does not load is safe where missing the one it loads is not. The last
// value of a repeated flag comes first (pflag keeps the last), then the
// environment's choice, so the kit's base URL is the selected file's.
func credentialFiles(args []string) []string {
	takeFeedbackStdin(args)
	startupFiles = pathsUnder(args)
	selectBaseURL(args)
	return startupFiles
}

// startupFiles is what Files returned, for HoldStartupFiles; startupProfiles is
// every run-profile file under the same --home values.
var (
	startupFiles    []string
	startupProfiles []string
)

// restoreBaseURLEnv puts AVANAN_BASE_URL back after the kit has read it.
var restoreBaseURLEnv = func() {}

// selectBaseURL gives the kit the base URL the program will really use. The kit
// takes the environment's over any file's, and otherwise the first file with a
// base_url, which can be a discovered default this run does not load; and it
// cannot see auth login's flags. So for the kit's read alone, the variable carries
// the answer. HoldStartupFiles restores it before the program reads it, and the
// kit holds a URL credential in it the way it holds one in the variable.
func selectBaseURL(args []string) {
	base := runBaseURL(args)
	if base == "" {
		return
	}
	const name = "AVANAN_BASE_URL"
	prior, had := os.LookupEnv(name)
	restoreBaseURLEnv = func() {
		if had {
			_ = os.Setenv(name, prior)
		} else {
			_ = os.Unsetenv(name)
		}
	}
	_ = os.Setenv(name, base)
}

// runBaseURL mirrors how the program settles its base URL, or is "" when the
// environment already names it: auth login's flags, then the environment, then the
// selected config under the --home the run keeps.
func runBaseURL(args []string) string {
	if v := lastOf(credkit.FlagValues(args, "--base-url")); v != "" {
		return v
	}
	if v := lastOf(credkit.FlagValues(args, "--region")); v != "" {
		if base, ok := avanansig.RegionBaseURL(v); ok {
			return base
		}
		if base, ok := avanansig.InfinityRegionBaseURL(v); ok {
			return base
		}
	}
	if cliutil.EnvOverride("AVANAN_BASE_URL") != "" {
		return ""
	}
	restore, err := cliutil.SetHomeOverride(lastOf(credkit.FlagValues(args, "--home")))
	if err != nil {
		return "" // the run refuses this --home before it sends anything
	}
	defer restore()
	return config.SelectedBaseURL(lastOf(credkit.FlagValues(args, "--config")))
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// HoldStartupFiles holds, as main's second statement, what the kit does not
// split out of those files itself: each part of a [headers] value (a cookie's
// value, a parameter's) and every string of a file that does not parse, with
// its TOML escapes decoded. The run-profile files are held here rather than
// declared in Files: the kit would hold every string of them, JSON keys and
// ordinary values like a limit included, and withhold most output.
func HoldStartupFiles() {
	restoreBaseURLEnv()
	restoreFeedbackStdin()
	config.HoldFiles(startupFiles)
	for _, p := range startupProfiles {
		cliutil.HoldProfileFile(p)
	}
}

func pathsUnder(args []string) []string {
	homes := append(lastFirst(credkit.FlagValues(args, "--home")), "")
	configs := append(lastFirst(credkit.FlagValues(args, "--config")), "")
	var out []string
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		for _, c := range configs {
			for _, f := range config.ReadPaths(c) {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			startupProfiles = append(startupProfiles, filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		startupProfiles = append(startupProfiles, filepath.Join(userHome, ".avanan-cli", "profiles.json"))
	}
	return out
}

func lastFirst(values []string) []string {
	slices.Reverse(values)
	return values
}

// feedbackStdin is a `feedback --stdin` body, taken off stdin before the kit
// reads it. StdinFlags holds a --stdin body whole, and held whole the feedback
// record was withheld while the command said it was recorded. Feedback text is
// no credential (`feedback <text>` is never held); a held credential pasted
// into it is still withheld from the record. Only a command line Cobra resolves
// to feedback qualifies, flags in front of it included.
var feedbackStdin []byte

// takeFeedbackStdin runs from Files, before the kit reads stdin.
func takeFeedbackStdin(args []string) {
	if cliutil.CommandPath == nil {
		return
	}
	// A copy: argv stays read-only for the kit's FlagValues.
	if p := cliutil.CommandPath(append([]string(nil), args...)); len(p) != 1 || p[0] != "feedback" {
		return
	}
	asks := false
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		asks = asks || name == "--stdin"
	}
	if !asks {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit's own read of what is left fails closed
	}
	feedbackStdin = data
}

// restoreFeedbackStdin hands the body back on os.Stdin after Start.
func restoreFeedbackStdin() {
	if feedbackStdin == nil {
		return
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // the command reads an empty body and says so
	}
	go func(b []byte) {
		_, _ = w.Write(b)
		_ = w.Close()
	}(feedbackStdin)
	os.Stdin = r
}
