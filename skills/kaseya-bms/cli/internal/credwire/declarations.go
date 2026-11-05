// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/config"
	"kaseya-bms-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// Kaseya BMS authenticates with a short-lived JWT: a token given in the
// environment or stored by auth set-token, or one the connector mints from
// the API user's login (KASEYA_BMS_USERNAME, KASEYA_BMS_PASSWORD,
// KASEYA_BMS_TENANT) at POST /v2/security/authenticate on the API host and
// holds itself (config.MintKbmsToken). Only the password of the login is a
// credential; the username and the tenant (company) name are identifiers the
// API prints on records (TenantName).
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"KASEYA_BMS_TOKEN",
		"KASEYA_BMS_BEARER_AUTH",
		"KASEYA_BMS_PASSWORD",
		"KASEYA_BMS_MCP_HTTP_TOKEN",
	},
	// The feedback endpoint is a URL, so the variable is an identifier below,
	// but a URL can carry a password or query value: those are held.
	URLEnv: []string{"KASEYA_BMS_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	// The credential keys of config.toml and the 0600 credentials.toml store,
	// and the credential flags' names, which a document HoldBody reads may
	// carry a flag value under (a saved profile's values are held by
	// HoldProfileStore, from CredentialFlags). Not "code": --code (the OAuth
	// authorization code of security authenticate) is held from the command
	// line, its request (CredentialFlags, client.holdCredentialEndpointRequest)
	// and a saved profile, as a declared "code" field would hold every account
	// code HoldBody reads.
	ConfigFields: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_secret",
		"bms_token",
		"bms_bearer_auth",
		"password",
		"client-secret",
		"mfa-code",
		"itc-token",
		"saml-key",
		"saml-response",
		"access-token",
		"refresh-token",
		"password-or-token",
		"remote-control-client-secret",
		"secret",
		"client-id",
		"remote-control-client-id",
	},
	StoredRecords: []string{
		"auth_header",
		"access_token",
		"refresh_token",
		"client_secret",
		"bms_token",
		"bms_bearer_auth",
	},
	// Every flag whose value is a credential: the security authenticate login
	// form, security refresh-token, the RMM integration settings, a webhook's
	// signing secret, auth login's one-time code, and the public half of a
	// client pair (the RMM RemoteControl ClientId and the QuickBooks export
	// ClientId), held as the connector held every clientid before the kit.
	CredentialFlags: credentialFlags,
	// --stdin on a write command reads its JSON request body (auth set-token
	// reads its token from stdin without a flag, and holds it itself).
	StdinFlags: []string{"--stdin"},
	// The request body fields that carry a credential, from the vendor spec:
	// the security authenticate login form (its OAuth Code and every other
	// value of a request to a credential endpoint are held by
	// client.holdCredentialEndpointRequest), TokenOutputDto's token pair on
	// security refresh-token, IntegrationSettingDto.PasswordOrToken and
	// RemoteControl's client pair, and the webhook signing Secret.
	CredentialFields: []string{
		"Password",
		"ClientSecret",
		"MFACode",
		"ItcToken",
		"SamlKey",
		"SamlResponse",
		"AccessToken",
		"RefreshToken",
		"PasswordOrToken",
		"Secret",
		"ClientId",
	},
	// The vendor response fields that carry a credential: TokenOutputDto,
	// the IT Glue and VSA access info AccessToken, the Pulseway remote
	// session Token and AuthorizationLink, and the FullCalendar license key.
	// The IT Glue password value is a bare Result string, named nothing; it
	// is held by the API client's guard (cli.holdCredentialResult), as a
	// declared Result would hold every record.
	ResponseFields: []string{
		"AccessToken",
		"RefreshToken",
		"Password",
		"ClientSecret",
		"MFACode",
		"ItcToken",
		"SamlKey",
		"SamlResponse",
		"PasswordOrToken",
		"Secret",
		"Token",
		"AuthorizationLink",
		"FullCalendarLicenseKey",
		"ClientId",
	},
	// The MCP tools take the body fields by name and the nested RMM client
	// secret by its flag name. Not "Code": see ConfigFields.
	MCPArguments: []string{
		"Password",
		"ClientSecret",
		"MFACode",
		"ItcToken",
		"SamlKey",
		"SamlResponse",
		"AccessToken",
		"RefreshToken",
		"PasswordOrToken",
		"Secret",
		"ClientId",
		"remote-control-client-secret",
		"remote-control-client-id",
		"client-id",
	},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "KASEYA_BMS_BASE_URL",
		Default:   "https://api.bms.kaseya.com",
	},
	Origins: []credkit.Origin{
		// The configured API host, and the three Kaseya BMS regions by name
		// (US, EMEA, APAC), so a credential store saved for one region does
		// not decide the origin of a run whose config names none. The
		// connector's own origin pin (api-origin-pinned) narrows every request
		// to the one configured host, redirects included.
		{Source: "*", Allowed: []string{"base_url", "https://api.bms.kaseya.com", "https://api.bmsemea.kaseya.com", "https://api.bmsapac.kaseya.com"}},
		// The MCP HTTP bearer authenticates callers of this server; it is
		// sent nowhere.
		{Source: "env:KASEYA_BMS_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		// Config and credential store keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "an expiry time"},
		{Source: "config:client_id", Reason: "the minted token's binding: a fingerprint of the API host, tenant and username (config.KbmsTokenBinding), never a secret"},
		{Source: "env:KASEYA_BMS_USERNAME", Reason: "the API user's name, an identifier; only the login's password is a credential"},
		{Source: "env:KASEYA_BMS_TENANT", Reason: "the tenant (company) name, printed on every record as TenantName"},
		{Source: "env:KASEYA_BMS_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:KASEYA_BMS_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:KASEYA_BMS_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:KASEYA_BMS_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:KASEYA_BMS_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:KASEYA_BMS_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_BMS_DATA_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_BMS_STATE_DIR", Reason: "directory path"},
		{Source: "env:KASEYA_BMS_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:HOME", Reason: "directory path"},
		{Source: "env:KASEYA_BMS_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:KASEYA_BMS_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:KASEYA_BMS_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:KASEYA_BMS_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:KASEYA_BMS_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:KASEYA_BMS_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:KASEYA_BMS_MCP_UPLOAD_ROOT", Reason: "directory the MCP server may read upload files from"},
		{Source: "env:KASEYA_BMS_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:KASEYA_BMS_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:KASEYA_BMS_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:KASEYA_BMS_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:KASEYA_BMS_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:KASEYA_BMS_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:KASEYA_BMS_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:KASEYA_BMS_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:CODEX_THREAD_ID", Reason: "agent harness session id; only a bounded hash is journaled"},
		{Source: "env:CODEX_SESSION_ID", Reason: "agent harness session id; only a bounded hash is journaled"},
		{Source: "env:CLAUDE_SESSION_ID", Reason: "agent harness session id; only a bounded hash is journaled"},
		{Source: "env:CLAUDE_CODE_SESSION", Reason: "agent harness session id; only a bounded hash is journaled"},
		{Source: "env:CURSOR_SESSION_ID", Reason: "agent harness session id; only a bounded hash is journaled"},
		{Source: "env:CURSOR_TRACE_ID", Reason: "agent harness session id; only a bounded hash is journaled"},
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
		{Source: "env:USERNAME", Reason: "the Windows account name, for the private-file ACL"},
		{Source: "env:SYSTEMROOT", Reason: "Windows directory path"},
		{Source: "env:PATH", Reason: "executable search path"},
		{Source: "env:GODEBUG", Reason: "Go runtime settings"},
	},
}

// credentialFiles is every file the program may read a credential from,
// given the command line, the one the program uses first: the config file
// and credential store under the --home the program applies (the last
// --home; config.CredentialFiles resolves KASEYA_BMS_CONFIG, the default
// path and the legacy one by Load's own rules), then those under every other
// --home and the default home, and the store beside each --config (the kit
// reads a --config file itself, and its base_url decides). Holding a file the
// program does not read is safe; missing the one it reads is not. It also
// holds the URL a --deliver flag names (its webhook's userinfo and query),
// and the credentials saved in profiles.json under every one of those homes
// and the legacy ~/.kaseya-bms-cli (holdProfileStore). The profile store is
// never returned to the kit as a credential file: the kit holds every string
// of nested JSON, so a saved profile's names and ordinary values ("daily",
// "select", "Id") would be held and ordinary program text would then refuse
// startup.
func credentialFiles(args []string) []string {
	for _, v := range credkit.FlagValues(args, "--deliver") {
		credkit.HoldURL("flag:--deliver", v)
	}
	var out, stores []string
	add := func(paths ...string) {
		for _, p := range paths {
			if p != "" && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	profiles := func(p string) {
		if !slices.Contains(stores, p) {
			stores = append(stores, p)
			holdProfileStore(p)
		}
	}
	homes := slices.Clone(credkit.FlagValues(args, "--home"))
	slices.Reverse(homes)
	for _, h := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(h)
		if err != nil {
			continue
		}
		add(config.CredentialFiles()...)
		if dir, err := cliutil.ConfigDir(); err == nil {
			profiles(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	for _, c := range credkit.FlagValues(args, "--config") {
		if p, err := cliutil.CredentialsFilePathForConfig(c); err == nil {
			add(p)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		profiles(filepath.Join(home, ".kaseya-bms-cli", "profiles.json"))
	}
	return out
}

// profileCredentialNames is every name a saved profile keeps a credential
// under: each credential flag's name as profiles.json keeps it ("password"
// for --password, "code" for --code), and base-url. Set from
// credentialFlags (Declarations.CredentialFlags) by init, so the two never
// differ.
var profileCredentialNames []string

// credentialFlags is Declarations.CredentialFlags, bound to its own name so the
// Declarations value is read only by credkit.Start (check_env_schema.py treats
// any other read of it as an environment read it cannot resolve).
var credentialFlags = []string{
	"--password",
	"--client-secret",
	"--code",
	"--mfa-code",
	"--itc-token",
	"--saml-key",
	"--saml-response",
	"--access-token",
	"--refresh-token",
	"--password-or-token",
	"--remote-control-client-secret",
	"--secret",
	"--client-id",
	"--remote-control-client-id",
}

func init() {
	for _, f := range credentialFlags {
		profileCredentialNames = append(profileCredentialNames, strings.TrimPrefix(f, "--"))
	}
}

// holdProfileStore holds the credentials saved in the profile store at path
// (HoldProfileStore). Only a regular file is read, so a pipe named
// profiles.json never blocks startup.
func holdProfileStore(path string) {
	if data, ok := readRegularFile(path); ok {
		HoldProfileStore(data)
	}
}

// HoldProfileStore holds the credentials a profile store saves, read as the
// program reads the store (cli.loadProfileStore, which calls this too: one
// JSON object {"profiles": {<name>: {"name", "description", "values":
// {<flag>: <value>}}}}): in a profile's values, every value under a
// credential flag's name (code among them) and the password and query values
// of every value under "base-url" or "base_url". Every member of a duplicate
// name is held: the token stream sees each one. Nothing else in the store is
// a credential: a profile's name and description, and every other flag value,
// an account code among them. So the store is never read as a whole document
// under the declared fields (credkit.HoldBody): a profile named "password"
// would hold every string of that profile ("name", "select", "Id"), and
// ordinary program text would then refuse startup. A store that is not valid
// JSON holds nothing: the program reads no profile from it either (its loader
// fails on it), and holding every word of it would refuse startup over
// ordinary program text (.review/followups.md, a P2 acquisition edge case).
func HoldProfileStore(data []byte) {
	if !json.Valid(data) {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	_ = walkProfileStore(dec, nil, true)
}

// walkProfileStore reads one JSON value from dec. path is the member names
// from the top of the document down to it; live is false inside an array,
// which the program's loader reads no profile from. Member names that
// Go's decoder matches to a struct field ("profiles", "values") are compared
// as it compares them, without case.
func walkProfileStore(dec *json.Decoder, path []string, live bool) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, isDelim := tok.(json.Delim)
	if !isDelim {
		if live {
			holdProfileValue(path, tok)
		}
		return nil
	}
	for dec.More() {
		if d == '[' {
			if err := walkProfileStore(dec, path, false); err != nil {
				return err
			}
			continue
		}
		k, err := dec.Token()
		if err != nil {
			return err
		}
		name, _ := k.(string)
		if err := walkProfileStore(dec, append(slices.Clip(path), name), live); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

// holdProfileValue holds one scalar of the store when it is a profile's
// value under a credential flag's name or base-url. A value the program
// cannot read as a string (an object, an array, a number, true or false)
// fails its loader, so nothing of it prints; a number is held as written all
// the same, and null reads as no value.
func holdProfileValue(path []string, tok json.Token) {
	if len(path) != 4 || !strings.EqualFold(path[0], "profiles") || !strings.EqualFold(path[2], "values") {
		return
	}
	var v string
	switch t := tok.(type) {
	case string:
		v = t
	case json.Number:
		v = t.String()
	default:
		return
	}
	switch name := path[3]; {
	case name == "base-url" || name == "base_url":
		credkit.HoldURL("profile:base-url", v)
	case slices.Contains(profileCredentialNames, name):
		credkit.Hold("profile:"+name, v)
	}
}

// readRegularFile reads path only when it is a regular file, and only the
// file the first look saw: a symbolic link is followed, as the program's own
// read follows it.
func readRegularFile(path string) ([]byte, bool) {
	dir := os.DirFS(filepath.Dir(path))
	name := filepath.Base(path)
	fi, err := fs.Stat(dir, name)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, false
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	got, err := f.Stat()
	if err != nil || !got.Mode().IsRegular() || !os.SameFile(fi, got) {
		return nil, false
	}
	b, err := io.ReadAll(f)
	return b, err == nil
}
