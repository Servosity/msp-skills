// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: PBS verify-failure spread.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

type verifyGroupRow struct {
	Tenant             string `json:"tenant"`
	Cluster            string `json:"cluster"`
	Store              string `json:"store"`
	Namespace          string `json:"namespace"`
	Group              string `json:"group"` // vm/100
	Snapshots          int    `json:"snapshots"`
	Failed             int    `json:"failed"`
	NeverVerified      int    `json:"never_verified"`
	Outdated           int    `json:"outdated"`
	FirstFailureAt     string `json:"first_failure_at,omitempty"`
	NewestVerifiedOKAt string `json:"newest_verified_ok_at,omitempty"`
	NewestIsBad        bool   `json:"newest_is_bad"`
	Status             string `json:"status"` // failing | unverified | outdated | ok
	// Attribution is "untied" when pbs_namespace could not be tied to this
	// cluster's datastores (PVE unreachable): the row may be another client's.
	// "shared: <clusters>" when another configured cluster writes, or may
	// write, to the namespace: tenant is then empty.
	Attribution string `json:"attribution,omitempty"`
}

type verifyStoreRow struct {
	Cluster            string   `json:"cluster"`
	Store              string   `json:"store"`
	Groups             int      `json:"groups"`
	FailingGroups      int      `json:"failing_groups"`
	FailedSnapshots    int      `json:"failed_snapshots"`
	NeverVerified      int      `json:"never_verified_snapshots"`
	GCStillBadChunks   int64    `json:"gc_still_bad_chunks"`
	GCRemovedBadChunks int64    `json:"gc_removed_bad_chunks"`
	OutdatedSnapshots  int      `json:"outdated_snapshots"`
	VerifyJobs         []string `json:"verify_jobs"`
	UnreachedNS        []string `json:"namespaces_without_verify_job"`
	// *Unknown is set when the fetch failed: the matching fields are not zero, they are unknown.
	VerifyJobsUnknown bool `json:"verify_jobs_unknown,omitempty"`
	GCUnknown         bool `json:"gc_unknown,omitempty"`
	// SnapshotsIncomplete: a namespace's snapshot list could not be read (or
	// a namespace below it not listed), or a snapshot had no readable
	// manifest, so the group counts cover only what was read.
	SnapshotsIncomplete bool   `json:"snapshots_incomplete,omitempty"`
	Attribution         string `json:"attribution,omitempty"` // see verifyGroupRow
}

type verifyReport struct {
	Clusters      []string         `json:"clusters"`
	Stores        []verifyStoreRow `json:"stores"`
	Groups        []verifyGroupRow `json:"groups"`
	Skipped       []string         `json:"skipped_clusters"`
	FetchFailures []fetchFailure   `json:"fetch_failures"`
}

type verifyJobDef struct {
	ID            string  `json:"id"`
	Store         string  `json:"store"`
	NS            string  `json:"ns"`
	MaxDepth      *int    `json:"max-depth"`
	OutdatedAfter flexInt `json:"outdated-after"`
	LastRunState  string  `json:"last-run-state"`
}

// reaches reports whether the verify job covers namespace ns (job ns is a
// prefix and the depth below it is within max-depth, default 7).
func (j verifyJobDef) reaches(ns string) bool { return nsReaches(j.NS, j.MaxDepth, ns) }

func newNovelVerifyHealthCmd(flags *rootFlags) *cobra.Command {
	var store, clientFilter string
	var all bool
	cmd := &cobra.Command{
		Use:   "verify-health",
		Short: "Verify-failure spread per PBS datastore and backup group, plus namespaces no verify job reaches",
		Long: "Use this command for verify-failure spread per PBS datastore and backup group, and namespaces no verify job reaches. " +
			"Do NOT use this command for a guest's current backup verdict; use 'posture' instead. " +
			"Do NOT use it for datastore space or GC runway; use 'capacity' instead.\n\n" +
			"Needs the PBS API (PROXMOX_PBS_HOST + PROXMOX_PBS_API_TOKEN, or pbs_host in clusters.toml). " +
			"A snapshot counts as outdated when its last verification is older than the reaching verify job's outdated-after days.",
		Example: strings.Trim(`
  proxmox-cli verify-health --agent
  proxmox-cli verify-health --store main --all
  proxmox-cli verify-health --cluster acme --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "verify-health")
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			markSharedPBS(ctx, flags, targets)
			rep := verifyReport{Stores: make([]verifyStoreRow, 0), Groups: make([]verifyGroupRow, 0), Skipped: make([]string, 0)}
			var pbsTargets []fleetTarget
			for _, t := range targets {
				rep.Clusters = append(rep.Clusters, t.Name)
				if !t.usable() || t.C.PBSConfigured() {
					pbsTargets = append(pbsTargets, t)
				} else {
					rep.Skipped = append(rep.Skipped, t.Name+": no PBS host configured")
				}
			}
			type result struct {
				stores []verifyStoreRow
				groups []verifyGroupRow
				fails  []fetchFailure
			}
			now := time.Now()
			results, failures := runFleet(ctx, pbsTargets, func(ctx context.Context, t fleetTarget) (result, error) {
				var res result
				s, g, f, err := collectVerifyHealth(ctx, t, store, now)
				res.stores, res.groups, res.fails = s, g, f
				return res, err
			})
			rep.FetchFailures = failures
			for _, r := range results {
				rep.Stores = append(rep.Stores, r.Value.stores...)
				rep.Groups = append(rep.Groups, r.Value.groups...)
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			if !all {
				kept := make([]verifyGroupRow, 0, len(rep.Groups))
				for _, g := range rep.Groups {
					if g.Status != "ok" {
						kept = append(kept, g)
					}
				}
				rep.Groups = kept
			}
			sort.SliceStable(rep.Groups, func(i, j int) bool {
				if rep.Groups[i].Failed != rep.Groups[j].Failed {
					return rep.Groups[i].Failed > rep.Groups[j].Failed
				}
				return rep.Groups[i].Group < rep.Groups[j].Group
			})
			printFetchFailures(cmd.ErrOrStderr(), rep.FetchFailures)
			for _, s := range rep.Skipped {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: skipped %s\n", s)
			}
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(pbsTargets), rep.FetchFailures)
			if len(pbsTargets) == 0 && len(rep.Skipped) > 0 {
				// Nothing was checked. Exit 5 like an all-unreachable fleet so a
				// script reading only the exit code never takes this for "no failures".
				msg := fmt.Sprintf("No data: 0 of %d cluster(s) have a PBS host configured; set PROXMOX_PBS_HOST and PROXMOX_PBS_API_TOKEN (or pbs_host in clusters.toml)", len(rep.Skipped))
				fmt.Fprintln(cmd.ErrOrStderr(), msg)
				allFailed, outcomeErr = true, apiErr(errors.New(msg))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if rowsOnly(flags) {
					out = rep.Groups
				}
				if flags.quiet {
					printQuietRows(cmd.OutOrStdout(), rep.Groups, func(r verifyGroupRow) string {
						return r.Cluster + " " + r.Store + "/" + r.Namespace + "/" + r.Group
					})
				} else if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, verifyKeep...); err != nil {
					return err
				}
				return outcomeErr
			}
			if allFailed {
				return outcomeErr
			}
			if len(rep.Stores) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No PBS datastores reachable; set PROXMOX_PBS_HOST and PROXMOX_PBS_API_TOKEN (or pbs_host in clusters.toml).")
				return nil
			}
			rows := make([][]string, 0, len(rep.Stores))
			for _, s := range rep.Stores {
				rows = append(rows, []string{s.Cluster, s.Store, fmt.Sprint(s.Groups), fmt.Sprint(s.FailingGroups), fmt.Sprint(s.FailedSnapshots), fmt.Sprint(s.NeverVerified), fmt.Sprint(s.GCStillBadChunks), strings.Join(s.UnreachedNS, ",")})
			}
			if err := flags.printTable(cmd, []string{"CLUSTER", "STORE", "GROUPS", "FAILING", "FAILED SNAPS", "NEVER VERIFIED", "GC STILL-BAD", "NS WITHOUT VERIFY JOB"}, rows); err != nil {
				return err
			}
			if len(rep.Groups) > 0 {
				fmt.Fprintln(cmd.OutOrStdout())
				grows := make([][]string, 0, len(rep.Groups))
				for _, g := range rep.Groups {
					grows = append(grows, []string{g.Store, g.Namespace, g.Group, g.Status, fmt.Sprint(g.Failed), fmt.Sprint(g.NeverVerified), g.NewestVerifiedOKAt, g.Attribution})
				}
				return flags.printTable(cmd, []string{"STORE", "NS", "GROUP", "STATUS", "FAILED", "NEVER", "NEWEST VERIFIED OK", "ATTRIBUTION"}, grows)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&store, "store", "", "Only this PBS datastore")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	f.BoolVar(&all, "all", false, "Include healthy groups (by default only failing or unverified groups are listed)")
	return cmd
}

func collectVerifyHealth(ctx context.Context, t fleetTarget, onlyStore string, now time.Time) ([]verifyStoreRow, []verifyGroupRow, []fetchFailure, error) {
	var fails []fetchFailure
	nsByStore, notes, untied, err := pbsNamespacesForCluster(ctx, t)
	if err != nil {
		return nil, nil, nil, err
	}
	gapStores := map[string]bool{} // a namespace below it was not listed or not read
	for _, n := range notes {
		gapStores[strings.TrimPrefix(n.Scope, "pbs ")] = true
		// --store narrows the notes about other datastores away too.
		if onlyStore == "" || !strings.HasPrefix(n.Scope, "pbs ") || n.Scope == "pbs "+onlyStore || n.Scope == "pbs namespace" {
			fails = append(fails, n)
		}
	}
	if t.hasPVE() {
		// A storage backing up to another PBS: its verify state was not read.
		fails = append(fails, uncheckedPBS(ctx, t, "pbs verify", "its verify state was")...)
	}
	if err := pbsDatastoreListHidden(ctx, t.C); err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs datastores", Error: err.Error()})
	}
	attribution := ""
	if untied {
		attribution = "untied"
	}
	if len(nsByStore) == 0 {
		if err := requireVisibility(ctx, t.C, true, client.PrivDatastoreAudit); err != nil {
			return nil, nil, nil, err
		}
	}
	var jobs []verifyJobDef
	jobsOK := true
	if err := getJSON(ctx, t.C, pbsPath("/admin/verify"), nil, &jobs); err != nil {
		jobsOK = false
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs verify jobs", Error: err.Error()})
	}
	var stores []verifyStoreRow
	var groups []verifyGroupRow
	seen := map[string]string{} // ACL path -> why its verify jobs are hidden ("" = visible)
	wanted, read := 0, 0
	for _, store := range sortedKeys(nsByStore) {
		if onlyStore != "" && store != onlyStore {
			continue
		}
		sr := verifyStoreRow{Cluster: t.Name, Store: store, VerifyJobs: []string{}, UnreachedNS: []string{}, VerifyJobsUnknown: !jobsOK, SnapshotsIncomplete: gapStores[store]}
		var storeJobs []verifyJobDef
		for _, j := range jobs {
			if j.Store == store {
				storeJobs = append(storeJobs, j)
				sr.VerifyJobs = append(sr.VerifyJobs, j.ID)
			}
		}
		var gc struct {
			StillBad   flexInt `json:"still-bad"`
			RemovedBad flexInt `json:"removed-bad"`
		}
		if err := getJSON(ctx, t.C, pbsPath("/admin/datastore/%s/gc", store), nil, &gc); err != nil {
			sr.GCUnknown = true
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs " + store + " gc", Error: err.Error()})
		}
		sr.GCStillBadChunks, sr.GCRemovedBadChunks = int64(gc.StillBad), int64(gc.RemovedBad)
		for _, ns := range nsByStore[store] {
			// A namespace another configured cluster writes to (or may) mixes
			// its groups with this client's: mark the rows, never drop them.
			nsAttribution := attribution
			if owners := t.sharedOwners(pbsStorageRef{server: t.pbsEndpoint(), datastore: store, fingerprint: t.Profile.PBSFingerprint}, ns); len(owners) > 0 {
				nsAttribution = "shared: " + strings.Join(owners, ", ")
				sr.Attribution = "shared"
			}
			var reaching []verifyJobDef
			for _, j := range storeJobs {
				if j.reaches(ns) {
					reaching = append(reaching, j)
				}
			}
			jobsKnown := jobsOK
			if jobsOK {
				if hidden := verifyJobsHidden(ctx, t.C, store, ns, seen); hidden != "" {
					jobsKnown, sr.VerifyJobsUnknown = false, true
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns) + " verify jobs", Error: hidden})
				}
			}
			if len(reaching) == 0 && jobsKnown {
				label := ns
				if label == "" {
					label = "(root)"
				}
				sr.UnreachedNS = append(sr.UnreachedNS, label)
			}
			outdatedDays := int64(0)
			for _, j := range reaching {
				if d := int64(j.OutdatedAfter); d > 0 && (outdatedDays == 0 || d < outdatedDays) {
					outdatedDays = d
				}
			}
			wanted++
			snaps, err := pbsSnapshots(ctx, t.C, store, ns, nil)
			if err != nil {
				sr.SnapshotsIncomplete = true
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns), Error: err.Error()})
				continue
			}
			read++
			// A snapshot without a readable manifest (a backup still running, or
			// a damaged one) cannot be verified: report it, verify the rest.
			complete := snaps[:0]
			noManifest := 0
			for _, s := range snaps {
				if s.Size > 0 {
					complete = append(complete, s)
				} else {
					noManifest++
				}
			}
			if noManifest > 0 {
				sr.SnapshotsIncomplete = true
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns), Error: fmt.Sprintf(
					"%d snapshot(s) listed without a readable manifest (a backup still running, or a damaged snapshot); they are not in the group counts", noManifest)})
			}
			for _, g := range groupVerify(t, store, ns, complete, outdatedDays, jobsKnown, now) {
				sr.Groups++
				sr.FailedSnapshots += g.Failed
				sr.NeverVerified += g.NeverVerified
				sr.OutdatedSnapshots += g.Outdated
				if g.Status == "failing" {
					sr.FailingGroups++
				}
				g.Attribution = nsAttribution
				if strings.HasPrefix(nsAttribution, "shared") {
					g.Tenant = "" // another configured cluster writes (or may write) here: no single client's
				}
				groups = append(groups, g)
			}
		}
		if sr.Attribution == "" {
			sr.Attribution = attribution
		}
		stores = append(stores, sr)
	}
	if wanted > 0 && read == 0 {
		return nil, nil, nil, fmt.Errorf("no PBS snapshot list could be read: %s", fails[len(fails)-1].Error) // no verify evidence at all
	}
	return stores, groups, fails, nil
}

// verifyJobsHidden explains why some verify job that may reach ns on store is
// not in the token's list, or "". PBS lists a verify job only to a token with
// Datastore.Audit or Datastore.Verify on the job's own path (its datastore,
// and namespace), so a job rooted at the datastore or at any namespace above
// ns is visible only when that holds at each of those paths.
func verifyJobsHidden(ctx context.Context, c *client.Client, store, ns string, seen map[string]string) string {
	paths := []string{nsACLPath(store, "")}
	parts := strings.Split(ns, "/")
	for i := range parts {
		if ns != "" {
			paths = append(paths, nsACLPath(store, strings.Join(parts[:i+1], "/")))
		}
	}
	for _, p := range paths {
		why, done := seen[p]
		if !done {
			privs, err := privsAt(ctx, c, true, p)
			switch {
			case err != nil:
				why = "whether the token sees the verify jobs on " + p + " is unknown (" + err.Error() + ")"
			case !hasKey(privs, client.PrivDatastoreAudit.String()) && !hasKey(privs, "Datastore.Verify"):
				why = "PBS lists a verify job only to a token with Datastore.Audit or Datastore.Verify on its own datastore or namespace path, and this token has neither on " + p + ", so a verify job there (and its outdated-after) is hidden"
			}
			seen[p] = why
		}
		if why != "" {
			return why + "; verification age is not judged"
		}
	}
	return ""
}

func hasKey(m map[string]flexBool, k string) bool { _, ok := m[k]; return ok }

// groupVerify aggregates snapshots per backup group. A verified-ok snapshot is
// outdated when its verification (start time encoded in the verify UPID) is
// older than the reaching verify job's outdated-after days.
func groupVerify(t fleetTarget, store, ns string, snaps []pbsSnapshot, outdatedDays int64, jobsKnown bool, now time.Time) []verifyGroupRow {
	byGroup := map[string]*verifyGroupRow{}
	newest := map[string]int64{}
	newestState := map[string]string{}
	futureNewest := map[string]int64{} // newest future-dated snapshot per group
	futureFailed := map[string]bool{}  // ... and whether it failed verification
	firstFail := map[string]int64{}
	newestOK := map[string]int64{}
	for _, s := range snaps {
		key := s.BackupType + "/" + s.BackupID
		g := byGroup[key]
		if g == nil {
			g = &verifyGroupRow{Tenant: t.Tenant, Cluster: t.Name, Store: store, Namespace: ns, Group: key}
			byGroup[key] = g
		}
		g.Snapshots++
		ts := int64(s.BackupTime)
		future := ts > now.Add(clockSkewTolerance).Unix() // a node clock ran ahead: no judgeable recency
		state := ""
		if s.Verification != nil {
			state = s.Verification.State
		}
		switch state {
		case "failed":
			g.Failed++
			if firstFail[key] == 0 || ts < firstFail[key] {
				firstFail[key] = ts
			}
		case "ok":
			if ts > newestOK[key] && !future {
				newestOK[key] = ts
			}
			if vt := upidStartTime(s.Verification.UPID); outdatedDays > 0 && vt > 0 && now.Sub(time.Unix(vt, 0)) > time.Duration(outdatedDays)*24*time.Hour {
				g.Outdated++
			}
		default:
			g.NeverVerified++
		}
		switch {
		case future && ts > futureNewest[key]:
			futureNewest[key], futureFailed[key] = ts, state == "failed" // newer than every real point
		case !future && ts > newest[key]:
			newest[key], newestState[key] = ts, state
		}
	}
	out := make([]verifyGroupRow, 0, len(byGroup))
	for _, key := range sortedKeys(byGroup) {
		g := byGroup[key]
		g.FirstFailureAt = unixRFC3339(firstFail[key])
		g.NewestVerifiedOKAt = unixRFC3339(newestOK[key])
		g.NewestIsBad = newestState[key] == "failed" || futureFailed[key]
		switch {
		case g.Failed > 0:
			g.Status = "failing"
		case g.NeverVerified > 0:
			g.Status = "unverified"
		case g.Outdated > 0:
			g.Status = "outdated" // verified, but longer ago than the verify job's outdated-after
		case !jobsKnown:
			// The verify-job list was unreadable, so no outdated-after applies and
			// a verification of any age would pass: that is unproven, not ok.
			g.Status = "verify-age-unknown"
		default:
			g.Status = "ok"
		}
		out = append(out, *g)
	}
	return out
}
