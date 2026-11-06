// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): this connector's one credkit.Declarations
// literal. Pinned by this connector's handfixes.json under the "credkit-wiring"
// entry. The connector's own credential stores (the shared credentials file
// `auth set-credentials` writes, the per-server store `servers login` writes)
// and its config file are credential SOURCES: their writes go outside the kit,
// which withholds them by contract, and Files lists every one of those files.
//
// The WinRM leg (internal/winrm) sends each server's password to the host the
// user configured for that server in [[servers]]. No static Origin can name a
// host that comes from config, so that client stays outside credkit.Transport,
// as credkit-sites.json records; winrm.New pins it (ValidHost, no proxy, no
// redirect, TLS verified or SHA-256 pinned). The press client's requests go to
// the virtual base https://winrm.invalid, answered in process, through
// credkit.Transport.

package cli

import (
	"os"
	"path/filepath"

	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/credkit"
	"windows-server-pp-cli/internal/winsrv"
)

// Declarations is what this connector declares about its credentials.
var Declarations = credkit.Declarations{
	// The shared WinRM password, and the bearer token the MCP HTTP transport
	// checks on every request.
	SecretEnv: []string{"WINDOWS_SERVER_PASSWORD", "WINDOWS_SERVER_MCP_HTTP_TOKEN"},
	// The feedback endpoint is a URL; a password or query value in it is held.
	URLEnv: []string{"WINDOWS_SERVER_FEEDBACK_ENDPOINT"},
	// Every config, credentials and profile file the command line can mean.
	Files: credentialFiles,
	// The credential keys of the config and the shared credentials file
	// (auth set-credentials writes server_password; the generated OAuth
	// fields are kept so a token saved by an older build is held too).
	ConfigFields: []string{"server_password", "access_token", "refresh_token", "client_secret"},
	// The per-server store servers login writes: one password per server.
	StoredRecords: []string{"password"},
	// No flag takes a credential: auth set-credentials takes the password as
	// its second argument (held by holdPasswordArg in winsrv_common.go), and
	// servers login reads it from stdin.
	CredentialFlags: nil,
	// No flag turns stdin into a credential: servers login always reads its
	// password from stdin and holds it as it reads it (servers.go); feedback
	// --stdin reads feedback text.
	StdinFlags: nil,
	// No MCP tool takes a credential: auth is never exposed over MCP, and
	// servers login is mcp:hidden.
	MCPArguments: nil,
	// The press client's requests go to this virtual base, answered in
	// process; the WinRM leg is outside the kit (see the file comment).
	BaseURL: credkit.BaseURL{ConfigKey: "base_url", Env: "WINDOWS_SERVER_BASE_URL", Default: "https://winrm.invalid"},
	// Every credential may reach only the virtual base.
	Origins: []credkit.Origin{{Source: "*", Allowed: []string{"base_url"}}},
	Identifiers: []credkit.Identifier{
		// Loop ruling 9 holds every string of a credential file outside the
		// canonical subset. This connector's files are never in it: the config
		// lists servers as a [[servers]] array of tables, and the per-server
		// store keys each entry by a quoted "<config>#<name>@<host>:<port>".
		// Every server name, host and username would be held, and every row
		// the connector exists to print withheld. Every declared credential
		// key (ConfigFields, StoredRecords, auth_header, a headers table, the
		// base URL's password and query values) is still held from these
		// files, a file that does not parse included.
		{Source: "config:imprecise", Reason: "the every-string hold of a file outside the canonical subset: the [[servers]] config and the per-server store are never in it, so it would hold every server name, host and username; every declared credential key is still held"},
		{Source: "basic-user", Reason: "the WinRM account name: servers list and doctor print it and servers report it (local-admins); its password is held"},
		{Source: "config:base_url", Reason: "the virtual API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "a path to a CA certificate file"},
		{Source: "config:tls_server_name", Reason: "a TLS host name"},
		{Source: "config:token_expiry", Reason: "a timestamp"},
		{Source: "config:client_id", Reason: "a public OAuth client id, sent in the clear by design"},
		{Source: "config:server_username", Reason: "the shared WinRM account name; its password is held"},
		{Source: "config:username", Reason: "a server's WinRM account name (config [[servers]] and the per-server store); its password is held"},
		{Source: "config:name", Reason: "a server name from [[servers]], what --server takes"},
		{Source: "config:host", Reason: "a server's DNS name or IP address"},
		{Source: "config:port", Reason: "a TCP port"},
		{Source: "config:transport", Reason: "https or http"},
		{Source: "config:auth", Reason: "ntlm or basic"},
		{Source: "config:ca_file", Reason: "a path to a CA certificate file"},
		{Source: "config:cert_sha256", Reason: "the server certificate's public SHA-256 fingerprint"},
		{Source: "config:default", Reason: "a switch"},
		// In-memory fields of the config and credential types (toml:"-" or
		// unexported): never a key of a file this program reads.
		{Source: "config:Path", Reason: "the path a config or credentials value was loaded from, set in memory"},
		{Source: "config:Source", Reason: "where a stored credential came from, a label set in memory"},
		{Source: "config:Err", Reason: "a load error kept in memory"},
		{Source: "config:AuthSource", Reason: "where the auth header came from, a label set in memory"},
		{Source: "config:CredentialSource", Reason: "where the credential came from, a label set in memory"},
		{Source: "config:configOwner", Reason: "the config file's owner, read from the file system"},
		{Source: "config:legacySourcePath", Reason: "the path of a migrated legacy config"},
		{Source: "config:envOverrides", Reason: "the names of environment variables that overrode a value"},
		// The learn entities file: query vocabulary, not credentials.
		{Source: "config:variant", Reason: "a query word variant in the learn entities file"},
		{Source: "config:canonical", Reason: "the canonical form of a query word in the learn entities file"},
		{Source: "config:stopwords", Reason: "query words the learn entities file ignores"},
		{Source: "config:synonyms", Reason: "query word synonyms in the learn entities file"},
		{Source: "env:WINDOWS_SERVER_USERNAME", Reason: "the shared WinRM account name; its password is held"},
		{Source: "env:WINDOWS_SERVER_CONFIG", Reason: "a path to the configuration file, which Files lists"},
		{Source: "env:WINDOWS_SERVER_HOME", Reason: "a directory"},
		{Source: "env:WINDOWS_SERVER_CONFIG_DIR", Reason: "a directory"},
		{Source: "env:WINDOWS_SERVER_DATA_DIR", Reason: "a directory"},
		{Source: "env:WINDOWS_SERVER_STATE_DIR", Reason: "a directory"},
		{Source: "env:WINDOWS_SERVER_CACHE_DIR", Reason: "a directory"},
		{Source: "env:WINDOWS_SERVER_CA_CERT", Reason: "a path to a CA certificate file"},
		{Source: "env:WINDOWS_SERVER_TLS_SERVER_NAME", Reason: "a TLS host name"},
		{Source: "env:WINDOWS_SERVER_USER_AGENT", Reason: "a User-Agent string"},
		{Source: "env:WINDOWS_SERVER_CLI_PATH", Reason: "a path to the companion CLI"},
		{Source: "env:WINDOWS_SERVER_DATA_SOURCE", Reason: "a mode name"},
		{Source: "env:WINDOWS_SERVER_FEEDBACK_AUTO_SEND", Reason: "a switch"},
		{Source: "env:WINDOWS_SERVER_FEEDBACK_ENDPOINT", Reason: "a URL; URLEnv holds any credential inside it"},
		{Source: "env:WINDOWS_SERVER_LEARN_NO_CAPTURE", Reason: "a switch"},
		{Source: "env:WINDOWS_SERVER_LEARN_SESSION", Reason: "a session name, stored only as a hash"},
		{Source: "env:WINDOWS_SERVER_LEARN_SURFACE", Reason: "a surface name"},
		{Source: "env:WINDOWS_SERVER_MAX_RESPONSE_BYTES", Reason: "a size"},
		{Source: "env:WINDOWS_SERVER_MCP_CHILD", Reason: "a marker this program sets on its own child"},
		{Source: "env:WINDOWS_SERVER_MCP_HTTP_ALLOWED_ORIGINS", Reason: "a list of browser origins"},
		{Source: "env:WINDOWS_SERVER_MCP_READ_ONLY", Reason: "a switch"},
		{Source: "env:WINDOWS_SERVER_NO_LEARN", Reason: "a switch"},
		{Source: "env:WINDOWS_SERVER_NO_LOCAL_DATA", Reason: "a switch"},
		{Source: "env:AGENT_ID", Reason: "an agent name, stored in a feedback entry"},
		{Source: "env:GODEBUG", Reason: "Go runtime settings"},
		{Source: "env:NO_COLOR", Reason: "a display switch"},
		{Source: "env:TERM", Reason: "a terminal type"},
		{Source: "env:USERNAME", Reason: "the local account name"},
		{Source: "env:SYSTEMROOT", Reason: "the Windows directory"},
		{Source: "env:PP_MCP_TRANSPORT", Reason: "the MCP transport name"},
		{Source: "env:PRESEED_DISABLED", Reason: "a switch"},
		{Source: "env:PRESEED_ROW_CAP", Reason: "a row count"},
		{Source: "env:PRINTING_PRESS_CLIENT_PROFILE", Reason: "a profile name"},
		{Source: "env:PRINTING_PRESS_DOGFOOD", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_VERIFY", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_VERIFY_LIVE_HTTP", Reason: "a test switch"},
		{Source: "env:PRINTING_PRESS_MCP_BOUND_PROFILE", Reason: "the client profile name an MCP server is bound to"},
		{Source: "env:XDG_CACHE_HOME", Reason: "a directory"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "a directory"},
		{Source: "env:XDG_DATA_HOME", Reason: "a directory"},
		{Source: "env:XDG_STATE_HOME", Reason: "a directory"},
	},
}

// credentialFiles is every file a credential may be read from under any
// --config and --home the command line can mean (credkit.FlagValues reads
// every spelling, past a "--" a flag may take as its value; holding a file the
// run does not load is safe where missing the one it loads is not): the config
// file (the explicit one, then the environment's, then the default), its
// legacy path, the shared credentials file beside an explicit config and the
// default one, the per-server store beside each, and the run profiles.
// The config the run selects comes first, so the base URL the kit binds
// credentials to is the one config.Load uses.
func credentialFiles(args []string) []string {
	configs := credkit.FlagValues(args, "--config")
	for i, j := 0, len(configs)-1; i < j; i, j = i+1, j-1 { // pflag keeps the last
		configs[i], configs[j] = configs[j], configs[i]
	}
	if env := os.Getenv("WINDOWS_SERVER_CONFIG"); env != "" {
		configs = append(configs, env)
	}
	configs = append(configs, "")
	var files []string
	for _, home := range append(credkit.FlagValues(args, "--home"), "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue
		}
		for _, c := range configs {
			path := c
			if path == "" {
				dir, err := cliutil.ConfigDir()
				if err != nil {
					continue
				}
				path = filepath.Join(dir, "config.toml")
			} else if creds, err := cliutil.CredentialsFilePathForConfig(path); err == nil {
				files = append(files, path, creds, winsrv.ServerCredentialsPath(creds))
				continue
			}
			files = append(files, path)
		}
		if creds, err := cliutil.CredentialsFilePath(); err == nil {
			files = append(files, creds, winsrv.ServerCredentialsPath(creds))
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			files = append(files, filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if legacy, err := config.LegacyConfigPath(); err == nil {
		files = append(files, legacy)
	}
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".windows-server-cli", "profiles.json"))
	}
	return files
}
