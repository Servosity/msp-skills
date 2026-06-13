// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// pp:data-source local
func newNovelAssetsPatchGapsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var flagSeverity string

	cmd := &cobra.Command{
		Use:   "patch-gaps",
		Short: "Rank assets missing critical patches across every customer.",
		Long: `Scan the patches table for patches whose status indicates they are not yet
installed (missing, pending, available, needed, not installed), join each patch
to its asset and the asset's customer, and rank assets by missing-patch count.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("syncro-cli")
			}
			db, err := syncroOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			assetName, assetCustomer := loadAssetInfo(db)
			custNames := loadCustomerNames(db)

			type agg struct {
				assetID      string
				assetName    string
				customerName string
				missing      int
				sampleKBs    []string
			}
			byAsset := map[string]*agg{}
			totalMissing := 0

			rows, err := db.Query(`SELECT customer_assets_id, data FROM patches`)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var assetID *string
					var data []byte
					if err := rows.Scan(&assetID, &data); err != nil {
						continue
					}
					var obj map[string]any
					if json.Unmarshal(data, &obj) != nil {
						continue
					}
					if !patchIsMissing(obj) {
						continue
					}
					if flagSeverity != "" {
						sev := strings.ToLower(novelJSONString(obj, "severity"))
						if !strings.Contains(sev, strings.ToLower(flagSeverity)) {
							continue
						}
					}
					aid := ""
					if assetID != nil {
						aid = *assetID
					}
					a := byAsset[aid]
					if a == nil {
						custID := assetCustomer[aid]
						a = &agg{
							assetID:      aid,
							assetName:    assetName[aid],
							customerName: custNames[custID],
						}
						byAsset[aid] = a
					}
					a.missing++
					totalMissing++
					kb := novelJSONString(obj, "kb", "kb_number", "title", "name")
					if kb != "" && len(a.sampleKBs) < 5 {
						a.sampleKBs = append(a.sampleKBs, kb)
					}
				}
			}

			type itemOut struct {
				AssetID        string   `json:"asset_id"`
				AssetName      string   `json:"asset_name"`
				CustomerName   string   `json:"customer_name"`
				MissingPatches int      `json:"missing_patches"`
				SampleKBs      []string `json:"sample_kbs"`
			}
			items := make([]itemOut, 0, len(byAsset))
			for _, a := range byAsset {
				kbs := a.sampleKBs
				if kbs == nil {
					kbs = []string{}
				}
				items = append(items, itemOut{
					AssetID:        a.assetID,
					AssetName:      a.assetName,
					CustomerName:   a.customerName,
					MissingPatches: a.missing,
					SampleKBs:      kbs,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].MissingPatches > items[j].MissingPatches
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"items":         items,
					"total_assets":  len(items),
					"total_missing": totalMissing,
				})
			}

			if len(items) == 0 {
				novelSyncHint(cmd.ErrOrStderr(), "No patch gaps found. If patches are not synced, run 'syncro-cli sync' first.")
				fmt.Fprintln(cmd.OutOrStdout(), "No assets with missing patches.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-26s %-24s %s\n", "ASSET_ID", "ASSET", "CUSTOMER", "MISSING")
			for _, it := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-26s %-24s %d\n", it.AssetID, truncate(it.AssetName, 26), truncate(it.CustomerName, 24), it.MissingPatches)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d asset(s), %d missing patch(es) total.\n", len(items), totalMissing)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum assets to show")
	cmd.Flags().StringVar(&flagSeverity, "severity", "", "Only patches whose severity contains this value (case-insensitive)")
	return cmd
}

// patchMissingStatuses lists status substrings that indicate a patch is not
// yet installed.
var patchMissingStatuses = []string{"missing", "pending", "available", "needed", "not installed", "notinstalled"}

// patchIsMissing reports whether a patch's JSON status indicates it still
// needs to be applied.
func patchIsMissing(obj map[string]any) bool {
	status := strings.ToLower(strings.TrimSpace(novelJSONString(obj, "status", "state", "install_status")))
	if status == "" {
		return false
	}
	for _, s := range patchMissingStatuses {
		if strings.Contains(status, s) {
			return true
		}
	}
	return false
}

// loadAssetInfo returns two maps keyed by asset id: display name and customer
// id, read from the customer_assets table. Empty on an unsynced store.
func loadAssetInfo(db *store.Store) (names map[string]string, customers map[string]string) {
	names = map[string]string{}
	customers = map[string]string{}
	rows, err := db.Query(`SELECT id, data FROM customer_assets`)
	if err != nil {
		return names, customers
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(data, &obj) != nil {
			continue
		}
		names[id] = novelJSONString(obj, "name", "asset_name", "hostname")
		customers[id] = novelJSONString(obj, "customer_id", "customerId")
	}
	return names, customers
}
