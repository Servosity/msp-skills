// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: one Apple Business API account identity per run.

package config

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// The generated identity hashes every credential field, and the first token
// mint persists client_id/client_secret into credentials.toml, so the second
// run hashed differently and synced into a fresh, empty store. The Client ID
// names exactly one API account (one per customer org) and survives key
// rotation, so it (with the API host) is the store's identity.
func init() {
	StoreScopeIdentity = func(c *Config) string {
		id := strings.TrimSpace(c.AppleBusinessClientId)
		if id == "" {
			id = strings.TrimSpace(c.ClientID)
		}
		if id == "" {
			return ""
		}
		return "apple_business_client_id=" + id + "|base_url=" + strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	}
}

// explicitAccountMark flags (inside envOverrides, which nothing iterates) a
// run that must not write credentials.toml or a config file: anything other
// than a pure saved-login run. Its minted tokens stay in memory.
const explicitAccountMark = "apple_business_explicit_account"

var accountEnvVars = []string{"APPLE_BUSINESS_CLIENT_ID", "APPLE_BUSINESS_PRIVATE_KEY", "APPLE_BUSINESS_KEY_ID", "APPLE_BUSINESS_BASE_URL", "APPLE_BUSINESS_TOKEN_URL"}

// ReconcileAppleAccount makes the token, signing key, key ID and store all
// belong to one API account, and decides who may persist. Called at the end
// of Load.
//
// Load merges credential fields from the config file, credentials files and
// the environment, so a field's presence says nothing about which account it
// belongs to. Provenance is therefore checked against the saved logins
// themselves: a token or client secret is kept only when a saved login for
// the active Client ID holds it, and business key material that another
// account's saved login holds is dropped unless the environment supplied it.
func ReconcileAppleAccount(c *Config, explicitConfigFile bool) {
	// Which file was refused matters: the active config file itself (default
	// or --config) is never re-read to name an account, and a refused
	// --config never falls back to another account. A refused global or
	// sibling credentials file was simply not merged, so nothing else changes.
	fileRefused := false
	for _, r := range c.CredentialRefusals {
		if r.Path == c.Path || (c.legacySourcePath != "" && r.Path == c.legacySourcePath) {
			fileRefused = true
		}
	}
	if explicitConfigFile && fileRefused {
		// Values the environment supplied stand.
		clearAccount(c)
		for _, f := range []struct {
			name string
			v    *string
		}{{"AppleBusinessClientId", &c.AppleBusinessClientId}, {"AppleBusinessPrivateKey", &c.AppleBusinessPrivateKey}, {"AppleBusinessKeyId", &c.AppleBusinessKeyId}} {
			if !c.envOverrides[f.name] {
				*f.v = ""
			}
		}
	}
	envAccount := false
	for _, v := range accountEnvVars {
		if strings.TrimSpace(os.Getenv(v)) != "" {
			envAccount = true
		}
	}
	global := globalLogin()
	saved := savedLogins(explicitConfigFile)
	// Load fills empty fields from the saved login, so the account is chosen
	// by where it was named: environment, then the active config file
	// (business_client_id, else client_id), then the saved login.
	var file fileAccount
	if !fileRefused {
		file = configFileAccount(cmp.Or(c.legacySourcePath, c.Path)) // the file Load actually read
	}
	want := strings.TrimSpace(c.AppleBusinessClientId)
	if !c.envOverrides["AppleBusinessClientId"] {
		want = cmp.Or(file.ClientID, file.GenericClientID, want, c.ClientID)
	}
	if want == "" {
		return // no Apple account (e.g. a bare `auth set-token`): generated behavior
	}
	c.AppleBusinessClientId = want
	// A custom Authorization header would replace this account's token on
	// every request; credentials come only through the provenance rules.
	c.dropAuthorizationHeader()
	other := func(match func(cliutil.Credentials) bool) bool {
		for _, s := range saved {
			if s.ClientID != "" && s.ClientID != want && match(s) {
				return true
			}
		}
		return false
	}
	ownedBy := func(match func(cliutil.Credentials) bool) bool {
		for _, s := range saved {
			if s.ClientID == want && match(s) {
				return true
			}
		}
		return false
	}
	// Business key material stands when the environment supplied it or this
	// account's saved login holds it. From the active config file it stands
	// only if that file names this account (or none); from a credentials
	// file, unless another account's saved login is where it came from.
	fileAcct := cmp.Or(file.ClientID, file.GenericClientID)
	keep := func(env bool, k, fromFile string, held func(cliutil.Credentials) bool) bool {
		switch {
		case env || ownedBy(held):
			return true
		case k == fromFile:
			return fileAcct == "" || fileAcct == want
		default:
			return !other(held)
		}
	}
	if k := c.AppleBusinessPrivateKey; k != "" && !keep(c.envOverrides["AppleBusinessPrivateKey"], k, file.PrivateKey,
		func(s cliutil.Credentials) bool { return s.AppleBusinessPrivateKey == k || s.ClientSecret == k }) {
		c.AppleBusinessPrivateKey = ""
	}
	if k := c.AppleBusinessKeyId; k != "" && !keep(c.envOverrides["AppleBusinessKeyId"], k, file.KeyID,
		func(s cliutil.Credentials) bool { return s.AppleBusinessKeyId == k }) {
		c.AppleBusinessKeyId = ""
	}
	// A bearer token is used only if a saved login for this account holds it.
	if tok := c.AccessToken; tok == "" || !ownedBy(func(s cliutil.Credentials) bool { return s.AccessToken == tok }) {
		c.AccessToken, c.RefreshToken = "", "" // auth_header has its own check below
		c.TokenExpiry = time.Time{}
	}
	// A raw auth_header outranks the token, so it needs the same provenance:
	// kept only when the environment set it or this account's login holds it.
	if h := c.AuthHeaderVal; h != "" && !c.envOverrides["AuthHeaderVal"] &&
		!ownedBy(func(s cliutil.Credentials) bool { return s.AuthHeaderVal == h }) {
		c.AuthHeaderVal = ""
	}
	if key := strings.TrimSpace(c.AppleBusinessPrivateKey); key != "" {
		c.ClientSecret = key
	} else if sec := c.ClientSecret; sec == "" || !ownedBy(func(s cliutil.Credentials) bool { return s.ClientSecret == sec }) {
		c.ClientSecret = ""
	}
	c.ClientID = want
	if explicitConfigFile || envAccount || global == nil || global.ClientID != want {
		c.markEnvOverride(explicitAccountMark)
	}
}

// useEnvAccessToken makes a token given in the environment the run's whole identity.
// A config's auth_header or Authorization header would otherwise outrank it and a
// saved login's Client ID would keep that login's store, so a write meant for this
// token goes to the other account and this token's inventory lands in its database.
// With no account left, the store is scoped by the token.
func (c *Config) useEnvAccessToken(token string) {
	clearAccount(c)
	c.AppleBusinessClientId, c.AppleBusinessPrivateKey, c.AppleBusinessKeyId = "", "", ""
	c.dropAuthorizationHeader()
	c.AccessToken = token
	c.markEnvOverride("AccessToken")
}

func (c *Config) dropAuthorizationHeader() {
	for k := range c.Headers {
		if strings.EqualFold(k, "Authorization") {
			delete(c.Headers, k)
		}
	}
}

// accountCanMint reports an account the next request can mint a token from, the same
// three values the assertion signs with. Present, not verified.
func (c *Config) accountCanMint() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != "" && strings.TrimSpace(c.AppleBusinessKeyId) != ""
}

// orgConfig is what a save under an explicit config writes: the file stays the whole
// saved identity of its customer. A minted token is not written; the next run mints
// its own from the key. A pasted token is the identity when no account is named. The
// fields are listed because the encoder skips an embedded field of an unexported type.
func (c *Config) orgConfig() any {
	out := struct {
		BaseURL                 string            `toml:"base_url"`
		TokenURL                string            `toml:"token_url,omitempty"`
		CACert                  string            `toml:"ca_cert,omitempty"`
		TLSServerName           string            `toml:"tls_server_name,omitempty"`
		Headers                 map[string]string `toml:"headers,omitempty"`
		AppleBusinessClientId   string            `toml:"business_client_id,omitempty"`
		AppleBusinessPrivateKey string            `toml:"business_private_key,omitempty"`
		AppleBusinessKeyId      string            `toml:"business_key_id,omitempty"`
		AccessToken             string            `toml:"access_token,omitempty"`
	}{
		BaseURL:                 c.BaseURL,
		TokenURL:                c.TokenURL,
		CACert:                  c.CACert,
		TLSServerName:           c.TLSServerName,
		Headers:                 c.Headers,
		AppleBusinessClientId:   c.AppleBusinessClientId,
		AppleBusinessPrivateKey: c.AppleBusinessPrivateKey,
		AppleBusinessKeyId:      c.AppleBusinessKeyId,
	}
	if c.AppleBusinessClientId == "" {
		out.AccessToken = c.AccessToken
	}
	return out
}

// SavesToOwnFile reports that this run saves to its explicit config, not the shared store.
func (c *Config) SavesToOwnFile() bool { return c.selectedLoginPath != "" }

// SaveRefusal stops a save over a config other users can change while that file names
// routing. Load set the routing aside as untrusted: writing it back 0600 would make it
// trusted, and leaving it out would erase the customer's setup. A token minted during an
// ordinary read is not saved (SaveAppleToken), so only a command that means to save stops.
func (c *Config) SaveRefusal() error {
	if !c.looseRouting {
		return nil
	}
	return fmt.Errorf("%s names hosts, TLS settings or headers and other users can change it; nothing was changed. Review it, chmod 600 it, and run this again", cmp.Or(c.legacySourcePath, c.Path))
}

// LogoutRefusal says why auth logout must change nothing: the environment selects an
// identity no saved login holds, so anything it could clear belongs to someone else. It
// reads the variables itself: Load does not apply them under a route it blocked, and they
// select the identity all the same.
func (c *Config) LogoutRefusal() string {
	if strings.TrimSpace(cliutil.EnvOverride("APPLE_BUSINESS_ACCESS_TOKEN")) != "" {
		return "APPLE_BUSINESS_ACCESS_TOKEN is set, so this run acts as that token and has no saved login to clear; nothing was changed. Unset it to log out."
	}
	sharedAccount, sharedSaved := "", false
	if shared, err := cliutil.CredentialsFilePath(); err == nil && c.selectedLoginPath == "" {
		var readErr error
		if sharedAccount, sharedSaved, readErr = loginFileAccount(shared); readErr != nil {
			return "the saved login at " + shared + " cannot be read, so nothing shows it is this run's; nothing was changed. Fix its permissions, then log out."
		}
	}
	envAccount := strings.TrimSpace(cliutil.EnvOverride("APPLE_BUSINESS_CLIENT_ID"))
	if envAccount == "" {
		return ""
	}
	file := configFileAccount(cmp.Or(c.legacySourcePath, c.Path))
	named := cmp.Or(file.ClientID, file.GenericClientID)
	other := named != envAccount && (named != "" || file.AccessToken != "" || file.AuthHeader != "")
	if other || (sharedSaved && sharedAccount != envAccount) {
		return "APPLE_BUSINESS_CLIENT_ID names another account than the login this would clear; nothing was changed. Unset it to log out."
	}
	return ""
}

// removeSelectedLogin removes the shared login if it holds the account this run kept;
// the file itself says so, since a store Load refused is still someone's login. With no
// account kept, a default config holding its own token never read the shared login, so
// a login naming an account is not this run's to remove; a bare token names nobody and
// goes with the config's. Under an explicit config there is none to remove: the save
// clears the config itself.
func (c *Config) removeSelectedLogin() error {
	if c.selectedLoginPath != "" {
		return nil
	}
	if shared, err := cliutil.CredentialsFilePath(); err == nil {
		account, ok, err := loginFileAccount(shared)
		if err != nil {
			return err
		}
		if ok && account != c.loginAccount && (c.loginAccount != "" || c.ownIdentity) {
			return nil
		}
	}
	return cliutil.RemoveCredentials()
}

// loginFileAccount reads the account a login file names, whatever its permissions.
// ok is false when there is no such file; a file it cannot read is an error, since it
// may hold anyone's login.
func loginFileAccount(path string) (account string, ok bool, err error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	cliutil.HoldConfigFile(data) // Hand-fix (handfixes.json credkit-wiring)
	var creds cliutil.Credentials
	_ = toml.Unmarshal(data, &creds)
	return strings.TrimSpace(cmp.Or(creds.AppleBusinessClientId, creds.ClientID)), true, nil
}

func clearAccount(c *Config) {
	c.ClientID, c.ClientSecret = "", ""
	c.AccessToken, c.RefreshToken, c.AuthHeaderVal = "", "", ""
	c.TokenExpiry = time.Time{}
}

func globalLogin() *cliutil.Credentials {
	path, err := cliutil.CredentialsFilePath()
	if err != nil {
		return nil
	}
	return readLogin(path)
}

// readLogin parses a credentials file silently (Load already reported any
// problem with the files it chose to read); a missing, unreadable, loose-
// permission or corrupt file is simply not a saved login.
func readLogin(path string) *cliutil.Credentials {
	real, err := filepath.EvalSymlinks(path)
	if err != nil || cliutil.VerifyCredsPerms(real) != nil {
		return nil
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return nil
	}
	cliutil.HoldConfigFile(data) // Hand-fix (handfixes.json credkit-wiring)
	var creds cliutil.Credentials
	if toml.Unmarshal(data, &creds) != nil {
		return nil
	}
	return &creds
}

// savedLogins are the credential sets Load may have merged in: the shared credentials
// file, unless an explicit config, which is its own saved identity, is in use.
func savedLogins(explicitConfigFile bool) []cliutil.Credentials {
	if explicitConfigFile {
		return nil
	}
	if g := globalLogin(); g != nil {
		return []cliutil.Credentials{*g}
	}
	return nil
}

var errNotSavedLoginOwner = errors.New("saved login belongs to another account")

// SaveAppleToken stores a freshly minted token. Only a pure saved-login run
// whose account still owns credentials.toml (checked under the credentials
// lock) persists there; every other run keeps the token in memory only, so
// nothing but the 0600 credentials file ever holds a credential.
func (c *Config) SaveAppleToken(clientID, clientSecret, accessToken string, expiry time.Time) error {
	credkit.Hold("minted:token", accessToken)
	if !c.envOverrides[explicitAccountMark] && !c.looseRouting {
		credsPath, err := cliutil.CredentialsFilePath()
		if err == nil && readLogin(credsPath) != nil {
			err = cliutil.WithFileLock(credsPath, func() error {
				// Same account AND same signing material as this run loaded: a
				// newer login (e.g. a rotated key) is never overwritten.
				if creds := readLogin(credsPath); creds == nil || creds.ClientID != clientID || creds.ClientSecret != clientSecret ||
					creds.AppleBusinessPrivateKey != c.AppleBusinessPrivateKey || creds.AppleBusinessKeyId != c.AppleBusinessKeyId {
					return errNotSavedLoginOwner
				}
				c.ClientID, c.ClientSecret = clientID, clientSecret
				c.AccessToken, c.RefreshToken, c.TokenExpiry = accessToken, "", expiry
				for _, f := range []string{"AccessToken", "RefreshToken", "TokenExpiry"} {
					delete(c.envOverrides, f)
				}
				for _, f := range []string{"ClientID", "ClientSecret", "AccessToken", "RefreshToken", "TokenExpiry"} {
					c.updateFileConfigField(f)
				}
				return c.saveCredentialsThenConfigLocked(credsPath)
			})
			if !errors.Is(err, errNotSavedLoginOwner) {
				return err
			}
		}
		// No saved login, or logged out or replaced since this run loaded:
		// never create or overwrite the saved login.
		c.markEnvOverride(explicitAccountMark)
	}
	// ponytail: explicit accounts re-mint once per process; a per-account
	// token cache would be a second file holding a credential.
	c.AccessToken, c.TokenExpiry = accessToken, expiry
	return nil
}

// WorkerCopy returns a frozen copy of this account's config for a parallel
// worker: it never re-reads credentials from disk and keeps any token it
// mints in memory, so concurrent workers share no mutable state.
func (c *Config) WorkerCopy() *Config {
	cp := *c
	cp.Headers = cloneStringMap(c.Headers)
	cp.envOverrides = maps.Clone(c.envOverrides)
	if cp.envOverrides == nil {
		cp.envOverrides = map[string]bool{}
	}
	cp.envOverrides[explicitAccountMark] = true
	cp.fileConfig = nil
	cp.CredentialRefusals = append([]cliutil.CredentialRefusal(nil), c.CredentialRefusals...)
	return &cp
}

// ClearAppleAccount runs before `auth set-token` saves a bare token: the
// token is its own identity, so the saved login must not keep an old
// account's Client ID, key or key ID (they would re-mint as that account).
// A pasted token has no known expiry.
func (c *Config) ClearAppleAccount() {
	c.BindAppleAccount("", "", "")
	c.dropAuthorizationHeader()
	c.TokenExpiry = time.Time{}
}

// AbsKeyRef makes a relative private-key file reference absolute so a saved
// login still finds the key from another directory. Inline PEM text, ~ paths
// and references to missing files are returned unchanged.
func AbsKeyRef(v string) string {
	t := strings.Trim(strings.TrimSpace(v), `"'`)
	if t == "" || filepath.IsAbs(t) || strings.HasPrefix(t, "~") || strings.Contains(t, "-----BEGIN") || strings.Contains(t, `\n`) {
		return v
	}
	abs, err := filepath.Abs(t)
	if err != nil {
		return v
	}
	if _, err := os.Stat(abs); err != nil {
		return v
	}
	return abs
}

// BindAppleAccount sets the login's account, key reference and key ID for the
// next save (auth login's SaveTokens), so they reach credentials.toml in the
// same transaction as the token. Env markers are dropped and the file
// snapshot updated, or configForSave would restore other values.
func (c *Config) BindAppleAccount(clientID, privateKey, keyID string) {
	c.AppleBusinessClientId, c.AppleBusinessPrivateKey, c.AppleBusinessKeyId = clientID, privateKey, strings.TrimSpace(keyID)
	for _, f := range []string{"AppleBusinessClientId", "AppleBusinessPrivateKey", "AppleBusinessKeyId"} {
		delete(c.envOverrides, f)
		c.updateFileConfigField(f)
	}
}

type fileAccount struct {
	ClientID        string `toml:"business_client_id"`
	GenericClientID string `toml:"client_id"`
	PrivateKey      string `toml:"business_private_key"`
	KeyID           string `toml:"business_key_id"`
	AccessToken     string `toml:"access_token"`
	AuthHeader      string `toml:"auth_header"`
}

// configFileAccount reads the business_* keys written in the config file
// itself (not merged from credentials files).
func configFileAccount(path string) fileAccount {
	var f fileAccount
	if path == "" {
		return f
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return f
	}
	cliutil.HoldConfigFile(data) // Hand-fix (handfixes.json credkit-wiring)
	_ = toml.Unmarshal(data, &f)
	return f
}
