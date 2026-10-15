// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: every restore point of one guest.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

type guestRestorePoints struct {
	Tenant        string   `json:"tenant"`
	Cluster       string   `json:"cluster"`
	VMID          int64    `json:"vmid"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Node          string   `json:"node"`
	CoveredByJobs []string `json:"covered_by_jobs"`
	DisabledJobs  []string `json:"disabled_jobs"`
	Count         int      `json:"count"`
	// Evidence is "full" when every source that may hold this guest's restore
	// points was read, else "partial" and NotRead names what was not: a count
	// of 0 then means none could be read, not that none exist.
	Evidence           string         `json:"evidence"`
	NotRead            []string       `json:"not_read,omitempty"`
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
			"clusters is reported once per cluster. prune_mark is what the next prune would do (keep, remove, protected), from Proxmox's " +
			"own prune preview with the covering job's retention (else the storage's); nothing is pruned. evidence is partial, and " +
			"not_read names the sources, when something that may hold the guest's restore points could not be read; then a count " +
			"of 0 means none could be read, and the command exits 5 when no match has a readable point.",
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
			markSharedPBS(ctx, flags, targets)
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
			// Every match came back empty while something was not read: that is
			// "could not be read", and must not exit like a clean "none".
			var unreadErr error
			if g := rep.Guests; len(g) > 0 {
				unreadErr = apiErr(fmt.Errorf("no restore point of %q could be read and some sources were not (see not_read and the warnings)", query))
				for _, gr := range g {
					if gr.Count > 0 || gr.Evidence != "partial" {
						unreadErr = nil
					}
				}
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
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, restoreKeep...); err != nil {
					return err
				}
				return unreadErr
			}
			for _, g := range rep.Guests {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s (%s on %s), jobs: %s\n", g.Cluster, g.VMID, g.Name, g.Type, g.Node, orDash(strings.Join(g.CoveredByJobs, ",")))
				rows := make([][]string, 0, len(g.RestorePoints))
				for _, rp := range g.RestorePoints {
					rows = append(rows, []string{rp.TimeRFC3339, rp.Source, rp.VerifyState, strconv.FormatBool(rp.Protected), rp.PruneMark, rp.Notes})
				}
				if len(rows) == 0 {
					if g.Evidence == "partial" {
						fmt.Fprintf(cmd.OutOrStdout(), "  no restore point could be read; not read: %s\n", strings.Join(g.NotRead, "; "))
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "  no restore points")
					}
					continue
				}
				if err := flags.printTable(cmd, []string{"BACKUP TIME", "SOURCE", "VERIFY", "PROTECTED", "PRUNE", "NOTES"}, rows); err != nil {
					return err
				}
				if g.Evidence == "partial" {
					fmt.Fprintf(cmd.OutOrStdout(), "  partial: not read: %s\n", strings.Join(g.NotRead, "; "))
				}
			}
			return unreadErr
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
	covOK := err == nil
	if err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "backup jobs", Error: err.Error()})
		cov = &coverage{EnabledFor: map[int64][]string{}, DisabledFor: map[int64][]string{}}
	}
	nodes, offline, err := fetchNodes(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	// clusterUnread: sources that may hold any guest's restore points but were
	// not read; every guest's evidence is then partial.
	var clusterUnread []string
	for _, n := range offline {
		clusterUnread = append(clusterUnread, "node "+n+" (offline)")
	}
	fails = append(fails, offlineFailures(t.Name, offline)...)
	if !covOK {
		// Unknown jobs mean unknown targets (a dumpdir is no storage).
		clusterUnread = append(clusterUnread, "backup jobs (their targets are unknown)")
	}
	storages, readers, sfails, disabled := backupStorages(ctx, t.C, nodes)
	for _, f := range sfails {
		f.Cluster = t.Name
		fails = append(fails, f)
		clusterUnread = append(clusterUnread, f.Scope)
	}
	for _, key := range sortedKeys(disabled) {
		fails = append(fails, disabledFailure(t.Name, key))
		clusterUnread = append(clusterUnread, "storage "+key+" (disabled)")
	}
	if err := storageListHidden(ctx, t.C); err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "backup storage", Error: err.Error()})
		clusterUnread = append(clusterUnread, "storages hidden from the token")
	}
	refs, err := fetchPBSStorageRefs(ctx, t.C)
	if err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage config", Error: err.Error()})
	}
	refs = t.withShared(refs)
	if len(t.sharedUnknown) > 0 && len(refs) > 0 {
		fails = append(fails, t.sharedUnknownFailure())
		clusterUnread = append(clusterUnread, "pbs sharing with "+strings.Join(t.sharedUnknown, ", "))
	}
	targets := newJobTargets(ctx, t.C, cov)
	var nsByStore map[string][]string
	pbsHost := t.pbsEndpoint()
	if t.C.PBSConfigured() {
		var warning string
		if nsByStore, warning, err = pbsAttribution(ctx, t, refs); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs", Error: err.Error()})
			clusterUnread = append(clusterUnread, "pbs")
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
		notRead := append([]string{}, clusterUnread...)
		read := map[string]bool{}             // storages whose archives were read for this guest
		standIn := map[string]pbsStorageRef{} // read through the PBS API instead
		pbsUnread := map[string]bool{}
		for _, key := range sortedKeys(storages) {
			s, node := storages[key], readers[key]
			covered := pbsCovers(refs, s.Storage, pbsHost, nsByStore)
			rps, err := readStorageArchives(ctx, t.C, node, s, vmid, refs, covered)
			if err != nil && covered {
				standIn[key] = refs[s.Storage] // the PBS API lists these below
				if !errors.Is(err, errCoveredByPBS) {
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error() + " (its datastore is read through the PBS API instead)"})
				}
				continue
			}
			if err != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error()})
				notRead = append(notRead, "storage "+key)
				continue
			}
			read[key] = true
			if len(rps) == 0 {
				continue
			}
			retention := jobRetention(cov, targets, vmid, g.Node, s.Storage)
			if !covOK {
				retention = retentionSpec{why: "the backup jobs could not be read, so the job retention that overrides the storage's is unknown; points left unmarked"}
			}
			marks := pruneMarks(ctx, t, node, s.Storage, vmid, g.Type, retention, &fails)
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
					notRead = append(notRead, pbsSourceLabel(store, ns))
					pbsUnread[pbsSourceLabel(store, ns)] = true
					continue
				}
				for _, s := range snaps {
					if s.BackupType == pbsType && s.BackupID == strconv.FormatInt(vmid, 10) {
						points = append(points, s.restorePoint(pbsHost, store, ns))
					}
				}
			}
		}
		// A covering job's target that was not read (a dumpdir, a hidden or
		// failed storage, a failed PBS stand-in) may hold this guest's points.
		pbsStandIns(standIn, pbsUnread, read)
		for _, gap := range jobStorageGaps(cov, vmid, g.Node, read, targets) {
			notRead = append(notRead, gap.String())
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
		gr.Evidence = "full"
		if len(notRead) > 0 {
			gr.Evidence, gr.NotRead = "partial", notRead
		}
		if limit > 0 && len(kept) > limit {
			kept = kept[:limit]
		}
		gr.RestorePoints = kept
		out = append(out, gr)
	}
	return out, fails, nil
}

// pruneMarks asks Proxmox what the next prune would do with each of the guest's
// backups on one storage (a GET preview; nothing is removed). retention is the
// covering job's prune-backups, which overrides the storage's; "" previews the
// storage's own. ok=false (jobs that disagree) leaves the points unmarked.
func pruneMarks(ctx context.Context, t fleetTarget, node, storage string, vmid int64, guestType string, retention retentionSpec, fails *[]fetchFailure) map[string]string {
	if !retention.ok {
		*fails = append(*fails, fetchFailure{Cluster: t.Name, Scope: "prune preview " + storage, Error: retention.why})
		return nil
	}
	var preview []struct {
		VolID string  `json:"volid"`
		CTime flexInt `json:"ctime"`
		Mark  string  `json:"mark"`
	}
	params := map[string]string{"vmid": strconv.FormatInt(vmid, 10), "type": guestType}
	if retention.spec != "" {
		params["prune-backups"] = retention.spec
	}
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

type retentionSpec struct {
	spec, why string
	ok        bool
}

// jobRetention is the retention the next prune of storage applies to vmid:
// the prune-backups of the enabled jobs covering it that write there (a job's
// own retention overrides the storage's), "" for the storage's own when none
// sets one. Jobs that disagree make the next prune depend on which runs.
func jobRetention(cov *coverage, targets *jobTargets, vmid int64, node, storage string) retentionSpec {
	specs := map[string]bool{}
	for _, id := range cov.EnabledFor[vmid] {
		if targets.target(id, node) != storage {
			continue
		}
		specs[pruneSpec(targets.jobs[id].PruneBackups)] = true
	}
	switch len(specs) {
	case 0:
		return retentionSpec{ok: true}
	case 1:
		for s := range specs {
			return retentionSpec{spec: s, ok: true}
		}
	}
	return retentionSpec{why: "the backup jobs writing this guest to " + storage + " keep different retention, so the next prune depends on which job runs; points left unmarked"}
}

// pruneSpec renders a job's prune-backups (object or property string) as the
// property string the prune preview takes; "" when the job sets none.
func pruneSpec(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return strings.Join(parts, ",")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
