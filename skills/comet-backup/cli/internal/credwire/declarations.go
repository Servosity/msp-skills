// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal both
// mains pass to credkit.Start. The SQLite driver every store opens through is
// internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"comet-backup-pp-cli/internal/cli"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/config"
	"comet-backup-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Declarations is what the connector holds and where each credential may go.
// Comet authenticates with form fields in every POST body: an admin username
// (not a credential) and a password or session key, from the environment,
// config.toml or the 0600 credentials.toml store.
//
// ResponseFields are the string-valued credential members of comet-go-sdk
// v2.7.0 (UserProfileConfig, DestinationConfig, Organization branding,
// BucketProperties, WebhookOption, PSAConfig, CustomRemoteBucketSettings).
// Under a name the SDK does not use, the shared walker
// (cliutil.HoldCredentialValues, cliutil.SecretFieldName) holds what a response
// carries; TestDeclaredNamesMatchTheWalker keeps the two in step.
// ResponseFields are the string-valued credential members of a Comet response
// (and Credential, Office365Credential and InstallToken, objects every leaf of
// which is held). CredentialFields and MCPArguments add the fields an admin
// request carries (form or JSON) and the tool arguments that mirror them. A
// declared name matches whatever its case.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"COMET_BACKUP_PASSWORD",
		"COMET_BACKUP_SESSION_KEY",
		"COMET_BACKUP_MCP_HTTP_TOKEN",
	},
	Files: credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"backup_password",
		"backup_session_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	StoredRecords: []string{
		"auth_header",
		"backup_password",
		"backup_session_key",
		"access_token",
		"refresh_token",
		"client_id",
		"client_secret",
	},
	CredentialFlags: []string{"--credential-ref", "--new-password", "--old-password", "--target-password"},
	CredentialFields: []string{
		"APIKey", "AZBAccountKey", "AccessKey", "AppKey", "AppleIDPass", "AuthToken", "Authorization", "BindPassword",
		"ClientSecret", "CometBucketKey", "Cookie", "Credential", "CredentialJSON", "CustomBody", "CustomHeaders",
		"EncryptedEncryptionKey", "EncryptionKey", "FTPPassword", "InstallToken", "Key", "LocalcopyWinSMBPassword",
		"NewBucketKey", "Office365Credential", "PartnerKey", "Passphrase", "Password", "PasswordHash", "PasswordRecovery",
		"PfxFilePassword", "ReadWriteKey", "S3AccessKey", "S3SecretKey", "SFTPPassword", "SFTPPrivateKey", "SMBPassword",
		"SMTPPassword", "SSHPassword", "SSHPrivateKey", "SSLCertKey", "SSLClientKeyPassword", "SSLPfxPassword", "Secret",
		"SecretKey", "SessionKey", "TOTP", "TOTPKey", "Token", "WebAuthnSign", "WindowsCodeSignAzureAppSecret",
		"WindowsCodeSignPKCS11Password", "WindowsCodeSignPKCS12Password",
		"TargetPassword", "NewPassword", "OldPassword",
	},
	ResponseFields: []string{
		"APIKey", "AZBAccountKey", "AccessKey", "AppKey", "AppleIDPass", "AuthToken", "Authorization", "BindPassword",
		"ClientSecret", "CometBucketKey", "Cookie", "Credential", "CredentialJSON", "CustomBody", "CustomHeaders",
		"EncryptedEncryptionKey", "EncryptionKey", "FTPPassword", "InstallToken", "Key", "LocalcopyWinSMBPassword",
		"NewBucketKey", "Office365Credential", "PartnerKey", "Passphrase", "Password", "PasswordHash", "PasswordRecovery",
		"PfxFilePassword", "ReadWriteKey", "S3AccessKey", "S3SecretKey", "SFTPPassword", "SFTPPrivateKey", "SMBPassword",
		"SMTPPassword", "SSHPassword", "SSHPrivateKey", "SSLCertKey", "SSLClientKeyPassword", "SSLPfxPassword", "Secret",
		"SecretKey", "SessionKey", "TOTP", "TOTPKey", "Token", "WebAuthnSign", "WindowsCodeSignAzureAppSecret",
		"WindowsCodeSignPKCS11Password", "WindowsCodeSignPKCS12Password",
	},
	MCPArguments: []string{
		"APIKey", "AZBAccountKey", "AccessKey", "AppKey", "AppleIDPass", "AuthToken", "Authorization", "BindPassword",
		"ClientSecret", "CometBucketKey", "Cookie", "Credential", "CredentialJSON", "CustomBody", "CustomHeaders",
		"EncryptedEncryptionKey", "EncryptionKey", "FTPPassword", "InstallToken", "Key", "LocalcopyWinSMBPassword",
		"NewBucketKey", "Office365Credential", "PartnerKey", "Passphrase", "Password", "PasswordHash", "PasswordRecovery",
		"PfxFilePassword", "ReadWriteKey", "S3AccessKey", "S3SecretKey", "SFTPPassword", "SFTPPrivateKey", "SMBPassword",
		"SMTPPassword", "SSHPassword", "SSHPrivateKey", "SSLCertKey", "SSLClientKeyPassword", "SSLPfxPassword", "Secret",
		"SecretKey", "SessionKey", "TOTP", "TOTPKey", "Token", "WebAuthnSign", "WindowsCodeSignAzureAppSecret",
		"WindowsCodeSignPKCS11Password", "WindowsCodeSignPKCS12Password",
		"TargetPassword", "NewPassword", "OldPassword",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "COMET_BACKUP_BASE_URL",
		Default:   "https://{server}",
	},
	Origins: []credkit.Origin{
		// Every credential goes only to the Comet Server this process is
		// configured for: a Comet Server's host is the MSP's, so no hosted
		// domain can be named here. credentialFiles resolves it as the program
		// will (COMET_BACKUP_BASE_URL, COMET_BACKUP_SERVER, config) and shows it
		// to the kit. The connector's own origin pin (api-origin-pinned) narrows
		// every request and redirect hop to the same origin.
		{Source: "*", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:COMET_BACKUP_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// config:<key> for every key in cliutil.NotCredentials, which the per-read
		// hold leaves unheld too (TestIdentifiersMatchConfigNotCredentials).
		{Source: "config:base_url", Reason: "the Comet Server URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:backup_username", Reason: "the admin account name, printed by doctor and auth status; the password is the credential"},
		{Source: "config:server", Reason: "the Comet Server host name (template_vars.server), printed in every API URL"},
		{Source: "config:template_vars.server", Reason: "the Comet Server host name, printed in every API URL"},
		{Source: "env:COMET_BACKUP_USERNAME", Reason: "the admin account name, printed by doctor and auth status; the password is the credential"},
		{Source: "env:COMET_BACKUP_SERVER", Reason: "the Comet Server host name, printed in every API URL"},
		{Source: "env:COMET_BACKUP_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:COMET_BACKUP_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:COMET_BACKUP_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:COMET_BACKUP_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:COMET_BACKUP_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:COMET_BACKUP_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:COMET_BACKUP_DATA_DIR", Reason: "directory path"},
		{Source: "env:COMET_BACKUP_STATE_DIR", Reason: "directory path"},
		{Source: "env:COMET_BACKUP_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:COMET_BACKUP_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:COMET_BACKUP_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:COMET_BACKUP_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:COMET_BACKUP_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:COMET_BACKUP_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:COMET_BACKUP_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:COMET_BACKUP_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:COMET_BACKUP_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:COMET_BACKUP_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside a URL is held by the outbound guard when sent"},
		{Source: "env:COMET_BACKUP_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:COMET_BACKUP_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:COMET_BACKUP_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:COMET_BACKUP_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:COMET_BACKUP_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
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
// credentials a flag scan cannot name, shows the kit this process's Comet
// Server, and names every file a credential may be read from. The command line
// is read the way the program reads it: the CLI's own command tree finds the
// command and pflag parses its flags, so a value that looks like a flag
// (--notes --home=x) stays the value it is. When the command line does not
// parse (an unknown command or flag, the MCP server's own flags, a --home the
// program would refuse), every file it could mean is named, the environment's
// first: holding a file the program does not read is safe, missing the one it
// reads is not.
func credentialFiles(args []string) []string {
	holdArgvCredentials(args)
	configPath, home, parsed := cobraPathFlags(args)
	if !parsed {
		configPath, home = lastOf(credkit.FlagValues(args, "--config")), lastOf(credkit.FlagValues(args, "--home"))
	}
	showBaseURL(configPath, home)
	holdProfileStores(home)
	if parsed {
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

// holdArgvCredentials holds the value of every credential-named flag, as
// --name=value or --name value, whether the command declares it or not, so a
// flag error or a preview cannot print it. A non-boolean flag takes the next
// argument whatever it starts with, so that argument is held too. Only a text
// flag (or one this command does not declare) carries a credential: an int or
// bool flag named like one (users add --require-password-change 1) holds a
// switch. Other arguments are free text, which is no credential source
// (credential leak contract section 1).
func holdArgvCredentials(args []string) {
	root := cli.RootCmd()
	cmd, _, _ := root.Find(args)
	for i, a := range args {
		if a == "--" {
			return
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || !cliutil.SecretFieldName(name) {
			continue
		}
		if typ := flagType(cmd, name); typ != "" && typ != "string" {
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				continue
			}
			value = args[i+1]
		}
		if value = strings.TrimSpace(value); value != "" {
			credkit.Hold(credkit.Source("flag:--"+name), value)
		}
	}
}

// flagType is the value type cmd or an ancestor declares for name ("" when
// none declares it).
func flagType(cmd *cobra.Command, name string) string {
	for c := cmd; c != nil; c = c.Parent() {
		for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
			if f := fs.Lookup(name); f != nil {
				return f.Value.Type()
			}
		}
	}
	return ""
}

// savedBaseURL is COMET_BACKUP_BASE_URL as it was before showBaseURL set it.
var (
	savedBaseURL    string
	savedBaseURLSet bool
	baseURLShown    bool
)

// showBaseURL tells the kit which origin "base_url" means for this process. The
// kit reads Declarations.BaseURL, whose default is a template it cannot resolve,
// and a Comet Server can be any host, so credentials could go nowhere otherwise.
// It reads the variable right after Files returns; AfterStart puts the variable
// back, so nothing the program does later sees a change. When nothing resolves,
// the variable is left alone and the kit allows no origin: a request carrying a
// credential is refused.
func showBaseURL(configPath, home string) {
	base := configuredBase(configPath, home)
	if base == "" {
		return
	}
	// The kit reads a base URL that parses; this one may not (a raw / in its
	// password), and its credentials are held all the same.
	credkit.HoldURL(credkit.Source("userinfo"), base)
	savedBaseURL, savedBaseURLSet = os.LookupEnv("COMET_BACKUP_BASE_URL")
	baseURLShown = os.Setenv("COMET_BACKUP_BASE_URL", base) == nil
}

// configuredBase is the base URL config.Load resolves under this command line,
// with its {placeholders} filled in, or "" when one is left or Load refuses.
func configuredBase(configPath, home string) string {
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

// AfterStart is the statement after credkit.Start in both mains, before anything
// can print. It puts COMET_BACKUP_BASE_URL back as the process found it.
func AfterStart() {
	if !baseURLShown {
		return
	}
	baseURLShown = false
	if savedBaseURLSet {
		_ = os.Setenv("COMET_BACKUP_BASE_URL", savedBaseURL)
	} else {
		_ = os.Unsetenv("COMET_BACKUP_BASE_URL")
	}
}

func lastOf(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
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

// holdProfileStores holds, inside Start, what a run profile store carries that
// is a credential source (brief item 8(c)): every value saved under a
// credential flag's name, and the password and query values of a value saved
// under base-url, every duplicate included. Profile names, descriptions and
// other values are free text. The current store and the legacy one are read;
// a store that does not parse is the profile command's error to report.
func holdProfileStores(home string) {
	if restore, err := cliutil.SetHomeOverride(home); err == nil {
		if dir, err := cliutil.ConfigDir(); err == nil {
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if userHome, err := os.UserHomeDir(); err == nil {
		holdProfileStore(filepath.Join(userHome, ".comet-backup-cli", "profiles.json"))
	}
}

func holdProfileStore(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		key, ok := tok.(string)
		if !ok || !dec.More() {
			continue
		}
		// A key is followed by its value; read the value when it is a string.
		next, err := dec.Token()
		if err != nil {
			return
		}
		v, isString := next.(string)
		if !isString || v == "" {
			continue
		}
		switch {
		case strings.EqualFold(strings.ReplaceAll(key, "_", "-"), "base-url"):
			credkit.HoldURL(credkit.Source("config:profile.base-url"), v)
		case cliutil.SecretFieldName(key):
			credkit.Hold(credkit.Source("config:profile."+key), v)
		}
	}
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
	files, err := config.ReadPaths(configPath)
	if err != nil {
		return nil, false
	}
	return files, true
}
