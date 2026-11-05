// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: mdm-servers health.
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

type serverHealthRow struct {
	Org             string   `json:"org,omitempty"`
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type,omitempty"`
	Status          string   `json:"status"`
	Devices         *int     `json:"devices"`
	LastConnected   string   `json:"last_connected,omitempty"`
	DaysSince       *int     `json:"days_since_connected"`
	DefaultFamilies []string `json:"default_families"`
	Issues          []string `json:"issues"`
}

// Issue names. stale matches fleet summary's stale_servers count, so this
// command lists the servers behind that number.
const (
	issueStale        = "stale"
	issueDeadOwnsDevs = "not_active_owns_devices"
)

func newNovelMdmServersHealthCmd(flags *rootFlags) *cobra.Command {
	var stale, dbPath string
	var allOrgs, issuesOnly bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Each MDM server's status, device count and last check-in, flagging stale servers and inactive ones that still own devices",
		Long: `Use this command to see every device management service (MDM server) with its status, device count, days since it last connected to Apple and the product families it receives by default, with stale and inactive-but-owns-devices servers flagged. Do NOT use this command for the devices waiting on no server; use 'devices unassigned' instead. Do NOT use it for per-customer counts only; use 'fleet summary' instead.

Reads the local store: run 'sync --resources mdm_servers' first. A server is
stale when it is ACTIVE, still owns devices (deviceCount), and last connected
before --stale: the same rule as fleet summary's stale_servers, so this lists
the servers behind that count. not_active_owns_devices flags an INACTIVE or
DELETED server whose deviceCount is still above zero: those devices are
assigned to a server that no longer checks in. Rows with issues sort first,
then the longest silence. --all-orgs reads every customer org's store.`,
		Example: strings.Trim(`
  apple-business-cli mdm-servers health --agent
  apple-business-cli mdm-servers health --all-orgs --issues-only --stale 7d --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mdm-servers health")
			}
			staleAfter, err := cliutil.ParseDurationLoose(stale)
			if err != nil || staleAfter <= 0 {
				return usageErr(fmt.Errorf("--stale must be a positive duration such as 14d"))
			}
			rows, err := serverHealthRows(cmd, flags, orgStorePaths(dbPath, allOrgs), allOrgs, storeDBPath(dbPath), time.Now(), staleAfter)
			if err != nil {
				return err
			}
			if issuesOnly {
				kept := rows[:0]
				for _, r := range rows {
					if len(r.Issues) > 0 {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No MDM servers to show from the local store.")
				return nil
			}
			for _, r := range rows {
				days, devs := "-", "-"
				if r.DaysSince != nil {
					days = fmt.Sprintf("%dd", *r.DaysSince)
				}
				if r.Devices != nil {
					devs = fmt.Sprint(*r.Devices)
				}
				issues := ""
				if len(r.Issues) > 0 {
					issues = "  [" + strings.Join(r.Issues, ", ") + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s devices  last seen %s  defaults %s%s\n", prefixOrg(r.Org), r.Name, r.Status, devs, days, firstNonEmpty(strings.Join(r.DefaultFamilies, ","), "-"), issues)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&stale, "stale", "14d", "Flag ACTIVE servers that own devices and last connected longer ago than this")
	cmd.Flags().BoolVar(&issuesOnly, "issues-only", false, "Only servers with at least one issue")
	cmd.Flags().BoolVar(&allOrgs, "all-orgs", false, "Read every customer org's local store, not just the active API account's")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the active API account's store)")
	return cmd
}

func serverHealthRows(cmd *cobra.Command, flags *rootFlags, paths []string, labelOrgs bool, current string, now time.Time, staleAfter time.Duration) ([]serverHealthRow, error) {
	multi := len(paths) > 1 || labelOrgs
	rows := make([]serverHealthRow, 0)
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: apple-business-cli sync --resources mdm_servers\n", path)
			continue
		}
		db, err := store.OpenReadOnlyContext(cmd.Context(), path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		label := ""
		skip := false
		if multi {
			skip = path != current && !storeHasData(cmd.Context(), db)
			label = orgLabel(cmd.Context(), db, path)
		}
		if !skip && db.GetLastSyncedAt("mdm_servers") == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: store %s has never synced mdm_servers; run 'apple-business-cli sync --resources mdm_servers' with that org's API account\n", firstNonEmpty(label, path))
		} else if !skip && !multi {
			hintIfStale(cmd, db, "mdm_servers", flags.maxAge)
		}
		servers, err := loadLocalServers(cmd.Context(), db)
		_ = db.Close()
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		for _, s := range servers {
			rows = append(rows, serverHealth(label, s, now, staleAfter))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if (len(rows[i].Issues) > 0) != (len(rows[j].Issues) > 0) {
			return len(rows[i].Issues) > 0
		}
		return daysOr(rows[i].DaysSince) > daysOr(rows[j].DaysSince)
	})
	return rows, nil
}

func serverHealth(org string, s mdmServer, now time.Time, staleAfter time.Duration) serverHealthRow {
	a := s.Attributes
	r := serverHealthRow{Org: org, ID: s.ID, Name: firstNonEmpty(a.ServerName, s.ID), Type: a.ServerType, Status: a.Status,
		Devices: a.DeviceCount, LastConnected: a.LastConnectedDateTime, DefaultFamilies: a.DefaultProductFamilies, Issues: []string{}}
	if r.DefaultFamilies == nil {
		r.DefaultFamilies = []string{}
	}
	owns := a.DeviceCount != nil && *a.DeviceCount > 0
	last, ok := parseTime(a.LastConnectedDateTime)
	if ok {
		d := int(now.Sub(last).Hours() / 24)
		r.DaysSince = &d
	}
	switch {
	case strings.EqualFold(a.Status, "ACTIVE"):
		if owns && ok && now.Sub(last) > staleAfter {
			r.Issues = append(r.Issues, issueStale)
		}
	case strings.EqualFold(a.Status, "INACTIVE") || strings.EqualFold(a.Status, "DELETED"):
		if owns {
			r.Issues = append(r.Issues, issueDeadOwnsDevs)
		}
	}
	return r
}

// daysOr orders unknown last-connected times after every known one.
func daysOr(d *int) int {
	if d == nil {
		return -1
	}
	return *d
}
