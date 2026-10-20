// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: a stable local-store identity for CloudAlly.
//
// The generated default names the local store after a hash of the auth
// header plus the client id and secret. CloudAlly access tokens rotate on
// every refresh, re-login and use-account switch, so each rotation opened an
// empty store: restore history vanished, `restore submit --resume` reported
// no earlier batch, and synced backup status disappeared.
//
// The store is keyed instead by the API origin and the principal: the
// CloudAlly user the active token acts as (auth_login.go). A partner user,
// each customer account that partner user reaches through use-account, and
// each company login get a store of their own, named by a hash of the login
// email. Different partner users of one partner (an administrator and a
// restricted user) therefore never share cached customer rows, while the
// same user keeps one store across token refreshes, re-logins and a rotated
// API key pair: the key pair identifies the integration, the login decides
// what the token can reach, so the client id is deliberately not in the key.
// No token or secret is part of the key.
//
// The principal comes from auth-context.json, and only while that record's
// fingerprints match the active token. A token the record does not describe
// as a known user (CLOUDALLY_ACCESS_TOKEN, set-token, a session whose user
// is not known) gets the generated per-token key: isolated, as before.
//
// A record that exists but cannot be used (refused for its permissions,
// unreadable, or describing another token) would silently send the run to
// an empty per-token store, where restore history and resume find nothing.
// requireBoundStoreScope lets the commands that read or write the restore
// ledger fail closed instead.
//
// The first run with a principal key adopts the store named for the current
// token (helpers.go adoptLegacyScopedStore, through StoreScopeLegacyCredential).
// That store was filled by this very token, so it is this user's; a store
// filled under any other key is never merged in.

package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
)

func init() {
	config.StoreScopeIdentity = cloudallyStoreScopeIdentity
}

// storeScopeDecision is the outcome of the most recent store-scope
// resolution, which is the one defaultDBPath is using.
var storeScopeDecision struct {
	sync.Mutex
	resolved bool
	err      error
}

// cloudallyStoreScopeIdentity returns the stable store key, or "" (the
// generated credential-derived key) when no auth record names the active
// token's user. It also records whether that fallback is acceptable, for
// requireBoundStoreScope.
func cloudallyStoreScopeIdentity(c *config.Config) string {
	id, err := resolveStoreScope(c)
	storeScopeDecision.Lock()
	storeScopeDecision.resolved, storeScopeDecision.err = true, err
	storeScopeDecision.Unlock()
	return id
}

// errStoreScopeUnbound wraps every requireBoundStoreScope refusal.
var errStoreScopeUnbound = errors.New("the local restore history cannot be matched to the signed-in CloudAlly user")

// resolveStoreScope decides the store identity. err is non-nil when the
// per-token fallback would be wrong (an auth record exists but cannot be
// used for the active credential), or when an unowned data.db could not be
// marked as belonging to no one. The marker is published on every path,
// the per-token ones included: any credential, not only a per-user one,
// must keep out of a data.db another tenant may have filled.
func resolveStoreScope(c *config.Config) (string, error) {
	id, err := decideStoreScope(c)
	if markErr := markUnscopedStoreUnclaimed(); markErr != nil && err == nil {
		err = fmt.Errorf("%w: %v", errStoreScopeUnbound, markErr)
	}
	return id, err
}

// decideStoreScope is resolveStoreScope without the data.db marker.
func decideStoreScope(c *config.Config) (string, error) {
	if c == nil {
		return "", nil
	}
	actx, status, readErr := readAuthContext()
	switch status {
	case authContextMissing:
		// No record: a token from the environment, set-token, or no login.
		return "", nil
	case authContextUnusable:
		return "", fmt.Errorf("%w: %v; fix or delete the file, then run `cloudally-cli auth login` (or `auth login --partner`) again", errStoreScopeUnbound, readErr)
	}
	token := strings.TrimSpace(c.AccessToken)
	if strings.TrimSpace(c.AuthHeaderVal) != "" {
		// A raw auth header from the config is the credential; the record
		// cannot describe it, so it keeps the per-token store.
		return "", nil
	}
	if env := strings.TrimSpace(cliutil.EnvOverride("CLOUDALLY_ACCESS_TOKEN")); env != "" && env == token {
		// A token from the environment is used as given, with its own store.
		return "", nil
	}
	binding := bindingFor(c)
	if token == "" || !actx.accessKnown(binding, token) {
		return "", fmt.Errorf("%w: %s describes a different session than the stored token; run `cloudally-cli auth login` (or `auth login --partner`, then `auth use-account <id>`) again", errStoreScopeUnbound, authContextFileName)
	}
	principal := actx.activePrincipal(binding, token)
	if principal == "" {
		// A known token whose user is not known: its own store, by design.
		return "", nil
	}
	id := "cloudally:origin=" + binding.origin + "|principal=" + principal
	return id, nil
}

// unscopedStoreAdoptable reports whether data.db already belongs to the
// scope with this hash, because its owner marker names it. The generated
// store logic (helpers.go defaultDBPathInDir) calls it before its own claim,
// so an unowned data.db (no marker, an empty one, or the unclaimed marker)
// is never adopted by any credential: a per-user login, a token from the
// environment, a raw auth header, or a token whose user is not known. A
// data.db an earlier run already claimed stays with that owner.
func unscopedStoreAdoptable(unscoped, scopeHash string) bool {
	if strings.TrimSpace(scopeHash) == "" {
		return false
	}
	data, err := os.ReadFile(unscoped + ".owner") // #nosec G304 -- app-owned data path from cliutil.DataDir.
	return err == nil && strings.TrimSpace(string(data)) == scopeHash
}

// unclaimedMarker is the owner marker written for an unscoped data.db.
const unclaimedMarker = "unclaimed: data.db predates per-user stores and is never adopted into one"

// markUnscopedStoreUnclaimed keeps an old unscoped data.db out of every
// store, including an older binary's (which would otherwise still claim it). The generated store logic hands an unowned data.db to the
// first scope that runs; with per-user stores that would give a store of
// unknown origin (perhaps filled by another user) to whichever user signs
// in first. A non-empty owner marker that names no scope makes that claim
// decline. It is published with a hard link, as the generated claim does,
// so it never appears empty. If no non-empty marker can be put in place the
// generated logic would fall back to data.db itself, so that is an error
// the ledger commands fail closed on.
func markUnscopedStoreUnclaimed() error {
	dir, err := cliutil.DataDir()
	if err != nil {
		return nil // defaultDBPath falls back elsewhere; nothing here to protect
	}
	unscoped := filepath.Join(dir, "data.db")
	if _, err := os.Stat(unscoped); err != nil {
		return nil
	}
	marker := unscoped + ".owner"
	if markerDecided(marker) {
		return nil
	}
	if tmp, err := os.CreateTemp(dir, ".data-db-unclaimed-*"); err == nil {
		tmpName := tmp.Name()
		_, werr := tmp.WriteString(unclaimedMarker + "\n")
		serr := tmp.Sync()
		cerr := tmp.Close()
		if werr == nil && serr == nil && cerr == nil {
			// A lost race leaves the other writer's marker in place.
			_ = os.Link(tmpName, marker)
		}
		_ = os.Remove(tmpName)
	}
	if markerDecided(marker) {
		return nil
	}
	return fmt.Errorf("%s has no owner and a marker keeping it out of this user's store could not be written in %s", unscoped, dir)
}

// markerDecided reports whether an owner marker with content exists, which
// makes the generated claim decide instead of falling back to data.db.
func markerDecided(marker string) bool {
	data, err := os.ReadFile(marker) // #nosec G304 -- app-owned data path from cliutil.DataDir.
	return err == nil && strings.TrimSpace(string(data)) != ""
}

// requireBoundStoreScope returns an error when the local store this run is
// using is a per-token fallback chosen only because the auth record could
// not be used. Commands that read or write the restore ledger call it before
// opening the store, so a refused, unreadable or stale record stops them
// instead of showing an empty history or resuming nothing. A run with no
// record, with CLOUDALLY_ACCESS_TOKEN, or with a token whose user is not
// known passes: its per-token store is the right one.
func requireBoundStoreScope() error {
	storeScopeDecision.Lock()
	resolved, err := storeScopeDecision.resolved, storeScopeDecision.err
	storeScopeDecision.Unlock()
	if !resolved {
		cfg, loadErr := config.Load("")
		if loadErr != nil {
			return configErr(loadErr)
		}
		_, err = resolveStoreScope(cfg)
	}
	if err != nil {
		return configErr(err)
	}
	return nil
}

// cloudallyStoreOrigin normalizes an API base URL: scheme and host lower
// case, no trailing slash, so spelling variants of one URL share a store.
// An unparseable value is used as written.
func cloudallyStoreOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
}
