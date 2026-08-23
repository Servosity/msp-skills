// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type discoveryGap struct {
	Client      string            `json:"client"`
	DeviceID    string            `json:"device_id"`
	DeviceName  string            `json:"device_name"`
	DeviceType  string            `json:"device_type,omitempty"`
	MakeModel   string            `json:"make_model,omitempty"`
	Status      string            `json:"status"`
	MissingWhat []string          `json:"credentials_rejected,omitempty"`
	Pending     []string          `json:"in_progress,omitempty"`
	Probes      map[string]string `json:"probes,omitempty"`
	Reason      string            `json:"reason"`
}

type discoveryGapsReport struct {
	DevicesTotal    int            `json:"devices_total"`
	StatusRecords   int            `json:"discovery_status_records"`
	FullyDiscovered int            `json:"fully_discovered"`
	NoEvidence      int            `json:"no_discovery_data"`
	Counts          map[string]int `json:"counts"`
	Gaps            []discoveryGap `json:"gaps"`
	Note            string         `json:"note,omitempty"`
}

// Discovery probe attributes, in the order they appear in Auvik's device
// detail payload. Each is a boolean-ish "did this method succeed" signal.
var discoveryProbes = []struct {
	field specField
	label string
}{
	{fDiscoveryLogin, "login"},
	{fDiscoverySNMP, "snmp"},
	{fDiscoveryWMI, "wmi"},
	{fDiscoveryVMware, "vmware"},
}

func newNovelDeviceDiscoveryGapsCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath string
		client string
		method string
		limit  int
	)

	cmd := &cobra.Command{
		Use:   "discovery-gaps",
		Short: "List every device Auvik cannot fully poll, per client, with the credential state behind each gap.",
		Long: strings.Trim(`
Joins the v2 device discoveryStatus records against device inventory and the
per-device credential probe results, then lists every device Auvik cannot fully
poll, grouped by client.

The v2 discoveryStatus endpoint has no v1 equivalent and is implemented by no
other Auvik tool. Today "why can't we see this box" takes three screens.

Use this command for devices Auvik cannot fully poll -- missing or failing
credentials and incomplete discovery. Do NOT use this command for devices that
are polled fine but out of hardware support; use 'eol' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,inventory-device-detail,auvik-inventory-discovery-status --full
`, "\n"),
		Example: strings.Trim(`
  # Everything Auvik cannot fully see
  auvik-cli device discovery-gaps --agent

  # Only devices where SNMP specifically is failing
  auvik-cli device discovery-gaps --method snmp --agent
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
				return writeDryRun(cmd.OutOrStdout(), flags, "device discovery-gaps")
			}
			if method != "" {
				known := false
				for _, p := range discoveryProbes {
					if p.label == method {
						known = true
						break
					}
				}
				if !known {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--method must be one of login, snmp, wmi, vmware"))
				}
			}

			empty := discoveryGapsReport{Counts: map[string]int{}, Gaps: []discoveryGap{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "auvik-inventory-discovery-status") {
				hintIfStale(cmd, db, "auvik-inventory-discovery-status", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			details, err := loadResources(ctx, db, rtDeviceDetail)
			if err != nil {
				return err
			}
			statuses, err := loadResources(ctx, db, rtDiscoveryStatus)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildDiscoveryGapsReport(devices, details, statuses, names)

			gaps := report.Gaps
			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]discoveryGap, 0, len(gaps))
				for _, g := range gaps {
					if strings.Contains(strings.ToLower(g.Client), needle) {
						filtered = append(filtered, g)
					}
				}
				gaps = filtered
			}
			if method != "" {
				filtered := make([]discoveryGap, 0, len(gaps))
				for _, g := range gaps {
					for _, m := range g.MissingWhat {
						if m == method {
							filtered = append(filtered, g)
							break
						}
					}
				}
				gaps = filtered
			}
			if limit > 0 && len(gaps) > limit {
				gaps = gaps[:limit]
			}
			report.Gaps = gaps

			if report.DevicesTotal == 0 {
				report.Note = "no devices in the local mirror; run 'auvik-cli sync --resources inventory --full'"
			} else if report.StatusRecords == 0 && len(details) == 0 {
				report.Note = "devices are synced but no discovery-status or device-detail records were found; run 'auvik-cli sync --resources auvik-inventory-discovery-status,inventory-device-detail --full'"
			}

			done, err := emitLocalReport(cmd, flags, report, report.Gaps)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Discovery gaps: %d flagged of %d devices (%d fully discovered, %d with no discovery data)\n",
				report.Counts["credentialsRejected"]+report.Counts["inProgress"], report.DevicesTotal,
				report.FullyDiscovered, report.NoEvidence)
			if len(report.Counts) > 0 {
				keys := make([]string, 0, len(report.Counts))
				for k := range report.Counts {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				parts := make([]string, 0, len(keys))
				for _, k := range keys {
					parts = append(parts, fmt.Sprintf("%s: %d", k, report.Counts[k]))
				}
				fmt.Fprintf(out, "  %s\n", strings.Join(parts, "   "))
			}
			fmt.Fprintln(out)
			if len(report.Gaps) == 0 {
				if report.Note != "" {
					fmt.Fprintln(out, report.Note)
				} else {
					fmt.Fprintln(out, "No discovery gaps found.")
				}
				return nil
			}
			fmt.Fprintf(out, "%-22s %-28s %-16s %s\n", "CLIENT", "DEVICE", "MISSING", "REASON")
			for _, g := range report.Gaps {
				fmt.Fprintf(out, "%-22s %-28s %-16s %s\n",
					truncate(g.Client, 22), truncate(g.DeviceName, 28),
					truncate(strings.Join(g.MissingWhat, ","), 16), g.Reason)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&client, "client", "", "Only show devices whose client name contains this substring")
	cmd.Flags().StringVar(&method, "method", "", "Only devices failing one probe: login, snmp, wmi, vmware")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

// buildDiscoveryGapsReport is the pure join core, split out for table tests.
func buildDiscoveryGapsReport(devices, details, statuses []auvikRow, tenants map[string]string) discoveryGapsReport {
	report := discoveryGapsReport{
		DevicesTotal:  len(devices),
		StatusRecords: len(statuses),
		Counts:        map[string]int{},
		Gaps:          make([]discoveryGap, 0),
	}

	statusByDevice := map[string]auvikRow{}
	for _, s := range statuses {
		id := firstNonEmpty(s.rel("device"), s.attrString("deviceId"), s.ID)
		statusByDevice[id] = s
	}
	detailByDevice := map[string]auvikRow{}
	for _, d := range details {
		id := firstNonEmpty(d.rel("device"), d.attrString("deviceId"), d.ID)
		detailByDevice[id] = d
	}

	ordered := make([]auvikRow, len(devices))
	copy(ordered, devices)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	for _, dev := range ordered {
		status := ""
		probeVals := map[string]string{}
		if st, ok := statusByDevice[dev.ID]; ok {
			// v2 deviceDiscoveryStatusAttributes: login/snmp/vmware/wmi, each an
			// enum of disabled|determining|notSupported|notAuthorized|
			// authorizing|authorized|privileged.
			for _, probe := range discoveryProbes {
				probeVals[probe.label] = st.attrString(probe.field.Field)
			}
		}
		if d, ok := detailByDevice[dev.ID]; ok {
			status = d.attrString(fDetailDiscoveryStatus.Field)
		}

		failing := make([]string, 0, len(discoveryProbes))
		pending := make([]string, 0, len(discoveryProbes))
		haveEvidence := false
		for _, probe := range discoveryProbes {
			v := probeVals[probe.label]
			if v == "" {
				continue
			}
			haveEvidence = true
			switch {
			case v == "notAuthorized":
				failing = append(failing, probe.label)
			case discoveryProbeTransitional(v):
				pending = append(pending, probe.label)
			}
		}
		if status != "" {
			haveEvidence = true
		}

		// A device with no discovery-status record AND no detail record tells us
		// nothing. Counting it either way would be a lie, so it is neither a gap
		// nor "fully discovered".
		if !haveEvidence {
			report.NoEvidence++
			continue
		}
		if len(failing) == 0 && len(pending) == 0 {
			report.FullyDiscovered++
			continue
		}

		reason := ""
		switch {
		case len(failing) > 0 && len(pending) > 0:
			reason = "credentials rejected: " + strings.Join(failing, ", ") +
				"; still determining: " + strings.Join(pending, ", ")
		case len(failing) > 0:
			reason = "credentials rejected: " + strings.Join(failing, ", ")
		default:
			reason = "discovery still in progress: " + strings.Join(pending, ", ")
		}
		if status != "" {
			reason += " (detail status: " + status + ")"
		}

		bucket := "credentialsRejected"
		if len(failing) == 0 {
			bucket = "inProgress"
		}
		report.Counts[bucket]++

		report.Gaps = append(report.Gaps, discoveryGap{
			Client:      tenantLabel(tenants, dev.rel("tenant")),
			DeviceID:    dev.ID,
			DeviceName:  firstNonEmpty(dev.attrString(fDeviceName.Field), dev.ID),
			DeviceType:  dev.attrString(fDeviceType.Field),
			MakeModel:   dev.attrString(fDeviceMakeModel.Field),
			Status:      status,
			Probes:      probeVals,
			MissingWhat: failing,
			Pending:     pending,
			Reason:      reason,
		})
	}

	sort.SliceStable(report.Gaps, func(i, j int) bool {
		a, b := report.Gaps[i], report.Gaps[j]
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		return a.DeviceName < b.DeviceName
	})
	return report
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "ok", "success", "succeeded", "discovered":
			return true
		}
		return false
	case float64:
		return t != 0
	}
	return false
}
