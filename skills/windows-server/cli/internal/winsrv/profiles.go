// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (server-profiles).

package winsrv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/credkit"
	"windows-server-pp-cli/internal/winrm"
)

// Profile is one server from the config file's [[servers]] list. It never
// holds a password in the file; Password is resolved at run time from the
// credential store or the environment.
type Profile struct {
	Name          string `toml:"name"`
	Host          string `toml:"host"`
	Port          int    `toml:"port"`
	Transport     string `toml:"transport"` // https (default) or http
	Auth          string `toml:"auth"`      // ntlm (default) or basic
	Username      string `toml:"username"`
	CAFile        string `toml:"ca_file"`
	CertSHA256    string `toml:"cert_sha256"`
	TLSServerName string `toml:"tls_server_name"`
	Default       bool   `toml:"default"`

	Password       string `toml:"-"`
	CredentialFrom string `toml:"-"` // where the password came from (never the value)
	ConfigFile     string `toml:"-"` // absolute path of the config that lists it
}

// ProfileNameRE is the only shape a server name may take: it can never carry a
// host, port, scheme or path.
var ProfileNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ErrNoServers means the config file lists no servers.
var ErrNoServers = errors.New("no servers configured: add a [[servers]] entry (name, host) to the config file")

type profileFile struct {
	Servers []Profile `toml:"servers"`
}

// LoadProfiles reads [[servers]] from the config file at path. A config file
// other users can change is refused when it lists servers, because the host a
// credential goes to comes from this file.
func LoadProfiles(path string) ([]Profile, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNoServers
	}
	data, trusted, err := cliutil.ReadConfigTrust(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoServers
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var pf profileFile
	if err := toml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing servers in %s: %w", path, err)
	}
	if len(pf.Servers) == 0 {
		return nil, ErrNoServers
	}
	if !trusted {
		return nil, fmt.Errorf("config %s can be changed by other users; its servers are not used (chmod 600 %s)", path, path)
	}
	abs := CanonicalPath(path)
	seen := map[string]bool{}
	defaults := 0
	for i := range pf.Servers {
		p := &pf.Servers[i]
		if err := p.normalize(); err != nil {
			return nil, err
		}
		p.ConfigFile = abs
		if seen[p.Name] {
			return nil, fmt.Errorf("server %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return nil, errors.New("more than one server is marked default = true")
	}
	return pf.Servers, nil
}

func (p *Profile) normalize() error {
	if !ProfileNameRE.MatchString(p.Name) {
		return fmt.Errorf("server name %q must be lowercase letters, digits and dashes", p.Name)
	}
	p.Host = strings.TrimSpace(p.Host)
	if !winrm.ValidHost(p.Host) {
		return fmt.Errorf("server %s: host %q must be a bare DNS name or IP address (no scheme, port or path)", p.Name, p.Host)
	}
	p.Transport = strings.ToLower(strings.TrimSpace(p.Transport))
	if p.Transport == "" {
		p.Transport = "https"
	}
	if p.Transport != "https" && p.Transport != "http" {
		return fmt.Errorf("server %s: transport must be https or http", p.Name)
	}
	p.Auth = strings.ToLower(strings.TrimSpace(p.Auth))
	if p.Auth == "" {
		p.Auth = "ntlm"
	}
	if p.Auth != "ntlm" && p.Auth != "basic" {
		return fmt.Errorf("server %s: auth must be ntlm or basic", p.Name)
	}
	if p.Auth == "basic" && p.Transport == "http" {
		return fmt.Errorf("server %s: auth = \"basic\" over transport = \"http\" would send the password in clear text; use https, or ntlm", p.Name)
	}
	if p.Port == 0 {
		p.Port = 5986
		if p.Transport == "http" {
			p.Port = 5985
		}
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("server %s: port %d out of range", p.Name, p.Port)
	}
	if p.CertSHA256 != "" {
		pin, err := winrm.ParseFingerprint(p.CertSHA256)
		if err != nil {
			return fmt.Errorf("server %s: %w", p.Name, err)
		}
		p.CertSHA256 = pin
	}
	return nil
}

// Select returns the named server, or every server when name is empty and
// all is true, or the single default server when name is empty.
func Select(profiles []Profile, name string, all bool) ([]Profile, error) {
	if name != "" {
		if !ProfileNameRE.MatchString(name) {
			return nil, fmt.Errorf("--server %q is not a server name", name)
		}
		for _, p := range profiles {
			if p.Name == name {
				return []Profile{p}, nil
			}
		}
		return nil, fmt.Errorf("no server named %q in the config file (have: %s)", name, strings.Join(names(profiles), ", "))
	}
	if all {
		return profiles, nil
	}
	if len(profiles) == 1 {
		return profiles, nil
	}
	for _, p := range profiles {
		if p.Default {
			return []Profile{p}, nil
		}
	}
	return nil, fmt.Errorf("%d servers are configured and none is default = true; pass --server (%s)", len(profiles), strings.Join(names(profiles), ", "))
}

func names(profiles []Profile) []string {
	out := make([]string, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// Endpoint is the WinRM endpoint for the profile.
func (p Profile) Endpoint(allowHTTP bool) winrm.Endpoint {
	return winrm.Endpoint{
		Host: p.Host, Port: p.Port, HTTP: p.Transport == "http", AllowHTTP: allowHTTP,
		CAFile: p.CAFile, CertSHA256: p.CertSHA256, TLSServerName: p.TLSServerName,
	}
}

// CredentialKey is a server's key in the credential store: the config file
// that lists it, its name, and the host and port it was saved for. A password
// stored for "dc01" under one client's config never goes to another client's
// "dc01", even at the same address (overlapping private ranges, short names).
func CredentialKey(p Profile) string {
	return CredentialPrefix(p.ConfigFile, p.Name) + strings.ToLower(p.Host) + ":" + strconv.Itoa(p.Port)
}

// CanonicalPath is the absolute, symlink-resolved form of a config path, so
// every spelling of one file keys the same stored credentials and store.
func CanonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// CredentialPrefix is the start of every key saved for server name under the
// config file at configFile (an absolute path).
func CredentialPrefix(configFile, name string) string {
	return configFile + "#" + name + "@"
}

// ServerCredential is one per-server entry in the credential store.
type ServerCredential struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type serverCredFile struct {
	Servers map[string]ServerCredential `toml:"servers"`
}

// serverStoreName is the file name of the per-server credential store: a 0600
// file next to the generated credentials file.
const serverStoreName = "server-credentials.toml"

// ServerCredentialsPath returns the store path beside credentialsPath.
func ServerCredentialsPath(credentialsPath string) string {
	return filepath.Join(filepath.Dir(credentialsPath), serverStoreName)
}

// LoadServerCredentials reads the per-server store. A missing file is empty;
// a file other users can read is refused.
func LoadServerCredentials(path string) (map[string]ServerCredential, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return map[string]ServerCredential{}, nil
	}
	if err := cliutil.VerifyCredsPerms(path); err != nil {
		return nil, fmt.Errorf("server credential store %s: %w", path, err)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var f serverCredFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Servers == nil {
		f.Servers = map[string]ServerCredential{}
	}
	// Start held the store as it was at startup; a password saved since
	// (servers login while the MCP server runs) is held here, before use.
	for _, c := range f.Servers {
		credkit.Hold("config:password", c.Password)
	}
	return f.Servers, nil
}

// SaveServerCredentials writes the per-server store with mode 0600.
func SaveServerCredentials(path string, creds map[string]ServerCredential) error {
	data, err := toml.Marshal(serverCredFile{Servers: creds}) // #nosec G117 -- written to a 0600 private file
	if err != nil {
		return err
	}
	if err := cliutil.AtomicWritePrivateFile(path, data, 0o600, 0o700); err != nil {
		return err
	}
	return cliutil.VerifyCredsPerms(path)
}

// MinPasswordLen is the shortest password the CLI sends. The output filter
// does not look for shorter values in decoded text, so a shorter password
// could be echoed back by a server unrecognized.
const MinPasswordLen = 8

// PasswordTooShort reports a password under MinPasswordLen once surrounding
// whitespace is ignored.
func PasswordTooShort(pw string) bool { return len(strings.TrimSpace(pw)) < MinPasswordLen }

// Shared is the account every server uses when it has no entry of its own.
type Shared struct {
	Username, Password, Source string
}

// ResolveCredentials fills p.Username and p.Password: the server's own store
// entry first, then the shared account. The username in the config file wins
// over the shared username; it is not a secret.
func ResolveCredentials(p *Profile, store map[string]ServerCredential, shared Shared) error {
	if c, ok := store[CredentialKey(*p)]; ok && c.Password != "" {
		if c.Username != "" {
			p.Username = c.Username
		}
		if p.Username == "" {
			p.Username = shared.Username
		}
		p.Password = c.Password
		p.CredentialFrom = "credential store (" + p.Name + ")"
	} else if shared.Password != "" {
		if p.Username == "" {
			p.Username = shared.Username
		}
		p.Password = shared.Password
		p.CredentialFrom = shared.Source
	}
	if p.Password != "" && PasswordTooShort(p.Password) {
		p.Password = ""
		return fmt.Errorf("server %s: the password is shorter than %d characters; the output filter cannot reliably recognize a value that short, so it is not used (set a longer password for this WinRM account)", p.Name, MinPasswordLen)
	}
	if p.Username == "" || p.Password == "" {
		login := "windows-server-cli servers login " + p.Name
		if p.ConfigFile != "" { // stored passwords are bound to the config file
			login += " --config '" + strings.ReplaceAll(p.ConfigFile, "'", `'\''`) + "'"
		}
		return fmt.Errorf("server %s has no credential: pipe its password on stdin to: %s --username <user>; or set WINDOWS_SERVER_USERNAME and WINDOWS_SERVER_PASSWORD", p.Name, login)
	}
	return nil
}
