// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: fleet datastore runway.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"proxmox-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type capacityRow struct {
	Tenant          string   `json:"tenant"`
	Cluster         string   `json:"cluster"`
	Kind            string   `json:"kind"` // pbs-datastore | pve-storage
	Name            string   `json:"name"`
	Node            string   `json:"node,omitempty"`
	TotalBytes      int64    `json:"total_bytes"`
	UsedBytes       int64    `json:"used_bytes"`
	AvailBytes      int64    `json:"avail_bytes"`
	UsedPct         float64  `json:"used_pct"`
	EstimatedFullAt string   `json:"estimated_full_at,omitempty"`
	DaysToFull      *float64 `json:"days_to_full"`
	GCLastState     string   `json:"gc_last_state,omitempty"`
	GCLastEndAt     string   `json:"gc_last_end_at,omitempty"`
	GCPendingBytes  int64    `json:"gc_pending_bytes,omitempty"`
	Flags           []string `json:"flags"`
	// Attribution is "shared: <clusters>" on a PBS datastore row when another
	// configured cluster also uses that PBS: tenant is then empty.
	Attribution string `json:"attribution,omitempty"`
}

type capacityReport struct {
	Clusters      []string       `json:"clusters"`
	Rows          []capacityRow  `json:"rows"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

func newNovelCapacityCmd(flags *rootFlags) *cobra.Command {
	var within, gcMargin, clientFilter string
	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "PBS datastores ranked by days to full, plus every PVE backup storage by percent used, with GC state",
		Long: "Use this command for days-to-full and GC state across every PBS datastore, and percent used on every PVE backup storage. " +
			"Do NOT use this command for verify failures or corrupt chunks; use 'verify-health' instead.\n\n" +
			"Flags: gc-deadlock-risk (free space below --gc-margin while the last garbage collection failed or never ran: GC needs " +
			"free space to run, so the datastore can no longer clean itself), filling (estimated full within --within), no-forecast (a PVE storage, or a PBS datastore with too few usage " +
			"samples for PBS to estimate: nothing forecasts its days to full, so it is always listed, --within included, and judged by used_pct), nearly-full (free space below --gc-margin). " +
			"PBS forecasts come from its own linear regression over the last month. A datastore another configured cluster also writes to, or a network share (NFS, SMB, CephFS, GlusterFS) another configured cluster also mounts, has attribution 'shared: <clusters>' and no tenant (an entry with only a PBS host owns what one cluster of its own client writes).",
		Example: strings.Trim(`
  proxmox-cli capacity --within 30d
  proxmox-cli capacity --gc-margin 10% --agent
  proxmox-cli capacity --cluster acme --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "capacity")
			}
			var withinDur time.Duration
			if within != "" {
				d, err := parseWindow(within, 0)
				if err != nil {
					return err
				}
				withinDur = d
			}
			margin, err := parsePercent(gcMargin)
			if err != nil {
				return usageErr(err)
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			markSharedPBS(ctx, flags, targets)
			now := time.Now()
			type result struct {
				rows  []capacityRow
				fails []fetchFailure
			}
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (result, error) {
				var r result
				var err error
				r.rows, r.fails, err = collectCapacity(ctx, t, now, margin, withinDur)
				return r, err
			})
			rep := capacityReport{Rows: make([]capacityRow, 0), FetchFailures: failures}
			for _, t := range targets {
				rep.Clusters = append(rep.Clusters, t.Name)
			}
			for _, r := range results {
				for _, row := range r.Value.rows {
					if withinDur > 0 && len(row.Flags) == 0 {
						continue // --within keeps only rows that fill in time or carry a flag
					}
					rep.Rows = append(rep.Rows, row)
				}
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			sortCapacity(rep.Rows)
			printFetchFailures(cmd.ErrOrStderr(), rep.FetchFailures)
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), rep.FetchFailures)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if rowsOnly(flags) {
					out = rep.Rows
				}
				if flags.quiet {
					printQuietRows(cmd.OutOrStdout(), rep.Rows, func(r capacityRow) string { return r.Cluster + " " + r.Name })
				} else if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, capacityKeep...); err != nil {
					return err
				}
				return outcomeErr
			}
			if allFailed {
				return outcomeErr
			}
			if len(rep.Rows) == 0 {
				allClear(cmd.OutOrStdout(), "No backup storage at risk.", len(rep.FetchFailures))
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			for _, r := range rep.Rows {
				days := "-"
				if r.DaysToFull != nil {
					days = fmt.Sprintf("%.1f", *r.DaysToFull)
				}
				rows = append(rows, []string{r.Cluster, r.Kind, r.Name, fmt.Sprintf("%.1f%%", r.UsedPct), days, r.GCLastState, strings.Join(r.Flags, ","), r.Attribution})
			}
			return flags.printTable(cmd, []string{"CLUSTER", "KIND", "NAME", "USED", "DAYS TO FULL", "GC LAST", "FLAGS", "ATTRIBUTION"}, rows)
		},
	}
	f := cmd.Flags()
	f.StringVar(&within, "within", "", "Only storage that fills within this window or carries a flag (e.g. 30d)")
	f.StringVar(&gcMargin, "gc-margin", "5%", "Free-space fraction below which a failed or missing GC is a deadlock risk")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	return cmd
}

func collectCapacity(ctx context.Context, t fleetTarget, now time.Time, margin float64, within time.Duration) ([]capacityRow, []fetchFailure, error) {
	var rows []capacityRow
	var fails []fetchFailure
	// The first PVE call doubles as the reachability check: when it fails, skip
	// the rest of PVE instead of paying the retry budget on every call.
	var refs map[string]pbsStorageRef
	pveErr := errNoPVE
	if t.hasPVE() {
		refs, pveErr = fetchPBSStorageRefs(ctx, t.C)
	}
	var jobs []backupJob
	if pveErr == nil {
		if err := getJSON(ctx, t.C, "/cluster/backup", nil, &jobs); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "backup jobs", Error: err.Error()})
		}
	}
	// A job with no storage of its own writes to its node's vzdump default.
	targets := &jobTargets{ctx: ctx, c: t.C, jobs: map[string]backupJob{}, defaults: map[string]string{}}
	for _, j := range jobs {
		targets.jobs[j.ID] = j
	}
	pbsHost := t.pbsEndpoint()
	pbsRowFor := map[string]bool{} // datastores that got an authoritative PBS row
	var usageErr error             // the PBS did not answer its usage read
	pbsListed := 0                 // datastores the PBS usage list showed this token
	if t.C.PBSConfigured() {
		// PBS reports usage only for the datastores the token may audit: a
		// hidden one could be the one filling up.
		if err := pbsDatastoreListHidden(ctx, t.C); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs datastores", Error: err.Error()})
		}
		var usage []struct {
			Store     string   `json:"store"`
			Total     flexInt  `json:"total"`
			Used      flexInt  `json:"used"`
			Avail     flexInt  `json:"avail"`
			FullDate  *flexInt `json:"estimated-full-date"` // absent: too few samples; 0 or past: not filling
			ErrorText string   `json:"error"`
		}
		if usageErr = getJSON(ctx, t.C, pbsPath("/status/datastore-usage"), nil, &usage); usageErr != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs datastore-usage", Error: usageErr.Error()})
		}
		var gcs []struct {
			Store        string  `json:"store"`
			LastRunState string  `json:"last-run-state"`
			LastRunEnd   flexInt `json:"last-run-endtime"`
			PendingBytes flexInt `json:"pending-bytes"`
		}
		if err := getJSON(ctx, t.C, pbsPath("/admin/gc"), nil, &gcs); err != nil {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs gc", Error: err.Error()})
		}
		pbsListed = len(usage)
		for _, u := range usage {
			if u.ErrorText != "" {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs " + u.Store, Error: u.ErrorText})
				continue
			}
			if u.Total <= 0 {
				// PBS leaves total/used/avail out when it cannot measure the store or
				// the token may not; 0 bytes is not 0% used.
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs " + u.Store, Error: "PBS returned no usage for this datastore (unmeasured, or the token lacks Datastore.Audit on it); its capacity is unknown"})
				continue
			}
			row := capacityRow{Cluster: t.Name, Kind: "pbs-datastore", Name: u.Store,
				TotalBytes: int64(u.Total), UsedBytes: int64(u.Used), AvailBytes: int64(u.Avail), Flags: []string{}}
			row.Tenant, row.Attribution = shareOf(t, t.ownSvcs(), u.Store)
			row.UsedPct = pct(row.UsedBytes, row.TotalBytes)
			// PBS dates an estimate in the past when usage is flat or falling, so a
			// full datastore is detected from free space, not from the forecast.
			if u.FullDate == nil {
				// PBS has too few usage samples to estimate: unknown, so --within
				// must keep it.
				row.Flags = append(row.Flags, "no-forecast")
			} else if est := int64(*u.FullDate); est > now.Unix() {
				d := math.Round(time.Unix(est, 0).Sub(now).Hours()/24*10) / 10
				row.DaysToFull = &d
				row.EstimatedFullAt = unixRFC3339(est)
			}
			for _, g := range gcs {
				if g.Store == u.Store {
					row.GCLastState, row.GCLastEndAt, row.GCPendingBytes = g.LastRunState, unixRFC3339(int64(g.LastRunEnd)), int64(g.PendingBytes)
				}
			}
			free := 1.0
			if row.TotalBytes > 0 {
				free = float64(row.AvailBytes) / float64(row.TotalBytes)
			}
			switch {
			case row.TotalBytes > 0 && free < 0.01:
				zero := 0.0
				row.DaysToFull = &zero
				row.Flags = append(row.Flags, "full")
			case free < margin:
				row.Flags = append(row.Flags, "nearly-full")
			}
			if free < margin && !strings.EqualFold(row.GCLastState, "ok") {
				row.Flags = append(row.Flags, "gc-deadlock-risk")
			}
			pbsRowFor[u.Store] = true
			if within > 0 && row.DaysToFull != nil && *row.DaysToFull <= within.Hours()/24 {
				row.Flags = append(row.Flags, "filling")
			}
			rows = append(rows, row)
		}
	}
	var nodes []string
	err := pveErr
	if err == nil {
		var offline []string
		nodes, offline, err = fetchNodes(ctx, t.C)
		fails = append(fails, offlineFailures(t.Name, offline)...)
	}
	if err == nil && len(nodes) == 0 && len(rows) == 0 {
		err = requireVisibility(ctx, t.C, false, client.PrivSysAudit)
	}
	if err != nil {
		if len(rows) == 0 {
			if errors.Is(err, errNoPVE) && t.C.PBSConfigured() {
				// PBS-only. Its failures are the cause, not the absent PVE host. A
				// PBS that did not answer, or showed this token no datastore at all
				// (no Datastore.Audit), measured nothing: the cluster counts as
				// unreachable (scope-less failure) so the fleet exit code says so.
				switch {
				case usageErr != nil:
					return nil, append(fails, fetchFailure{Cluster: t.Name, Error: "no Backup Server capacity could be read: " + usageErr.Error()}), nil
				case pbsListed == 0:
					return nil, append(fails, fetchFailure{Cluster: t.Name, Error: "the Backup Server lists no datastore this token can see"}), nil
				}
				return nil, fails, nil // it listed datastores it could not measure: the scoped failures say why
			}
			return nil, nil, err // nothing reachable on this cluster
		}
		if !errors.Is(err, errNoPVE) { // a PBS-only profile has no PVE storage to report
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pve nodes", Error: err.Error()})
		}
		return rows, fails, nil
	}
	storages, readers, sfails, _ := backupStorages(ctx, t.C, nodes) // a disabled storage takes no backups
	if len(storages) == 0 && len(sfails) > 0 && len(rows) == 0 {
		return nil, nil, fmt.Errorf("no backup storage could be read on this cluster: %s", sfails[0].Error) // no capacity evidence at all
	}
	// Storage lists are filtered by privilege, not refused: a storage the token
	// may not see is missing, not empty, and could be the one filling up.
	if err := storageListHidden(ctx, t.C); err != nil {
		if len(storages) == 0 && len(rows) == 0 {
			return nil, nil, err
		}
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pve storage", Error: err.Error()})
	}
	// Every enabled job's target, including one inherited from the node's
	// vzdump defaults: a target that cannot be measured is unknown capacity.
	unmeasured := map[string]string{}
	for _, j := range jobs {
		if !j.enabled() {
			continue
		}
		jobNodes := nodes
		if j.Node != "" {
			jobNodes = []string{j.Node}
		}
		for _, n := range jobNodes {
			tgt := targets.target(j.ID, n)
			if strings.HasPrefix(tgt, "dumpdir ") || strings.HasPrefix(tgt, "unresolved ") {
				unmeasured["job "+j.ID] = "backup job " + j.ID + " writes to " + tgt + ", which is not a storage this command can measure; its capacity is unknown"
				continue
			}
			seen := false
			for key := range storages {
				seen = seen || key == tgt || key == tgt+"@"+n || (j.Node == "" && strings.HasPrefix(key, tgt+"@"))
			}
			if !seen {
				unmeasured["storage "+tgt] = "backup job " + j.ID + " writes here, but the storage is not in this token's list (hidden, inactive, or without backup content); its capacity is unknown"
			}
		}
	}
	for _, scope := range sortedKeys(unmeasured) {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: scope, Error: unmeasured[scope]})
	}
	for _, f := range sfails {
		f.Cluster = t.Name
		fails = append(fails, f)
	}
	for _, key := range sortedKeys(storages) {
		s := storages[key]
		if ref, ok := refs[s.Storage]; s.Type == "pbs" && ok && pbsRowFor[ref.datastore] && strings.EqualFold(ref.server, pbsHost) {
			continue // the PBS datastore row above is authoritative for this storage
		}
		row := capacityRow{Tenant: t.Tenant, Cluster: t.Name, Kind: "pve-storage", Name: s.Storage, Node: readers[key],
			TotalBytes: int64(s.Total), UsedBytes: int64(s.Used), AvailBytes: int64(s.Avail), Flags: []string{}}
		if ref, ok := refs[s.Storage]; s.Type == "pbs" && ok && t.own != nil {
			row.Tenant, row.Attribution = shareOf(t, t.own.svc.of(ref.server, ref.fingerprint), ref.datastore)
		}
		if t.own != nil && netStorageType(s.Type) {
			if sharers := t.netMounters(s); len(sharers) > 0 { // one NAS export, not one per client
				row.Tenant, row.Attribution = "", "shared: "+strings.Join(sharers, ", ")
			}
		}
		if bool(s.Shared) {
			row.Node = ""
		}
		row.UsedPct = pct(row.UsedBytes, row.TotalBytes)
		if row.TotalBytes > 0 && float64(row.AvailBytes)/float64(row.TotalBytes) < margin {
			row.Flags = append(row.Flags, "nearly-full")
		}
		// No days-to-full here, so --within cannot rule it out: keep it listed.
		row.Flags = append(row.Flags, "no-forecast")
		rows = append(rows, row)
	}
	return rows, fails, nil
}

func pct(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(used)/float64(total)*1000) / 10
}

func parsePercent(s string) (float64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || v > 100 {
		return 0, fmt.Errorf("--gc-margin %q must be a percentage between 0 and 100", s)
	}
	return v / 100, nil
}

func sortCapacity(rows []capacityRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].DaysToFull, rows[j].DaysToFull
		switch {
		case a != nil && b != nil && *a != *b:
			return *a < *b
		case a != nil && b == nil:
			return true
		case a == nil && b != nil:
			return false
		}
		return rows[i].UsedPct > rows[j].UsedPct
	})
}

// shareOf credits a datastore row to this target's tenant only when no other
// configured cluster writes, or may write, to that datastore.
func shareOf(t fleetTarget, svcs []string, store string) (tenant, attribution string) {
	if sharers := t.datastoreSharers(svcs, store); len(sharers) > 0 {
		return "", "shared: " + strings.Join(sharers, ", ")
	}
	return t.Tenant, ""
}

// errNoPVE: this target has no Proxmox VE host (a PBS-only profile).
var errNoPVE = errors.New("this cluster has no Proxmox VE host configured")
