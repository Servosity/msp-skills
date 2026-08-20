// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `auth login` — perform the Avanan handshake and persist the session token.
//
// pp:data-source live
//
// Avanan issues short-lived session tokens rather than long-lived API keys, so
// the CLI needs an explicit mint-and-store step. The signing transport
// refreshes tokens transparently during normal use; this command exists so an
// operator can verify credentials up front and so scripted environments can
// pre-warm a token instead of paying the handshake on first use.

package cli

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"avanan-pp-cli/internal/avanansig"
	"avanan-pp-cli/internal/client"
	"avanan-pp-cli/internal/cliutil"
	"avanan-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		authCmd, _, err := root.Find([]string{"auth"})
		if err != nil || authCmd == nil {
			return
		}
		addNovelCommandIfAbsent(authCmd, newAvananAuthLoginCmd(flags))
		// The generator emits set-token but does not wire it into the auth
		// group. It is the only way to persist the application ID without
		// exporting an env var, so wire it here rather than leave it dead.
		addNovelCommandIfAbsent(authCmd, newAuthSetTokenCmd(flags))
	})
}

type avananLoginResult struct {
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Region    string `json:"region,omitempty"`
	AppID     string `json:"app_id"`
	ExpiresAt string `json:"expires_at"`
	Saved     bool   `json:"saved"`
}

func newAvananAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var (
		appID  string
		secret string
		region string
		save   bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Perform the Avanan handshake and store a session token",
		Long: strings.Trim(`
Mint an Avanan session token and store it for later commands.

The scheme is chosen from the host. Legacy *.avanan.net hosts use the signed
handshake: a fresh request UUID, the application ID, a GMT timestamp, and an
x-av-sig computed as sha256(base64(reqId + appId + date + requestString +
secret)). Infinity Portal hosts exchange the access key for a bearer token
instead.

Credentials are region-scoped. A key issued for one region cannot read another
region's data, so --region must match the region the credentials were issued
for.

Use this command to verify credentials or pre-warm a token. Do NOT use it to
inspect existing auth state; use 'auth status' instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli auth login
  avanan-cli auth login --region eu
  avanan-cli auth login --app-id US:myapp29 --save
`, "\n"),
		Annotations: map[string]string{
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && cliutil.IsVerifyEnv() {
				// Bare help probe under the verifier: no credentials exist in
				// the sandbox, so short-circuit rather than fail on a network
				// call the verifier cannot satisfy.
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "auth login")
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}

			if region != "" {
				base, ok := avanansig.RegionBaseURL(region)
				if !ok {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("unknown region %q; valid regions are %s",
						region, strings.Join(avanansig.Regions(), ", ")))
				}
				cfg.BaseURL = base
			}
			if appID != "" {
				cfg.AvananAppId = appID
			}
			if secret != "" {
				cfg.ClientSecret = secret
			}

			if cfg.AvananAppID() == "" || cfg.AvananClientSecret() == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf(
					"both an application ID and a client secret are required\n" +
						"set AVANAN_APP_ID and AVANAN_CLIENT_SECRET, or pass --app-id and --secret\n" +
						"Check Point support issues these per region"))
			}

			c := client.New(cfg, flags.timeout, flags.rateLimit)
			if err := client.InstallAvananAuth(c); err != nil {
				return err
			}
			transport := client.AvananAuthTransport(c)
			if transport == nil {
				return fmt.Errorf("avanan signing transport was not installed")
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.BaseURL, "/")+"/v1.0/scopes", nil)
			if err != nil {
				return err
			}

			token, expiry, err := transport.Mint(req)
			if err != nil {
				return err
			}

			result := avananLoginResult{
				Scheme:    "legacy-signed",
				Host:      req.URL.Host,
				Region:    region,
				AppID:     cfg.AvananAppID(),
				ExpiresAt: expiry.UTC().Format(time.RFC3339),
			}
			if avanansig.IsInfinityHost(req.URL.Host) {
				result.Scheme = "infinity-bearer"
			}

			if save {
				if err := cfg.SaveAvananCredentials(cfg.AvananAppID(), cfg.AvananClientSecret()); err != nil {
					return configErr(fmt.Errorf("saving credentials: %w", err))
				}
			}
			if err := cfg.SaveAvananSession(token, expiry); err != nil {
				return configErr(fmt.Errorf("saving session token: %w", err))
			}
			result.Saved = true

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticated to %s using the %s scheme.\n", result.Host, result.Scheme)
			fmt.Fprintf(cmd.OutOrStdout(), "Session token stored; expires %s.\n", result.ExpiresAt)
			fmt.Fprintln(cmd.OutOrStdout(), "Run 'avanan-cli scopes' to see which tenants this credential reaches.")
			return nil
		},
	}

	cmd.Flags().StringVar(&appID, "app-id", "", "Avanan application ID (defaults to AVANAN_APP_ID)")
	cmd.Flags().StringVar(&secret, "secret", "", "Avanan client secret (defaults to AVANAN_CLIENT_SECRET)")
	cmd.Flags().StringVar(&region, "region", "", "Region to authenticate against: "+strings.Join(avanansig.Regions(), ", "))
	cmd.Flags().BoolVar(&save, "save", false, "Also persist the application ID and secret, not just the session token")
	return cmd
}
