// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type hygieneAccount struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type hygieneSKUGap struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Missing []string `json:"missing"`
}

type hygieneView struct {
	TotalAccounts    int              `json:"total_accounts"`
	UnmappedCount    int              `json:"unmapped_count"`
	UnmappedAccounts []hygieneAccount `json:"unmapped_accounts"`
	SKUCount         int              `json:"sku_count"`
	SKUGaps          []hygieneSKUGap  `json:"sku_gaps"`
	Note             string           `json:"note,omitempty"`
}

func newNovelHygieneUnmappedCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "unmapped",
		Short: "Mapping-hygiene work queue: unmapped accounts plus SKU gaps",
		Long: strings.Trim(`
Use this command for the full mapping-hygiene work queue (unmapped accounts
AND service/SKU gaps) in one rollup. Do NOT use this command to fetch a raw
filtered account list; use 'accounts list'.

Joins two live reads - the integration's accounts (isMapped flag) and the
vendor profile's SKU catalog - into one agent-shaped rollup: every account the
MSP has not mapped yet, plus every SKU missing the description/category
metadata that mapping in the Synthesize UI relies on.
`, "\n"),
		Example: strings.Trim(`
  gradient-cli hygiene unmapped --agent
  gradient-cli hygiene unmapped --limit 50 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("hygiene unmapped takes no positional arguments"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch accounts (unmapped) and the vendor SKU catalog, then roll up mapping gaps")
				return nil
			}
			if flags.dataSource == "local" {
				return fmt.Errorf("hygiene unmapped has no local data source: mapping state lives upstream; it reads the live accounts and vendor endpoints")
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			view := hygieneView{UnmappedAccounts: []hygieneAccount{}, SKUGaps: []hygieneSKUGap{}}

			// All accounts; isMapped on each row is the work-queue signal.
			accData, err := c.Get(cmd.Context(), "/vendor-api/organization/accounts", nil)
			if err != nil {
				return classifyAPIError(fmt.Errorf("fetching accounts: %w", err), flags)
			}
			var accounts []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				MappingID   string `json:"mappingId"`
				IsMapped    *bool  `json:"isMapped"`
			}
			if err := json.Unmarshal(accData, &accounts); err != nil {
				return fmt.Errorf("parsing accounts response: %w", err)
			}
			view.TotalAccounts = len(accounts)
			for _, a := range accounts {
				if !accountIsMapped(a.IsMapped, a.MappingID) {
					view.UnmappedCount++
					if flagLimit <= 0 || len(view.UnmappedAccounts) < flagLimit {
						view.UnmappedAccounts = append(view.UnmappedAccounts, hygieneAccount{ID: a.ID, Name: a.Name, Description: a.Description})
					}
				}
			}

			// Vendor SKU catalog: SKUs missing mapping-relevant metadata.
			venData, err := c.Get(cmd.Context(), "/vendor-api", nil)
			if err != nil {
				return classifyAPIError(fmt.Errorf("fetching vendor profile: %w", err), flags)
			}
			var vendor struct {
				Data struct {
					Skus []struct {
						ID          string `json:"id"`
						Name        string `json:"name"`
						Description string `json:"description"`
						Category    string `json:"category"`
						Subcategory string `json:"subcategory"`
					} `json:"skus"`
				} `json:"data"`
			}
			if err := json.Unmarshal(venData, &vendor); err != nil {
				return fmt.Errorf("parsing vendor response: %w", err)
			}
			view.SKUCount = len(vendor.Data.Skus)
			for _, s := range vendor.Data.Skus {
				missing := []string{}
				if strings.TrimSpace(s.Description) == "" {
					missing = append(missing, "description")
				}
				if strings.TrimSpace(s.Category) == "" {
					missing = append(missing, "category")
				}
				if strings.TrimSpace(s.Subcategory) == "" {
					missing = append(missing, "subcategory")
				}
				if len(missing) > 0 {
					view.SKUGaps = append(view.SKUGaps, hygieneSKUGap{ID: s.ID, Name: s.Name, Missing: missing})
				}
			}

			if view.UnmappedCount == 0 && len(view.SKUGaps) == 0 {
				view.Note = "no mapping gaps: every account is mapped and every SKU carries full metadata"
			} else if flagLimit > 0 && view.UnmappedCount > flagLimit {
				view.Note = fmt.Sprintf("showing %d of %d unmapped accounts; raise --limit to see more", flagLimit, view.UnmappedCount)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 25, "Maximum unmapped accounts to list (0 = all)")
	return cmd
}

// accountIsMapped is the single source of truth for what counts as a mapped
// account: an explicit isMapped=true OR a present mappingId. Used by both
// 'hygiene unmapped' and 'status ready' so the invariant cannot drift.
func accountIsMapped(isMapped *bool, mappingID string) bool {
	return (isMapped != nil && *isMapped) || mappingID != ""
}
