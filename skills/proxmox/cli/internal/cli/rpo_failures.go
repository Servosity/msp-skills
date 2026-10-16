// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the only proof rpo accepts that a scheduled run left no
// restore point: a vzdump task log, from a task that wrote to that run's
// destination, recording the guest's backup failing or stopping, with no
// backup of the guest succeeding in the same window. Absence alone proves
// nothing (a run may not have been due).

package cli

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

// rpoMaxLogs bounds the vzdump task logs rpo reads per cluster; past it no
// miss is proven (a success may sit in an unread log).
const rpoMaxLogs = 400

// vzStorageRe is the storage a vzdump job wrote to, from its first line.
var vzStorageRe = regexp.MustCompile(`starting new backup job: vzdump .*--storage (\S+)`)

// vzRecord is one guest's outcome in one vzdump task.
type vzRecord struct {
	at      int64
	typ     string // qemu | lxc as the log names it; "" when it does not
	storage string // the storage the task wrote to ("" = its node's default)
	node    string
	ok      bool
}

// rpoTaskRecords maps a VMID to its outcomes in the vzdump tasks since start.
// complete is false when a task list or log could not be read or the log cap
// was reached: then no miss on that cluster is proven.
func rpoTaskRecords(ctx context.Context, t fleetTarget, nodes []string, start time.Time) (map[int64][]vzRecord, bool) {
	out, complete := map[int64][]vzRecord{}, true
	perms, err := fetchPrivs(ctx, t.C, false)
	if err != nil {
		return out, false
	}
	logs := 0
	for _, node := range nodes {
		if taskListVisibility(perms, false, node) != nil {
			complete = false
			continue
		}
		tasks, capped, err := fetchTasks(ctx, t.C, false, node, map[string]string{"typefilter": "vzdump", "since": strconv.FormatInt(start.Add(-taskStartSlack).Unix(), 10)})
		if err != nil || capped {
			complete = false
			if err != nil {
				continue
			}
		}
		for _, task := range tasks {
			if task.EndTime == 0 {
				continue
			}
			if logs >= rpoMaxLogs {
				complete = false
				break
			}
			logs++
			lines, err := fetchTaskLog(ctx, t.C, false, node, task.UPID)
			if err != nil {
				complete = false
				continue
			}
			storage := ""
			for _, l := range lines {
				if m := vzStorageRe.FindStringSubmatch(l); m != nil {
					storage = m[1]
					break
				}
			}
			at := int64(task.StartTime)
			seen := map[int64]bool{}
			for _, r := range parseVzdumpLog(lines) {
				seen[r.VMID] = true
				out[r.VMID] = append(out[r.VMID], vzRecord{at: at, typ: r.Type, storage: storage, node: node, ok: r.Result == "ok"})
			}
			if !taskOK(task.Status) && !vzdumpJobFinished(lines) { // aborted: the guests it never reached
				for _, id := range vzdumpJobVMIDs(lines) {
					if !seen[id] {
						out[id] = append(out[id], vzRecord{at: at, storage: storage, node: node})
					}
				}
			}
		}
	}
	return out, complete
}
