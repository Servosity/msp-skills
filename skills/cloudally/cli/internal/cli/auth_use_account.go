// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

// Partner-access-token endpoints. The {id} segment is the customer account.
const (
	cloudallyDistributorHandoffPath = "/v2/distributors/accounts/{id}/partner-access-token"
	cloudallyResellerHandoffPath    = "/v2/resellers/accounts/{id}/partner-access-token"
)

// useAccountPlaceholderID stands in for the account id when the harness
// waives required input and no id was given.
const useAccountPlaceholderID = "<accountId>"

func newAuthUseAccountCmd(flags *rootFlags) *cobra.Command {
	var (
		reseller bool
		partner  bool
	)
	cmd := &cobra.Command{
		Use:   "use-account [accountId]",
		Short: "Act inside a customer account (or --partner to switch back)",
		Long: "Mint an account-scoped access token for a customer account and make it the active token.\n\n" +
			"Account-scoped endpoints (backup tasks, recovery, user) then act inside that account.\n" +
			"The partner token from auth login --partner is kept aside so `auth use-account --partner` can\n" +
			"restore it without signing in again. Uses the distributor endpoint by default;\n" +
			"pass --reseller when the partner is a reseller.",
		Example: "  cloudally-cli auth use-account 5f1c2a\n" +
			"  cloudally-cli auth use-account 5f1c2a --reseller\n" +
			"  cloudally-cli auth use-account --partner",
		Annotations: map[string]string{"pp:happy-args": "<accountId>=5f1c2a", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if partner {
				if len(args) > 0 {
					return usageErr(errors.New("--partner takes no account id; drop the argument or drop --partner"))
				}
				return runUseAccountPartner(cmd, flags)
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("expected one account id, got %d; usage: auth use-account <accountId>", len(args)))
			}
			accountID := ""
			if len(args) == 1 {
				accountID = strings.TrimSpace(args[0])
			}
			if accountID == "" {
				if !requiredInputWaived() {
					return usageErr(errors.New("missing account id: pass it as the first argument (auth use-account <accountId>), or use --partner to restore the partner token"))
				}
				accountID = useAccountPlaceholderID
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			path := cloudallyDistributorHandoffPath
			if reseller {
				path = cloudallyResellerHandoffPath
			}
			if accountID != useAccountPlaceholderID {
				path = replaceURLIDPathParam(path, "id", accountID)
			}
			action := fmt.Sprintf("POST %s and make the account token active", path)
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return writeDryRun(cmd.OutOrStdout(), flags, action)
			}
			if cfg.ClientID == "" || cfg.ClientSecret == "" {
				return usageErr(errors.New("missing API client credentials: set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET"))
			}

			// Mint with the partner token even when an account token is active,
			// otherwise switching between two accounts needs a fresh login.
			actx, _ := loadAuthContext()
			if actx == nil {
				actx = &authContext{Scope: authScopePartner}
			}
			partnerToken := strings.TrimSpace(cfg.AccessToken)
			partnerRefresh := cfg.RefreshToken
			partnerExpiry := cfg.TokenExpiry
			stashed := false
			// activeKnown and activePrincipal say whether the auth record
			// describes the active credential (access and refresh token) and
			// which user it is; mintingPartner is the partner user the
			// minting session is known to belong to. None is assumed: a
			// company token, or one from the environment, is never filed
			// under a partner user.
			binding := bindingFor(cfg)
			activeKnown := actx.pairKnown(binding, cfg.AccessToken, cfg.RefreshToken)
			activePrincipal := actx.refreshPrincipal(binding, cfg.AccessToken, cfg.RefreshToken)
			mintingPartner := ""
			if isPartnerPrincipal(activePrincipal) {
				mintingPartner = activePrincipal
			}
			if actx.Scope == authScopeAccount && actx.PartnerAccessToken != "" {
				partnerToken = actx.PartnerAccessToken
				partnerRefresh = actx.PartnerRefreshToken
				partnerExpiry = actx.PartnerTokenExpiry
				stashed = true
				mintingPartner = actx.stashPrincipal(binding)
			}
			if partnerToken == "" {
				return authErr(errors.New("no partner token available: run `cloudally-cli auth login --partner` first, or set CLOUDALLY_ACCESS_TOKEN"))
			}
			if partnerTokenNeedsRefresh(partnerRefresh, partnerExpiry, time.Now()) {
				access, refresh, expiry, refreshExpiry, err := cloudallyRefreshTokens(cfg.BaseURL, cfg.ClientID, cfg.ClientSecret, partnerRefresh)
				if err != nil {
					return err
				}
				partnerToken, partnerRefresh, partnerExpiry = access, refresh, expiry
				// Persist the refreshed partner session before minting, so a
				// failed mint does not strand a rotated refresh token.
				actx.stashPartner(binding, access, refresh, expiry, mintingPartner)
				if !refreshExpiry.IsZero() {
					actx.RefreshExpiresAt = refreshExpiry
				}
				if !stashed {
					// The refreshed token becomes the active one. It keeps the
					// user the record knew for the old pair; a pair the record
					// did not describe gives a known token with no user.
					principal := ""
					if activeKnown {
						principal = activePrincipal
					}
					actx.bindTo(binding, access, refresh, principal)
					cfg.AuthHeaderVal = ""
					if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, access, refresh, expiry); err != nil {
						return configErr(fmt.Errorf("saving refreshed partner token: %w", err))
					}
				}
				if err := saveAuthContext(actx); err != nil {
					return configErr(fmt.Errorf("saving refreshed partner token (%s): %w", authContextFileName, err))
				}
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			body, status, err := cloudallyAuthPost(ctx, cfg.BaseURL, path, cfg.ClientID, cfg.ClientSecret, partnerToken, nil, flags.timeout)
			if err != nil {
				return apiErr(fmt.Errorf("minting account token: %w", err))
			}
			if status == 404 {
				return notFoundErr(fmt.Errorf("account %q not found under this partner (try --reseller if the partner is a reseller)", accountID))
			}
			if status < 200 || status >= 300 {
				statusErr := cloudallyAuthStatusError(status, body)
				if status == 401 || status == 403 {
					return authErr(fmt.Errorf("minting account token: %w; run `cloudally-cli auth login --partner` if the partner token expired", statusErr))
				}
				return apiErr(fmt.Errorf("minting account token: %w", statusErr))
			}
			var resp struct {
				AccessToken string `json:"accessToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil || resp.AccessToken == "" {
				return apiErr(errors.New("minting account token: response has no accessToken"))
			}
			expiry, _ := jwtExpiry(resp.AccessToken)

			// Stash the partner session before overwriting the active token.
			actx.Scope = authScopeAccount
			actx.AccountID = accountID
			actx.stashPartner(binding, partnerToken, partnerRefresh, partnerExpiry, mintingPartner)
			// The API minted this token for this account on behalf of the
			// minting partner user; the store is that user's view of that
			// account. Without a known partner user the token has no user.
			principal := ""
			if mintingPartner != "" {
				principal = accountPrincipal(mintingPartner, accountID)
			}
			actx.bindTo(binding, resp.AccessToken, "", principal)
			if err := saveAuthContext(actx); err != nil {
				return configErr(fmt.Errorf("saving partner token aside (%s): %w", authContextFileName, err))
			}
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, resp.AccessToken, "", expiry); err != nil {
				return configErr(fmt.Errorf("saving account token: %w", err))
			}
			return writeUseAccountResult(cmd, flags, authScopeAccount, accountID, expiry)
		},
	}
	cmd.Flags().BoolVar(&reseller, "reseller", false, "Mint through /v2/resellers/accounts/{id}/partner-access-token")
	cmd.Flags().BoolVar(&partner, "partner", false, "Restore the partner token saved by auth login")
	return cmd
}

// runUseAccountPartner puts the stashed partner token back as the active one.
func runUseAccountPartner(cmd *cobra.Command, flags *rootFlags) error {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return configErr(err)
	}
	if dryRunOK(flags) || cliutil.IsVerifyEnv() {
		return writeDryRun(cmd.OutOrStdout(), flags, "restore the partner token from "+authContextFileName)
	}
	actx, ok := loadAuthContext()
	if ok && actx.Scope == authScopePartner && cfg.AccessToken != "" &&
		(actx.PartnerAccessToken == "" || isPartnerPrincipal(actx.activePrincipal(bindingFor(cfg), cfg.AccessToken))) {
		// The partner session is already the active one.
		return writeUseAccountResult(cmd, flags, authScopePartner, "", cfg.TokenExpiry)
	}
	if !ok || actx.PartnerAccessToken == "" {
		return authErr(errors.New("no saved partner token to restore: run `cloudally-cli auth login --partner`"))
	}
	// Only a kept session known to come from a partner user's login is
	// filed under that user; anything else is a known token with no user,
	// which gets the per-token store. Decided before the record changes
	// scope, so an old record's unverified stash cannot pass as a partner
	// session.
	binding := bindingFor(cfg)
	restored := actx.stashPrincipal(binding)
	cfg.AuthHeaderVal = ""
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, actx.PartnerAccessToken, actx.PartnerRefreshToken, actx.PartnerTokenExpiry); err != nil {
		return configErr(fmt.Errorf("restoring partner token: %w", err))
	}
	actx.Scope = authScopePartner
	actx.AccountID = ""
	actx.bindTo(binding, actx.PartnerAccessToken, actx.PartnerRefreshToken, restored)
	actx.stashPartner(binding, actx.PartnerAccessToken, actx.PartnerRefreshToken, actx.PartnerTokenExpiry, restored)
	if err := saveAuthContext(actx); err != nil {
		return configErr(fmt.Errorf("the partner token is active again, but %s could not be updated: %w; local restore history stays unavailable until `cloudally-cli auth login --partner`", authContextFileName, err))
	}
	return writeUseAccountResult(cmd, flags, authScopePartner, "", actx.PartnerTokenExpiry)
}

func writeUseAccountResult(cmd *cobra.Command, flags *rootFlags, scope, accountID string, expiry time.Time) error {
	expiresAt := ""
	if !expiry.IsZero() {
		expiresAt = expiry.UTC().Format(time.RFC3339)
	}
	if flags.asJSON {
		out := map[string]any{
			"scope":      scope,
			"expires_at": expiresAt,
		}
		if accountID != "" {
			out["account_id"] = accountID
		}
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	when := "unknown"
	if expiresAt != "" {
		when = expiresAt
	}
	if scope == authScopeAccount {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s Acting as account %s; token expires %s\n", green("OK"), accountID, when)
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s Restored partner token; token expires %s\n", green("OK"), when)
	return nil
}

// partnerTokenNeedsRefresh reports whether the partner token is expired or
// within cloudallyRefreshWindow of expiry and a refresh token can renew it.
// An unknown (zero) expiry is left alone.
func partnerTokenNeedsRefresh(refreshToken string, expiry, now time.Time) bool {
	if strings.TrimSpace(refreshToken) == "" || expiry.IsZero() {
		return false
	}
	return expiry.Sub(now) <= cloudallyRefreshWindow
}
