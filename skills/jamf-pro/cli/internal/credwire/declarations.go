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

	"jamf-pro-pp-cli/internal/cli"
	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/credkit"
	"jamf-pro-pp-cli/internal/fleet"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// KitDeclarations is what the connector holds and where each credential may go.
// Jamf Pro authenticates with an OAuth API client (a public client ID and a
// secret) or a pasted bearer token, from the environment, config.toml, the 0600
// credentials.toml store or a per-instance entry in JAMF_PRO_FLEET_SECRETS (each
// held in credentialFiles; the whole value names each instance beside its secret).
//
// ResponseFields, CredentialFields and MCPArguments are the string-valued
// credential names in the Jamf Pro spec and the OAuth exchange. One kind of name
// is treated on its own:
//   - a PIN in a response (pin): a device PIN is a few digits, so holding it
//     would withhold every later unit holding those digits. It is replaced by
//     name in every response (cliutil.RedactBody), stored nowhere, and named in
//     Identifiers below. A PIN the operator supplies (a flag, a tool argument or
//     a request body) is one value they chose to send, so it is held.
//
// Every other credential-shaped member (a name/value record labelled as a
// credential, a credential-named object) is held by the one shared walker
// (cliutil.HoldCredentialValues) before any command reads the response.
var KitDeclarations = credkit.Declarations{
	SecretEnv: []string{
		"JAMF_PRO_CLIENT_SECRET",
		"JAMF_PRO_MCP_HTTP_TOKEN",
		"JAMF_PRO_FLEET_SECRETS",
	},
	URLEnv: []string{"JAMF_PRO_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"client_secret",
		"access_token",
		"refresh_token",
	},
	StoredRecords: []string{
		"auth_header",
		"client_secret",
		"access_token",
		"refresh_token",
	},
	CredentialFlags: []string{
		"--account-settings-admin-password", "--client-secret", "--pin", "--recovery-lock-password", "--service-token", "--tvos-airplay-password",
	},
	StdinFlags:       []string{"--stdin"},
	CredentialFields: operatorFieldNames,
	ResponseFields:   credentialFieldNames,
	MCPArguments:     operatorFieldNames,
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "JAMF_PRO_BASE_URL",
		Default:   "https://{server}",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the Jamf Pro server this process is
		// configured for: a server's host is the operator's, so no hosted domain
		// can be named here. credentialFiles resolves it as the program will
		// (an inherited identity, --instance, JAMF_PRO_SERVER, config) and shows
		// it to the kit. jamfTransport narrows every request to the same server.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:JAMF_PRO_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "response:pin", Reason: "a device PIN in a response is a few digits; replaced by name in every response, stored nowhere"},
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_url", Reason: "the token endpoint URL, refused unless it is the server's own"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "the OAuth client ID, the public half of an API client pair"},
		{Source: "config:server", Reason: "the Jamf Pro server host name (template_vars.server), printed in every API URL"},
		{Source: "config:template_vars.server", Reason: "the Jamf Pro server host name, printed in every API URL"},
		{Source: "basic-user", Reason: "the OAuth client ID in a token request's Basic header, the public half of the pair"},
		{Source: "env:JAMF_PRO_CLIENT_ID", Reason: "the OAuth client ID, the public half of an API client pair; Jamf Pro prints it in its own API client listing"},
		{Source: "env:JAMF_PRO_SERVER", Reason: "the Jamf Pro server host name, printed in every API URL"},
		{Source: "env:JAMF_PRO_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:JAMF_PRO_OAUTH_SCOPE", Reason: "OAuth scope name"},
		{Source: "env:JAMF_PRO_FLEET_SECRETS", Reason: "the whole list names each instance beside its secret; credentialFiles holds each secret on its own (minted:fleet-secret), and SecretEnv still keeps the variable from other programs"},
		{Source: "env:JAMF_PRO_INSTANCE", Reason: "name of a registered Jamf Pro instance"},
		{Source: "env:PP_JAMF_PRO_IDENTITY", Reason: "a parent's identity for its child: server, client ID and instance name, no credential"},
		{Source: "env:JAMF_PRO_FLEET_DIR", Reason: "directory path"},
		{Source: "env:JAMF_PRO_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:JAMF_PRO_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:JAMF_PRO_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:JAMF_PRO_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:JAMF_PRO_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:JAMF_PRO_DATA_DIR", Reason: "directory path"},
		{Source: "env:JAMF_PRO_STATE_DIR", Reason: "directory path"},
		{Source: "env:JAMF_PRO_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:JAMF_PRO_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:JAMF_PRO_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:JAMF_PRO_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:JAMF_PRO_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:JAMF_PRO_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:JAMF_PRO_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:JAMF_PRO_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:JAMF_PRO_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:JAMF_PRO_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv and refused by the outbound guard"},
		{Source: "env:JAMF_PRO_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:JAMF_PRO_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:JAMF_PRO_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:JAMF_PRO_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:JAMF_PRO_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFieldNames are the string-valued credential members of the Jamf Pro
// API (requests, responses and the tool arguments that carry them) and of the
// OAuth exchange. A declared name matches exactly. operatorFieldNames adds the
// PIN to what the operator's own requests and tool arguments hold.
var credentialFieldNames = []string{
	"adminPassword", "airPlayPassword", "airplayPassword", "appleCareToken", "bootstrapToken", "clientSecret",
	"encodedToken", "managed_password", "newPassword", "of_password", "password", "personalRecoveryKey",
	"plainPassword", "recoveryLockPassword", "serviceToken", "unlockToken",
	"access_token", "refresh_token", "client_secret",
}

var operatorFieldNames = append(slices.Clone(credentialFieldNames), "pin")

// credentialFiles runs inside Start, before anything can print. It holds the
// per-instance secret family and what a run profile carries, shows the kit this
// process's server, and names every file a credential may be read from under any
// --config and --home the command line can mean: holding a file the run does not
// load is safe, missing the one it loads is not. The fleet registry is not one of
// them: it holds host names and the NAME of each secret's variable, and the kit
// holds every value in a file it is given.
func credentialFiles(args []string) []string {
	holdInstanceSecrets()
	showBaseURL(args)
	out := pathsUnder(args)
	for _, home := range append(credkit.FlagValues(args, "--home"), "") {
		if restore, err := cliutil.SetHomeOverride(home); err == nil {
			cli.HoldProfileStores()
			restore()
		}
	}
	reg, err := fleet.Load()
	if err != nil {
		return out
	}
	base, err := fleet.BaseDir()
	if err != nil {
		return out
	}
	for _, p := range reg.Instances {
		asPinned(p, p.Home(base), func() {
			cli.HoldProfileStores() // each instance keeps its own run profiles
			if files, err := config.ReadPaths(""); err == nil {
				for _, f := range files {
					if !slices.Contains(out, f) {
						out = append(out, f)
					}
				}
			}
		})
	}
	return out
}

// asPinned runs fn with the paths a run pinned to instance p resolves: the
// environment pinning keeps (fleet.Scrub, so no JAMF_PRO_* path variable), the
// instance's config file, and its home. Everything is put back before it
// returns. Hand-fix (handfixes.json credkit-wiring).
func asPinned(p fleet.Instance, home string, fn func()) {
	saved := os.Environ()
	kept := map[string]bool{}
	for _, kv := range fleet.Scrub(saved) {
		k, _, _ := strings.Cut(kv, "=")
		kept[k] = true
	}
	defer func() {
		now := map[string]bool{}
		for _, kv := range saved {
			k, v, _ := strings.Cut(kv, "=")
			now[k] = true
			_ = os.Setenv(k, v)
		}
		for _, kv := range os.Environ() {
			if k, _, _ := strings.Cut(kv, "="); !now[k] {
				_ = os.Unsetenv(k)
			}
		}
	}()
	for _, kv := range saved {
		if k, _, _ := strings.Cut(kv, "="); !kept[k] {
			_ = os.Unsetenv(k)
		}
	}
	_ = os.Setenv("JAMF_PRO_CONFIG", filepath.Join(home, "config", "config.toml"))
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	fn()
}

// holdInstanceSecrets holds every instance secret JAMF_PRO_FLEET_SECRETS carries,
// as given and without a pasted "Bearer " prefix (the whole value is a declared
// Identifier: it names each instance beside its secret). A value that does not
// parse is held piece by piece (fleet.SecretFragments), less the registered
// instance names output prints, so a malformed variable fails closed.
func holdInstanceSecrets() {
	hold := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		credkit.Hold(instanceHoldSource, v)
		if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
			credkit.Hold(instanceHoldSource, strings.TrimSpace(v[7:]))
		}
	}
	raw := fleet.RawSecrets()
	if secrets, err := fleet.ParseSecrets(raw); err == nil {
		for _, v := range secrets {
			hold(v)
		}
		return
	}
	var names []string
	if reg, err := fleet.Load(); err == nil {
		names = reg.Names()
	}
	for _, f := range fleet.SecretFragments(raw) {
		if !slices.Contains(names, f) {
			hold(f)
		}
	}
}

// instanceHoldSource is the kit source of each secret JAMF_PRO_FLEET_SECRETS carries.
const instanceHoldSource = "minted:fleet-secret"

// savedBaseURL is JAMF_PRO_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// kit reads Declarations.BaseURL, whose default is a template it cannot resolve,
// and a Jamf Pro server can be any host, so credentials could go nowhere otherwise.
// It reads the variable right after Files returns; AfterStart puts the variable
// back, so nothing the program does later sees a change. When nothing resolves,
// the variable is left alone and the kit allows no origin: a request carrying a
// credential is refused.
func showBaseURL(args []string) {
	// The swap below hides the real value from the kit, and the selection read
	// here can differ from the one the run makes, so the real value's credentials
	// are held first, whatever this run selects.
	cliutil.HoldBaseURL(os.Getenv("JAMF_PRO_BASE_URL"))
	base := selectedServer(args)
	if base == "" {
		base = configuredBase(args)
	}
	if base == "" {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("JAMF_PRO_BASE_URL")
	baseURLShown = os.Setenv("JAMF_PRO_BASE_URL", base) == nil
}

// selectedServer is the server applyInstanceSelection acts on: the identity a
// parent handed down, else the instance --instance or JAMF_PRO_INSTANCE names,
// reading them as it does (the flag's last value, then the variable). A broken
// selection gives "": the run refuses it before it sends anything.
func selectedServer(args []string) string {
	if raw := strings.TrimSpace(jamfauth.Inherited()); raw != "" {
		if id, err := jamfauth.Decode(raw); err == nil {
			return id.Server()
		}
		return ""
	}
	name := lastOf(credkit.FlagValues(args, "--instance"))
	if name == "" {
		name = jamfauth.Selected()
	}
	if name = strings.TrimSpace(name); name == "" {
		return ""
	}
	reg, err := fleet.Load()
	if err != nil {
		return ""
	}
	p, ok := reg.Get(name)
	if !ok {
		return ""
	}
	return "https://" + p.Server
}

// configuredBase is the base URL config.Load resolves under this command line,
// with its {placeholders} filled in, or "" when one is left or Load refuses.
func configuredBase(args []string) string {
	home, configPath := lastOf(credkit.FlagValues(args, "--home")), lastOf(credkit.FlagValues(args, "--config"))
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

// AfterStart is the statement after credkit.Start in both mains: it puts
// JAMF_PRO_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("JAMF_PRO_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("JAMF_PRO_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

func pathsUnder(args []string) []string {
	homes := append(credkit.FlagValues(args, "--home"), "")
	configs := append(credkit.FlagValues(args, "--config"), "")
	var out []string
	for _, home := range homes {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		for _, c := range configs {
			files, err := config.ReadPaths(c)
			if err != nil {
				continue
			}
			for _, f := range files {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		restore()
	}
	return out
}
