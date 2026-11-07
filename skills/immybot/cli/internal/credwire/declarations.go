// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"net/url"
	"os"
	"regexp"
	"slices"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/config"
	"immybot-pp-cli/internal/credkit"
)

// entraOrigins is where the Entra client secret and refresh token may go:
// Microsoft Entra's sign-in hosts, plus the https origin of IMMYBOT_TOKEN_URL,
// the token endpoint this run's environment names (config.Load applies the same
// override). Like base_url, the endpoint the run will use is the one it may
// sign in at. A value that is not https with an ASCII host[:port] adds nothing,
// so the secret stays refused there.
var entraOrigins = func() []string {
	o := []string{"https://login.microsoftonline.com", "https://login.microsoftonline.us", "https://login.partner.microsoftonline.cn", "https://login.chinacloudapi.cn"}
	if u, err := url.Parse(cliutil.EnvOverride("IMMYBOT_TOKEN_URL")); err == nil && u.Scheme == "https" && u.User == nil && asciiHostPort.MatchString(u.Host) {
		o = append(o, "https://"+u.Host)
	}
	return o
}()

var asciiHostPort = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]{1,5})?$`)

// Declarations is what the connector holds and where each credential may go.
// spec.json carries these secrets beyond the Entra sign-in: the SMTP
// password, the provisioning package's passwords and Wi-Fi key, and an Entra app
// registration's secret, which the vendor also returns in
// AzureTenantAuthDetails.customAppRegSecret. All of them are declared request,
// flag and tool-argument fields, and the returned one a response field.
var Declarations = credkit.Declarations{
	SecretEnv:     []string{"IMMYBOT_CLIENT_SECRET", "IMMYBOT_MCP_HTTP_TOKEN"},
	Files:         credentialFiles,
	URLEnv:        []string{"IMMYBOT_TOKEN_URL", "IMMYBOT_FEEDBACK_ENDPOINT"},
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_secret"},
	CredentialFlags: []string{
		"--client-secret",
		"--password",
		"--ppkg-options-local-admin-password",
		"--ppkg-options-package-password",
		"--ppkg-options-wireless-key",
		"--auth-details-custom-app-reg-secret",
	},
	// A --stdin body is held whole and line by line, so a command that echoes any
	// part of it prints the kit's notice instead (followups, measured).
	StdinFlags:       []string{"--stdin"},
	CredentialFields: []string{"client_secret", "refresh_token", "password", "localAdminPassword", "packagePassword", "wirelessKey", "customAppRegSecret"},
	ResponseFields:   []string{"access_token", "refresh_token", "id_token", "customAppRegSecret"},
	MCPArguments: []string{
		"password", "localAdminPassword", "packagePassword", "wirelessKey", "customAppRegSecret",
		"ppkg-options-local-admin-password", "ppkg-options-package-password", "ppkg-options-wireless-key", "auth-details-custom-app-reg-secret",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "IMMYBOT_BASE_URL",
		Default:   "https://{subdomain}.immy.bot",
	},
	Origins: []credkit.Origin{
		// base_url is the instance this run will use (selectKitBaseURL), and no
		// other: a redirect to another origin, another instance's included, carries
		// no held value at all.
		{Source: "*", Allowed: []string{"base_url"}},
		// The client secret and a refresh token go only to Microsoft Entra's sign-in
		// hosts, public and sovereign, and to the IMMYBOT_TOKEN_URL this run signs in
		// at. A config token_url on any other host is refused.
		{Source: "env:IMMYBOT_CLIENT_SECRET", Allowed: entraOrigins},
		{Source: "config:client_secret", Allowed: entraOrigins},
		{Source: "flag:--client-secret", Allowed: entraOrigins},
		{Source: "field:client_secret", Allowed: entraOrigins},
		{Source: "config:refresh_token", Allowed: entraOrigins},
		{Source: "field:refresh_token", Allowed: entraOrigins},
		{Source: "response:refresh_token", Allowed: entraOrigins},
		// The MCP HTTP bearer authenticates callers of this server, and the TLS
		// key is this server's own; neither is sent anywhere.
		{Source: "env:IMMYBOT_MCP_HTTP_TOKEN", Allowed: nil},
		{Source: "minted:tls-key", Allowed: nil},
		// A proxy's password goes to that proxy only: the kit's transport adds its origin.
		{Source: "proxy-userinfo", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "basic-user", Reason: "the Entra application id in the token request's Basic credential, an identifier"},
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:token_url", Reason: "the token endpoint URL; a credential inside it is held by config.HoldCredentials"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "config:client_id", Reason: "the Entra application id, an identifier the portal shows"},
		{Source: "config:tenant_id", Reason: "the Entra directory id, an identifier"},
		{Source: "env:IMMYBOT_CLIENT_ID", Reason: "the Entra application id, an identifier the portal shows"},
		{Source: "env:IMMYBOT_TENANT_ID", Reason: "the Entra directory id, an identifier"},
		{Source: "env:IMMYBOT_SUBDOMAIN", Reason: "the instance's host name label"},
		{Source: "env:IMMYBOT_OAUTH_SCOPE", Reason: "an OAuth scope name"},
		{Source: "env:IMMYBOT_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:IMMYBOT_TOKEN_URL", Reason: "the token endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:IMMYBOT_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and the outbound guard refuses any held value sent to it"},
		{Source: "env:IMMYBOT_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:IMMYBOT_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:IMMYBOT_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:IMMYBOT_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:IMMYBOT_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:IMMYBOT_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "name of a client profile, not its contents"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "test-harness switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "test-harness switch"},
		{Source: "env:PRESEED_DISABLED", Reason: "boolean switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "MCP transport name (stdio or http)"},
		{Source: "env:AGENT_ID", Reason: "agent label recorded with feedback"},
		{Source: "env:NO_COLOR", Reason: "terminal color switch"},
		{Source: "env:TERM", Reason: "terminal type"},
		{Source: "env:IMMYBOT_HOME", Reason: "root directory for config, data, state and cache; Files resolves it"},
		{Source: "env:IMMYBOT_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:IMMYBOT_DATA_DIR", Reason: "directory path"},
		{Source: "env:IMMYBOT_STATE_DIR", Reason: "directory path"},
		{Source: "env:IMMYBOT_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:IMMYBOT_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:IMMYBOT_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:IMMYBOT_NO_CONFIG_WRITE", Reason: "boolean switch"},
		{Source: "env:IMMYBOT_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:IMMYBOT_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:IMMYBOT_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:IMMYBOT_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:IMMYBOT_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:IMMYBOT_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:IMMYBOT_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:IMMYBOT_MCP_UPLOAD_ROOT", Reason: "directory an MCP upload may read from"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "name of the profile an MCP server is bound to"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "test-harness switch"},
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

// credentialFiles is every config file and credentials store the command line
// could mean: under each --home and the default home, for each --config and the
// environment's choice. It names more than the program reads when a flag's value
// looks like a flag; holding a file the program does not read is safe, missing the
// one it reads is not.
func credentialFiles(args []string) []string {
	var out []string
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	for _, home := range append(slices.Clone(homes), "") {
		underHome(home, func() {
			for _, c := range append(slices.Clone(configs), "") {
				for _, f := range config.ReadPaths(c) {
					if !slices.Contains(out, f) {
						out = append(out, f)
					}
				}
			}
		})
	}
	ReadProfileStores(ProfileStorePaths(homes), HoldProfileCredentials)
	// The choice stops at the first "--", as a flag parser's does, and pflag keeps
	// the last of a repeated flag.
	end := slices.Index(args, "--")
	if end < 0 {
		end = len(args)
	}
	args = args[:end]
	configFlag := lastOf(credkit.FlagValues(args, "--config"))
	underHome(lastOf(credkit.FlagValues(args, "--home")), func() { selectKitBaseURL(configFlag) })
	return out
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// underHome runs f with home as --home would set it. The run refuses a --home that
// does not apply before it loads anything, so f is skipped.
func underHome(home string, f func()) {
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		return
	}
	defer restore()
	f()
}

// restoreBaseURLEnv undoes selectKitBaseURL; HoldStartup runs it.
var restoreBaseURLEnv = func() {}

// selectKitBaseURL makes the kit's base_url the instance Load will route to. The
// kit reads an explicit --config and BaseURL.Env, so it sees neither {subdomain}
// filled nor what --home selects. Start reads BaseURL.Env after Files, so the
// choice goes there for Start alone.
func selectKitBaseURL(configFlag string) {
	const env = "IMMYBOT_BASE_URL"
	// Nothing is held here: this runs inside Start's public-text check, and Load
	// holds a config's every word, which that check would refuse. The real Load and
	// HoldStartup hold them after it.
	hold := cliutil.HoldValues
	cliutil.HoldValues = func(credkit.Source, ...string) {}
	want := config.SelectedBaseURL(configFlag)
	cliutil.HoldValues = hold
	was, set := os.LookupEnv(env)
	if want == "" || was == want || os.Setenv(env, want) != nil {
		return
	}
	restoreBaseURLEnv = func() {
		if set {
			_ = os.Setenv(env, was)
		} else {
			_ = os.Unsetenv(env)
		}
	}
}
