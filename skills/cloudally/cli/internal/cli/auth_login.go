// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
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
	UpdatedAt           time.Time `json:"updated_at"`
}

func authContextPath() (string, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, authContextFileName), nil
}

// loadAuthContext reads the sidecar. A missing or unreadable file reports
// ok=false; callers treat that as "partner scope, nothing stashed".
func loadAuthContext() (*authContext, bool) {
	path, err := authContextPath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, false
	}
	var actx authContext
	if err := json.Unmarshal(data, &actx); err != nil {
		return nil, false
	}
	return &actx, true
}

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
	return cliutil.AtomicWritePrivateFile(path, data, 0o600, 0o700)
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
CLOUDALLY_PARTNER_PASSWORD, and is required for auth use-account and the distributor
and reseller commands.

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
			if scope == authScopePartner {
				actx.PartnerAccessToken = accessToken
				actx.PartnerRefreshToken = refreshToken
				actx.PartnerTokenExpiry = expiry
			}
			if err := saveAuthContext(actx); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not write %s: %v\n", authContextFileName, err)
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
