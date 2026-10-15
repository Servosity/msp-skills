// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: every restore point of one guest.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

type guestRestorePoints struct {
	Tenant             string         `json:"tenant"`
	Cluster            string         `json:"cluster"`
	VMID               int64          `json:"vmid"`
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	Node               string         `json:"node"`
	CoveredByJobs      []string       `json:"covered_by_jobs"`
	DisabledJobs       []string       `json:"disabled_jobs"`
	Count              int            `json:"count"`
	NewestAt           string         `json:"newest_at,omitempty"`
	NewestVerifiedOKAt string         `json:"newest_verified_ok_at,omitempty"`
	RestorePoints      []restorePoint `json:"restore_points"`
}

type restorePointsReport struct {
	Query         string               `json:"query"`
	Guests        []guestRestorePoints `json:"guests"`
	FetchFailures []fetchFailure       `json:"fetch_failures"`
}

func newNovelRestorePointsCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var verifiedOnly bool
	var clientFilter string
	cmd := &cobra.Command{
		Use:   "restore-points [vmid-or-name]",
		Short: "Every restore point of one guest across all storages and PBS namespaces, with verify state and prune marks",
		Long: "Use this command for every restore point of ONE guest (vmid or name) across all storages and PBS namespaces. " +
			"Do NOT use this command for a fleet-wide verdict; use 'posture' instead. " +
			"Do NOT use it for gaps between restore points over time; use 'rpo' instead.\n\n" +
			"The guest is resolved by vmid or name (case-insensitive) on every configured cluster; a vmid that exists on several " +
			"clusters is reported once per cluster. prune_mark is what the storage's next prune would do (keep, remove, protected), " +
			"from Proxmox's own prune preview; nothing is pruned.",
		Example: strings.Trim(`
  proxmox-cli restore-points 104 --agent
  proxmox-cli restore-points files --cluster acme
  proxmox-cli restore-points 104 --verified-only --limit 5 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "guest=100"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "restore-points")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(errors.New("a guest vmid or name is required"))
			}
			query := strings.TrimSpace(args[0])
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			type result struct {
				guests []guestRestorePoints
				fails  []fetchFailure
			}
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (result, error) {
				var r result
				var err error
				r.guests, r.fails, err = collectRestorePoints(ctx, t, query, verifiedOnly, limit)
				return r, err
			})
			rep := restorePointsReport{Query: query, Guests: make([]guestRestorePoints, 0), FetchFailures: failures}
			for _, r := range results {
				rep.Guests = append(rep.Guests, r.Value.guests...)
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			printFetchFailures(cmd.ErrOrStderr(), rep.FetchFailures)
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), rep.FetchFailures)
			if allFailed {
				return apiErr(fmt.Errorf("guest %q could not be resolved: %w", query, outcomeErr))
			}
			if len(rep.Guests) == 0 {
				return notFoundErr(fmt.Errorf("no guest matching %q on the reachable cluster(s) (%d visited)", query, len(targets)))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if flags.csv {
					var flat []restorePoint
					for _, g := range rep.Guests {
						flat = append(flat, g.RestorePoints...)
					}
					out = flat
				}
				return printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, restoreKeep...)
			}
			for _, g := range rep.Guests {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s (%s on %s), jobs: %s\n", g.Cluster, g.VMID, g.Name, g.Type, g.Node, orDash(strings.Join(g.CoveredByJobs, ",")))
				rows := make([][]string, 0, len(g.RestorePoints))
				for _, rp := range g.RestorePoints {
					rows = append(rows, []string{rp.TimeRFC3339, rp.Source, rp.VerifyState, strconv.FormatBool(rp.Protected), rp.PruneMark, rp.Notes})
				}
				if len(rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  no restore points")
					continue
				}
				if err := flags.printTable(cmd, []string{"BACKUP TIME", "SOURCE", "VERIFY", "PROTECTED", "PRUNE", "NOTES"}, rows); err != nil {
					return err
				}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&limit, "limit", 0, "Return at most this many newest restore points per guest (0 = all)")
	f.BoolVar(&verifiedOnly, "verified-only", false, "Only restore points whose verification state is ok")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	return cmd
}

func matchGuests(guests []pveGuest, query string) []pveGuest {
	var out []pveGuest
	vmid, isID := strconv.ParseInt(query, 10, 64)
	for _, g := range guests {
		if (isID == nil && int64(g.VMID) == vmid) || strings.EqualFold(g.Name, query) {
			out = append(out, g)
		}
	}
	return out
}

func collectRestorePoints(ctx context.Context, t fleetTarget, query string, verifiedOnly bool, limit int) ([]guestRestorePoints, []fetchFailure, error) {
	guests, err := fetchGuests(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	if len(guests) == 0 {
		if err := requireVisibility(ctx, t.C, false, client.PrivVMAudit); err != nil {
			return nil, nil, err
		}
	}
	matches := matchGuests(guests, query)
	if len(matches) == 0 {
		// A guest the token cannot audit is left out of the list, so "not found"
		// is only true when the list is complete.
		if err := guestListHidden(ctx, t.C); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	var fails []fetchFailure
	cov, err := fetchCoverage(ctx, t.C)
	if err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "backup jobs", Error: err.Error()})
		cov = &coverage{EnabledFor: map[int64][]string{}, DisabledFor: map[int64][]string{}}
	}
	nodes, offline, err := fetchNodes(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	fails = append(fails, offlineFailures(t.Name, offline)...)
	storages, readers, sfails := backupStorages(ctx, t.C, nodes)
	for _, f := range sfails {
		f.Cluster = t.Name
		fails = append(fails, f)
	}
	refs, err := fetchPBSStorageRefs(ctx, t.C)
	if err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage config", Error: err.Error()})
	}
	var nsByStore map[string][]string
	pbsHost := t.pbsEndpoint()
	if t.C.PBSConfigured() {
		var warning string
		if nsByStore, warning, err = pbsAttribution(ctx, t, refs); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs", Error: err.Error()})
		} else if warning != "" {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs attribution", Error: warning})
		}
	}
	var out []guestRestorePoints
	for _, g := range matches {
		vmid := int64(g.VMID)
		gr := guestRestorePoints{Tenant: t.Tenant, Cluster: t.Name, VMID: vmid, Name: g.Name, Type: g.Type, Node: g.Node,
			CoveredByJobs: append([]string{}, cov.EnabledFor[vmid]...), DisabledJobs: append([]string{}, cov.DisabledFor[vmid]...)}
		var points []restorePoint
		for _, key := range sortedKeys(storages) {
			s, node := storages[key], readers[key]
			rps, err := readStorageArchives(ctx, t.C, node, s, vmid, refs, pbsCovers(refs, s.Storage, pbsHost, nsByStore))
			if err != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error()})
				continue
			}
			if len(rps) == 0 {
				continue
			}
			marks := pruneMarks(ctx, t, node, s.Storage, vmid, g.Type, &fails)
			for i := range rps {
				rps[i].PruneMark = marks[fmt.Sprintf("%s@%d", rps[i].Volume, rps[i].Time)]
			}
			points = append(points, rps...)
		}
		pbsType := map[string]string{"qemu": "vm", "lxc": "ct"}[g.Type]
		for _, store := range sortedKeys(nsByStore) {
			for _, ns := range nsByStore[store] {
				snaps, err := pbsSnapshots(ctx, t.C, store, ns, map[string]string{"backup-type": pbsType, "backup-id": strconv.FormatInt(vmid, 10)})
				if err != nil {
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns), Error: err.Error()})
					continue
				}
				for _, s := range snaps {
					if s.BackupType == pbsType && s.BackupID == strconv.FormatInt(vmid, 10) {
						points = append(points, s.restorePoint(pbsHost, store, ns))
					}
				}
			}
		}
		points = dedupeRestorePoints(points)
		kept := make([]restorePoint, 0, len(points))
		for _, rp := range points {
			if rp.GuestType != "" && rp.GuestType != g.Type {
				continue
			}
			if verifiedOnly && rp.VerifyState != "ok" {
				continue
			}
			rp.Cluster = t.Name
			kept = append(kept, rp)
		}
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].Time > kept[j].Time })
		for _, rp := range kept {
			if rp.VerifyState == "ok" && gr.NewestVerifiedOKAt == "" {
				gr.NewestVerifiedOKAt = rp.TimeRFC3339
			}
		}
		if len(kept) > 0 {
			gr.NewestAt = kept[0].TimeRFC3339
		}
		gr.Count = len(kept)
		if limit > 0 && len(kept) > limit {
			kept = kept[:limit]
		}
		gr.RestorePoints = kept
		out = append(out, gr)
	}
	return out, fails, nil
}

// pruneMarks asks Proxmox what the next prune would do with each of the guest's
// backups on one storage (a GET preview; nothing is removed).
func pruneMarks(ctx context.Context, t fleetTarget, node, storage string, vmid int64, guestType string, fails *[]fetchFailure) map[string]string {
	var preview []struct {
		VolID string  `json:"volid"`
		CTime flexInt `json:"ctime"`
		Mark  string  `json:"mark"`
	}
	params := map[string]string{"vmid": strconv.FormatInt(vmid, 10), "type": guestType}
	if err := getJSON(ctx, t.C, pvePath("/nodes/%s/storage/%s/prunebackups", node, storage), params, &preview); err != nil {
		*fails = append(*fails, fetchFailure{Cluster: t.Name, Scope: "prune preview " + storage, Error: err.Error()})
		return nil
	}
	marks := make(map[string]string, len(preview))
	for _, p := range preview {
		marks[fmt.Sprintf("%s@%d", p.VolID, int64(p.CTime))] = p.Mark
	}
	return marks
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
