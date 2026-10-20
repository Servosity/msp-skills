// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

// Auth context scopes. "partner" is the token from auth login; "account" is
// a customer-scoped token minted by auth use-account.
const (
	authScopePartner = "partner"
	authScopeAccount = "account"
)

// authContextFileName is the sidecar in the state dir that remembers which
// scope is active and keeps the partner token while an account token is in
// use, so use-account --partner can switch back without another login.
const authContextFileName = "auth-context.json"

// authContext is the on-disk shape of the sidecar. Token values live here
// only so the partner session survives a use-account switch; they are never
// printed.
type authContext struct {
	Scope               string    `json:"scope"`
	AccountID           string    `json:"account_id,omitempty"`
	PartnerAccessToken  string    `json:"partner_access_token,omitempty"`
	PartnerRefreshToken string    `json:"partner_refresh_token,omitempty"`
	PartnerTokenExpiry  time.Time `json:"partner_token_expiry,omitempty"`
	RefreshExpiresAt    time.Time `json:"refresh_expires_at,omitempty"`
	// TokenFingerprint and RefreshFingerprint bind this record to the
	// active credential it describes: truncated SHA-256 hashes, never the
	// tokens, each taken over the API origin, the API client id and the
	// token. Principal is who that credential acts as: "partner",
	// "account:<id>" after use-account, or "company:<email hash>" after a
	// company login. The local store is keyed by the principal only while
	// the active access token, origin and client id match, so a token from
	// the environment, from set-token or from another API client is never
	// filed under a principal it was not issued for; and a refresh follows
	// the binding only when the refresh token matches too.
	TokenFingerprint   string `json:"token_fingerprint,omitempty"`
	RefreshFingerprint string `json:"refresh_fingerprint,omitempty"`
	Principal          string `json:"principal,omitempty"`
	// PartnerFingerprint is set only when the kept partner session (access
	// and refresh token together) is known to come from a partner login,
	// directly or by refreshing one. A session kept without that provenance
	// is still used to mint and restore, but is never filed as the partner.
	PartnerFingerprint string    `json:"partner_fingerprint,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Store principals. principalPartner covers every partner login of one API
// client: the key pair is created in one partner portal.
const (
	principalPartner       = "partner"
	principalAccountPrefix = "account:"
	principalCompanyPrefix = "company:"
)

// authBinding is what a binding fingerprint is taken over besides the
// token: the API origin and the API client id the token was issued through.
type authBinding struct {
	origin   string
	clientID string
}

// bindingFor reads the binding inputs from a loaded config.
func bindingFor(cfg *config.Config) authBinding {
	if cfg == nil {
		return authBinding{}
	}
	return authBinding{origin: cloudallyStoreOrigin(cfg.BaseURL), clientID: strings.TrimSpace(cfg.ClientID)}
}

// fingerprint is a truncated SHA-256 over the binding and the given tokens.
// An empty token list element is hashed as empty, so "no refresh token" is
// itself a recorded fact.
func (b authBinding) fingerprint(tokens ...string) string {
	h := sha256.New()
	h.Write([]byte("cloudally-auth-binding\x00" + b.origin + "\x00" + b.clientID))
	for _, t := range tokens {
		h.Write([]byte("\x00" + strings.TrimSpace(t)))
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// companyPrincipal names a company login by a hash of its login email, so
// two companies signed in with the same API key pair never share a store.
func companyPrincipal(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return principalCompanyPrefix + hex.EncodeToString(sum[:8])
}

// bindTo records that access (with its refresh token) is the active
// credential for binding b and acts as principal. An empty principal clears
// the binding, so the token gets the per-token store.
func (a *authContext) bindTo(b authBinding, access, refresh, principal string) {
	if principal == "" || strings.TrimSpace(access) == "" || b.clientID == "" {
		a.TokenFingerprint, a.RefreshFingerprint, a.Principal = "", "", ""
		return
	}
	a.TokenFingerprint = b.fingerprint(access)
	a.RefreshFingerprint = b.fingerprint(refresh)
	a.Principal = principal
}

// activePrincipal returns the principal this record names for access under
// binding b, or "" when the record does not describe it. A record without a
// fingerprint, or one for another token, origin or client id, vouches for
// nothing.
func (a *authContext) activePrincipal(b authBinding, access string) string {
	if a == nil || strings.TrimSpace(access) == "" || a.TokenFingerprint == "" || b.clientID == "" {
		return ""
	}
	if a.TokenFingerprint != b.fingerprint(access) {
		return ""
	}
	return a.Principal
}

// refreshPrincipal is activePrincipal for a credential about to be
// refreshed: the refresh token has to be the one bound too, or a refresh
// token from another session could mint a token filed under this principal.
func (a *authContext) refreshPrincipal(b authBinding, access, refresh string) string {
	p := a.activePrincipal(b, access)
	if p == "" || a.RefreshFingerprint != b.fingerprint(refresh) {
		return ""
	}
	return p
}

// stashPartner keeps a partner session aside. verified says the session is
// known to come from a partner login; only then is it fingerprinted as one.
func (a *authContext) stashPartner(b authBinding, access, refresh string, expiry time.Time, verified bool) {
	a.PartnerAccessToken, a.PartnerRefreshToken, a.PartnerTokenExpiry = access, refresh, expiry
	a.PartnerFingerprint = ""
	if verified && strings.TrimSpace(access) != "" && b.clientID != "" {
		a.PartnerFingerprint = b.fingerprint(access, refresh)
	}
}

// stashIsPartner reports whether the kept session is a verified partner
// session for binding b.
func (a *authContext) stashIsPartner(b authBinding) bool {
	return a != nil && strings.TrimSpace(a.PartnerAccessToken) != "" && b.clientID != "" &&
		a.PartnerFingerprint == b.fingerprint(a.PartnerAccessToken, a.PartnerRefreshToken)
}

func authContextPath() (string, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, authContextFileName), nil
}

// authContextVerifyPerms is the private-file guard the sidecar must pass on
// every write and read: the same check credentials.toml gets (mode 0600 on
// POSIX, owner and DACL on Windows). A variable only so a test can stand in
// for a platform where the written file comes out unsafe.
var authContextVerifyPerms = cliutil.VerifyCredsPerms

// authContextSource names the sidecar in a refusal message.
const authContextSource = "partner session file"

// loadAuthContext reads the sidecar. A missing or unreadable file reports
// ok=false; callers treat that as "partner scope, nothing stashed". The file
// holds partner tokens, so like credentials.toml it is refused (with a
// message on stderr naming the path and the fix) when another user can read
// it, rather than trusted.
func loadAuthContext() (*authContext, bool) {
	path, err := authContextPath()
	if err != nil {
		return nil, false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			cliutil.ReportCredentialRefusal(cliutil.CredentialRefusal{Source: authContextSource, Path: path, Err: err, CredentialsPresent: true})
		}
		return nil, false
	}
	if permErr := authContextVerifyPerms(real); permErr != nil {
		cliutil.ReportCredentialRefusal(cliutil.CredentialRefusal{Source: authContextSource, Path: path, Err: permErr, CredentialsPresent: true})
		return nil, false
	}
	data, err := os.ReadFile(filepath.Clean(real)) // #nosec G304 -- app-owned state path from cliutil.StateDir.
	if err != nil {
		return nil, false
	}
	var actx authContext
	if err := json.Unmarshal(data, &actx); err != nil {
		return nil, false
	}
	return &actx, true
}

// saveAuthContext writes the sidecar the way SaveCredentials writes
// credentials.toml: an atomic private write, then the platform guard on the
// file that landed. A file that fails the guard is removed rather than left
// holding partner tokens where another user can read them.
func saveAuthContext(actx *authContext) error {
	path, err := authContextPath()
	if err != nil {
		return err
	}
	actx.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(actx, "", "  ")
	if err != nil {
		return err
	}
	if err := cliutil.AtomicWritePrivateFile(path, data, 0o600, 0o700); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		err = authContextVerifyPerms(real)
	}
	if err != nil {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return fmt.Errorf("%s at %s failed the private-file check (%v) and could not be removed: %w; delete it by hand", authContextSource, path, err, rmErr)
		}
		return fmt.Errorf("%s at %s failed the private-file check and was removed: %w", authContextSource, path, err)
	}
	return nil
}

// clearAuthContext deletes the sidecar. Every command that clears or
// replaces the stored credential calls it: the sidecar keeps a partner
// access and refresh token aside, so leaving it behind would let
// `auth use-account --partner` bring the partner session back after a
// logout. A file that cannot be removed is an error the caller must surface.
func clearAuthContext() error {
	path, err := authContextPath()
	if err != nil {
		return fmt.Errorf("locating %s: %w", authContextFileName, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the %s %s (it holds the partner tokens): %w; delete it by hand", authContextSource, path, err)
	}
	return nil
}

// maskEmail keeps the first character and the domain: t***@example.com.
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "(none)"
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return email[:1] + "***"
	}
	return local[:1] + "***@" + domain
}

// Env vars for the default single-company account login. Partner login keeps
// the generated CLOUDALLY_PARTNER_EMAIL / CLOUDALLY_PARTNER_PASSWORD so the two
// credential sets cannot be mixed up.
const (
	cloudallyAccountEmailEnv    = "CLOUDALLY_EMAIL"
	cloudallyAccountPasswordEnv = "CLOUDALLY_PASSWORD"
)

// cloudallyPartnerLoginEnv names the partner login's email and password
// variables, in that order.
var cloudallyPartnerLoginEnv = [2]string{"CLOUDALLY_PARTNER_EMAIL", "CLOUDALLY_PARTNER_PASSWORD"}

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var (
		email         string
		password      string
		passwordStdin bool
		partner       bool
		asAccount     bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in as one company account (or --partner) and store the tokens",
		Long: `Sign in to CloudAlly and store the access and refresh tokens in the credentials file.

By default this is a company account login (POST /v2/auth): the CLI can only see and
change that one company's backups. The email and password default to CLOUDALLY_EMAIL
and CLOUDALLY_PASSWORD.

Use --partner for a partner portal login (POST /v2/auth/partner), which can see and
change every customer account. Partner login reads CLOUDALLY_PARTNER_EMAIL and
CLOUDALLY_PARTNER_PASSWORD, and is required for auth use-account and partner-wide
commands such as backup health.

Prefer --password-stdin over --password so the secret never appears in process
arguments or shell history. Every request also needs CLOUDALLY_CLIENT_ID and
CLOUDALLY_CLIENT_SECRET.`,
		Example: `  cloudally-cli auth login
  cloudally-cli auth login --email backups@contoso.com --password-stdin < password-file
  cloudally-cli auth login --partner --email you@example.com --password-stdin < password-file`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			emailEnv, passwordEnv := cloudallyAccountEmailEnv, cloudallyAccountPasswordEnv
			if partner {
				emailEnv, passwordEnv = cloudallyPartnerLoginEnv[0], cloudallyPartnerLoginEnv[1]
			}
			if email == "" {
				if partner {
					email = cfg.CloudallyPartnerEmail
				} else {
					email = os.Getenv(cloudallyAccountEmailEnv)
				}
			}
			if passwordStdin {
				if password != "" {
					return usageErr(errors.New("use either --password or --password-stdin, not both"))
				}
				secret, err := readSecretFromStdin(cmd.InOrStdin())
				if err != nil {
					return usageErr(fmt.Errorf("--password-stdin: %w", err))
				}
				password = secret
			}
			if password == "" {
				if partner {
					password = cfg.CloudallyPartnerPassword
				} else {
					password = os.Getenv(cloudallyAccountPasswordEnv)
				}
			}
			if !requiredInputWaived() {
				if email == "" {
					return usageErr(fmt.Errorf("missing email: pass --email or set %s", emailEnv))
				}
				if password == "" {
					return usageErr(fmt.Errorf("missing password: pass --password-stdin (or --password) or set %s", passwordEnv))
				}
			}

			if partner && asAccount {
				return usageErr(errors.New("use either --partner or --as-account, not both"))
			}
			path := cloudallyAccountLoginPath
			scope := authScopeAccount
			if partner {
				path = cloudallyPartnerLoginPath
				scope = authScopePartner
			}
			action := fmt.Sprintf("POST %s as %s (%s login)", path, maskEmail(email), scope)
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return writeDryRun(cmd.OutOrStdout(), flags, action)
			}
			if cfg.ClientID == "" || cfg.ClientSecret == "" {
				return usageErr(errors.New("missing API client credentials: set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			body, status, err := cloudallyAuthPost(ctx, cfg.BaseURL, path, cfg.ClientID, cfg.ClientSecret, "",
				map[string]string{"email": email, "password": password}, flags.timeout)
			if err != nil {
				return apiErr(fmt.Errorf("login failed: %w", err))
			}
			if status < 200 || status >= 300 {
				statusErr := cloudallyAuthStatusError(status, body)
				if status == 401 || status == 403 {
					return authErr(fmt.Errorf("login failed: %w", statusErr))
				}
				return apiErr(fmt.Errorf("login failed: %w", statusErr))
			}
			accessToken, refreshToken, expiry, refreshExpiry, err := cloudallyParseTokenResponse(body)
			if err != nil {
				return apiErr(fmt.Errorf("login failed: %w", err))
			}

			// Clear any legacy auth_header so AuthHeader() uses the new token.
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, accessToken, refreshToken, expiry); err != nil {
				return configErr(fmt.Errorf("saving tokens: %w", err))
			}
			actx := &authContext{Scope: scope, RefreshExpiresAt: refreshExpiry}
			binding := bindingFor(cfg)
			if scope == authScopePartner {
				actx.stashPartner(binding, accessToken, refreshToken, expiry, true)
				actx.bindTo(binding, accessToken, refreshToken, principalPartner)
			} else {
				actx.bindTo(binding, accessToken, refreshToken, companyPrincipal(email))
			}
			if err := saveAuthContext(actx); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not write %s: %v\n", authContextFileName, err)
				// A sidecar left from an earlier session would describe the
				// wrong token (and could hold an older partner session).
				if clearErr := clearAuthContext(); clearErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", clearErr)
				}
			}

			expiresAt := ""
			if !expiry.IsZero() {
				expiresAt = expiry.UTC().Format(time.RFC3339)
			}
			refreshExpiresAt := ""
			if !refreshExpiry.IsZero() {
				refreshExpiresAt = refreshExpiry.UTC().Format(time.RFC3339)
			}
			if flags.asJSON {
				out := map[string]any{
					"scope":              scope,
					"expires_at":         expiresAt,
					"refresh_expires_at": refreshExpiresAt,
					"credentials_path":   credentialSavePath(cfg),
				}
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			when := "unknown"
			if expiresAt != "" {
				when = expiresAt
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%s Logged in as %s; token expires %s\n", green("OK"), scope, when)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Login email (default: CLOUDALLY_EMAIL, or CLOUDALLY_PARTNER_EMAIL with --partner)")
	cmd.Flags().StringVar(&password, "password", "", "Login password (default: CLOUDALLY_PASSWORD, or CLOUDALLY_PARTNER_PASSWORD with --partner); prefer --password-stdin")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	cmd.Flags().BoolVar(&partner, "partner", false, "Sign in to the partner portal (POST /v2/auth/partner) with access to every customer account")
	cmd.Flags().BoolVar(&asAccount, "as-account", false, "Company account login; this is already the default")
	_ = cmd.Flags().MarkHidden("as-account")
	return cmd
}
