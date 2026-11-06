// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the one identity a process acts as.

// Package jamfauth holds the identity, (normalized server origin, client
// id), behind every credential decision. An Identity is an immutable value:
// it is created once (New, ForInstance or Decode), established once per
// process (Establish), and handed to child processes whole (Encode, in
// IdentityEnv). Nothing re-derives it from a registry name or a config
// after that: callers compare against the established value and use
// Identity.Bind, Identity.Key, Identity.Allows and Identity.CheckMint.
package jamfauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/config"
)

const (
	// SelectEnv names the registered instance a process is pinned to.
	SelectEnv = "JAMF_PRO_INSTANCE"
	// IdentityEnv carries a parent's established identity to a child. Only
	// this program sets it, so it carries the internal PP_ prefix.
	IdentityEnv = "PP_JAMF_PRO_IDENTITY"
	// MintPath is Jamf Pro's OAuth token endpoint on every server.
	MintPath = "/api/v1/oauth/token"
)

// Inherited is IdentityEnv as this process reads it, the one read of that
// variable, so the name it reads is visible where it is declared.
func Inherited() string { return os.Getenv(IdentityEnv) }

// Selected is SelectEnv as this process reads it, the one read of that
// variable. An MCPB placeholder left unresolved reads as unset.
func Selected() string { return cliutil.EnvOverride(SelectEnv) }

// Identity is one Jamf Pro server and one API client (ClientID is empty for
// a pasted token). Instance is set when it is a registered instance (its
// secret is the JAMF_PRO_FLEET_SECRETS entry of that name). The fields are unexported: the value cannot be edited.
type Identity struct {
	server, clientID, instance string
}

// New creates the identity for a template-resolved server URL and client id.
func New(serverURL, clientID string) (Identity, error) {
	server, err := Origin(serverURL)
	if err != nil {
		return Identity{}, err
	}
	return Identity{server: server, clientID: strings.TrimSpace(clientID)}, nil
}

// ForInstance creates the identity of a registered instance (server is a
// bare host[:port], always reached over https).
func ForInstance(name, server, clientID string) (Identity, error) {
	id, err := New("https://"+server, clientID)
	if err != nil {
		return Identity{}, err
	}
	if name == "" {
		return Identity{}, errors.New("an instance identity needs a name")
	}
	id.instance = name
	return id, nil
}

func (i Identity) Server() string   { return i.server }
func (i Identity) ClientID() string { return i.clientID }
func (i Identity) Instance() string { return i.instance }

// Host is the server's host[:port].
func (i Identity) Host() string {
	return strings.TrimPrefix(strings.TrimPrefix(i.server, "https://"), "http://")
}

// Same reports the same server and client.
func (i Identity) Same(o Identity) bool { return i.server == o.server && i.clientID == o.clientID }

// Key is the identity as a string: the store scope and binding input.
func (i Identity) Key() string { return "server=" + i.server + "\nclient_id=" + i.clientID }

// Bind ties a credential (token, secret) to this identity. Stored next to
// the config, it lets a later process use the credential only as the same
// identity: the hash covers the exact credential, so a binding never
// vouches for a different one.
func (i Identity) Bind(credential string) string {
	sum := sha256.Sum256([]byte(credential + "\n" + i.Key()))
	return hex.EncodeToString(sum[:])
}

// TokenURL is the only URL this identity posts its client secret to.
func (i Identity) TokenURL() string { return i.server + MintPath }

// Allows reports whether a request URL is on this identity's server.
func (i Identity) Allows(u *url.URL) bool {
	return u != nil && strings.EqualFold(u.Scheme+"://"+u.Host, i.server)
}

// CheckMint refuses a credential post anywhere but exactly TokenURL (no
// trailing slash, query or fragment) or for another client id.
func (i Identity) CheckMint(u *url.URL, clientID string) error {
	if u == nil || !i.Allows(u) || u.Path != MintPath || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("credentials are only posted to %s; refused %v", i.TokenURL(), u)
	}
	if strings.TrimSpace(clientID) != i.clientID {
		return fmt.Errorf("token mint for client %q refused: this process acts as %q", clientID, i.clientID)
	}
	return nil
}

type wire struct {
	Server   string `json:"server"`
	ClientID string `json:"client_id"`
	Instance string `json:"instance,omitempty"`
}

// Encode serializes the identity for IdentityEnv.
func (i Identity) Encode() string {
	b, _ := json.Marshal(wire{i.server, i.clientID, i.instance})
	return string(b)
}

// Decode rebuilds an identity a parent encoded, validating it as New and
// ForInstance do.
func Decode(s string) (Identity, error) {
	var w wire
	if err := json.Unmarshal([]byte(s), &w); err != nil {
		return Identity{}, fmt.Errorf("%s is not an identity: %w", IdentityEnv, err)
	}
	id, err := New(w.Server, w.ClientID)
	if err != nil || w.Instance == "" {
		return id, err
	}
	if !strings.HasPrefix(id.server, "https://") {
		return Identity{}, fmt.Errorf("instance %q must use https", w.Instance)
	}
	return ForInstance(w.Instance, id.Host(), w.ClientID)
}

var process struct {
	sync.Mutex
	id          Identity
	established bool
	waived      bool
}

// Establish records the identity this process acts as. It is set once: a
// later call with the same identity is a no-op, any other is refused.
func Establish(id Identity) error {
	process.Lock()
	defer process.Unlock()
	if process.established {
		if process.id != id {
			return fmt.Errorf("this process already acts as %s (client %q); refusing %s (client %q)", process.id.server, process.id.clientID, id.server, id.clientID)
		}
		return nil
	}
	process.id, process.established = id, true
	return nil
}

// Established returns the process identity, if one was established.
func Established() (Identity, bool) {
	process.Lock()
	defer process.Unlock()
	return process.id, process.established
}

// Waive marks a verify dry-run that named an unregistered instance; it sends
// nothing, so no instance identity is required.
func Waive() {
	process.Lock()
	defer process.Unlock()
	process.waived = true
}

// Waived reports Waive.
func Waived() bool {
	process.Lock()
	defer process.Unlock()
	return process.waived
}

// BeginExecution starts one command execution with no identity. A real
// process runs exactly one execution (the MCP server runs commands as child
// processes), so this changes nothing there; it is what keeps executions
// in one test binary from inheriting each other's identity.
func BeginExecution() { ResetForTest() }

// ResetForTest forgets the process identity (each test phase is a "process").
func ResetForTest() {
	process.Lock()
	defer process.Unlock()
	process.id, process.established, process.waived = Identity{}, false, false
}

// OAuthErrorCodes are the token-endpoint error codes RFC 6749 section 5.2
// defines. They are the only text from a token endpoint that is ever
// printed (with the HTTP status); any other "error" value is dropped, since
// a server could put anything there, a secret included.
var OAuthErrorCodes = map[string]bool{
	"invalid_request": true, "invalid_client": true, "invalid_grant": true,
	"unauthorized_client": true, "unsupported_grant_type": true, "invalid_scope": true,
}

// Pair is the client pair a client mints with: the environment pair as a
// unit when either half is set (a partial env pair never borrows the other
// half from the cache), else the pair cached by 'auth login'. Environment
// values go through cliutil.EnvOverride, as config.Load reads them, so an
// unfilled MCPB "${...}" placeholder counts as unset.
func Pair(cfg *config.Config) (id, secret string) {
	if PairFromEnv() || cfg == nil {
		return env("JAMF_PRO_CLIENT_ID"), env("JAMF_PRO_CLIENT_SECRET")
	}
	return strings.TrimSpace(cfg.ClientID), strings.TrimSpace(cfg.ClientSecret)
}

// PairFromEnv reports whether Pair comes from the environment.
func PairFromEnv() bool {
	return env("JAMF_PRO_CLIENT_ID") != "" || env("JAMF_PRO_CLIENT_SECRET") != ""
}

func env(name string) string { return strings.TrimSpace(cliutil.EnvOverride(name)) }

// Origin normalizes a template-resolved URL to scheme://host[:port].
func Origin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("%q is not a server URL", raw)
	}
	if strings.ContainsAny(u.Host, "{}") {
		return "", fmt.Errorf("%q still holds an unfilled {placeholder}", raw)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}
