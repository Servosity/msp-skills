// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: applecare expiring.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type expiringRow struct {
	Org             string `json:"org,omitempty"`
	Serial          string `json:"serial"`
	Model           string `json:"model"`
	Family          string `json:"family"`
	OrderNumber     string `json:"order_number,omitempty"`
	ServerName      string `json:"server_name,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	EndDate         string `json:"end_date,omitempty"`
	DaysLeft        *int   `json:"days_left,omitempty"`
	AgreementNumber string `json:"agreement_number,omitempty"`
	Renewable       bool   `json:"renewable"`
	Uncovered       bool   `json:"uncovered"`
}

type expiringReport struct {
	Within      string        `json:"within"`
	Devices     []expiringRow `json:"devices"`
	Groups      []countByKey  `json:"groups,omitempty"`
	NotEnriched int           `json:"not_enriched"`
	Note        string        `json:"note,omitempty"`
}

func newNovelApplecareExpiringCmd(flags *rootFlags) *cobra.Command {
	var within, groupBy, dbPath string
	var uncovered, allOrgs bool
	cmd := &cobra.Command{
		Use:   "expiring",
		Short: "Devices whose AppleCare ends within a window, or that have no active coverage, grouped for renewals",
		Long: `Use this command to list devices whose AppleCare ends within a window, or that have no active coverage, across one or all customer orgs. Do NOT use this command for one device's coverage; use 'devices lookup' instead. Do NOT use it for per-customer counts; use 'fleet summary' instead.

Reads coverage stored by 'devices enrich --applecare'. A device counts as
expiring when its latest active, dated coverage ends inside --within and no
open-ended coverage (AppleCare+ for Business subscriptions) keeps it covered.
--uncovered lists devices with no active coverage instead.`,
		Example: strings.Trim(`
  apple-business-cli applecare expiring --within 90d --group-by order --agent
  apple-business-cli applecare expiring --uncovered --all-orgs --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "applecare expiring")
			}
			window, err := cliutil.ParseDurationLoose(within)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--within must be a positive duration such as 90d"))
			}
			switch groupBy {
			case "", "order", "family", "server", "org":
			default:
				return usageErr(fmt.Errorf("--group-by must be order, family, server or org"))
			}
			rep := expiringReport{Within: within, Devices: make([]expiringRow, 0)}
			paths := orgStorePaths(dbPath, allOrgs)
			multi := len(paths) > 1 || allOrgs || groupBy == "org" // org grouping needs each row's org label
			now := time.Now()
			for _, path := range paths {
				if _, err := os.Stat(path); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: apple-business-cli sync --resources devices && apple-business-cli devices enrich --applecare\n", path)
					continue
				}
				rows, missing, err := expiringRowsFrom(cmd, flags, path, multi, now, window, uncovered)
				if err != nil {
					return err
				}
				rep.Devices = append(rep.Devices, rows...)
				rep.NotEnriched += missing
			}
			sort.SliceStable(rep.Devices, func(i, j int) bool { return rep.Devices[i].EndDate < rep.Devices[j].EndDate })
			if groupBy != "" {
				counts := map[string]int{}
				for _, r := range rep.Devices {
					k := map[string]string{"order": r.OrderNumber, "family": r.Family, "server": r.ServerName, "org": r.Org}[groupBy]
					counts[firstNonEmpty(k, "(none)")]++
				}
				for k, n := range counts {
					rep.Groups = append(rep.Groups, countByKey{k, n})
				}
				sortCounts(rep.Groups)
			}
			if rep.NotEnriched > 0 {
				rep.Note = fmt.Sprintf("%d device(s) have no AppleCare data yet; run 'apple-business-cli devices enrich --applecare'", rep.NotEnriched)
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+rep.Note)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			w := cmd.OutOrStdout()
			if len(rep.Devices) == 0 {
				fmt.Fprintln(w, "No matching devices.")
				return nil
			}
			for _, g := range rep.Groups {
				fmt.Fprintf(w, "%-30s %d\n", g.Key, g.Count)
			}
			for _, r := range rep.Devices {
				days := "-"
				if r.DaysLeft != nil {
					days = fmt.Sprintf("%dd", *r.DaysLeft)
				}
				fmt.Fprintf(w, "%s%s  %s  %s  %s ends %s (%s)  order %s\n", prefixOrg(r.Org), r.Serial, r.Family, r.Model, firstNonEmpty(r.Coverage, "no active coverage"), firstNonEmpty(r.EndDate, "-"), days, firstNonEmpty(r.OrderNumber, "-"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "90d", "Window from now to look for coverage end dates (e.g. 30d, 90d, 12w)")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Also count devices by order, family, server or org")
	cmd.Flags().BoolVar(&uncovered, "uncovered", false, "List devices with no active AppleCare coverage instead")
	cmd.Flags().BoolVar(&allOrgs, "all-orgs", false, "Read every customer org's local store, not just the active API account's")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	return cmd
}

func expiringRowsFrom(cmd *cobra.Command, flags *rootFlags, path string, multi bool, now time.Time, window time.Duration, uncovered bool) ([]expiringRow, int, error) {
	ctx := cmd.Context()
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("opening %s: %w", path, err)
	}
	defer db.Close()
	if !multi && !hintIfUnsynced(cmd, db, "devices") {
		hintIfStale(cmd, db, "devices", flags.maxAge)
	}
	devices, err := loadLocalDevices(ctx, db)
	if err != nil {
		return nil, 0, err
	}
	care, err := loadEnrichment(ctx, db, "applecare")
	if err != nil {
		return nil, 0, err
	}
	links, _ := loadEnrichment(ctx, db, "server")
	servers, _ := loadLocalServers(ctx, db)
	names := map[string]string{}
	for _, s := range servers {
		names[s.ID] = s.Attributes.ServerName
	}
	label := ""
	if multi {
		if path != storeDBPath(flagDB(cmd)) && !storeHasData(ctx, db) {
			return nil, 0, nil
		}
		label = orgLabel(ctx, db, path)
		hintStoreUnsynced(cmd, db, label)
	}
	rows := make([]expiringRow, 0)
	missing := 0
	for _, d := range devices {
		if d.released() {
			continue
		}
		e, ok := care[d.serial()]
		if !ok {
			missing++
			continue
		}
		v := coverageSummary(e.Data)
		if (uncovered && !v.uncovered(now)) || (!uncovered && !v.expiringWithin(now, window)) {
			continue
		}
		r := expiringRow{Org: label, Serial: d.serial(), Model: d.Attributes.DeviceModel, Family: d.Attributes.ProductFamily,
			OrderNumber: d.Attributes.OrderNumber, Coverage: v.Description, AgreementNumber: v.AgreementNumber, Renewable: v.Renewable, Uncovered: v.uncovered(now)}
		if v.End != nil {
			r.EndDate = v.End.Format("2006-01-02")
			days := int(v.End.Sub(now).Hours() / 24)
			r.DaysLeft = &days
		}
		if link, ok := links[d.serial()]; ok {
			r.ServerName = names[serverLinkID(link)]
		}
		rows = append(rows, r)
	}
	return rows, missing, nil
}
