// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// Hand-authored. `fleet health`: one row per customer with device health
// counts, rolled up from the synced devices x device-group tree.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

const (
	fleetHealthStatusPrefix      = "DEVICE_FUNCTIONALITY_STATUS_"
	fleetHealthEnrollPending     = "DEVICE_ENROLLMENT_STATUS_PENDING"
	fleetHealthEnrollFailed      = "DEVICE_ENROLLMENT_STATUS_FAILED"
	fleetHealthDefaultStaleDays  = 7
	fleetHealthDefaultLimit      = 100
	fleetHealthScoreFormulaShort = "attention_required*3 + stale*2 + attention_recommended + unenrolled + no_product"
)

// fleetHealthCounts are the per-device tallies shared by customer rows and totals.
type fleetHealthCounts struct {
	Devices              int   `json:"devices"`
	OK                   int   `json:"ok"`
	AttentionRecommended int   `json:"attention_recommended"`
	AttentionRequired    int   `json:"attention_required"`
	Problems             int64 `json:"problems"`
	Muted                int   `json:"muted"`
	Stale                int   `json:"stale"`
	Unenrolled           int   `json:"unenrolled"`
	NoProduct            int   `json:"no_product"`
	Score                int   `json:"score"`
}

type fleetHealthRow struct {
	Customer     string `json:"customer"`
	CustomerUUID string `json:"customer_uuid"`
	fleetHealthCounts
}

type fleetHealthView struct {
	Customers []fleetHealthRow  `json:"customers"`
	Totals    fleetHealthCounts `json:"totals"`
	StaleDays int               `json:"stale_days"`
	// UnattributedDevices counts synced devices whose device group is not in
	// the synced tree, so their customer is unknown. They roll up to the
	// "(group not in synced tree)" row, which --customer leaves out because
	// they may be that customer's (handfixes.json:
	// customer-views-count-unattributed-rows).
	UnattributedDevices     int      `json:"unattributed_devices"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	SyncedHint              string   `json:"synced_hint,omitempty"`

	customer string // --customer name for the human output
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

// fleetHealthUnenrolled reports enrollment that never completed.
func fleetHealthUnenrolled(status string) bool {
	return status == fleetHealthEnrollPending || status == fleetHealthEnrollFailed
}

// fleetHealthIsStale is true when lastSyncTime is missing, malformed, or older
// than staleDays before now.
func fleetHealthIsStale(d esetDevice, now time.Time, staleDays int) bool {
	seen, ok := d.lastSeen()
	if !ok {
		return true
	}
	return seen.Before(now.Add(-time.Duration(staleDays) * 24 * time.Hour))
}

// add tallies one device into the counts.
func (c *fleetHealthCounts) add(d esetDevice, now time.Time, staleDays int) {
	c.Devices++
	switch d.FunctionalityStatus {
	case fleetHealthStatusPrefix + "OK":
		c.OK++
	case fleetHealthStatusPrefix + "ATTENTION_RECOMMENDED":
		c.AttentionRecommended++
	case fleetHealthStatusPrefix + "ATTENTION_REQUIRED":
		c.AttentionRequired++
	}
	c.Problems += int64(d.ProblemCount)
	if d.IsMuted {
		c.Muted++
	}
	if fleetHealthIsStale(d, now, staleDays) {
		c.Stale++
	}
	if fleetHealthUnenrolled(d.EnrollmentStatus) {
		c.Unenrolled++
	}
	if len(d.ActiveProducts) == 0 {
		c.NoProduct++
	}
}

// fleetHealthScore is the documented worst-first sort key.
func fleetHealthScore(c fleetHealthCounts) int {
	return c.AttentionRequired*3 + c.Stale*2 + c.AttentionRecommended + c.Unenrolled + c.NoProduct
}

// fleetHealthRollup returns one row per COMPANY customer (even with zero
// devices) plus the "(unassigned)" / "(tenant)" buckets when they hold
// devices, sorted worst first.
func fleetHealthRollup(devices []esetDevice, tree *esetTree, now time.Time, staleDays int) []fleetHealthRow {
	byKey := map[string]*fleetHealthRow{}
	order := make([]string, 0)
	keyOf := func(g esetGroup) string {
		if g.UUID != "" {
			return g.UUID
		}
		return "bucket:" + g.Name
	}
	ensure := func(g esetGroup) *fleetHealthRow {
		k := keyOf(g)
		if r, ok := byKey[k]; ok {
			return r
		}
		r := &fleetHealthRow{Customer: g.Name, CustomerUUID: g.UUID}
		byKey[k] = r
		order = append(order, k)
		return r
	}
	for _, g := range tree.companies() {
		ensure(g)
	}
	for _, d := range devices {
		// A device whose group the tree lacks is owner-unknown, not unassigned.
		owner, _ := tree.ownerOf(d.ParentGroupUUID)
		ensure(owner).add(d, now, staleDays)
	}
	rows := make([]fleetHealthRow, 0, len(order))
	for _, k := range order {
		r := *byKey[k]
		r.Score = fleetHealthScore(r.fleetHealthCounts)
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Devices != rows[j].Devices {
			return rows[i].Devices > rows[j].Devices
		}
		return strings.ToLower(rows[i].Customer) < strings.ToLower(rows[j].Customer)
	})
	return rows
}

// fleetHealthTotal sums customer rows; Score is recomputed from the sums.
func fleetHealthTotal(rows []fleetHealthRow) fleetHealthCounts {
	var t fleetHealthCounts
	for _, r := range rows {
		t.Devices += r.Devices
		t.OK += r.OK
		t.AttentionRecommended += r.AttentionRecommended
		t.AttentionRequired += r.AttentionRequired
		t.Problems += r.Problems
		t.Muted += r.Muted
		t.Stale += r.Stale
		t.Unenrolled += r.Unenrolled
		t.NoProduct += r.NoProduct
	}
	t.Score = fleetHealthScore(t)
	return t
}

func fleetHealthPrintTable(w io.Writer, v fleetHealthView) error {
	if len(v.Customers) == 0 {
		_, err := fmt.Fprintln(w, "No customers or devices in the local mirror.")
		if err == nil && v.SyncedHint != "" {
			_, err = esetFprintln(w, v.SyncedHint)
		}
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CUSTOMER\tDEVICES\tOK\tATTN_REC\tATTN_REQ\tPROBLEMS\tMUTED\tSTALE\tUNENROLLED\tNO_PRODUCT\tSCORE")
	line := func(name string, c fleetHealthCounts) {
		esetFprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", name, c.Devices, c.OK, c.AttentionRecommended,
			c.AttentionRequired, c.Problems, c.Muted, c.Stale, c.Unenrolled, c.NoProduct, c.Score)
	}
	for _, r := range v.Customers {
		line(r.Customer, r.fleetHealthCounts)
	}
	line("TOTAL", v.Totals)
	if err := tw.Flush(); err != nil {
		return err
	}
	if un := esetUnattributedLine(v.UnattributedDevices, "synced device(s)", v.UnattributedDeviceUUIDs, v.customer, "Run '"+esetCLIName+" sync --resources device-groups' to attribute them.", v.parentNotVisible); un != "" {
		if _, err := esetFprintf(w, "\n%s\n", un); err != nil {
			return err
		}
	}
	_, err := esetFprintf(w, "\nstale = no check-in for %d days; score = %s\n", v.StaleDays, fleetHealthScoreFormulaShort)
	if err == nil && v.SyncedHint != "" {
		_, err = esetFprintln(w, v.SyncedHint)
	}
	return err
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var staleDays, limit int
	var customer, dbPath string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "One row per customer with device count, attention-required and attention-recommended devices, problem count, muted",
		Long: "Per-customer device health from the local mirror: a join of synced devices with the\n" +
			"device-group ancestry. A device's customer is the nearest ancestor group linked to a\n" +
			"COMPANY; devices outside every COMPANY subtree roll up to \"(unassigned)\", and a tree with\n" +
			"no COMPANY group (a single-tenant account) rolls up to one \"(tenant)\" row. \"(unassigned)\"\n" +
			"holds the MSP's own devices: a device whose ancestry reaches the root, a group linked to\n" +
			"the MSP, or the hidden root group of a scoped API user (the one parent every visible\n" +
			"top-level group names but ESET never returns) without passing a COMPANY. A device whose\n" +
			"group is not in the synced tree has no known customer and rolls up to \"(group not in\n" +
			"synced tree)\". One whose group is synced but sits under a parent group the mirror does not\n" +
			"hold (other than that hidden root) rolls up to \"(parent group not visible)\": the parent is\n" +
			"usually outside the API user's permissions, which no sync changes. Both are counted in\n" +
			"unattributed_devices (uuids in unattributed_device_uuids); --customer leaves those rows out,\n" +
			"since those devices may be the customer's. Those buckets appear only when they hold devices;\n" +
			"every COMPANY customer\n" +
			"appears, even with 0 devices.\n\n" +
			"Per customer: devices, functionalityStatus OK / ATTENTION_RECOMMENDED / ATTENTION_REQUIRED\n" +
			"(a device with an unspecified status counts only in devices), problems (sum of\n" +
			"functionalityProblemCount), muted, stale (lastSyncTime older than --stale-days, or\n" +
			"missing), unenrolled (enrollment PENDING or FAILED) and no_product (no active ESET product).\n\n" +
			"Rows are sorted worst first by\n" +
			"  score = " + fleetHealthScoreFormulaShort + "\n" +
			"then by devices, then by name. totals sums every customer in scope before --limit.\n\n" +
			"Use this command for per-customer totals of device health. Do NOT use it to list the individual stale devices; use 'fleet stale' instead. Do NOT use it to list devices missing or lagging components; use 'fleet gaps' instead.",
		Example: strings.Trim(`
  eset-protect-cli fleet health
  eset-protect-cli fleet health --stale-days 14 --limit 20
  eset-protect-cli fleet health --customer "Acme Dental" --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if staleDays < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale-days must be at least 1 (got %d)", staleDays))
			}
			if limit < 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be 0 (no limit) or more (got %d)", limit))
			}
			view := fleetHealthView{Customers: make([]fleetHealthRow, 0), StaleDays: staleDays}
			emit := func() error {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return fleetHealthPrintTable(cmd.OutOrStdout(), view)
			}

			resources := []string{"devices", "device-groups"}
			db, err := openEsetStore(cmd, dbPath, resources...)
			if err != nil {
				return err
			}
			// One completeness check (handfixes.json: mirror-views-synced-hint):
			// an interrupted sync leaves partial rows, never a "no hint" answer.
			mirror := esetMirrorCheck(db, resources...)
			if db == nil {
				view.SyncedHint = mirror.Hint
				return emit()
			}
			defer db.Close()
			esetSyncHints(cmd, db, flags.maxAge, resources...)

			groups, err := loadEsetGroups(db)
			if err != nil {
				return fmt.Errorf("reading synced device groups: %w", err)
			}
			devices, err := loadEsetDevices(db)
			if err != nil {
				return fmt.Errorf("reading synced devices: %w", err)
			}
			tree := newEsetTree(groups)
			view.parentNotVisible = tree.parentNotVisible()
			view.ParentNotVisible = tree.parentNotVisibleGroups()
			rows := fleetHealthRollup(devices, tree, time.Now(), staleDays)
			var un esetUnattributed
			for _, d := range devices {
				if _, known := tree.ownerOf(d.ParentGroupUUID); !known {
					un.add(d.UUID)
				}
			}
			view.UnattributedDevices, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()

			if customer != "" {
				g, rerr := tree.resolveCustomer(customer)
				if rerr != nil {
					return usageErr(rerr)
				}
				view.customer = g.Name
				kept := make([]fleetHealthRow, 0, 1)
				for _, r := range rows {
					if r.CustomerUUID == g.UUID {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			view.Totals = fleetHealthTotal(rows)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			view.Customers = rows
			empty := ""
			switch {
			case len(devices) == 0:
				empty = "no devices in the local mirror; run '" + esetCLIName + " sync --resources devices,device-groups'"
			case len(groups) == 0:
				empty = "no device groups in the local mirror, so every device rolled up to " + esetCustomerTenant + "; run '" + esetCLIName + " sync --resources device-groups' for per-customer rows"
			}
			view.SyncedHint = esetJoinHints(mirror.Hint, empty)
			return emit()
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", fleetHealthDefaultStaleDays, "Count a device as stale when its last check-in is older than this many days (or missing)")
	cmd.Flags().StringVar(&customer, "customer", "", "Only this customer: COMPANY group name, unique name fragment, or UUID")
	cmd.Flags().IntVar(&limit, "limit", fleetHealthDefaultLimit, "Maximum customer rows to show, worst first (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
