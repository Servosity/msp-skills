// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: whether retention may have deleted a covering job's older
// restore points. Then a missed run older than a point that was kept may have
// had one (retention never deletes the newest backup): unproven, never a
// proven miss. Only "keep everything" lets an older miss stand as proven.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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

// prunes: some keep-* count is set and keep-all is not (no option at all
// means keep everything).
func (k keepPolicy) prunes() bool {
	if k["keep-all"] > 0 {
		return false
	}
	for name, n := range k {
		if name != "keep-all" && n > 0 {
			return true
		}
	}
	return false
}

// pbsPruneJob is a PBS prune job: it prunes a datastore namespace on its own.
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

// rpoRetention answers, per covering job, whether retention may delete the
// guest's older restore points on that job's target.
type rpoRetention struct {
	ctx      context.Context
	c        *client.Client
	defaults map[string]vzdumpDefaults // node + "\x00" + storage
	pbsJobs  []pbsPruneJob
	pbsErr   error
	pbsRead  bool
	hidden   map[string]string
}

func newRPORetention(ctx context.Context, c *client.Client) *rpoRetention {
	return &rpoRetention{ctx: ctx, c: c, defaults: map[string]vzdumpDefaults{}, hidden: map[string]string{}}
}

// storageDefaults is what a job that sets nothing inherits for target on
// node: /etc/vzdump.conf with the target storage's own prune-backups laid
// over it, as vzdump resolves it (the defaults API takes the storage).
func (r *rpoRetention) storageDefaults(node, target string) vzdumpDefaults {
	key := node + "\x00" + target
	if d, ok := r.defaults[key]; ok {
		return d
	}
	var d vzdumpDefaults
	d.err = getJSON(r.ctx, r.c, pvePath("/nodes/%s/vzdump/defaults", node), map[string]string{"storage": target}, &d)
	r.defaults[key] = d
	return d
}

// mayPrune: retention may delete older restore points of the guest on
// target. Unknown (a policy or prune job list not read) counts as yes.
func (r *rpoRetention) mayPrune(job backupJob, node, target string, ref *pbsStorageRef) bool {
	if r.vzdumpMayPrune(job, node, target) {
		return true
	}
	if ref == nil {
		return false
	}
	// A PBS prune job on the datastore prunes on its own. PBS lists one only
	// to a token that may audit its datastore or namespace path.
	if !r.pbsRead {
		r.pbsRead = true
		if !r.c.PBSConfigured() {
			r.pbsErr = errNoPBS
		} else {
			r.pbsErr = getJSON(r.ctx, r.c, pbsPath("/admin/prune"), nil, &r.pbsJobs)
		}
	}
	if r.pbsErr != nil || r.pruneJobsHidden(ref.datastore, ref.namespace) {
		return true
	}
	for _, pj := range r.pbsJobs {
		if bool(pj.Disable) || pj.Schedule == "" || pj.Store != ref.datastore || !pj.Keep.prunes() {
			continue
		}
		if pj.NS == "" || ref.namespace == pj.NS || strings.HasPrefix(ref.namespace, pj.NS+"/") {
			return true
		}
	}
	return false
}

// pruneJobsHidden: PBS lists a prune job only to a token that may audit its
// datastore or namespace path, so a job on store/ns or an ancestor may be
// missing from the list.
func (r *rpoRetention) pruneJobsHidden(store, ns string) bool {
	paths := []string{nsACLPath(store, "")}
	if ns != "" {
		parts := strings.Split(ns, "/")
		for i := range parts {
			paths = append(paths, nsACLPath(store, strings.Join(parts[:i+1], "/")))
		}
	}
	for _, p := range paths {
		why, seen := r.hidden[p]
		if !seen {
			privs, err := privsAt(r.ctx, r.c, true, p)
			if err != nil || !hasKey(privs, client.PrivDatastoreAudit.String()) {
				why = "hidden"
			}
			r.hidden[p] = why
		}
		if why != "" {
			return true
		}
	}
	return false
}

// vzdumpMayPrune: vzdump prunes after each backup by the job's
// prune-backups, else the target storage's, else vzdump.conf's, unless remove
// is off (job, else vzdump.conf).
func (r *rpoRetention) vzdumpMayPrune(job backupJob, node, target string) bool {
	remove, policy := job.Remove, job.Prune
	if remove != nil && !bool(*remove) {
		return false
	}
	if remove == nil || len(policy) == 0 {
		d := r.storageDefaults(node, target)
		if d.err != nil {
			return true
		}
		if remove == nil {
			remove = d.Remove
		}
		if len(policy) == 0 {
			policy = d.Prune
		}
	}
	return (remove == nil || bool(*remove)) && policy.prunes()
}

// onTarget: rp was written by a job whose target storage is target (for a
// PBS storage: its datastore and namespace, read through either API).
func onTarget(rp restorePoint, target string, ref *pbsStorageRef) bool {
	if ref != nil {
		return strings.HasPrefix(rp.canonical, "pbs:") && rp.pbsStore == ref.datastore && rp.pbsNS == ref.namespace
	}
	return strings.HasPrefix(rp.canonical, "pve:"+target+"@")
}

var errNoPBS = errors.New("no Proxmox Backup Server configured")
