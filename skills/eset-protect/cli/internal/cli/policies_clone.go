// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `policies clone`: verbatim copy of a policy under a new name.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func newNovelPoliciesCloneCmd(flags *rootFlags) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "clone <policy-uuid>",
		Short: "Copy an existing policy's feature settings into a new policy (preview unless --yes)",
		Long: "Fetch a policy and build a create-policy request that copies its features byte for\n" +
			"byte under a new name. Prints the request by default; --yes creates the policy. The\n" +
			"source policy is never modified, and nothing is assigned to devices.\n\n" +
			"Changing settings inside the copy is not supported: re-encoding the endpoint.lzma\n" +
			"blob cannot be verified without a live tenant, and a wrong blob would push a broken\n" +
			"policy. Inspect settings with 'policies decode', then edit the copy in the console.",
		Example: strings.Trim(`
  eset-protect-cli policies clone 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --name "Acme Dental - Workstations"
  eset-protect-cli policies clone 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --name "Acme Dental - Workstations" --yes`, "\n"),
		// Creation needs the root --yes, which MCP never forwards, so the MCP
		// tool can only preview: read-only there by construction.
		Annotations: map[string]string{
			"mcp:read-only":         "true",
			"pp:data-source":        "live",
			"pp:happy-args":         "policy-uuid=example-value;--name=example-copy",
			"pp:preview-happy-path": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "policies clone")
			}
			if len(args) == 0 || strings.TrimSpace(name) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass the source policy UUID and --name for the copy"))
			}
			doc, err := fetchPolicyDoc(cmd, flags, args[0], "")
			if err != nil {
				return err
			}
			src, _ := doc["policy"].(map[string]any)
			if src == nil {
				return apiErr(fmt.Errorf("policy %s response has no policy object", args[0]))
			}
			features, _ := src["features"].([]any)
			if len(features) == 0 {
				return apiErr(fmt.Errorf("policy %s has no features to copy", args[0]))
			}
			srcName, _ := src["displayName"].(string)
			desc := description
			if desc == "" {
				desc = fmt.Sprintf("Copy of %q (%s)", srcName, args[0])
			}
			body := map[string]any{"policy": map[string]any{
				"displayName": strings.TrimSpace(name),
				"description": desc,
				"features":    features,
			}}
			if !flags.yes {
				sendURL, err := esetPreviewURL(flags, esetHost("policy-management", "/v2/policies"))
				if err != nil {
					return err
				}
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"preview":       true,
					"source_policy": map[string]any{"uuid": args[0], "display_name": srcName, "features": len(features)},
					"request":       map[string]any{"method": "POST", "url": sendURL, "body": omitPolicyDataBlobs(body)},
					"note":          "nothing was created; rerun with --yes to create this policy",
				}, flags)
			}
			if cliutil.IsAnyHarness() {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"refused": true,
					"reason":  "policy creation is disabled under the verify/dogfood harness",
				}, flags)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			resp, _, err := c.Post(ctx, esetHost("policy-management", "/v2/policies"), body)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			// The policy exists now: never fail here (a retry would create a
			// second copy). Pass the response through verbatim so large ids
			// are not rounded; fall back to text for a non-JSON body.
			var created any = string(resp)
			if json.Valid(resp) {
				created = json.RawMessage(resp)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"created": created, "source_policy": args[0]}, flags)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Display name for the new policy (required)")
	cmd.Flags().StringVar(&description, "description", "", "Description for the new policy (default: 'Copy of <source>')")
	return cmd
}
