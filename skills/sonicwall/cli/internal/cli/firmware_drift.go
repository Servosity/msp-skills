// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
)

type driftRow struct {
	Serial            string   `json:"serial"`
	Name              string   `json:"name"`
	TenantID          string   `json:"tenant_id,omitempty"`
	Current           string   `json:"current"`
	Latest            string   `json:"latest,omitempty"`
	LatestReleaseDate string   `json:"latest_release_date,omitempty"`
	VersionsBehind    *int     `json:"versions_behind,omitempty"`
	Status            string   `json:"status"`          // current, behind, unknown
	AdvisoryStatus    string   `json:"advisory_status"` // pass, fail or unknown for the running build
	AdvisoriesCurrent []string `json:"advisories_on_current"`
	AdvisoriesCleared []string `json:"advisories_cleared_by_latest"`
	Error             string   `json:"error,omitempty"`
}

// advisoryLookup memoises /devices/vulnerableFirmware per build across the
// fan-out; concurrent misses on one build share a single request.
type advisoryLookup struct {
	mu      sync.Mutex
	entries map[string]*advisoryEntry
	c       *client.Client
}

type advisoryEntry struct {
	once sync.Once
	ids  []string
	err  error
}

func (l *advisoryLookup) get(ctx context.Context, fw string) ([]string, error) {
	l.mu.Lock()
	e := l.entries[fw]
	if e == nil {
		e = &advisoryEntry{}
		l.entries[fw] = e
	}
	l.mu.Unlock()
	e.once.Do(func() {
		advs, err := advisoriesForFirmware(ctx, l.c, fw)
		e.ids, e.err = advisoryIDs(advs), err
	})
	return e.ids, e.err
}

func newestReleaseDate(raw json.RawMessage, version string) string {
	var doc struct {
		AvailableVersions []struct {
			Version     string `json:"version"`
			ReleaseDate string `json:"releaseDate"`
		} `json:"availableVersions"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	for _, v := range doc.AvailableVersions {
		if v.Version == version {
			return v.ReleaseDate
		}
	}
	return ""
}

func minus(a, b []string) []string {
	drop := map[string]bool{}
	for _, x := range b {
		drop[x] = true
	}
	out := make([]string, 0)
	for _, x := range a {
		if !drop[x] {
			out = append(out, x)
		}
	}
	return out
}

func newNovelFirmwareDriftCmd(flags *rootFlags) *cobra.Command {
	var tenant string
	var behindOnly bool
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Show how far each firewall's SonicOS build is behind the newest available build and which advisories upgrading clears; --behind-only hides current firewalls",
		Long: "Use this command to see how far each firewall's running firmware is behind the newest available build and which\n" +
			"advisories the upgrade target clears. Do NOT use this command to list firewalls hit by one named advisory\n" +
			"or CVE; use 'exposure' instead.\n\n" +
			"Compares each firewall's running build with NSM's available builds for that serial, and asks NSM which\n" +
			"published advisories apply to the running build and to the newest build. A lookup failure is reported as\n" +
			"status 'unknown', never 'current'; advisory_status is pass, fail or unknown for the running build.",
		Example: strings.Trim(`
  sonicwall-cli firmware drift --agent
  sonicwall-cli firmware drift --behind-only --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "firmware drift")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			devices, err := fetchFleet(ctx, c, tenant, "")
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			devices, _ = capForDogfood(devices)
			if len(devices) == 0 {
				return noFirewallsErr(cmd.OutOrStdout(), flags)
			}
			lookup := &advisoryLookup{entries: map[string]*advisoryEntry{}, c: c}
			results, fails := cliutil.FanoutRun(ctx, devices,
				func(d fleetDevice) string { return d.SerialNumber },
				func(ctx context.Context, d fleetDevice) (driftRow, error) {
					r := driftRow{Serial: d.SerialNumber, Name: d.FriendlyName, TenantID: d.TenantID, Current: d.FirmwareVersion,
						Status: "unknown", AdvisoryStatus: "unknown", AdvisoriesCurrent: make([]string, 0), AdvisoriesCleared: make([]string, 0)}
					var errs []string
					if cur, err := lookup.get(ctx, d.FirmwareVersion); err != nil {
						errs = append(errs, "advisories on running build: "+errText(err))
					} else {
						r.AdvisoriesCurrent = cur
						r.AdvisoryStatus = "pass"
						if len(cur) > 0 {
							r.AdvisoryStatus = "fail"
						}
					}
					raw, err := fetchAvailableVersions(ctx, c, d.SerialNumber)
					if err != nil {
						r.Error = strings.Join(append(errs, "available firmware: "+errText(err)), "; ")
						return r, nil
					}
					versions, ok := posture.AvailableVersions(raw)
					if !ok {
						r.Error = strings.Join(append(errs, "available firmware: unexpected response shape"), "; ")
						return r, nil
					}
					behind, newest, ok := posture.FirmwareGap(d.FirmwareVersion, versions)
					if !ok {
						r.Error = strings.Join(append(errs, "running firmware version not reported by NSM"), "; ")
						return r, nil
					}
					r.VersionsBehind, r.Latest = &behind, newest
					r.LatestReleaseDate = newestReleaseDate(raw, newest)
					r.Status = "current"
					if behind > 0 {
						r.Status = "behind"
						if r.AdvisoryStatus != "unknown" {
							if tgt, err := lookup.get(ctx, newest); err != nil {
								errs = append(errs, "advisories on newest build: "+errText(err))
							} else {
								r.AdvisoriesCleared = minus(r.AdvisoriesCurrent, tgt)
							}
						}
					}
					r.Error = strings.Join(errs, "; ")
					return r, nil
				})
			bySerial := map[string]driftRow{}
			for _, res := range results {
				bySerial[res.Source] = res.Value
			}
			failErr := map[string]error{}
			for _, f := range fails {
				failErr[f.Source] = f.Err
			}
			rows := make([]driftRow, 0, len(devices))
			for _, d := range devices {
				r, done := bySerial[d.SerialNumber]
				if !done {
					r = driftRow{Serial: d.SerialNumber, Name: d.FriendlyName, TenantID: d.TenantID, Current: d.FirmwareVersion,
						Status: "unknown", AdvisoryStatus: "unknown", AdvisoriesCurrent: make([]string, 0), AdvisoriesCleared: make([]string, 0),
						Error: "not checked: " + errText(failErr[d.SerialNumber])}
				}
				// --behind-only hides only rows proven current with a readable advisory check.
				if behindOnly && r.Status == "current" && r.AdvisoryStatus != "unknown" && r.Error == "" {
					continue
				}
				rows = append(rows, r)
			}
			partial := interrupted(ctx)
			if partial == nil && len(fails) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+fanoutFailureNote(fails, len(devices)))
				partial = partialFailureErr(fmt.Errorf("%s", fanoutFailureNote(fails, len(devices))))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
					return err
				}
				return partial
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No firewalls to report.")
				return partial
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				behind := "?"
				if r.VersionsBehind != nil {
					behind = fmt.Sprint(*r.VersionsBehind)
				}
				table = append(table, map[string]any{"serial": r.Serial, "name": r.Name, "current": r.Current, "latest": r.Latest,
					"behind": behind, "status": r.Status, "advisories": r.AdvisoryStatus, "clears": strings.Join(r.AdvisoriesCleared, ",")})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"serial", "name", "current", "latest", "behind", "status", "advisories", "clears"}, table); err != nil {
				return err
			}
			return partial
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Only firewalls in this NSM tenant ID")
	cmd.Flags().BoolVar(&behindOnly, "behind-only", false, "Hide firewalls already on the newest build")
	return cmd
}
