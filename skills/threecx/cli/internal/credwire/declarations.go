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

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/config"
	"threecx-pp-cli/internal/credkit"
	"threecx-pp-cli/internal/fleet"
)

// Declarations is what the connector holds and where each credential may go.
// 3CX authenticates with an OAuth client ID (a public DN number) and a client
// secret, from the environment, config.toml, the 0600 credentials.toml store or
// a per-PBX variable the fleet registry names (<env_prefix>_CLIENT_SECRET, held in
// credentialFiles because the name is operator data).
//
// ResponseFields, MCPArguments, CredentialFields and CredentialFlags are the
// string-valued credential names in the 3CX XAPI spec and the CLI's flags
// (TestDeclaredCredentialFlagsMatchTheCommands keeps the flags complete). Three
// kinds of name are
// left out on purpose:
//   - PINs in a response (PinNumber, VMPIN): a 3CX PIN is a few digits and often
//     equals the extension number, so holding a listing's PINs would withhold every
//     later unit holding those numbers. They are replaced by name in every response
//     (cliutil.RedactBody), stored nowhere, and named in Identifiers below. A PIN
//     the operator supplies (CredentialFields, MCPArguments, the PIN flags) is held.
//   - Key and key: a name/value record's name (Pbx.CrmChoice {Key, Value}). The
//     shared walker (cliutil.HoldCredentialValues) holds a Key that is not one.
//   - object-valued names (SecretKey, PrivateKey, ServiceAccountCredentials,
//     TranscribeSecretKey, RefreshTokens): the same walker holds what they contain.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"THREECX_CLIENT_SECRET",
		"THREECX_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
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
		"--api-key-value", "--config-custom-relay-config-client-private-key", "--settings-location-sftp-private-key", "--custom-relay-config-client-private-key-contents", "--license-key", "--location-sftp-private-key-contents",
		"--management-api-key-value", "--model-api-key", "--private-key-contents", "--remote-postgre-config-client-private-key-contents",
		"--sbc-private-key-contents", "--transcribe-settings-transcribe-key-grok-value",
		"--access-password", "--access-token", "--auth-password", "--auth-password-value", "--client-secret", "--client-secret-value",
		"--description-contents-encrypt-backup-password", "--deskphone-password", "--dir-search-settings-exchange-password-value",
		"--location-ftp-password-value", "--location-ns-password-value", "--location-sftp-password-value", "--mail-password-value",
		"--password", "--pin-number", "--provision-link", "--provision-url", "--provisioning-prov-link",
		"--remote-my-sql-config-password-value", "--remote-postgre-config-password-value", "--remote-sql-server-config-password-value",
		"--secret", "--service-account-credentials-access-key", "--service-account-credentials-secret-key-value",
		"--settings-contents-encrypt-backup-password", "--settings-encrypt-backup-password-value", "--settings-location-ftp-password",
		"--settings-location-ns-password", "--settings-location-sftp-password", "--settings-mail-password-value",
		"--state-pkce-code-verifier", "--transcribe-settings-transcribe-secret-key-value", "--trunk-auth-password-value",
		"--user-access-password", "--user-auth-password", "--user-deskphone-password", "--user-vmpin", "--verifier", "--vmpin",
	},
	CredentialHeaders: nil,
	StdinFlags:        []string{"--stdin"},
	// The string-valued credential members of the 3CX XAPI (requests, responses and
	// the tool arguments that carry them) and of the OAuth exchange. A declared name
	// matches exactly, so the lowercase "code" is the OAuth authorization code and
	// the XAPI's own Code members are untouched. CredentialFields and MCPArguments
	// add the PIN names to what the operator's own requests and tool arguments hold
	// (TestFieldDeclarationsAgree keeps the three lists the same).
	CredentialFields: []string{
		"APIKey", "AccessKey", "AccessPassword", "ApiKey", "AuthPassword", "ClientPrivateKey", "ClientSecret",
		"DeskphonePassword", "ExchangePassword", "FtpPassword", "LicenseKey", "MailPassword", "ManagementAPIKey",
		"NsPassword", "PKCECodeVerifier", "Password", "PhoneWebPassword", "ProvLink", "ProvisionLink", "ProvisionUrl",
		"ProvisioningLinkExt", "ProvisioningLinkLocal", "Secret", "SftpPassword", "SftpPrivateKey", "Token",
		"TranscribeKeyGrok", "accessToken", "licenseKey", "password", "verifier",
		"access_token", "refresh_token", "client_secret", "code", "code_verifier",
		"PinNumber", "VMPIN",
	},
	ResponseFields: []string{
		"APIKey", "AccessKey", "AccessPassword", "ApiKey", "AuthPassword", "ClientPrivateKey", "ClientSecret",
		"DeskphonePassword", "ExchangePassword", "FtpPassword", "LicenseKey", "MailPassword", "ManagementAPIKey",
		"NsPassword", "PKCECodeVerifier", "Password", "PhoneWebPassword", "ProvLink", "ProvisionLink", "ProvisionUrl",
		"ProvisioningLinkExt", "ProvisioningLinkLocal", "Secret", "SftpPassword", "SftpPrivateKey", "Token",
		"TranscribeKeyGrok", "accessToken", "licenseKey", "password", "verifier",
		"access_token", "refresh_token", "client_secret", "code", "code_verifier",
	},
	MCPArguments: []string{
		"APIKey", "AccessKey", "AccessPassword", "ApiKey", "AuthPassword", "ClientPrivateKey", "ClientSecret",
		"DeskphonePassword", "ExchangePassword", "FtpPassword", "LicenseKey", "MailPassword", "ManagementAPIKey",
		"NsPassword", "PKCECodeVerifier", "Password", "PhoneWebPassword", "ProvLink", "ProvisionLink", "ProvisionUrl",
		"ProvisioningLinkExt", "ProvisioningLinkLocal", "Secret", "SftpPassword", "SftpPrivateKey", "Token",
		"TranscribeKeyGrok", "accessToken", "licenseKey", "password", "verifier",
		"access_token", "refresh_token", "client_secret", "code", "code_verifier",
		"PinNumber", "VMPIN",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "THREECX_BASE_URL",
		Default:   "https://{fqdn}/xapi/v1",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the phone system this process is
		// configured for: a PBX's FQDN is the operator's, so no hosted domain can
		// be named here. credentialFiles resolves it as the program will (--pbx,
		// THREECX_PBX, THREECX_FQDN, config) and shows it to the kit. The
		// connector's own origin pin (api-origin-pinned) narrows every request,
		// redirects and the token exchange included, to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:THREECX_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "response:VMPIN", Reason: "a voicemail PIN in a listing often equals the extension number beside it; replaced by name in every response, stored nowhere"},
		{Source: "response:PinNumber", Reason: "a user PIN in a listing often equals the extension number beside it; replaced by name in every response, stored nowhere"},
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_url", Reason: "the token endpoint URL, on the phone system's origin"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "the OAuth client ID is the service principal's DN number, an ordinary number in every 3CX listing, not a secret"},
		{Source: "config:fqdn", Reason: "the phone system's host name, printed in every API URL"},
		{Source: "config:template_vars.fqdn", Reason: "the phone system's host name, printed in every API URL"},
		{Source: "basic-user", Reason: "the OAuth client ID in the token request's Basic header: the service principal's DN number, not a secret"},
		{Source: "env:THREECX_CLIENT_ID", Reason: "the OAuth client ID, the service principal's DN number, not a secret"},
		{Source: "env:THREECX_FQDN", Reason: "the phone system's host name, printed in every API URL"},
		{Source: "env:THREECX_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:THREECX_TOKEN_URL", Reason: "the token endpoint URL, on the phone system's origin"},
		{Source: "env:THREECX_OAUTH_SCOPE", Reason: "OAuth scope name"},
		{Source: "env:THREECX_PBX", Reason: "name of a registered phone system"},
		{Source: "env:THREECX_FLEET_DIR", Reason: "directory path"},
		{Source: "env:THREECX_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:THREECX_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:THREECX_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:THREECX_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:THREECX_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:THREECX_DATA_DIR", Reason: "directory path"},
		{Source: "env:THREECX_STATE_DIR", Reason: "directory path"},
		{Source: "env:THREECX_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:THREECX_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:THREECX_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:THREECX_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:THREECX_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:THREECX_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:THREECX_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:THREECX_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:THREECX_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:THREECX_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential sent to it is refused by the outbound guard"},
		{Source: "env:THREECX_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:THREECX_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:THREECX_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:THREECX_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:THREECX_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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

// credentialFiles runs inside Start, before anything can print. It holds the
// per-PBX secret family and the saved profiles, shows the kit this process's
// phone system, and names every file a credential may be read from under any
// --config and --home the command line can mean: holding a file the run does not
// load is safe, missing the one it loads is not. The fleet registry is not one of
// them: it holds host names and the NAME of each secret's variable, and the kit
// holds every value in a file it is given.
func credentialFiles(args []string) []string {
	holdPBXSecrets()
	holdOAuthCode(args)
	selected, selectedOK := selectedPBX(args)
	showBaseURL(args, selected, selectedOK)
	out := pathsUnder(args)
	holdProfileStoresUnder(args)
	reg, err := fleet.Load()
	if err != nil {
		return out
	}
	base, err := fleet.BaseDir()
	if err != nil {
		return out
	}
	for _, p := range reg.PBXes {
		restore, err := cliutil.SetHomeOverride(p.Home(base))
		if err != nil {
			continue
		}
		if files, err := config.ReadPaths(""); err == nil {
			for _, f := range files {
				if !slices.Contains(out, f) {
					out = append(out, f)
				}
			}
		}
		// A selection moves the home after Start, so its profiles are held here.
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	return out
}

// holdPBXSecrets holds every THREECX_*_CLIENT_SECRET variable (the default
// per-PBX family) and each registered PBX's <env_prefix>_CLIENT_SECRET, as given
// and without a pasted "Bearer " prefix. A PBX's prefix is operator data, so
// SecretEnv cannot list them.
func holdPBXSecrets() {
	hold := func(name, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		credkit.Hold(credkit.Source("env:"+name), v)
		if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
			credkit.Hold(credkit.Source("env:"+name), strings.TrimSpace(v[7:]))
		}
	}
	names := fleet.SecretEnvNames()
	for _, kv := range os.Environ() {
		if k, v, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "THREECX_") && strings.HasSuffix(k, fleet.SecretSuffix) || slices.Contains(names, k) {
			hold(k, v)
		}
	}
}

// holdProfileStoresUnder holds what the saved profiles under any --home the
// command line can mean carry.
func holdProfileStoresUnder(args []string) {
	for _, home := range append(credkit.FlagValues(args, "--home"), "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it loads anything
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if home, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(home, ".threecx-cli", "profiles.json"))
	}
}

// holdProfileStore holds the saved profiles in path through the same walker
// loadProfileStore runs on every read (cliutil.HoldProfileStore).
func holdProfileStore(path string) {
	data, _, err := cliutil.ReadFileWithLegacyFallback(path, "")
	if err != nil {
		return
	}
	cliutil.HoldProfileStore(data)
}

// holdOAuthCode holds an OAuth exchange's --code. A lone --code is not a
// credential (the XAPI has many), so it is held only beside --verifier or
// --token-endpoint, the same shape cliutil.OAuthCodeName reads in a body.
func holdOAuthCode(args []string) {
	if len(credkit.FlagValues(args, "--verifier")) == 0 && len(credkit.FlagValues(args, "--token-endpoint")) == 0 {
		return
	}
	credkit.Hold("flag:--code", credkit.FlagValues(args, "--code")...)
}

// selectedPBX is the registered PBX --pbx or THREECX_PBX names, as
// applyPBXSelection reads them: the flag's last value, then the variable.
func selectedPBX(args []string) (fleet.PBX, bool) {
	name := ""
	if vals := credkit.FlagValues(args, "--pbx"); len(vals) > 0 {
		name = strings.TrimSpace(vals[len(vals)-1])
	}
	if name == "" {
		name = strings.TrimSpace(os.Getenv("THREECX_PBX"))
	}
	if name == "" {
		return fleet.PBX{}, false
	}
	reg, err := fleet.Load()
	if err != nil {
		return fleet.PBX{}, false
	}
	return reg.Get(name)
}

// savedBaseURL is THREECX_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// kit reads Declarations.BaseURL, whose default is a template it cannot resolve,
// and a PBX's FQDN can be any host, so credentials could go nowhere otherwise.
// It reads the variable right after Files returns; AfterStart puts the variable
// back, so nothing the program does later sees a change. When nothing resolves,
// the variable is left alone and the kit allows no origin: a request carrying a
// credential is refused.
func showBaseURL(args []string, selected fleet.PBX, selectedOK bool) {
	base := ""
	if selectedOK {
		base = "https://" + selected.FQDN + "/xapi/v1"
	} else {
		base = configuredBase(args)
	}
	if base == "" {
		return
	}
	savedBaseURL, savedBaseURLSet = os.LookupEnv("THREECX_BASE_URL")
	baseURLShown = os.Setenv("THREECX_BASE_URL", base) == nil
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
// THREECX_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("THREECX_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("THREECX_BASE_URL")
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
