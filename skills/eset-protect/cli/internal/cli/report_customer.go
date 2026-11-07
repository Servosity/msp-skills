// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
//
// Hand-authored. `report customer <name>`: one customer's QBR one-pager.
// Local sections read the synced mirror (devices, device groups, v1 endpoint
// detections, incidents); live sections call ESET Connect scoped to the
// customer's COMPANY group (device vulnerabilities, patching details, recent
// vulnerability scans, and incident hydration, because the incident list
// leaves deviceUuids empty). Live rows are kept only when their nearest
// COMPANY group is the customer, exactly as the local sections filter by
// subtree. Every section fails independently: an error is recorded under
// fetch_failures and that section is null, the rest stand. A section that
// needs subtree membership is null when devices or groups cannot be read.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

const (
	reportCustomerStaleAfter      = 7 * 24 * time.Hour
	reportCustomerHydrationCap    = 50
	reportCustomerHydrationWorker = 4
	reportCustomerTopCVEs         = 10
	reportCustomerTopCategories   = 5
	reportCustomerIncidentClosed  = "INCIDENT_STATUS_CLOSED"
)

// reportCustomerSections lists the report sections in rendering order.
var reportCustomerSections = []string{"health", "detections", "incidents", "vulnerabilities", "patches", "scan_coverage"}

// reportCustomerDetection is one synced v1 endpoint detection.
type reportCustomerDetection struct {
	UUID          string `json:"uuid"`
	Category      string `json:"category"`
	TypeName      string `json:"typeName"`
	OccurTime     string `json:"occurTime"`
	SeverityLevel string `json:"severityLevel"`
	Context       struct {
		DeviceUUID string `json:"deviceUuid"`
	} `json:"context"`
	Responses []struct {
		DisplayName string `json:"displayName"`
	} `json:"responses"`
}

// reportCustomerIncident is one v2 incident (list row or hydrated GET).
type reportCustomerIncident struct {
	UUID             string          `json:"uuid"`
	DisplayName      string          `json:"displayName"`
	Severity         string          `json:"severity"`
	Status           string          `json:"status"`
	CreateTime       string          `json:"createTime"`
	UpdateTime       string          `json:"updateTime"`
	DeviceUUIDs      []string        `json:"deviceUuids"`
	TriageDuration   json.RawMessage `json:"triageDuration"`
	ResponseDuration json.RawMessage `json:"responseDuration"`
}

// reportCustomerScan is one v1ScanDetails row from /v1/scans/recent.
type reportCustomerScan struct {
	DeviceUUID        string `json:"deviceUuid"`
	StartTime         string `json:"startTime"`
	EndTime           string `json:"endTime"`
	EndedSuccessfully bool   `json:"endedSuccessfully"`
}

type reportCustomerRef struct {
	Name      string `json:"name"`
	UUID      string `json:"uuid"`
	GroupPath string `json:"group_path"`
}

type reportCustomerWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}

// reportCustomerSectionSync marks a section built from a mirror resource whose
// last sync did not complete (handfixes.json: mirror-views-synced-hint), so a
// partial mirror never reads as a complete quarter.
type reportCustomerSectionSync struct {
	Incomplete bool   `json:"incomplete"`
	SyncedHint string `json:"synced_hint,omitempty"`
}

type reportCustomerHealth struct {
	reportCustomerSectionSync
	Devices              int `json:"devices"`
	OK                   int `json:"ok"`
	AttentionRecommended int `json:"attention_recommended"`
	AttentionRequired    int `json:"attention_required"`
	Stale                int `json:"stale"`
	StaleDays            int `json:"stale_days"`
	Muted                int `json:"muted"`
	// UnattributedDevices counts synced devices whose device group is not in
	// the synced tree, so they may be this customer's (handfixes.json:
	// customer-views-count-unattributed-rows); they are not in the counts above.
	UnattributedDevices     int      `json:"unattributed_devices"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
}

type reportCustomerCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type reportCustomerDetections struct {
	reportCustomerSectionSync
	Total      int                   `json:"total"`
	BySeverity map[string]int        `json:"by_severity"`
	ByCategory []reportCustomerCount `json:"by_category"`
	Responses  map[string]int        `json:"responses"`
	Unhandled  int                   `json:"unhandled"`
	// UnattributedDetections counts window detections on a device the mirror
	// does not hold (deleted, or enrolled after the last devices sync) or
	// whose group is not in the synced tree: they may be this customer's, so
	// they are counted here instead of being dropped.
	UnattributedDetections     int      `json:"unattributed_detections"`
	UnattributedDetectionUUIDs []string `json:"unattributed_detection_uuids,omitempty"`
}

type reportCustomerIncidents struct {
	reportCustomerSectionSync
	Opened int `json:"opened"`
	// Closed counts incidents that are CLOSED now and whose updateTime falls
	// in the window. ESET exposes no close time, so this is not "closed during
	// the window": a rename after closing also updates updateTime.
	Closed              int            `json:"closed_updated_in_window"`
	OpenNow             int            `json:"open_now"`
	BySeverity          map[string]int `json:"by_severity"`
	MeanTriageSeconds   *float64       `json:"mean_triage_seconds"`
	MeanResponseSeconds *float64       `json:"mean_response_seconds"`
	Hydrated            int            `json:"hydrated"`
	HydrationSkipped    int            `json:"hydration_skipped"`
	HydrationFailed     int            `json:"hydration_failed"`
	// UnattributedIncidents counts incidents in the window (or open now) that
	// list no device, or only devices the mirror cannot place, and none of
	// this customer's; UnattributedOpen is how many of them are open now.
	UnattributedIncidents     int      `json:"unattributed_incidents"`
	UnattributedOpen          int      `json:"unattributed_open"`
	UnattributedIncidentUUIDs []string `json:"unattributed_incident_uuids,omitempty"`
}

type reportCustomerCVE struct {
	CVE            string `json:"cve"`
	RiskScore      int    `json:"risk_score"`
	Severity       string `json:"severity"`
	Devices        int    `json:"devices"`
	Software       string `json:"software"`
	PatchAvailable bool   `json:"patch_available"`
}

type reportCustomerVulns struct {
	reportCustomerSectionSync
	VulnerableDevices int                 `json:"vulnerable_devices"`
	Total             int                 `json:"total"`
	DistinctCVEs      int                 `json:"distinct_cves"`
	BySeverity        map[string]int      `json:"by_severity"`
	TopCVEs           []reportCustomerCVE `json:"top_cves"`
	Truncated         bool                `json:"truncated"`
	// UnattributedRows counts live rows whose owner is unknown (their group
	// and their device's group are both missing from the synced tree).
	UnattributedRows        int      `json:"unattributed_rows"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
}

type reportCustomerPatches struct {
	reportCustomerSectionSync
	PendingSecurityPatches     int            `json:"pending_security_patches"`
	DevicesWithPendingSecurity int            `json:"devices_with_pending_security"`
	FailedAttempts             int            `json:"failed_attempts"`
	FailedDevices              int            `json:"failed_devices"`
	FailedByStatus             map[string]int `json:"failed_by_status"`
	PatchSource                string         `json:"patch_source"`
	Truncated                  bool           `json:"truncated"`
	// UnattributedRows counts pending security patches and failed attempts
	// whose owner is unknown.
	UnattributedRows        int      `json:"unattributed_rows"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
}

type reportCustomerScanCoverage struct {
	reportCustomerSectionSync
	Devices        int     `json:"devices"`
	DevicesScanned int     `json:"devices_scanned"`
	CoveragePct    float64 `json:"coverage_pct"`
	FailedScans    int     `json:"failed_scans"`
	Truncated      bool    `json:"truncated"`
	Note           string  `json:"note,omitempty"`
	// UnattributedScans counts scans of a device whose owner is unknown.
	UnattributedScans       int      `json:"unattributed_scans"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
}

type reportCustomerFailure struct {
	Section string `json:"section"`
	Error   string `json:"error"`
}

type reportCustomerView struct {
	Customer        reportCustomerRef           `json:"customer"`
	Window          reportCustomerWindow        `json:"window"`
	Health          *reportCustomerHealth       `json:"health"`
	Detections      *reportCustomerDetections   `json:"detections"`
	Incidents       *reportCustomerIncidents    `json:"incidents"`
	Vulnerabilities *reportCustomerVulns        `json:"vulnerabilities"`
	Patches         *reportCustomerPatches      `json:"patches"`
	ScanCoverage    *reportCustomerScanCoverage `json:"scan_coverage"`
	FetchFailures   []reportCustomerFailure     `json:"fetch_failures"`
	// RejectedRows counts stored and live rows that could not be decoded.
	RejectedRows int `json:"rejected_rows"`
	// OtherCustomerRows counts live rows the recursive group scope returned
	// for a customer nested under this one; they are left out. Rows whose
	// owner is unknown are counted apart, in each section's unattributed_*.
	OtherCustomerRows int `json:"other_customer_rows"`
	// UnattributedRows sums every section's unattributed_* count: rows that
	// may be this customer's but that nothing in the mirror ties to any
	// customer. Each such section also carries incomplete=true.
	UnattributedRows int    `json:"unattributed_rows"`
	GeneratedAt      string `json:"generated_at"`
	Note             string `json:"note,omitempty"`
	// SyncedHint is set when a section was built from a mirror resource with
	// no completed sync; that section carries incomplete=true.
	SyncedHint string `json:"synced_hint,omitempty"`
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

// reportCustomerFail records a section failure; the section stays null.
func (v *reportCustomerView) fail(section string, err error) {
	v.FetchFailures = append(v.FetchFailures, reportCustomerFailure{Section: section, Error: err.Error()})
}

// reportCustomerResolve maps the positional to a customer. On a single-tenant
// tree (no COMPANY groups) "tenant" / "(tenant)" selects the whole account.
func reportCustomerResolve(tree *esetTree, q string) (esetGroup, bool, error) {
	lq := strings.ToLower(strings.TrimSpace(q))
	if !tree.hasCompanies {
		if lq == "tenant" || lq == strings.ToLower(esetCustomerTenant) {
			return esetGroup{Name: esetCustomerTenant}, true, nil
		}
		return esetGroup{}, false, fmt.Errorf("no customer (COMPANY) groups in the synced device-group tree; this looks like a single-tenant account, so pass 'tenant' to report on all of it")
	}
	g, err := tree.resolveCustomer(q)
	return g, false, err
}

// reportCustomerDevicesIn returns the synced devices in the customer's subtree.
func reportCustomerDevicesIn(devices []esetDevice, tree *esetTree, customerUUID string, all bool) map[string]esetDevice {
	out := make(map[string]esetDevice)
	for _, d := range devices {
		if all || tree.customerOf(d.ParentGroupUUID).UUID == customerUUID {
			out[d.UUID] = d
		}
	}
	return out
}

// reportCustomerHealthOf splits device health. A device whose last sync is
// older than staleAfter, or that never synced, counts as stale.
func reportCustomerHealthOf(devices map[string]esetDevice, now time.Time, staleAfter time.Duration) reportCustomerHealth {
	h := reportCustomerHealth{Devices: len(devices), StaleDays: int(staleAfter / (24 * time.Hour))}
	for _, d := range devices {
		switch shortEnum(d.FunctionalityStatus, "DEVICE_FUNCTIONALITY_STATUS_") {
		case "OK":
			h.OK++
		case "ATTENTION_RECOMMENDED":
			h.AttentionRecommended++
		case "ATTENTION_REQUIRED":
			h.AttentionRequired++
		}
		if d.IsMuted {
			h.Muted++
		}
		if seen, ok := d.lastSeen(); !ok || now.Sub(seen) > staleAfter {
			h.Stale++
		}
	}
	return h
}

// reportCustomerInWindow reports since <= t <= until (a zero since is open).
func reportCustomerInWindow(ts string, since, until time.Time) bool {
	t, ok := exposureParseTime(ts)
	if !ok {
		return false
	}
	return (since.IsZero() || !t.Before(since)) && !t.After(until)
}

func reportCustomerSeverityKey(s, prefix string) string {
	k := shortEnum(s, prefix)
	if k == "" {
		return "UNSPECIFIED"
	}
	return k
}

// reportCustomerDetectionsOf summarizes detections in the window on the
// customer's devices: severity, top categories, and the responses protection
// took. A window detection whose owner is unknown is counted in
// unattributed_detections and marks the section incomplete.
func reportCustomerDetectionsOf(dets []reportCustomerDetection, attrib func(deviceUUID string) esetAttribution, since, until time.Time) reportCustomerDetections {
	out := reportCustomerDetections{BySeverity: map[string]int{}, ByCategory: make([]reportCustomerCount, 0), Responses: map[string]int{}}
	cats := map[string]int{}
	var un esetUnattributed
	for _, d := range dets {
		if !reportCustomerInWindow(d.OccurTime, since, until) {
			continue
		}
		switch attrib(d.Context.DeviceUUID) {
		case esetAttribUnknown:
			un.add(d.UUID)
			continue
		case esetAttribOther:
			continue
		}
		out.Total++
		out.BySeverity[reportCustomerSeverityKey(d.SeverityLevel, exposureSeverityPrefix)]++
		cat := shortEnum(d.Category, "DETECTION_CATEGORY_")
		if cat == "" || cat == "UNSPECIFIED" {
			cat = d.TypeName
		}
		if cat == "" {
			cat = "UNSPECIFIED"
		}
		cats[cat]++
		handled := false
		for _, r := range d.Responses {
			if name := strings.TrimSpace(r.DisplayName); name != "" {
				out.Responses[name]++
				handled = true
			}
		}
		if !handled {
			out.Unhandled++
		}
	}
	out.ByCategory = reportCustomerTopCounts(cats, reportCustomerTopCategories)
	out.UnattributedDetections, out.UnattributedDetectionUUIDs = un.N, un.UUIDs()
	out.Incomplete = un.N > 0
	return out
}

func reportCustomerTopCounts(m map[string]int, n int) []reportCustomerCount {
	out := make([]reportCustomerCount, 0, len(m))
	for k, c := range m {
		out = append(out, reportCustomerCount{Name: k, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// reportCustomerParseDuration reads a protobuf duration: the JSON string
// form ("3600s", "1.5s"), a Go-style string ("1h30m"), a {seconds, nanos}
// object, or a bare number of seconds. ok is false when absent or unreadable.
func reportCustomerParseDuration(raw json.RawMessage) (float64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	switch s[0] {
	case '"':
		var str string
		if json.Unmarshal(raw, &str) != nil {
			return 0, false
		}
		str = strings.TrimSpace(str)
		if str == "" {
			return 0, false
		}
		if strings.HasSuffix(str, "s") {
			if v, err := strconv.ParseFloat(strings.TrimSuffix(str, "s"), 64); err == nil {
				return v, true
			}
		}
		if d, err := time.ParseDuration(str); err == nil {
			return d.Seconds(), true
		}
		return 0, false
	case '{':
		var obj struct {
			Seconds flexInt `json:"seconds"`
			Nanos   flexInt `json:"nanos"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return 0, false
		}
		return float64(obj.Seconds) + float64(obj.Nanos)/1e9, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// reportCustomerIncidentTouched reports whether an incident belongs in the
// window (created or updated in it) or is still open.
func reportCustomerIncidentTouched(inc reportCustomerIncident, since, until time.Time) bool {
	return inc.Status != reportCustomerIncidentClosed ||
		reportCustomerInWindow(inc.CreateTime, since, until) ||
		reportCustomerInWindow(inc.UpdateTime, since, until)
}

// reportCustomerHydrationPlan picks incidents whose list row lacks
// deviceUuids, newest first, up to max; the rest are counted as skipped.
func reportCustomerHydrationPlan(incs []reportCustomerIncident, since, until time.Time, max int) ([]string, int) {
	type cand struct {
		uuid    string
		created time.Time
	}
	cands := make([]cand, 0)
	for _, inc := range incs {
		if len(inc.DeviceUUIDs) > 0 || inc.UUID == "" || !reportCustomerIncidentTouched(inc, since, until) {
			continue
		}
		t, _ := exposureParseTime(inc.CreateTime)
		cands = append(cands, cand{uuid: inc.UUID, created: t})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].created.After(cands[j].created) })
	out := make([]string, 0, len(cands))
	for i, c := range cands {
		if max > 0 && i >= max {
			break
		}
		out = append(out, c.uuid)
	}
	return out, len(cands) - len(out)
}

// reportCustomerIncidentsOf attributes incidents to the customer (any device
// in scope, or every incident in tenant mode) and reports the window's
// opened and closed counts, what is open now, and mean triage and response
// durations over incidents created or updated in the window. ESET exposes no
// close time, so CLOSED incidents are reported as closed_updated_in_window
// (status CLOSED now, updateTime in the window), not as closures in the window.
//
// An incident that names no device, or only devices whose owner is unknown,
// and none of this customer's may be this customer's: when it is in the
// window or still open it is counted in unattributed_incidents (open ones
// also in unattributed_open) and marks the section incomplete, as triage
// --customer does.
func reportCustomerIncidentsOf(incs []reportCustomerIncident, attrib func(deviceUUID string) esetAttribution, all bool, since, until time.Time) reportCustomerIncidents {
	out := reportCustomerIncidents{BySeverity: map[string]int{}}
	var triageSum, respSum float64
	var triageN, respN int
	var un esetUnattributed
	for _, inc := range incs {
		attributed := all
		unresolved := len(inc.DeviceUUIDs) == 0
		for _, d := range inc.DeviceUUIDs {
			switch attrib(d) {
			case esetAttribIn:
				attributed = true
			case esetAttribUnknown:
				unresolved = true
			}
		}
		if !attributed {
			if unresolved && reportCustomerIncidentTouched(inc, since, until) {
				un.add(inc.UUID)
				if inc.Status != reportCustomerIncidentClosed {
					out.UnattributedOpen++
				}
			}
			continue
		}
		created := reportCustomerInWindow(inc.CreateTime, since, until)
		updated := reportCustomerInWindow(inc.UpdateTime, since, until)
		closed := inc.Status == reportCustomerIncidentClosed
		if !closed {
			out.OpenNow++
		}
		if created {
			out.Opened++
			out.BySeverity[reportCustomerSeverityKey(inc.Severity, "INCIDENT_SEVERITY_LEVEL_")]++
		}
		if closed && updated {
			out.Closed++
		}
		if !created && !updated {
			continue
		}
		if v, ok := reportCustomerParseDuration(inc.TriageDuration); ok && v >= 0 {
			triageSum += v
			triageN++
		}
		if v, ok := reportCustomerParseDuration(inc.ResponseDuration); ok && v >= 0 {
			respSum += v
			respN++
		}
	}
	if triageN > 0 {
		m := math.Round(triageSum/float64(triageN)*10) / 10
		out.MeanTriageSeconds = &m
	}
	if respN > 0 {
		m := math.Round(respSum/float64(respN)*10) / 10
		out.MeanResponseSeconds = &m
	}
	out.UnattributedIncidents, out.UnattributedIncidentUUIDs = un.N, un.UUIDs()
	out.Incomplete = un.N > 0
	return out
}

// reportCustomerVulnsOf summarizes live device vulnerabilities already scoped
// to the customer's group.
func reportCustomerVulnsOf(vulns []exposureVuln, truncated bool) reportCustomerVulns {
	out := reportCustomerVulns{BySeverity: map[string]int{}, TopCVEs: make([]reportCustomerCVE, 0), Truncated: truncated}
	devices := map[string]bool{}
	type acc struct {
		row     reportCustomerCVE
		rank    int
		devices map[string]bool
	}
	byCVE := map[string]*acc{}
	for _, v := range vulns {
		core := v.core()
		out.Total++
		if v.DeviceUUID != "" {
			devices[v.DeviceUUID] = true
		}
		out.BySeverity[reportCustomerSeverityKey(core.Severity, exposureSeverityPrefix)]++
		cve := strings.ToUpper(strings.TrimSpace(core.CVENumber))
		if cve == "" {
			continue
		}
		a := byCVE[cve]
		if a == nil {
			a = &acc{row: reportCustomerCVE{CVE: cve, Software: v.name()}, rank: -1, devices: map[string]bool{}}
			byCVE[cve] = a
		}
		if int(core.RiskScore) > a.row.RiskScore {
			a.row.RiskScore = int(core.RiskScore)
		}
		if r := exposureSeverityRank(core.Severity); r > a.rank {
			a.rank = r
			a.row.Severity = exposureSeverityShort(core.Severity)
		}
		if core.PatchAvailable || len(v.AvailablePatchUUIDs) > 0 {
			a.row.PatchAvailable = true
		}
		if v.DeviceUUID != "" {
			a.devices[v.DeviceUUID] = true
		}
	}
	out.VulnerableDevices = len(devices)
	out.DistinctCVEs = len(byCVE)
	for _, a := range byCVE {
		row := a.row
		row.Devices = len(a.devices)
		out.TopCVEs = append(out.TopCVEs, row)
	}
	sort.Slice(out.TopCVEs, func(i, j int) bool {
		a, b := out.TopCVEs[i], out.TopCVEs[j]
		if a.RiskScore != b.RiskScore {
			return a.RiskScore > b.RiskScore
		}
		if a.Devices != b.Devices {
			return a.Devices > b.Devices
		}
		return a.CVE > b.CVE
	})
	if len(out.TopCVEs) > reportCustomerTopCVEs {
		out.TopCVEs = out.TopCVEs[:reportCustomerTopCVEs]
	}
	return out
}

// reportCustomerPatchesOf counts pending security patches (one device-patch
// row per patch per device) and failed patch attempts.
func reportCustomerPatchesOf(patches []patchesBacklogDevicePatch, details []patchesBacklogProcess) reportCustomerPatches {
	out := reportCustomerPatches{FailedByStatus: map[string]int{}}
	pendingDevices := map[string]bool{}
	for _, p := range patches {
		if !p.isSecurity() {
			continue
		}
		out.PendingSecurityPatches++
		if p.DeviceUUID != "" {
			pendingDevices[p.DeviceUUID] = true
		}
	}
	out.DevicesWithPendingSecurity = len(pendingDevices)
	failedDevices := map[string]bool{}
	for _, d := range details {
		st := d.status()
		if !patchesBacklogIsFailure(st) {
			continue
		}
		out.FailedAttempts++
		out.FailedByStatus[patchesBacklogStatusShort(st)]++
		if d.DeviceUUID != "" {
			failedDevices[d.DeviceUUID] = true
		}
	}
	out.FailedDevices = len(failedDevices)
	return out
}

// reportCustomerScanCoverageOf counts subtree devices with a successful
// vulnerability scan in the window.
func reportCustomerScanCoverageOf(scans []reportCustomerScan, subtree map[string]esetDevice, since, until time.Time) reportCustomerScanCoverage {
	out := reportCustomerScanCoverage{Devices: len(subtree)}
	ok := map[string]bool{}
	for _, s := range scans {
		ts := s.EndTime
		if ts == "" {
			ts = s.StartTime
		}
		if !reportCustomerInWindow(ts, since, until) {
			continue
		}
		if !s.EndedSuccessfully {
			out.FailedScans++
			continue
		}
		if _, in := subtree[s.DeviceUUID]; in {
			ok[s.DeviceUUID] = true
		}
	}
	out.DevicesScanned = len(ok)
	if out.Devices > 0 {
		out.CoveragePct = math.Round(float64(out.DevicesScanned)/float64(out.Devices)*1000) / 10
	}
	return out
}

// reportCustomerDecode decodes one source's rows; a row that is not a JSON
// object (null decodes "successfully" into a zero row) or does not decode is
// counted in rej under "<kind> row <index>".
func reportCustomerDecode[T any](items []json.RawMessage, kind string, rej *esetRejects) []T {
	out := make([]T, 0, len(items))
	for i, raw := range items {
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("%s row %d", kind, i), errors.New("not a JSON object"))
			continue
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			rej.add(fmt.Sprintf("%s row %d", kind, i), err)
			continue
		}
		out = append(out, v)
	}
	return out
}

// reportCustomerSourceErr reports a source whose every row was rejected: it
// failed entirely, so its section must be null rather than zero.
func reportCustomerSourceErr(kind string, total int, src esetRejects) error {
	if total > 0 && src.N >= total {
		return fmt.Errorf("all %d %s row(s) could not be decoded (first: %s)", total, kind, src.First)
	}
	return nil
}

// reportCustomerAddRejects folds one source's rejects into the report total.
func reportCustomerAddRejects(dst *esetRejects, src esetRejects) {
	dst.N += src.N
	if dst.First == "" {
		dst.First = src.First
	}
}

// reportCustomerLive carries what the live sections share: the bounded
// lister, the customer scope parameters, and the attribution filter that
// keeps rows whose nearest COMPANY is the customer.
type reportCustomerLive struct {
	list   func(url string, params map[string]string, key string) (esetPageResult, error)
	params func(extra map[string]string) map[string]string
	scope  exposureScope
	target string // customer UUID; "" in tenant mode keeps every row
	rej    *esetRejects
	other  int
}

func reportCustomerIncidentURL(uuid string) string {
	return replacePathParam(esetHost("incident-management", "/v2/incidents/{incidentUuid}"), "incidentUuid", uuid)
}

func reportCustomerScansURL() string {
	return esetHost("vulnerability-management", "/v1/scans/recent")
}

// reportCustomerHydrate fetches full incidents for the planned UUIDs and
// merges their deviceUuids into incs. It returns the counts and the errors.
func reportCustomerHydrate(ctx context.Context, live exposureLiveAPI, incs []reportCustomerIncident, uuids []string) (int, []cliutil.FanoutError) {
	results, errs := cliutil.FanoutRun(ctx, uuids, func(u string) string { return u },
		func(ctx context.Context, u string) (reportCustomerIncident, error) {
			raw, err := live.get(ctx, reportCustomerIncidentURL(u))
			if err != nil {
				return reportCustomerIncident{}, err
			}
			var env struct {
				Incident json.RawMessage `json:"incident"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				return reportCustomerIncident{}, fmt.Errorf("decoding incident %s: %w", u, err)
			}
			if !esetIsObject(env.Incident) {
				return reportCustomerIncident{}, fmt.Errorf("decoding incident %s: the response has no incident object", u)
			}
			var inc reportCustomerIncident
			if err := json.Unmarshal(env.Incident, &inc); err != nil {
				return reportCustomerIncident{}, fmt.Errorf("decoding incident %s: %w", u, err)
			}
			return inc, nil
		}, cliutil.WithConcurrency(reportCustomerHydrationWorker))
	byUUID := make(map[string]reportCustomerIncident, len(results))
	for _, r := range results {
		byUUID[r.Source] = r.Value
	}
	for i := range incs {
		if h, ok := byUUID[incs[i].UUID]; ok {
			incs[i].DeviceUUIDs = h.DeviceUUIDs
			if len(h.TriageDuration) > 0 {
				incs[i].TriageDuration = h.TriageDuration
			}
			if len(h.ResponseDuration) > 0 {
				incs[i].ResponseDuration = h.ResponseDuration
			}
		}
	}
	return len(results), errs
}

// reportCustomerDataSourceErr rejects --data-source live and local before
// any IO: the report mixes the synced mirror with live ESET calls.
func reportCustomerDataSourceErr(dataSource string) error {
	switch dataSource {
	case "", "auto":
		return nil
	case "live":
		return usageErr(fmt.Errorf("report customer supports only --data-source auto: health, detections and incidents come from the synced mirror and have no live equivalent here"))
	default:
		return usageErr(fmt.Errorf("report customer supports only --data-source auto: vulnerabilities, patching details and scans are live-only in ESET Connect, so a mirror-only report cannot be built"))
	}
}

// reportCustomerNeedsMembers reports whether a section depends on knowing
// which devices are in the customer's subtree. In tenant mode every row is in
// scope, so only health and scan coverage (which count devices) need it.
func reportCustomerNeedsMembers(section string, all bool) bool {
	if section == "health" || section == "scan_coverage" {
		return true
	}
	return !all
}

func newNovelReportCustomerCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagDB string
	var flagMaxScanPages int

	cmd := &cobra.Command{
		Use:   "customer <name>",
		Short: "One customer's quarterly story in one document: device health, detections by severity and response",
		Long: strings.Trim(`
Build one customer's QBR one-pager for its COMPANY group and everything under it.
Pass the customer's name (exact, or a unique substring) or group UUID; on a
single-tenant account with no customer groups, pass 'tenant'.

Sections:
  health           synced devices: OK / attention recommended / attention required,
                   stale (no sync in 7 days, or never), muted
  detections       synced v1 endpoint detections in --since: by severity, top 5
                   categories, responses taken, unhandled (no response)
  incidents        synced incidents opened in --since, closed (status CLOSED with updateTime in --since; ESET has no close time), open now, mean
                   triage and response seconds; incidents whose list row has no
                   devices are fetched live to attribute them (up to 50)
  vulnerabilities  live, scoped to the customer's group: vulnerable devices,
                   by severity, top 10 CVEs by risk
  patches          pending security patches (synced, live fallback) and failed
                   patch attempts in --since (live)
  scan_coverage    live recent vulnerability scans: devices with a successful
                   scan vs devices in the customer (ESET keeps only a few days)

ESET scopes live calls to the customer's group recursively, so live rows of a
customer nested under this one are left out (counted in other_customer_rows),
matching how the synced sections are filtered. A live row whose device group is
not yet in the synced tree (created after the last sync) is attributed through
its device's synced group when there is one. A row nothing in the mirror ties
to any customer (a detection or incident on a device the mirror does not hold,
for example a deleted endpoint; an incident that names no device; a row whose
group and device group are both missing from the synced tree) may be this
customer's: it is not in the totals, it is counted in the section's
unattributed_* field (uuids alongside, open incidents in unattributed_open),
the section is marked incomplete, and unattributed_rows sums them. Re-sync
devices and device-groups to attribute them. Rows that cannot be decoded
(including null rows) are counted in rejected_rows. A section that cannot be built is null and its error
is listed under fetch_failures; the rest of the report still prints. When synced
devices or groups cannot be read, every section that needs to know which devices
belong to the customer is null. Only --data-source auto is supported: the report
mixes the synced mirror with live ESET calls.

Use this command for one customer's QBR one-pager across health, detections,
incidents, vulnerabilities and patches. Do NOT use it for vulnerabilities alone;
use 'report vulnerabilities' instead. Do NOT use it for current device health
across all customers; use 'fleet health' instead.`, "\n"),
		Example: strings.Trim(`
  eset-protect-cli report customer 'Acme Dental' --since 90d --agent
  eset-protect-cli report customer 6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f --since 30d --json
  eset-protect-cli report customer tenant --since 90d`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "customer=example-value",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "report customer")
			}
			if err := reportCustomerDataSourceErr(flags.dataSource); err != nil {
				_ = cmd.Usage()
				return err
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass the customer name or group UUID (or 'tenant' on a single-tenant account)"))
			}
			if len(args) > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass one customer; quote names with spaces, e.g. 'Acme Dental'"))
			}
			since, err := esetSince(flagSince)
			if err != nil {
				return err
			}
			now := time.Now()
			view := reportCustomerView{
				Customer:      reportCustomerRef{Name: strings.TrimSpace(args[0])},
				Window:        reportCustomerWindow{Until: now.UTC().Format(time.RFC3339)},
				FetchFailures: make([]reportCustomerFailure, 0),
				GeneratedAt:   now.UTC().Format(time.RFC3339),
			}
			if !since.IsZero() {
				view.Window.Since = since.UTC().Format(time.RFC3339)
			}
			// mirror and all are set once the store is open and the customer is
			// resolved; finish() reads them to flag sections built from a sync
			// that did not complete.
			var mirror esetMirrorState
			all := false
			var rej esetRejects
			finish := func() error {
				view.RejectedRows = rej.N
				if n := len(view.FetchFailures); n > 0 {
					names := make([]string, 0, n)
					for _, f := range view.FetchFailures {
						names = append(names, f.Section)
					}
					esetFprintf(cmd.ErrOrStderr(), "warning: %d report section fetch(es) failed (%s); see fetch_failures\n", n, strings.Join(names, ", "))
				}
				rej.warn(cmd)
				reportCustomerMarkIncomplete(&view, mirror, all)
				view.UnattributedRows = reportCustomerUnattributedTotal(view)
				if view.UnattributedRows > 0 {
					esetFprintf(cmd.ErrOrStderr(), "warning: %d row(s) could not be tied to any customer (%s%s) and are not in %s's totals; see each section's unattributed_* fields\n", view.UnattributedRows, esetUnattributedCause, esetHiddenParentCause(view.parentNotVisible), view.Customer.Name)
				}
				// Health, detections and incidents always come from the mirror,
				// so the --agent meta.source is "local", not the "auto" default
				// (handfixes.json: agent-meta-source-matches-results).
				flags.agentSource = "local"
				return reportCustomerRender(cmd.OutOrStdout(), flags, view)
			}

			resources := []string{"device-groups", "devices", "detections", "incidents"}
			db, err := openEsetStore(cmd, flagDB, resources...)
			if err != nil {
				return err
			}
			if db == nil {
				// The customer can only be resolved through the synced group
				// tree; an empty "success" report would hide that.
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("cannot resolve customer %q: no local mirror; run '%s sync --resources device-groups,devices,detections,incidents' first", args[0], esetCLIName))
			}
			defer db.Close()
			esetSyncHints(cmd, db, flags.maxAge, resources...)
			mirror = esetMirrorCheck(db, append(resources, "device-patches")...)

			groups, gerr := loadEsetGroups(db)
			if gerr != nil {
				// Without the tree neither the customer nor any row can be placed.
				cause := fmt.Errorf("customer attribution unavailable: reading synced device groups: %w", gerr)
				for _, sec := range reportCustomerSections {
					view.fail(sec, cause)
				}
				return finish()
			}
			tree := newEsetTree(groups)
			view.parentNotVisible = tree.parentNotVisible()
			view.ParentNotVisible = tree.parentNotVisibleGroups()
			cust, tenantMode, err := reportCustomerResolve(tree, args[0])
			all = tenantMode
			if err != nil {
				return exposureCustomerErr(err)
			}
			view.Customer = reportCustomerRef{Name: cust.Name, UUID: cust.UUID, GroupPath: tree.path(cust.UUID)}

			devices, derr := loadEsetDevices(db)
			var memberErr error
			if derr != nil {
				memberErr = fmt.Errorf("subtree membership unknown: reading synced devices: %w", derr)
			}
			// blocked records a membership-dependent section as failed.
			blocked := func(section string) bool {
				if memberErr == nil || !reportCustomerNeedsMembers(section, all) {
					return false
				}
				view.fail(section, memberErr)
				return true
			}
			subtree := reportCustomerDevicesIn(devices, tree, cust.UUID, all)
			byUUID := esetDevicesByUUID(devices)
			// attrib places a device-keyed row: this customer's, another
			// known owner's, or owner unknown (handfixes.json:
			// customer-views-count-unattributed-rows).
			attrib := func(deviceUUID string) esetAttribution {
				if all {
					return esetAttribIn
				}
				owner, known := esetDeviceOwner(tree, byUUID, deviceUUID)
				return esetAttribute(owner, known, cust.UUID)
			}
			if !blocked("health") {
				h := reportCustomerHealthOf(subtree, now, reportCustomerStaleAfter)
				if !all {
					var un esetUnattributed
					for _, d := range devices {
						if _, known := tree.ownerOf(d.ParentGroupUUID); !known {
							un.add(d.UUID)
						}
					}
					h.UnattributedDevices, h.UnattributedDeviceUUIDs = un.N, un.UUIDs()
					h.Incomplete = un.N > 0
				}
				view.Health = &h
			}

			if !blocked("detections") {
				if rows, lerr := loadEsetRaw(db, "detections"); lerr != nil {
					view.fail("detections", fmt.Errorf("reading synced detections: %w", lerr))
				} else {
					var src esetRejects
					dets := reportCustomerDecode[reportCustomerDetection](rows, "detection", &src)
					// A detection whose occurTime cannot be read cannot be
					// placed in the window; count it instead of dropping it.
					timed := dets[:0]
					for i, d := range dets {
						if _, ok := exposureParseTime(d.OccurTime); !ok {
							src.add(fmt.Sprintf("detection row %d", i), fmt.Errorf("unreadable occurTime %q", d.OccurTime))
							continue
						}
						timed = append(timed, d)
					}
					dets = timed
					reportCustomerAddRejects(&rej, src)
					if serr := reportCustomerSourceErr("detection", len(rows), src); serr != nil {
						view.fail("detections", serr)
					} else {
						d := reportCustomerDetectionsOf(dets, attrib, since, now)
						view.Detections = &d
					}
				}
			}

			live, liveErr := reportCustomerNewLive(flags)
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			maxPages := exposureMaxPages(flagMaxScanPages)
			scope := newExposureScope(groups, devices)
			lv := &reportCustomerLive{
				list: func(url string, params map[string]string, key string) (esetPageResult, error) {
					if liveErr != nil {
						return esetPageResult{}, liveErr
					}
					return live.list(ctx, url, params, key, maxPages)
				},
				params: func(extra map[string]string) map[string]string {
					p := map[string]string{}
					if cust.UUID != "" {
						p["deviceGroupUuid"] = cust.UUID
					}
					for k, v := range extra {
						p[k] = v
					}
					return p
				},
				scope:  scope,
				target: cust.UUID,
				rej:    &rej,
			}

			if !blocked("incidents") {
				if rows, lerr := loadEsetRaw(db, "incidents"); lerr != nil {
					view.fail("incidents", fmt.Errorf("reading synced incidents: %w", lerr))
				} else {
					var src esetRejects
					incs := reportCustomerDecode[reportCustomerIncident](rows, "incident", &src)
					reportCustomerAddRejects(&rej, src)
					if serr := reportCustomerSourceErr("incident", len(rows), src); serr != nil {
						view.fail("incidents", serr)
					} else {
						var plan []string
						skipped := 0
						if !all {
							plan, skipped = reportCustomerHydrationPlan(incs, since, now, reportCustomerHydrationCap)
						}
						hydrated, failed := 0, 0
						if len(plan) > 0 {
							if liveErr != nil {
								failed = len(plan)
								view.fail("incidents", fmt.Errorf("hydrating %d incident(s): %w", failed, liveErr))
							} else {
								var errs []cliutil.FanoutError
								hydrated, errs = reportCustomerHydrate(ctx, live, incs, plan)
								failed = len(errs)
								if failed > 0 {
									view.fail("incidents", fmt.Errorf("%d of %d incident hydration(s) failed, first: %s: %v", failed, len(plan), errs[0].Source, errs[0].Err))
								}
							}
						}
						in := reportCustomerIncidentsOf(incs, attrib, all, since, now)
						in.Hydrated, in.HydrationSkipped, in.HydrationFailed = hydrated, skipped, failed
						view.Incidents = &in
					}
				}
			}

			if !blocked("vulnerabilities") {
				if v, verr := reportCustomerVulnsSection(lv); verr != nil {
					view.fail("vulnerabilities", verr)
				} else {
					view.Vulnerabilities = &v
				}
			}

			if !blocked("patches") {
				if rows, lerr := loadEsetRaw(db, "device-patches"); lerr != nil {
					view.fail("patches", fmt.Errorf("reading synced device patches: %w", lerr))
				} else if p, perr := reportCustomerPatchesSection(rows, lv, since, now); perr != nil {
					view.fail("patches", perr)
				} else {
					view.Patches = &p
				}
			}

			if !blocked("scan_coverage") {
				if sc, serr := reportCustomerScansSection(lv, subtree, since, now); serr != nil {
					view.fail("scan_coverage", serr)
				} else {
					view.ScanCoverage = &sc
				}
			}
			view.OtherCustomerRows = lv.other
			return finish()
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "90d", "Report window (e.g. 30d, 90d, 12w)")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", exposureDefaultMaxScanPages, "Maximum pages of 1000 rows per live list (1 under the dogfood harness)")
	cmd.Flags().StringVar(&flagDB, "db", "", esetDBFlagUsage)
	return cmd
}

// reportCustomerVulnsSection summarizes the customer's live device
// vulnerabilities, leaving out rows of nested customers and of device groups
// not in the synced tree (both counted in lv.other).
func reportCustomerVulnsSection(lv *reportCustomerLive) (reportCustomerVulns, error) {
	page, err := lv.list(exposureVulnsURL(), lv.params(nil), "vulnerabilities")
	if err != nil {
		return reportCustomerVulns{}, err
	}
	var src esetRejects
	vulns := exposureDecodeVulns(page.Items, &src)
	reportCustomerAddRejects(lv.rej, src)
	if serr := reportCustomerSourceErr("vulnerability", len(page.Items), src); serr != nil {
		return reportCustomerVulns{}, serr
	}
	vulns, n, unknown := exposureFilterRows(vulns, exposureVulnKey, lv.scope, lv.target)
	lv.other += n
	out := reportCustomerVulnsOf(vulns, page.Truncated)
	var un esetUnattributed
	for _, v := range unknown {
		un.add(v.DeviceUUID)
	}
	out.UnattributedRows, out.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	out.Incomplete = un.N > 0
	return out, nil
}

// reportCustomerScansSection measures scan coverage from the customer's live
// recent scans, leaving out scans of nested customers' devices and of devices
// whose group is not in the synced tree.
func reportCustomerScansSection(lv *reportCustomerLive, subtree map[string]esetDevice, since, until time.Time) (reportCustomerScanCoverage, error) {
	page, err := lv.list(reportCustomerScansURL(), lv.params(nil), "scanDetails")
	if err != nil {
		return reportCustomerScanCoverage{}, err
	}
	var src esetRejects
	scans := reportCustomerDecode[reportCustomerScan](page.Items, "scan", &src)
	reportCustomerAddRejects(lv.rej, src)
	if serr := reportCustomerSourceErr("scan", len(page.Items), src); serr != nil {
		return reportCustomerScanCoverage{}, serr
	}
	scans, n, unknown := exposureFilterRows(scans, func(s reportCustomerScan) (string, string) { return "", s.DeviceUUID }, lv.scope, lv.target)
	lv.other += n
	sc := reportCustomerScanCoverageOf(scans, subtree, since, until)
	sc.Truncated = page.Truncated
	var un esetUnattributed
	for _, s := range unknown {
		ts := s.EndTime
		if ts == "" {
			ts = s.StartTime
		}
		if reportCustomerInWindow(ts, since, until) {
			un.add(s.DeviceUUID)
		}
	}
	sc.UnattributedScans, sc.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	sc.Incomplete = un.N > 0
	if since.IsZero() || until.Sub(since) > 7*24*time.Hour {
		sc.Note = "ESET keeps only the last few days of vulnerability scans, so coverage reflects recent scanning"
	}
	return sc, nil
}

// reportCustomerPatchesSection reads device patches (local first, live when
// the mirror has none) and the failed attempts in the window, keeping only
// the customer's rows: local rows by nearest COMPANY, live rows the same way
// with nested customers and groups not in the synced tree counted in lv.other.
func reportCustomerPatchesSection(rows []json.RawMessage, lv *reportCustomerLive, since, until time.Time) (reportCustomerPatches, error) {
	source := "local"
	truncated := false
	var un esetUnattributed
	var patches []patchesBacklogDevicePatch
	if len(rows) > 0 {
		var src esetRejects
		patches = patchesBacklogDecodePatches(rows, &src)
		reportCustomerAddRejects(lv.rej, src)
		if serr := reportCustomerSourceErr("device-patch", len(rows), src); serr != nil {
			return reportCustomerPatches{}, serr
		}
		if lv.target != "" {
			patchesBacklogUnattributedPatches(patches, lv.scope, true, &un)
			patches = patchesBacklogInCustomer(patches, lv.scope, lv.target)
		}
	} else {
		source = "live"
		page, err := lv.list(patchesBacklogDevicePatchesURL(), lv.params(nil), "devices")
		if err != nil {
			return reportCustomerPatches{}, err
		}
		var src esetRejects
		patches = patchesBacklogDecodePatches(page.Items, &src)
		reportCustomerAddRejects(lv.rej, src)
		if serr := reportCustomerSourceErr("device-patch", len(page.Items), src); serr != nil {
			return reportCustomerPatches{}, serr
		}
		var n int
		var unknown []patchesBacklogDevicePatch
		patches, n, unknown = exposureFilterRows(patches, patchesBacklogPatchKey, lv.scope, lv.target)
		patchesBacklogUnattributedPatches(unknown, lv.scope, true, &un)
		lv.other += n
		truncated = page.Truncated
	}
	window := patchesBacklogWindowParams("", since, until)
	page, err := lv.list(patchesBacklogProcessURL(), lv.params(window), "patchingDetails")
	if err != nil {
		return reportCustomerPatches{}, err
	}
	var src esetRejects
	details := patchesBacklogDecodeProcesses(page.Items, &src)
	reportCustomerAddRejects(lv.rej, src)
	if serr := reportCustomerSourceErr("patching-detail", len(page.Items), src); serr != nil {
		return reportCustomerPatches{}, serr
	}
	details, n, unknownDetails := exposureFilterRows(details, patchesBacklogProcessKey, lv.scope, lv.target)
	patchesBacklogUnattributedDetails(unknownDetails, lv.scope, &un)
	lv.other += n
	out := reportCustomerPatchesOf(patches, details)
	out.PatchSource = source
	out.Truncated = truncated || page.Truncated
	out.UnattributedRows, out.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	out.Incomplete = un.N > 0
	return out, nil
}

// reportCustomerSectionResources names the mirror resources a section is
// built from. device-groups and devices place rows in the customer's subtree;
// in tenant mode every row is in scope, so only the device count needs
// devices. patches reads device-patches only when it used the mirror.
func reportCustomerSectionResources(section string, all bool, patchSource string) []string {
	base := []string{"device-groups", "devices"}
	if all {
		base = nil
	}
	switch section {
	case "health", "scan_coverage":
		if all {
			return []string{"devices"}
		}
		return base
	case "detections", "incidents":
		return append(base, section)
	case "patches":
		if patchSource == "local" {
			return append(base, "device-patches")
		}
		return base
	}
	return base
}

// reportCustomerMarkIncomplete flags every present section built from a
// resource with no completed sync (incomplete=true plus its synced_hint) and
// sets the report's synced_hint over every resource the report relied on,
// device-groups always (the customer is resolved through the tree).
func reportCustomerMarkIncomplete(view *reportCustomerView, st esetMirrorState, all bool) {
	used := map[string]bool{"device-groups": true}
	mark := func(sec *reportCustomerSectionSync, section, patchSource string) {
		res := reportCustomerSectionResources(section, all, patchSource)
		for _, r := range res {
			used[r] = true
		}
		if h := st.hintFor(res...); h != "" {
			sec.Incomplete, sec.SyncedHint = true, h
		}
	}
	if view.Health != nil {
		mark(&view.Health.reportCustomerSectionSync, "health", "")
	}
	if view.Detections != nil {
		mark(&view.Detections.reportCustomerSectionSync, "detections", "")
	}
	if view.Incidents != nil {
		mark(&view.Incidents.reportCustomerSectionSync, "incidents", "")
	}
	if view.Vulnerabilities != nil {
		mark(&view.Vulnerabilities.reportCustomerSectionSync, "vulnerabilities", "")
	}
	if view.Patches != nil {
		mark(&view.Patches.reportCustomerSectionSync, "patches", view.Patches.PatchSource)
	}
	if view.ScanCoverage != nil {
		mark(&view.ScanCoverage.reportCustomerSectionSync, "scan_coverage", "")
	}
	var usedList []string
	for _, r := range []string{"device-groups", "devices", "detections", "incidents", "device-patches"} {
		if used[r] {
			usedList = append(usedList, r)
		}
	}
	view.SyncedHint = st.hintFor(usedList...)
}

// reportCustomerUnattributedTotal sums every present section's unattributed
// count (handfixes.json: customer-views-count-unattributed-rows).
func reportCustomerUnattributedTotal(view reportCustomerView) int {
	n := 0
	if view.Health != nil {
		n += view.Health.UnattributedDevices
	}
	if view.Detections != nil {
		n += view.Detections.UnattributedDetections
	}
	if view.Incidents != nil {
		n += view.Incidents.UnattributedIncidents
	}
	if view.Vulnerabilities != nil {
		n += view.Vulnerabilities.UnattributedRows
	}
	if view.Patches != nil {
		n += view.Patches.UnattributedRows
	}
	if view.ScanCoverage != nil {
		n += view.ScanCoverage.UnattributedScans
	}
	return n
}

// reportCustomerUnattributedNotes is the human note per section for rows
// that may be this customer's but could not be attributed.
func reportCustomerUnattributedNotes(view reportCustomerView) map[string]string {
	out := map[string]string{}
	name := view.Customer.Name
	add := func(sec string, n int, what string, ids []string, next string) {
		if line := esetUnattributedLine(n, what, ids, name, next, view.parentNotVisible); line != "" {
			out[sec] = line
		}
	}
	if h := view.Health; h != nil {
		add("health", h.UnattributedDevices, "synced device(s)", h.UnattributedDeviceUUIDs, "Run '"+esetCLIName+" sync --resources device-groups' to attribute them.")
	}
	if d := view.Detections; d != nil {
		add("detections", d.UnattributedDetections, "detection(s) in the window", d.UnattributedDetectionUUIDs, "Check each with '"+esetCLIName+" detections get <uuid>', or run '"+esetCLIName+" sync --resources devices,device-groups' to attribute them.")
	}
	if in := view.Incidents; in != nil {
		what := "incident(s)"
		if in.UnattributedOpen > 0 {
			what = fmt.Sprintf("incident(s) (%d open now)", in.UnattributedOpen)
		}
		add("incidents", in.UnattributedIncidents, what, in.UnattributedIncidentUUIDs, "Check each with '"+esetCLIName+" incidents context <uuid>'.")
	}
	if v := view.Vulnerabilities; v != nil {
		add("vulnerabilities", v.UnattributedRows, "vulnerability row(s)", v.UnattributedDeviceUUIDs, esetUnattributedResync)
	}
	if p := view.Patches; p != nil {
		add("patches", p.UnattributedRows, "patch row(s)", p.UnattributedDeviceUUIDs, esetUnattributedResync)
	}
	if sc := view.ScanCoverage; sc != nil {
		add("scan_coverage", sc.UnattributedScans, "scan(s) in the window", sc.UnattributedDeviceUUIDs, esetUnattributedResync)
	}
	return out
}

// reportCustomerUnattributedSuffix is the inline count on a section line, so
// a zero total never reads as an all-clear while rows could not be placed.
func reportCustomerUnattributedSuffix(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf(" (+%d unattributed, see note)", n)
}

// reportCustomerSectionLabels names each section in the human report.
var reportCustomerSectionLabels = map[string]string{
	"health": "Health", "detections": "Detections", "incidents": "Incidents",
	"vulnerabilities": "Vulnerabilities", "patches": "Patches", "scan_coverage": "Scan coverage",
}

// reportCustomerLimits lists, per section, what the human report must say
// next to the numbers so a capped or partial report never reads as complete.
func reportCustomerLimits(view reportCustomerView) map[string][]string {
	out := map[string][]string{}
	if in := view.Incidents; in != nil && in.HydrationSkipped > 0 {
		out["incidents"] = append(out["incidents"], fmt.Sprintf("%d incident(s) with no devices on their list row were not fetched (cap %d), so they are not attributed to this customer", in.HydrationSkipped, reportCustomerHydrationCap))
	}
	if v := view.Vulnerabilities; v != nil && v.Truncated {
		out["vulnerabilities"] = append(out["vulnerabilities"], "the scan stopped at --max-scan-pages; totals may be low")
	}
	if p := view.Patches; p != nil && p.Truncated {
		out["patches"] = append(out["patches"], "a live list stopped at --max-scan-pages; totals may be low")
	}
	if s := view.ScanCoverage; s != nil {
		if s.Truncated {
			out["scan_coverage"] = append(out["scan_coverage"], "the scan list stopped at --max-scan-pages; coverage may be low")
		}
		if s.Note != "" {
			out["scan_coverage"] = append(out["scan_coverage"], s.Note)
		}
	}
	for _, f := range view.FetchFailures {
		out[f.Section] = append(out[f.Section], "partial: "+truncate(f.Error, 160))
	}
	for sec, sync := range reportCustomerSectionSyncs(view) {
		if sync.Incomplete && sync.SyncedHint != "" {
			out[sec] = append(out[sec], "incomplete: "+sync.SyncedHint)
		}
	}
	for sec, line := range reportCustomerUnattributedNotes(view) {
		out[sec] = append(out[sec], "incomplete: "+line)
	}
	return out
}

// reportCustomerSectionSyncs returns the sync marker of every present section.
func reportCustomerSectionSyncs(view reportCustomerView) map[string]reportCustomerSectionSync {
	out := map[string]reportCustomerSectionSync{}
	if view.Health != nil {
		out["health"] = view.Health.reportCustomerSectionSync
	}
	if view.Detections != nil {
		out["detections"] = view.Detections.reportCustomerSectionSync
	}
	if view.Incidents != nil {
		out["incidents"] = view.Incidents.reportCustomerSectionSync
	}
	if view.Vulnerabilities != nil {
		out["vulnerabilities"] = view.Vulnerabilities.reportCustomerSectionSync
	}
	if view.Patches != nil {
		out["patches"] = view.Patches.reportCustomerSectionSync
	}
	if view.ScanCoverage != nil {
		out["scan_coverage"] = view.ScanCoverage.reportCustomerSectionSync
	}
	return out
}

func reportCustomerRender(w io.Writer, flags *rootFlags, view reportCustomerView) error {
	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, view, flags)
	}
	title := view.Customer.Name
	if view.Customer.GroupPath != "" {
		title = view.Customer.GroupPath
	}
	esetFprintf(w, "%s\nwindow %s to %s\n\n", title, view.Window.Since, view.Window.Until)
	if view.Note != "" {
		esetFprintln(w, view.Note)
		return nil
	}
	limits := reportCustomerLimits(view)
	present := map[string]bool{
		"health": view.Health != nil, "detections": view.Detections != nil, "incidents": view.Incidents != nil,
		"vulnerabilities": view.Vulnerabilities != nil, "patches": view.Patches != nil, "scan_coverage": view.ScanCoverage != nil,
	}
	for _, sec := range reportCustomerSections {
		if !present[sec] {
			// A null section prints where it belongs, with why it is missing.
			for _, f := range view.FetchFailures {
				if f.Section == sec {
					esetFprintf(w, "%s: unavailable: %s\n", reportCustomerSectionLabels[sec], truncate(f.Error, 160))
				}
			}
			continue
		}
		switch sec {
		case "health":
			h := view.Health
			esetFprintf(w, "Health: %d devices%s, %d OK, %d attention recommended, %d attention required, %d stale (>%dd), %d muted\n",
				h.Devices, reportCustomerUnattributedSuffix(h.UnattributedDevices), h.OK, h.AttentionRecommended, h.AttentionRequired, h.Stale, h.StaleDays, h.Muted)
		case "detections":
			d := view.Detections
			esetFprintf(w, "Detections: %d total%s, %d unhandled; by severity %s; top categories %s\n",
				d.Total, reportCustomerUnattributedSuffix(d.UnattributedDetections), d.Unhandled, reportCustomerMapString(d.BySeverity), reportCustomerCountsString(d.ByCategory))
		case "incidents":
			in := view.Incidents
			esetFprintf(w, "Incidents: %d opened, %d closed (updated in window), %d open now%s; mean triage %s, mean response %s\n",
				in.Opened, in.Closed, in.OpenNow, reportCustomerUnattributedSuffix(in.UnattributedIncidents), reportCustomerSeconds(in.MeanTriageSeconds), reportCustomerSeconds(in.MeanResponseSeconds))
		case "vulnerabilities":
			v := view.Vulnerabilities
			esetFprintf(w, "Vulnerabilities: %d on %d device(s)%s, %d distinct CVEs; by severity %s\n",
				v.Total, v.VulnerableDevices, reportCustomerUnattributedSuffix(v.UnattributedRows), v.DistinctCVEs, reportCustomerMapString(v.BySeverity))
			for _, c := range v.TopCVEs {
				esetFprintf(w, "  %s risk %d %s on %d device(s) (%s)\n", c.CVE, c.RiskScore, c.Severity, c.Devices, c.Software)
			}
		case "patches":
			p := view.Patches
			esetFprintf(w, "Patches: %d pending security patch(es) on %d device(s); %d failed attempt(s) on %d device(s)%s (patch source: %s)\n",
				p.PendingSecurityPatches, p.DevicesWithPendingSecurity, p.FailedAttempts, p.FailedDevices, reportCustomerUnattributedSuffix(p.UnattributedRows), p.PatchSource)
		case "scan_coverage":
			s := view.ScanCoverage
			esetFprintf(w, "Scan coverage: %d of %d device(s) (%.1f%%), %d failed scan(s)%s\n", s.DevicesScanned, s.Devices, s.CoveragePct, s.FailedScans, reportCustomerUnattributedSuffix(s.UnattributedScans))
		}
		for _, l := range limits[sec] {
			esetFprintf(w, "  note: %s\n", l)
		}
	}
	if view.RejectedRows > 0 {
		esetFprintf(w, "\n%d row(s) could not be decoded and were left out of the totals\n", view.RejectedRows)
	}
	if view.OtherCustomerRows > 0 {
		esetFprintf(w, "%d live row(s) belong to a customer nested under %s and were left out\n", view.OtherCustomerRows, view.Customer.Name)
	}
	if view.UnattributedRows > 0 {
		esetFprintf(w, "%d row(s) could not be tied to any customer and are not in these totals; see the incomplete notes above\n", view.UnattributedRows)
	}
	if view.SyncedHint != "" {
		esetFprintf(w, "\nsynced data: %s\n", view.SyncedHint)
	}
	return nil
}

func reportCustomerMapString(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func reportCustomerCountsString(c []reportCustomerCount) string {
	if len(c) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(c))
	for _, x := range c {
		parts = append(parts, fmt.Sprintf("%s %d", x.Name, x.Count))
	}
	return strings.Join(parts, ", ")
}

func reportCustomerSeconds(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return (time.Duration(*v * float64(time.Second))).Round(time.Second).String()
}
