// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/store"
)

// propsCoverage is per-customer coverage of a required custom property.
type propsCoverage struct {
	CustomerID     string         `json:"customerId"`
	CustomerName   string         `json:"customerName"`
	Total          int            `json:"total"`
	Set            int            `json:"set"`
	Missing        int            `json:"missing"`
	CoveragePct    float64        `json:"coveragePct"`
	MissingDevices []propsDevice  `json:"missingDevices"`
	Values         map[string]int `json:"values,omitempty"`
}

type propsDevice struct {
	DeviceID string `json:"deviceId"`
	LongName string `json:"longName"`
}

// propsScanDevice is a device row pulled from the local mirror to scan.
type propsScanDevice struct {
	deviceID     string
	longName     string
	customerID   string
	customerName string
}

func newNovelPropsAuditCmd(flags *rootFlags) *cobra.Command {
	var flagRequired string
	var flagCustomer int
	var flagLimit int
	var flagShowValues bool

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Report which devices are missing a required custom-property value, grouped by customer.",
		Long: `Audit custom-property coverage across devices. For each device in the local
mirror, fetch its custom properties live and check whether the --required
property is present with a non-empty value. Results are grouped by customer
with a coverage percentage and a list of the devices still missing the value.

Requires --required and live API access (the property values are fetched
per-device).`,
		Example: `  n-central-cli props audit --required "Backup Plan"
  n-central-cli props audit --required "AssetTag" --customer 100 --show-values --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagRequired == "" {
				return usageErr(fmt.Errorf("--required is required: name the custom property to audit, e.g. --required \"Backup Plan\""))
			}
			if flagLimit <= 0 {
				flagLimit = 200
			}

			if cliutil.IsVerifyEnv() {
				return flags.printJSON(cmd, []propsCoverage{})
			}

			devices, err := propsScanDevices(cmd, flagCustomer, flagLimit)
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no devices in the local store to audit; run 'n-central-cli sync' first")
				return flags.printJSON(cmd, []propsCoverage{})
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			byCust := map[string]*propsCoverage{}
			for _, d := range devices {
				cust := byCust[d.customerID]
				if cust == nil {
					cust = &propsCoverage{
						CustomerID:   d.customerID,
						CustomerName: d.customerName,
						Values:       map[string]int{},
					}
					byCust[d.customerID] = cust
				}
				cust.Total++

				path := "/devices/" + d.deviceID + "/custom-properties"
				raw, gerr := c.Get(cmd.Context(), path, nil)
				if gerr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: custom-properties fetch failed for device %s: %v\n", d.deviceID, classifyAPIError(gerr, flags))
					cust.Missing++
					cust.MissingDevices = append(cust.MissingDevices, propsDevice{DeviceID: d.deviceID, LongName: d.longName})
					continue
				}
				value, found := propsFindValue(raw, flagRequired)
				if found && value != "" {
					cust.Set++
					if flagShowValues {
						cust.Values[value]++
					}
				} else {
					cust.Missing++
					cust.MissingDevices = append(cust.MissingDevices, propsDevice{DeviceID: d.deviceID, LongName: d.longName})
				}
			}

			out := make([]propsCoverage, 0, len(byCust))
			for _, c := range byCust {
				if c.Total > 0 {
					c.CoveragePct = float64(c.Set) / float64(c.Total) * 100
				}
				if !flagShowValues || len(c.Values) == 0 {
					c.Values = nil
				}
				out = append(out, *c)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].CoveragePct != out[j].CoveragePct {
					return out[i].CoveragePct < out[j].CoveragePct
				}
				return out[i].CustomerName < out[j].CustomerName
			})

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, bold("CUSTOMER")+"\t"+bold("SET")+"\t"+bold("MISSING")+"\t"+bold("COVERAGE"))
				for _, c := range out {
					name := c.CustomerName
					if name == "" {
						name = c.CustomerID
					}
					fmt.Fprintf(tw, "%s\t%d\t%d\t%.0f%%\n", name, c.Set, c.Missing, c.CoveragePct)
				}
				return tw.Flush()
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagRequired, "required", "", "Name of the custom property to audit (required)")
	cmd.Flags().IntVar(&flagCustomer, "customer", 0, "Scope to a single customer ID")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum devices to scan")
	cmd.Flags().BoolVar(&flagShowValues, "show-values", false, "Include a distinct-value histogram per customer")
	return cmd
}

// propsScanDevices reads devices to audit from the local mirror.
func propsScanDevices(cmd *cobra.Command, customerID, limit int) ([]propsScanDevice, error) {
	dbPath := defaultDBPath("n-central-cli")
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'n-central-cli sync' first.", err)
	}
	defer db.Close()

	query := `SELECT device_id, long_name, customer_id, customer_name FROM devices WHERE device_id IS NOT NULL`
	var args []any
	if customerID > 0 {
		query += ` AND customer_id = ?`
		args = append(args, customerID)
	}
	query += ` ORDER BY customer_id, device_id LIMIT ?`
	args = append(args, limit)

	rows, err := db.DB().QueryContext(cmd.Context(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading devices: %w", err)
	}
	defer rows.Close()

	var out []propsScanDevice
	for rows.Next() {
		var deviceID sql.NullInt64
		var longName, customerName sql.NullString
		var custID sql.NullInt64
		if err := rows.Scan(&deviceID, &longName, &custID, &customerName); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		if !deviceID.Valid {
			continue
		}
		cid := ""
		if custID.Valid {
			cid = fmt.Sprintf("%d", custID.Int64)
		}
		out = append(out, propsScanDevice{
			deviceID:     fmt.Sprintf("%d", deviceID.Int64),
			longName:     longName.String,
			customerID:   cid,
			customerName: customerName.String,
		})
	}
	return out, rows.Err()
}

// propsFindValue searches a custom-properties response for the named property
// and returns its value. Tolerant of field naming: keys "name"/"propertyName",
// values "value"/"values". A list value is joined; an empty/absent value
// yields ("", true) when the property exists but is unset, or ("", false) when
// the property is absent entirely.
func propsFindValue(raw []byte, propName string) (string, bool) {
	for _, item := range unwrapData(raw) {
		obj := decodeObj(item)
		if obj == nil {
			continue
		}
		name := asString(firstField(obj, "name", "propertyName", "property_name", "label"))
		if name == "" || !equalFoldTrim(name, propName) {
			continue
		}
		// Found the property. Resolve its value.
		if v := firstField(obj, "value"); v != nil {
			if s := asString(v); s != "" {
				return s, true
			}
		}
		if v := firstField(obj, "values"); v != nil {
			if arr, ok := v.([]any); ok {
				parts := make([]string, 0, len(arr))
				for _, e := range arr {
					if s := asString(e); s != "" {
						parts = append(parts, s)
					}
				}
				if len(parts) > 0 {
					return joinComma(parts), true
				}
			} else if s := asString(v); s != "" {
				return s, true
			}
		}
		return "", true // present but empty
	}
	return "", false
}
