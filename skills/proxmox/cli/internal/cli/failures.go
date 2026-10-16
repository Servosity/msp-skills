// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: failure triage across PVE and PBS task feeds.
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
	"proxmox-pp-cli/internal/cliutil"
)

type failureRow struct {
	Tenant    string   `json:"tenant"`
	Cluster   string   `json:"cluster"`
	Product   string   `json:"product"` // pve | pbs
	Type      string   `json:"type"`    // vzdump | verify | gc | sync | prune
	Target    string   `json:"target"`
	VMID      int64    `json:"vmid,omitempty"`
	GuestName string   `json:"guest_name,omitempty"`
	Result    string   `json:"result"` // failed | incomplete
	Reason    string   `json:"reason"`
	UPID      string   `json:"upid"`
	Node      string   `json:"node"`
	StartedAt string   `json:"started_at"`
	EndedAt   string   `json:"ended_at,omitempty"`
	LogTail   []string `json:"log_tail,omitempty"`
	// Attribution is "shared: <clusters>" for a PBS task on a PBS that another
	// configured cluster also writes to: a PBS task names no PVE cluster, so it
	// belongs to no single client and tenant is left empty.
	Attribution string `json:"attribution,omitempty"`
	pbsService  string // canonical PBS service identity (see pbsServices), for dedupe
}

type failuresReport struct {
	Since         string         `json:"since"`
	Clusters      []string       `json:"clusters"`
	ScannedTasks  int            `json:"scanned_tasks"`
	LogsRead      int            `json:"logs_read"`
	MaxLogs       int            `json:"max_logs"`
	LogCapHit     bool           `json:"log_cap_hit"`
	Rows          []failureRow   `json:"rows"`
	Note          string         `json:"note,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

// pbsWorkerKinds maps PBS worker types to the --type vocabulary.
var pbsWorkerKinds = map[string]string{
	"verify": "verify", "verificationjob": "verify", "verify_group": "verify", "verify_snapshot": "verify",
	"garbage_collection": "gc", "syncjob": "sync", "sync": "sync", "prunejob": "prune", "prune": "prune",
}

func newNovelFailuresCmd(flags *rootFlags) *cobra.Command {
	var since, types, clientFilter string
	var tail, maxLogs int
	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Failed or silently incomplete backup, verify, GC, sync and prune tasks, one row per failed guest or datastore",
		Long: "Use this command for failed or silently incomplete backup, verify, GC, sync and prune tasks in a time window, one row per failed guest or datastore with the reason line. " +
			"Do NOT use this command for whether a guest currently has a good backup; use 'posture' instead. " +
			"Do NOT use it to read one task's complete log; use 'tasks log' instead.\n\n" +
			"A multi-guest vzdump task logs each guest's outcome only inside its task log, so the log is read and every guest is attributed: " +
			"'ERROR: Backup of VM N failed - ...' is failed, and 'Starting Backup of VM N' with no finish and no error is incomplete " +
			"(the silent mid-job stop). A task counts when it ended inside --since (one that started up to 48h earlier is read). PBS tasks are read with errors=1. A PBS task names no PVE cluster: on a PBS that another configured cluster also writes to, " +
			"its row carries attribution 'shared: <clusters>' and no tenant (an entry with only a PBS host owns what one cluster of its own client writes), and a task two profiles both read is listed once.",
		Example: strings.Trim(`
  proxmox-cli failures --since 24h --agent
  proxmox-cli failures --since 7d --type vzdump,verify
  proxmox-cli failures --cluster acme --json --select rows.guest_name,rows.reason`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "failures")
			}
			window, err := parseWindow(since, 24*time.Hour)
			if err != nil {
				return err
			}
			want := map[string]bool{}
			for _, ty := range strings.Split(types, ",") {
				ty = strings.TrimSpace(ty)
				switch ty {
				case "":
				case "all":
					for _, k := range []string{"vzdump", "verify", "gc", "sync", "prune"} {
						want[k] = true
					}
				case "vzdump", "verify", "gc", "sync", "prune":
					want[ty] = true
				default:
					return usageErr(fmt.Errorf("--type %q: use vzdump, verify, gc, sync, prune or all", ty))
				}
			}
			if cliutil.IsDogfoodEnv() && maxLogs > 3 {
				maxLogs = 3
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			sinceT := time.Now().Add(-window)
			if want["verify"] || want["gc"] || want["sync"] || want["prune"] {
				markSharedPBS(ctx, flags, targets)
			}
			type result struct {
				rows          []failureRow
				scanned, logs int
				capHit        bool
				fails         []fetchFailure
			}
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (result, error) {
				var r result
				var err error
				r.rows, r.scanned, r.logs, r.capHit, r.fails, err = collectFailures(ctx, t, sinceT, want, tail, maxLogs)
				return r, err
			})
			rep := failuresReport{Since: sinceT.UTC().Format(time.RFC3339), Rows: make([]failureRow, 0), MaxLogs: maxLogs, FetchFailures: failures}
			for _, t := range targets {
				rep.Clusters = append(rep.Clusters, t.Name)
			}
			seenPBS := map[string]bool{}
			for _, r := range results {
				for _, row := range r.Value.rows {
					// One task read by two profiles of the same PBS endpoint is one
					// row; two PBS servers (a clone keeps the node name) can mint the
					// same UPID, so the endpoint is part of the key.
					if row.Product == "pbs" {
						key := row.pbsService + "\x00" + row.UPID
						if seenPBS[key] {
							continue
						}
						seenPBS[key] = true
					}
					rep.Rows = append(rep.Rows, row)
				}
				rep.ScannedTasks += r.Value.scanned
				rep.LogsRead += r.Value.logs
				rep.LogCapHit = rep.LogCapHit || r.Value.capHit
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			sort.SliceStable(rep.Rows, func(i, j int) bool { return rep.Rows[i].StartedAt > rep.Rows[j].StartedAt })
			switch {
			case len(rep.Rows) == 0 && rep.LogCapHit:
				rep.Note = fmt.Sprintf("no failed tasks in %d task(s) since %s, but only %d log(s) were read (--max-logs %d); incomplete guests in unread logs are not detected", rep.ScannedTasks, rep.Since, rep.LogsRead, maxLogs)
			case len(rep.Rows) == 0 && len(rep.FetchFailures) > 0:
				rep.Note = fmt.Sprintf("no failed tasks among the %d task(s) that could be read since %s; %d fetch failure(s) mean the picture is incomplete", rep.ScannedTasks, rep.Since, len(rep.FetchFailures))
			case len(rep.Rows) == 0:
				rep.Note = fmt.Sprintf("no failed or incomplete tasks in %d task(s) since %s", rep.ScannedTasks, rep.Since)
			}
			printFetchFailures(cmd.ErrOrStderr(), rep.FetchFailures)
			if rep.LogCapHit { // every output form: only JSON has log_cap_hit
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: only %d of the vzdump task logs were read (--max-logs %d); incomplete guests in unread logs are not listed\n", rep.LogsRead, maxLogs)
			}
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), rep.FetchFailures)
			if allFailed {
				rep.Note = ""
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				if rowsOnly(flags) {
					out = rep.Rows
				}
				if flags.quiet {
					printQuietRows(cmd.OutOrStdout(), rep.Rows, func(r failureRow) string { return r.UPID })
				} else if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, failuresKeep...); err != nil {
					return err
				}
				return outcomeErr
			}
			if allFailed {
				return outcomeErr
			}
			if len(rep.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), rep.Note)
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			for _, r := range rep.Rows {
				reason := r.Reason
				if r.Attribution != "" {
					reason = "[" + r.Attribution + "] " + reason
				}
				rows = append(rows, []string{r.StartedAt, r.Cluster, r.Type, r.Target, r.GuestName, r.Result, reason})
			}
			return flags.printTable(cmd, []string{"STARTED", "CLUSTER", "TYPE", "TARGET", "GUEST", "RESULT", "REASON"}, rows)
		},
	}
	f := cmd.Flags()
	f.StringVar(&since, "since", "24h", "Look back this far (e.g. 24h, 3d, 1w)")
	f.StringVar(&types, "type", "all", "Comma-separated task kinds: vzdump, verify, gc, sync, prune, or all")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	f.IntVar(&tail, "log-tail", 5, "Log lines to keep per failed task (0 for none)")
	f.IntVar(&maxLogs, "max-logs", 50, "Maximum task logs to read per cluster (bounds scan cost)")
	return cmd
}

// taskStartSlack: how long before --since a task may have started and still
// end inside the window (a long backup or verify job).
const taskStartSlack = 48 * time.Hour

func collectFailures(ctx context.Context, t fleetTarget, since time.Time, want map[string]bool, tail, maxLogs int) ([]failureRow, int, int, bool, []fetchFailure, error) {
	var rows []failureRow
	var fails []fetchFailure
	scanned, logs := 0, 0
	capHit := false
	var pveErr, pbsErr error
	guestByID := map[int64]pveGuest{}
	birthOf := map[int64]guestBirth{}
	var births *guestBirths // set once the PVE node list is read
	// nameAt: the guest name for a VMID at a task's start; none for a task
	// that may be from before this VM was created (an earlier guest's with the
	// same VMID).
	nameAt := func(id int64, typ string, at int64) string {
		g, ok := guestByID[id]
		if !ok || births == nil || (typ != "" && typ != g.Type) {
			return ""
		}
		b, seen := birthOf[id]
		if !seen {
			b = births.read(ctx, g)
			birthOf[id] = b
		}
		if b.of(at) != ofThisGuest {
			return ""
		}
		return g.Name
	}
	// Proxmox filters task lists by start time; a task that started before
	// the window and ended (failed) inside it counts, so ask from earlier and
	// keep tasks by end time.
	sinceParam := fmt.Sprint(since.Add(-taskStartSlack).Unix())
	endedBefore := func(tk pveTask) bool { return tk.EndTime > 0 && int64(tk.EndTime) < since.Unix() }
	if want["vzdump"] {
		guests, err := fetchGuests(ctx, t.C)
		var nodes []string
		var perms tokenPrivs
		var privErr error
		switch {
		case err != nil:
			// PVE unreachable or unauthorized: record it, skip the rest of PVE, and
			// still collect PBS evidence below.
			pveErr = err
		case len(guests) == 0:
			pveErr = requireVisibility(ctx, t.C, false, client.PrivVMAudit)
		}
		if pveErr == nil {
			for _, g := range guests {
				guestByID[int64(g.VMID)] = g
			}
			var offline []string
			if nodes, offline, err = fetchNodes(ctx, t.C); err != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pve nodes", Error: err.Error()})
			}
			fails = append(fails, offlineFailures(t.Name, offline)...)
			births = newGuestBirths(t, time.Now(), offline)
			perms, privErr = fetchPrivs(ctx, t.C, false)
		}
		// Every node's vzdump tasks merged newest first, so --max-logs drops
		// only the oldest logs cluster-wide.
		type nodeTask struct {
			node string
			task pveTask
		}
		var all []nodeTask
		readable := 0
		for _, node := range nodes {
			scope := "node " + node + " tasks"
			verr := privErr
			if verr == nil {
				verr = taskListVisibility(perms, false, node)
			}
			if verr != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: scope, Error: verr.Error()})
				continue
			}
			tasks, capped, err := fetchTasks(ctx, t.C, false, node, map[string]string{"typefilter": "vzdump", "since": sinceParam})
			if err != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: scope, Error: err.Error()})
				continue
			}
			readable++
			if capped {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: scope, Error: fmt.Sprintf("more than %d vzdump tasks in the window; older ones not scanned", taskListPage*taskListMaxPages)})
			}
			for _, task := range tasks {
				all = append(all, nodeTask{node, task})
			}
		}
		if pveErr == nil && len(nodes) > 0 && readable == 0 {
			pveErr = errors.New("no node's vzdump task list could be read")
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].task.StartTime > all[j].task.StartTime })
		for _, nt := range all {
			node, task := nt.node, nt.task
			if endedBefore(task) {
				continue
			}
			scanned++
			if task.EndTime == 0 {
				continue
			}
			if logs >= maxLogs {
				capHit = true
				if !taskOK(task.Status) {
					// Past the log cap: still report the failed task, just without per-guest attribution.
					rows = append(rows, failureRow{Tenant: t.Tenant, Cluster: t.Name, Product: "pve", Type: "vzdump", Target: task.target(),
						Result: "failed", Reason: task.Status + " (log not read: --max-logs reached)", UPID: task.UPID, Node: node,
						StartedAt: unixRFC3339(int64(task.StartTime)), EndedAt: unixRFC3339(int64(task.EndTime))})
				}
				continue
			}
			logs++
			lines, err := fetchTaskLog(ctx, t.C, false, node, task.UPID)
			if err != nil {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "task log " + task.UPID, Error: err.Error()})
				if !taskOK(task.Status) {
					// The task list already says it failed: report it without per-guest attribution.
					rows = append(rows, failureRow{Tenant: t.Tenant, Cluster: t.Name, Product: "pve", Type: "vzdump", Target: task.target(),
						Result: "failed", Reason: task.Status + " (log not readable)", UPID: task.UPID, Node: node,
						StartedAt: unixRFC3339(int64(task.StartTime)), EndedAt: unixRFC3339(int64(task.EndTime))})
				}
				continue
			}
			base := failureRow{Tenant: t.Tenant, Cluster: t.Name, Product: "pve", Type: "vzdump", UPID: task.UPID, Node: node,
				StartedAt: unixRFC3339(int64(task.StartTime)), EndedAt: unixRFC3339(int64(task.EndTime)), LogTail: tailLines(lines, tail)}
			attributed := false
			for _, g := range parseVzdumpLog(lines) {
				if g.Result == "ok" {
					continue
				}
				attributed = true
				r := base
				r.VMID, r.GuestName, r.Target, r.Result, r.Reason = g.VMID, nameAt(g.VMID, g.Type, int64(task.StartTime)), fmt.Sprint(g.VMID), g.Result, g.Reason
				rows = append(rows, r)
			}
			finished := vzdumpJobFinished(lines)
			if !attributed && !taskOK(task.Status) {
				r := base
				r.Target, r.Result, r.Reason = task.target(), "failed", task.Status
				rows = append(rows, r)
			} else if !taskOK(task.Status) && !finished {
				// Aborted part-way: the guests it never reached are not in its log.
				r := base
				r.Target, r.Result, r.Reason = task.target(), "failed", "job aborted before finishing ("+task.Status+"); remaining guests were not backed up"
				rows = append(rows, r)
			}
			if !taskOK(task.Status) && !finished {
				seen := map[int64]bool{}
				for _, g := range parseVzdumpLog(lines) {
					seen[g.VMID] = true
				}
				for _, id := range vzdumpJobVMIDs(lines) {
					if !seen[id] {
						r := base
						r.VMID, r.GuestName, r.Target, r.Result, r.Reason = id, nameAt(id, "", int64(task.StartTime)), fmt.Sprint(id), "incomplete", "job aborted before reaching this guest"
						rows = append(rows, r)
					}
				}
			}
		}
	}
	wantPBS := want["verify"] || want["gc"] || want["sync"] || want["prune"]
	if wantPBS && t.C.PBSConfigured() {
		var tasks []pveTask
		perms, err := fetchPrivs(ctx, t.C, true)
		if err == nil {
			err = taskListVisibility(perms, true, "localhost")
		}
		if err == nil {
			var capped bool
			tasks, capped, err = fetchTasks(ctx, t.C, true, "localhost", map[string]string{"errors": "1", "since": sinceParam})
			if capped {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs tasks", Error: fmt.Sprintf("more than %d failed PBS tasks in the window; older ones not scanned", taskListPage*taskListMaxPages)})
			}
		}
		if err != nil {
			pbsErr = err
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs tasks", Error: err.Error()})
		}
		tenant, attribution := t.Tenant, ""
		if sharers := t.pbsServiceSharers(); len(sharers) > 0 {
			tenant, attribution = "", "shared: "+strings.Join(sharers, ", ")
		}
		for _, task := range tasks {
			if endedBefore(task) {
				continue
			}
			scanned++
			kind := pbsWorkerKinds[task.kind()]
			if kind == "" || !want[kind] || taskOK(task.Status) {
				continue
			}
			r := failureRow{Tenant: tenant, Cluster: t.Name, Product: "pbs", Type: kind, Target: task.target(), Result: "failed",
				Reason: task.Status, UPID: task.UPID, Node: "localhost", StartedAt: unixRFC3339(int64(task.StartTime)), EndedAt: unixRFC3339(int64(task.EndTime)),
				Attribution: attribution, pbsService: t.pbsServiceID()}
			if tail > 0 && logs < maxLogs {
				logs++
				if lines, err := fetchTaskLog(ctx, t.C, true, "localhost", task.UPID); err == nil {
					r.LogTail = tailLines(lines, tail)
				} else {
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs task log " + task.UPID, Error: err.Error()})
				}
			}
			rows = append(rows, r)
		}
	}
	if wantPBS && pveErr == nil && t.hasPVE() { // a PBS-only profile has no PVE storages
		fails = append(fails, uncheckedPBS(ctx, t, "pbs tasks", "its verify, GC, sync and prune tasks were")...)
	}
	pbsAnswered := wantPBS && t.C.PBSConfigured() && pbsErr == nil
	switch {
	case pveErr != nil && !pbsAnswered:
		return nil, 0, 0, false, nil, pveErr // nothing on this cluster answered
	case pveErr != nil:
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pve", Error: pveErr.Error()})
	case !want["vzdump"] && !pbsAnswered:
		// PBS-only query (e.g. --type verify) that obtained no evidence.
		if pbsErr != nil {
			return nil, 0, 0, false, nil, pbsErr
		}
		return nil, 0, 0, false, nil, errors.New("this cluster has no Proxmox Backup Server configured, so PBS task failures cannot be checked")
	}
	if births != nil {
		fails = append(fails, births.fails...) // a guest whose name was left off its failed task
	}
	return rows, scanned, logs, capHit, fails, nil
}

func tailLines(lines []string, n int) []string {
	if n <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return append([]string{}, lines...)
}

// uncheckedPBS names the Proxmox Backup Servers this cluster's storages back
// up to whose tasks were not read: every PBS but the configured one, or all
// of them when none is configured. A storage list the token cannot fully see,
// or an unreadable storage config, leaves that unknown.
func uncheckedPBS(ctx context.Context, t fleetTarget, scope, what string) []fetchFailure {
	fail := func(msg string) []fetchFailure {
		return []fetchFailure{{Cluster: t.Name, Scope: scope, Error: msg}}
	}
	refs, err := fetchPBSStorageRefs(ctx, t.C)
	if err != nil {
		return fail("the storage config could not be read (" + err.Error() + "), so whether a Proxmox Backup Server other than the configured one holds this cluster's backups is unknown, and " + what + " not checked there")
	}
	var other []string
	for _, id := range sortedKeys(refs) {
		if !t.C.PBSConfigured() || !t.samePBS(refs[id]) {
			other = append(other, id+" -> "+refs[id].server)
		}
	}
	var out []fetchFailure
	if len(other) > 0 {
		out = fail("storage " + strings.Join(other, ", ") + " backs up to a Proxmox Backup Server that is not this cluster's configured PBS host, so " + what + " not checked; set PROXMOX_PBS_HOST (or pbs_host) to it")
	}
	if err := storageListHidden(ctx, t.C); err != nil {
		out = append(out, fetchFailure{Cluster: t.Name, Scope: scope, Error: "a storage hidden from this token may back up to a Proxmox Backup Server, and " + what + " not checked there: " + err.Error()})
	}
	return out
}
