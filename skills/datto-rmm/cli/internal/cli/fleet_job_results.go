// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: fleet-wide quick-job verification. The API only
// returns job results per device (/v2/job/{jobUid}/results/{deviceUid}); this
// command fans those calls out across a device cohort and rolls them into one
// pass/fail table, with partial-failure accounting.
package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"datto-rmm-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type jobComponentResult struct {
	ComponentName    string `json:"componentName"`
	ComponentStatus  string `json:"componentStatus"`
	NumberOfWarnings int    `json:"numberOfWarnings"`
	HasStdOut        bool   `json:"hasStdOut"`
	HasStdErr        bool   `json:"hasStdErr"`
}

type jobDeviceResult struct {
	DeviceUID  string               `json:"deviceUid"`
	Hostname   string               `json:"hostname"`
	SiteName   string               `json:"siteName"`
	Status     string               `json:"status"`
	Verdict    string               `json:"verdict"`
	RanOn      string               `json:"ranOn,omitempty"`
	Components []jobComponentResult `json:"components,omitempty"`
}

type jobResultsFailure struct {
	DeviceUID string `json:"deviceUid"`
	Error     string `json:"error"`
}

type jobResultsView struct {
	JobUID         string              `json:"jobUid"`
	Total          int                 `json:"total"`
	Passed         int                 `json:"passed"`
	Warnings       int                 `json:"warnings"`
	Failed         int                 `json:"failed"`
	Running        int                 `json:"running"`
	NoResult       int                 `json:"noResult"`
	ScannedDevices int                 `json:"scanned_devices"`
	Results        []jobDeviceResult   `json:"results"`
	FetchFailures  []jobResultsFailure `json:"fetch_failures,omitempty"`
	Note           string              `json:"note,omitempty"`
}

// classifyJobVerdict maps Datto's jobDeploymentStatus enum to a rollup verdict.
func classifyJobVerdict(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return "pass"
	case "warning":
		return "warning"
	case "failure", "expired":
		return "fail"
	case "pending", "running":
		return "running"
	case "":
		return "no-result"
	default:
		return "fail"
	}
}

// buildJobRollup aggregates per-device results into the fleet view. Results
// are sorted: failures first, then warnings, running, no-result, passes.
func buildJobRollup(jobUID string, results []jobDeviceResult, failures []jobResultsFailure, scanned int, failedOnly bool) jobResultsView {
	view := jobResultsView{
		JobUID:         jobUID,
		ScannedDevices: scanned,
		Results:        []jobDeviceResult{},
		FetchFailures:  failures,
	}
	rank := map[string]int{"fail": 0, "warning": 1, "running": 2, "no-result": 3, "pass": 4}
	sorted := append([]jobDeviceResult(nil), results...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if rank[sorted[i].Verdict] != rank[sorted[j].Verdict] {
			return rank[sorted[i].Verdict] < rank[sorted[j].Verdict]
		}
		return sorted[i].Hostname < sorted[j].Hostname
	})
	for _, r := range sorted {
		view.Total++
		switch r.Verdict {
		case "pass":
			view.Passed++
		case "warning":
			view.Warnings++
		case "fail":
			view.Failed++
		case "running":
			view.Running++
		case "no-result":
			view.NoResult++
		}
		if failedOnly && r.Verdict == "pass" {
			continue
		}
		view.Results = append(view.Results, r)
	}
	return view
}

// pp:data-source live
func newNovelFleetJobResultsCmd(flags *rootFlags) *cobra.Command {
	var devicesCSV string
	var siteName string
	var allDevices bool
	var maxScan int
	var failedOnly bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "job-results <jobUid>",
		Short: "Roll every device's result for one quick job into a single pass/fail table",
		Long: strings.TrimSpace(`
Use this command to verify a fleet-wide quick-job push as one pass/fail table.
Do NOT use it for one device's raw output; use 'job results stdout' / 'job results stderr' instead.
Do NOT use it to launch a job; use 'device quickjob create-quick-job' instead.

The device cohort comes from --devices (explicit UIDs), --site (every synced
device in a site), or --all (every synced device, capped by --max-scan).
Results are fetched live per device; the local store supplies the cohort.`),
		Example: `  datto-rmm-cli fleet job-results 8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f --site "Acme Corporation" --failed-only
  datto-rmm-cli fleet job-results 8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f --devices 0a1b2c3d-uid1,4e5f6a7b-uid2 --json`,
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "jobUid=8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f;--devices=0a1b2c3d-example-device",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch per-device results for the job and roll them into one pass/fail table")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<jobUid> is required"))
			}
			jobUID := args[0]

			// Resolve the cohort.
			var cohort []fleetDevice
			switch {
			case devicesCSV != "":
				for _, uid := range strings.Split(devicesCSV, ",") {
					uid = strings.TrimSpace(uid)
					if uid != "" {
						cohort = append(cohort, fleetDevice{UID: uid, Hostname: uid})
					}
				}
			case siteName != "" || allDevices:
				db, err := openFleetStore(cmd, dbPath)
				if err != nil {
					return err
				}
				defer db.Close()
				if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
					hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
				}
				devices, err := loadFleetDevices(cmd.Context(), db)
				if err != nil {
					return err
				}
				for _, d := range devices {
					if d.Deleted || d.UID == "" {
						continue
					}
					if siteName != "" && !strings.EqualFold(d.SiteName, siteName) {
						continue
					}
					cohort = append(cohort, d)
				}
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("name the device cohort: --devices <csv>, --site <name>, or --all"))
			}

			if len(cohort) == 0 {
				return flags.printJSON(cmd, buildJobRollup(jobUID, nil, []jobResultsFailure{}, 0, failedOnly))
			}

			// Bound scan effort separately from output size.
			capN := maxScan
			if cliutil.IsDogfoodEnv() && capN > 3 {
				capN = 3
			}
			scanCapHit := false
			if capN > 0 && len(cohort) > capN {
				cohort = cohort[:capN]
				scanCapHit = true
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			type fetchOut struct {
				idx    int
				result jobDeviceResult
				err    error
			}
			outs := make(chan fetchOut, len(cohort))
			sem := make(chan struct{}, 8)
			var wg sync.WaitGroup
			for idx, dev := range cohort {
				wg.Add(1)
				go func(idx int, dev fleetDevice) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					path := "/v2/job/" + url.PathEscape(jobUID) + "/results/" + url.PathEscape(dev.UID)
					raw, err := c.Get(cmd.Context(), path, nil)
					if err != nil {
						outs <- fetchOut{idx: idx, err: err, result: jobDeviceResult{DeviceUID: dev.UID, Hostname: dev.Hostname, SiteName: dev.SiteName}}
						return
					}
					var parsed struct {
						DeviceUID           string               `json:"deviceUid"`
						RanOn               json.RawMessage      `json:"ranOn"`
						JobDeploymentStatus string               `json:"jobDeploymentStatus"`
						ComponentResults    []jobComponentResult `json:"componentResults"`
					}
					r := jobDeviceResult{DeviceUID: dev.UID, Hostname: dev.Hostname, SiteName: dev.SiteName}
					if err := json.Unmarshal(raw, &parsed); err != nil {
						r.Verdict = "no-result"
					} else {
						r.Status = parsed.JobDeploymentStatus
						r.Verdict = classifyJobVerdict(parsed.JobDeploymentStatus)
						r.Components = parsed.ComponentResults
						if t, ok := parseDattoTime(parsed.RanOn); ok {
							r.RanOn = t.Format("2006-01-02T15:04:05Z07:00")
						}
					}
					outs <- fetchOut{idx: idx, result: r}
				}(idx, dev)
			}
			go func() { wg.Wait(); close(outs) }()

			results := make([]jobDeviceResult, 0, len(cohort))
			failures := []jobResultsFailure{}
			for o := range outs {
				if o.err != nil {
					// A 404-shaped miss means the job never targeted this
					// device — that's a no-result row, not a fetch failure.
					if strings.Contains(o.err.Error(), "404") {
						o.result.Verdict = "no-result"
						results = append(results, o.result)
						continue
					}
					failures = append(failures, jobResultsFailure{DeviceUID: o.result.DeviceUID, Error: o.err.Error()})
					continue
				}
				results = append(results, o.result)
			}

			view := buildJobRollup(jobUID, results, failures, len(cohort), failedOnly)
			if scanCapHit {
				view.Note = fmt.Sprintf("cohort capped at %d devices; raise --max-scan to widen the verification", capN)
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d result fetches failed; rollup computed over the remaining %d devices\n",
					len(failures), len(cohort), view.Total)
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "STATUS", "VERDICT"}
			rows := make([][]string, 0, len(view.Results))
			for _, r := range view.Results {
				rows = append(rows, []string{r.Hostname, r.SiteName, r.Status, r.Verdict})
			}
			if err := flags.printTable(cmd, headers, rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d pass, %d warning, %d fail, %d running, %d no-result (of %d devices)\n",
				view.Passed, view.Warnings, view.Failed, view.Running, view.NoResult, view.Total)
			return nil
		},
	}
	cmd.Flags().StringVar(&devicesCSV, "devices", "", "Comma-separated device UIDs the job targeted")
	cmd.Flags().StringVar(&siteName, "site", "", "Verify against every synced device in this site")
	cmd.Flags().BoolVar(&allDevices, "all", false, "Verify against every synced device (capped by --max-scan)")
	cmd.Flags().IntVar(&maxScan, "max-scan", 100, "Maximum devices to fetch results for (0 = unlimited)")
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "Only list devices that did not pass")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
