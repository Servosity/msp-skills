// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// fanoutResult is one org's answer with the provenance the official Meraki
// MCP lacks: when it was observed, how many items came back, and whether
// every page was read (and if not, the cursor that resumes it).
type fanoutResult struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	SourceOperation  string `json:"source_operation"`
	ObservedAt       string `json:"observed_at"`
	pageResult
	Error string            `json:"error,omitempty"`
	Data  []json.RawMessage `json:"data"`
}

// normalizeFanoutPath accepts "organizations/{organizationId}/x" with or
// without a leading slash or /api/v1 prefix, and requires the org placeholder
// so the command can only read org-scoped collections.
func normalizeFanoutPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "api/v1/")
	if !strings.HasPrefix(p, "organizations/{organizationId}") {
		return "", fmt.Errorf("path must start with organizations/{organizationId}, got %q", raw)
	}
	if strings.ContainsAny(p, "?#") {
		return "", fmt.Errorf("pass query parameters with --param key=value, not in the path")
	}
	rest := strings.TrimPrefix(p, "organizations/{organizationId}")
	if strings.Contains(rest, "{") {
		return "", fmt.Errorf("path %q has a placeholder besides {organizationId}; fan-out can only fill the organization", raw)
	}
	return "/" + p, nil
}

func parseKVParams(values []string) (url.Values, error) {
	out := url.Values{}
	for _, kv := range values {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("--param %q must be key=value", kv)
		}
		out.Add(strings.TrimSpace(k), v)
	}
	return out, nil
}

func newNovelFleetFanoutCmd(flags *rootFlags) *cobra.Command {
	var orgFilter, paramFlags []string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "fanout [organizations/{organizationId}/...]",
		Short: "Run any organization-scoped GET against every org, fully paginated, with observed-at, item count and completeness per org",
		Long: strings.TrimSpace(`
Runs one org-scoped GET (any path under organizations/{organizationId}) against
every organization the API key can see, under the shared rate limiter. Every
Link-header page is followed unless --max-pages stops it, and each org's result
carries observed_at, pages_fetched, items_returned, complete and, when cut
short, next_cursor and limit_reason. An org that errors keeps its row with the
error instead of disappearing. Use --json for these fields: --agent compacts
away next_cursor, limit_reason and error. Array query parameters take Meraki's bracket
form, e.g. --param 'productTypes[]=wireless'. Read-only: only GET is sent.

Use this command to run one org-scoped GET across every organization the key can see, with completeness metadata per org.
Do NOT use this command for writes; use 'fleet push' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet fanout organizations/{organizationId}/firmware/upgrades --json
  cisco-meraki-cli fleet fanout organizations/{organizationId}/appliance/security/events --param timespan=86400 --json
  cisco-meraki-cli fleet fanout organizations/{organizationId}/devices --param 'productTypes[]=wireless' --max-pages 2 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "path=organizations/{organizationId}/networks"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet fanout")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("one org-scoped path is required, e.g. organizations/{organizationId}/networks"))
			}
			path, err := normalizeFanoutPath(args[0])
			if err != nil {
				return usageErr(err)
			}
			params, err := parseKVParams(paramFlags)
			if err != nil {
				return usageErr(err)
			}
			if maxPages < 0 {
				return usageErr(fmt.Errorf("--max-pages must be 0 (unlimited) or positive"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			op := "GET " + path
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) (fanoutResult, error) {
				r := fanoutResult{OrganizationID: o.ID, OrganizationName: o.Name, SourceOperation: op, Data: make([]json.RawMessage, 0)}
				if !o.APIEnabled {
					r.Error = errOrgAPIDisabled.Error()
					r.ObservedAt = time.Now().UTC().Format(time.RFC3339)
					return r, nil
				}
				res, err := merakiGetAllValues(ctx, c, orgPath(path, o.ID), params, maxPages)
				r.ObservedAt = time.Now().UTC().Format(time.RFC3339)
				r.pageResult, r.Data = res, res.Items
				if err != nil {
					r.Error = err.Error()
					r.Complete = false
				}
				return r, nil
			})
			results := make([]fanoutResult, 0, len(outcomes))
			failed, partial := 0, 0
			for _, oc := range outcomes {
				r := oc.Value
				if oc.Err != nil {
					r = fanoutResult{OrganizationID: oc.Org.ID, OrganizationName: oc.Org.Name, SourceOperation: op, Error: oc.Err.Error(), Data: make([]json.RawMessage, 0), ObservedAt: time.Now().UTC().Format(time.RFC3339)}
				}
				if r.Error != "" {
					failed++
				} else if !r.Complete {
					partial++
				}
				results = append(results, r)
			}
			if failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d organizations returned an error; see each row's error field\n", failed, len(results))
				for _, r := range results {
					if r.Error != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s (%s): %s\n", r.OrganizationName, r.OrganizationID, r.Error)
					}
				}
			}
			if partial > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d organizations are incomplete (limit_reason set); rerun with a higher --max-pages\n", partial)
			}
			return printFleetRows(cmd, flags, results, []string{"organization_name", "items_returned", "pages_fetched", "complete", "error"}, "No organizations visible to this API key.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringArrayVar(&paramFlags, "param", nil, "Query parameter key=value (repeatable); arrays use key[]=value")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "Stop each org after N pages and report next_cursor (0 = read every page)")
	return cmd
}
