// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: fleet summary.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type fleetRow struct {
	Org               string `json:"org"`
	Store             string `json:"store"`
	Devices           int    `json:"devices"`
	Unassigned        int    `json:"unassigned"`
	AppleCareExpiring int    `json:"applecare_expiring"`
	Uncovered         int    `json:"uncovered"`
	UserLocked        int    `json:"user_locked"`
	StaleServers      int    `json:"stale_servers"`
	NotEnriched       int    `json:"not_enriched"`
	LockNotEnriched   int    `json:"lock_not_enriched"`
	LastSynced        string `json:"last_synced"`
}

func newNovelFleetSummaryCmd(flags *rootFlags) *cobra.Command {
	var within, stale, dbPath string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "One row of counts per customer org: devices, unassigned, AppleCare, user-locked, stale MDM servers",
		Long: `Use this command for one row of counts per customer org (devices, unassigned, AppleCare expiring, uncovered, user-locked, stale MDM servers, sync age). Do NOT use this command to list the devices behind a count; use 'devices unassigned', 'applecare expiring' or 'devices locked' with --all-orgs instead.

Every API account (one per customer org) syncs into its own local store; this
reads all of them read-only and never touches credentials or the network.
AppleCare and Activation Lock counts need 'devices enrich' per org;
not_enriched and lock_not_enriched count devices still missing that data. A server is stale when it
is ACTIVE, still owns devices, and last connected before --stale. A store is
named by the Client ID that 'devices enrich' records; until then, by file name.`,
		Example: strings.Trim(`
  apple-business-cli fleet summary --within 90d --agent
  apple-business-cli fleet summary --stale 14d --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet summary")
			}
			window, err := cliutil.ParseDurationLoose(within)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--within must be a positive duration such as 90d"))
			}
			staleAfter, err := cliutil.ParseDurationLoose(stale)
			if err != nil || staleAfter <= 0 {
				return usageErr(fmt.Errorf("--stale must be a positive duration such as 14d"))
			}
			rows := make([]fleetRow, 0)
			current := storeDBPath(dbPath)
			for _, path := range orgStorePaths(dbPath, true) {
				if _, err := os.Stat(path); err != nil {
					continue
				}
				r, ok, err := fleetRowFrom(cmd, path, path == current, time.Now(), window, staleAfter)
				if err != nil {
					return err
				}
				if ok {
					rows = append(rows, r)
				}
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no local stores yet; run 'apple-business-cli sync' with each customer org's API account")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				return nil
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-44s %7s %10s %9s %9s %7s %6s  %s\n", "ORG", "DEVICES", "UNASSIGNED", "EXPIRING", "UNCOVERED", "LOCKED", "STALE", "SYNCED")
			for _, r := range rows {
				fmt.Fprintf(w, "%-44s %7d %10d %9d %9d %7d %6d  %s\n", r.Org, r.Devices, r.Unassigned, r.AppleCareExpiring, r.Uncovered, r.UserLocked, r.StaleServers, r.LastSynced)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "90d", "AppleCare expiry window (e.g. 90d)")
	cmd.Flags().StringVar(&stale, "stale", "14d", "Flag ACTIVE servers that last connected longer ago than this")
	cmd.Flags().StringVar(&dbPath, "db", "", "Any store in the stores directory (default: the active API account's store)")
	return cmd
}

// fleetRowFrom summarizes one store; ok is false for an empty leftover store
// that is not the active account's.
func fleetRowFrom(cmd *cobra.Command, path string, isCurrent bool, now time.Time, window, staleAfter time.Duration) (fleetRow, bool, error) {
	ctx := cmd.Context()
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return fleetRow{}, false, fmt.Errorf("opening %s: %w", path, err)
	}
	defer db.Close()
	if !isCurrent && !storeHasData(ctx, db) {
		return fleetRow{}, false, nil
	}
	r := fleetRow{Org: orgLabel(ctx, db, path), Store: path, LastSynced: db.GetLastSyncedAt("devices")}
	if r.LastSynced == "" {
		r.LastSynced = "never"
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: store %s has never synced devices; its counts are empty, not healthy. Run 'apple-business-cli sync' with that org's API account.\n", r.Org)
	}
	devices, err := loadLocalDevices(ctx, db)
	if err != nil {
		return r, false, err
	}
	care, err := loadEnrichment(ctx, db, "applecare")
	if err != nil {
		return r, false, err
	}
	locks, _ := loadEnrichment(ctx, db, "lock")
	for _, d := range devices {
		if d.released() {
			continue
		}
		r.Devices++
		if strings.EqualFold(d.Attributes.Status, "UNASSIGNED") {
			r.Unassigned++
		}
		e, ok := care[d.serial()]
		if !ok {
			r.NotEnriched++
		} else {
			v := coverageSummary(e.Data)
			if v.expiringWithin(now, window) {
				r.AppleCareExpiring++
			}
			if v.uncovered(now) {
				r.Uncovered++
			}
		}
		if e, ok := locks[d.serial()]; !ok {
			r.LockNotEnriched++
		} else if l, ok := lockFrom(e); ok && l.IsLocked && strings.EqualFold(l.LockType, "USER") {
			r.UserLocked++
		}
	}
	servers, _ := loadLocalServers(ctx, db)
	for _, s := range servers {
		a := s.Attributes
		last, ok := parseTime(a.LastConnectedDateTime)
		if strings.EqualFold(a.Status, "ACTIVE") && a.DeviceCount != nil && *a.DeviceCount > 0 && ok && now.Sub(last) > staleAfter {
			r.StaleServers++
		}
	}
	return r, true, nil
}
