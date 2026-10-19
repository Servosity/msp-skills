// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// taskInfoOperation reports each entity on a backup task with its stored
// size. The documented API's entity records carry no size at all, so this is
// the only way to tell a substantial mailbox from an empty one.
const taskInfoOperation = "getBackupTaskInfo"

// jobsDashboardOperation reports job counts by status. It counts every job,
// including ones that finished with nothing to download, which the chunked
// listing omits.
const jobsDashboardOperation = "getJobsDasboard"

// statusNotAccessible is the size-report status for an entity whose backup
// cannot be read at all.
const statusNotAccessible = "Not Accessible"

// TaskEntityInfo is one entity's stored size, as the portal reports it.
//
// Size is a total across every sub-source as of now. It is not per
// sub-source and not as of any past date, so it tells you a mailbox is
// hopeless, not that a particular sub-source on a particular date is empty.
type TaskEntityInfo struct {
	Entity    string `json:"entity"`
	ExtID     string `json:"extid"`
	Status    string `json:"status"`
	Size      string `json:"size"`
	SizeBytes int64  `json:"-"`
}

// Accessible reports whether the backup can be read at all.
func (t TaskEntityInfo) Accessible() bool {
	return !strings.EqualFold(strings.TrimSpace(t.Status), statusNotAccessible)
}

// TaskEntityInfos returns every entity on a task keyed by lowercase entity
// name, with its size parsed to bytes.
func (s *Session) TaskEntityInfos(ctx context.Context, taskID string) (map[string]TaskEntityInfo, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("portal: task info needs a task id")
	}
	raw, err := s.Call(ctx, taskInfoOperation, map[string]any{
		"taskId": taskID, "_search": false, "rows": 1000, "page": 1,
	})
	if err != nil {
		return nil, err
	}
	var rows []TaskEntityInfo
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("portal: decoding %s rows: %w", taskInfoOperation, err)
	}
	out := make(map[string]TaskEntityInfo, len(rows))
	for _, r := range rows {
		r.SizeBytes = ParseSizeBytes(r.Size)
		out[strings.ToLower(strings.TrimSpace(r.Entity))] = r
	}
	return out, nil
}

// JobCounts is the dashboard's tally of jobs by status.
type JobCounts struct {
	Total     int            `json:"total"`
	ByStatus  map[string]int `json:"by_status"`
	Succeeded int            `json:"succeeded"`
	InProcess int            `json:"in_process"`
	Failed    int            `json:"failed"`
}

// dashboardEnvelope is the shape jobsDashboardOperation returns.
type dashboardEnvelope struct {
	DashboardElements []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	} `json:"dashboardElements"`
	Total int `json:"total"`
}

// JobCounts reads the dashboard tally. This counts EVERY job, so comparing
// it either side of a submit is the only way to tell "created but produced
// nothing" from "never created at all": a job with no files is absent from
// the chunked listing but still counted here.
func (s *Session) JobCounts(ctx context.Context) (JobCounts, error) {
	raw, err := s.Call(ctx, jobsDashboardOperation, map[string]any{
		"_search": false, "rows": downloadInfoPageSize, "page": 1, "sord": "desc",
	})
	if err != nil {
		return JobCounts{}, err
	}
	var env dashboardEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return JobCounts{}, fmt.Errorf("portal: decoding %s: %w", jobsDashboardOperation, err)
	}
	counts := JobCounts{Total: env.Total, ByStatus: map[string]int{}}
	for _, e := range env.DashboardElements {
		counts.ByStatus[e.Status] = e.Count
		switch strings.ToUpper(e.Status) {
		case "SUCCEEDED":
			counts.Succeeded = e.Count
		case "IN_PROCESS":
			counts.InProcess = e.Count
		case "FAILED":
			counts.Failed = e.Count
		}
	}
	return counts, nil
}

// sizePattern matches the portal's human size strings, e.g. "871.26 MB",
// "359.20 KB", "0 Bytes".
var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(bytes?|b|kb|mb|gb|tb|pb)\s*$`)

// sizeUnits maps a unit to its multiplier. The portal's "MB" is really MiB:
// a chunk it called 871.26 MB downloaded as 913,581,909 bytes, which is
// 871.26 * 1024^2.
var sizeUnits = map[string]int64{
	"b": 1, "byte": 1, "bytes": 1,
	"kb": 1 << 10, "mb": 1 << 20, "gb": 1 << 30, "tb": 1 << 40, "pb": 1 << 50,
}

// ParseSizeBytes converts a portal size string to bytes, or -1 when it
// cannot be read. An unreadable size must not be treated as zero: that
// would skip a mailbox that may hold data.
func ParseSizeBytes(s string) int64 {
	m := sizePattern.FindStringSubmatch(s)
	if m == nil {
		return -1
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return -1
	}
	mult, ok := sizeUnits[strings.ToLower(m[2])]
	if !ok {
		return -1
	}
	return int64(val * float64(mult))
}
