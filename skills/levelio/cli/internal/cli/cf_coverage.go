// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type cfCoverageField struct {
	FieldName        string   `json:"field_name"`
	Reference        string   `json:"reference,omitempty"`
	CustomFieldID    string   `json:"custom_field_id"`
	AdminOnly        bool     `json:"admin_only"`
	TotalDevices     int      `json:"total_devices"`
	DevicesWithValue int      `json:"devices_with_value"`
	CoveragePct      float64  `json:"coverage_pct"`
	OtherAssignees   int      `json:"other_assignees"`
	Missing          []string `json:"missing,omitempty"`
	Present          []string `json:"present,omitempty"`
}

type cfCoverageResult struct {
	FieldFilter  string            `json:"field_filter,omitempty"`
	TotalDevices int               `json:"total_devices"`
	Fields       []cfCoverageField `json:"fields"`
}

// lvlComputeCfCoverage anti-joins custom fields against custom-field values and
// devices to report device coverage (and the missing-device list) per field.
func lvlComputeCfCoverage(fields []lvlCustomField, values []lvlCustomFieldValue, devices []lvlDevice, fieldFilter string, includeMissing, includePresent bool) cfCoverageResult {
	res := cfCoverageResult{FieldFilter: fieldFilter, TotalDevices: len(devices)}
	ff := strings.ToLower(strings.TrimSpace(fieldFilter))

	deviceIDs := make(map[string]bool, len(devices))
	deviceLabel := make(map[string]string, len(devices))
	for _, d := range devices {
		deviceIDs[d.ID] = true
		deviceLabel[d.ID] = lvlDeviceLabel(d)
	}

	// assignees-with-value per custom_field_id.
	assignees := map[string]map[string]bool{}
	for _, v := range values {
		if strings.TrimSpace(v.Value) == "" {
			continue
		}
		m := assignees[v.CustomFieldID]
		if m == nil {
			m = map[string]bool{}
			assignees[v.CustomFieldID] = m
		}
		m[v.AssignedToID] = true
	}

	for _, f := range fields {
		if ff != "" && !strings.Contains(strings.ToLower(f.Reference), ff) && !strings.Contains(strings.ToLower(f.Name), ff) {
			continue
		}
		assigned := assignees[f.ID]
		withValue := 0
		other := 0
		for id := range assigned {
			if deviceIDs[id] {
				withValue++
			} else {
				other++
			}
		}
		cov := 0.0
		if len(devices) > 0 {
			cov = round1(float64(withValue) / float64(len(devices)) * 100.0)
		}
		fc := cfCoverageField{
			FieldName: f.Name, Reference: f.Reference, CustomFieldID: f.ID, AdminOnly: f.AdminOnly,
			TotalDevices: len(devices), DevicesWithValue: withValue, CoveragePct: cov, OtherAssignees: other,
		}
		if includeMissing {
			var missing []string
			for _, d := range devices {
				if assigned == nil || !assigned[d.ID] {
					missing = append(missing, deviceLabel[d.ID])
				}
			}
			sort.Strings(missing)
			fc.Missing = missing
		}
		if includePresent {
			var present []string
			for _, d := range devices {
				if assigned != nil && assigned[d.ID] {
					present = append(present, deviceLabel[d.ID])
				}
			}
			sort.Strings(present)
			fc.Present = present
		}
		res.Fields = append(res.Fields, fc)
	}

	sort.SliceStable(res.Fields, func(i, j int) bool {
		if res.Fields[i].CoveragePct != res.Fields[j].CoveragePct {
			return res.Fields[i].CoveragePct < res.Fields[j].CoveragePct
		}
		return res.Fields[i].FieldName < res.Fields[j].FieldName
	})
	return res
}

// pp:data-source local
func newNovelCfCoverageCmd(flags *rootFlags) *cobra.Command {
	var field string
	var missing bool
	var present bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "cf-coverage",
		Short:       "Audit which devices are missing a custom-field value (anti-join)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Audit custom-field coverage across devices: for each field, how many
devices carry a value, the coverage percentage, and — with --missing — the exact
devices that have no value (or, with --present, the devices that have one).
Computed offline by anti-joining custom fields, custom-field values, and devices.
Group/org-level values are reported separately as other_assignees. Narrow to one
field with --field (matches reference or name).

Use this command to audit which devices/groups/org are MISSING (or have) a
CUSTOM-FIELD value. Do NOT use it for tag-data hygiene; use 'tag-audit' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Coverage for every custom field, worst first
  levelio-cli cf-coverage

  # List the devices missing any field value, JSON for agents
  levelio-cli cf-coverage --missing --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "custom-fields") {
				hintIfStale(cmd, db, "custom-fields", flags.maxAge)
			}

			fields, err := lvlCustomFields(db)
			if err != nil {
				return fmt.Errorf("loading custom fields: %w", err)
			}
			values, err := lvlCustomFieldValues(db)
			if err != nil {
				return fmt.Errorf("loading custom field values: %w", err)
			}
			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			res := lvlComputeCfCoverage(fields, values, devices, field, missing, present)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d custom field(s) across %d device(s)\n", len(res.Fields), res.TotalDevices)
			if len(res.Fields) == 0 {
				return nil
			}
			fmt.Fprintln(out, "COVERAGE\tWITH_VALUE\tTOTAL\tFIELD")
			for _, f := range res.Fields {
				fmt.Fprintf(out, "%.1f%%\t%d\t%d\t%s\n", f.CoveragePct, f.DevicesWithValue, f.TotalDevices, f.FieldName)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&field, "field", "", "Limit to one custom field (matches reference or name)")
	cmd.Flags().BoolVar(&missing, "missing", false, "Include the list of devices missing a value for each field")
	cmd.Flags().BoolVar(&present, "present", false, "Include the list of devices that have a value for each field")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
