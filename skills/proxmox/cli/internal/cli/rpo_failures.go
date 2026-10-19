// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the only proof rpo accepts that a scheduled run left no
// restore point: the log of a vzdump task that wrote to that run's
// destination (an explicit --storage), recording the guest's backup failing
// or stopping, with no backup of the guest succeeding in the same window and
// every task that overlaps the window read and finished. Absence alone
// proves nothing (a run may not have been due).

package cli

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

// rpoMaxLogs bounds the vzdump task logs rpo reads per cluster; a window
// whose logs it did not read proves nothing.
const rpoMaxLogs = 400

// vzStorageRe is the storage a vzdump job wrote to, from its first line.
var vzStorageRe = regexp.MustCompile(`starting new backup job: vzdump .*--storage (\S+)`)

// vzTask is one vzdump task and, once read, what its log says.
type vzTask struct {
	node     string
	task     pveTask
	read     bool
	unread   bool // the log could not be read
	storage  string
	outcomes []vzdumpGuestResult // incl. "incomplete" for guests an aborted job never reached
}

// rpoEvidence reads vzdump task logs on demand, per run window, for one cluster.
type rpoEvidence struct {
	ctx      context.Context
	t        fleetTarget
	tasks    []*vzTask
	complete bool // every node's task list (archived and running) was read
	logs     int
}

func newRPOEvidence(ctx context.Context, t fleetTarget, nodes []string, start time.Time) *rpoEvidence {
	e := &rpoEvidence{ctx: ctx, t: t, complete: true}
	perms, err := fetchPrivs(ctx, t.C, false)
	if err != nil {
		e.complete = false
		return e
	}
	for _, node := range nodes {
		if taskListVisibility(perms, false, node) != nil {
			e.complete = false
			continue
		}
		// source=all: running tasks too (Proxmox lists archived ones by default).
		tasks, capped, err := fetchTasks(ctx, t.C, false, node, map[string]string{"typefilter": "vzdump", "source": "all", "since": strconv.FormatInt(start.Add(-taskStartSlack).Unix(), 10)})
		if err != nil || capped {
			e.complete = false
		}
		for _, tk := range tasks {
			e.tasks = append(e.tasks, &vzTask{node: node, task: tk})
		}
	}
	return e
}

// window reports, for a guest of type typ, whether in [from, until) some task
// that wrote to destination key logged its backup failing or stopping, and
// whether every task overlapping the window was finished and read with no
// success of the guest. A miss is proven only when both hold.
func (e *rpoEvidence) window(vmid int64, typ, key string, refs map[string]pbsStorageRef, from, until int64) bool {
	if e == nil || !e.complete {
		return false
	}
	failedHere := false
	for _, vt := range e.tasks {
		start, end := int64(vt.task.StartTime), int64(vt.task.EndTime)
		if start >= until || (end != 0 && end < from) {
			continue // does not overlap the window
		}
		if end == 0 {
			return false // still running: its outcome for this guest is not known yet
		}
		if !e.load(vt) {
			return false
		}
		for _, r := range vt.outcomes {
			if r.VMID != vmid || (r.Type != "" && r.Type != typ) {
				continue
			}
			if r.Result == "ok" {
				return false // backed up in this window: its point may be pruned or elsewhere
			}
			if vt.storage != "" && start >= from {
				var ref *pbsStorageRef
				if rf, ok := refs[vt.storage]; ok {
					ref = &rf
				}
				failedHere = failedHere || destinationKey(vt.storage, ref) == key
			}
		}
	}
	return failedHere
}

func (e *rpoEvidence) load(vt *vzTask) bool {
	if vt.read {
		return !vt.unread
	}
	if e.logs >= rpoMaxLogs {
		return false
	}
	vt.read = true
	e.logs++
	lines, err := fetchTaskLog(e.ctx, e.t.C, false, vt.node, vt.task.UPID)
	if err != nil {
		vt.unread = true
		return false
	}
	for _, l := range lines {
		if m := vzStorageRe.FindStringSubmatch(l); m != nil {
			vt.storage = m[1]
			break
		}
	}
	vt.outcomes = parseVzdumpLog(lines)
	if !taskOK(vt.task.Status) && !vzdumpJobFinished(lines) { // aborted: the guests it never reached
		seen := map[int64]bool{}
		for _, r := range vt.outcomes {
			seen[r.VMID] = true
		}
		for _, id := range vzdumpJobVMIDs(lines) {
			if !seen[id] {
				vt.outcomes = append(vt.outcomes, vzdumpGuestResult{VMID: id, Result: "incomplete"})
			}
		}
	}
	return true
}
