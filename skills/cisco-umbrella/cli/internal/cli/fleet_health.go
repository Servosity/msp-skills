// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/config"
)

// The token endpoint allows 20 requests per minute per key; each customer needs
// one child-org token, so customers start at least this far apart.
var fleetMintSpacing = 3 * time.Second

type fleetCustomer struct {
	ID    string `json:"customer_id"`
	Name  string `json:"customer_name"`
	Seats *int64 `json:"seats"` // null when the console did not report seats
}

type fleetRow struct {
	fleetCustomer
	RoamingTotal       int            `json:"roaming_total"`
	RoamingOff         int            `json:"roaming_off"`
	RoamingUninstalled int            `json:"roaming_uninstalled"`
	RoamingStale       int            `json:"roaming_stale"`
	SWGUnprotected     int            `json:"swg_unprotected"`
	VATotal            int            `json:"va_total"`
	VANotHealthy       int            `json:"va_not_healthy"`
	VAHealth           map[string]int `json:"va_health"`
	TunnelsTotal       int            `json:"tunnels_total"`
	TunnelsDown        int            `json:"tunnels_down"`
	Issues             int            `json:"issues"`
	Skipped            []string       `json:"skipped,omitempty"`
}

type fleetFailure struct {
	CustomerID string `json:"customer_id"`
	Name       string `json:"customer_name"`
	Error      string `json:"error"`
}

type fleetReport struct {
	StaleDays        int            `json:"stale_days"`
	ScannedCustomers int            `json:"scanned_customers"`
	Customers        []fleetRow     `json:"customers"`
	FetchFailures    []fleetFailure `json:"fetch_failures,omitempty"`
	CustomerSource   string         `json:"customer_list_source"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var staleDays, maxCustomers int
	var only string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "One row per managed customer: seats, issues, and off, uninstalled, stale, SWG-unprotected roaming computers",
		Long: `From a Multi-org or MSP parent console key, list the managed customers, mint a
child-org token for each (X-Umbrella-OrgId), and roll up roaming computers, virtual
appliances, and tunnel state into one row per customer. Customers that fail are
listed under fetch_failures and excluded from the rows. A 403 on virtual appliances
or tunnels lists that section under the row's "skipped" field instead of counting 0.
Costs one token mint plus about three deployment calls per customer; customers start
3s apart for the 20/min token limit, so narrow large fleets with --customers or
--max-customers. Needs a parent console key and no --org / CISCO_UMBRELLA_ORG_ID.

Use this command for a one-row-per-customer health rollup across every child org in
an MSP/multi-org console. Do NOT use this command for device-level findings inside
one org; use 'hygiene' instead.`,
		Example: strings.Trim(`
  cisco-umbrella-cli fleet health --stale-days 7 --agent
  cisco-umbrella-cli fleet health --customers 8675309,8675310 --json --select customers.customer_name,customers.roaming_off`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--stale-days=7"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if config.ChildOrgID() != "" {
				return usageErr(fmt.Errorf("fleet health runs from the parent console; drop --org / %s", config.OrgEnv))
			}
			if staleDays < 1 {
				return usageErr(fmt.Errorf("--stale-days must be at least 1"))
			}
			// ponytail: no whole-command --timeout here; this fans out across every list/customer
			// and each request is already bounded by the client's per-request timeout.
			ctx := cmd.Context()
			parent, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireCredentials(parent); err != nil {
				return err
			}
			customers, source, err := listFleetCustomers(ctx, parent)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if only != "" {
				keep := map[string]bool{}
				for _, id := range strings.Split(only, ",") {
					keep[strings.TrimSpace(id)] = true
				}
				filtered := customers[:0]
				for _, cu := range customers {
					if keep[cu.ID] {
						filtered = append(filtered, cu)
					}
				}
				customers = filtered
			}
			if cliutil.IsDogfoodEnv() && len(customers) > 1 {
				customers = customers[:1] // live matrix has a 30s budget per command
			}
			if maxCustomers > 0 && len(customers) > maxCustomers {
				customers = customers[:maxCustomers]
			}

			report := fleetReport{StaleDays: staleDays, Customers: []fleetRow{}, FetchFailures: []fleetFailure{}, CustomerSource: source}
			cutoff := time.Now().Add(-time.Duration(staleDays) * 24 * time.Hour)
			for i, cu := range customers {
				if i > 0 && !cliutil.IsDogfoodEnv() {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(fleetMintSpacing): // ponytail: fixed spacing; switch to a shared limiter if fan-out goes parallel
					}
				}
				report.ScannedCustomers++
				row, err := fleetRowFor(ctx, flags, cu, cutoff)
				if err != nil {
					report.FetchFailures = append(report.FetchFailures, fleetFailure{CustomerID: cu.ID, Name: cu.Name, Error: err.Error()})
					continue
				}
				report.Customers = append(report.Customers, row)
			}
			if n := len(report.FetchFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d customers failed; rows cover the remaining %d\n", n, report.ScannedCustomers, len(report.Customers))
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			rows := make([]map[string]any, 0, len(report.Customers))
			for _, r := range report.Customers {
				seats := "?"
				if r.Seats != nil {
					seats = strconv.FormatInt(*r.Seats, 10)
				}
				rows = append(rows, map[string]any{"customer": r.Name, "id": r.ID, "seats": seats, "roaming": r.RoamingTotal, "off": r.RoamingOff, "uninstalled": r.RoamingUninstalled, "stale": r.RoamingStale, "swg_unprotected": r.SWGUnprotected, "va_not_healthy": r.VANotHealthy, "tunnels_down": r.TunnelsDown, "skipped": len(r.Skipped)})
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No customers reported.")
			} else if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
				return err
			}
			for _, f := range report.FetchFailures {
				fmt.Fprintf(cmd.ErrOrStderr(), "failed: %s (%s): %s\n", f.Name, f.CustomerID, f.Error)
			}
			for _, r := range report.Customers {
				for _, sk := range r.Skipped {
					fmt.Fprintf(cmd.ErrOrStderr(), "not checked: %s (%s): %s\n", r.Name, r.ID, sk)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Count a roaming computer as stale when its last sync is older than this many days")
	cmd.Flags().StringVar(&only, "customers", "", "Comma-separated customer (child org) IDs to include; default every managed customer")
	cmd.Flags().IntVar(&maxCustomers, "max-customers", 0, "Stop after this many customers (0 = all)")
	return cmd
}

// listFleetCustomers reads the MSP console's managed customers, falling back to a
// provider console's customers. It returns which endpoint answered.
func listFleetCustomers(ctx context.Context, c *client.Client) ([]fleetCustomer, string, error) {
	sources := []string{"/admin/v2/managed/customers", "/admin/v2/providers/customers"}
	var lastErr error
	for _, path := range sources {
		out := make([]fleetCustomer, 0)
		err := pageData(ctx, c, path, func(raw json.RawMessage) (int, error) {
			var page []map[string]json.RawMessage
			if err := decodeArray(raw, &page); err != nil {
				return 0, fmt.Errorf("parsing customers from %s: %w", path, err)
			}
			for _, m := range page {
				// IDs are read from their source digits: a float64 round-trip would
				// change large org IDs and mint a token for the wrong customer.
				cu := fleetCustomer{ID: rawID(m["customerId"]), Name: rawID(m["customerName"])}
				if cu.ID == "" {
					cu.ID = rawID(m["organizationId"])
				}
				var seats *int64
				if json.Unmarshal(m["seats"], &seats) == nil && seats != nil {
					cu.Seats = seats
				}
				// A record without a usable id still becomes a row so fleetRowFor
				// reports it under fetch_failures instead of silently dropping it.
				out = append(out, cu)
			}
			return len(page), nil
		})
		if err == nil {
			return out, path, nil
		}
		lastErr = err
		if !isForbidden(err) && !isNotFound(err) {
			return nil, path, err
		}
	}
	return nil, "", fmt.Errorf("no managed or provider customers visible to this key (needs an MSP, Multi-org, or provider console key with the admin customers scope): %w", lastErr)
}

func fleetRowFor(ctx context.Context, flags *rootFlags, cu fleetCustomer, staleCutoff time.Time) (fleetRow, error) {
	row := fleetRow{fleetCustomer: cu, VAHealth: map[string]int{}}
	if !config.ValidOrgID(cu.ID) {
		return row, fmt.Errorf("customer id %q is not a numeric org id", cu.ID)
	}
	c, err := flags.newClient()
	if err != nil {
		return row, err
	}
	client.ScopeToChildOrg(c, cu.ID)

	unreadable := map[string]int{}
	err = pageData(ctx, c, "/deployments/v2/roamingcomputers", func(raw json.RawMessage) (int, error) {
		var page []json.RawMessage
		if err := decodeArray(raw, &page); err != nil {
			return 0, fmt.Errorf("parsing roaming computers: %w", err)
		}
		for _, item := range page {
			ev, ok := evalRoaming(item, staleCutoff)
			if !ok {
				unreadable["roaming computers"]++
				continue
			}
			row.RoamingTotal++
			if ev.Off {
				row.RoamingOff++
			}
			if ev.Uninstalled {
				row.RoamingUninstalled++
			}
			if ev.SWGUnprotected {
				row.SWGUnprotected++
			}
			for _, k := range ev.Unchecked {
				unreadable["roaming computers: "+k+" check"]++
			}
			if ev.Stale {
				row.RoamingStale++
			}
		}
		return len(page), nil
	})
	if err != nil {
		return row, fmt.Errorf("roaming computers: %w", err)
	}

	// The spec declares page/limit on this list while Cisco's pagination guide says it
	// is unpaginated, so page through it and stop when a page adds no new appliance.
	seenVA := map[string]bool{}
	err = pageData(ctx, c, "/deployments/v2/virtualappliances", func(raw json.RawMessage) (int, error) {
		var page []json.RawMessage
		if err := decodeArray(raw, &page); err != nil {
			return 0, fmt.Errorf("parsing virtual appliances: %w", err)
		}
		added := 0
		for _, item := range page {
			ev, ok := evalVA(item)
			if ev.ID != "" && seenVA[ev.ID] {
				continue
			}
			if ev.ID != "" {
				seenVA[ev.ID] = true
			}
			added++
			if !ok {
				unreadable["virtual appliances"]++
				continue
			}
			row.VATotal++
			for _, k := range ev.Unchecked {
				if k == "va-not-healthy" {
					unreadable["virtual appliances: health check"]++
				}
			}
			if ev.Health != "" {
				row.VAHealth[ev.Health]++
			}
			if ev.NotHealthy {
				row.VANotHealthy++
			}
		}
		if added == 0 {
			return 0, nil // a repeated page means the server ignored paging
		}
		return len(page), nil
	})
	switch {
	case isForbidden(err):
		row.Skipped = append(row.Skipped, "virtual appliances: forbidden (key lacks deployments.virtualappliances:read)")
	case err != nil:
		row.Skipped = append(row.Skipped, "virtual appliances: "+err.Error())
	}

	raw, err := c.Get(ctx, "/deployments/v2/tunnelsState", nil)
	switch {
	case isForbidden(err):
		row.Skipped = append(row.Skipped, "tunnel state: forbidden (key lacks deployments.tunnels:read)")
	case err != nil:
		return row, fmt.Errorf("tunnel state: %w", err)
	default:
		var tunnels []json.RawMessage
		if perr := decodeArray(unwrapData(raw), &tunnels); perr != nil {
			row.Skipped = append(row.Skipped, "tunnel state: unparseable response: "+perr.Error())
		}
		for _, item := range tunnels {
			ev, ok := evalTunnel(item)
			if !ok {
				unreadable["tunnels"]++
				continue
			}
			row.TunnelsTotal++
			if len(ev.Unchecked) > 0 {
				unreadable["tunnels: status check"]++
			}
			if ev.Down {
				row.TunnelsDown++
			}
		}
	}
	for _, what := range sortedKeys(unreadable) {
		row.Skipped = append(row.Skipped, fmt.Sprintf("%s: %d record(s) with no usable value in a required field; not checked", what, unreadable[what]))
	}
	row.Issues = row.RoamingOff + row.RoamingUninstalled + row.RoamingStale + row.SWGUnprotected + row.VANotHealthy + row.TunnelsDown
	return row, nil
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	}
	return ""
}
