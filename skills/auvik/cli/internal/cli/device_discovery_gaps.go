// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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

// Discovery probes. Each is an enum (disabled, determining, notSupported,
// notAuthorized, authorizing, authorized, privileged) carried both by v2
// deviceDiscoveryStatusAttributes and by v1 deviceDetailsAttributes.discoveryStatus.
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
Joins the v2 device discoveryStatus records and the v1 device detail
discoveryStatus probes against device inventory, then lists every device Auvik
cannot fully poll, grouped by client. A probe rejected in either source is a gap.
Today "why can't we see this box" takes three screens.

Use this command for devices Auvik cannot fully poll -- missing or failing
credentials and incomplete discovery. Do NOT use this command for devices that
are polled fine but out of hardware support; use 'eol' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory-device-info,inventory-device-detail,auvik-inventory --full
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

			if !hintIfUnsynced(cmd, db, rtDiscoveryStatus.current()) {
				hintIfStale(cmd, db, rtDiscoveryStatus.current(), flags.maxAge)
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
				report.Note = "no devices in the local mirror; run 'auvik-cli sync --resources inventory-device-info --full'"
			} else if report.StatusRecords == 0 && len(details) == 0 {
				report.Note = "devices are synced but no discovery-status or device-detail records were found; run 'auvik-cli sync --resources auvik-inventory,inventory-device-detail --full'"
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
		// Two documented sources carry the same four probe enums: v2
		// deviceDiscoveryStatusAttributes (top-level login/snmp/wmi/vmware)
		// and v1 deviceDetailsAttributes.discoveryStatus (the same four,
		// nested one level down). Both go through one reader, and the worst
		// value per probe wins, so a rejected credential in either source is
		// a gap whatever the other says.
		status := ""
		var sources []map[string]string
		if st, ok := statusByDevice[dev.ID]; ok {
			sources = append(sources, discoveryProbeStates(st))
		}
		if d, ok := detailByDevice[dev.ID]; ok {
			sources = append(sources, discoveryProbeStates(d, fDetailDiscoveryStatus.Field))
			status = detailDiscoveryStatusText(d)
		}
		probeVals := mergeDiscoveryProbes(sources...)

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

// discoveryProbeStates reads the four discovery probe enums from one record,
// at attributes.<base...>.<probe>. Values that are not strings are skipped:
// they carry no documented probe state.
func discoveryProbeStates(r auvikRow, base ...string) map[string]string {
	out := map[string]string{}
	for _, p := range discoveryProbes {
		path := append(append([]string{}, base...), p.field.Field)
		if v, ok := r.attr(path...).(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				out[p.label] = v
			}
		}
	}
	return out
}

// discoveryProbeSeverity ranks a probe value so the worst one wins a merge:
// a rejected credential, then discovery still in progress, then anything else.
func discoveryProbeSeverity(v string) int {
	switch {
	case v == "":
		return 0
	case v == "notAuthorized":
		return 3
	case discoveryProbeTransitional(v):
		return 2
	default:
		return 1
	}
}

// mergeDiscoveryProbes folds several sources into one value per probe,
// keeping the most severe. Every signal any source had is kept.
func mergeDiscoveryProbes(sources ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, src := range sources {
		for label, v := range src {
			if discoveryProbeSeverity(v) > discoveryProbeSeverity(out[label]) {
				out[label] = v
			}
		}
	}
	return out
}

// detailDiscoveryStatusText renders deviceDetailsAttributes.discoveryStatus
// for the report. The documented shape is an object of probe enums; a bare
// string is passed through, and any other shape renders as nothing rather
// than as a Go value dump.
func detailDiscoveryStatusText(d auvikRow) string {
	switch v := d.attr(fDetailDiscoveryStatus.Field).(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		states := discoveryProbeStates(d, fDetailDiscoveryStatus.Field)
		parts := make([]string, 0, len(states))
		for _, p := range discoveryProbes {
			if s, ok := states[p.label]; ok {
				parts = append(parts, p.label+"="+s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
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
