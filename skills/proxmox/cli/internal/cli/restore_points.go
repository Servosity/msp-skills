// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: every restore point of one guest.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"proxmox-pp-cli/internal/client"

	"github.com/spf13/cobra"
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
	Readable      int      `json:"-"` // points read before --verified-only filtered them
	// PredatingGuest: backups under this VMID older than this VM's creation
	// (an earlier guest's), not listed.
	PredatingGuest int `json:"predating_guest,omitempty"`
	// MayBeEarlierGuest: listed points that cannot be told from an earlier
	// guest's (marked may_be_earlier_guest); newest_at and
	// newest_verified_ok_at count only points proven this VM's.
	MayBeEarlierGuest int `json:"may_be_earlier_guest,omitempty"`
	// HiddenUnverified: read points --verified-only left out (none verified).
	HiddenUnverified int `json:"hidden_unverified,omitempty"`
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
		Short: "Every restore point of one guest across all storages and PBS namespaces, with verify state and protection",
		Long: "Use this command for every restore point of ONE guest (vmid or name) across all storages and PBS namespaces. " +
			"Do NOT use this command for a fleet-wide verdict; use 'posture' instead. " +
			"Do NOT use it for gaps between restore points over time; use 'rpo' instead.\n\n" +
			"The guest is resolved by vmid or name (case-insensitive) on every configured cluster; a vmid that exists on several " +
			"clusters is reported once per cluster. protected marks a point Proxmox never prunes; the CLI does not predict what a future prune removes. " +
			"evidence is partial, and " +
			"not_read names the sources, when something that may hold the guest's restore points could not be read; then a count " +
			"of 0 means none could be read, and the command exits 5 when no match has a readable point. " +
			"predating_guest counts backups under the VMID from before this VM was created (an earlier guest's with the same VMID), " +
			"which are not listed. A backup that may be either guest's (the VM's config, which records its creation time, could not be read, " +
			"or the backup is dated within an hour before that time) is listed with may_be_earlier_guest, counted in the guest's may_be_earlier_guest, " +
			"and left out of newest_at and newest_verified_ok_at; evidence is then partial. --csv reports predating_guest and partial evidence on stderr.",
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
			if nothingReadable(rep.Guests) {
				unreadErr = apiErr(fmt.Errorf("no restore point of %q could be read and some sources were not (see not_read and the warnings)", query))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if flags.csv {
					flat := []restorePoint{}
					for _, g := range rep.Guests {
						flat = append(flat, g.RestorePoints...)
						if g.Evidence == "partial" { // CSV has no column for it
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s %d: partial evidence, not read: %s\n", g.Cluster, g.VMID, strings.Join(g.NotRead, "; "))
						}
						if g.PredatingGuest > 0 {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s %d: %d older backup(s) under this VMID predate this VM's creation (an earlier guest's) and are not listed\n", g.Cluster, g.VMID, g.PredatingGuest)
						}
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
				if g.PredatingGuest > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "  %d older backup(s) under this VMID predate this VM's creation (an earlier guest's) and are not listed\n", g.PredatingGuest)
				}
				rows := make([][]string, 0, len(g.RestorePoints))
				for _, rp := range g.RestorePoints {
					notes := rp.Notes
					if rp.MayBeEarlierGuest {
						notes = strings.TrimSpace("(may be an earlier guest's) " + notes)
					}
					rows = append(rows, []string{rp.TimeRFC3339, rp.Source, rp.VerifyState, strconv.FormatBool(rp.Protected), notes})
				}
				if len(rows) == 0 {
					if g.HiddenUnverified > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  %d restore point(s) read, none verified\n", g.HiddenUnverified)
						if g.Evidence == "partial" {
							fmt.Fprintf(cmd.OutOrStdout(), "  partial: not read: %s\n", strings.Join(g.NotRead, "; "))
						}
					} else if g.Evidence == "partial" {
						fmt.Fprintf(cmd.OutOrStdout(), "  no restore point could be read; not read: %s\n", strings.Join(g.NotRead, "; "))
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "  no restore points")
					}
					continue
				}
				if err := flags.printTable(cmd, []string{"BACKUP TIME", "SOURCE", "VERIFY", "PROTECTED", "NOTES"}, rows); err != nil {
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
	if _, isID := strconv.ParseInt(query, 10, 64); isID != nil {
		// Names repeat: another guest with this name may be hidden.
		if err := guestListHidden(ctx, t.C); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "guests", Error: err.Error() + "; another guest named " + strconv.Quote(query) + " may be among them"})
		}
	}
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
	births := newGuestBirths(t, time.Now(), offline)
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
	pveOnlyNoted := map[string]bool{}
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
			if merr := t.storageUnattributed(s, refs); merr != nil {
				rps, err = nil, merr
			}
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
			if s.Type == "pbs" && !covered {
				notRead = append(notRead, "storage "+key+" (listed through its saved PBS credential, which may not see every backup group)")
				if !pveOnlyNoted[key] {
					pveOnlyNoted[key] = true
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: pveOnlyPBSNote(key)})
				}
			}
			if len(rps) == 0 {
				continue
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
		gaps := map[storageGap]bool{}
		for _, gap := range jobStorageGaps(cov, vmid, g.Node, read, targets) {
			notRead = append(notRead, gap.String())
			gaps[gap] = true
		}
		fails = append(fails, gapFailures(t.Name, gaps)...) // as posture: a gap reaches stderr and --csv runs too
		points = dedupeRestorePoints(points)
		var birth guestBirth // an earlier guest's backups under this VMID are not this one's
		if len(points) > 0 {
			birth = births.read(ctx, g)
		}
		var either []restorePoint
		points = slices.DeleteFunc(points, func(rp restorePoint) bool {
			if rp.GuestType != "" && rp.GuestType != g.Type {
				return false
			}
			if birth.of(rp.Time) == ofEarlierGuest {
				gr.PredatingGuest++
				return true
			}
			return false
		})
		for i, rp := range points {
			if (rp.GuestType == "" || rp.GuestType == g.Type) && birth.of(rp.Time) == ofEitherGuest {
				points[i].MayBeEarlierGuest = true // listed, never withheld: it may be the one to restore
				either = append(either, rp)
			}
		}
		if len(either) > 0 {
			gr.MayBeEarlierGuest = len(either)
			notRead = append(notRead, eitherGuestWhy(either, birth)+" (listed with may_be_earlier_guest)")
		}
		points, unreadable := splitUnreadable(points)
		for _, rp := range unreadable {
			if rp.GuestType == "" || rp.GuestType == g.Type {
				notRead = append(notRead, fmt.Sprintf("%s backup at %s (listed without a readable manifest: still running, or damaged)", rp.Source, rp.TimeRFC3339))
			}
		}
		kept := make([]restorePoint, 0, len(points))
		for _, rp := range points {
			if rp.GuestType != "" && rp.GuestType != g.Type {
				continue
			}
			gr.Readable++
			if verifiedOnly && rp.VerifyState != "ok" {
				continue
			}
			rp.Cluster = t.Name
			kept = append(kept, rp)
		}
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].Time > kept[j].Time })
		cutoff := time.Now().Add(clockSkewTolerance).Unix()
		for _, rp := range kept {
			if rp.Time > cutoff || rp.MayBeEarlierGuest {
				continue // listed, but a node clock ran ahead, or it may not be this VM's
			}
			if gr.NewestAt == "" {
				gr.NewestAt = rp.TimeRFC3339
			}
			if rp.VerifyState == "ok" && gr.NewestVerifiedOKAt == "" {
				gr.NewestVerifiedOKAt = rp.TimeRFC3339
			}
		}
		gr.Count = len(kept)
		gr.HiddenUnverified = gr.Readable - gr.Count
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
	return out, append(fails, births.fails...), nil
}

// nothingReadable: no match has a readable restore point while some match's
// evidence is partial, so "none" is unproven for the guest asked about.
func nothingReadable(guests []guestRestorePoints) bool {
	anyPoint, anyPartial := false, false
	for _, g := range guests {
		anyPoint = anyPoint || g.Count > 0 || g.Readable > 0
		anyPartial = anyPartial || g.Evidence == "partial"
	}
	return !anyPoint && anyPartial
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
