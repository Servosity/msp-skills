// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credwire is this connector's one wiring file for the shared
// credential kit (handfixes.json credkit-wiring): the Declarations literal
// both mains pass to credkit.Start. The SQLite driver every store opens
// through is internal/credsql. Hand-written; a reprint never regenerates it.
package credwire

import (
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"proxmox-pp-cli/internal/cli"
	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/cliutil"
	"proxmox-pp-cli/internal/config"
	"proxmox-pp-cli/internal/credkit"
)

// Declarations is what the connector holds and where each credential may go.
//
// Credentials: the Proxmox VE API token (PROXMOX_API_TOKEN, config.toml
// api_token, the 0600 credentials.toml store, `auth set-token` on stdin) sent
// as PVEAPIToken=... to the Proxmox VE host only; the Proxmox Backup Server
// token (PROXMOX_PBS_API_TOKEN, pbs_api_token) sent as PBSAPIToken=... to the
// PBS host only; each clusters.toml entry's tokens, read from the environment
// variables the entry names and sent to that entry's own hosts only; the MCP
// HTTP bearer; and SPACEFAST_TOKEN for --deliver spacefast:. Tokens are static:
// nothing is minted or refreshed. No flag carries a credential.
//
// A Proxmox token is USER@REALM!TOKENID=SECRET. Only SECRET is the credential:
// USER@REALM!TOKENID is the token's id, which Proxmox lists in ordinary reads
// (the user of every task the token started, token lists, ACLs). So the kit
// holds each token's SECRET (cliutil.TokenSecret) under a minted:* source
// bound to that token's own origin, and the raw forms (the environment
// variable, the config key, the Authorization header) are exempt below.
//
// No ResponseFields: Proxmox keeps secrets out of its read APIs except a
// storage plugin's storage.cfg entry, whose field names are the plugin's own
// (freenas_password, truenas_secret, ...). The client's chokepoint
// (handfixes.json response-secret-redaction) replaces every secret-named field
// before the cache, the store and any output.
var Declarations = credkit.Declarations{
	SecretEnv: []string{"PROXMOX_MCP_HTTP_TOKEN", "SPACEFAST_TOKEN"},
	URLEnv:    []string{"PROXMOX_FEEDBACK_ENDPOINT", "SPACEFAST_API_URL"},
	Files:     credentialFiles,
	// The generated config and credential-store keys the press keeps for other
	// auth schemes; Proxmox auth never fills them, so any value there is held
	// whole. api_token and pbs_api_token are held as their SECRET (Files).
	ConfigFields:  []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	StoredRecords: []string{"auth_header", "access_token", "refresh_token", "client_id", "client_secret"},
	BaseURL: credkit.BaseURL{
		ConfigKey: "base_url",
		Env:       "PROXMOX_BASE_URL",
		// A template: the host comes from PROXMOX_HOST, bound to the Proxmox VE
		// token below (pveHostOrigin).
		Default: "https://{host}:8006/api2/json",
	},
	Origins: append([]credkit.Origin{
		// Anything else held (a [headers] value, auth_header) goes only to the
		// Proxmox VE base URL, never to a --deliver webhook, the feedback
		// endpoint or a redirect's host.
		{Source: "*", Allowed: []string{"base_url"}},
		{Source: "minted:pve-token", Allowed: pveOrigins()},
		// auth set-token's token, read from stdin, is a Proxmox VE token.
		{Source: "stdin", Allowed: pveOrigins()},
		{Source: "minted:pbs-token", Allowed: pbsOrigins()},
		// A token secret the client is about to send that startup did not hold
		// (none should exist): bound nowhere, so the request is refused.
		{Source: "minted:authorization", Allowed: nil},
		// The MCP HTTP bearer authenticates callers of this server; it is sent nowhere.
		{Source: "env:PROXMOX_MCP_HTTP_TOKEN", Allowed: nil},
		// --deliver spacefast:<name> publishes to Spacefast's API.
		{Source: "env:SPACEFAST_TOKEN", Allowed: []string{"https://api.spacefast.com"}},
		// An anonymous Spacefast page's key goes only to Spacefast's API.
		{Source: "minted:spacefast-page-key", Allowed: []string{"https://api.spacefast.com"}},
	}, clusterOrigins()...),
	Identifiers: []credkit.Identifier{
		{Source: "env:PROXMOX_API_TOKEN", Reason: "USER@REALM!TOKENID=SECRET: the token id is public (Proxmox lists it in task and token reads); its SECRET is held from startup as minted:pve-token"},
		{Source: "env:PROXMOX_PBS_API_TOKEN", Reason: "USER@REALM!TOKENID:SECRET: the token id is public; its SECRET is held from startup as minted:pbs-token"},
		{Source: "config:api_token", Reason: "a Proxmox VE token; its SECRET is held from startup as minted:pve-token (holdFileTokens)"},
		{Source: "config:pbs_api_token", Reason: "a PBS token; its SECRET is held from startup as minted:pbs-token (holdFileTokens)"},
		{Source: "header:authorization", Reason: "this connector sends PVEAPIToken=<id>=<secret>, PBSAPIToken=<id>:<secret> and the Spacefast bearer; every secret is held from startup under its own source and origin, and the public token id appears in ordinary Proxmox reads"},
		{Source: "config:base_url", Reason: "the Proxmox VE base URL; a credential inside it is held as userinfo and query:<name>"},
		{Source: "config:ca_cert", Reason: "path to a PEM trust bundle, not a credential"},
		{Source: "config:tls_server_name", Reason: "TLS server name override, a host name"},
		{Source: "config:token_expiry", Reason: "a timestamp, not a credential"},
		{Source: "config:template_vars", Reason: "the PVE and PBS host names that fill {host} and {pbs_host}"},
		{Source: "env:PROXMOX_BASE_URL", Reason: "the Proxmox VE base URL; a credential inside it is held through BaseURL"},
		{Source: "env:PROXMOX_HOST", Reason: "the Proxmox VE host name"},
		{Source: "env:PROXMOX_PBS_HOST", Reason: "the Proxmox Backup Server host name"},
		{Source: "env:PROXMOX_TLS_FINGERPRINT", Reason: "a certificate SHA-256 fingerprint (public)"},
		{Source: "env:PROXMOX_PBS_TLS_FINGERPRINT", Reason: "a certificate SHA-256 fingerprint (public)"},
		{Source: "env:PROXMOX_CA_CERT", Reason: "path to a PEM trust bundle"},
		{Source: "env:PROXMOX_TLS_SERVER_NAME", Reason: "TLS server name override, a host name"},
		{Source: "env:PROXMOX_CLUSTER", Reason: "name of a clusters.toml entry"},
		{Source: "env:PROXMOX_CLUSTERS_FILE", Reason: "path of clusters.toml, which holds variable NAMES and hosts, never a token"},
		{Source: "env:PROXMOX_CONFIG", Reason: "path of the config file, which Files names"},
		{Source: "env:PROXMOX_HOME", Reason: "root directory for config, data, state and cache"},
		{Source: "env:PROXMOX_CONFIG_DIR", Reason: "directory path; Files resolves it"},
		{Source: "env:PROXMOX_DATA_DIR", Reason: "directory path"},
		{Source: "env:PROXMOX_STATE_DIR", Reason: "directory path"},
		{Source: "env:PROXMOX_CACHE_DIR", Reason: "directory path"},
		{Source: "env:XDG_CONFIG_HOME", Reason: "directory path"},
		{Source: "env:XDG_DATA_HOME", Reason: "directory path"},
		{Source: "env:XDG_STATE_HOME", Reason: "directory path"},
		{Source: "env:XDG_CACHE_HOME", Reason: "directory path"},
		{Source: "env:PROXMOX_USER_AGENT", Reason: "User-Agent string"},
		{Source: "env:PROXMOX_MAX_RESPONSE_BYTES", Reason: "a size limit"},
		{Source: "env:PROXMOX_DATA_SOURCE", Reason: "data source mode (auto, live, local)"},
		{Source: "env:PROXMOX_NO_LOCAL_DATA", Reason: "boolean switch"},
		{Source: "env:PROXMOX_MCP_READ_ONLY", Reason: "boolean switch"},
		{Source: "env:PROXMOX_MCP_CHILD", Reason: "marker the MCP server sets on its CLI children"},
		{Source: "env:PROXMOX_MCP_HTTP_ALLOWED_ORIGINS", Reason: "browser origins the MCP HTTP server accepts, not a credential"},
		{Source: "env:PROXMOX_CLI_PATH", Reason: "path of the companion CLI binary"},
		{Source: "env:PROXMOX_FEEDBACK_ENDPOINT", Reason: "feedback endpoint URL; a credential inside it is held through URLEnv, and the outbound guard refuses any held value sent to it"},
		{Source: "env:PROXMOX_FEEDBACK_AUTO_SEND", Reason: "boolean switch"},
		{Source: "env:PROXMOX_NO_LEARN", Reason: "boolean switch"},
		{Source: "env:PROXMOX_LEARN_NO_CAPTURE", Reason: "boolean switch"},
		{Source: "env:PROXMOX_LEARN_SESSION", Reason: "session grouping label; the journal stores only its hash"},
		{Source: "env:PROXMOX_LEARN_SURFACE", Reason: "surface label (cli or mcp)"},
		{Source: "env:SPACEFAST_API_URL", Reason: "Spacefast API base URL; a credential inside it is held through URLEnv"},
		{Source: "env:SPACEFAST_TEAM", Reason: "Spacefast team id, not a credential"},
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

// pveOrigins is where a Proxmox VE token may go: the configured base URL and
// the host PROXMOX_HOST names.
func pveOrigins() []string {
	out := []string{"base_url"}
	if o := pveHostOrigin(cliutil.EnvOverride("PROXMOX_HOST")); o != "" {
		out = append(out, o)
	}
	return out
}

// pbsOrigins is where a Proxmox Backup Server token may go: the host
// PROXMOX_PBS_HOST names.
func pbsOrigins() []string {
	if o, err := client.PBSOrigin(strings.TrimSpace(cliutil.EnvOverride("PROXMOX_PBS_HOST"))); err == nil {
		return []string{o}
	}
	if cliutil.IsAnyHarness() {
		// The Printing Press harness's mock answers PBS paths on its loopback
		// base when no PBS host is set (client.pbsHarnessStandIn); the client
		// refuses that stand-in outside the harness.
		return []string{"base_url"}
	}
	return nil
}

// clusterOrigins binds each clusters.toml entry's tokens to that entry's own
// hosts, one source per entry and service, so one client's token is never
// allowed to another client's host. It runs once, when the package
// initializes, and writes nothing: it reads clusters.toml under the default
// home and each --home on the command line, in the order credentialFiles
// holds their tokens.
func clusterOrigins() []credkit.Origin {
	var out []credkit.Origin
	forEachHome(credkit.FlagValues(os.Args[1:], "--home"), func(i int) {
		for _, cl := range cli.ClusterTokenSources() {
			out = append(out,
				credkit.Origin{Source: clusterSource(i, cl.Name, "pve"), Allowed: nonEmpty(cl.PVEOrigin)},
				credkit.Origin{Source: clusterSource(i, cl.Name, "pbs"), Allowed: nonEmpty(cl.PBSOrigin)},
			)
		}
	})
	return out
}

// pveHostOrigin is the origin PROXMOX_HOST names, built as config.Load builds
// the base URL from it (https://{host}:8006/api2/json); "" when it names none.
func pveHostOrigin(host string) string {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	for _, scheme := range []string{"https://", "http://"} {
		if len(host) >= len(scheme) && strings.EqualFold(host[:len(scheme)], scheme) {
			host = host[len(scheme):]
			break
		}
	}
	if host == "" || config.TokenShaped(host) {
		return ""
	}
	u, err := url.Parse("https://" + host + ":8006/api2/json")
	if err != nil || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func clusterSource(home int, name, service string) credkit.Source {
	return "minted:cluster:" + strconv.Itoa(home) + ":" + name + ":" + service
}

func nonEmpty(origin string) []string {
	if origin == "" {
		return nil
	}
	return []string{origin}
}

// forEachHome runs fn under the default home (index 0) and each --home
// (index 1 on); an invalid --home is one the program refuses before reading
// any file.
func forEachHome(homes []string, fn func(i int)) {
	for i, h := range append([]string{""}, homes...) {
		restore, err := cliutil.SetHomeOverride(h)
		if err != nil {
			continue
		}
		fn(i)
		restore()
	}
}

// credentialFiles names every config and credentials file the command line
// and the environment could mean, and holds every Proxmox token's SECRET
// before Start returns. The first file is the config file the program reads
// when no --config is given (the kit takes the base URL from it; an explicit
// --config is read by the kit itself and wins). Holding a file the program
// does not read is safe; missing the one it reads is not.
func credentialFiles(args []string) []string {
	var files []string
	add := func(p string, err error) {
		if err == nil && p != "" && !slices.Contains(files, p) {
			files = append(files, p)
		}
	}
	homes := credkit.FlagValues(args, "--home")
	configs := credkit.FlagValues(args, "--config")
	startupNamedConfigOrHome = len(homes) > 0 || len(configs) > 0
	last := ""
	if n := len(homes); n > 0 {
		last = homes[n-1]
	}
	if env := os.Getenv("PROXMOX_CONFIG"); env != "" {
		add(env, nil)
	} else {
		underHome(last, func() { add(readConfigPath()) })
	}
	forEachHome(homes, func(i int) {
		add(primaryConfigPath())
		add(config.LegacyConfigPath())
		add(cliutil.CredentialsFilePath())
		holdProfileStores()
		for _, cl := range cli.ClusterTokenSources() {
			credkit.Hold(clusterSource(i, cl.Name, "pve"), envTokenSecret(cl.PVETokenEnv))
			credkit.Hold(clusterSource(i, cl.Name, "pbs"), envTokenSecret(cl.PBSTokenEnv))
		}
	})
	for _, c := range append(configs, os.Getenv("PROXMOX_CONFIG")) {
		if c != "" {
			add(c, nil)
			add(cliutil.CredentialsFilePathForConfig(c))
		}
	}
	credkit.Hold("minted:pve-token", cliutil.TokenSecret(strings.TrimSpace(cliutil.EnvOverride("PROXMOX_API_TOKEN"))))
	credkit.Hold("minted:pbs-token", cliutil.TokenSecret(strings.TrimSpace(cliutil.EnvOverride("PROXMOX_PBS_API_TOKEN"))))
	for _, f := range files {
		holdFileTokens(f)
	}
	return files
}

// envTokenSecret is the SECRET of the token in the environment variable a
// clusters.toml entry names, read as the client reads it (an unfilled .mcpb
// prompt is unset).
func envTokenSecret(name string) string {
	if name == "" {
		return ""
	}
	v := strings.TrimSpace(cliutil.EnvOverride(name))
	if v == "" {
		return ""
	}
	return cliutil.TokenSecret(v)
}

// maxTokenFile bounds holdFileTokens' read; a larger file is no config this
// program writes, and the kit reads every file Files names in full anyway.
const maxTokenFile = 1 << 20

// holdFileTokens holds the SECRET of the api_token and pbs_api_token keys of
// a config or credentials file. A file that does not parse is read by the kit
// itself, token by token (every string in it held whole).
func holdFileTokens(path string) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxTokenFile {
		return
	}
	data, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return
	}
	var f struct {
		APIToken    string `toml:"api_token"`
		PBSAPIToken string `toml:"pbs_api_token"`
	}
	if toml.Unmarshal(data, &f) != nil {
		return
	}
	credkit.Hold("minted:pve-token", cliutil.TokenSecret(f.APIToken))
	credkit.Hold("minted:pbs-token", cliutil.TokenSecret(f.PBSAPIToken))
}

// startupNamedConfigOrHome records that the command line named a --config or
// --home, which Start read. Set once, inside Start, before main goes on.
var startupNamedConfigOrHome bool

// StartupNamedConfigOrHome reports whether the command line named a --config
// or --home that Start read. The MCP server has neither flag, so it refuses to
// run when Start took a config, and with it an allowed origin, from one.
func StartupNamedConfigOrHome() bool { return startupNamedConfigOrHome }

// underHome runs fn with the program's --home override set to h ("" is the
// default home); an invalid --home is one the program refuses before reading
// any file.
func underHome(h string, fn func()) {
	restore, err := cliutil.SetHomeOverride(h)
	if err != nil {
		return
	}
	defer restore()
	fn()
}

func primaryConfigPath() (string, error) {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// readConfigPath is the default config file the program reads: the primary
// one, else the legacy path when only that exists (config.Load).
func readConfigPath() (string, error) {
	p, err := primaryConfigPath()
	if err != nil {
		return "", err
	}
	if _, serr := os.Stat(p); serr != nil {
		if legacy, lerr := config.LegacyConfigPath(); lerr == nil {
			if _, e := os.Stat(legacy); e == nil {
				return legacy, nil
			}
		}
	}
	return p, nil
}

// holdProfileStores holds the run profile stores (profiles.json, current and
// legacy): every value under a declared credential name, and every value under
// base-url as a URL (item 8c).
func holdProfileStores() {
	var stores []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		stores = append(stores, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		stores = append(stores, filepath.Join(home, ".proxmox-cli", "profiles.json"))
	}
	for _, p := range stores {
		data, err := fs.ReadFile(os.DirFS(filepath.Dir(p)), filepath.Base(p))
		if err != nil {
			continue
		}
		credkit.HoldBody(data)
		var store struct {
			Profiles map[string]struct {
				Values map[string]string `json:"values"`
			} `json:"profiles"`
		}
		if json.Unmarshal(data, &store) != nil {
			continue
		}
		for _, prof := range store.Profiles {
			if v := prof.Values["base-url"]; v != "" {
				credkit.HoldURL("profile:base-url", v)
			}
		}
	}
}
