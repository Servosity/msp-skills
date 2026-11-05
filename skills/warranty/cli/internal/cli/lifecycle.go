// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

type quarter struct {
	Label      string
	Start, End string // [Start, End) as YYYY-MM-DD
}

// quarters returns n calendar quarters starting with the one holding asOf.
func quarters(asOf string, n int) []quarter {
	t, _ := time.Parse("2006-01-02", asOf)
	q0 := (int(t.Month()) - 1) / 3
	start := time.Date(t.Year(), time.Month(q0*3+1), 1, 0, 0, 0, 0, time.UTC)
	out := make([]quarter, 0, n)
	for i := 0; i < n; i++ {
		end := start.AddDate(0, 3, 0)
		out = append(out, quarter{Label: fmt.Sprintf("%d-Q%d", start.Year(), (int(start.Month())-1)/3+1),
			Start: start.Format("2006-01-02"), End: end.Format("2006-01-02")})
		start = end
	}
	return out
}

// lifecycleRows groups devices and counts coverage per group. Ending counts
// cover coverage that ends from asOf on, in each quarter; earlier ends are
// expired. Age uses the ship date, else the purchase date.
func lifecycleRows(devices []store.Device, by, asOf string, qs []quarter, ageYears float64) []map[string]any {
	type acc struct {
		devices, covered, expired, unverified, open, pastAge, noAge int
		ending                                                      []int
	}
	groups := map[string]*acc{}
	ageCut := ""
	if t, err := time.Parse("2006-01-02", asOf); err == nil {
		ageCut = t.AddDate(0, 0, -int(ageYears*365.25)).Format("2006-01-02")
	}
	for _, d := range devices {
		key := d.Customer
		switch by {
		case "model":
			key = d.Model
		case "vendor":
			key = d.Vendor
		}
		if key == "" {
			key = "(none)"
		}
		a := groups[key]
		if a == nil {
			a = &acc{ending: make([]int, len(qs))}
			groups[key] = a
		}
		a.devices++
		st, _ := warranty.Status(warranty.Summary{End: d.CoverageEnd, Open: d.CoverageOpen}, asOf, 0)
		trusted := coverageTrusted(d)
		switch {
		case !trusted:
			a.unverified++
		case d.CoverageOpen:
			a.covered++
			a.open++
		case st == "unknown":
			a.unverified++
		case st == "expired":
			a.expired++
		default:
			a.covered++
		}
		if trusted && !d.CoverageOpen && d.CoverageEnd >= asOf {
			for i, q := range qs {
				if d.CoverageEnd >= q.Start && d.CoverageEnd < q.End {
					a.ending[i]++
				}
			}
		}
		born := d.ShipDate
		if born == "" {
			born = d.PurchaseDate
		}
		switch {
		case born == "":
			a.noAge++
		case ageCut != "" && born <= ageCut:
			a.pastAge++
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
	rows := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		a := groups[k]
		row := map[string]any{"group": k, "devices": a.devices, "covered": a.covered, "open_ended": a.open,
			"expired": a.expired, "unverified": a.unverified, "past_age": a.pastAge, "age_unknown": a.noAge}
		for i, q := range qs {
			row["ends_"+strings.ReplaceAll(q.Label, "-", "_")] = a.ending[i]
		}
		rows = append(rows, row)
	}
	return rows
}

func newNovelLifecycleCmd(flags *rootFlags) *cobra.Command {
	var customer, by, age, asOf string
	var nq int
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Per client: devices covered now, expired, unverified, coverage ending in each of the next quarters",
		Long: `Use this command for per-client counts of coverage ending by quarter and
devices past an age threshold, for refresh budgets and QBR slides. Do NOT use
this command to list the individual devices expiring within N days; use
'report' instead.

One row per customer (or per model or vendor with --by): devices, covered
(open_ended counts AppleCare+ for Business), expired, unverified (no coverage
data yet), ends_<year>_Q<n> for each of the next --quarters calendar quarters
starting with the current one, past_age (shipped, or bought, more than --age
ago) and age_unknown. Reads the local mirror only; --csv gives one line per
row.`,
		Example: strings.Trim(`
  warranty-cli lifecycle --quarters 4 --age 4y --agent
  warranty-cli lifecycle --customer "Acme Dental" --by model --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "lifecycle forecast from the local mirror")
			}
			switch by {
			case "customer", "model", "vendor":
			default:
				return usageErr(fmt.Errorf("--by must be customer, model or vendor"))
			}
			if nq < 1 || nq > 12 {
				return usageErr(fmt.Errorf("--quarters must be between 1 and 12"))
			}
			ageDays, err := parseDays("--age", age)
			if err != nil {
				return err
			}
			day, err := today(asOf)
			if err != nil {
				return err
			}
			customer = normalizeCustomer(customer)
			devices := []store.Device{}
			db, err := openWarrantyRead(cmd.Context())
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
				if devices, err = db.ListDevices(cmd.Context(), customer); err != nil {
					return err
				}
				if err = judgeCoverageOn(cmd.Context(), db, devices, day); err != nil {
					return err
				}
			}
			qs := quarters(day, nq)
			rows := lifecycleRows(devices, by, day, qs, float64(ageDays)/365.25)
			if len(devices) == 0 {
				// "hint:" lines reach MCP callers too (the shell-out forwards them).
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: the warranty mirror has no devices for this filter: run import, then recheck")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			headers := []string{strings.ToUpper(by), "DEVICES", "COVERED", "EXPIRED", "UNVERIFIED"}
			for _, q := range qs {
				headers = append(headers, q.Label)
			}
			headers = append(headers, "PAST AGE")
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				line := []string{fmt.Sprint(r["group"]), fmt.Sprint(r["devices"]), fmt.Sprint(r["covered"]), fmt.Sprint(r["expired"]), fmt.Sprint(r["unverified"])}
				for _, q := range qs {
					line = append(line, fmt.Sprint(r["ends_"+strings.ReplaceAll(q.Label, "-", "_")]))
				}
				table = append(table, append(line, fmt.Sprint(r["past_age"])))
			}
			if len(table) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No devices in the mirror.")
				return nil
			}
			return flags.printTable(cmd, headers, table)
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer's devices")
	cmd.Flags().StringVar(&by, "by", "customer", "Group rows by customer, model or vendor")
	cmd.Flags().IntVar(&nq, "quarters", 4, "Calendar quarters to count coverage endings in (1 to 12)")
	cmd.Flags().StringVar(&age, "age", "4y", "Devices shipped or bought longer ago than this count as past_age (e.g. 4y or 1460d)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Forecast as of this date (YYYY-MM-DD; default today)")
	return cmd
}
