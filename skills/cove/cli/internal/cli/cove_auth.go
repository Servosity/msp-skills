// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built auth lifecycle for Cove's Login→visa session model. The visa
// lives inside the JSON-RPC body, so this command group — not the generated
// header-based auth — owns credentials for every hand-built command.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

func newCoveAuthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage the Cove session (Login → visa)",
		Long: strings.Trim(`
Cove's JSON-RPC API authenticates with a partner-scoped login, not a bearer
token. Create an API User in the Cove Management Console (Users > API Users); it
issues a login name and an API token (shown only once). Set COVE_USERNAME to the
API user's login name, COVE_PASSWORD to its API token, and COVE_PARTNER to the
customer the API user was created for (COVE_PARTNER is required for API Users),
then run 'auth login'. The API token is the password, not a visa, and is never
sent as a header. The returned session token (the "visa") is cached locally and
injected into every hand-built command automatically. N-able removed the older
per-user "API access" checkbox; API Users cannot sign in to the console.
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newCoveAuthLoginCmd(flags))
	cmd.AddCommand(newCoveAuthStatusCmd(flags))
	cmd.AddCommand(newCoveAuthTokenCmd(flags))
	cmd.AddCommand(newCoveAuthLogoutCmd(flags))
	return cmd
}

// pp:data-source live
func newCoveAuthLoginCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with COVE_USERNAME/COVE_PASSWORD and cache the session visa",
		Example: strings.Trim(`
  export COVE_PARTNER="Acme Corp" COVE_USERNAME=api-user COVE_PASSWORD=...   # API user creds, once per shell
  cove-cli auth login
  cove-cli auth login --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false", "pp:requires-tier": "credentials"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would call JSON-RPC Login with COVE_USERNAME/COVE_PASSWORD and cache the visa")
				return nil
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			if !c.Creds.Present() {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("COVE_USERNAME (API user login) and COVE_PASSWORD (API token) must be set; set COVE_PARTNER to the customer name when using an API User"))
			}
			result, err := c.Login(cmd.Context())
			if err != nil {
				return authErr(err)
			}
			inner, _ := coverpc.InnerResult(result)
			var info struct {
				ID           int64  `json:"Id"`
				PartnerID    int64  `json:"PartnerId"`
				Name         string `json:"Name"`
				EmailAddress string `json:"EmailAddress"`
				RoleID       int64  `json:"RoleId"`
			}
			_ = json.Unmarshal(inner, &info)
			view := map[string]any{
				"logged_in":  true,
				"user_id":    info.ID,
				"partner_id": info.PartnerID,
				"role_id":    info.RoleID,
				"user":       info.EmailAddress,
				"session":    "visa cached; hand-built commands authenticate automatically",
			}
			return flags.printJSON(cmd, view)
		},
	}
	return cmd
}

// pp:data-source local
func newCoveAuthStatusCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "status",
		Short:       "Show cached session state without printing secrets",
		Example:     "  cove-cli auth status --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would report cached Cove session state")
				return nil
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			age, ok := c.SessionAge()
			view := map[string]any{
				"credentials_in_env": c.Creds.Present(),
				"session_cached":     ok,
			}
			if ok {
				view["session_age_seconds"] = int64(age.Seconds())
				view["session_stale"] = age > 15*time.Minute
			} else {
				view["hint"] = "run `cove-cli auth login`"
			}
			return flags.printJSON(cmd, view)
		},
	}
	return cmd
}

// pp:data-source live
func newCoveAuthTokenCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print a fresh visa for composing raw endpoint commands",
		Long: strings.Trim(`
Prints the current session visa (logging in first when needed) so the raw
generated endpoint commands can authenticate:

  cove-cli devices list --visa "$(cove-cli auth token)" --params-partner-id 1234

The visa is a short-lived session secret — avoid writing it to logs.
`, "\n"),
		Example:     "  cove-cli server info --visa \"$(cove-cli auth token)\"",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:requires-tier": "credentials"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would print a fresh session visa")
				return nil
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			visa, err := c.Visa(cmd.Context())
			if err != nil {
				return authErr(err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), visa)
			return nil
		},
	}
	return cmd
}

// pp:data-source local
func newCoveAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "logout",
		Short:       "Delete the cached session visa",
		Example:     "  cove-cli auth logout",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:requires-tier": "credentials"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would delete the cached session visa")
				return nil
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			if err := c.ClearSession(); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			return flags.printJSON(cmd, map[string]any{"logged_out": true})
		},
	}
	return cmd
}
