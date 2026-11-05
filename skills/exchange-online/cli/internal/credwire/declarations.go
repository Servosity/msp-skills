// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"exchange-online-pp-cli/internal/cliutil"
	"exchange-online-pp-cli/internal/config"
	"exchange-online-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Exchange Online takes a bearer token minted outside this program for the
// customer tenant, from the environment, config.toml or the 0600 credential store.
// The store and config.toml are credential SOURCES: their own writes go outside the
// kit (cliutil.AtomicWriteCredentialFile) and they are named in Files, because the
// kit would withhold them by contract.
//
// No cmdlet response or parameter in the spec has a credential name
// (TestDeclaredFieldsAreNotSpecFields keeps that true). The names below are what a
// token endpoint answers with and the parameters a cmdlet run through
// exchange-online_execute or --stdin can carry (New-Mailbox -Password), matched
// exactly; the shared walker (cliutil.HoldCredentialValues) catches other spellings.
// Every name is string-valued, because the kit holds every scalar under a declared
// name: an object here would hold an ordinary member and withhold a write's own
// answer. Ordinary identifiers (mailbox names, GUIDs, UPNs, a Get-Mailbox Identity)
// are not held, or every listing would be withheld.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"EXCHANGE_ONLINE_TOKEN",
		"EXCHANGE_ONLINE_MCP_HTTP_TOKEN",
	},
	URLEnv: []string{
		"EXCHANGE_ONLINE_FEEDBACK_ENDPOINT",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token", "online_token",
	},
	StoredRecords: []string{
		"auth_header", "client_id", "client_secret", "access_token", "refresh_token", "online_token",
	},
	CredentialFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret", "password", "Password", "NewPassword", "OldPassword", "secret", "token",
	},
	ResponseFields: []string{
		"access_token", "refresh_token", "id_token", "client_secret", "password", "Password", "secret", "token",
	},
	MCPArguments: []string{
		"access_token", "refresh_token", "id_token", "client_secret", "password", "Password", "NewPassword", "OldPassword", "secret", "token",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "EXCHANGE_ONLINE_BASE_URL",
		Default:   "https://outlook.office365.com",
	},
	Origins: []credkit.Origin{
		// A token goes only to the API base the program will really use. The
		// connector's own origin pin (api-origin-pinned) narrows every request,
		// redirects and nextLink pages included, to that same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearers authenticate callers of this server; they are sent nowhere.
		{Source: "env:EXCHANGE_ONLINE_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:skip_tls_verify", Reason: "a boolean setting"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "flag:--credential-ref", Reason: "a reference naming where a client profile's credential lives, not the credential"},
		{Source: "flag:--additional-credential-ref", Reason: "references naming where a client profile's credentials live, not the credentials"},
		{Source: "env:EXCHANGE_ONLINE_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:EXCHANGE_ONLINE_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:EXCHANGE_ONLINE_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:EXCHANGE_ONLINE_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:EXCHANGE_ONLINE_ANCHOR_MAILBOX", Reason: "the mailbox UPN that routes a cmdlet, an ordinary identifier"},
		{Source: "env:EXCHANGE_ONLINE_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:EXCHANGE_ONLINE_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:EXCHANGE_ONLINE_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:EXCHANGE_ONLINE_DATA_DIR", Reason: "directory path"},
		{Source: "env:EXCHANGE_ONLINE_STATE_DIR", Reason: "directory path"},
		{Source: "env:EXCHANGE_ONLINE_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:EXCHANGE_ONLINE_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:EXCHANGE_ONLINE_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:EXCHANGE_ONLINE_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:EXCHANGE_ONLINE_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:EXCHANGE_ONLINE_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:EXCHANGE_ONLINE_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:EXCHANGE_ONLINE_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:EXCHANGE_ONLINE_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:EXCHANGE_ONLINE_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:EXCHANGE_ONLINE_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:EXCHANGE_ONLINE_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:EXCHANGE_ONLINE_LEARN_SESSION", Reason: "session grouping label, hashed before it is written"},
		{Source: "env:EXCHANGE_ONLINE_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It holds what
// `auth set-token` was given, by argument or pipe, and the credential fields of a
// --stdin body, and names every file a credential may be read from under any
// --config and --home the command line can mean: holding a file the run does not
// load is safe, missing the one it loads is not.
func credentialFiles(args []string) []string {
	holdSetTokenArguments(args)
	holdPipedToken(args)
	holdStdinBody(args)
	return pathsUnder(args)
}

// setTokenArguments is what follows `auth set-token` on the line, when the line
// names that command. The two words are matched in order, so `search set-token`
// reads no pipe.
func setTokenArguments(args []string) ([]string, bool) {
	i := slices.Index(args, "auth")
	if i < 0 {
		return nil, false
	}
	j := slices.Index(args[i+1:], "set-token")
	if j < 0 {
		return nil, false
	}
	return args[i+1+j+1:], true
}

// holdSetTokenArguments holds a key pasted on the `auth set-token` line. The command
// takes no argument, and its parser repeats what it was given before the command
// runs. Only the command's own flags, and their values, are not a key.
func holdSetTokenArguments(args []string) {
	rest, ok := setTokenArguments(args)
	if !ok {
		return
	}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if a != name || hasValue { // a dashed word
			if takesValue, known := setTokenFlags[name]; known {
				if !hasValue && takesValue {
					i++ // the next word is this flag's value
				}
				continue
			}
			credkit.Hold("stdin", a, name)
			if hasValue {
				credkit.Hold("stdin", value)
			}
			continue
		}
		if strings.TrimSpace(a) != "" {
			credkit.Hold("stdin", a)
		}
	}
}

// setTokenFlags is every flag `auth set-token` declares and whether it takes a value.
// It is data, not a walk of the command tree: Files runs inside Start, and a call
// into the commands from here would put every command before Start for the wiring
// checker. TestSetTokenFlagsMatchTheCommand keeps it true.
var setTokenFlags = map[string]bool{
	"agent":                 false,
	"allow-partial-failure": false,
	"audit-dir":             true,
	"ca-cert":               true,
	"client-profile":        true,
	"compact":               false,
	"config":                true,
	"csv":                   false,
	"data-source":           true,
	"deliver":               true,
	"dry-run":               false,
	"h":                     false,
	"help":                  false,
	"home":                  true,
	"human-friendly":        false,
	"idempotent":            false,
	"insecure":              false,
	"json":                  false,
	"max-age":               true,
	"no-cache":              false,
	"no-color":              false,
	"no-input":              false,
	"no-learn":              false,
	"plain":                 false,
	"profile":               true,
	"quiet":                 false,
	"rate-limit":            true,
	"receipt":               false,
	"receipt-file":          true,
	"select":                true,
	"timeout":               true,
	"tls-server-name":       true,
	"yes":                   false,
}

// stdinHoldLimit is how much of a piped token is read ahead: past the 64 KiB
// `auth set-token` accepts, so the command still sees an oversized pipe and refuses
// it, and bounded, so a pipe that never ends cannot fill memory first.
const stdinHoldLimit = 128 << 10

// holdPipedToken holds what `auth set-token` will read before anything can print
// (contract section 1, stdin included): the parser reads the command line before the
// command reads the pipe, and its errors repeat what it was given. It holds the pipe
// whole and line by line, and gives the program the same bytes on a new os.Stdin.
func holdPipedToken(args []string) {
	if _, ok := setTokenArguments(args); !ok || stdinIsTerminal() || asksForHelp(args) {
		return
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
	refeedStdin(b)
}

// holdStdinBody holds the credential fields of a --stdin request body before anything
// prints, previews and --dry-run included, and gives the program the same bytes on a
// new os.Stdin. It is not the kit's StdinFlags: those hold every line of a body, so a
// pretty-printed body stops the program (a line that is only "{" collides with public
// text) and a one-line body withholds its own preview. credkit.HoldBody reads the body
// under the declared fields, and holds every word of one that is no document (the
// command refuses it, and its error may repeat it). feedback --stdin reads a note, not
// a body, so it stays out of HoldBody.
func holdStdinBody(args []string) {
	if slices.Contains(args, "feedback") || stdinIsTerminal() || asksForHelp(args) || !namesStdinFlag(args) {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		b = nil // a partial read is no body: the command refuses an empty one
	}
	credkit.HoldBody(b)
	refeedStdin(b)
}

// namesStdinFlag is a plain loop, not slices.ContainsFunc: check_mcp_file_flags.py ties
// the kit's FlagValues to its flag only while argv goes nowhere it cannot read.
func namesStdinFlag(args []string) bool {
	for _, a := range args {
		if a == "--stdin" || strings.HasPrefix(a, "--stdin=") {
			return true
		}
	}
	return false
}

// asksForHelp reports whether the parser answers this line with help, so nothing will
// read stdin and a harness that leaves its pipe open must not wait on it. A --help that
// may be the value of the flag before it is not asking: reading such a line wrongly only
// waits on a pipe, and the other mistake would leave a piped value unheld.
func asksForHelp(args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	for i, a := range args {
		if a == "--" {
			return false
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if a == name || (name != "help" && name != "h") {
			continue
		}
		if hasValue {
			if on, err := strconv.ParseBool(value); err != nil || !on {
				continue
			}
		}
		if i == 0 || !mayTakeNextWord(args[i-1]) {
			return true
		}
	}
	return false
}

// mayTakeNextWord reports whether word is a flag that may take the next word as its
// value: one of the flags every command shares that takes a value, or a flag this file
// does not know, whose own command may declare it that way.
func mayTakeNextWord(word string) bool {
	name := strings.TrimLeft(word, "-")
	if name == word || strings.Contains(word, "=") {
		return false
	}
	takesValue, known := setTokenFlags[name]
	return name != "stdin" && (takesValue || !known)
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// refeedStdin gives the program the bytes startup already read.
func refeedStdin(b []byte) {
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent: the command reads nothing and refuses
	}
	go func() {
		_, _ = w.Write(b)
		_ = w.Close()
	}()
	os.Stdin = r
}

// pathsUnder is what the kit reads for the run. Files are listed for the --home the
// run will use, the last on the line; every other file under any --home or --config
// the line can mean is only held, along with the configs Load merely consults, the
// stores that record a base URL and the run profile stores. A listed file's base_url
// becomes the one origin credentials may go to (contract 2c), so nothing the run does
// not take its base URL from may be listed.
func pathsUnder(args []string) []string {
	homes := credkit.FlagValues(args, "--home")
	configs := append(credkit.FlagValues(args, "--config"), "")
	selectedHome, selectedConfig := lastFlag(args, "--home"), lastFlag(args, "--config")
	var out []string
	listIf := func(run bool, f string) {
		if run && !slices.Contains(out, f) {
			out = append(out, f)
		} else if !run {
			holdFile(f)
		}
	}
	for _, home := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if consulted, err := config.ConsultedPaths(); err == nil {
			for _, f := range consulted {
				holdFile(f)
			}
		}
		for _, c := range configs {
			run := home == selectedHome && c == selectedConfig
			if files, err := config.ReadPaths(c); err == nil {
				for _, f := range files {
					listIf(run, f)
				}
			}
			if stores, err := config.StorePaths(c); err == nil {
				for _, f := range stores {
					// A store records the base URL its token was issued for, which
					// decides nothing about this run's base URL.
					if data, err := os.ReadFile(filepath.Clean(f)); err == nil && bytes.Contains(data, []byte("base_url")) {
						cliutil.HoldConfigFile(data)
					} else {
						listIf(run, f)
					}
				}
			}
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			cliutil.HoldProfileStoreFile(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldProfileStoreFile(filepath.Join(home, ".exchange-online-cli", "profiles.json"))
	}
	return out
}

// lastFlag is the value the parser takes for a flag: the last before any "--".
func lastFlag(args []string, name string) string {
	if end := slices.Index(args, "--"); end >= 0 {
		args = args[:end]
	}
	values := credkit.FlagValues(args, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func holdFile(path string) {
	if opened, err := cliutil.ReadOpenedConfig(path); err == nil {
		cliutil.HoldConfigFile(opened.Data)
	}
}
