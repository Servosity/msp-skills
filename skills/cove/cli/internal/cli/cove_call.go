// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newCoveCallCmd is the generic JSON-RPC escape hatch: any of the 251
// documented Management Service methods with automatic visa injection.
func newCoveCallCmd(flags *rootFlags) *cobra.Command {
	var paramsJSON string
	var anonymous bool
	cmd := &cobra.Command{
		Use:   "call <Method>",
		Short: "Call any documented JSON-RPC method with automatic visa injection",
		Long: strings.Trim(`
Invokes a Cove Management Service JSON-RPC method by name. The session visa
is injected automatically (logging in first when credentials are present);
methods and parameter names are case sensitive — see the vendor schema for
the full list of 251 methods.

Use this command for the long tail the typed commands don't cover. Do NOT
use it for fleet triage; 'devices failures', 'devices stale', and 'fleet
health' decode column codes for you.
`, "\n"),
		Example: strings.Trim(`
  cove-cli call GetServerInfo --json
  cove-cli call EnumeratePartners --params '{"parentPartnerId":1234,"fetchRecursively":true,"fields":[0,1,3,5,8,9,10,18,20]}'
  cove-cli call GetPartnerInfoById --params '{"partnerId":1234}'`, "\n"),
		Annotations: map[string]string{
			// The method dispatched is caller-chosen and may mutate; surface
			// as non-read-only so MCP hosts gate it appropriately.
			"mcp:read-only":          "false",
			"pp:happy-args":          "Method=GetServerInfo",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would call the named JSON-RPC method with the session visa injected")
				return nil
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a JSON-RPC method name is required, e.g. `cove-cli call GetServerInfo`"))
			}
			method := args[0]
			var params any
			if paramsJSON != "" {
				if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--params must be valid JSON: %w", err))
				}
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			var result json.RawMessage
			if anonymous {
				result, err = c.CallAnonymous(cmd.Context(), method, params)
			} else {
				result, err = c.Call(cmd.Context(), method, params)
			}
			if err != nil {
				return apiErr(err)
			}
			var view any
			if err := json.Unmarshal(result, &view); err != nil {
				view = string(result)
			}
			return flags.printJSON(cmd, map[string]any{"method": method, "result": view})
		},
	}
	cmd.Flags().StringVar(&paramsJSON, "params", "", "JSON object of method parameters (names are case sensitive)")
	cmd.Flags().BoolVar(&anonymous, "no-auth", false, "Send the call without a visa (probing only)")
	return cmd
}
