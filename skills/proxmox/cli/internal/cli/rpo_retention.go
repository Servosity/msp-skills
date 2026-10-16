// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: which of rpo's missed runs retention may have thinned. A
// keep-daily/weekly/monthly/yearly policy deletes older restore points
// between the ones it keeps, so a run older than its dense horizon with no
// point left may have had one: unproven, never a proven miss.

package cli

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"proxmox-pp-cli/internal/client"
)

// keepPolicy is a prune-backups policy: keep-last, keep-hourly, ... -> count.
type keepPolicy map[string]int64

// UnmarshalJSON accepts the property string Proxmox VE returns
// ("keep-last=3,keep-daily=7") and an object of counts.
func (k *keepPolicy) UnmarshalJSON(b []byte) error {
	*k = keepPolicy{}
	var s string
	if json.Unmarshal(b, &s) == nil {
		for _, kv := range strings.Split(s, ",") {
			name, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); ok && err == nil {
				(*k)[strings.TrimSpace(name)] = n
			}
		}
		return nil
	}
	var m map[string]flexInt
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for name, n := range m {
		(*k)[name] = int64(n)
	}
	return nil
}

var keepPeriods = map[string]int64{"keep-hourly": 3600, "keep-daily": 86400, "keep-weekly": 7 * 86400, "keep-monthly": 28 * 86400, "keep-yearly": 365 * 86400}

// denseSince is the oldest time from which policy keeps the restore point of
// every run in runs (ascending): 0 when it thins nothing (keep-all, none, or
// keep-last alone, which prunes only the oldest: the window cap handles that).
// Each keep-<period>=N keeps every run within N periods only when runs are at
// least a period apart; the policy keeps a point when any option does.
func denseSince(policy keepPolicy, runs []int64, now time.Time) int64 {
	thins := false
	for name, n := range policy {
		thins = thins || (n > 0 && name != "keep-last" && name != "keep-all")
	}
	if policy["keep-all"] > 0 || !thins {
		return 0
	}
	gap := int64(1 << 62)
	for i := 1; i < len(runs); i++ {
		gap = min(gap, runs[i]-runs[i-1])
	}
	since := now.Unix()
	if n := policy["keep-last"]; n > 0 {
		if int(n) >= len(runs) {
			return 0
		}
		since = min(since, runs[len(runs)-int(n)])
	}
	for name, period := range keepPeriods {
		if n := policy[name]; n > 0 && gap >= period {
			since = min(since, now.Unix()-n*period)
		}
	}
	return since
}

// pbsPruneJob is a PBS prune job: it thins a datastore namespace on its own.
type pbsPruneJob struct {
	Store    string     `json:"store"`
	NS       string     `json:"ns"`
	Disable  flexBool   `json:"disable"`
	Schedule string     `json:"schedule"`
	Keep     keepPolicy `json:"-"`
}

func (j *pbsPruneJob) UnmarshalJSON(b []byte) error {
	type plain pbsPruneJob
	if err := json.Unmarshal(b, (*plain)(j)); err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	j.Keep = keepPolicy{}
	for name, raw := range m {
		var n flexInt
		if strings.HasPrefix(name, "keep-") && json.Unmarshal(raw, &n) == nil {
			j.Keep[name] = int64(n)
		}
	}
	return nil
}

// rpoRetention resolves, per covering job, where retention may have thinned
// a guest's restore points. unknown: a policy could not be read, so every
// run older than the guest's newest point may have been pruned.
type rpoRetention struct {
	storage    map[string]keepPolicy
	storageErr bool
	pbsJobs    []pbsPruneJob
	pbsErr     bool
	pbsRead    bool
	ctx        context.Context
	c          *client.Client
}

func newRPORetention(ctx context.Context, c *client.Client) *rpoRetention {
	r := &rpoRetention{storage: map[string]keepPolicy{}, ctx: ctx, c: c}
	var list []struct {
		Storage string     `json:"storage"`
		Prune   keepPolicy `json:"prune-backups"`
	}
	if err := getJSON(ctx, c, "/storage", nil, &list); err != nil {
		r.storageErr = true
	}
	for _, s := range list {
		r.storage[s.Storage] = s.Prune
	}
	return r
}

func (r *rpoRetention) pbsPruneJobs() ([]pbsPruneJob, bool) {
	if !r.pbsRead {
		r.pbsRead = true
		if !r.c.PBSConfigured() || getJSON(r.ctx, r.c, pbsPath("/admin/prune"), nil, &r.pbsJobs) != nil {
			r.pbsErr = true
		}
	}
	return r.pbsJobs, !r.pbsErr
}

// prunableBefore: a run older than this may have lost its restore point to
// job's retention (0: none did). runs are the job's runs, ascending.
func (r *rpoRetention) prunableBefore(job backupJob, node, target string, nodeDef vzdumpDefaults, ref *pbsStorageRef, runs []int64, now time.Time) int64 {
	unknown := now.Unix()
	policy := job.Prune
	switch {
	case len(policy) > 0:
	case r.storageErr:
		return unknown
	case len(r.storage[target]) > 0:
		policy = r.storage[target]
	case nodeDef.err != nil:
		return unknown
	default:
		policy = nodeDef.Prune
	}
	since := denseSince(policy, runs, now)
	if ref == nil {
		return since
	}
	// A PBS prune job on the datastore thins on its own: a point survives
	// only if both keep it.
	jobs, ok := r.pbsPruneJobs()
	if !ok {
		return unknown
	}
	for _, pj := range jobs {
		if bool(pj.Disable) || pj.Schedule == "" || pj.Store != ref.datastore {
			continue
		}
		if pj.NS == "" || ref.namespace == pj.NS || strings.HasPrefix(ref.namespace, pj.NS+"/") {
			since = max(since, denseSince(pj.Keep, runs, now))
		}
	}
	return since
}
