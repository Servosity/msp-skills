// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the only proof rpo accepts that a scheduled run left no
// restore point: a vzdump task log recording the guest's backup failing or
// stopping. Absence alone proves nothing (a run may not have been due).

package cli

import (
	"context"
	"strconv"
	"time"
)

// rpoMaxFailedLogs bounds the task logs rpo reads per cluster.
const rpoMaxFailedLogs = 200

// failureRecord is one vzdump task whose log records a guest's backup failing.
type failureRecord struct {
	at  int64
	typ string // qemu | lxc as the log names it; "" when it does not
}

// rpoFailureRecords maps a VMID to the vzdump tasks since
// start whose log records its backup failing or stopping. Only tasks that did
// not end OK can hold one. A list or log that cannot be read leaves no
// record, so the misses it could prove stay unproven.
func rpoFailureRecords(ctx context.Context, t fleetTarget, nodes []string, start time.Time) map[int64][]failureRecord {
	out := map[int64][]failureRecord{}
	perms, err := fetchPrivs(ctx, t.C, false)
	if err != nil {
		return out
	}
	logs := 0
	for _, node := range nodes {
		if taskListVisibility(perms, false, node) != nil {
			continue
		}
		tasks, _, err := fetchTasks(ctx, t.C, false, node, map[string]string{"typefilter": "vzdump", "since": strconv.FormatInt(start.Add(-taskStartSlack).Unix(), 10)})
		if err != nil {
			continue
		}
		for _, task := range tasks {
			if task.EndTime == 0 || taskOK(task.Status) || logs >= rpoMaxFailedLogs {
				continue
			}
			logs++
			lines, err := fetchTaskLog(ctx, t.C, false, node, task.UPID)
			if err != nil {
				continue
			}
			at := int64(task.StartTime)
			seen := map[int64]bool{}
			for _, r := range parseVzdumpLog(lines) {
				seen[r.VMID] = true
				if r.Result == "failed" || r.Result == "incomplete" {
					out[r.VMID] = append(out[r.VMID], failureRecord{at: at, typ: r.Type})
				}
			}
			if !vzdumpJobFinished(lines) { // aborted: the guests it never reached
				for _, id := range vzdumpJobVMIDs(lines) {
					if !seen[id] {
						out[id] = append(out[id], failureRecord{at: at})
					}
				}
			}
		}
	}
	return out
}
