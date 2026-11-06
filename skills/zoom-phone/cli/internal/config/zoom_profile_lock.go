// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"zoom-phone-pp-cli/internal/cliutil"
)

// Per-profile identity lock (zoom-phone hand-fix: zoom-phone-profile-save-lock).
//
// A process loads a profile (tenant A), then waits on the token endpoint. If
// `accounts connect <name> --force` replaces the profile (another tenant, or
// just a rotated secret), or another process saves a different identity to
// it, the automatic mint's save would write A back over it. Every writer of a
// profile's identity takes one cross-process lock found by ZoomProfileLockPath
// (cliutil.WithFileLock; <accounts>/.locks/<name>.lock for an account profile):
//
//   - the automatic mint: ZoomLockProfileForSave, which re-reads the on-disk
//     identity under the lock and refuses (ErrZoomProfileChanged) unless it is
//     exactly the snapshot this process loaded;
//   - `auth login` (an intentional identity replacement): ZoomLockProfileForReplace;
//   - `accounts connect --force` and `accounts remove`: ZoomAcquireProfileLock.

// ErrZoomProfileChanged reports that the saved profile was replaced, created or
// removed after this process loaded it; nothing was saved.
var ErrZoomProfileChanged = errors.New("saved profile changed while this command was running")

// The snapshot covers every credential-bearing field the generated config
// persists (config.Config and cliutil.Credentials): each as present/absent
// plus a SHA-256 of its value. Values are never compared in clear or logged.
// An empty field is an absent one, which is how config.Load treats it.
type zoomFieldDigest struct {
	Present bool
	Hash    [sha256.Size]byte
}

func zoomDigest(v string) zoomFieldDigest {
	if v = strings.TrimSpace(v); v == "" {
		return zoomFieldDigest{}
	}
	return zoomFieldDigest{Present: true, Hash: sha256.Sum256([]byte(v))}
}

func zoomDigestTime(t time.Time) zoomFieldDigest {
	if t.IsZero() {
		return zoomFieldDigest{}
	}
	return zoomDigest(t.UTC().Format(time.RFC3339Nano))
}

// zoomProfileIdentity is what names the tenant and app: any change here is a
// different profile.
type zoomProfileIdentity struct {
	AuthHeader, ClientID, ClientSecret, AccountID, PhoneClientID, PhoneClientSecret zoomFieldDigest
}

// zoomProfileTokens is what an app's own mint or refresh replaces.
type zoomProfileTokens struct {
	AccessToken, RefreshToken, TokenExpiry zoomFieldDigest
}

type zoomProfileSnapshot struct {
	Identity zoomProfileIdentity
	Tokens   zoomProfileTokens
}

func (s zoomProfileSnapshot) empty() bool { return s == zoomProfileSnapshot{} }

// anchored reports a complete app identity that tokens derive from.
func (s zoomProfileSnapshot) anchored() bool {
	return s.Identity.ClientID.Present && s.Identity.ClientSecret.Present
}

// zoomSnapshotChanged refuses any identity difference (including absent ->
// present). Token differences are refused too, except when the profile had a
// complete app identity at load and still has exactly that identity: then the
// difference is the same app's own newer token (another process of this
// account minted), and saving this run's equally valid token is harmless.
// A token-only or empty profile has no such anchor, so a token appearing or
// changing there (auth set-token, another first run) is always refused.
func zoomSnapshotChanged(loaded, disk zoomProfileSnapshot) bool {
	if loaded.Identity != disk.Identity {
		return true
	}
	return loaded.Tokens != disk.Tokens && !loaded.anchored()
}

func zoomSnapshotFrom(f zoomStoredFields) zoomProfileSnapshot {
	return zoomProfileSnapshot{
		Identity: zoomProfileIdentity{
			AuthHeader:        zoomDigest(f.AuthHeader),
			ClientID:          zoomDigest(f.ClientID),
			ClientSecret:      zoomDigest(f.ClientSecret),
			AccountID:         zoomDigest(f.AccountID),
			PhoneClientID:     zoomDigest(f.PhoneClientID),
			PhoneClientSecret: zoomDigest(f.PhoneClientSecret),
		},
		Tokens: zoomProfileTokens{
			AccessToken:  zoomDigest(f.AccessToken),
			RefreshToken: zoomDigest(f.RefreshToken),
			TokenExpiry:  zoomDigestTime(f.TokenExpiry),
		},
	}
}

// zoomLoadedSnapshot is the identity this process loaded from disk: the file
// snapshot config.Load takes after applying stored credentials and before
// environment overrides.
func (c *Config) zoomLoadedSnapshot() zoomProfileSnapshot {
	if c == nil || c.fileConfig == nil {
		return zoomProfileSnapshot{}
	}
	return zoomSnapshotFrom(zoomStoredFromConfig(c.fileConfig))
}

// zoomStoredFields mirrors every credential field of cliutil.Credentials.
type zoomStoredFields struct {
	AuthHeader        string    `toml:"auth_header"`
	AccessToken       string    `toml:"access_token"`
	RefreshToken      string    `toml:"refresh_token"`
	TokenExpiry       time.Time `toml:"token_expiry"`
	ClientID          string    `toml:"client_id"`
	ClientSecret      string    `toml:"client_secret"`
	AccountID         string    `toml:"phone_account_id"`
	PhoneClientID     string    `toml:"phone_client_id"`
	PhoneClientSecret string    `toml:"phone_client_secret"`
	MintRecord        string    `toml:"zoom_mint_record"` // derived from the token; never compared
}

func zoomStoredFromConfig(c *Config) zoomStoredFields {
	return zoomStoredFields{
		AuthHeader: c.AuthHeaderVal, AccessToken: c.AccessToken, RefreshToken: c.RefreshToken, TokenExpiry: c.TokenExpiry,
		ClientID: c.ClientID, ClientSecret: c.ClientSecret, AccountID: c.ZoomPhoneAccountId,
		PhoneClientID: c.ZoomPhoneClientId, PhoneClientSecret: c.ZoomPhoneClientSecret,
		MintRecord: c.ZoomMintRecord,
	}
}

func (f zoomStoredFields) hasValues() bool {
	return f.AuthHeader != "" || f.AccessToken != "" || f.RefreshToken != "" || f.ClientID != "" || f.ClientSecret != "" ||
		f.AccountID != "" || f.PhoneClientID != "" || f.PhoneClientSecret != ""
}

// zoomReadStored reads one config or credentials file the way config.Load
// trusts it: a group/world-accessible file is refused (its fields ignored).
func zoomReadStored(path string) (f zoomStoredFields, exists, refused bool) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- the CLI's own config / credentials paths.
	if err != nil {
		return f, false, false
	}
	if toml.Unmarshal(data, &f) != nil {
		return zoomStoredFields{}, true, false
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		if cliutil.VerifyCredsPerms(real) != nil && f.hasValues() {
			return zoomStoredFields{}, true, true
		}
	}
	return f, true, false
}

// zoomDiskStored re-reads what a fresh config.Load of c.Path would take as the
// stored credentials: the config file; when it is not complete (no auth header,
// no access token, not both client id and secret), the config's colocated
// credentials file or, when that one is missing, empty and not refused, the
// data-dir credentials file, filling only empty fields (applyCredentials).
func (c *Config) zoomDiskStored() zoomStoredFields {
	merged, _, _ := zoomReadStored(c.Path)
	if merged.AuthHeader != "" || merged.AccessToken != "" || (merged.ClientID != "" && merged.ClientSecret != "") {
		return merged
	}
	var creds zoomStoredFields
	loaded, refused := false, false
	if p, err := cliutil.CredentialsFilePathForConfig(c.Path); err == nil {
		var exists bool
		creds, exists, refused = zoomReadStored(p)
		loaded = exists && !refused && creds.hasValues()
	}
	if !loaded && !refused {
		if p, err := cliutil.CredentialsFilePath(); err == nil {
			var exists, defRefused bool
			creds, exists, defRefused = zoomReadStored(p)
			loaded = exists && !defRefused && creds.hasValues()
		}
	}
	if loaded {
		fillStr := func(dst *string, src string) {
			if *dst == "" {
				*dst = src
			}
		}
		fillStr(&merged.AuthHeader, creds.AuthHeader)
		fillStr(&merged.AccessToken, creds.AccessToken)
		fillStr(&merged.RefreshToken, creds.RefreshToken)
		if merged.TokenExpiry.IsZero() {
			merged.TokenExpiry = creds.TokenExpiry
		}
		fillStr(&merged.ClientID, creds.ClientID)
		fillStr(&merged.ClientSecret, creds.ClientSecret)
		fillStr(&merged.AccountID, creds.AccountID)
		fillStr(&merged.PhoneClientID, creds.PhoneClientID)
		fillStr(&merged.PhoneClientSecret, creds.PhoneClientSecret)
	}
	return merged
}

func (c *Config) zoomDiskSnapshot() zoomProfileSnapshot {
	return zoomSnapshotFrom(c.zoomDiskStored())
}

func zoomProfileChangedErr(path string) error {
	return fmt.Errorf("%w: %s was replaced, created or removed by another command (a different Zoom app, secret or account), so this run's token was not saved over it; run the command again", ErrZoomProfileChanged, path)
}

// zoomAcctNameRe matches a saved account name (cli acctNameRe).
var zoomAcctNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// ZoomProfileLockPath is the one function every profile writer uses to find
// its lock (cliutil.WithFileLock locks this path + ".lock"). An account
// profile <...>/accounts/<name>/config.toml locks
// <...>/accounts/.locks/<name>.lock: outside the removable account tree, so
// `accounts remove` never deletes a lock a waiter already holds open and a
// recreated profile reuses the same lock inode. The shape match is
// case-insensitive with a lowercased name, so case-variant spellings of one
// profile share the lock. Any other config locks <config>.lock beside itself.
func ZoomProfileLockPath(configPath string) string {
	p := filepath.Clean(configPath)
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if strings.EqualFold(filepath.Base(p), "config.toml") {
		home := filepath.Dir(p)
		name := strings.ToLower(filepath.Base(home))
		root := filepath.Dir(home)
		if zoomAcctNameRe.MatchString(name) && name != "zoom-phone-cli" && strings.EqualFold(filepath.Base(root), "accounts") {
			return filepath.Join(root, zoomLocksDirName, name)
		}
	}
	return p
}

const zoomLocksDirName = ".locks"

// zoomPrepareLockDir creates the lock file's directory (0700). For the shared
// <accounts>/.locks directory it refuses a symlinked accounts or .locks
// directory, so locks cannot be redirected.
func zoomPrepareLockDir(lockBase string) error {
	dir := filepath.Dir(lockBase)
	if filepath.Base(dir) == zoomLocksDirName {
		for _, p := range []string{filepath.Dir(dir), dir} {
			if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing profile lock directory %s: it is a symlink", p)
			}
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating profile lock directory: %w", err)
	}
	if st, err := os.Lstat(dir); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing profile lock directory %s: it is a symlink", dir)
	}
	return nil
}

// ZoomAcquireProfileLock takes the cross-process lock for the profile at
// configPath (see ZoomProfileLockPath) and returns its release. The lock
// file is created 0600 in a 0700 directory and never deleted.
func ZoomAcquireProfileLock(configPath string) (func(), error) {
	lockBase := ZoomProfileLockPath(configPath)
	if err := zoomPrepareLockDir(lockBase); err != nil {
		return nil, err
	}
	locked := make(chan error, 1)
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		err := cliutil.WithFileLock(lockBase, func() error {
			locked <- nil
			<-done
			return nil
		})
		if err != nil {
			select {
			case locked <- err:
			default:
			}
		}
	}()
	if err := <-locked; err != nil {
		return nil, err
	}
	// The release returns only once the goroutine has unlocked and closed the
	// lock file: an open handle under <accounts>/.locks makes Windows refuse
	// to rename or remove the accounts directory (Access is denied).
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-exited
		})
	}, nil
}

// ZoomLockProfileForSave locks the profile for an automatic token save and
// refuses with ErrZoomProfileChanged unless the on-disk identity (app, secret
// hash, account, each present or absent) is exactly what this process loaded.
// That includes a first run that loaded nothing: if another writer created a
// profile meanwhile, the save is refused. Call the release after SaveTokens.
func (c *Config) ZoomLockProfileForSave() (func(), error) {
	return c.zoomLockProfile(true)
}

// ZoomLockProfileForReplace locks the profile for an explicit identity
// replacement (`auth login`): no stale check, but it waits for and excludes
// concurrent saves and `accounts connect --force` / `accounts remove`.
func (c *Config) ZoomLockProfileForReplace() (func(), error) {
	return c.zoomLockProfile(false)
}

func (c *Config) zoomLockProfile(checkStale bool) (func(), error) {
	if c == nil || strings.TrimSpace(c.Path) == "" {
		return func() {}, nil
	}
	loaded := c.zoomLoadedSnapshot()
	// The lock lives outside the profile directory, so it is taken first and
	// every check below runs under it.
	release, err := ZoomAcquireProfileLock(c.Path)
	if err != nil {
		return nil, err
	}
	if checkStale {
		// A removed profile must not be recreated by a save.
		if _, err := os.Lstat(filepath.Dir(c.Path)); errors.Is(err, fs.ErrNotExist) && !loaded.empty() {
			release()
			return nil, zoomProfileChangedErr(c.Path)
		}
		if zoomSnapshotChanged(loaded, c.zoomDiskSnapshot()) {
			release()
			return nil, zoomProfileChangedErr(c.Path)
		}
	}
	return func() {
		// After the save, the file snapshot names what is on disk again (the
		// generated save clears its credential fields), so a later save in this
		// process is checked against the identity it just wrote.
		if c.fileConfig != nil {
			d := c.zoomDiskStored()
			fc := c.fileConfig
			fc.AuthHeaderVal, fc.AccessToken, fc.RefreshToken, fc.TokenExpiry = d.AuthHeader, d.AccessToken, d.RefreshToken, d.TokenExpiry
			fc.ClientID, fc.ClientSecret, fc.ZoomPhoneAccountId = d.ClientID, d.ClientSecret, d.AccountID
			fc.ZoomPhoneClientId, fc.ZoomPhoneClientSecret = d.PhoneClientID, d.PhoneClientSecret
			fc.ZoomMintRecord = d.MintRecord
		}
		release()
	}, nil
}
