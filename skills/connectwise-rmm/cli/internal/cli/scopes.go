// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: probe which OAuth scopes the configured API key was granted.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/client"
	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/config"
)

type scopeDenial struct {
	Scope string `json:"scope"`
	Error string `json:"error"`
}

type scopesView struct {
	TokenURL   string        `json:"token_url"`
	Granted    []string      `json:"granted"`
	Denied     []scopeDenial `json:"denied"`
	CombinedOK bool          `json:"combined_ok"`
	Export     string        `json:"export,omitempty"`
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newScopesCmd(flags))
	})
}

func newScopesCmd(flags *rootFlags) *cobra.Command {
	var scopes string
	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "Find which OAuth scopes your API key was granted and print the exact scope setting",
		Long: strings.Trim(`
ConnectWise rejects a whole token request when it asks for a single scope the
API key was not granted. This command mints a token for each candidate scope
on its own, reports granted and denied scopes, confirms the granted set works
together, and prints the CONNECTWISE_RMM_OAUTH_SCOPE line to export.

Candidates are --scopes when given, otherwise the configured scope setting (or
every scope this CLI uses). Tokens are minted and discarded; nothing is saved
and no token or secret is printed.

Exit codes: 0 when at least one scope was granted; 4 when credentials are
missing or no scope could be minted.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli scopes --agent
  connectwise-rmm-cli scopes --scopes "platform.devices.read platform.patching.read"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,4"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "scopes")
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			id, secret := cfg.AsioClientCredentials()
			if id == "" || secret == "" {
				return authErr(fmt.Errorf("set CONNECTWISE_RMM_CLIENT_ID and CONNECTWISE_RMM_CLIENT_SECRET (Integrations > API Access in the ConnectWise portal)"))
			}
			// The verify harness mocks tokens (its token URL is a placeholder),
			// so never mint under it.
			if cliutil.IsVerifyEnv() {
				return printScopes(cmd, flags, scopesView{TokenURL: cfg.AsioTokenURL(), Granted: make([]string, 0), Denied: make([]scopeDenial, 0)})
			}
			candidates := strings.FieldsFunc(firstNonEmpty(scopes, resolveClientCredentialsScope()), func(r rune) bool { return r == ' ' || r == ',' })
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := scopesView{TokenURL: cfg.AsioTokenURL(), Granted: make([]string, 0), Denied: make([]scopeDenial, 0)}
			hc := cliutil.OAuthTokenHTTPClient(http.DefaultClient)
			for _, s := range candidates {
				if err := probeScope(ctx, hc, view.TokenURL, id, secret, s); err != nil {
					view.Denied = append(view.Denied, scopeDenial{Scope: s, Error: err.Error()})
					continue
				}
				view.Granted = append(view.Granted, s)
			}
			if len(view.Granted) > 0 {
				joined := strings.Join(view.Granted, " ")
				view.CombinedOK = probeScope(ctx, hc, view.TokenURL, id, secret, joined) == nil
				view.Export = fmt.Sprintf("export CONNECTWISE_RMM_OAUTH_SCOPE=%q", joined)
			}
			if len(view.Granted) == 0 && len(view.Denied) > 0 && !strings.Contains(view.Denied[0].Error, "HTTP 4") {
				return apiErr(fmt.Errorf("token request failed: %s", view.Denied[0].Error))
			}
			if err := printScopes(cmd, flags, view); err != nil {
				return err
			}
			if len(view.Granted) == 0 {
				return authErr(fmt.Errorf("no scope could be minted; check the client ID and secret, and the region (CONNECTWISE_RMM_BASE_URL) for %s", view.TokenURL))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scopes, "scopes", "", "Space- or comma-separated scopes to test (default: configured or all scopes this CLI uses)")
	return cmd
}

func printScopes(cmd *cobra.Command, flags *rootFlags, view scopesView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Granted (%d): %s\n", len(view.Granted), strings.Join(view.Granted, " "))
	for _, d := range view.Denied {
		fmt.Fprintf(w, "Denied: %s (%s)\n", d.Scope, d.Error)
	}
	if view.Export != "" {
		fmt.Fprintf(w, "\nCombined set works: %v\n%s\n", view.CombinedOK, view.Export)
	}
	return nil
}

// probeScope mints and discards one token. Errors carry the status code and a
// sanitized body; the token itself is never returned or logged.
func probeScope(ctx context.Context, hc *http.Client, tokenURL, id, secret, scope string) error {
	req, err := client.NewAsioTokenRequest(ctx, tokenURL, id, secret, scope, resolveClientCredentialsUserAgent())
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, client.RedactTokenBody(string(body), secret))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(body, &tok) != nil || tok.AccessToken == "" {
		return fmt.Errorf("HTTP 200 without an access_token")
	}
	return nil
}
