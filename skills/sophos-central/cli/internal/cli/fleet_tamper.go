// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
)

// tamperEndpoint is one endpoint with tamper protection off.
type tamperEndpoint struct {
	EndpointID string `json:"endpoint_id"`
	Hostname   string `json:"hostname"`
	Type       string `json:"type"`
	Action     string `json:"action"` // enable | skip
	Status     string `json:"status"` // planned | ok | error | skipped
	Error      string `json:"error,omitempty"`
}

// tamperTenant is one tenant's tamper-protection state and plan.
type tamperTenant struct {
	TenantID      string           `json:"tenant_id"`
	TenantName    string           `json:"tenant_name"`
	Status        string           `json:"status"` // ok | global_off | forbidden | no_api_host | error
	GlobalEnabled *bool            `json:"global_enabled"`
	Endpoints     []tamperEndpoint `json:"endpoints"`
	Error         string           `json:"error,omitempty"`
}

type tamperView struct {
	Mode       string         `json:"mode"` // preview | executed
	Tenants    []tamperTenant `json:"tenants"`
	ToEnable   int            `json:"to_enable"`
	Enabled    int            `json:"enabled"`
	Failed     int            `json:"failed"`
	GlobalOff  int            `json:"tenants_global_off"`
	Unreadable int            `json:"tenants_unreadable"`
	Note       string         `json:"note,omitempty"`
}

func newNovelFleetTamperCmd(flags *rootFlags) *cobra.Command {
	var flagIncludeTrial bool
	var flagMaxEndpoints int

	cmd := &cobra.Command{
		Use:   "tamper",
		Short: "Find every supported endpoint with tamper protection off across customers and, with --yes, turn it back on.",
		Long: `Use this command to find and (with --yes) enable tamper protection on every supported endpoint where it is off, across tenants. Do NOT use this command to read or change one endpoint's tamper protection or its password; use 'endpoints get-tamper-protection' or 'endpoints set-tamper-protection' instead. Do NOT use it only to see which tenants have gaps; use 'fleet gaps' instead.

For each tenant it reads the tenant-wide tamper-protection setting and lists endpoints where tamper protection is supported but off. Endpoint-level tamper protection only takes effect when the tenant-wide setting is on, so tenants with it off are reported (status global_off) and their endpoints skipped; turn it on in Sophos Central first. Without --yes nothing changes. With --yes each endpoint is enabled one call at a time under the rate limiter; passwords are not regenerated and never printed. Reads are live, not from the store.`,
		Example: strings.Trim(`
  sophos-central-cli fleet tamper
  sophos-central-cli fleet tamper --tenant 6f3c4a2e-1b9d-4e8f-a7c2-5d0e9b1f3a64 --yes
  sophos-central-cli fleet tamper --agent --select tenants.tenant_name,tenants.status,to_enable`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":              "false",
			"mcp:read-only-unless-flags": "yes",
			"pp:data-source":             "live",
			"pp:preview-happy-path":      "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet tamper")
			}
			if flags.yes && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "enable tamper protection")
			}
			if flagMaxEndpoints < 0 {
				return usageErr(fmt.Errorf("--max-endpoints must be zero (no cap) or positive"))
			}
			// No boundCtx: these runs span many tenants under the rate limit;
			// --timeout bounds each request inside the generated client.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// The plan must reflect the tenant's current setting: a cached
			// global_off would skip a tenant just switched on in Central.
			c.NoCache = true
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			tenants, _, err := fleetTenantsFiltered(ctx, c, filter, flagIncludeTrial)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if cliutil.IsDogfoodEnv() && len(tenants) > 2 {
				tenants = tenants[:2]
			}
			view := tamperView{Mode: "preview", Tenants: make([]tamperTenant, 0, len(tenants))}
			for _, t := range tenants {
				if ctx.Err() != nil {
					break
				}
				tt := planTamperTenant(ctx, c, t, flagMaxEndpoints)
				if flags.yes && tt.Status == "ok" {
					applyTamper(ctx, c, &tt)
				}
				view.Tenants = append(view.Tenants, tt)
			}
			if flags.yes {
				view.Mode = "executed"
			} else {
				view.Note = "preview only: nothing was changed; rerun with --yes to enable tamper protection on the listed endpoints"
			}
			for _, tt := range view.Tenants {
				switch tt.Status {
				case "global_off":
					view.GlobalOff++
				case "forbidden", "no_api_host", "error":
					view.Unreadable++
				}
				for _, e := range tt.Endpoints {
					switch e.Status {
					case "planned":
						view.ToEnable++
					case "ok":
						view.Enabled++
					case "error":
						view.Failed++
					}
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
			} else {
				rows := make([]map[string]any, 0)
				for _, tt := range view.Tenants {
					rows = append(rows, map[string]any{"tenant": tt.TenantName, "status": tt.Status, "endpoints_off": len(tt.Endpoints), "error": tt.Error})
				}
				if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
					return err
				}
				if view.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("fleet tamper interrupted after %d of %d tenants: %w", len(view.Tenants), len(tenants), err)
			}
			if view.Failed > 0 {
				return apiErr(fmt.Errorf("%d endpoint(s) failed to enable; see tenants[].endpoints[].error", view.Failed))
			}
			// Per-tenant 403s are expected; any tenant that failed for another
			// reason (auth, timeouts, server errors) was not checked or fixed,
			// and that must not look like a clean run.
			if _, unexpected := tamperReadability(view.Tenants); unexpected > 0 {
				return apiErr(fmt.Errorf("%d tenant(s) could not be read (unexpected errors), so they were not checked; see tenants[].error", unexpected))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagIncludeTrial, "include-trial", false, "Include trial tenants (they usually return 403 for partners)")
	cmd.Flags().IntVar(&flagMaxEndpoints, "max-endpoints", 0, "Cap endpoints listed per tenant (0 = no cap)")
	return cmd
}

// tamperReadability counts tenants that were read (ok or global_off) and
// tenants that failed for reasons other than an expected 403 or missing host.
func tamperReadability(tenants []tamperTenant) (readable, unexpected int) {
	for _, tt := range tenants {
		switch tt.Status {
		case "ok", "global_off":
			readable++
		case "forbidden", "no_api_host":
		default:
			unexpected++
		}
	}
	return readable, unexpected
}

// planTamperTenant reads one tenant's global setting and its endpoints with
// tamper protection supported but off.
func planTamperTenant(ctx context.Context, c *client.Client, t fleetTenant, maxEndpoints int) tamperTenant {
	tt := tamperTenant{TenantID: t.ID, TenantName: t.Name, Endpoints: make([]tamperEndpoint, 0)}
	raw, err := c.GetWithHeaders(ctx, "/endpoint/v1/settings/tamper-protection", nil, tenantHeaders(t.ID))
	if err != nil {
		tt.Status, tt.Error = classifyTenantErr(err), truncateErr(err)
		return tt
	}
	var global struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &global); err != nil || global.Enabled == nil {
		tt.Status, tt.Error = "error", "the tenant tamper-protection setting has no readable enabled field"
		return tt
	}
	tt.GlobalEnabled = global.Enabled
	if cliutil.IsDogfoodEnv() && (maxEndpoints == 0 || maxEndpoints > 50) {
		maxEndpoints = 50
	}
	items, err := fetchKeyPaged(ctx, c, "/endpoint/v1/endpoints", map[string]string{
		"pageSize": "500", "tamperProtectionSupported": "true", "tamperProtectionEnabled": "false", "view": "basic",
	}, t.ID, maxEndpoints)
	if err != nil {
		tt.Status, tt.Error = classifyTenantErr(err), truncateErr(err)
		return tt
	}
	tt.Status = "ok"
	action, status := "enable", "planned"
	if !*global.Enabled {
		tt.Status, action, status = "global_off", "skip", "skipped"
	}
	for _, r := range items {
		var e struct {
			ID       string `json:"id"`
			Hostname string `json:"hostname"`
			Type     string `json:"type"`
		}
		if json.Unmarshal(r, &e) != nil || e.ID == "" {
			// An unreadable row would be silently left unprotected.
			tt.Status, tt.Error = "error", "an endpoint row had no readable id; tenant not changed"
			tt.Endpoints = make([]tamperEndpoint, 0)
			return tt
		}
		tt.Endpoints = append(tt.Endpoints, tamperEndpoint{EndpointID: e.ID, Hostname: e.Hostname, Type: e.Type, Action: action, Status: status})
	}
	return tt
}

// applyTamper enables tamper protection endpoint by endpoint. The response
// (which can carry the tamper password) is discarded, never printed.
func applyTamper(ctx context.Context, c *client.Client, tt *tamperTenant) {
	for i := range tt.Endpoints {
		e := &tt.Endpoints[i]
		if e.Action != "enable" {
			continue
		}
		path := "/endpoint/v1/endpoints/" + url.PathEscape(e.EndpointID) + "/tamper-protection"
		if _, _, err := c.PostWithHeaders(ctx, path, map[string]any{"enabled": true, "regeneratePassword": false}, tenantHeaders(tt.TenantID)); err != nil {
			e.Status, e.Error = "error", truncateErr(err)
			continue
		}
		e.Status = "ok"
		if ctx.Err() != nil {
			return
		}
	}
}
