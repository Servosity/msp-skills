// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// Hand-authored. `fleet stale`: the individual devices that stopped checking
// in, worst first, with their customer. Absorbs the ESET-MCP
// audit_inactive_devices(days) prompt.
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if fleet, _, err := root.Find([]string{"fleet"}); err == nil && fleet != root {
			addNovelCommandIfAbsent(fleet, newNovelFleetStaleCmd(flags))
		}
	})
}

type fleetStaleRow struct {
	Customer string `json:"customer"`
	// CustomerUUID is the COMPANY group's uuid, null for an owner-unknown or
	// unassigned bucket, so a real customer named like a bucket, or two
	// customers sharing a name, stay apart (handfixes.json:
	// customer-uuid-beside-customer-name).
	CustomerUUID        *string `json:"customer_uuid"`
	Device              string  `json:"device"`
	DeviceUUID          string  `json:"device_uuid"`
	Hostname            string  `json:"hostname"`
	LastSeen            string  `json:"last_seen"`
	DaysSince           int     `json:"days_since"`
	FunctionalityStatus string  `json:"functionality_status"`
	GroupPath           string  `json:"group_path"`
	OS                  string  `json:"os"`

	lastSeen time.Time
}

type fleetStaleView struct {
	Devices       []fleetStaleRow `json:"devices"`
	Total         int             `json:"total"`
	MutedExcluded int             `json:"muted_excluded"`
	ThresholdDays int             `json:"threshold_days"`
	Customer      string          `json:"customer,omitempty"`
	// UnattributedDevices counts stale devices whose device group is not in
	// the synced tree, so their customer is unknown. Without --customer they
	// are listed as "(group not in synced tree)"; with it they are left out,
	// because they may be that customer's (handfixes.json:
	// customer-views-count-unattributed-rows).
	UnattributedDevices     int      `json:"unattributed_devices"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	// SyncedHint is set when the mirror is missing or a needed resource has no
	// completed sync, so an empty devices list is not read as a clean fleet.
	SyncedHint string `json:"synced_hint,omitempty"`
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

type fleetStaleOptions struct {
	Days         int
	Customer     *esetGroup // nil: every customer
	IncludeMuted bool
	Limit        int
}

// fleetStaleRank lists devices whose last sync is older than opt.Days (or
// missing), never-seen first, then oldest first. Total counts every stale
// device before opt.Limit; muted stale devices are excluded and counted
// unless opt.IncludeMuted.
func fleetStaleRank(devices []esetDevice, tree *esetTree, opt fleetStaleOptions, now time.Time) fleetStaleView {
	view := fleetStaleView{Devices: make([]fleetStaleRow, 0), ThresholdDays: opt.Days}
	if opt.Customer != nil {
		view.Customer = opt.Customer.Name
	}
	cutoff := now.Add(-time.Duration(opt.Days) * 24 * time.Hour)
	rows := make([]fleetStaleRow, 0)
	var un esetUnattributed
	for _, d := range devices {
		// A device whose group the tree lacks is owner-unknown, not unassigned.
		cust, known := tree.ownerOf(d.ParentGroupUUID)
		seen, ok := d.lastSeen()
		if ok && !seen.Before(cutoff) {
			continue
		}
		if d.IsMuted && !opt.IncludeMuted {
			if opt.Customer == nil || cust.UUID == opt.Customer.UUID {
				view.MutedExcluded++
			}
			continue
		}
		if !known {
			un.add(d.UUID)
		}
		if opt.Customer != nil && cust.UUID != opt.Customer.UUID {
			continue
		}
		row := fleetStaleRow{
			Customer:            cust.Name,
			CustomerUUID:        esetCustomerUUID(cust),
			Device:              d.name(),
			DeviceUUID:          d.UUID,
			Hostname:            d.Hostname,
			DaysSince:           -1,
			FunctionalityStatus: shortEnum(d.FunctionalityStatus, "DEVICE_FUNCTIONALITY_STATUS_"),
			GroupPath:           tree.path(d.ParentGroupUUID),
			OS:                  d.OperatingSystem.DisplayName,
		}
		if ok {
			row.lastSeen = seen
			row.LastSeen = seen.UTC().Format(time.RFC3339)
			row.DaysSince = int(now.Sub(seen).Hours() / 24)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := rows[i].lastSeen.IsZero(), rows[j].lastSeen.IsZero()
		if ai != aj {
			return ai // never seen first
		}
		if !rows[i].lastSeen.Equal(rows[j].lastSeen) {
			return rows[i].lastSeen.Before(rows[j].lastSeen)
		}
		if rows[i].Customer != rows[j].Customer {
			return strings.ToLower(rows[i].Customer) < strings.ToLower(rows[j].Customer)
		}
		return strings.ToLower(rows[i].Device) < strings.ToLower(rows[j].Device)
	})
	view.Total = len(rows)
	view.UnattributedDevices, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	if opt.Limit > 0 && len(rows) > opt.Limit {
		rows = rows[:opt.Limit]
	}
	view.Devices = append(view.Devices, rows...)
	return view
}

func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var days, limit int
	var customer, dbPath string
	var includeMuted bool
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Devices that stopped checking in to ESET PROTECT, most stale first, with their customer",
		Long: "Use this command to list the individual devices that stopped checking in to ESET PROTECT,\n" +
			"worst first. Do NOT use it for per-customer totals; use 'fleet health' instead. Do NOT use\n" +
			"it for devices missing or lagging components; use 'fleet gaps' instead.\n\n" +
			"Reads the local mirror (sync devices and device-groups first). A device is stale when its\n" +
			"lastSyncTime is older than --days or missing; never-seen devices (days_since -1) list\n" +
			"first, then the oldest check-ins. Muted devices are left out and counted in\n" +
			"muted_excluded unless --include-muted. total counts every stale device before --limit.\n" +
			"A stale device whose group is not in the synced tree has no known customer: it is listed\n" +
			"as \"(group not in synced tree)\" (or \"(parent group not visible)\" when its group is synced\n" +
			"but a parent is not visible to the API user; see 'fleet health') and counted in unattributed_devices (uuids in\n" +
			"unattributed_device_uuids); --customer leaves it out, since it may be that customer's.",
		Example: strings.Trim(`
  eset-protect-cli fleet stale
  eset-protect-cli fleet stale --days 30 --customer "Acme Dental" --agent
  eset-protect-cli fleet stale --days 7 --include-muted --limit 25 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "local",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet stale")
			}
			if days < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--days must be at least 1"))
			}
			if limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1"))
			}
			resources := []string{"devices", "device-groups"}
			db, err := openEsetStore(cmd, dbPath, resources...)
			if err != nil {
				return err
			}
			view := fleetStaleView{Devices: make([]fleetStaleRow, 0), ThresholdDays: days}
			if db != nil {
				defer db.Close()
				esetSyncHints(cmd, db, flags.maxAge, resources...)
				groups, err := loadEsetGroups(db)
				if err != nil {
					return fmt.Errorf("reading device groups: %w", err)
				}
				devices, err := loadEsetDevices(db)
				if err != nil {
					return fmt.Errorf("reading devices: %w", err)
				}
				tree := newEsetTree(groups)
				opt := fleetStaleOptions{Days: days, IncludeMuted: includeMuted, Limit: limit}
				if strings.TrimSpace(customer) != "" {
					g, err := tree.resolveCustomer(customer)
					if err != nil {
						return usageErr(err)
					}
					opt.Customer = &g
				}
				view = fleetStaleRank(devices, tree, opt, time.Now())
				view.parentNotVisible = tree.parentNotVisible()
				view.ParentNotVisible = tree.parentNotVisibleGroups()
			}
			view.SyncedHint = esetMirrorHint(db, resources...)
			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			unattributed := ""
			if view.Customer != "" {
				unattributed = esetUnattributedLine(view.UnattributedDevices, "stale device(s)", view.UnattributedDeviceUUIDs, view.Customer, "Run '"+esetCLIName+" sync --resources device-groups' to attribute them.", view.parentNotVisible)
			}
			if len(view.Devices) == 0 {
				if unattributed != "" {
					// Never an all-clear while a stale device may be this customer's.
					esetFprintf(w, "No stale device attributed to %s for more than %d days (%d muted excluded).\n%s\n", view.Customer, days, view.MutedExcluded, unattributed)
					if view.SyncedHint != "" {
						esetFprintln(w, view.SyncedHint)
					}
					return nil
				}
				esetFprintf(w, "No devices stale for more than %d days (%d muted excluded).\n", days, view.MutedExcluded)
				if view.SyncedHint != "" {
					esetFprintln(w, view.SyncedHint)
				}
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "CUSTOMER\tDEVICE\tLAST SEEN\tDAYS\tSTATUS\tOS")
			for _, r := range view.Devices {
				last, ago := r.LastSeen, fmt.Sprint(r.DaysSince)
				if r.DaysSince < 0 {
					last, ago = "never", "-"
				}
				esetFprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Customer, r.Device, last, ago, r.FunctionalityStatus, r.OS)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			esetFprintf(w, "\n%d of %d stale devices (older than %d days), %d muted excluded\n", len(view.Devices), view.Total, days, view.MutedExcluded)
			if unattributed != "" {
				esetFprintln(w, unattributed)
			}
			if view.SyncedHint != "" {
				esetFprintln(w, view.SyncedHint)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Stale threshold: devices whose last check-in is older than this many days")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer (COMPANY group name, unique name fragment or UUID)")
	cmd.Flags().BoolVar(&includeMuted, "include-muted", false, "Include muted devices (excluded and counted in muted_excluded by default)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum devices to return")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
