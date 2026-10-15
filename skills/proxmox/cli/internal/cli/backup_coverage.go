// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: backup coverage with reasons.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

type coverageRow struct {
	Tenant           string   `json:"tenant"`
	Cluster          string   `json:"cluster"`
	VMID             int64    `json:"vmid"`
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	Node             string   `json:"node"`
	Disk             string   `json:"disk,omitempty"`
	Reason           string   `json:"reason"` // no-job | excluded | disabled-job-only | disk-excluded | template
	Detail           string   `json:"detail"`
	DisabledJobs     []string `json:"disabled_jobs,omitempty"`
	UpstreamReported bool     `json:"upstream_not_backed_up"`
	Disagrees        bool     `json:"disagrees_with_upstream"`
}

type coverageReport struct {
	Clusters      []string       `json:"clusters"`
	Guests        int            `json:"guests_checked"`
	Uncovered     int            `json:"uncovered_guests"`
	Rows          []coverageRow  `json:"rows"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

func newNovelBackupCoverageCmd(flags *rootFlags) *cobra.Command {
	var includeDisks, includeTemplates bool
	var clientFilter, elsewhereTag string
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Every guest or disk that no ENABLED backup job includes, with the reason (no job, disabled job only, disk excluded)",
		Long: "Use this command for which guests or disks no ENABLED backup job includes, and the reason for each. " +
			"Do NOT use this command for backup age or verify state; use 'posture' instead.\n\n" +
			"Job selection (all, pool, vmid lists, excludes, per-disk backup=0) is resolved by Proxmox itself through each job's " +
			"included_volumes, then split by whether the job is enabled. The upstream 'backup not-backed-up' list counts guests " +
			"that only disabled jobs include as covered; rows where the two disagree are flagged.",
		Example: strings.Trim(`
  proxmox-cli backup coverage --include-disks --agent
  proxmox-cli backup coverage --cluster acme
  proxmox-cli backup coverage --protected-elsewhere-tag veeam --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "backup coverage")
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			type result struct {
				rows   []coverageRow
				guests int
			}
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (result, error) {
				guests, err := fetchGuests(ctx, t.C)
				if err != nil {
					return result{}, err
				}
				if len(guests) == 0 {
					if err := requireVisibility(ctx, t.C, false, client.PrivVMAudit); err != nil {
						return result{}, err
					}
				}
				cov, err := fetchCoverage(ctx, t.C)
				if err != nil {
					return result{}, err
				}
				rows := coverageRows(t, guests, cov, includeDisks, includeTemplates, elsewhereTag)
				return result{rows: rows, guests: len(guests)}, nil
			})
			rep := coverageReport{Rows: make([]coverageRow, 0), FetchFailures: failures}
			for _, t := range targets {
				rep.Clusters = append(rep.Clusters, t.Name)
			}
			for _, r := range results {
				rep.Rows = append(rep.Rows, r.Value.rows...)
				rep.Guests += r.Value.guests
			}
			for _, r := range rep.Rows {
				if r.Disk == "" && r.Reason != "template" {
					rep.Uncovered++
				}
			}
			printFetchFailures(cmd.ErrOrStderr(), failures)
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), failures)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if flags.csv {
					out = rep.Rows
				}
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, coverageKeep...); err != nil {
					return err
				}
				return outcomeErr
			}
			if allFailed {
				return outcomeErr
			}
			if len(rep.Rows) == 0 {
				allClear(cmd.OutOrStdout(), fmt.Sprintf("Every one of %d guest(s) is included by an enabled backup job.", rep.Guests), len(rep.FetchFailures))
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			for _, r := range rep.Rows {
				rows = append(rows, []string{r.Cluster, fmt.Sprint(r.VMID), r.Name, r.Disk, r.Reason, r.Detail})
			}
			return flags.printTable(cmd, []string{"CLUSTER", "VMID", "NAME", "DISK", "REASON", "DETAIL"}, rows)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&includeDisks, "include-disks", false, "Also list disks that every enabled job including the guest skips (backup=0)")
	f.BoolVar(&includeTemplates, "include-templates", false, "Report uncovered templates too (skipped by default)")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	f.StringVar(&elsewhereTag, "protected-elsewhere-tag", "", "PVE guest tag meaning 'backed up by another product'; such guests are not reported")
	return cmd
}

func coverageRows(t fleetTarget, guests []pveGuest, cov *coverage, includeDisks, includeTemplates bool, elsewhereTag string) []coverageRow {
	rows := []coverageRow{}
	for _, g := range guests {
		vmid := int64(g.VMID)
		base := coverageRow{Tenant: t.Tenant, Cluster: t.Name, VMID: vmid, Name: g.Name, Type: g.Type, Node: g.Node, UpstreamReported: cov.NotBackedUp[vmid]}
		if len(cov.EnabledFor[vmid]) > 0 {
			if includeDisks {
				for _, disk := range cov.DiskExcluded[vmid] {
					r := base
					r.Disk, r.Reason = disk, "disk-excluded"
					r.Detail = "excluded by every enabled job that includes the guest (backup=0)"
					rows = append(rows, r)
				}
			}
			continue
		}
		if g.hasTag(elsewhereTag) {
			continue
		}
		r := base
		switch {
		case bool(g.Template):
			if !includeTemplates {
				continue
			}
			r.Reason, r.Detail = "template", "template; no enabled job includes it"
		case len(excludingJobs(cov, vmid)) > 0:
			r.Reason = "excluded"
			r.Detail = "explicitly excluded by enabled job(s): " + strings.Join(excludingJobs(cov, vmid), ", ")
		case len(cov.DisabledFor[vmid]) > 0:
			r.Reason = "disabled-job-only"
			r.DisabledJobs = append([]string{}, cov.DisabledFor[vmid]...)
			sort.Strings(r.DisabledJobs)
			r.Detail = "only included by disabled job(s): " + strings.Join(r.DisabledJobs, ", ")
		default:
			r.Reason, r.Detail = "no-job", "no backup job includes this guest"
		}
		r.Disagrees = !r.UpstreamReported
		rows = append(rows, r)
	}
	return rows
}

func excludingJobs(cov *coverage, vmid int64) []string {
	var ids []string
	for _, j := range cov.Jobs {
		if j.enabled() && j.excludes(vmid) {
			ids = append(ids, j.ID)
		}
	}
	return ids
}
