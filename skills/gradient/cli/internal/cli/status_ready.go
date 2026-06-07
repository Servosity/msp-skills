// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type readyView struct {
	IntegrationID    string   `json:"integration_id,omitempty"`
	VendorName       string   `json:"vendor_name,omitempty"`
	Status           string   `json:"status"`
	LastSyncedAt     string   `json:"last_synced_at,omitempty"`
	TotalAccounts    int      `json:"total_accounts"`
	UnmappedAccounts int      `json:"unmapped_accounts"`
	Ready            bool     `json:"ready"`
	Reasons          []string `json:"reasons"`
}

func newNovelStatusReadyCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Go/no-go: is the integration ready to flip to active?",
		Long: strings.Trim(`
Use this command to decide whether the integration is ready to flip to
active. Do NOT use this command to change the status; use
'integration update-status'.

Joins the live integration record (status, lastSyncedAt) with the live
account mapping state into a single go/no-go verdict: ready means every
account is mapped and the integration is in a state where activating makes
sense (pending, or already active with a recent sync).
`, "\n"),
		Example: strings.Trim(`
  gradient-cli status ready --agent
  gradient-cli status ready --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("status ready takes no positional arguments"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch the integration record and account mapping state, then print a go/no-go verdict")
				return nil
			}
			if flags.dataSource == "local" {
				return fmt.Errorf("status ready has no local data source: readiness is live state; it reads the organization and accounts endpoints")
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			orgData, err := c.Get(cmd.Context(), "/vendor-api/organization", nil)
			if err != nil {
				return classifyAPIError(fmt.Errorf("fetching integration record: %w", err), flags)
			}
			var integ struct {
				IntegrationID string `json:"integrationId"`
				VendorName    string `json:"vendorName"`
				Status        string `json:"status"`
				LastSyncedAt  string `json:"lastSyncedAt"`
				Data          *struct {
					IntegrationID string `json:"integrationId"`
					VendorName    string `json:"vendorName"`
					Status        string `json:"status"`
					LastSyncedAt  string `json:"lastSyncedAt"`
				} `json:"data"`
			}
			if err := json.Unmarshal(orgData, &integ); err != nil {
				return fmt.Errorf("parsing integration response: %w", err)
			}
			if integ.Data != nil && integ.Status == "" {
				integ.IntegrationID = integ.Data.IntegrationID
				integ.VendorName = integ.Data.VendorName
				integ.Status = integ.Data.Status
				integ.LastSyncedAt = integ.Data.LastSyncedAt
			}

			accData, err := c.Get(cmd.Context(), "/vendor-api/organization/accounts", nil)
			if err != nil {
				return classifyAPIError(fmt.Errorf("fetching accounts: %w", err), flags)
			}
			var accounts []struct {
				MappingID string `json:"mappingId"`
				IsMapped  *bool  `json:"isMapped"`
			}
			if err := json.Unmarshal(accData, &accounts); err != nil {
				return fmt.Errorf("parsing accounts response: %w", err)
			}

			view := readyView{
				IntegrationID: integ.IntegrationID,
				VendorName:    integ.VendorName,
				Status:        integ.Status,
				LastSyncedAt:  integ.LastSyncedAt,
				TotalAccounts: len(accounts),
				Reasons:       []string{},
			}
			for _, a := range accounts {
				if !accountIsMapped(a.IsMapped, a.MappingID) {
					view.UnmappedAccounts++
				}
			}

			switch strings.ToLower(view.Status) {
			case "active":
				view.Ready = true
				view.Reasons = append(view.Reasons, "integration is already active")
				if ts, err := time.Parse(time.RFC3339, view.LastSyncedAt); err == nil && time.Since(ts) > 48*time.Hour {
					view.Reasons = append(view.Reasons, fmt.Sprintf("warning: last sync was %s ago - pushes may be overdue", time.Since(ts).Round(time.Hour)))
				}
			case "pending":
				if view.UnmappedAccounts == 0 {
					view.Ready = true
					view.Reasons = append(view.Reasons, "status is pending and every account is mapped - safe to flip to active")
				} else {
					view.Reasons = append(view.Reasons, fmt.Sprintf("%d of %d accounts are still unmapped - map them in Synthesize before activating", view.UnmappedAccounts, view.TotalAccounts))
				}
			default:
				view.Reasons = append(view.Reasons, fmt.Sprintf("integration status is %q - set it to pending (integration update-status pending) to start mapping", view.Status))
			}
			if view.UnmappedAccounts > 0 && strings.ToLower(view.Status) == "active" {
				view.Ready = false
				view.Reasons = append(view.Reasons, fmt.Sprintf("%d unmapped accounts will not reconcile - run 'hygiene unmapped' for the work queue", view.UnmappedAccounts))
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	return cmd
}
