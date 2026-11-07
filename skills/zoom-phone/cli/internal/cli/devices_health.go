// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: desk phone health by site.
// pp:data-source local
// Joins the local device rows (GET /phone/devices?type=assigned|unassigned),
// phone users and common area desk phones. No API calls.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var localDevicesSyncResources = []string{"assigned", "unassigned", "users", "common_areas"}

const (
	localFlagOffline          = "OFFLINE"
	localFlagUnassigned       = "UNASSIGNED"
	localFlagFirmwareDrift    = "FIRMWARE_DRIFT"
	localFlagAssigneeInactive = "ASSIGNEE_INACTIVE"
	localFlagNoTemplate       = "NO_TEMPLATE"
	localFlagAmbiguous        = "ASSIGNMENT_AMBIGUOUS"
)

type localDeviceFinding struct {
	DeviceID        string   `json:"device_id"`
	DisplayName     string   `json:"display_name"`
	MacAddress      string   `json:"mac_address,omitempty"`
	DeviceType      string   `json:"device_type"`
	FirmwareVersion string   `json:"firmware_version,omitempty"`
	Status          string   `json:"status"`
	Site            string   `json:"site"`
	AssigneeName    string   `json:"assignee_name,omitempty"`
	AssigneeType    string   `json:"assignee_type,omitempty"`
	Flags           []string `json:"flags"`
	Details         []string `json:"details"`
}

type localFirmwareBaseline struct {
	DeviceType string         `json:"device_type"`
	Baseline   string         `json:"baseline_version,omitempty"`
	Devices    int            `json:"devices"`
	Versions   map[string]int `json:"versions"`
	Tie        bool           `json:"tie,omitempty"`
}

type localSiteHealth struct {
	Site     string               `json:"site"`
	Devices  int                  `json:"devices"`
	Counts   map[string]int       `json:"counts"`
	Findings []localDeviceFinding `json:"findings"`
}

type localDevicesHealthResult struct {
	Assessed          bool                    `json:"assessed"`
	SiteFilter        string                  `json:"site_filter,omitempty"`
	DevicesChecked    int                     `json:"devices_checked"`
	Counts            map[string]int          `json:"counts"`
	BySite            []localSiteHealth       `json:"by_site"`
	FirmwareBaselines []localFirmwareBaseline `json:"firmware_baselines"`
	MissingLocal      []string                `json:"missing_local"`
	MissingReasons    map[string]string       `json:"missing_reasons"`
	Note              string                  `json:"note,omitempty"`
}

func localNewDevicesHealthResult(site string) *localDevicesHealthResult {
	return &localDevicesHealthResult{
		SiteFilter:        site,
		Counts:            localDeviceCounts(),
		BySite:            make([]localSiteHealth, 0),
		FirmwareBaselines: make([]localFirmwareBaseline, 0),
		MissingLocal:      make([]string, 0),
		MissingReasons:    map[string]string{},
	}
}

// localDeviceFlags lists every flag the command reports.
var localDeviceFlags = []string{localFlagOffline, localFlagUnassigned, localFlagFirmwareDrift, localFlagAssigneeInactive, localFlagNoTemplate, localFlagAmbiguous}

// localDeviceCounts returns per-flag counts pre-seeded at 0 plus "total",
// the number of flagged devices (findings), so the shape never varies.
func localDeviceCounts() map[string]int {
	m := map[string]int{"total": 0}
	for _, f := range localDeviceFlags {
		m[f] = 0
	}
	return m
}

func localDeviceTypeKey(d localDevice) string {
	return strings.ToLower(strings.TrimSpace(d.DeviceType.String()))
}

// localFirmwareBaselines finds, per device type (manufacturer + model as
// returned in device_type), the most common firmware version. A tie for the
// top count has no baseline, so no device of that type is called drifted.
func localFirmwareBaselines(devices []localDevice) map[string]localFirmwareBaseline {
	byType := map[string]*localFirmwareBaseline{}
	for _, d := range devices {
		fw := d.FirmwareVersion.String()
		key := localDeviceTypeKey(d)
		if fw == "" || key == "" {
			continue
		}
		b, ok := byType[key]
		if !ok {
			b = &localFirmwareBaseline{DeviceType: d.DeviceType.String(), Versions: map[string]int{}}
			byType[key] = b
		}
		b.Devices++
		b.Versions[fw]++
	}
	out := map[string]localFirmwareBaseline{}
	for key, b := range byType {
		best, bestN, second := "", 0, 0
		versions := make([]string, 0, len(b.Versions))
		for v := range b.Versions {
			versions = append(versions, v)
		}
		sort.Strings(versions)
		for _, v := range versions {
			n := b.Versions[v]
			switch {
			case n > bestN:
				second = bestN
				best, bestN = v, n
			case n > second:
				second = n
			}
		}
		if len(versions) > 1 && bestN == second {
			b.Tie = true
		} else {
			b.Baseline = best
		}
		out[key] = *b
	}
	return out
}

// localDevicesHealth is the pure join behind `devices health`.
//
// usersSynced is whether the users collection finished syncing; only then is
// a user assignee absent from it flagged as not in the phone users.
func localDevicesHealth(devices []localDevice, users []localUser, usersSynced bool, areas []localCommonArea, site string) *localDevicesHealthResult {
	res := localNewDevicesHealthResult(site)
	known := map[string]bool{}
	for _, d := range devices {
		known[d.ID.String()] = true
	}
	all := append([]localDevice{}, devices...)
	for _, a := range areas {
		for _, p := range a.DeskPhones {
			if p.ID.String() == "" || known[p.ID.String()] {
				continue
			}
			known[p.ID.String()] = true
			all = append(all, localDevice{
				ID:          p.ID,
				DisplayName: p.DisplayName,
				DeviceType:  p.DeviceType,
				Status:      p.Status,
				Site:        a.Site,
				Assignee:    &localDeviceAssignee{ID: a.ID, Name: a.DisplayName, ExtensionNumber: a.ExtensionNumber, ExtensionType: "commonArea"},
				localSource: "common_areas",
			})
		}
	}
	usersByID := map[string]localUser{}
	for _, u := range users {
		usersByID[u.ID.String()] = u
	}

	baselines := localFirmwareBaselines(all)
	keys := make([]string, 0, len(baselines))
	for k := range baselines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		res.FirmwareBaselines = append(res.FirmwareBaselines, baselines[k])
	}

	sites := map[string]*localSiteHealth{}
	for _, d := range all {
		if !localSiteMatches(site, d.Site) {
			continue
		}
		res.DevicesChecked++
		siteLabel := localSiteLabel(d.Site)
		if siteLabel == "" {
			siteLabel = "(no site)"
		}
		sh, ok := sites[siteLabel]
		if !ok {
			sh = &localSiteHealth{Site: siteLabel, Counts: localDeviceCounts(), Findings: make([]localDeviceFinding, 0)}
			sites[siteLabel] = sh
		}
		sh.Devices++

		f := localDeviceFinding{
			DeviceID:        d.ID.String(),
			DisplayName:     d.DisplayName.String(),
			MacAddress:      d.MacAddress.String(),
			DeviceType:      d.DeviceType.String(),
			FirmwareVersion: d.FirmwareVersion.String(),
			Status:          d.Status.String(),
			Site:            siteLabel,
			Flags:           make([]string, 0),
			Details:         make([]string, 0),
		}
		flag := func(name, detail string) {
			f.Flags = append(f.Flags, name)
			f.Details = append(f.Details, detail)
		}
		if strings.EqualFold(d.Status.String(), "offline") {
			flag(localFlagOffline, "device reports status offline")
		}
		assignees := localDeviceAssigneeList(d)
		if len(assignees) > 0 {
			f.AssigneeName = assignees[0].Name.String()
			f.AssigneeType = assignees[0].ExtensionType.String()
		}
		if d.localSource == "unassigned" || len(assignees) == 0 {
			flag(localFlagUnassigned, "device has no assignee")
		}
		if b, ok := baselines[localDeviceTypeKey(d)]; ok && b.Baseline != "" && f.FirmwareVersion != "" && f.FirmwareVersion != b.Baseline {
			flag(localFlagFirmwareDrift, fmt.Sprintf("firmware %s differs from %s, the most common version for %s (%d of %d devices)",
				f.FirmwareVersion, b.Baseline, b.DeviceType, b.Versions[b.Baseline], b.Devices))
		}
		for _, a := range assignees {
			typ := strings.ToLower(a.ExtensionType.String())
			if typ == "commonarea" {
				continue
			}
			u, found := usersByID[a.ID.String()]
			switch {
			case found:
				if reason := localUserInactiveReason(u); reason != "" {
					flag(localFlagAssigneeInactive, fmt.Sprintf("assigned to %s, whose phone user is %s", u.Name.String(), strings.ReplaceAll(reason, "_", " ")))
				}
			case typ == "user" && usersSynced && a.ID.String() != "":
				flag(localFlagAssigneeInactive, fmt.Sprintf("assigned to %s (%s), who is not in the synced phone users", a.Name.String(), a.ID.String()))
			}
		}
		if d.ambiguous() {
			flag(localFlagAmbiguous, fmt.Sprintf("device has conflicting %s rows synced at the same time, so its assignment is unknown; re-sync assigned,unassigned",
				strings.Join(d.ambiguousSources, "/")))
		}
		if d.ProvisionTemplateID != nil && d.ProvisionTemplateID.String() == "" {
			flag(localFlagNoTemplate, "provision_template_id is empty (no provisioning template set)")
		}
		if len(f.Flags) == 0 {
			continue
		}
		for _, fl := range f.Flags {
			sh.Counts[fl]++
			res.Counts[fl]++
		}
		sh.Counts["total"]++
		res.Counts["total"]++
		sh.Findings = append(sh.Findings, f)
	}
	names := make([]string, 0, len(sites))
	for n := range sites {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sh := sites[n]
		sort.SliceStable(sh.Findings, func(i, j int) bool {
			if sh.Findings[i].DisplayName != sh.Findings[j].DisplayName {
				return sh.Findings[i].DisplayName < sh.Findings[j].DisplayName
			}
			return sh.Findings[i].DeviceID < sh.Findings[j].DeviceID
		})
		res.BySite = append(res.BySite, *sh)
	}
	return res
}

func newNovelDevicesHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var site string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Flag offline or unassigned desk phones, firmware drift per model and devices assigned to inactive users, grouped by site (local store; --site filters).",
		Long: strings.TrimSpace(`
Flags desk phones from the local store, grouped by site:
  OFFLINE            status is offline
  UNASSIGNED         no user or common area assignee
  FIRMWARE_DRIFT     firmware differs from the most common version for the
                     same device_type (no baseline when the top two tie)
  ASSIGNEE_INACTIVE  assigned to a deactivated or not-ready phone user, or
                     one no longer in the synced phone users
  NO_TEMPLATE        provision_template_id is present but empty
  ASSIGNMENT_AMBIGUOUS
                     the assigned and unassigned lists both hold the device
                     with the same sync time, so its assignment is unknown

counts holds every flag (0 when absent) plus total, the number of flagged
devices. Firmware baselines are computed account-wide before --site filters
the output. Reads the local store only. "assessed" is true only when every
resource it reads has a completed, full, unfiltered sync (zoom-phone-cli
sync --full; a plain sync does not delete removed devices), i.e.
missing_local is empty, with missing_reasons per resource; a --db path that
does not exist is a usage error (exit 2).`),
		Example: strings.Trim(`
  zoom-phone-cli devices health --agent
  zoom-phone-cli devices health --site "San Jose HQ" --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices health")
			}
			res := localNewDevicesHealthResult(site)
			db, err := localOpenStore(cmd, dbPath, localDevicesSyncResources)
			if err != nil {
				return err
			}
			if db != nil {
				// Rows and coverage come from one read snapshot.
				var devices []localDevice
				var users []localUser
				var areas []localCommonArea
				var cov localCoverage
				err := localReadSnapshot(cmd.Context(), db, func(q localQuerier) error {
					var err error
					if devices, err = localLoadDevices(q); err != nil {
						return err
					}
					if users, err = localLoadUsers(q); err != nil {
						return err
					}
					if areas, err = localLoad[localCommonArea](q, "common_areas"); err != nil {
						return err
					}
					cov = localCoverageFor(q, nil, localDevicesSyncResources...)
					return nil
				})
				synced := cov.synced
				_ = db.Close()
				if err != nil {
					return err
				}
				res = localDevicesHealth(devices, users, synced["users"], areas, site)
				// Account-wide health needs BOTH device partitions: assigned
				// and unassigned are separate syncs, and either alone would
				// hide every device in the other.
				missing := localMissing(synced, localDevicesSyncResources...)
				res.MissingLocal = missing
				res.MissingReasons = localMissingReasons(cov, missing)
				// assessed: every resource the health check reads finished syncing.
				res.Assessed = len(missing) == 0
				localHintMissing(cmd, missing, res.MissingReasons)
				if res.DevicesChecked == 0 && site != "" && len(devices) > 0 {
					res.Note = fmt.Sprintf("no devices matched --site %q", site)
				}
				if !res.Assessed {
					res.Note = localNotAssessedNote(missing, res.Note)
				}
			} else {
				res.MissingLocal = append(res.MissingLocal, localDevicesSyncResources...)
				sort.Strings(res.MissingLocal)
				for _, m := range res.MissingLocal {
					res.MissingReasons[m] = "no local store"
				}
				res.Note = "no local store; nothing was assessed; run: " + localSyncCommand(localDevicesSyncResources)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			localWriteNotAssessed(cmd.OutOrStdout(), res.Assessed, res.Note)
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SITE\tDEVICE\tTYPE\tFIRMWARE\tSTATUS\tFLAGS")
			for _, sh := range res.BySite {
				for _, f := range sh.Findings {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", sh.Site, f.DisplayName, f.DeviceType, f.FirmwareVersion, f.Status, strings.Join(f.Flags, ","))
				}
			}
			fmt.Fprintf(w, "\n%d device(s) checked\n", res.DevicesChecked)
			if res.Note != "" && res.Assessed {
				fmt.Fprintf(w, "note: %s\n", res.Note)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: the synced store for the active account)")
	cmd.Flags().StringVar(&site, "site", "", "Only report devices in this site (name or id, case-insensitive)")
	return cmd
}
