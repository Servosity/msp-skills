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
	// tokens, each taken over the API origin and the token. Principal is
	// the CloudAlly user that credential acts as, which keys the local
	// store: "partner:<email hash>" after a partner login,
	// "partner:<email hash>/account:<id>" after use-account,
	// "company:<email hash>" after a company login. A record whose
	// fingerprints match the active credential but whose Principal is empty
	// describes a token whose user is not known (minted or restored from a
	// session without that provenance); it gets the per-token store.
	TokenFingerprint   string `json:"token_fingerprint,omitempty"`
	RefreshFingerprint string `json:"refresh_fingerprint,omitempty"`
	Principal          string `json:"principal,omitempty"`
	// PartnerFingerprint and PartnerPrincipal are set only when the kept
	// partner session (access and refresh token together) is known to come
	// from that partner user's login, directly or by refreshing one. A
	// session kept without that provenance is still used to mint and
	// restore, but is never filed under a partner principal.
	PartnerFingerprint string    `json:"partner_fingerprint,omitempty"`
	PartnerPrincipal   string    `json:"partner_principal,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Store principals. Each names one CloudAlly user (by a hash of the login
// email), never just an API key pair: one key pair serves every partner
// user and every company login, and those users have different rights.
const (
	principalPartnerPrefix = "partner:"
	principalCompanyPrefix = "company:"
	principalAccountSep    = "/account:"
	// principalLegacyPrefix names a partner session recorded by a build
	// that did not record the login email. It is keyed by the session's own
	// token, so it never shares a store with any other login.
	principalLegacyPrefix = "partner-legacy:"
)

// userKey is a short hash of a login email, case and spacing ignored.
func userKey(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:8])
}

// partnerPrincipal names one partner user.
func partnerPrincipal(email string) string { return principalPartnerPrefix + userKey(email) }

// companyPrincipal names one company login, so two companies signed in with
// the same API key pair never share a store.
func companyPrincipal(email string) string { return principalCompanyPrefix + userKey(email) }

// accountPrincipal names one customer account reached by one partner user.
func accountPrincipal(partner, accountID string) string {
	return partner + principalAccountSep + accountID
}

// isPartnerPrincipal reports whether p names a partner user's own session
// (not a customer account reached through it).
func isPartnerPrincipal(p string) bool {
	return (strings.HasPrefix(p, principalPartnerPrefix) || strings.HasPrefix(p, principalLegacyPrefix)) &&
		!strings.Contains(p, principalAccountSep)
}

// authBinding is what a binding fingerprint is taken over besides the
// token: the API origin the token was issued by. The API client id is
// deliberately not part of it: the key pair identifies the integration,
// the login decides what a token can reach, so rotating the key pair keeps
// the same user (and store).
type authBinding struct {
	origin string
}

// bindingFor reads the binding inputs from a loaded config.
func bindingFor(cfg *config.Config) authBinding {
	if cfg == nil {
		return authBinding{}
	}
	return authBinding{origin: cloudallyStoreOrigin(cfg.BaseURL)}
}

// fingerprint is a truncated SHA-256 over the binding and the given tokens.
// An empty token list element is hashed as empty, so "no refresh token" is
// itself a recorded fact.
func (b authBinding) fingerprint(tokens ...string) string {
	h := sha256.New()
	h.Write([]byte("cloudally-auth-binding\x00" + b.origin))
	for _, t := range tokens {
		h.Write([]byte("\x00" + strings.TrimSpace(t)))
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// legacyPartnerToken returns the kept partner token of a record written
// before fingerprints existed, or "" for any other record. In such a record
// the kept partner token is the active token.
func (a *authContext) legacyPartnerToken() string {
	if a == nil || a.TokenFingerprint != "" || a.PartnerFingerprint != "" || a.Scope != authScopePartner {
		return ""
	}
	return strings.TrimSpace(a.PartnerAccessToken)
}

// legacyPrincipal is the principal for a pre-fingerprint partner record:
// keyed by that session's token, so it is that session's alone.
func (b authBinding) legacyPrincipal(token string) string {
	return principalLegacyPrefix + b.fingerprint("legacy-partner", token)
}

// bindTo records that access (with its refresh token) is the active
// credential and acts as principal. An empty principal records a known
// token whose user is not known: it gets the per-token store.
func (a *authContext) bindTo(b authBinding, access, refresh, principal string) {
	if strings.TrimSpace(access) == "" || b.origin == "" {
		a.TokenFingerprint, a.RefreshFingerprint, a.Principal = "", "", ""
		return
	}
	a.TokenFingerprint = b.fingerprint(access)
	a.RefreshFingerprint = b.fingerprint(refresh)
	a.Principal = principal
}

// accessKnown reports whether this record describes the access token.
func (a *authContext) accessKnown(b authBinding, access string) bool {
	access = strings.TrimSpace(access)
	if a == nil || access == "" || b.origin == "" {
		return false
	}
	if legacy := a.legacyPartnerToken(); legacy != "" {
		return legacy == access
	}
	return a.TokenFingerprint != "" && a.TokenFingerprint == b.fingerprint(access)
}

// pairKnown reports whether this record describes both the access token
// and the refresh token that goes with it.
func (a *authContext) pairKnown(b authBinding, access, refresh string) bool {
	if !a.accessKnown(b, access) {
		return false
	}
	if a.legacyPartnerToken() != "" {
		return strings.TrimSpace(a.PartnerRefreshToken) == strings.TrimSpace(refresh)
	}
	return a.RefreshFingerprint == b.fingerprint(refresh)
}

// activePrincipal returns the principal this record names for access, or
// "" when the record does not describe it or names no user for it.
func (a *authContext) activePrincipal(b authBinding, access string) string {
	if !a.accessKnown(b, access) {
		return ""
	}
	if a.legacyPartnerToken() != "" {
		return b.legacyPrincipal(access)
	}
	return a.Principal
}

// refreshPrincipal is activePrincipal for a credential about to be
// refreshed: the refresh token has to be the one recorded too, or a refresh
// token from another session could mint a token filed under this principal.
func (a *authContext) refreshPrincipal(b authBinding, access, refresh string) string {
	if !a.pairKnown(b, access, refresh) {
		return ""
	}
	return a.activePrincipal(b, access)
}

// stashPartner keeps a partner session aside. principal is the partner user
// it is known to belong to, or "" when that is not known; only a known
// partner session is fingerprinted as one.
func (a *authContext) stashPartner(b authBinding, access, refresh string, expiry time.Time, principal string) {
	a.PartnerAccessToken, a.PartnerRefreshToken, a.PartnerTokenExpiry = access, refresh, expiry
	a.PartnerFingerprint, a.PartnerPrincipal = "", ""
	if isPartnerPrincipal(principal) && strings.TrimSpace(access) != "" && b.origin != "" {
		a.PartnerFingerprint = b.fingerprint(access, refresh)
		a.PartnerPrincipal = principal
	}
}

// stashPrincipal returns the partner user the kept session is known to
// belong to, or "".
func (a *authContext) stashPrincipal(b authBinding) string {
	if a == nil || strings.TrimSpace(a.PartnerAccessToken) == "" || b.origin == "" {
		return ""
	}
	if legacy := a.legacyPartnerToken(); legacy != "" {
		return b.legacyPrincipal(legacy)
	}
	if a.PartnerFingerprint == "" || a.PartnerFingerprint != b.fingerprint(a.PartnerAccessToken, a.PartnerRefreshToken) {
		return ""
	}
	if !isPartnerPrincipal(a.PartnerPrincipal) {
		return ""
	}
	return a.PartnerPrincipal
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

// authContextStatus says what readAuthContext found.
type authContextStatus int

const (
	// authContextMissing: there is no auth record.
	authContextMissing authContextStatus = iota
	// authContextOK: the record was read and passed the private-file check.
	authContextOK
	// authContextUnusable: a record exists but was refused (another user
	// can read it, or its owner is wrong) or could not be read or parsed.
	authContextUnusable
)

// readAuthContext reads the sidecar and reports why it could not when it
// exists but is unusable. The file holds partner tokens, so like
// credentials.toml it is refused (with a message on stderr naming the path
// and the fix) when another user can read it, rather than trusted.
func readAuthContext() (*authContext, authContextStatus, error) {
	path, err := authContextPath()
	if err != nil {
		return nil, authContextUnusable, fmt.Errorf("locating %s: %w", authContextFileName, err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, lerr := os.Lstat(path); lerr != nil && errors.Is(lerr, os.ErrNotExist) {
				return nil, authContextMissing, nil
			}
		}
		cliutil.ReportCredentialRefusal(cliutil.CredentialRefusal{Source: authContextSource, Path: path, Err: err, CredentialsPresent: true})
		return nil, authContextUnusable, fmt.Errorf("%s %s cannot be resolved: %w", authContextSource, path, err)
	}
	if permErr := authContextVerifyPerms(real); permErr != nil {
		cliutil.ReportCredentialRefusal(cliutil.CredentialRefusal{Source: authContextSource, Path: path, Err: permErr, CredentialsPresent: true})
		return nil, authContextUnusable, fmt.Errorf("%s %s was refused: %w", authContextSource, path, permErr)
	}
	data, err := os.ReadFile(filepath.Clean(real)) // #nosec G304 -- app-owned state path from cliutil.StateDir.
	if err != nil {
		return nil, authContextUnusable, fmt.Errorf("reading %s %s: %w", authContextSource, path, err)
	}
	var actx authContext
	if err := json.Unmarshal(data, &actx); err != nil {
		return nil, authContextUnusable, fmt.Errorf("parsing %s %s: %w", authContextSource, path, err)
	}
	return &actx, authContextOK, nil
}

// loadAuthContext reads the sidecar. A missing or unusable file reports
// ok=false; callers treat that as "partner scope, nothing stashed". Code
// that keys local state by the record checks readAuthContext's status
// instead (requireBoundStoreScope), so an unusable record fails closed.
func loadAuthContext() (*authContext, bool) {
	actx, status, _ := readAuthContext()
	return actx, status == authContextOK
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
  cloudally-cli auth login --partner --email you@example.com --password-stdin < password-file
  # Windows PowerShell has no '<' redirection:
  Get-Content password-file | cloudally-cli auth login --partner --email you@example.com --password-stdin`,
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
				principal := partnerPrincipal(email)
				actx.stashPartner(binding, accessToken, refreshToken, expiry, principal)
				actx.bindTo(binding, accessToken, refreshToken, principal)
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
