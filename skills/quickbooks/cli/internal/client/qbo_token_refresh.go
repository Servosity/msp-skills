// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/config"
)

// QuickBooks access tokens last about an hour; the refresh token that mints a
// new one rotates on refresh. The client refreshes an access token that is
// missing or expires within qboRefreshWindow before a request, and refreshes
// once and retries once after a 401. Every refresh runs inside one
// cross-process lock (cliutil.QBOWithTokenLock beside the credential store) around
// re-read, check, refresh and save, so the CLI and the MCP server refreshing
// at the same moment make one token call: the second re-reads the store and
// uses the token the first saved.

// qboRefreshWindow refreshes before the expiry so a request in flight does
// not race it.
const qboRefreshWindow = 60 * time.Second

// QBOTokenGrant performs the OAuth 2.0 refresh_token grant against the token
// endpoint and returns the new access token, the (possibly rotated) refresh
// token and the access token's lifetime in seconds. The cli package installs
// it (the same code `auth refresh` uses); with none installed the client
// never refreshes. It must hold every value of the token answer with the
// credential kit as it reads it, and its errors must quote nothing the
// endpoint wrote beyond an RFC 6749 error code.
var QBOTokenGrant func(ctx context.Context, clientID, clientSecret, refreshToken string) (access, refresh string, expiresIn int, err error)

type qboTokenTransportKey struct{}

// QBOTokenTransport is the transport QBORefreshTokens gave the grant it called,
// or nil (the default transport) for a grant called directly.
func QBOTokenTransport(ctx context.Context) http.RoundTripper {
	rt, _ := ctx.Value(qboTokenTransportKey{}).(http.RoundTripper)
	return rt
}

// ErrRefreshTokenRejected marks a grant the token endpoint refused with
// invalid_grant: the refresh token is expired, revoked or already rotated.
var ErrRefreshTokenRejected = errors.New("refresh token rejected")

// QBOTokenSaveError is a refresh whose new tokens could not be saved. The
// request that needed the refresh fails with it rather than going ahead:
// QuickBooks may already have retired the refresh token the grant sent, so
// the operator has to know the store did not take the new one.
type QBOTokenSaveError struct{ Err error }

func (e *QBOTokenSaveError) Error() string { return "saving refreshed token: " + e.Err.Error() }
func (e *QBOTokenSaveError) Unwrap() error { return e.Err }

// qboRejectedRefreshTokens remembers, for the life of the process, every
// refresh token the token endpoint rejected with invalid_grant (by digest),
// so a long command or the MCP server never sends a dead refresh token again
// on each later request. A transient failure (network, 5xx) is not
// remembered: the next request may succeed.
var qboRejectedRefreshTokens sync.Map

func qboRefreshTokenDigest(rt string) [32]byte { return sha256.Sum256([]byte(rt)) }

// QBORefreshCredentials resolves the OAuth client and the refresh tokens to
// try, in order. The client id and secret come from the environment, else the
// stored configuration. The stored refresh token comes first, because a
// refresh saves the token QuickBooks rotated to there; QUICKBOOKS_REFRESH_TOKEN
// follows, for a first refresh or when the stored token is rejected. An
// unresolved bundle placeholder in the environment counts as unset.
func QBORefreshCredentials(cfg *config.Config) (clientID, clientSecret string, refreshTokens []string) {
	if cfg == nil {
		return "", "", nil
	}
	clientID = firstSetValue(cliutil.EnvOverride("QUICKBOOKS_CLIENT_ID"), cfg.ClientID)
	clientSecret = firstSetValue(cliutil.EnvOverride("QUICKBOOKS_CLIENT_SECRET"), cfg.ClientSecret)
	for _, rt := range []string{cfg.RefreshToken, cliutil.EnvOverride("QUICKBOOKS_REFRESH_TOKEN")} {
		rt = strings.TrimSpace(rt)
		if rt == "" || authHeaderLooksLikePlaceholderCredential(rt) {
			continue
		}
		if _, rejected := qboRejectedRefreshTokens.Load(qboRefreshTokenDigest(rt)); rejected {
			continue
		}
		if len(refreshTokens) == 0 || refreshTokens[0] != rt {
			refreshTokens = append(refreshTokens, rt)
		}
	}
	return clientID, clientSecret, refreshTokens
}

func firstSetValue(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// qboCanRefresh reports whether this configuration may refresh on its own:
// a grant is installed, the route may carry credentials, no static
// auth_header is configured (that header is the operator's choice, not an
// OAuth token), this is not a mock verify run, and a client id, client
// secret and refresh token are all available.
func qboCanRefresh(cfg *config.Config) bool {
	if cfg == nil || QBOTokenGrant == nil {
		return false
	}
	if cfg.CredentialRouteBlocked() || strings.TrimSpace(cfg.AuthHeaderVal) != "" {
		return false
	}
	if cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv() {
		return false
	}
	id, secret, rts := QBORefreshCredentials(cfg)
	if id == "" || secret == "" || len(rts) == 0 {
		return false
	}
	return !authHeaderLooksLikePlaceholderCredential(id) && !authHeaderLooksLikePlaceholderCredential(secret)
}

// qboTokenExpiring reports whether the access token is missing or expires
// within qboRefreshWindow. A token with no recorded expiry is trusted until
// the API answers 401.
func qboTokenExpiring(cfg *config.Config) bool {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return true
	}
	return !cfg.TokenExpiry.IsZero() && time.Until(cfg.TokenExpiry) < qboRefreshWindow
}

// QBORefreshResult says what a refresh did.
type QBORefreshResult struct {
	// Adopted is true when another process had already saved a fresh token,
	// which this one now uses without a token call.
	Adopted   bool
	ExpiresIn int
	Expiry    time.Time
}

// QBORefreshTokens is the one token refresh. Inside the cross-process lock it
// re-reads the configuration; unless alwaysGrant, a stored access token that
// differs from current and is good for longer than qboRefreshWindow is
// adopted (another process refreshed first). Otherwise it calls the token
// grant, trying the next refresh token only when one is rejected
// (invalid_grant), saves the new access token, refresh token and expiry to
// the credential store before returning, and points cfg at them. The caller
// serializes access to cfg.
func QBORefreshTokens(ctx context.Context, cfg *config.Config, current string, alwaysGrant bool) (QBORefreshResult, error) {
	var res QBORefreshResult
	if cfg == nil {
		return res, errors.New("no configuration loaded")
	}
	if QBOTokenGrant == nil {
		return res, errors.New("no token grant is installed")
	}
	lockPath, err := cfg.QBOTokenLockPath()
	if err != nil {
		return res, fmt.Errorf("locating the credential store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return res, fmt.Errorf("creating the credential store directory: %w", err)
	}
	err = cliutil.QBOWithTokenLock(lockPath, func() error {
		fresh, err := cfg.QBOReload()
		if err != nil {
			return fmt.Errorf("re-reading the configuration: %w", err)
		}
		if !alwaysGrant {
			stored, expiry := fresh.QBOStoredToken()
			if stored != "" && stored != current && !expiry.IsZero() && time.Until(expiry) >= qboRefreshWindow {
				cfg.QBOAdoptTokens(stored, fresh.RefreshToken, expiry)
				res = QBORefreshResult{Adopted: true, ExpiresIn: int(time.Until(expiry).Seconds()), Expiry: expiry}
				return nil
			}
		}
		clientID, clientSecret, refreshTokens := QBORefreshCredentials(fresh)
		if clientID == "" || clientSecret == "" || len(refreshTokens) == 0 {
			return errors.New("a refresh needs QUICKBOOKS_CLIENT_ID, QUICKBOOKS_CLIENT_SECRET and a refresh token")
		}
		// Hand-fix (handfixes.json qbo-token-refresh): the token endpoint gets the
		// trust the API gets (ca_cert, tls_server_name), so a token endpoint on
		// the base URL's origin verifies as that origin does.
		tokenClient, err := newHTTPClient(0, plainNilJar(), tlsSettingsFromConfig(fresh))
		if err != nil {
			return err
		}
		grantCtx := context.WithValue(ctx, qboTokenTransportKey{}, tokenClient.Transport)
		var lastErr error
		for _, rt := range refreshTokens {
			access, rotated, expiresIn, err := QBOTokenGrant(grantCtx, clientID, clientSecret, rt)
			if err != nil {
				lastErr = err
				if errors.Is(err, ErrRefreshTokenRejected) {
					qboRejectedRefreshTokens.Store(qboRefreshTokenDigest(rt), struct{}{})
					continue
				}
				break
			}
			if rotated == "" {
				rotated = rt // not every answer rotates
			}
			if expiresIn <= 0 {
				expiresIn = 3600
			}
			expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
			// Saved before anything retries with it: QuickBooks may already
			// have retired the refresh token this call sent. Only the tokens
			// are saved; a client secret set only in the environment stays
			// out of the store.
			if err := fresh.QBOSaveRefreshedTokens(access, rotated, expiry); err != nil {
				// This process keeps the new pair, so its later requests do
				// not spend the old refresh token again.
				cfg.QBOAdoptTokens(access, rotated, expiry)
				return &QBOTokenSaveError{Err: err}
			}
			cfg.QBOAdoptTokens(access, rotated, expiry)
			res = QBORefreshResult{ExpiresIn: expiresIn, Expiry: expiry}
			return nil
		}
		return lastErr
	})
	return res, err
}

// qboRefreshBeforeRequest refreshes an access token that is missing or about
// to expire. It never runs for --dry-run.
func (c *Client) qboRefreshBeforeRequest(ctx context.Context) error {
	if c == nil || c.Config == nil || c.DryRun {
		return nil
	}
	c.lockTokenState()
	defer c.unlockTokenState()
	if !qboCanRefresh(c.Config) || !qboTokenExpiring(c.Config) {
		return nil
	}
	_, err := QBORefreshTokens(ctx, c.Config, c.Config.AccessToken, false)
	return err
}

// qboRefreshAfterUnauthorized handles a 401 sent with failedHeader. It
// returns the header to retry with, or retry=false when there is nothing new
// to try. A refresh that already failed before this request (earlier) is not
// repeated: its error is returned.
func (c *Client) qboRefreshAfterUnauthorized(ctx context.Context, failedHeader string, earlier error) (header string, retry bool, err error) {
	if c == nil || c.Config == nil || c.DryRun {
		return "", false, nil
	}
	c.lockTokenState()
	defer c.unlockTokenState()
	if !qboCanRefresh(c.Config) {
		return "", false, nil
	}
	if earlier != nil {
		return "", false, earlier
	}
	// Another request on this client may have refreshed while this one was
	// in flight.
	if h := c.Config.AuthHeader(); h != "" && h != failedHeader {
		return h, true, nil
	}
	if _, err := QBORefreshTokens(ctx, c.Config, c.Config.AccessToken, false); err != nil {
		return "", false, err
	}
	h := c.Config.AuthHeader()
	if h == "" || h == failedHeader || authHeaderLooksLikePlaceholderCredential(h) {
		return "", false, nil
	}
	return h, true, nil
}
