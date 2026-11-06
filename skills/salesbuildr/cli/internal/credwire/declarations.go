// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared credential
// kit (handfixes.json credkit-wiring): the Declarations literal both mains pass
// to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"salesbuildr-pp-cli/internal/cli"
	"salesbuildr-pp-cli/internal/client"
	"salesbuildr-pp-cli/internal/cliutil"
	"salesbuildr-pp-cli/internal/config"
	"salesbuildr-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// The Salesbuildr public API authenticates with one api-key header (no
// prefix): SALESBUILDR_API_KEY, or the api_key / auth_header of config.toml or
// the 0600 credentials.toml store. Some endpoints also take a vendor-issued
// integration-key header (--integration-key, or the integration-key argument
// of an MCP call). The store and config.toml are credential SOURCES: their own
// writes go outside the kit (cliutil.AtomicWriteCredentialFile, a declared
// exemption: the kit withholds them by contract) and both are named in Files.
// Every --stdin body is held (StdinFlags) except feedback's prose
// (compactStdinBody). The documented responses carry no credential; the
// response fields below are the names a vendor answer could carry one under.
// Every name is matched exactly (case folded), so useSignatureImage and
// shippingType stay readable.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"SALESBUILDR_API_KEY",
		"SALESBUILDR_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	// The credential keys of config.toml, the legacy config and the 0600
	// credentials.toml store (config.Config and cliutil.Credentials).
	ConfigFields:      []string{"api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords:     []string{"api_key", "auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	CredentialFlags:   []string{"--integration-key"},
	StdinFlags:        []string{"--stdin"},
	CredentialHeaders: []string{"api-key", "integration-key"},
	// The lists are written out in each field so credkit_verify can read them.
	CredentialFields: []string{"password", "otp", "pin", "passcode", "integrationKey", "integration-key", "integration_key", "api_key", "api-key"},
	ResponseFields:   []string{"password", "otp", "pin", "passcode", "integrationKey", "integration-key", "integration_key", "api_key", "api-key"},
	MCPArguments:     []string{"password", "otp", "pin", "passcode", "integrationKey", "integration-key", "integration_key", "api_key", "api-key"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "SALESBUILDR_BASE_URL",
		// A template the kit cannot resolve, so it allows no origin unless
		// showBaseURL resolves the base the run really uses.
		Default: "https://{tenant}.salesbuildr.com/public-api",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the API base the program will really use.
		// The connector's own origin pin (api-origin-pinned) narrows every request,
		// redirect hops included, to that same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:SALESBUILDR_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Keys of config.toml and credentials.toml that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:template_vars", Reason: "the Salesbuildr subdomain that fills {tenant} in the base URL, printed in every API URL"},
		{Source: "flag:--credential-ref", Reason: "a reference naming where a client profile's credential lives, not the credential"},
		{Source: "flag:--additional-credential-ref", Reason: "references naming where a client profile's credentials live, not the credentials"},
		{Source: "env:SALESBUILDR_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:SALESBUILDR_TENANT", Reason: "the Salesbuildr subdomain that fills {tenant} in the base URL, printed in every API URL"},
		{Source: "env:SALESBUILDR_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:SALESBUILDR_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:SALESBUILDR_CACHE_DIR", Reason: "directory path"},
		{Source: "env:SALESBUILDR_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:SALESBUILDR_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:SALESBUILDR_DATA_DIR", Reason: "directory path"},
		{Source: "env:SALESBUILDR_STATE_DIR", Reason: "directory path"},
		{Source: "env:SALESBUILDR_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:SALESBUILDR_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:SALESBUILDR_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:SALESBUILDR_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		// Not in URLEnv: its origin cannot be declared, so a held query value would refuse the feedback POST itself.
		{Source: "env:SALESBUILDR_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a held credential sent to it is refused by the outbound guard"},
		{Source: "env:SALESBUILDR_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:SALESBUILDR_LEARN_SESSION", Reason: "session grouping label, stored only hashed (handfixes.json journal-credential-hardening)"},
		{Source: "env:SALESBUILDR_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SALESBUILDR_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:SALESBUILDR_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:SALESBUILDR_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:SALESBUILDR_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:SALESBUILDR_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:SALESBUILDR_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:SALESBUILDR_USER_AGENT", Reason: "User-Agent string"},
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
	},
}

// credentialFiles runs inside Start, before anything can print. It shows the kit
// the base URL this run will use (showBaseURL), then returns the files the kit
// reads as this run's own, the config the program will really use FIRST: an
// explicit --config or SALESBUILDR_CONFIG wins, as it does in config.Load, and
// the last --home decides the default paths, as pflag does. Every other file
// any --config or --home could mean is only held (cliutil.HoldFile): its values
// cannot print, and its base_url cannot become an allowed origin. No flag arity
// is guessed: credkit.FlagValues counts every occurrence.
func credentialFiles(args []string) []string {
	compactStdinBody(args)
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	explicit := ""
	if len(configs) > 0 {
		explicit = configs[len(configs)-1]
	}
	if strings.TrimSpace(explicit) == "" { // as config.Load reads a blank --config
		explicit = os.Getenv("SALESBUILDR_CONFIG")
	}
	showBaseURL(last(homes), explicit)
	var used, held []string
	if len(homes) == 0 {
		used = runFiles(explicit)
	}
	for i, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		files := runFiles(explicit)
		held = append(held, runFiles("")...)
		restore()
		if i == len(homes)-1 {
			used = files
		} else {
			held = append(held, files...)
		}
	}
	held = append(held, runFiles("")...)
	for _, cfg := range configs {
		held = append(held, runFiles(cfg)...)
	}
	// Held outside Files, whose first base_url would become the allowed origin. A
	// file the kit reads itself (Files and every --config) is left to its reader.
	for _, f := range held {
		if !slices.Contains(used, f) && !slices.Contains(configs, f) {
			cliutil.HoldFile(f)
		}
	}
	return used
}

// runFiles is what a run with this explicit config (or none) reads, in the order
// config.Load prefers it: the config file, the legacy config when no explicit one
// was named and the default is absent, then the credential stores (the one beside
// an explicit config, and the default one).
func runFiles(explicit string) []string {
	var files []string
	if strings.TrimSpace(explicit) != "" {
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
	// The run profile store is a source too (credential leak contract section 1:
	// a value under a credential flag's name, --integration-key), read here and
	// not in the returned list: it is nested JSON, which the kit's file reader
	// answers by holding every string in it, so a profile named "p" or a saved
	// --limit 7 would refuse every later start. HoldFile reads it with the kit's
	// HoldBody, which holds the declared credential names at any depth.
	if dir, err := cliutil.ConfigDir(); err == nil {
		cliutil.HoldFile(filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		cliutil.HoldFile(filepath.Join(home, ".salesbuildr-cli", "profiles.json"))
	}
	return files
}

// last is the value the flag parser takes for a repeated flag: the last one.
func last(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// savedBaseURL is SALESBUILDR_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// built-in base is the template https://{tenant}.salesbuildr.com/public-api,
// which the kit cannot resolve, so it resolves the base the way the client
// does: config.Load under this run's --home and --config, then
// client.ResolvedBaseURL over the loaded template variables
// (SALESBUILDR_TENANT, template_vars, then the shipped placeholder). A tenant
// the origin pin refuses (cli.TenantPinnable: not one DNS label, or the
// shipped placeholder) shows nothing. The kit reads SALESBUILDR_BASE_URL right
// after Files returns; AfterStart puts the variable back. When Load refuses or
// nothing resolves, the variable is left alone and the template allows no
// origin: a request carrying a credential is refused.
func showBaseURL(home, configPath string) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	cfg, err := config.Load(configPath)
	if err != nil {
		return
	}
	if strings.Contains(cfg.BaseURL, "{tenant}") && !cli.TenantPinnable(cfg.TemplateVars["tenant"]) {
		return
	}
	base := client.ResolvedBaseURL(strings.TrimRight(cfg.BaseURL, "/"), cfg.TemplateVars)
	if base == "" || strings.ContainsAny(base, "{}") {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("SALESBUILDR_BASE_URL")
	baseURLShown = os.Setenv("SALESBUILDR_BASE_URL", base) == nil
}

// AfterStart is the statement after credkit.Start in both mains: it puts
// SALESBUILDR_BASE_URL back as the process found it, and gives
// `feedback --stdin` its body back on os.Stdin (without a feedback body that
// part does nothing).
func AfterStart() {
	restoreFeedbackStdin()
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("SALESBUILDR_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("SALESBUILDR_BASE_URL")
	}
}

// compactStdinBody puts a --stdin JSON body on one line before the kit reads
// it. Declarations.StdinFlags holds a body whole and line by line, and a line
// of "{" alone collides with public text and refuses the start, so a body from
// `jq .` would end the run. The credentials stay held: StdinFlags does that.
// Anything that is not one JSON document stays as it came.
//
// `feedback --stdin` is the exception: feedback text is no credential
// (`feedback <text>` is never held), and held whole its local record was
// withheld while the command said it was recorded. Its body is taken off stdin
// here, so the kit reads an empty one, and AfterStart hands it back after
// Start. Only a command line that starts with "feedback" qualifies: one with a
// flag in front stays held, which loses the record but never a secret.
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
	if len(args) > 0 && args[0] == "feedback" {
		feedbackStdin, data = data, nil
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

// feedbackStdin is the `feedback --stdin` body compactStdinBody took off stdin.
var feedbackStdin []byte

// restoreFeedbackStdin gives `feedback --stdin` its body back on os.Stdin.
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
	feedbackStdin = nil
	os.Stdin = r
}
