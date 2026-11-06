// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/config"
	"aws-billing-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
// AWS authenticates through the AWS SDK v2 default chain: the environment,
// the shared credentials and config files, credential_process, the SSO and
// login token caches, a web identity token file and the container credential
// endpoint (EC2 instance metadata is off: internal/awsx r1b-imds-off). Requests are SigV4: the access key ID is in the
// Authorization header, the session token in X-Amz-Security-Token, and the
// secret key is never sent. What the SDK resolves or refreshes at run time
// is held as it is retrieved (internal/awsx holdingProvider). The press
// config.toml (auth_header, [headers]) is the generated client's.
//
// The SDK writes the SSO and login token caches itself when it refreshes a
// token; those files are credential sources (named in Files), and the write
// is the SDK's own, outside this program's code.
var Declarations = credkit.Declarations{
	SecretEnv: []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SECRET_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_SECURITY_TOKEN",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN",
		"AWS_BILLING_MCP_HTTP_TOKEN",
	},
	// The container credential endpoint is a URL: a password or query value
	// inside it is held.
	URLEnv: []string{"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_BILLING_FEEDBACK_ENDPOINT"},
	Files:  credentialFiles,
	ConfigFields: []string{
		"auth_header",
		"aws_access_key_id",
		"aws_secret_access_key",
		"aws_session_token",
		"aws_security_token",
		// The SSO and login token caches.
		"accessToken",
		"refreshToken",
		"idToken",
		"clientSecret",
		"accessKeyId",
		"secretAccessKey",
		"sessionToken",
		"dpopKey",
	},
	// feedback --stdin reads its note from stdin.
	StdinFlags: []string{"--stdin"},
	// The SSO portal's bearer token and the session token travel in headers
	// of their own.
	CredentialHeaders: []string{"X-Amz-Security-Token", "X-Amz-Sso_bearer_token"},
	// SSO OIDC CreateToken sends the client secret and the refresh token;
	// STS AssumeRoleWithWebIdentity sends the web identity token.
	CredentialFields: []string{"clientSecret", "refreshToken", "WebIdentityToken"},
	// The credentials an AWS credential service answers with (SSO
	// GetRoleCredentials, SSO OIDC, STS, the container and instance metadata
	// endpoints).
	ResponseFields: []string{"AccessKeyId", "SecretAccessKey", "SessionToken", "Token", "accessToken", "refreshToken", "idToken", "clientSecret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "AWS_BILLING_BASE_URL",
		// The generated client's nominal base URL; the AWS doctor never calls it.
		Default: "https://ce.us-east-1.amazonaws.com",
	},
	// A credential may go to the AWS endpoints of every partition's DNS suffix
	// the SDK uses for the services this connector calls, and to the generated
	// client's base URL. The SDK client's own pin (internal/awsx
	// awsEndpointAllowed, aws-endpoint-pinned) narrows every AWS request
	// further to those services, refuses a custom endpoint and refuses every
	// redirect.
	Origins: []credkit.Origin{
		{Source: "*", Allowed: []string{
			"https://*.amazonaws.com",
			"https://*.amazonaws.com.cn",
			"https://*.amazonaws.eu",
			"https://*.api.aws",
			"https://*.api.amazonwebservices.com.cn",
			"base_url",
		}},
		// config.toml's auth_header is the generated client's credential: it goes
		// to that client's base URL only, never to AWS.
		{Source: "config:auth_header", Allowed: []string{"base_url"}},
		{Source: "minted:authorization", Allowed: []string{"base_url"}},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:AWS_BILLING_MCP_HTTP_TOKEN", Allowed: nil},
	},
	Identifiers: []credkit.Identifier{
		{Source: "header:authorization", Reason: "a SigV4 signature, made for one request: the access key ID inside it is held from the credential provider (minted:aws-credentials) and the secret key is never sent"},
		// Config keys that are not credentials.
		{Source: "config:base_url", Reason: "the API base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "env:AWS_BILLING_BASE_URL", Reason: "the API base URL; a credential inside it is held through BaseURL"},
		{Source: "env:AWS_BILLING_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:AWS_BILLING_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:AWS_BILLING_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:AWS_BILLING_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:AWS_BILLING_CONFIG_DIR", Reason: "directory path"},
		{Source: "env:AWS_BILLING_DATA_DIR", Reason: "directory path"},
		{Source: "env:AWS_BILLING_STATE_DIR", Reason: "directory path"},
		{Source: "env:AWS_BILLING_CACHE_DIR", Reason: "directory path"},
		{Source: "env:AWS_BILLING_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:AWS_BILLING_SLACK_CHANNEL", Reason: "a Slack channel or DM ID"},
		{Source: "env:AWS_BILLING_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:AWS_BILLING_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:AWS_BILLING_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:AWS_BILLING_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:AWS_BILLING_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:AWS_BILLING_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:AWS_BILLING_MCP_HTTP_ALLOWED_ORIGINS", Reason: "list of browser origins allowed to call the MCP HTTP server"},
		{Source: "env:AWS_BILLING_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv"},
		{Source: "env:AWS_BILLING_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:AWS_BILLING_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:AWS_BILLING_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:AWS_BILLING_LEARN_SESSION", Reason: "session grouping label"},
		{Source: "env:AWS_BILLING_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:AWS_PROFILE", Reason: "name of an AWS shared-config profile"},
		{Source: "env:AWS_DEFAULT_PROFILE", Reason: "name of an AWS shared-config profile"},
		{Source: "env:AWS_REGION", Reason: "an AWS region name"},
		{Source: "env:AWS_DEFAULT_REGION", Reason: "an AWS region name"},
		{Source: "env:AWS_CONFIG_FILE", Reason: "path of the AWS config file, whose credential keys are held"},
		{Source: "env:AWS_SHARED_CREDENTIALS_FILE", Reason: "path of the AWS shared credentials file, which Files names"},
		{Source: "env:AWS_WEB_IDENTITY_TOKEN_FILE", Reason: "path of the web identity token file, which Files names"},
		{Source: "env:AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", Reason: "path of the container authorization token file, which Files names"},
		{Source: "env:AWS_CONTAINER_CREDENTIALS_FULL_URI", Reason: "the container credential endpoint; a credential inside it is held through URLEnv"},
		{Source: "env:AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", Reason: "a path on the ECS credential endpoint"},
		{Source: "env:AWS_LOGIN_CACHE_DIRECTORY", Reason: "directory of the login token cache, whose files Files names"},
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

// credentialFiles runs inside Start, before anything can print. It returns
// the files the kit reads token by token: the press config the program will
// really use first (the kit takes the base URL from the first file naming
// one; an explicit --config it reads itself), then every other config any
// --home could mean, then the AWS credential files. Files the kit would read
// too widely are held here instead: the AWS config file (holdAWSConfig) and
// the run profile store (holdProfileStore). A path is named as written and
// trimmed, since the SDK opens an environment path as written (Codex r2 class
// holder-incomplete): naming a file the program does not read is safe,
// missing the one it reads is not.
func credentialFiles(args []string) []string {
	var files []string
	add := func(paths ...string) {
		for _, p := range paths {
			for _, f := range []string{p, strings.TrimSpace(p)} {
				if f != "" && !slices.Contains(files, f) {
					files = append(files, f)
				}
			}
		}
	}
	add(os.Getenv("AWS_BILLING_CONFIG"))
	homes := credkit.FlagValues(args, "--home")
	if len(homes) > 0 {
		homes = append([]string{homes[len(homes)-1]}, homes...) // the last --home is the one the run uses
	}
	for _, home := range append(homes, "") {
		restore, err := cliutil.SetHomeOverride(home)
		if err != nil {
			continue // the run refuses this --home before it reads a file
		}
		if dir, err := cliutil.ConfigDir(); err == nil {
			add(filepath.Join(dir, "config.toml"))
			holdProfileStore(filepath.Join(dir, "profiles.json"))
		}
		restore()
	}
	if legacy, err := config.LegacyConfigPath(); err == nil {
		add(legacy)
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		holdProfileStore(filepath.Join(home, ".aws-billing-cli", "profiles.json"))
	}

	add(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
	add(os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"), os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"))
	awsConfigs := []string{os.Getenv("AWS_CONFIG_FILE"), strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE"))}
	ssoDirs := []string{}
	loginDirs := []string{os.Getenv("AWS_LOGIN_CACHE_DIRECTORY"), strings.TrimSpace(os.Getenv("AWS_LOGIN_CACHE_DIRECTORY"))}
	if home != "" {
		add(filepath.Join(home, ".aws", "credentials"))
		awsConfigs = append(awsConfigs, filepath.Join(home, ".aws", "config"))
		ssoDirs = append(ssoDirs, filepath.Join(home, ".aws", "sso", "cache"))
		loginDirs = append(loginDirs, filepath.Join(home, ".aws", "login", "cache"))
	}
	for _, p := range awsConfigs {
		if p != "" {
			add(holdAWSConfig(p)...)
		}
	}
	for _, dir := range append(ssoDirs, loginDirs...) {
		if dir == "" {
			continue
		}
		if m, err := filepath.Glob(filepath.Join(dir, "*.json")); err == nil {
			add(m...)
		}
	}
	return files
}

// awsConfigCredentialKeys are the keys of the AWS config file whose values
// are credentials.
var awsConfigCredentialKeys = []string{"aws_access_key_id", "aws_secret_access_key", "aws_session_token", "aws_security_token"}

// holdAWSConfig holds the credential keys of an AWS config file and returns
// the web identity token files its profiles name. The kit does not read this
// file: a "[profile name]" section header is outside the canonical subset the
// kit reads precisely, so it would hold every value in the file (every
// region and output name among them) and withhold ordinary output. The
// reading is the SDK's (config/internal/ini): a "key = value" or "key: value"
// line under any section, key case folded, the value as written and trimmed,
// and with a trailing comment cut. Only a regular file is opened.
func holdAWSConfig(path string) (tokenFiles []string) {
	data, ok := readRegular(path)
	if !ok {
		return nil
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), len(data)+1)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.IndexAny(line, "=:")
		if i <= 0 || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		raw := line[i+1:]
		vals := []string{raw, strings.TrimSpace(raw)}
		if j := strings.IndexAny(raw, "#;"); j > 0 {
			vals = append(vals, strings.TrimSpace(raw[:j]))
		}
		switch {
		case slices.Contains(awsConfigCredentialKeys, key):
			credkit.Hold("config:"+key, vals...)
		case key == "web_identity_token_file":
			// Hand-fix (handfixes.json r1c-config-token-file-as-opened): only the
			// path the SDK opens. An untrimmed variant is no file name on Windows
			// (" C:\..."), and a path the kit cannot stat marks the held set
			// unknown, which withholds every write of the run.
			tokenFiles = append(tokenFiles, sdkPropertyValue(raw))
		}
	}
	return tokenFiles
}

// sdkPropertyValue is a config property value as the SDK reads it
// (config/internal/ini trimPropertyComment, then TrimSpace): cut at the first
// " #", " ;", "\t#" or "\t;", then trimmed.
func sdkPropertyValue(raw string) string {
	for _, sep := range []string{" #", " ;", "\t#", "\t;"} {
		raw, _, _ = strings.Cut(raw, sep)
	}
	return strings.TrimSpace(raw)
}

// holdProfileStore holds what a run profile store carries under a credential
// name or base-url, every duplicate key included. The kit is not given the
// file: it is nested JSON, so the kit would hold every string in it, and a
// profile's plain "true" would refuse every start. Only a regular file is
// read: a pipe named profiles.json would hang startup.
func holdProfileStore(path string) {
	data, ok := readRegular(path)
	if !ok {
		return
	}
	credkit.HoldBody(data) // values under a credential name, every duplicate; every word of a file that does not parse
	dec := json.NewDecoder(bytes.NewReader(data))
	key := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		if s, ok := tok.(string); ok {
			if key == "base-url" || key == "base_url" {
				credkit.HoldURL("profile:base-url", s)
			}
			key = s
			continue
		}
		key = ""
	}
}

func readRegular(path string) ([]byte, bool) {
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	fsys := os.DirFS(dir)
	if info, err := fs.Stat(fsys, name); err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := fs.ReadFile(fsys, name)
	return data, err == nil
}
