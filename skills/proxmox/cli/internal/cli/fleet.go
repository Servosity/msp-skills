// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared fleet plumbing for the backup-assurance commands
// (posture, backup coverage, verify-health, failures, capacity,
// restore-points, rpo). Every call here is a GET.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/cliutil"
)

// fleetConcurrency caps clusters visited at once. Within a cluster calls are
// sequential, so each Proxmox host sees at most one request from us at a time
// (pveproxy runs 3 workers by default).
const fleetConcurrency = 4

// fleetTarget is one cluster a fleet command visits.
type fleetTarget struct {
	Name    string
	Tenant  string // client display name, falls back to the cluster name
	Profile clusterProfile
	C       *client.Client
	Err     error // client construction failed; reported as this cluster's failure
	// sharedPBS: PBS namespaces (pbsStorageRef.sourceKey) this cluster shares
	// with other configured clusters, mapped to every cluster writing there.
	sharedPBS map[string][]string
}

// fleetTargets resolves the clusters to visit: --cluster narrows to one entry;
// otherwise every clusters.toml entry (optionally filtered by --client); with no
// clusters.toml, the single cluster configured by PROXMOX_* env vars ("default").
// A profile whose client cannot be built (for example a malformed pin) becomes
// a target carrying Err: runFleet records it as that cluster's failure, so one
// bad entry never aborts the whole fleet.
func fleetTargets(flags *rootFlags, clientFilter string) ([]fleetTarget, error) {
	all, path, err := loadClusters()
	if err != nil {
		return nil, configErr(err)
	}
	if name := strings.TrimSpace(selectedCluster); name != "" {
		cl, ok := all[name]
		if !ok {
			return nil, configErr(fmt.Errorf("cluster %q is not defined in %s", name, path))
		}
		if err := cl.validate(name); err != nil {
			return nil, configErr(fmt.Errorf("%s: %w", path, err))
		}
		all = map[string]clusterProfile{name: cl}
	}
	if len(all) == 0 {
		c, err := flags.newClient()
		if err != nil {
			return nil, err
		}
		return []fleetTarget{{Name: "default", Tenant: "default", C: c}}, nil
	}
	var targets []fleetTarget
	for _, name := range sortedClusterNames(all) {
		cl := all[name]
		tenant := cl.Client
		if tenant == "" {
			tenant = name
		}
		if clientFilter != "" && !strings.EqualFold(tenant, clientFilter) && !strings.EqualFold(name, clientFilter) {
			continue
		}
		var c *client.Client
		err := cl.validate(name)
		if err == nil {
			c, err = newClusterClient(flags, cl)
		}
		targets = append(targets, fleetTarget{Name: name, Tenant: tenant, Profile: cl, C: c, Err: err})
	}
	if len(targets) == 0 {
		return nil, usageErr(fmt.Errorf("no configured cluster matches --client %q", clientFilter))
	}
	return targets, nil
}

// usable reports whether the target's client was built (see fleetTargets).
func (t fleetTarget) usable() bool { return t.Err == nil && t.C != nil }

// runFleet visits every target concurrently and keeps per-cluster failures
// separate from results so no failed cluster becomes a phantom empty one.
func runFleet[T any](ctx context.Context, targets []fleetTarget, fn func(context.Context, fleetTarget) (T, error)) ([]cliutil.FanoutResult[T], []fetchFailure) {
	guarded := func(ctx context.Context, t fleetTarget) (T, error) {
		if !t.usable() {
			var zero T
			return zero, t.Err
		}
		return fn(ctx, t)
	}
	results, errs := cliutil.FanoutRun(ctx, targets, func(t fleetTarget) string { return t.Name }, guarded,
		cliutil.WithConcurrency(fleetConcurrency))
	failures := make([]fetchFailure, 0, len(errs))
	for _, e := range errs {
		failures = append(failures, fetchFailure{Cluster: e.Source, Error: e.Err.Error()})
	}
	return results, failures
}

// tenantFailures counts, per client, the clusters runFleet could not read at
// all (scope-less failures) and the scoped fetch failures on the rest, so a
// per-client rollup carries its own incompleteness instead of silently
// dropping a client, or the guests and storages that were not read.
func tenantFailures(targets []fleetTarget, failures []fetchFailure) (unreachable, scoped map[string]int) {
	tenant := map[string]string{}
	for _, t := range targets {
		tenant[t.Name] = t.Tenant
	}
	unreachable, scoped = map[string]int{}, map[string]int{}
	for _, f := range failures {
		tn, ok := tenant[f.Cluster]
		switch {
		case !ok:
		case f.Scope == "":
			unreachable[tn]++
		default:
			scoped[tn]++
		}
	}
	return unreachable, scoped
}

type fetchFailure struct {
	Cluster string `json:"cluster"`
	Scope   string `json:"scope,omitempty"`
	Error   string `json:"error"`
}

func (f fetchFailure) String() string {
	if f.Scope == "" {
		return f.Cluster + ": " + f.Error
	}
	return f.Cluster + " " + f.Scope + ": " + f.Error
}

// Row keys --agent/--compact must keep for each fleet command: compact
// otherwise drops sparse keys (present in under 80% of rows), which are
// exactly the answers (age, last attempt, failing disk, guest name).
var (
	postureKeep  = []string{"rows", "clients", "warnings", "fetch_failures", "red", "amber", "clusters", "tenant", "cluster", "vmid", "asset_name", "asset_id", "verdict", "severity", "reason", "covered_by_job", "age_hours", "last_success_at", "verify_state", "last_verified_ok_at", "last_attempt_status", "last_attempt_at", "offsite_last_success_at", "guests", "stale", "never", "uncovered", "verify_failed", "unverified", "last_attempt_failed", "attempt_unknown", "archives_incomplete", "unproven", "clusters_unreachable"}
	coverageKeep = []string{"rows", "fetch_failures", "uncovered_guests", "guests_checked", "cluster", "vmid", "name", "disk", "reason", "detail", "disabled_jobs", "disagrees_with_upstream"}
	verifyKeep   = []string{"stores", "groups", "skipped_clusters", "fetch_failures", "cluster", "store", "namespace", "group", "status", "snapshots", "failed", "never_verified", "outdated", "first_failure_at", "newest_verified_ok_at", "newest_is_bad", "failing_groups", "failed_snapshots", "never_verified_snapshots", "gc_still_bad_chunks", "namespaces_without_verify_job", "attribution"}
	failuresKeep = []string{"rows", "fetch_failures", "scanned_tasks", "logs_read", "log_cap_hit", "note", "cluster", "product", "type", "target", "vmid", "guest_name", "result", "reason", "upid", "started_at"}
	capacityKeep = []string{"rows", "fetch_failures", "cluster", "kind", "name", "node", "used_pct", "days_to_full", "estimated_full_at", "gc_last_state", "gc_pending_bytes", "prune_jobs", "flags"}
	restoreKeep  = []string{"query", "guests", "fetch_failures", "cluster", "vmid", "name", "covered_by_jobs", "count", "newest_at", "newest_verified_ok_at", "restore_points", "backup_time", "source", "verify_state", "protected", "encrypted", "prune_mark", "evidence", "not_read"}
	rpoKeep      = []string{"rows", "clients", "fetch_failures", "days", "window_start", "tenant", "cluster", "vmid", "name", "missed_runs", "scheduled_runs", "missed_run_times", "largest_gap_hours", "verified_ok_days", "met_schedule", "window_capped", "note", "guests", "guests_met_schedule", "worst_gap_hours", "evidence", "points_unread", "guests_unproven", "missed_runs_unproven", "guests_without_restore_points", "clusters_unreachable"}
)

func printFetchFailures(w io.Writer, failures []fetchFailure) {
	for _, f := range failures {
		fmt.Fprintf(w, "warning: %s\n", f)
	}
}

// fleetOutcome classifies cluster-level failures (a cluster whose first call
// failed; runFleet records those without a scope). When every visited cluster
// failed there is no data at all, so the command must say so and exit 5
// instead of ending on a healthy-looking empty result. allFailed tells the
// caller to skip human-mode "all clear" prose.
func fleetOutcome(w io.Writer, visited int, failures []fetchFailure) (allFailed bool, err error) {
	down := map[string]bool{}
	for _, f := range failures {
		if f.Scope == "" {
			down[f.Cluster] = true
		}
	}
	switch {
	case visited > 0 && len(down) == visited:
		msg := fmt.Sprintf("No data: %d of %d cluster(s) unreachable or unauthorized", len(down), visited)
		fmt.Fprintln(w, msg)
		return true, apiErr(errors.New(msg))
	case len(down) > 0:
		fmt.Fprintf(w, "PARTIAL: %d of %d cluster(s) unreachable or unauthorized; results cover the rest\n", len(down), visited)
	}
	return false, nil
}

// --- tolerant decoding: PVE sends booleans as 0/1, PBS as true/false, and some
// numbers as strings.

type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		*b = true
	default:
		*b = false
	}
	return nil
}

type flexInt int64

func (n *flexInt) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = 0
		return nil
	}
	*n = flexInt(f)
	return nil
}

func getJSON(ctx context.Context, c *client.Client, path string, params map[string]string, out any) error {
	if strings.HasPrefix(path, "/") && strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("this cluster has no Proxmox VE host configured (set pve_host or pve_base_url in clusters.toml)")
	}
	// Assurance reads must see current state: never answer from the response cache.
	raw, err := c.GetNoCache(ctx, path, params)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func pvePath(format string, args ...string) string {
	esc := make([]any, len(args))
	for i, a := range args {
		esc[i] = url.PathEscape(a)
	}
	return fmt.Sprintf(format, esc...)
}

func pbsPath(format string, args ...string) string {
	return client.PBSPath(pvePath(format, args...))
}

// --- PVE inventory

type pveGuest struct {
	ID       string   `json:"id"`
	VMID     flexInt  `json:"vmid"`
	Name     string   `json:"name"`
	Node     string   `json:"node"`
	Type     string   `json:"type"` // qemu | lxc
	Status   string   `json:"status"`
	Template flexBool `json:"template"`
	Tags     string   `json:"tags"`
	Pool     string   `json:"pool"`
}

func (g pveGuest) hasTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, t := range strings.FieldsFunc(g.Tags, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func fetchGuests(ctx context.Context, c *client.Client) ([]pveGuest, error) {
	var res []pveGuest
	if err := getJSON(ctx, c, "/cluster/resources", map[string]string{"type": "vm"}, &res); err != nil {
		return nil, fmt.Errorf("cluster resources: %w", err)
	}
	out := res[:0]
	for _, g := range res {
		if g.Type == "qemu" || g.Type == "lxc" {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VMID < out[j].VMID })
	return out, nil
}

type pveNode struct {
	Node   string `json:"node"`
	Status string `json:"status"`
}

// fetchNodes splits the cluster's nodes into online ones and the rest. An
// offline node's tasks and local backup storage cannot be read: callers record
// it (offlineFailures) as missing evidence, never as an empty node.
func fetchNodes(ctx context.Context, c *client.Client) (online, offline []string, err error) {
	var nodes []pveNode
	if err := getJSON(ctx, c, "/nodes", nil, &nodes); err != nil {
		return nil, nil, fmt.Errorf("nodes: %w", err)
	}
	for _, n := range nodes {
		if n.Status == "" || n.Status == "online" {
			online = append(online, n.Node)
		} else {
			offline = append(offline, n.Node)
		}
	}
	sort.Strings(online)
	sort.Strings(offline)
	return online, offline, nil
}

func offlineFailures(cluster string, offline []string) []fetchFailure {
	out := make([]fetchFailure, 0, len(offline))
	for _, n := range offline {
		out = append(out, fetchFailure{Cluster: cluster, Scope: "node " + n,
			Error: "node is not online; its tasks and local backup storage could not be read"})
	}
	return out
}

// storageGap is a covering job whose backup target was not read.
type storageGap struct{ job, target string }

func (g storageGap) String() string { return "job " + g.job + " -> " + g.target }

// jobTargets resolves where an enabled job writes when it backs up a guest on
// a given node: its storage, else its dumpdir (never readable as a storage, so
// always a gap), else that node's default from vzdump.conf
// (/nodes/{node}/vzdump/defaults). Proxmox returns the "local" fallback
// explicitly; a defaults answer with neither storage nor dumpdir (dumpdir is
// filtered without Sys.Audit on the node) or an unreadable one is
// "unresolved", itself a gap.
type jobTargets struct {
	ctx      context.Context
	c        *client.Client
	jobs     map[string]backupJob
	defaults map[string]string
}

func newJobTargets(ctx context.Context, c *client.Client, cov *coverage) *jobTargets {
	r := &jobTargets{ctx: ctx, c: c, jobs: map[string]backupJob{}, defaults: map[string]string{}}
	for _, j := range cov.Jobs {
		r.jobs[j.ID] = j
	}
	return r
}

func (r *jobTargets) target(jobID, node string) string {
	j := r.jobs[jobID]
	switch {
	case j.Storage != "":
		return j.Storage
	case j.Dumpdir != "":
		return "dumpdir " + j.Dumpdir
	}
	if v, ok := r.defaults[node]; ok {
		return v
	}
	var d struct {
		Storage string `json:"storage"`
		Dumpdir string `json:"dumpdir"`
	}
	v := "unresolved default on node " + node
	if err := getJSON(r.ctx, r.c, pvePath("/nodes/%s/vzdump/defaults", node), nil, &d); err != nil {
		v += " (" + err.Error() + ")"
	} else if d.Storage != "" {
		v = d.Storage
	} else if d.Dumpdir != "" {
		v = "dumpdir " + d.Dumpdir
	}
	r.defaults[node] = v
	return v
}

// jobStorageGaps lists the covering jobs of vmid (a guest on node) whose
// target was not read. read holds backupStorages keys: "<storage>" for shared
// storage and "<storage>@<node>" for node-local storage, so reading local on
// one node never vouches for local on another.
func jobStorageGaps(cov *coverage, vmid int64, node string, read map[string]bool, targets *jobTargets) []storageGap {
	var gaps []storageGap
	for _, id := range cov.EnabledFor[vmid] {
		t := targets.target(id, node)
		if !read[t] && !read[t+"@"+node] {
			gaps = append(gaps, storageGap{id, t + " (guest on node " + node + ")"})
		}
	}
	return gaps
}

// gapFailures turns the distinct gaps into scoped fetch failures, so gates that
// trip on missing evidence (posture --fail-on-red) see them.
func gapFailures(cluster string, gaps map[storageGap]bool) []fetchFailure {
	keys := make([]storageGap, 0, len(gaps))
	for g := range gaps {
		keys = append(keys, g)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	out := make([]fetchFailure, 0, len(keys))
	for _, g := range keys {
		out = append(out, fetchFailure{Cluster: cluster, Scope: "job " + g.job,
			Error: "backup target " + g.target + " was not read (not visible to this token, inactive, failed, or not a storage); the job's guests are unproven"})
	}
	return out
}

func joinGaps(gaps []storageGap) string {
	parts := make([]string, 0, len(gaps))
	for _, g := range gaps {
		parts = append(parts, g.String())
	}
	return strings.Join(parts, ", ")
}

// allClear prints a human-mode all-clear line, qualified when fetch failures
// mean part of the fleet was not read.
func allClear(w io.Writer, msg string, failures int) {
	if failures > 0 {
		fmt.Fprintf(w, "%s (only for what could be read: %d fetch failure(s) above leave the rest unverified)\n", strings.TrimSuffix(msg, "."), failures)
		return
	}
	fmt.Fprintln(w, msg)
}

type backupJob struct {
	ID       string    `json:"id"`
	Enabled  *flexBool `json:"enabled"`
	Schedule string    `json:"schedule"`
	Storage  string    `json:"storage"`
	Dumpdir  string    `json:"dumpdir"`
	Node     string    `json:"node"`
	Comment  string    `json:"comment"`
	Exclude  string    `json:"exclude"`
	// PruneBackups is the job's retention (object or property string).
	PruneBackups json.RawMessage `json:"prune-backups"`
	NextRun      flexInt         `json:"next-run"`
}

// excludes reports whether the job's exclude list names vmid.
func (j backupJob) excludes(vmid int64) bool {
	for _, f := range strings.FieldsFunc(j.Exclude, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64); err == nil && n == vmid {
			return true
		}
	}
	return false
}

// enabled: Proxmox omits the key when the job is enabled (default 1).
func (j backupJob) enabled() bool { return j.Enabled == nil || bool(*j.Enabled) }

type includedVolumes struct {
	Children []struct {
		ID       flexInt `json:"id"`
		Name     string  `json:"name"`
		Type     string  `json:"type"`
		Children []struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			Included flexBool `json:"included"`
			Reason   string   `json:"reason"`
		} `json:"children"`
	} `json:"children"`
}

// coverage is the server-resolved job selection for one cluster.
type coverage struct {
	Jobs         []backupJob
	EnabledFor   map[int64][]string // vmid -> enabled job ids that include it
	DisabledFor  map[int64][]string // vmid -> disabled job ids that include it
	DiskExcluded map[int64][]string // vmid -> volume keys excluded by every enabled job
	NotBackedUp  map[int64]bool     // upstream /cluster/backup-info/not-backed-up
}

func fetchCoverage(ctx context.Context, c *client.Client) (*coverage, error) {
	cov := &coverage{EnabledFor: map[int64][]string{}, DisabledFor: map[int64][]string{}, DiskExcluded: map[int64][]string{}, NotBackedUp: map[int64]bool{}}
	if err := getJSON(ctx, c, "/cluster/backup", nil, &cov.Jobs); err != nil {
		return nil, fmt.Errorf("backup jobs: %w", err)
	}
	excludedBy := map[int64]map[string]int{} // vmid -> volume -> enabled jobs excluding it
	enabledJobs := map[int64]int{}
	for _, job := range cov.Jobs {
		var iv includedVolumes
		if err := getJSON(ctx, c, pvePath("/cluster/backup/%s/included_volumes", job.ID), nil, &iv); err != nil {
			return nil, fmt.Errorf("backup job %s included volumes: %w", job.ID, err)
		}
		for _, g := range iv.Children {
			vmid := int64(g.ID)
			if !job.enabled() {
				cov.DisabledFor[vmid] = append(cov.DisabledFor[vmid], job.ID)
				continue
			}
			cov.EnabledFor[vmid] = append(cov.EnabledFor[vmid], job.ID)
			enabledJobs[vmid]++
			for _, vol := range g.Children {
				if !vol.Included {
					if excludedBy[vmid] == nil {
						excludedBy[vmid] = map[string]int{}
					}
					excludedBy[vmid][vol.ID]++
				}
			}
		}
	}
	for vmid, vols := range excludedBy {
		for vol, n := range vols {
			if n == enabledJobs[vmid] {
				cov.DiskExcluded[vmid] = append(cov.DiskExcluded[vmid], vol)
			}
		}
		sort.Strings(cov.DiskExcluded[vmid])
	}
	var nbu []struct {
		VMID flexInt `json:"vmid"`
	}
	if err := getJSON(ctx, c, "/cluster/backup-info/not-backed-up", nil, &nbu); err != nil {
		return nil, fmt.Errorf("not-backed-up: %w", err)
	}
	for _, g := range nbu {
		cov.NotBackedUp[int64(g.VMID)] = true
	}
	return cov, nil
}

// --- restore points

type verification struct {
	State string `json:"state"`
	UPID  string `json:"upid"`
}

// restorePoint is one backup archive, from a PVE storage listing or PBS.
type restorePoint struct {
	Cluster     string `json:"cluster,omitempty"`
	VMID        int64  `json:"vmid"`
	GuestType   string `json:"guest_type,omitempty"` // qemu | lxc
	Time        int64  `json:"-"`
	TimeRFC3339 string `json:"backup_time"`
	Source      string `json:"source"` // pve-storage:<id> | pbs:<store>[/<ns>]
	Volume      string `json:"volid,omitempty"`
	Size        int64  `json:"size_bytes,omitempty"`
	VerifyState string `json:"verify_state"` // ok | failed | none | n/a
	VerifyUPID  string `json:"verify_upid,omitempty"`
	Protected   bool   `json:"protected"`
	Encrypted   bool   `json:"encrypted"`
	Notes       string `json:"notes,omitempty"`
	PBSBacked   bool   `json:"-"`
	PruneMark   string `json:"prune_mark,omitempty"`
	// unreadable: listed without a readable manifest (PBS omits the size, PVE's
	// PBS plugin reports 1, a dir archive is 0 bytes): a backup still running,
	// or a damaged one. Not a restore point, and not proof of none either.
	unreadable bool
	// canonical identifies the underlying archive across views (a PVE
	// storage listing and the PBS API can both show the same PBS snapshot).
	canonical   string
	pbsStore    string // PBS datastore/namespace holding the archive, when PBS-backed
	pbsNS       string
	storageID   string
	storageNode string
}

type storageInfo struct {
	Storage string    `json:"storage"`
	Type    string    `json:"type"`
	Shared  flexBool  `json:"shared"`
	Active  *flexBool `json:"active"`
	Enabled *flexBool `json:"enabled"`
	Content string    `json:"content"`
	Used    flexInt   `json:"used"`
	Total   flexInt   `json:"total"`
	Avail   flexInt   `json:"avail"`
	UsedFr  float64   `json:"used_fraction"`
}

func (s storageInfo) active() bool { return s.Active == nil || bool(*s.Active) }

// backupStorages returns each backup-content storage once, with a node that
// can read it (shared storages are visited from the first node only).
// disabled holds the backup storages disabled in the config: Proxmox cannot
// list them, yet they may hold archives from before (a former job target).
func backupStorages(ctx context.Context, c *client.Client, nodes []string) (map[string]storageInfo, map[string]string, []fetchFailure, map[string]storageInfo) {
	infos := map[string]storageInfo{}
	reader := map[string]string{}   // key "<storage>@<node>" or "<storage>" for shared -> node
	inactive := map[string]string{} // enabled but unavailable; a shared one may be active on another node
	disabled := map[string]storageInfo{}
	var failures []fetchFailure
	for _, node := range nodes {
		var list []storageInfo
		if err := getJSON(ctx, c, pvePath("/nodes/%s/storage", node), map[string]string{"content": "backup"}, &list); err != nil {
			failures = append(failures, fetchFailure{Scope: "node " + node + " storage", Error: err.Error()})
			continue
		}
		for _, s := range list {
			key := s.Storage
			if !s.Shared {
				key = s.Storage + "@" + node
			}
			if s.Enabled != nil && !bool(*s.Enabled) {
				disabled[key] = s
				continue
			}
			if !s.active() {
				if _, seen := reader[key]; !seen {
					inactive[key] = node
				}
				continue
			}
			if _, seen := reader[key]; seen {
				continue
			}
			reader[key] = node
			infos[key] = s
			delete(inactive, key)
		}
	}
	// An enabled backup storage that no node could activate holds unreadable
	// archives: that is missing evidence, not an empty storage.
	for _, key := range sortedKeys(inactive) {
		failures = append(failures, fetchFailure{Scope: "storage " + key,
			Error: "enabled backup storage is not active (unavailable on node " + inactive[key] + "); its archives and capacity could not be read"})
	}
	return infos, reader, failures, disabled
}

// disabledFailure: a disabled backup storage cannot be listed, so the archives
// it may still hold (from before it was disabled) were not read.
func disabledFailure(cluster, key string) fetchFailure {
	return fetchFailure{Cluster: cluster, Scope: "storage " + key,
		Error: "backup storage is disabled in the storage config, so the archives it may still hold (from before it was disabled) were not read"}
}

type contentEntry struct {
	VolID        string        `json:"volid"`
	VMID         flexInt       `json:"vmid"`
	CTime        flexInt       `json:"ctime"`
	Size         flexInt       `json:"size"`
	Subtype      string        `json:"subtype"`
	Protected    flexBool      `json:"protected"`
	Encrypted    string        `json:"encrypted"`
	Notes        string        `json:"notes"`
	Verification *verification `json:"verification"`
}

func fetchStorageArchives(ctx context.Context, c *client.Client, node string, s storageInfo, vmid int64, refs map[string]pbsStorageRef) ([]restorePoint, error) {
	params := map[string]string{"content": "backup"}
	if vmid > 0 {
		params["vmid"] = strconv.FormatInt(vmid, 10)
	}
	var entries []contentEntry
	if err := getJSON(ctx, c, pvePath("/nodes/%s/storage/%s/content", node, s.Storage), params, &entries); err != nil {
		return nil, err
	}
	out := make([]restorePoint, 0, len(entries))
	for _, e := range entries {
		// No size: an archive that cannot be restored is not a restore point.
		// For a PBS storage, PVE's PBSPlugin reports a snapshot with no manifest
		// (a backup still running, or crash debris) as size 1 ("$item->{size}
		// // 1"), so 1 byte is that sentinel there; no real snapshot is 1 byte.
		minSize := flexInt(0)
		if s.Type == "pbs" {
			minSize = 1
		}
		if e.VMID == 0 || e.CTime == 0 {
			continue
		}
		rp := restorePoint{
			VMID: int64(e.VMID), GuestType: e.Subtype, Time: int64(e.CTime), Source: "pve-storage:" + s.Storage,
			Volume: e.VolID, Size: int64(e.Size), Protected: bool(e.Protected), Encrypted: e.Encrypted != "" && e.Encrypted != "0",
			Notes: e.Notes, PBSBacked: s.Type == "pbs", VerifyState: "n/a", storageID: s.Storage, storageNode: node,
			unreadable: e.Size <= minSize,
		}
		if rp.PBSBacked {
			rp.VerifyState = "none"
		}
		if e.Verification != nil && e.Verification.State != "" {
			rp.VerifyState, rp.VerifyUPID = e.Verification.State, e.Verification.UPID
		}
		rp.TimeRFC3339 = time.Unix(rp.Time, 0).UTC().Format(time.RFC3339)
		if ref, ok := refs[s.Storage]; ok {
			rp.canonical = pbsCanonical(ref.server, ref.datastore, ref.namespace, rp.GuestType, rp.VMID, rp.Time)
			rp.pbsStore, rp.pbsNS = ref.datastore, ref.namespace
		} else {
			rp.canonical = "pve:" + s.Storage + "@" + node + ":" + e.VolID
		}
		out = append(out, rp)
	}
	return out, nil
}

// backupListingHidden reports whether Proxmox VE filters backup volumes out of
// a storage's content list for this token. PVE::Storage::check_volume_access
// shows a backup volume only with Datastore.Allocate on the storage, or
// Datastore.AllocateSpace on it plus VM.Backup on the owning guest. An
// audit-only token (PVEAuditor) has neither and gets an empty list, not a 403.
// vmid 0 means every guest, so VM.Backup must then cover /vms.
func backupListingHidden(ctx context.Context, c *client.Client, storage string, vmid int64) (bool, error) {
	sp, err := privsAt(ctx, c, false, "/storage/"+storage)
	if err != nil {
		return true, err
	}
	if _, ok := sp[client.PrivDatastoreAllocate.String()]; ok {
		return false, nil
	}
	if _, ok := sp[client.PrivDatastoreAllocateSpace.String()]; !ok {
		return true, nil
	}
	if vmid == 0 {
		// Every guest: VM.Backup must reach each one (propagating from /vms,
		// no deeper ACL dropping it), or that guest's archives are filtered out.
		gap, err := subtreeGap(ctx, c, false, "/vms", "guests", client.PrivVMBackup)
		return gap != "", err
	}
	vprivs, err := privsAt(ctx, c, false, "/vms/"+strconv.FormatInt(vmid, 10))
	if err != nil {
		return true, err
	}
	_, ok := vprivs[client.PrivVMBackup.String()]
	return !ok, nil
}

// pbsCovers reports whether the PBS API reads the datastore and namespace that
// a PVE storage of type pbs writes to, which makes its PVE content list
// redundant. A failed PBS read is reported by the caller on its own.
func pbsCovers(refs map[string]pbsStorageRef, storage, endpoint string, nsByStore map[string][]string) bool {
	ref, ok := refs[storage]
	if !ok || endpoint == "" || !strings.EqualFold(ref.server, endpoint) {
		return false
	}
	for _, ns := range nsByStore[ref.datastore] {
		if ns == ref.namespace {
			return true
		}
	}
	return false
}

// errCoveredByPBS: the token cannot see this storage's PVE content list, but
// the PBS API reads its datastore. The caller counts the storage as read only
// once that PBS read succeeds (see pbsStandIns).
var errCoveredByPBS = errors.New("read through the PBS API instead of the PVE content list")

// splitUnreadable separates the restore points from the archives listed
// without a readable manifest (see restorePoint.unreadable).
func splitUnreadable(in []restorePoint) (points, unreadable []restorePoint) {
	for _, rp := range in {
		if rp.unreadable {
			unreadable = append(unreadable, rp)
		} else {
			points = append(points, rp)
		}
	}
	return points, unreadable
}

// readStorageArchives is fetchStorageArchives behind the visibility rule: a
// content list the token cannot fully see is missing evidence, never "no
// backups". When the PBS API reads the same archives (covered), a hidden PVE
// list returns errCoveredByPBS instead.
func readStorageArchives(ctx context.Context, c *client.Client, node string, s storageInfo, vmid int64, refs map[string]pbsStorageRef, covered bool) ([]restorePoint, error) {
	if ref, ok := refs[s.Storage]; ok && len(ref.sharedWith) > 0 {
		return nil, sharedPBSError(ref, ref.namespace, ref.sharedWith)
	}
	hidden, err := backupListingHidden(ctx, c, s.Storage, vmid)
	if err != nil {
		return nil, fmt.Errorf("cannot tell whether the token may list backup archives: %w", err)
	}
	if hidden {
		if covered {
			return nil, errCoveredByPBS
		}
		hint := ""
		if ref, ok := refs[s.Storage]; ok {
			if !c.PBSConfigured() || !strings.EqualFold(pbsEndpointOf(c), ref.server) {
				hint = fmt.Sprintf(" Its backups are on PBS %s: set PROXMOX_PBS_HOST (or pbs_host) to that server, spelled the same way, and the PBS API reads them with an audit-only token.", ref.server)
			} else {
				hint = fmt.Sprintf(" The PBS API did not read datastore %s namespace %q for this cluster either: see the pbs failure, or set pbs_namespace to that namespace or one above it.", ref.datastore, ref.namespace)
			}
		}
		return nil, fmt.Errorf("Proxmox VE lists backup archives on storage %s only to a token with Datastore.Allocate on /storage/%s, or Datastore.AllocateSpace on it plus VM.Backup on the guest; this token has neither, so it would see an empty list and the archives were not read.%s", s.Storage, s.Storage, hint)
	}
	return fetchStorageArchives(ctx, c, node, s, vmid, refs)
}

// pbsStandIns settles the storages whose archives the PBS API stood in for: a
// storage counts as read only when the PBS snapshot list of its datastore and
// namespace succeeded, so a failed PBS read leaves the job's target unread
// (unproven) instead of empty (never).
func pbsStandIns(standIn map[string]pbsStorageRef, pbsUnread map[string]bool, read map[string]bool) {
	for key, ref := range standIn {
		if !pbsUnread[pbsSourceLabel(ref.datastore, ref.namespace)] {
			read[key] = true
		}
	}
}

func pbsCanonical(server, store, ns, guestType string, vmid, t int64) string {
	return fmt.Sprintf("pbs:%s/%s/%s/%s/%d/%d", server, store, ns, guestType, vmid, t)
}

// pbsStorageRef is a PVE storage of type pbs: where on which PBS its backups
// live. server is the normalized endpoint "host:port" (see pbsEndpoint);
// sharedWith names every configured cluster writing to the same namespace.
type pbsStorageRef struct {
	server, datastore, namespace, pruneBackups, fingerprint string
	sharedWith                                              []string
}

// sourceKeys identify one PBS namespace across clusters, by endpoint and, when
// the storage pins one, by PBS certificate fingerprint: two storages name the
// same namespace when they share either (two spellings of a host, or one
// storage without a fingerprint).
func (r pbsStorageRef) sourceKeys(ns string) []string {
	keys := []string{"ep:" + r.server + "|" + r.datastore + "|" + ns}
	if fp := strings.ToLower(strings.NewReplacer(":", "", " ", "").Replace(r.fingerprint)); fp != "" {
		keys = append(keys, "fp:"+fp+"|"+r.datastore+"|"+ns)
	}
	return keys
}

// sharedOwners is every configured cluster writing to ns of ref's datastore
// on the same PBS (see markSharedPBS); empty when only this one does.
func (t fleetTarget) sharedOwners(ref pbsStorageRef, ns string) []string {
	set := map[string]bool{}
	for _, k := range ref.sourceKeys(ns) {
		for _, o := range t.sharedPBS[k] {
			set[o] = true
		}
	}
	return sortedKeys(set)
}

// markSharedPBS finds PBS namespaces that two or more configured clusters
// write to. VMIDs collide across clusters, so a snapshot of vm/100 there may
// be either cluster's: each target learns the shared keys. It looks at EVERY
// configured cluster, not only the ones --cluster/--client selected. Unknown
// ownership counts as shared: a cluster whose client cannot be built, whose
// storage config cannot be read, or whose storage list the token may not
// fully see is assumed to write to every namespace on the PBS host its
// clusters.toml entry names.
func markSharedPBS(ctx context.Context, flags *rootFlags, targets []fleetTarget) {
	all, err := allClusterTargets(flags)
	if err != nil || len(all) < 2 {
		return
	}
	owners := map[string][]string{}
	pves := map[string]map[string]bool{} // key -> distinct PVE clusters writing there
	unreadable := map[string]string{}    // cluster -> its PBS endpoint ("" when it names none)
	keys := make([]map[string]bool, len(all))
	// ponytail: one /storage read per cluster, sequential; fine for tens of clusters.
	for i, t := range all {
		var refs map[string]pbsStorageRef
		var err error = t.Err
		if err == nil && strings.TrimSpace(t.C.BaseURL) == "" {
			continue // PBS-only profile: it writes nothing through PVE
		}
		if err == nil {
			refs, err = fetchPBSStorageRefs(ctx, t.C)
		}
		if err != nil {
			unreadable[t.Name] = pbsEndpointFromHost(t.Profile.PBSHost)
			continue
		}
		// A storage the token may not see is missing from the list, so an
		// incomplete list cannot prove which namespaces this cluster uses: the
		// visible ones still count, and the rest is unknown.
		if storageListHidden(ctx, t.C) != nil {
			unreadable[t.Name] = pbsEndpointFromHost(t.Profile.PBSHost)
		}
		keys[i] = map[string]bool{}
		for _, ref := range refs {
			for _, k := range ref.sourceKeys(ref.namespace) {
				keys[i][k] = true
			}
			if t.Profile.PBSNamespace != "" {
				for _, k := range ref.sourceKeys(t.Profile.PBSNamespace) {
					keys[i][k] = true
				}
			}
		}
		// Two entries for the same PVE cluster write the same guests, not
		// colliding ones: a cluster is identified by its API base URL (not its
		// certificate, which a wildcard can share across clients). One cluster
		// reached through two node URLs is flagged, loudly, never missed.
		id := strings.ToLower(strings.TrimRight(strings.TrimSpace(t.C.BaseURL), "/"))
		for k := range keys[i] {
			owners[k] = append(owners[k], t.Name)
			if pves[k] == nil {
				pves[k] = map[string]bool{}
			}
			pves[k][id] = true
		}
	}
	byName := map[string]int{}
	for i, t := range all {
		byName[t.Name] = i
	}
	for ti := range targets {
		i, ok := byName[targets[ti].Name]
		if !ok || keys[i] == nil {
			continue
		}
		for k := range keys[i] {
			if len(pves[k]) > 1 {
				if targets[ti].sharedPBS == nil {
					targets[ti].sharedPBS = map[string][]string{}
				}
				targets[ti].sharedPBS[k] = owners[k]
			}
		}
		for _, name := range sortedKeys(unreadable) {
			if name == targets[ti].Name {
				continue
			}
			// Only a cluster that names this PBS in clusters.toml: one naming no
			// PBS (or another) is not assumed to write here, or every outage
			// would mark every other client's PBS evidence unknown.
			ep := unreadable[name]
			for k := range keys[i] {
				if ep != "" && strings.HasPrefix(k, "ep:"+ep+"|") {
					if targets[ti].sharedPBS == nil {
						targets[ti].sharedPBS = map[string][]string{}
					}
					if len(targets[ti].sharedPBS[k]) == 0 {
						targets[ti].sharedPBS[k] = []string{targets[ti].Name}
					}
					targets[ti].sharedPBS[k] = append(targets[ti].sharedPBS[k], name+" (could not be read)")
				}
			}
		}
	}
}

// allClusterTargets builds a client for every configured cluster, ignoring
// --cluster/--client: collision detection must see the clusters a run skips.
func allClusterTargets(flags *rootFlags) ([]fleetTarget, error) {
	all, _, err := loadClusters()
	if err != nil {
		return nil, err
	}
	var out []fleetTarget
	for _, name := range sortedClusterNames(all) {
		cl := all[name]
		var c *client.Client
		err := cl.validate(name)
		if err == nil {
			c, err = newClusterClient(flags, cl)
		}
		out = append(out, fleetTarget{Name: name, Tenant: cl.Client, Profile: cl, C: c, Err: err})
	}
	return out, nil
}

// withShared marks the refs whose namespace another configured cluster also
// writes to (see markSharedPBS).
func (t fleetTarget) withShared(refs map[string]pbsStorageRef) map[string]pbsStorageRef {
	for id, ref := range refs {
		if owners := t.sharedOwners(ref, ref.namespace); len(owners) > 0 {
			ref.sharedWith = owners
			refs[id] = ref
		}
	}
	return refs
}

func sharedPBSError(ref pbsStorageRef, ns string, owners []string) error {
	return fmt.Errorf("PBS datastore %s namespace %q is, or may be, written by more than one configured cluster (%s): VMIDs collide in one namespace, so these restore points may be another cluster's and were not counted; give each cluster its own PBS namespace", ref.datastore, ns, strings.Join(owners, ", "))
}

// pbsEndpoint identifies a PBS service as lowercase "host:port" (default port
// 8007): two PBS instances on one host are different services.
func pbsEndpoint(host string, port int64) string {
	p := pbsDefaultPortNum
	if port > 0 {
		p = port
	}
	return net.JoinHostPort(strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")), strconv.FormatInt(p, 10))
}

const pbsDefaultPortNum int64 = 8007

// fetchPBSStorageRefs reads the cluster's storage config (/storage) for PBS storages.
func fetchPBSStorageRefs(ctx context.Context, c *client.Client) (map[string]pbsStorageRef, error) {
	var list []struct {
		Storage   string  `json:"storage"`
		Type      string  `json:"type"`
		Server    string  `json:"server"`
		Datastore string  `json:"datastore"`
		Namespace string  `json:"namespace"`
		Prune     string  `json:"prune-backups"`
		Port      flexInt `json:"port"`
		Print     string  `json:"fingerprint"`
	}
	if err := getJSON(ctx, c, "/storage", map[string]string{"type": "pbs"}, &list); err != nil {
		return nil, fmt.Errorf("storage config: %w", err)
	}
	refs := map[string]pbsStorageRef{}
	for _, s := range list {
		if s.Type == "pbs" {
			refs[s.Storage] = pbsStorageRef{server: pbsEndpoint(s.Server, int64(s.Port)), datastore: s.Datastore, namespace: s.Namespace, pruneBackups: s.Prune, fingerprint: s.Print}
		}
	}
	return refs, nil
}

// pbsEndpoint is the configured PBS service of this target as "host:port"
// (comparable with pbsStorageRef.server), or "".
func (t fleetTarget) pbsEndpoint() string { return pbsEndpointOf(t.C) }

func pbsEndpointOf(c *client.Client) string {
	return pbsEndpointFromHost(c.Config.TemplateVars["pbs_host"])
}

// pbsEndpointFromHost normalizes a PBS host setting (host, host:port or
// https://host:port) to "host:port", or "".
func pbsEndpointFromHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" || host == client.PBSHostNone || host == "pbs_host_placeholder" {
		return ""
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port, _ := strconv.ParseInt(u.Port(), 10, 64)
	return pbsEndpoint(u.Hostname(), port)
}

// pbsAttribution returns the PBS datastore/namespace pairs whose snapshots
// belong to this cluster's guests. vmids collide across clusters on a shared
// PBS, so ownership must be explicit: the profile's pbs_namespace, or the
// datastore + namespace that this cluster's own PVE storage config points at
// on this PBS host. Anything else is not attributed (the PVE storage listing
// still reports the cluster's own archives).
func pbsAttribution(ctx context.Context, t fleetTarget, refs map[string]pbsStorageRef) (map[string][]string, string, error) {
	stores, err := pbsDatastores(ctx, t.C)
	if err != nil {
		return nil, "", err
	}
	out := map[string][]string{}
	host := t.pbsEndpoint()
	known := map[string]bool{}
	for _, st := range stores {
		known[st] = true
	}
	var shared []string
	if ns := t.Profile.PBSNamespace; ns != "" {
		// Only the datastores this cluster's own PVE storages write to on this
		// PBS: the same namespace name on another datastore can be another
		// client's backups.
		tied := map[string]pbsStorageRef{}
		for _, ref := range refs {
			if !strings.EqualFold(ref.server, host) || !known[ref.datastore] {
				continue
			}
			if owners := t.sharedOwners(ref, ns); len(owners) > 0 {
				shared = append(shared, sharedPBSError(ref, ns, owners).Error())
				continue
			}
			tied[ref.datastore] = ref
		}
		// pbs_namespace names the client's namespace and every one below it: a
		// storage moved from acme/old to acme leaves its older points in acme/old.
		var listErr error
		for _, store := range sortedKeys(tied) {
			nss, err := pbsNamespaces(ctx, t.C, store)
			if err != nil {
				listErr = fmt.Errorf("PBS %s: the namespaces below %q could not be listed, so restore points there were not read: %w", store, ns, err)
			}
			for _, n := range nss {
				if !inNamespace(n, ns) {
					continue
				}
				if owners := t.sharedOwners(tied[store], n); len(owners) > 0 {
					shared = append(shared, sharedPBSError(tied[store], n, owners).Error())
					continue
				}
				out[store] = append(out[store], n)
			}
			if len(out[store]) == 0 {
				out[store] = []string{ns} // read it anyway, so a missing namespace fails loudly
			}
		}
		warning := ""
		switch {
		case len(shared) > 0:
			warning = t.Name + ": " + strings.Join(dedupeStrings(shared), "; ")
		case len(out) == 0:
			warning = fmt.Sprintf("%s: PBS snapshots not attributed to guests: no PVE storage on this cluster points at PBS host %q, so pbs_namespace %q is not tied to a datastore", t.Name, host, ns)
		}
		return out, warning, listErr
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !strings.EqualFold(ref.server, host) || !known[ref.datastore] {
			continue
		}
		if len(ref.sharedWith) > 0 {
			shared = append(shared, sharedPBSError(ref, ref.namespace, ref.sharedWith).Error())
			continue
		}
		key := ref.datastore + "\x00" + ref.namespace
		if !seen[key] {
			seen[key] = true
			out[ref.datastore] = append(out[ref.datastore], ref.namespace)
		}
	}
	if len(shared) > 0 {
		return out, t.Name + ": " + strings.Join(dedupeStrings(shared), "; "), nil
	}
	if len(out) == 0 {
		return out, fmt.Sprintf("%s: PBS snapshots not attributed to guests: no PVE storage on this cluster points at PBS host %q and no pbs_namespace is set", t.Name, host), nil
	}
	return out, "", nil
}

func dedupeStrings(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// --- PBS

type pbsSnapshot struct {
	BackupType   string        `json:"backup-type"`
	BackupID     string        `json:"backup-id"`
	BackupTime   flexInt       `json:"backup-time"`
	Size         flexInt       `json:"size"`
	Protected    flexBool      `json:"protected"`
	Owner        string        `json:"owner"`
	Comment      string        `json:"comment"`
	Fingerprint  string        `json:"fingerprint"`
	Verification *verification `json:"verification"`
}

func pbsDatastores(ctx context.Context, c *client.Client) ([]string, error) {
	var ds []struct {
		Store string `json:"store"`
	}
	if err := getJSON(ctx, c, pbsPath("/admin/datastore"), nil, &ds); err != nil {
		return nil, fmt.Errorf("PBS datastores: %w", err)
	}
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Store)
	}
	sort.Strings(out)
	return out, nil
}

func pbsNamespaces(ctx context.Context, c *client.Client, store string) ([]string, error) {
	var ns []struct {
		NS string `json:"ns"`
	}
	if err := getJSON(ctx, c, pbsPath("/admin/datastore/%s/namespace", store), nil, &ns); err != nil {
		return nil, err
	}
	out := []string{}
	seen := map[string]bool{}
	for _, n := range ns {
		if !seen[n.NS] {
			seen[n.NS] = true
			out = append(out, n.NS)
		}
	}
	if !seen[""] {
		out = append([]string{""}, out...)
	}
	sort.Strings(out)
	return out, nil
}

func pbsSnapshots(ctx context.Context, c *client.Client, store, ns string, params map[string]string) ([]pbsSnapshot, error) {
	p := map[string]string{}
	for k, v := range params {
		p[k] = v
	}
	if ns != "" {
		p["ns"] = ns
	}
	// PBS lists every group only with Datastore.Audit on the namespace's ACL
	// path; with Datastore.Backup alone it silently lists just the groups the
	// token owns, and a partial list must not read as a complete one.
	acl := "/datastore/" + store
	if ns != "" {
		acl += "/" + ns
	}
	privs, err := privsAt(ctx, c, true, acl)
	if err != nil {
		return nil, err
	}
	if _, ok := privs[client.PrivDatastoreAudit.String()]; !ok {
		return nil, fmt.Errorf("the PBS token lacks Datastore.Audit on %s, so PBS would list only the backup groups it owns; snapshots there were not read", acl)
	}
	var snaps []pbsSnapshot
	if err := getJSON(ctx, c, pbsPath("/admin/datastore/%s/snapshots", store), p, &snaps); err != nil {
		return nil, err
	}
	// A snapshot whose manifest PBS could not read (a backup still running, or
	// a damaged one) comes back with no size: callers treat it as uncertain
	// evidence (restorePoint.unreadable), never as a restore point.
	return snaps, nil
}

func pbsSourceLabel(store, ns string) string {
	if ns == "" {
		return "pbs:" + store
	}
	return "pbs:" + store + "/" + ns
}

func (s pbsSnapshot) restorePoint(host, store, ns string) restorePoint {
	vmid, _ := strconv.ParseInt(s.BackupID, 10, 64)
	gt := map[string]string{"vm": "qemu", "ct": "lxc"}[s.BackupType]
	rp := restorePoint{
		VMID: vmid, GuestType: gt, Time: int64(s.BackupTime), Source: pbsSourceLabel(store, ns),
		Volume: fmt.Sprintf("%s/%s/%d", s.BackupType, s.BackupID, int64(s.BackupTime)), Size: int64(s.Size),
		Protected: bool(s.Protected), Encrypted: s.Fingerprint != "", Notes: s.Comment, PBSBacked: true, VerifyState: "none",
		unreadable: s.Size <= 0,
	}
	if s.Verification != nil && s.Verification.State != "" {
		rp.VerifyState, rp.VerifyUPID = s.Verification.State, s.Verification.UPID
	}
	rp.TimeRFC3339 = time.Unix(rp.Time, 0).UTC().Format(time.RFC3339)
	rp.canonical = pbsCanonical(host, store, ns, gt, vmid, rp.Time)
	rp.pbsStore, rp.pbsNS = store, ns
	return rp
}

// inNamespace reports whether n is ns or a namespace below it (PBS nests them).
func inNamespace(n, ns string) bool { return n == ns || strings.HasPrefix(n, ns+"/") }

// pbsNamespacesForCluster limits a shared PBS to this cluster's namespace, and
// the namespaces below it, on the datastores its PVE storages write to when the
// profile names one; otherwise every namespace on every datastore. notes name
// what was left out or could not be tied, so nothing is dropped silently.
// untied: the datastores could not be tied (PVE unreachable), so every row read
// may be another client's and must say so.
func pbsNamespacesForCluster(ctx context.Context, t fleetTarget) (out map[string][]string, notes []fetchFailure, untied bool, err error) {
	stores, err := pbsDatastores(ctx, t.C)
	if err != nil {
		return nil, nil, false, err
	}
	out = map[string][]string{}
	ns := t.Profile.PBSNamespace
	host := t.pbsEndpoint()
	var refs map[string]pbsStorageRef
	var own map[string]bool // nil: every datastore (PBS-only profile, or untied)
	// A PBS-only profile (no PVE host) is scoped by the profile itself.
	if ns != "" && strings.TrimSpace(t.C.BaseURL) != "" {
		// The namespace is this client's only on the datastores this cluster's
		// own PVE storages write to; the same name elsewhere can be another's.
		var rerr error
		if refs, rerr = fetchPBSStorageRefs(ctx, t.C); rerr != nil {
			// PVE down must not blank the PBS view: read the namespace wherever
			// it exists, and mark every row as possibly another client's.
			untied = true
			notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "pbs namespace", Error: fmt.Sprintf(
				"could not tie pbs_namespace %q to this cluster's datastores (%v); it is read on every datastore that has it, and each row is marked untied: it may be another client's", ns, rerr)})
		} else {
			own = map[string]bool{}
			for _, ref := range refs {
				if strings.EqualFold(ref.server, host) {
					own[ref.datastore] = true
				}
			}
			if len(own) == 0 {
				return nil, nil, false, fmt.Errorf("no PVE storage on this cluster points at PBS host %q (spelled as in the storage's server field), so pbs_namespace %q is not tied to a datastore and nothing was read", host, ns)
			}
		}
	}
	for _, store := range stores {
		nss, lerr := pbsNamespaces(ctx, t.C, store)
		if ns == "" {
			if lerr != nil {
				return nil, nil, false, fmt.Errorf("PBS %s namespaces: %w", store, lerr)
			}
			out[store] = nss
			continue
		}
		if own != nil && !own[store] {
			// Not this cluster's datastore: say so when it carries the namespace
			// (a sync target, or another client's), rather than drop it silently.
			if lerr != nil {
				notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "pbs " + store, Error: "namespaces not listed: " + lerr.Error()})
				continue
			}
			for _, n := range nss {
				if inNamespace(n, ns) {
					notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "pbs " + store, Error: fmt.Sprintf(
						"datastore %s also has namespace %q, but no PVE storage on this cluster writes to it (a sync target, or another client's), so it was not read", store, n)})
					break
				}
			}
			continue
		}
		if lerr != nil {
			// Read the namespace itself so a failure there still surfaces.
			notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "pbs " + store, Error: fmt.Sprintf(
				"namespaces not listed (%v); only %q was read, not the namespaces below it", lerr, ns)})
			out[store] = []string{ns}
			continue
		}
		var mine []string
		for _, n := range nss {
			if inNamespace(n, ns) {
				mine = append(mine, n)
			}
		}
		switch {
		case len(mine) > 0:
			out[store] = mine
		case own != nil:
			notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "pbs " + store, Error: fmt.Sprintf(
				"namespace %q does not exist on datastore %s, which this cluster's PVE storage writes to; nothing there was read", ns, store)})
		}
	}
	if ns != "" && own != nil {
		for _, id := range sortedKeys(refs) {
			ref := refs[id]
			if strings.EqualFold(ref.server, host) && !inNamespace(ref.namespace, ns) {
				notes = append(notes, fetchFailure{Cluster: t.Name, Scope: "storage " + id, Error: fmt.Sprintf(
					"writes to datastore %s namespace %q, outside pbs_namespace %q, so its verify state was not read", ref.datastore, ref.namespace, ns)})
			}
		}
	}
	if ns != "" && len(out) == 0 {
		return nil, notes, untied, fmt.Errorf("pbs_namespace %q exists on no datastore this cluster may read, so nothing was read", ns)
	}
	return out, notes, untied, nil
}

// --- tasks and vzdump logs

type pveTask struct {
	UPID      string  `json:"upid"`
	Node      string  `json:"node"`
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	User      string  `json:"user"`
	Status    string  `json:"status"`
	StartTime flexInt `json:"starttime"`
	EndTime   flexInt `json:"endtime"`
	// PBS names
	WorkerType string `json:"worker_type"`
	WorkerID   string `json:"worker_id"`
}

func (t pveTask) kind() string {
	if t.Type != "" {
		return t.Type
	}
	return t.WorkerType
}

func (t pveTask) target() string {
	if t.ID != "" {
		return t.ID
	}
	return t.WorkerID
}

// taskOK: "WARNINGS: N" means the task completed with warnings, not failed.
func taskOK(status string) bool {
	return status == "OK" || status == "" || strings.HasPrefix(status, "WARNINGS")
}

// tokenPrivs is /access/permissions: ACL path -> privilege -> propagate bit.
type tokenPrivs map[string]map[string]flexBool

func fetchPrivs(ctx context.Context, c *client.Client, pbs bool) (tokenPrivs, error) {
	path := "/access/permissions"
	if pbs {
		path = client.PBSPath(path)
	}
	var perms tokenPrivs
	if err := getJSON(ctx, c, path, nil, &perms); err != nil {
		return nil, fmt.Errorf("checking token privileges: %w", err)
	}
	return perms, nil
}

// has reports whether priv applies at path. /access/permissions lists the
// effective privileges per ACL path, and a deeper ACL replaces what it would
// inherit (PVE::AccessControl::roles), so the nearest listed path at or above
// path decides alone: the path itself grants priv, or that ancestor grants it
// with the propagate bit.
func (p tokenPrivs) has(priv, path string) bool {
	_, ok := p.at(path)[priv]
	return ok
}

// at is the privilege set that applies at path by the nearest-listed rule.
func (p tokenPrivs) at(path string) map[string]flexBool {
	want := "/" + strings.Trim(path, "/")
	cur := want
	for {
		if privs, listed := p[cur]; listed {
			if cur == want {
				return privs
			}
			out := map[string]flexBool{}
			for k, v := range privs {
				if bool(v) {
					out[k] = v
				}
			}
			return out
		}
		if cur == "/" {
			return nil
		}
		if i := strings.LastIndex(cur, "/"); i > 0 {
			cur = cur[:i]
		} else {
			cur = "/"
		}
	}
}

// privsAt asks Proxmox for the token's effective privileges at exactly path
// (/access/permissions?path=...), which applies every inherited, replaced and
// pool-granted ACL the way Proxmox itself does. A server that ignores the
// parameter is answered from its full dump by the nearest-listed rule.
func privsAt(ctx context.Context, c *client.Client, pbs bool, path string) (map[string]flexBool, error) {
	ep := "/access/permissions"
	if pbs {
		ep = client.PBSPath(ep)
	}
	var perms tokenPrivs
	if err := getJSON(ctx, c, ep, map[string]string{"path": path}, &perms); err != nil {
		return nil, fmt.Errorf("checking token privileges on %s: %w", path, err)
	}
	return perms.at(path), nil
}

// guestListHidden explains an incomplete guest list: Proxmox VE lists only the
// guests the token holds VM.Audit on, and answers with the rest silently left
// out, so a missing guest would read as no guest.
func guestListHidden(ctx context.Context, c *client.Client) error {
	return subtreeHidden(ctx, c, false, "/vms", "guests", client.PrivVMAudit)
}

// pbsDatastoreListHidden: PBS lists (and reports usage for) only the
// datastores a token may audit.
func pbsDatastoreListHidden(ctx context.Context, c *client.Client) error {
	return subtreeHidden(ctx, c, true, "/datastore", "datastores", client.PrivDatastoreAudit)
}

// storageListHidden is guestListHidden for storages: Proxmox VE lists a
// storage only to a token with Datastore.Audit or Datastore.AllocateSpace on it.
func storageListHidden(ctx context.Context, c *client.Client) error {
	return subtreeHidden(ctx, c, false, "/storage", "storages", client.PrivDatastoreAudit, client.PrivDatastoreAllocateSpace)
}

// subtreeHidden explains a Proxmox VE list filtered object by object: it is
// complete only when one of privs is granted at root WITH the propagate flag
// and no deeper ACL path the token holds privileges on drops it. One limit no
// read can see past: an explicit NoAccess on a single object leaves no trace
// in any API answer, so the docs tell operators never to put one on the
// monitoring token.
func subtreeHidden(ctx context.Context, c *client.Client, pbs bool, root, what string, privs ...client.Privilege) error {
	gap, err := subtreeGap(ctx, c, pbs, root, what, privs...)
	if err != nil {
		return err
	}
	if gap != "" {
		return errors.New(gap)
	}
	return nil
}

// subtreeGap is subtreeHidden's reason, "" when the list is complete.
func subtreeGap(ctx context.Context, c *client.Client, pbs bool, root, what string, privs ...client.Privilege) (string, error) {
	product := "Proxmox VE"
	if pbs {
		product = "Proxmox Backup Server"
	}
	names := make([]string, len(privs))
	for i, p := range privs {
		names[i] = p.String()
	}
	want := strings.Join(names, " or ")
	at, err := privsAt(ctx, c, pbs, root)
	if err != nil {
		return "", err
	}
	propagates, held := false, false
	for _, n := range names {
		if v, ok := at[n]; ok {
			held = true
			propagates = propagates || bool(v)
		}
	}
	switch {
	case held && !propagates:
		return fmt.Sprintf("the token holds %s on %s without the propagate flag, so %s shows only the %s granted one by one; the others are missing from this report, not absent", want, root, product, what), nil
	case !held:
		return fmt.Sprintf("the token lacks %s on %s, so %s shows only the %s it may see; the others are missing from this report, not absent", want, root, product, what), nil
	}
	dump, err := fetchPrivs(ctx, c, pbs)
	if err != nil {
		return "", err
	}
	var dropped []string
	for path, p := range dump {
		if !strings.HasPrefix(path, root+"/") {
			continue
		}
		ok := false
		for _, n := range names {
			_, has := p[n]
			ok = ok || has
		}
		if !ok {
			dropped = append(dropped, path)
		}
	}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		return fmt.Sprintf("an ACL on %s replaces the inherited %s, so those %s are missing from this report, not absent", strings.Join(dropped, ", "), want, what), nil
	}
	return "", nil
}

// anywhere reports whether priv is granted on any path.
func (p tokenPrivs) anywhere(priv string) bool {
	for _, privs := range p {
		if _, ok := privs[priv]; ok {
			return true
		}
	}
	return false
}

// requireVisibility turns "the token sees nothing" into a cluster failure
// unless the token holds priv somewhere: a privilege-separated token without
// an ACL (or with only unrelated privileges) gets empty lists, not 403, which
// would otherwise read as a healthy, empty cluster.
func requireVisibility(ctx context.Context, c *client.Client, pbs bool, p client.Privilege) error {
	priv := p.String()
	perms, err := fetchPrivs(ctx, c, pbs)
	if err != nil {
		return err
	}
	if perms.anywhere(priv) {
		return nil
	}
	role := "PVEAuditor on /"
	if pbs {
		role = "Audit on /"
	}
	if len(perms) == 0 {
		return fmt.Errorf("the token sees nothing and holds no privileges: privilege-separated tokens need their own ACL (e.g. %s)", role)
	}
	return fmt.Errorf("the token sees nothing and holds no %s privilege: grant it an ACL such as %s", priv, role)
}

// taskListVisibility explains why a task list is incomplete: PVE shows other
// users' tasks (scheduled vzdump runs as root@pam) only with Sys.Audit on
// /nodes/<node>, PBS only with Sys.Audit on /system/tasks.
func taskListVisibility(perms tokenPrivs, pbs bool, node string) error {
	path := "/nodes/" + node
	if pbs {
		path = "/system/tasks"
	}
	if perms.has(client.PrivSysAudit.String(), path) {
		return nil
	}
	return fmt.Errorf("the token lacks Sys.Audit on %s, so it sees only its own tasks (not scheduled backup, verify, GC or sync runs)", path)
}

const (
	taskListPage     = 500
	taskListMaxPages = 20 // 10k tasks per node and window; past it the list is reported as capped
)

// fetchTasks reads a node's task list page by page (newest first). capped is
// true when the page limit stopped it before the list ended.
func fetchTasks(ctx context.Context, c *client.Client, pbs bool, node string, params map[string]string) (tasks []pveTask, capped bool, err error) {
	path := pvePath("/nodes/%s/tasks", node)
	if pbs {
		path = pbsPath("/nodes/%s/tasks", node)
	}
	q := map[string]string{"limit": strconv.Itoa(taskListPage)}
	for k, v := range params {
		q[k] = v
	}
	for page := 0; page < taskListMaxPages; page++ {
		q["start"] = strconv.Itoa(page * taskListPage)
		var batch []pveTask
		if err := getJSON(ctx, c, path, q, &batch); err != nil {
			return tasks, false, err
		}
		tasks = append(tasks, batch...)
		if len(batch) < taskListPage {
			return tasks, false, nil
		}
	}
	return tasks, true, nil
}

type taskLogLine struct {
	N flexInt `json:"n"`
	T string  `json:"t"`
}

const (
	taskLogPage     = 5000
	taskLogMaxPages = 40 // 200k lines: far beyond any vzdump log; past it the log is not judged
)

// fetchTaskLog reads the whole task log page by page; a truncated log would
// turn guests that finished after the cut into false "incomplete" verdicts.
func fetchTaskLog(ctx context.Context, c *client.Client, pbs bool, node, upid string) ([]string, error) {
	path := pvePath("/nodes/%s/tasks/%s/log", node, upid)
	if pbs {
		path = client.PBSPath(path)
	}
	var out []string
	for page := 0; page < taskLogMaxPages; page++ {
		var lines []taskLogLine
		params := map[string]string{"start": strconv.Itoa(len(out)), "limit": strconv.Itoa(taskLogPage)}
		if err := getJSON(ctx, c, path, params, &lines); err != nil {
			return nil, err
		}
		for _, l := range lines {
			out = append(out, l.T)
		}
		if len(lines) < taskLogPage {
			return out, nil
		}
	}
	return nil, fmt.Errorf("task log %s is longer than %d lines; not judged", upid, taskLogPage*taskLogMaxPages)
}

var (
	vzStartRe  = regexp.MustCompile(`Starting Backup of VM (\d+)\b`)
	vzFinishRe = regexp.MustCompile(`Finished Backup of VM (\d+)\b`)
	vzErrorRe  = regexp.MustCompile(`ERROR: Backup of VM (\d+) failed - (.*)$`)
)

// vzdumpGuestResult is one guest's outcome inside a (possibly multi-guest) vzdump task.
type vzdumpGuestResult struct {
	VMID   int64
	Result string // ok | failed | incomplete
	Reason string
}

// parseVzdumpLog attributes per-guest results from a vzdump task log:
// "Finished Backup of VM N" = ok, "ERROR: Backup of VM N failed - why" = failed,
// started but neither = incomplete (the silent mid-job stop).
var vzJobLineRe = regexp.MustCompile(`starting new backup job: vzdump((?: \d+)*)`)

// vzdumpJobFinished reports whether the job ran its whole guest list: Proxmox
// logs "Backup job finished successfully" or "... with errors" only then
// (PVE::VZDump::exec_backup). A job that died part-way ("Backup job failed -
// ...", a killed worker) did not reach its remaining guests.
func vzdumpJobFinished(lines []string) bool {
	for _, l := range lines {
		if strings.Contains(l, "Backup job finished successfully") || strings.Contains(l, "Backup job finished with errors") {
			return true
		}
	}
	return false
}

var vzSkipExternalRe = regexp.MustCompile(`skip external VMs: ([\d, ]+)`)

// vzdumpJobVMIDs is this node's explicit guest list: the job's command line
// minus the guests Proxmox logs as "skip external VMs" (they live on other
// nodes, whose own task backs them up). nil when the job selects guests with
// --all or --pool.
func vzdumpJobVMIDs(lines []string) []int64 {
	var listed []int64
	external := map[int64]bool{}
	found := false
	for _, l := range lines {
		if m := vzJobLineRe.FindStringSubmatch(l); m != nil && !found {
			found = true
			for _, f := range strings.Fields(m[1]) {
				if id, err := strconv.ParseInt(f, 10, 64); err == nil {
					listed = append(listed, id)
				}
			}
		}
		if m := vzSkipExternalRe.FindStringSubmatch(l); m != nil {
			for _, f := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' }) {
				if id, err := strconv.ParseInt(f, 10, 64); err == nil {
					external[id] = true
				}
			}
		}
	}
	if !found || listed == nil {
		return nil
	}
	ids := []int64{}
	for _, id := range listed {
		if !external[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseVzdumpLog(lines []string) []vzdumpGuestResult {
	order := []int64{}
	res := map[int64]*vzdumpGuestResult{}
	get := func(s string) *vzdumpGuestResult {
		id, _ := strconv.ParseInt(s, 10, 64)
		if r, ok := res[id]; ok {
			return r
		}
		r := &vzdumpGuestResult{VMID: id, Result: "incomplete"}
		res[id] = r
		order = append(order, id)
		return r
	}
	for _, line := range lines {
		if m := vzErrorRe.FindStringSubmatch(line); m != nil {
			r := get(m[1])
			r.Result, r.Reason = "failed", strings.TrimSpace(m[2])
			continue
		}
		if m := vzFinishRe.FindStringSubmatch(line); m != nil {
			if r := get(m[1]); r.Result != "failed" {
				r.Result = "ok"
			}
			continue
		}
		if m := vzStartRe.FindStringSubmatch(line); m != nil {
			get(m[1])
		}
	}
	out := make([]vzdumpGuestResult, 0, len(order))
	for _, id := range order {
		r := *res[id]
		if r.Result == "incomplete" {
			r.Reason = "backup started but never finished and logged no error"
		}
		out = append(out, r)
	}
	return out
}

// --- small helpers

func unixRFC3339(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

func hoursSince(now time.Time, sec int64) float64 {
	if sec <= 0 {
		return 0
	}
	h := now.Sub(time.Unix(sec, 0)).Hours()
	return float64(int64(h*10)) / 10
}

// parseWindow accepts Go durations plus d/w suffixes (26h, 7d, 2w).
func parseWindow(s string, def time.Duration) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	if d, err := cliutil.ParseDurationLoose(s); err == nil && d > 0 {
		return d, nil
	}
	return 0, usageErr(fmt.Errorf("invalid duration %q (use e.g. 26h, 7d, 2w)", s))
}

// upidStartTime extracts the task start time from a UPID. PVE:
// UPID:node:pid:pstart:starttime:type:id:user: ; PBS:
// UPID:node:pid:pstart:taskid:starttime:type:id:authid: (hex fields).
func upidStartTime(upid string) int64 {
	parts := strings.Split(upid, ":")
	if len(parts) < 8 || parts[0] != "UPID" {
		return 0
	}
	idx := 4
	if len(parts) >= 10 {
		idx = 5
	}
	v, err := strconv.ParseInt(parts[idx], 16, 64)
	if err != nil {
		return 0
	}
	return v
}

// nsReaches reports whether a job rooted at jobNS with maxDepth (default 7)
// covers namespace ns.
func nsReaches(jobNS string, maxDepth *int, ns string) bool {
	depth := 7
	if maxDepth != nil {
		depth = *maxDepth
	}
	if jobNS != "" && ns != jobNS && !strings.HasPrefix(ns, jobNS+"/") {
		return false
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(ns, jobNS), "/")
	levels := 0
	if rel != "" {
		levels = strings.Count(rel, "/") + 1
	}
	return levels <= depth
}

// pbsSyncJob is a PBS sync job; only push jobs copy this datastore offsite.
type pbsSyncJob struct {
	ID            string   `json:"id"`
	Store         string   `json:"store"`
	NS            string   `json:"ns"`
	MaxDepth      *int     `json:"max-depth"`
	GroupFilter   []string `json:"group-filter"`
	Direction     string   `json:"sync-direction"`
	LastRunState  string   `json:"last-run-state"`
	LastRunEnd    flexInt  `json:"last-run-endtime"`
	LastRunUPID   string   `json:"last-run-upid"`
	EncryptedOnly flexBool `json:"encrypted-only"`
	VerifiedOnly  flexBool `json:"verified-only"`
	TransferLast  flexInt  `json:"transfer-last"`
	// evidenced: source snapshots (syncEvidenceKey: datastore, namespace, group,
	// snapshot time) the last run's own task log shows it pushed. Filters and
	// namespaces can change after a run, so the current job config alone is
	// not evidence; a skip ("older than the newest snapshot present on sync
	// target") proves only that the target holds a newer snapshot, not this one.
	evidenced map[string]bool
}

// Push-sync task log lines, optionally behind the task log's own timestamp
// ("2026-10-13T07:01:02+02:00: "). syncNamespaceLineRe opens each source
// namespace ("Syncing datastore 'main', namespace 'a' into ..."; print_store_and_ns
// in pbs-api-types); syncGroupLineRe is one pushed snapshot (PBS BufferedLogger
// labels a group's lines "[vm/100]: ", push.rs logs "<snapshot time>: sync done").
var (
	syncNamespaceLineRe = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}T\S+: )?Syncing datastore '([^']+)', (?:root namespace|namespace '([^']*)') into `)
	syncGroupLineRe     = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}T\S+: )?\[((?:vm|ct|host)/[^\]]+)\]: (\d{4}-\d{2}-\d{2}T[0-9:]+Z): sync done$`)
)

func syncEvidenceKey(store, ns, group string, snapshot int64) string {
	return store + "\x00" + ns + "\x00" + group + "\x00" + strconv.FormatInt(snapshot, 10)
}

// parseSyncEvidence returns the source snapshots a push run's log shows it
// pushed; snapshot lines outside a namespace section are ignored.
func parseSyncEvidence(lines []string) map[string]bool {
	out := map[string]bool{}
	store, ns, inSection := "", "", false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if m := syncNamespaceLineRe.FindStringSubmatch(l); m != nil {
			store, ns, inSection = m[1], m[2], true
			continue
		}
		if m := syncGroupLineRe.FindStringSubmatch(l); m != nil && inSection {
			if ts, err := time.Parse(time.RFC3339, m[2]); err == nil {
				out[syncEvidenceKey(store, ns, m[1], ts.Unix())] = true
			}
		}
	}
	return out
}

// loadSyncEvidence reads the task log of each successful push job's last run
// and records the groups it handled. A log that cannot be read leaves the
// job without evidence (no offsite claim), reported as a warning.
func loadSyncEvidence(ctx context.Context, c *client.Client, jobs []pbsSyncJob) []string {
	var warnings []string
	for i := range jobs {
		j := &jobs[i]
		if !strings.EqualFold(j.Direction, "push") || !strings.EqualFold(j.LastRunState, "ok") || j.LastRunUPID == "" {
			continue
		}
		lines, err := fetchTaskLog(ctx, c, true, "localhost", j.LastRunUPID)
		if err != nil {
			warnings = append(warnings, "sync job "+j.ID+": last run log unreadable, so no offsite evidence from it: "+err.Error())
			continue
		}
		j.evidenced = parseSyncEvidence(lines)
	}
	return warnings
}

// admits applies the job's snapshot restrictions to rp (one of the guest's
// restore points rps) for its last run, which ended at end. The archive must
// predate the run (its start, from the run's UPID, when known). verified-only
// needs proof the archive was verified before that run started; a later
// verification does not qualify it retroactively.
func (j pbsSyncJob) admits(rp restorePoint, rps []restorePoint, end int64) bool {
	start := upidStartTime(j.LastRunUPID)
	if start > 0 && rp.Time >= start {
		return false
	}
	if bool(j.EncryptedOnly) && !rp.Encrypted {
		return false
	}
	if bool(j.VerifiedOnly) {
		vt := upidStartTime(rp.VerifyUPID)
		if rp.VerifyState != "ok" || start == 0 || vt == 0 || vt >= start {
			return false
		}
	}
	if n := int64(j.TransferLast); n > 0 {
		newer := int64(0) // snapshots of the group that existed at the run and are newer than rp
		for _, o := range rps {
			if o.canonical != rp.canonical && o.GuestType == rp.GuestType && o.pbsStore == rp.pbsStore && o.pbsNS == rp.pbsNS && o.Time > rp.Time && o.Time <= end {
				newer++
			}
		}
		if newer >= n {
			return false
		}
	}
	return true
}

// covers reports whether this job copies backup group typ/id in namespace ns
// of its store offsite (push direction, namespace reach, group filter).
func (j pbsSyncJob) covers(store, ns, typ, id string) bool {
	if j.Store != store || !strings.EqualFold(j.Direction, "push") || !nsReaches(j.NS, j.MaxDepth, ns) {
		return false
	}
	group := typ + "/" + id
	var includes, excludes []string
	for _, f := range j.GroupFilter {
		if rest, ok := strings.CutPrefix(f, "exclude:"); ok {
			excludes = append(excludes, rest)
		} else {
			includes = append(includes, strings.TrimPrefix(f, "include:"))
		}
	}
	matches := func(f string) bool {
		switch {
		case strings.HasPrefix(f, "type:"):
			return strings.TrimPrefix(f, "type:") == typ
		case strings.HasPrefix(f, "group:"):
			return strings.TrimPrefix(f, "group:") == group
		case strings.HasPrefix(f, "regex:"):
			re, err := regexp.Compile(strings.TrimPrefix(f, "regex:"))
			return err == nil && re.MatchString(group)
		}
		return false
	}
	if len(includes) > 0 {
		hit := false
		for _, f := range includes {
			if matches(f) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	for _, f := range excludes {
		if matches(f) {
			return false
		}
	}
	return true
}
