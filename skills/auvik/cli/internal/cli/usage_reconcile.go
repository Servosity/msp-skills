// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type reconcileClient struct {
	Client         string   `json:"client"`
	TenantID       string   `json:"tenant_id"`
	BilledCount    *int     `json:"billed_count"`
	InventoryCount int      `json:"inventory_count"`
	Delta          *int     `json:"delta"`
	BilledNotSeen  []string `json:"billed_not_seen,omitempty"`
	SeenNotBilled  []string `json:"seen_not_billed,omitempty"`

	// Counts Auvik publishes alongside the billed device list.
	MonitoredServers         int `json:"monitored_servers"`
	MonitoredWorkstations    int `json:"monitored_workstations"`
	MaxMonitoredServers      int `json:"max_monitored_servers"`
	MaxMonitoredWorkstations int `json:"max_monitored_workstations"`
	BillableDays             int `json:"billable_days"`

	Note string `json:"note,omitempty"`
}

type reconcileReport struct {
	Clients      []reconcileClient `json:"clients"`
	ClientsTotal int               `json:"clients_total"`
	Reconciled   int               `json:"reconciled"`
	Mismatched   int               `json:"mismatched"`
	Note         string            `json:"note,omitempty"`
}

func newNovelUsageReconcileCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath       string
		client       string
		mismatchOnly bool
		limit        int
	)

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Put each client's billable usage count next to the actual synced inventory and show the device rows behind the difference.",
		Long: strings.Trim(`
Joins the locally synced billing usage records against the synced device
inventory per client and reports the delta, naming the devices on each side.

Auvik's usage record carries the LIST of devices it billed for, but the Auvik
UI shows you a number. When a client's billable count moves, this is what tells
you which devices caused it -- matched by device id on both sides.

Use this command for billable-device counts and invoice reconciliation per
client. Do NOT use this command for what changed in the fleet since the last
sync; use 'inventory diff' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,billing --full
`, "\n"),
		Example: strings.Trim(`
  # Every client, billed vs seen
  auvik-cli usage reconcile --agent

  # Only clients whose counts disagree
  auvik-cli usage reconcile --mismatch-only --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "usage reconcile")
			}

			empty := reconcileReport{Clients: []reconcileClient{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "billing") {
				hintIfStale(cmd, db, "billing", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			billing, err := loadResources(ctx, db, rtBilling)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildReconcileReport(devices, billing, names)

			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]reconcileClient, 0, len(report.Clients))
				for _, c := range report.Clients {
					if strings.Contains(strings.ToLower(c.Client), needle) {
						filtered = append(filtered, c)
					}
				}
				report.Clients = filtered
			}
			if mismatchOnly {
				filtered := make([]reconcileClient, 0, len(report.Clients))
				for _, c := range report.Clients {
					if c.Delta != nil && *c.Delta != 0 {
						filtered = append(filtered, c)
					}
				}
				report.Clients = filtered
			}
			if limit > 0 && len(report.Clients) > limit {
				report.Clients = report.Clients[:limit]
			}

			if len(billing) == 0 {
				report.Note = "no billing usage in the local mirror; run 'auvik-cli sync --resources billing --full'. Inventory counts are shown without a billed comparison."
			}

			done, err := emitLocalReport(cmd, flags, report, report.Clients)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Billable reconciliation across %d client(s): %d match, %d mismatch\n\n",
				report.ClientsTotal, report.Reconciled, report.Mismatched)
			if report.Note != "" {
				fmt.Fprintf(out, "%s\n\n", report.Note)
			}
			if len(report.Clients) == 0 {
				fmt.Fprintln(out, "No clients match the given filters.")
				return nil
			}
			fmt.Fprintf(out, "%-26s %8s %10s %8s  %s\n", "CLIENT", "BILLED", "INVENTORY", "DELTA", "NOTE")
			for _, c := range report.Clients {
				billed, delta := "-", "-"
				if c.BilledCount != nil {
					billed = fmt.Sprintf("%d", *c.BilledCount)
				}
				if c.Delta != nil {
					delta = fmt.Sprintf("%+d", *c.Delta)
				}
				fmt.Fprintf(out, "%-26s %8s %10d %8s  %s\n",
					truncate(c.Client, 26), billed, c.InventoryCount, delta, c.Note)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&client, "client", "", "Only show clients whose name contains this substring")
	cmd.Flags().BoolVar(&mismatchOnly, "mismatch-only", false, "Only show clients whose billed and inventory counts disagree")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clients to return (0 = all)")
	return cmd
}

// buildReconcileReport is the pure join core, split out for table tests.
//
// FIELD SOURCING: there is no "billable device count" attribute anywhere in
// clientUsageAttributes. What Auvik actually publishes is
// clientUsageRelationships.devices -- the LIST of devices it billed for -- plus
// monitoredServers / monitoredWorkstations counts. The device list is stronger
// than a bare count: it lets us name the devices on each side of a delta, which
// is exactly the attribution the usage endpoint refuses to give.
// The usage record identifies its client by the domainPrefix ATTRIBUTE, which is
// the same value tenants carry, so that is the join key.
func buildReconcileReport(devices, billing []auvikRow, tenants map[string]string) reconcileReport {
	// Inventory: tenant id -> device id -> device name.
	inventoryByTenant := map[string]map[string]string{}
	for _, d := range devices {
		t := d.rel("tenant")
		if inventoryByTenant[t] == nil {
			inventoryByTenant[t] = map[string]string{}
		}
		inventoryByTenant[t][d.ID] = firstNonEmpty(d.attrString(fDeviceName.Field), d.ID)
	}

	// tenant label -> tenant id, so a usage record's domainPrefix can find it.
	tenantByLabel := map[string]string{}
	for id, label := range tenants {
		tenantByLabel[strings.ToLower(label)] = id
	}

	type usageFacts struct {
		tenantID        string
		label           string
		billedDevices   map[string]string // id -> name
		monServers      int
		monWorkstations int
		maxServers      int
		maxWorkstations int
		billableDays    int
		haveUsage       bool
	}
	usage := map[string]*usageFacts{}

	for _, b := range billing {
		prefix := b.attrString(fUsageDomainPrefix.Field)
		tenantID := tenantByLabel[strings.ToLower(prefix)]
		if tenantID == "" {
			// Fall back to the clients relationship when domainPrefix does not
			// match a synced tenant.
			if ids := b.relMany(rUsageClients.Name); len(ids) > 0 {
				tenantID = ids[0]
			}
		}
		key := tenantID
		if key == "" {
			key = "prefix:" + prefix
		}
		f := usage[key]
		if f == nil {
			f = &usageFacts{tenantID: tenantID, label: firstNonEmpty(prefix, tenantLabel(tenants, tenantID)),
				billedDevices: map[string]string{}}
			usage[key] = f
		}
		f.haveUsage = true
		for id, name := range b.relManyAttr(rUsageDevices.Name, fDeviceName.Field) {
			f.billedDevices[id] = name
		}
		if n, ok := numberAttr(b, fUsageMonServers.Field); ok {
			f.monServers = n
		}
		if n, ok := numberAttr(b, fUsageMonWorkstations.Field); ok {
			f.monWorkstations = n
		}
		if n, ok := numberAttr(b, fUsageMaxServers.Field); ok {
			f.maxServers = n
		}
		if n, ok := numberAttr(b, fUsageMaxWorkstations.Field); ok {
			f.maxWorkstations = n
		}
		if n, ok := numberAttr(b, fUsageBillableDays.Field); ok {
			f.billableDays = n
		}
	}

	keys := map[string]bool{}
	for t := range inventoryByTenant {
		keys[t] = true
	}
	for k, f := range usage {
		if f.tenantID != "" {
			keys[f.tenantID] = true
		} else {
			keys[k] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	report := reconcileReport{Clients: make([]reconcileClient, 0, len(ordered))}
	for _, key := range ordered {
		inv := inventoryByTenant[key]
		var f *usageFacts
		for _, cand := range usage {
			if cand.tenantID == key {
				f = cand
				break
			}
		}
		if f == nil {
			f = usage[key]
		}

		row := reconcileClient{
			Client:         tenantLabel(tenants, key),
			TenantID:       key,
			InventoryCount: len(inv),
		}
		if f != nil && f.label != "" {
			row.Client = f.label
		}
		if f == nil || !f.haveUsage {
			row.Note = "no billing usage synced for this client"
			report.Clients = append(report.Clients, row)
			continue
		}

		billed := len(f.billedDevices)
		row.BilledCount = &billed
		delta := len(inv) - billed
		row.Delta = &delta
		row.MonitoredServers = f.monServers
		row.MonitoredWorkstations = f.monWorkstations
		row.MaxMonitoredServers = f.maxServers
		row.MaxMonitoredWorkstations = f.maxWorkstations
		row.BillableDays = f.billableDays

		// Attribution by device ID -- ids on both sides, never id-vs-name.
		for id, name := range f.billedDevices {
			if _, present := inv[id]; !present {
				row.BilledNotSeen = append(row.BilledNotSeen, firstNonEmpty(name, id))
			}
		}
		for id, name := range inv {
			if _, billedHere := f.billedDevices[id]; !billedHere {
				row.SeenNotBilled = append(row.SeenNotBilled, firstNonEmpty(name, id))
			}
		}
		sort.Strings(row.BilledNotSeen)
		sort.Strings(row.SeenNotBilled)

		switch {
		case delta == 0 && len(row.BilledNotSeen) == 0 && len(row.SeenNotBilled) == 0:
			row.Note = "counts agree"
			report.Reconciled++
		case delta > 0:
			row.Note = fmt.Sprintf("%d device(s) in inventory are not in the billed set", delta)
			report.Mismatched++
		case delta < 0:
			row.Note = fmt.Sprintf("%d billed device(s) are not present in inventory", -delta)
			report.Mismatched++
		default:
			row.Note = "counts match but the device sets differ"
			report.Mismatched++
		}
		report.Clients = append(report.Clients, row)
	}
	report.ClientsTotal = len(report.Clients)
	return report
}

// numberAttr reads a numeric attribute, accepting JSON numbers and the
// JSON-encoded-string form some feeds use.
func numberAttr(r auvikRow, name string) (int, bool) {
	switch v := r.attr(name).(type) {
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
