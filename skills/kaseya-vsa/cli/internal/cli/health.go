// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

var healthChecks = []string{"offline", "av", "firewall", "uac", "disk", "cpu", "memory"}

type healthRow struct {
	DeviceID     string `json:"device_id"`
	Device       string `json:"device"`
	Organization string `json:"organization"`
	Site         string `json:"site"`
	Check        string `json:"check"`
	State        string `json:"state"` // "fail" or "unknown"
	Value        string `json:"value"`
	Detail       string `json:"detail"`
}

// healthView keeps the scan size next to the rows: an empty result from an
// empty or unsynced mirror must never read as an all-clear.
type healthView struct {
	Checks         []string                  `json:"checks"`
	ScannedDevices int                       `json:"scanned_devices"`
	FailingDevices int                       `json:"failing_devices"`
	Counts         map[string]map[string]int `json:"counts"`
	Results        any                       `json:"results"`
	Note           string                    `json:"note,omitempty"`
}

const healthUnsyncedNote = "no synced assets: run 'kaseya-vsa-cli sync --resources organizations,sites,assets' first; an empty result here is not an all-clear"

type healthOrgRow struct {
	OrganizationID int64          `json:"organization_id"`
	Organization   string         `json:"organization"`
	Devices        int            `json:"devices"`
	FailingDevices int            `json:"failing_devices"`
	Failures       map[string]int `json:"failures"`
	Unknown        int            `json:"unknown"`
}

func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var flagCheck, flagOfflineFor, flagOrg, flagBy, dbPath string
	var diskBelow, cpuAbove, memAbove float64
	var failOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "health",
		Short: "List devices across every client failing health checks: offline, antivirus, firewall, UAC, disk, CPU, memory",
		Long: `Lists every synced device that fails a health or security-posture check, one row per (device, check),
with scanned_devices, failing_devices and per-check fail/unknown counts beside the rows. Reads the local
mirror only: run 'kaseya-vsa-cli sync --resources organizations,sites,assets' first; with nothing
synced, scanned_devices is 0 and a note says so.

Checks: offline (LastSeenOnline older than --offline-for, measured from when the row was synced, so a
stale mirror does not report the whole fleet offline), av (antivirus disabled or out of date),
firewall, uac, disk (any disk below --disk-below percent free), cpu, memory. A check whose value is
missing or unrecognized is reported as state "unknown", never as a pass; --fail-only hides those.

Use this command for fleet-wide lists of devices failing health or security-posture checks (offline,
antivirus, firewall, UAC, low disk, high CPU or memory). Do NOT use this command for pending OS
updates or KB/CVE exposure; use 'patch-exposure' instead. Do NOT use it for the full picture of one
device; use 'devices brief' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli health --check offline,av,firewall --offline-for 72h --agent
  kaseya-vsa-cli health --check offline --offline-for 7d --by org --agent
  kaseya-vsa-cli health --check disk --disk-below 15 --org "Acme Dental"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "health")
			}
			checks := healthChecks
			if cmd.Flags().Changed("check") {
				checks = splitCSV(flagCheck)
				if len(checks) == 0 {
					return usageErr(fmt.Errorf("--check names no check (valid: %s)", strings.Join(healthChecks, ",")))
				}
				unique := make([]string, 0, len(checks))
				for _, c := range checks {
					if !containsString(healthChecks, c) {
						return usageErr(fmt.Errorf("unknown --check %q (valid: %s)", c, strings.Join(healthChecks, ",")))
					}
					if !containsString(unique, c) { // --check disk,disk must not double-count
						unique = append(unique, c)
					}
				}
				checks = unique
			}
			offlineFor, err := cliutil.ParseDurationLoose(flagOfflineFor)
			if err != nil || offlineFor <= 0 {
				return usageErr(fmt.Errorf("invalid --offline-for %q (use e.g. 72h, 7d, 2w)", flagOfflineFor))
			}
			if flagBy != "" && flagBy != "org" {
				return usageErr(fmt.Errorf("invalid --by %q (only \"org\" is supported)", flagBy))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(cmd, flags, dbPath, "organizations", "sites", "assets")
			if err != nil {
				return err
			}
			view := healthView{Checks: checks, Counts: map[string]map[string]int{}, Results: make([]healthRow, 0)}
			for _, c := range checks {
				view.Counts[c] = map[string]int{"fail": 0, "unknown": 0}
			}
			if db == nil {
				view.Note = healthUnsyncedNote
				if flagBy == "org" {
					view.Results = make([]healthOrgRow, 0)
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer db.Close()
			orgs, err := loadNameIndex(ctx, db, "organizations")
			if err != nil {
				return err
			}
			sites, err := loadNameIndex(ctx, db, "sites")
			if err != nil {
				return err
			}
			assets, err := loadFleetAssets(ctx, db)
			if err != nil {
				return err
			}
			opts := healthOptions{checks: checks, offlineFor: offlineFor, diskBelow: diskBelow, cpuAbove: cpuAbove, memAbove: memAbove, now: time.Now().UTC()}
			rows := make([]healthRow, 0)
			byOrg := map[int64]*healthOrgRow{}
			for _, a := range assets {
				orgName := orgs[a.OrganizationID]
				if !orgFilter(flagOrg, a.OrganizationID, orgName) {
					continue
				}
				o := byOrg[a.OrganizationID]
				if o == nil {
					o = &healthOrgRow{OrganizationID: a.OrganizationID, Organization: orgName, Failures: map[string]int{}}
					byOrg[a.OrganizationID] = o
				}
				o.Devices++
				view.ScannedDevices++
				failing := false
				for _, r := range evaluateHealth(a, opts) {
					view.Counts[r.Check][r.State]++
					if r.State == "unknown" {
						o.Unknown++
						if failOnly {
							continue
						}
					} else {
						o.Failures[r.Check]++
						failing = true
					}
					r.Organization, r.Site = orgName, sites[a.SiteID]
					rows = append(rows, r)
				}
				if failing {
					o.FailingDevices++
					view.FailingDevices++
				}
			}
			if view.ScannedDevices == 0 {
				view.Note = healthUnsyncedNote
				if len(assets) > 0 {
					view.Note = "no synced device matched --org " + strconv.Quote(flagOrg)
				}
			}
			if flagBy == "org" {
				out := make([]healthOrgRow, 0, len(byOrg))
				for _, o := range byOrg {
					out = append(out, *o)
				}
				sort.Slice(out, func(i, j int) bool {
					if out[i].FailingDevices != out[j].FailingDevices {
						return out[i].FailingDevices > out[j].FailingDevices
					}
					return out[i].Organization < out[j].Organization
				})
				view.Results = out
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				if len(out) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No synced devices matched. "+view.Note)
					return nil
				}
				for _, o := range out {
					fmt.Fprintf(cmd.OutOrStdout(), "%-30s devices=%-5d failing=%-5d %v unknown=%d\n", o.Organization, o.Devices, o.FailingDevices, o.Failures, o.Unknown)
				}
				return nil
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Organization != rows[j].Organization {
					return rows[i].Organization < rows[j].Organization
				}
				if rows[i].Device != rows[j].Device {
					return rows[i].Device < rows[j].Device
				}
				return rows[i].Check < rows[j].Check
			})
			if limit > 0 && len(rows) > limit {
				fmt.Fprintf(cmd.ErrOrStderr(), "showing %d of %d rows; raise --limit to see all\n", limit, len(rows))
				rows = rows[:limit]
			}
			view.Results = rows
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "scanned %d devices, %d failing\n", view.ScannedDevices, view.FailingDevices)
			if len(rows) == 0 {
				if view.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				return nil
			}
			for _, r := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-20s %-9s %-7s %s\n", r.Device, r.Organization, r.Check, r.State, r.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagCheck, "check", "", "Comma-separated checks to run: offline,av,firewall,uac,disk,cpu,memory (default all)")
	cmd.Flags().StringVar(&flagOfflineFor, "offline-for", "7d", "Report a device offline when it was last seen longer ago than this (e.g. 72h, 7d)")
	cmd.Flags().Float64Var(&diskBelow, "disk-below", 10, "Report a disk with less free space than this percentage")
	cmd.Flags().Float64Var(&cpuAbove, "cpu-above", 90, "Report CPU usage at or above this percentage")
	cmd.Flags().Float64Var(&memAbove, "memory-above", 90, "Report memory usage at or above this percentage")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Only devices in this organization (numeric ID or name substring)")
	cmd.Flags().StringVar(&flagBy, "by", "", "Roll results up by \"org\" instead of one row per device and check")
	cmd.Flags().BoolVar(&failOnly, "fail-only", false, "Hide checks whose value is missing or unrecognized (state \"unknown\")")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the store sync writes)")
	return cmd
}

type healthOptions struct {
	checks                        []string
	offlineFor                    time.Duration
	diskBelow, cpuAbove, memAbove float64
	now                           time.Time
}

// evaluateHealth returns the failing and unknown checks for one asset.
func evaluateHealth(a *fleetAsset, o healthOptions) []healthRow {
	out := make([]healthRow, 0)
	add := func(check, state, value, detail string) {
		out = append(out, healthRow{DeviceID: a.Identifier, Device: a.Name, Check: check, State: state, Value: value, Detail: detail})
	}
	for _, check := range o.checks {
		switch check {
		case "offline":
			seen, ok := parseVSATime(a.LastSeenOnline)
			if !ok {
				add(check, "unknown", a.LastSeenOnline, "LastSeenOnline missing or unparseable")
				continue
			}
			ref := a.syncedAt
			if ref.IsZero() {
				ref = o.now
			}
			if age := ref.Sub(seen); age > o.offlineFor {
				add(check, "fail", a.LastSeenOnline, fmt.Sprintf("last seen %s before sync", roundAge(age)))
			}
		case "av":
			switch v := lowerPtr(a.AntivirusEnabled); v {
			case "enabled", "true", "yes", "on":
			case "disabled", "false", "no", "off":
				add(check, "fail", v, "antivirus disabled")
				continue
			default:
				add(check, "unknown", v, unrecognized("antivirus state", v))
				continue
			}
			switch v := lowerPtr(a.AntivirusUpToDate); v {
			case "yes", "true", "up_to_date", "uptodate":
			case "no", "false", "out_of_date", "outdated":
				add(check, "fail", v, "antivirus definitions out of date")
			default:
				add(check, "unknown", v, unrecognized("antivirus definition state", v))
			}
		case "firewall":
			boolCheck(a.FirewallEnabled, check, "firewall disabled", add)
		case "uac":
			boolCheck(a.UacEnabled, check, "UAC disabled", add)
		case "disk":
			if len(a.Disks) == 0 {
				add(check, "unknown", "", "no disk data synced (sync assets with the Disks section)")
				continue
			}
			// One row per device and check: low disks combine; a missing value only makes the
			// check unknown when no disk failed outright.
			var low, missing []string
			lowest := -1.0
			for _, d := range a.Disks {
				switch {
				case d.FreePercentage == nil:
					missing = append(missing, d.Name)
				case *d.FreePercentage < o.diskBelow:
					low = append(low, fmt.Sprintf("%s %g%% free", d.Name, *d.FreePercentage))
					if lowest < 0 || *d.FreePercentage < lowest {
						lowest = *d.FreePercentage
					}
				}
			}
			switch {
			case len(low) > 0:
				add(check, "fail", fmt.Sprintf("%g", lowest), "low disk: "+strings.Join(low, ", "))
			case len(missing) > 0:
				add(check, "unknown", "", "no free-space value for disk "+strings.Join(missing, ", "))
			}
		case "cpu":
			pctCheck(a.CPUUsage, o.cpuAbove, check, "CPU", add)
		case "memory":
			pctCheck(a.MemoryUsage, o.memAbove, check, "memory", add)
		}
	}
	return out
}

func boolCheck(v *bool, check, failDetail string, add func(check, state, value, detail string)) {
	switch {
	case v == nil:
		add(check, "unknown", "", check+" state not reported")
	case !*v:
		add(check, "fail", "false", failDetail)
	}
}

func pctCheck(v *float64, threshold float64, check, label string, add func(check, state, value, detail string)) {
	switch {
	case v == nil:
		add(check, "unknown", "", label+" usage not reported")
	case *v >= threshold:
		add(check, "fail", fmt.Sprintf("%g", *v), fmt.Sprintf("%s at %g%%", label, *v))
	}
}

func unrecognized(what, v string) string {
	if v == "" {
		return what + " not reported"
	}
	return fmt.Sprintf("%s %q not recognized", what, v)
}

func lowerPtr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*s))
}

func roundAge(d time.Duration) string {
	if d >= 48*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
