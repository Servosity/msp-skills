// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type patchKBRow struct {
	KB               string   `json:"kb"`
	Title            string   `json:"title"`
	Severity         string   `json:"severity"`
	MaxCvss          *float64 `json:"max_cvss"`
	Cves             []string `json:"cves"`
	Devices          int      `json:"devices"`
	Organizations    int      `json:"organizations"`
	ReleaseDate      string   `json:"release_date"`
	DaysSinceRelease *int     `json:"days_since_release"`
}

type patchOrgRow struct {
	OrganizationID int64    `json:"organization_id"`
	Organization   string   `json:"organization"`
	DevicesExposed int      `json:"devices_exposed"`
	PendingUpdates int      `json:"pending_updates"`
	Critical       int      `json:"critical"`
	Important      int      `json:"important"`
	MaxCvss        *float64 `json:"max_cvss"`
}

type patchDeviceRow struct {
	DeviceID       string   `json:"device_id"`
	Device         string   `json:"device"`
	Organization   string   `json:"organization"`
	PendingUpdates int      `json:"pending_updates"`
	Critical       int      `json:"critical"`
	Important      int      `json:"important"`
	MaxCvss        *float64 `json:"max_cvss"`
	KBs            []string `json:"kbs"`
}

// patchExposureView keeps the "no data" count next to the results: a device whose
// AvailableUpdates section was never synced is not a patched device.
type patchExposureView struct {
	By                  string `json:"by"`
	Sort                string `json:"sort"`
	Results             any    `json:"results"`
	ScannedDevices      int    `json:"scanned_devices"`
	DevicesWithoutData  int    `json:"devices_without_update_data"`
	DevicesWithFindings int    `json:"devices_with_findings"`
	Note                string `json:"note,omitempty"`
}

func newNovelPatchExposureCmd(flags *rootFlags) *cobra.Command {
	var flagBy, flagSeverity, flagKB, flagCVE, flagOrg, flagSort, dbPath string
	var minCvss float64
	var limit int

	cmd := &cobra.Command{
		Use:   "patch-exposure",
		Short: "Rank pending OS updates across all clients by KB, client, or device, filtered by severity, CVSS, KB, or CVE.",
		Long: `Unrolls every synced device's pending OS updates (the asset AvailableUpdates section) and ranks
exposure across the whole fleet: by KB (devices missing it, organizations affected, max CVSS, days
since release), by organization, or by device. Rows rank by how many devices are exposed, ties broken
by CVSS; --sort cvss ranks by the highest CVSS score first. Reads the local mirror only: run
'kaseya-vsa-cli sync --resources organizations,assets' first.

A device whose AvailableUpdates section was never synced counts in devices_without_update_data and
is never treated as patched; an empty list means the device reported no pending updates.

Use this command for pending OS update exposure across the fleet, by KB, by organization or by
device, filtered by severity, CVSS, KB or CVE. Do NOT use this command for third-party application
versions; use 'software' instead. Do NOT use it for antivirus, firewall or disk posture; use
'health' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli patch-exposure --by kb --severity critical,important --min-cvss 7 --agent
  kaseya-vsa-cli patch-exposure --by org --severity critical
  kaseya-vsa-cli patch-exposure --by device --cve CVE-2026-41001 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "patch-exposure")
			}
			if flagBy == "" {
				flagBy = "kb"
			}
			if flagBy != "kb" && flagBy != "org" && flagBy != "device" {
				return usageErr(fmt.Errorf("invalid --by %q (use kb, org, or device)", flagBy))
			}
			if flagSort != "devices" && flagSort != "cvss" {
				return usageErr(fmt.Errorf("invalid --sort %q (use devices or cvss)", flagSort))
			}
			severities, err := parseSeverities(flagSeverity)
			if err != nil {
				return usageErr(err)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := patchExposureView{By: flagBy, Sort: flagSort}
			db, err := openFleetStore(cmd, flags, dbPath, "organizations", "assets")
			if err != nil {
				return err
			}
			if db == nil {
				view.Results = make([]any, 0)
				view.Note = "no synced assets: run 'kaseya-vsa-cli sync --resources organizations,assets' first; an empty result here is not an all-clear"
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer db.Close()
			orgs, err := loadNameIndex(ctx, db, "organizations")
			if err != nil {
				return err
			}
			assets, err := loadFleetAssets(ctx, db)
			if err != nil {
				return err
			}
			filter := patchFilter{severities: severities, minCvss: minCvss, sortCvss: flagSort == "cvss",
				kb: normalizeKB(flagKB), cve: strings.ToUpper(strings.TrimSpace(flagCVE))}
			view.Results, view.ScannedDevices, view.DevicesWithoutData, view.DevicesWithFindings =
				rankPatchExposure(assets, orgs, flagOrg, flagBy, filter, time.Now().UTC())
			switch {
			case view.ScannedDevices == 0:
				view.Note = "no synced assets: run 'kaseya-vsa-cli sync --resources organizations,assets' first; an empty result here is not an all-clear"
			case view.DevicesWithoutData == view.ScannedDevices:
				view.Note = "no synced device has an AvailableUpdates section; run 'kaseya-vsa-cli sync --resources assets'"
			}
			view.Results = limitResults(view.Results, limit)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			return printPatchHuman(cmd, view)
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "kb", "Group results by kb, org, or device")
	cmd.Flags().StringVar(&flagSort, "sort", "devices", "Rank by number of devices exposed (devices) or by highest CVSS score (cvss)")
	cmd.Flags().StringVar(&flagSeverity, "severity", "", "Only these update severities, comma-separated (e.g. critical,important)")
	cmd.Flags().Float64Var(&minCvss, "min-cvss", 0, "Only updates with a CVSS score at or above this value")
	cmd.Flags().StringVar(&flagKB, "kb", "", "Only this KB article (e.g. KB5066835 or 5066835)")
	cmd.Flags().StringVar(&flagCVE, "cve", "", "Only updates that fix this CVE (e.g. CVE-2026-41001)")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Only devices in this organization (numeric ID or name substring)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum result rows (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the store sync writes)")
	return cmd
}

type patchFilter struct {
	severities []string
	minCvss    float64
	kb, cve    string
	sortCvss   bool // rank by max CVSS first instead of by device count
}

var patchSeverities = map[string]string{"critical": "critical", "crit": "critical", "important": "important",
	"moderate": "moderate", "low": "low", "unspecified": "unspecified"}

// parseSeverities validates --severity so a typo cannot silently filter everything out.
func parseSeverities(csv string) ([]string, error) {
	out := make([]string, 0)
	for _, v := range splitCSV(csv) {
		s, ok := patchSeverities[v]
		if !ok {
			return nil, fmt.Errorf("unknown --severity %q (use critical, important, moderate, low, unspecified)", v)
		}
		out = append(out, s)
	}
	return out, nil
}

func normalizeKB(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.TrimPrefix(s, "KB")
}

func (f patchFilter) match(u fleetUpdate) bool {
	if len(f.severities) > 0 && !containsString(f.severities, strings.ToLower(u.Severity)) {
		return false
	}
	if f.minCvss > 0 && (u.CvssScore == nil || *u.CvssScore < f.minCvss) {
		return false
	}
	if f.kb != "" && normalizeKB(u.KbArticle) != f.kb {
		return false
	}
	if f.cve != "" {
		found := false
		for _, c := range u.CveCodes {
			if strings.EqualFold(strings.TrimSpace(c), f.cve) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func maxPtr(cur *float64, v *float64) *float64 {
	if v == nil {
		return cur
	}
	if cur == nil || *v > *cur {
		x := *v
		return &x
	}
	return cur
}

// rankPatchExposure returns the grouped rows plus scanned / no-data / with-findings device counts.
func rankPatchExposure(assets []*fleetAsset, orgs map[int64]string, orgFilterValue, by string, f patchFilter, now time.Time) (any, int, int, int) {
	kbs := map[string]*patchKBRow{}
	kbOrgs := map[string]map[int64]bool{}
	kbDevices := map[string]map[string]bool{} // distinct devices per KB: one device can list a KB twice
	byOrg := map[int64]*patchOrgRow{}
	devices := make([]patchDeviceRow, 0)
	scanned, noData, withFindings := 0, 0, 0
	for _, a := range assets {
		orgName := orgs[a.OrganizationID]
		if !orgFilter(orgFilterValue, a.OrganizationID, orgName) {
			continue
		}
		scanned++
		if !a.hasUpdateData() {
			noData++
			continue
		}
		dev := patchDeviceRow{DeviceID: a.Identifier, Device: a.Name, Organization: orgName, KBs: make([]string, 0)}
		for _, u := range a.updates {
			if !f.match(u) {
				continue
			}
			key := normalizeKB(u.KbArticle)
			if key == "" {
				key = u.Title
			}
			k := kbs[key]
			if k == nil {
				k = &patchKBRow{KB: u.KbArticle, Title: u.Title, Severity: u.Severity, ReleaseDate: u.ReleaseDate, Cves: make([]string, 0)}
				if rel, ok := parseVSATime(u.ReleaseDate); ok {
					days := int(now.Sub(rel).Hours() / 24)
					k.DaysSinceRelease = &days
				}
				kbs[key] = k
				kbOrgs[key] = map[int64]bool{}
				kbDevices[key] = map[string]bool{}
			}
			if !kbDevices[key][a.Identifier] {
				kbDevices[key][a.Identifier] = true
				k.Devices++
			}
			k.MaxCvss = maxPtr(k.MaxCvss, u.CvssScore)
			for _, c := range u.CveCodes {
				if !containsString(k.Cves, c) {
					k.Cves = append(k.Cves, c)
				}
			}
			kbOrgs[key][a.OrganizationID] = true
			dev.PendingUpdates++
			dev.MaxCvss = maxPtr(dev.MaxCvss, u.CvssScore)
			switch strings.ToLower(u.Severity) {
			case "critical":
				dev.Critical++
			case "important":
				dev.Important++
			}
			if kb := normalizeKB(u.KbArticle); kb != "" {
				dev.KBs = append(dev.KBs, "KB"+kb) // one format, no empty entries
			} else {
				dev.KBs = append(dev.KBs, u.Title)
			}
		}
		if dev.PendingUpdates == 0 {
			continue
		}
		withFindings++
		devices = append(devices, dev)
		o := byOrg[a.OrganizationID]
		if o == nil {
			o = &patchOrgRow{OrganizationID: a.OrganizationID, Organization: orgName}
			byOrg[a.OrganizationID] = o
		}
		o.DevicesExposed++
		o.PendingUpdates += dev.PendingUpdates
		o.Critical += dev.Critical
		o.Important += dev.Important
		o.MaxCvss = maxPtr(o.MaxCvss, dev.MaxCvss)
	}
	cvss := func(p *float64) float64 {
		if p == nil {
			return -1
		}
		return *p
	}
	switch by {
	case "org":
		out := make([]patchOrgRow, 0, len(byOrg))
		for _, o := range byOrg {
			out = append(out, *o)
		}
		sort.Slice(out, func(i, j int) bool {
			if f.sortCvss && cvss(out[i].MaxCvss) != cvss(out[j].MaxCvss) {
				return cvss(out[i].MaxCvss) > cvss(out[j].MaxCvss)
			}
			if out[i].DevicesExposed != out[j].DevicesExposed {
				return out[i].DevicesExposed > out[j].DevicesExposed
			}
			if cvss(out[i].MaxCvss) != cvss(out[j].MaxCvss) {
				return cvss(out[i].MaxCvss) > cvss(out[j].MaxCvss)
			}
			return out[i].Organization < out[j].Organization
		})
		return out, scanned, noData, withFindings
	case "device":
		sort.Slice(devices, func(i, j int) bool {
			if cvss(devices[i].MaxCvss) != cvss(devices[j].MaxCvss) {
				return cvss(devices[i].MaxCvss) > cvss(devices[j].MaxCvss)
			}
			if devices[i].PendingUpdates != devices[j].PendingUpdates {
				return devices[i].PendingUpdates > devices[j].PendingUpdates
			}
			return devices[i].Device < devices[j].Device
		})
		return devices, scanned, noData, withFindings
	default:
		out := make([]patchKBRow, 0, len(kbs))
		for key, k := range kbs {
			k.Organizations = len(kbOrgs[key])
			out = append(out, *k)
		}
		sort.Slice(out, func(i, j int) bool {
			byDev := func() (bool, bool) {
				return out[i].Devices > out[j].Devices, out[i].Devices != out[j].Devices
			}
			byCvss := func() (bool, bool) {
				return cvss(out[i].MaxCvss) > cvss(out[j].MaxCvss), cvss(out[i].MaxCvss) != cvss(out[j].MaxCvss)
			}
			first, second := byDev, byCvss
			if f.sortCvss {
				first, second = byCvss, byDev
			}
			if less, differ := first(); differ {
				return less
			}
			if less, differ := second(); differ {
				return less
			}
			return out[i].KB < out[j].KB
		})
		return out, scanned, noData, withFindings
	}
}

// limitResults truncates any of the result slice types to n (0 = no limit).
func limitResults(v any, n int) any {
	if n <= 0 {
		return v
	}
	switch s := v.(type) {
	case []patchKBRow:
		if len(s) > n {
			return s[:n]
		}
	case []patchOrgRow:
		if len(s) > n {
			return s[:n]
		}
	case []patchDeviceRow:
		if len(s) > n {
			return s[:n]
		}
	}
	return v
}

func printPatchHuman(cmd *cobra.Command, v patchExposureView) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "scanned %d devices, %d with matching pending updates, %d without update data\n", v.ScannedDevices, v.DevicesWithFindings, v.DevicesWithoutData)
	cv := func(p *float64) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprintf("%.1f", *p)
	}
	switch rows := v.Results.(type) {
	case []patchKBRow:
		for _, r := range rows {
			kb := "-"
			if n := normalizeKB(r.KB); n != "" {
				kb = "KB" + n
			}
			fmt.Fprintf(w, "%-12s %-10s cvss=%-4s devices=%-5d orgs=%-3d %s\n", kb, r.Severity, cv(r.MaxCvss), r.Devices, r.Organizations, r.Title)
		}
	case []patchOrgRow:
		for _, r := range rows {
			fmt.Fprintf(w, "%-30s exposed=%-5d critical=%-5d important=%-5d cvss=%s\n", r.Organization, r.DevicesExposed, r.Critical, r.Important, cv(r.MaxCvss))
		}
	case []patchDeviceRow:
		for _, r := range rows {
			fmt.Fprintf(w, "%-24s %-24s pending=%-3d cvss=%-4s %s\n", r.Device, r.Organization, r.PendingUpdates, cv(r.MaxCvss), strings.Join(r.KBs, ","))
		}
	}
	return nil
}
