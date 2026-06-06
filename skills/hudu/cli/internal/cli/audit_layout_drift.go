// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: layout-drift detector. Surfaces
// post-migration residue by joining each asset's custom_fields against its
// layout's current schema in the local mirror — assets carrying fields the
// schema no longer defines, or missing newly-added schema fields.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type driftRow struct {
	AssetID       int      `json:"asset_id"`
	AssetName     string   `json:"asset_name"`
	CompanyID     int      `json:"company_id"`
	CompanyName   string   `json:"company_name,omitempty"`
	LayoutID      int      `json:"layout_id"`
	LayoutName    string   `json:"layout_name"`
	ExtraFields   []string `json:"extra_fields,omitempty"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

func newNovelAuditLayoutDriftCmd(flags *rootFlags) *cobra.Command {
	var layoutName string
	var layoutID int
	var flagCompany int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "layout-drift",
		Short:       "Find assets carrying custom fields not in their layout's current schema, or missing newly-added fields.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Detect schema drift from the local mirror (run 'sync' first). For each asset,
the command compares the labels present in its custom_fields against its asset
layout's current field schema and reports:
  - extra fields:   values the asset carries that the schema no longer defines
                    (typical post-migration residue)
  - missing fields: schema fields the asset has no entry for at all

Filter to one layout with --layout-id or --layout-name.`,
		Example: `  # Drift across all layouts
  hudu-cli audit layout-drift

  # One layout by name, as JSON
  hudu-cli audit layout-drift --layout-name "Server" --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}
			companyNames := loadCompanyNames(cmd.Context(), db)

			rawSchemas, err := loadLayoutSchemas(cmd.Context(), db)
			if err != nil {
				return err
			}
			type schemaView struct {
				name   string
				labels map[string]bool // normalized schema labels
			}
			schemas := make(map[int]schemaView, len(rawSchemas))
			for id, s := range rawSchemas {
				schemas[id] = schemaView{name: s.Name, labels: s.labelSet()}
			}
			wantName := strings.ToLower(strings.TrimSpace(layoutName))

			q := `SELECT data FROM assets`
			var qargs []any
			if layoutID > 0 {
				q += ` WHERE asset_layout_id = ?`
				qargs = append(qargs, layoutID)
			}
			assetRows, err := queryDataRows(cmd.Context(), db, q, qargs...)
			if err != nil {
				return fmt.Errorf("reading assets: %w", err)
			}

			out := []driftRow{}
			for _, raw := range assetRows {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				lid := intField(m, "asset_layout_id")
				sc, ok := schemas[lid]
				if !ok || len(sc.labels) == 0 {
					continue
				}
				if wantName != "" && strings.ToLower(sc.name) != wantName {
					continue
				}
				cid := intField(m, "company_id")
				if flagCompany > 0 && cid != flagCompany {
					continue
				}
				cf, _ := json.Marshal(m["custom_fields"])
				present := filledLabels(cf) // keys = labels the asset carries (any value)
				var extra, missing []string
				for label := range present {
					if !sc.labels[label] {
						extra = append(extra, label)
					}
				}
				for label := range sc.labels {
					if _, has := present[label]; !has {
						missing = append(missing, label)
					}
				}
				if len(extra) == 0 && len(missing) == 0 {
					continue
				}
				sort.Strings(extra)
				sort.Strings(missing)
				out = append(out, driftRow{
					AssetID: intField(m, "id"), AssetName: asString(m["name"]),
					CompanyID: cid, CompanyName: companyNames[cid],
					LayoutID: lid, LayoutName: sc.name,
					ExtraFields: extra, MissingFields: missing,
				})
			}
			sort.Slice(out, func(i, j int) bool {
				di := len(out[i].ExtraFields) + len(out[i].MissingFields)
				dj := len(out[j].ExtraFields) + len(out[j].MissingFields)
				return di > dj
			})

			return emitAudit(cmd, flags, out, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintln(w, "No layout drift detected. (Run 'hudu-cli sync' first if unexpected.)")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "ASSET\tLAYOUT\tCOMPANY\tEXTRA\tMISSING")
				for _, r := range out {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						r.AssetName+" #"+strconv.Itoa(r.AssetID), r.LayoutName, r.CompanyName,
						joinOrDash(r.ExtraFields), joinOrDash(r.MissingFields))
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&layoutName, "layout-name", "", "Limit to a layout by name")
	cmd.Flags().IntVar(&layoutID, "layout-id", 0, "Limit to a layout by id")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func joinOrDash(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ", ")
}
