// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `exposure`: which customers and devices carry a given CVE or
// vulnerable application. ESET Connect has no CVE filter on any
// vulnerability endpoint, and device vulnerabilities have no identifier the
// local mirror can key on, so this command scans the live
// /v1/device-vulnerabilities list (scoped to one customer's COMPANY group when
// --customer is given; ESET applies the group recursively) and matches
// client-side. The synced device-group tree attributes each hit to its
// nearest COMPANY ancestor and the synced devices supply device names. With
// no synced tree the scan still runs and every hit lands in one
// "(unsynced device tree)" bucket, so a fresh install never reads as clean.
//
// The vulnerability types and the live-fetch seam defined here are shared with
// `patches backlog` and `report customer`.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

const (
	exposureDefaultMaxScanPages = 20
	exposureDeviceCap           = 50
	exposureSeverityPrefix      = "SEVERITY_LEVEL_"
	exposureScopeApplication    = "VULNERABILITY_SCOPE_APPLICATION"
	// exposureUnsyncedBucket holds every row when no device-group tree is
	// synced: rows are listed by device UUID and no customer is guessed.
	exposureUnsyncedBucket = "(unsynced device tree)"
	exposureSyncResources  = "devices,device-groups"
)

// exposureLiveAPI is the seam between the novel commands and ESET Connect.
// Tests replace the package-level constructors below with canned responses.
type exposureLiveAPI struct {
	list func(ctx context.Context, url string, params map[string]string, itemsKey string, maxPages int) (esetPageResult, error)
	get  func(ctx context.Context, url string) (json.RawMessage, error)
}

// exposureClientAPI binds the live seam to the real authenticated client.
func exposureClientAPI(flags *rootFlags) (exposureLiveAPI, error) {
	c, err := flags.newClient()
	if err != nil {
		return exposureLiveAPI{}, err
	}
	return exposureLiveAPI{
		list: func(ctx context.Context, url string, params map[string]string, itemsKey string, maxPages int) (esetPageResult, error) {
			return esetFetchAll(ctx, c, url, params, itemsKey, maxPages)
		},
		get: func(ctx context.Context, url string) (json.RawMessage, error) {
			return c.GetNoCache(ctx, url, nil)
		},
	}, nil
}

// Live-fetch constructors, one per command so tests can stub them independently.
var (
	exposureNewLive       = exposureClientAPI
	patchesBacklogNewLive = exposureClientAPI
	reportCustomerNewLive = exposureClientAPI
)

// exposureVulnsURL is the live device-vulnerabilities list.
func exposureVulnsURL() string {
	return esetHost("vulnerability-management", "/v1/device-vulnerabilities")
}

// exposureMaxPages applies the scan cap, clamped to 1 under the dogfood matrix.
func exposureMaxPages(n int) int {
	if cliutil.IsDogfoodEnv() || n < 1 {
		return 1
	}
	return n
}

// exposureVulnCore holds the fields shared by application, operating system
// and package vulnerabilities.
type exposureVulnCore struct {
	CVENumber       string  `json:"cveNumber"`
	RiskScore       flexInt `json:"riskScore"`
	Severity        string  `json:"severity"`
	FirstDetectTime string  `json:"firstDetectTime"`
	LastDetectTime  string  `json:"lastDetectTime"`
	PatchAvailable  bool    `json:"patchAvailable"`
	VulnerabilityID flexInt `json:"vulnerabilityId"`
}

type exposureApplication struct {
	DisplayName          string      `json:"displayName"`
	DeveloperDisplayName string      `json:"developerDisplayName"`
	Version              esetVersion `json:"version"`
	UUID                 string      `json:"uuid"`
}

type exposurePackage struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	VersionName string `json:"versionName"`
}

type exposureAppVuln struct {
	exposureVulnCore
	Application exposureApplication `json:"application"`
}

type exposureOSVuln struct {
	exposureVulnCore
	OSFamilyID flexInt `json:"osFamilyId"`
}

type exposurePackageVuln struct {
	exposureVulnCore
	Package exposurePackage `json:"package"`
}

// exposureVuln is one v1DeviceVulnerability: exactly one of App, OS or
// Package is set.
type exposureVuln struct {
	DeviceUUID          string               `json:"deviceUuid"`
	DeviceGroupUUID     string               `json:"deviceGroupUuid"`
	AvailablePatchUUIDs []string             `json:"availablePatchUuids"`
	App                 *exposureAppVuln     `json:"applicationVulnerability"`
	OS                  *exposureOSVuln      `json:"osVulnerability"`
	Package             *exposurePackageVuln `json:"packageVulnerability"`
}

func (v exposureVuln) kind() string {
	switch {
	case v.App != nil:
		return "application"
	case v.OS != nil:
		return "os"
	case v.Package != nil:
		return "package"
	}
	return ""
}

func (v exposureVuln) core() exposureVulnCore {
	switch {
	case v.App != nil:
		return v.App.exposureVulnCore
	case v.OS != nil:
		return v.OS.exposureVulnCore
	case v.Package != nil:
		return v.Package.exposureVulnCore
	}
	return exposureVulnCore{}
}

// name is the vulnerable software's human name.
func (v exposureVuln) name() string {
	switch {
	case v.App != nil:
		if v.App.Application.DisplayName != "" {
			return v.App.Application.DisplayName
		}
		return v.App.Application.DeveloperDisplayName
	case v.OS != nil:
		return "Operating system"
	case v.Package != nil:
		if v.Package.Package.DisplayName != "" {
			return v.Package.Package.DisplayName
		}
		return v.Package.Package.Name
	}
	return ""
}

// exposureVulnVariantErr enforces the v1DeviceVulnerability oneof: exactly
// one of applicationVulnerability, osVulnerability and packageVulnerability
// is a (non-null) object. A row with none or several names no single piece of
// vulnerable software, so the caller rejects it instead of counting it.
func exposureVulnVariantErr(app, os, pkg bool) error {
	n := 0
	for _, set := range []bool{app, os, pkg} {
		if set {
			n++
		}
	}
	switch n {
	case 1:
		return nil
	case 0:
		return errors.New("no applicationVulnerability, osVulnerability or packageVulnerability object")
	}
	return fmt.Errorf("%d of applicationVulnerability, osVulnerability and packageVulnerability are set; exactly one is allowed", n)
}

// exposureDecodeVulns decodes live items. A row that does not decode (a
// wrongly typed cveNumber, a riskScore that is not an integer), is not a JSON
// object (null decodes "successfully" into an empty row), or does not carry
// exactly one vulnerability variant is counted in rej rather than silently
// dropped or read as a non-match, so a shrunken total is always reported.
func exposureDecodeVulns(items []json.RawMessage, rej *esetRejects) []exposureVuln {
	out := make([]exposureVuln, 0, len(items))
	for i, raw := range items {
		what := fmt.Sprintf("vulnerability row %d", i)
		if !esetIsObject(raw) {
			rej.add(what, errors.New("not a JSON object"))
			continue
		}
		var v exposureVuln
		if err := json.Unmarshal(raw, &v); err != nil {
			rej.add(what, err)
			continue
		}
		if err := exposureVulnVariantErr(v.App != nil, v.OS != nil, v.Package != nil); err != nil {
			rej.add(what, err)
			continue
		}
		out = append(out, v)
	}
	return out
}

// exposureVulnKey is the attribution key of a vulnerability row.
func exposureVulnKey(v exposureVuln) (string, string) {
	return v.DeviceGroupUUID, v.DeviceUUID
}

// exposureSeverityRank orders ESET severity enums; unknown values rank 0.
func exposureSeverityRank(s string) int {
	switch strings.TrimPrefix(strings.ToUpper(s), exposureSeverityPrefix) {
	case "DIAGNOSTIC":
		return 1
	case "INFORMATIONAL":
		return 2
	case "LOW":
		return 3
	case "MEDIUM":
		return 4
	case "HIGH":
		return 5
	}
	return 0
}

// exposureSeverityShort renders SEVERITY_LEVEL_HIGH as HIGH ("" stays "").
func exposureSeverityShort(s string) string {
	return shortEnum(s, exposureSeverityPrefix)
}

// exposureParseTime parses an RFC3339 timestamp; ok is false when absent.
func exposureParseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// exposureScope attributes a group or device to its customer. unsynced means
// there is no synced device-group tree at all: every row lands in
// exposureUnsyncedBucket.
type exposureScope struct {
	tree     *esetTree
	devices  map[string]esetDevice
	unsynced bool
}

func newExposureScope(groups []esetGroup, devices []esetDevice) exposureScope {
	m := make(map[string]esetDevice, len(devices))
	for _, d := range devices {
		m[d.UUID] = d
	}
	return exposureScope{tree: newEsetTree(groups), devices: m}
}

// customerFor resolves the owning customer: the nearest COMPANY ancestor of
// the row's group, by the one ancestry walk (esetTree.resolve). ESET's
// deviceGroupUuid filter is recursive, so a query scoped to COMPANY A also
// returns rows of a COMPANY B nested under A; those belong to B. A row with
// no group of its own (patching details, scans) uses its device's synced
// parent group. A group the synced tree does not hold at all (created after
// the last sync) falls back to the device's synced parent group. A group the
// tree holds is never second-guessed: when its walk meets a missing ancestor
// or a cycle the row is "(group not in synced tree)" with no UUID, never the
// customer the query was scoped to: recursive membership does not prove
// nearest-COMPANY ownership.
func (s exposureScope) customerFor(groupUUID, deviceUUID string) esetGroup {
	c, _ := s.owner(groupUUID, deviceUUID)
	return c
}

// exposureFilterRows keeps the rows whose nearest COMPANY is targetUUID. It
// counts the rows of another known customer (a customer nested under the
// target that the recursive group scope also returned, or a known group
// outside every customer) in other, and returns the rows whose owner is
// unknown (neither their group nor their device's group is in the synced
// tree) apart: they may be the target's, so callers count them as
// unattributed (handfixes.json: customer-views-count-unattributed-rows). An
// empty target, or a scope with no synced tree to attribute against, keeps
// every row.
func exposureFilterRows[T any](rows []T, keyOf func(T) (string, string), scope exposureScope, targetUUID string) (kept []T, other int, unknown []T) {
	if targetUUID == "" || scope.unsynced {
		return rows, 0, nil
	}
	kept = make([]T, 0, len(rows))
	for _, r := range rows {
		g, d := keyOf(r)
		owner, known := scope.owner(g, d)
		switch {
		case !known:
			unknown = append(unknown, r)
		case owner.UUID == targetUUID:
			kept = append(kept, r)
		default:
			other++
		}
	}
	return kept, other, unknown
}

// exposureOpenScope opens the local mirror and builds the attribution scope.
// db is nil when the mirror file is missing; the scope is then unsynced, as
// it is when the mirror holds no device groups. hint is the view's
// synced_hint ("" when the tree and device names are both in hand and both
// completed a sync). Only device-groups and devices are judged here: a
// caller that also reads another resource (patches backlog's device-patches)
// judges that one when it actually uses the rows.
func exposureOpenScope(cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (*store.Store, exposureScope, string, error) {
	sync := "run '" + esetCLIName + " sync --resources " + exposureSyncResources + "'"
	db, err := openEsetStore(cmd, dbPath, resources...)
	if err != nil {
		return nil, exposureScope{}, "", err
	}
	if db == nil {
		scope := newExposureScope(nil, nil)
		scope.unsynced = true
		return nil, scope, "no local mirror, so rows cannot be attributed to customers; " + sync, nil
	}
	attribution := []string{"device-groups", "devices"}
	esetSyncHints(cmd, db, flags.maxAge, attribution...)
	incomplete := esetMirrorHint(db, attribution...)
	// On a load failure the read error is what the caller needs; a close
	// error on the read-only handle would only mask it.
	groups, err := loadEsetGroups(db)
	if err != nil {
		_ = db.Close()
		return nil, exposureScope{}, "", fmt.Errorf("reading synced device groups: %w", err)
	}
	devices, err := loadEsetDevices(db)
	if err != nil {
		_ = db.Close()
		return nil, exposureScope{}, "", fmt.Errorf("reading synced devices: %w", err)
	}
	scope := newExposureScope(groups, devices)
	empty := ""
	switch {
	case len(groups) == 0:
		scope.unsynced = true
		empty = "no device groups in the local mirror, so rows cannot be attributed to customers; " + sync
	case len(devices) == 0:
		empty = "no devices in the local mirror, so device names are blank; " + sync
	}
	return db, scope, esetJoinHints(incomplete, empty), nil
}

// exposureIsUUID reports whether s is a canonical 8-4-4-4-12 hex UUID.
func exposureIsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}

// exposureResolveTarget resolves --customer. With a synced tree it takes a
// COMPANY name or UUID. Without one a name cannot be resolved, so only a
// device-group UUID is accepted; it scopes the live call (recursively) and
// is echoed back as both name and UUID.
func exposureResolveTarget(scope exposureScope, q string) (esetGroup, error) {
	q = strings.TrimSpace(q)
	if scope.unsynced {
		if exposureIsUUID(q) {
			return esetGroup{UUID: q, Name: q}, nil
		}
		return esetGroup{}, usageErr(fmt.Errorf("--customer %q is a name, and names need the synced device-group tree; run '%s sync --resources %s' or pass the customer's device-group UUID", q, esetCLIName, exposureSyncResources))
	}
	g, err := scope.tree.resolveCustomer(q)
	if err != nil {
		return esetGroup{}, exposureCustomerErr(err)
	}
	return g, nil
}

// exposureUnsyncedNote explains the unsynced bucket; groupUUID is the
// --customer group the live calls were scoped to, if any.
func exposureUnsyncedNote(groupUUID string) string {
	n := "customer attribution needs '" + esetCLIName + " sync --resources " + exposureSyncResources + "'; every row is listed under " + exposureUnsyncedBucket + " by device UUID"
	if groupUUID != "" {
		n += "; the scan covered group " + groupUUID + " and every group under it, which can include nested customers"
	}
	return n
}

// exposureOtherNote explains rows of another known customer left out of a
// --customer scope: a customer nested under it, or a known group outside
// every customer. Owner-unknown rows are counted apart (unattributed_rows).
func exposureOtherNote(other int, customer string) string {
	if other == 0 {
		return ""
	}
	return fmt.Sprintf("%d row(s) belong to another customer (one nested under %s, which ESET's group filter includes) and were left out; query that customer to see its rows", other, customer)
}

// exposureJoinNotes joins the non-empty notes.
func exposureJoinNotes(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "; ")
}

func (s exposureScope) deviceName(uuid string) string {
	if d, ok := s.devices[uuid]; ok {
		return d.name()
	}
	return ""
}

// exposureCustomerKey identifies a customer row; buckets have no UUID.
func exposureCustomerKey(g esetGroup) string {
	if g.UUID != "" {
		return g.UUID
	}
	return g.Name
}

type exposureQuery struct {
	CVEs     []string `json:"cve,omitempty"`
	App      string   `json:"app,omitempty"`
	Version  string   `json:"version,omitempty"`
	Customer string   `json:"customer,omitempty"`
}

// exposureParseCVEs splits a comma list into upper-cased, de-duplicated IDs.
func exposureParseCVEs(s string) []string {
	out := make([]string, 0)
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		id := strings.ToUpper(strings.TrimSpace(part))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// exposureMatch reports whether a vulnerability satisfies the query: an exact
// (case-insensitive) cveNumber on any vulnerability kind, or an application
// display-name substring with an optional exact version name.
func exposureMatch(q exposureQuery, v exposureVuln) bool {
	if len(q.CVEs) > 0 {
		cve := strings.TrimSpace(v.core().CVENumber)
		if cve == "" {
			return false
		}
		for _, want := range q.CVEs {
			if strings.EqualFold(cve, want) {
				return true
			}
		}
		return false
	}
	if q.App == "" || v.App == nil {
		return false
	}
	if !strings.Contains(strings.ToLower(v.App.Application.DisplayName), strings.ToLower(strings.TrimSpace(q.App))) {
		return false
	}
	if q.Version != "" && strings.TrimSpace(v.App.Application.Version.String()) != strings.TrimSpace(q.Version) {
		return false
	}
	return true
}

type exposureDeviceRef struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type exposureCustomerRow struct {
	Customer            string              `json:"customer"`
	CustomerUUID        string              `json:"customer_uuid"`
	AffectedDevices     int                 `json:"affected_devices"`
	Devices             []exposureDeviceRef `json:"devices"`
	DevicesTruncated    bool                `json:"devices_truncated"`
	MaxRiskScore        int                 `json:"max_risk_score"`
	Severity            string              `json:"severity"`
	EarliestFirstDetect string              `json:"earliest_first_detect"`
	PatchAvailable      bool                `json:"patch_available"`
	AvailablePatchUUIDs []string            `json:"available_patch_uuids"`
	CVEs                []string            `json:"cves"`
}

// exposureTotals: Customers counts only customers the mirror proves own a
// matching row (COMPANY groups, or the one "(tenant)"; the "(unassigned)" row
// of the MSP's own devices is listed but is not a customer, handfixes.json:
// customer-counts-exclude-unassigned); the devices of rows whose owner is
// unknown (the "(group not in synced tree)" and "(unsynced device tree)"
// rows) are counted in UnknownOwnerDevices instead, since each may be another
// customer (handfixes.json: spread-counts-uncertainty-before-thresholds).
type exposureTotals struct {
	AffectedDevices     int `json:"affected_devices"`
	Customers           int `json:"customers"`
	UnknownOwnerDevices int `json:"unknown_owner_devices"`
}

type exposureView struct {
	Query                  exposureQuery         `json:"query"`
	Customers              []exposureCustomerRow `json:"customers"`
	Totals                 exposureTotals        `json:"totals"`
	ScannedVulnerabilities int                   `json:"scanned_vulnerabilities"`
	Pages                  int                   `json:"pages"`
	Truncated              bool                  `json:"truncated"`
	RejectedRows           int                   `json:"rejected_rows"`
	OtherCustomerRows      int                   `json:"other_customer_rows"`
	// UnattributedRows counts matching rows whose owner is unknown: neither
	// their device group nor their device's synced group is in the mirror.
	// Under --customer they are left out of the customer's rows (they may be
	// its); without it they are the "(group not in synced tree)" row
	// (handfixes.json: customer-views-count-unattributed-rows).
	UnattributedRows        int      `json:"unattributed_rows"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	SyncedHint              string   `json:"synced_hint,omitempty"`
	Note                    string   `json:"note,omitempty"`
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

// exposureCountUnattributed counts the matching rows whose owner is unknown.
func exposureCountUnattributed(q exposureQuery, vulns []exposureVuln, scope exposureScope) esetUnattributed {
	var u esetUnattributed
	for _, v := range vulns {
		if !exposureMatch(q, v) {
			continue
		}
		if _, known := scope.owner(v.DeviceGroupUUID, v.DeviceUUID); !known {
			u.add(v.DeviceUUID)
		}
	}
	return u
}

// exposureAssemble fills the attribution half of view from the scanned
// rows: unattributed_rows (counted before --customer drops anything), the
// rows of other customers, the per-customer rows and the totals.
// totals.unknown_owner_devices counts the distinct devices of every matching
// row whose owner is unknown, also before --customer, so it always agrees
// with unattributed_device_uuids (adversarial round 4: under --customer the
// totals were built from the kept rows only and read 0 beside
// unattributed_rows 1). With no synced tree at all every device is
// owner-unknown, as exposureBuild already counts.
func exposureAssemble(view *exposureView, q exposureQuery, vulns []exposureVuln, scope exposureScope, target string) {
	un := exposureCountUnattributed(q, vulns, scope)
	view.UnattributedRows, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	view.parentNotVisible = scope.tree.parentNotVisible()
	view.ParentNotVisible = scope.tree.parentNotVisibleGroups()
	kept, other, _ := exposureFilterRows(vulns, exposureVulnKey, scope, target)
	view.OtherCustomerRows = other
	view.Customers, view.Totals = exposureBuild(q, kept, scope, exposureDeviceCap)
	if !scope.unsynced {
		view.Totals.UnknownOwnerDevices = len(view.UnattributedDeviceUUIDs)
	}
}

// exposureBuild rolls matching vulnerabilities up per customer, most affected
// first. Devices per customer are capped at deviceCap (0 = no cap).
func exposureBuild(q exposureQuery, vulns []exposureVuln, scope exposureScope, deviceCap int) ([]exposureCustomerRow, exposureTotals) {
	type acc struct {
		row       exposureCustomerRow
		known     bool
		devices   map[string]bool
		patches   map[string]bool
		cves      map[string]bool
		earliest  time.Time
		sevRank   int
		hasSevRow bool
	}
	byCustomer := map[string]*acc{}
	allDevices := map[string]bool{}
	for _, v := range vulns {
		if !exposureMatch(q, v) {
			continue
		}
		cust, known := scope.owner(v.DeviceGroupUUID, v.DeviceUUID)
		if scope.unsynced {
			known = false
		}
		key := exposureCustomerKey(cust)
		a := byCustomer[key]
		if a == nil {
			a = &acc{
				row:     exposureCustomerRow{Customer: cust.Name, CustomerUUID: cust.UUID},
				known:   known,
				devices: map[string]bool{}, patches: map[string]bool{}, cves: map[string]bool{},
			}
			byCustomer[key] = a
		}
		core := v.core()
		if v.DeviceUUID != "" {
			a.devices[v.DeviceUUID] = true
			allDevices[v.DeviceUUID] = true
		}
		if int(core.RiskScore) > a.row.MaxRiskScore {
			a.row.MaxRiskScore = int(core.RiskScore)
		}
		if r := exposureSeverityRank(core.Severity); !a.hasSevRow || r > a.sevRank {
			a.sevRank, a.hasSevRow = r, true
			a.row.Severity = exposureSeverityShort(core.Severity)
		}
		if t, ok := exposureParseTime(core.FirstDetectTime); ok && (a.earliest.IsZero() || t.Before(a.earliest)) {
			a.earliest = t
			a.row.EarliestFirstDetect = core.FirstDetectTime
		}
		if core.PatchAvailable || len(v.AvailablePatchUUIDs) > 0 {
			a.row.PatchAvailable = true
		}
		for _, p := range v.AvailablePatchUUIDs {
			if p != "" {
				a.patches[p] = true
			}
		}
		if cve := strings.ToUpper(strings.TrimSpace(core.CVENumber)); cve != "" {
			a.cves[cve] = true
		}
	}
	rows := make([]exposureCustomerRow, 0, len(byCustomer))
	totals := exposureTotals{AffectedDevices: len(allDevices)}
	unknownDevices := map[string]bool{}
	for _, a := range byCustomer {
		if a.known {
			if esetCountsAsCustomer(esetGroup{UUID: a.row.CustomerUUID, Name: a.row.Customer}) {
				totals.Customers++
			}
		} else {
			for id := range a.devices {
				unknownDevices[id] = true
			}
		}
		row := a.row
		row.AffectedDevices = len(a.devices)
		row.Devices = make([]exposureDeviceRef, 0, len(a.devices))
		for id := range a.devices {
			row.Devices = append(row.Devices, exposureDeviceRef{UUID: id, Name: scope.deviceName(id)})
		}
		sort.Slice(row.Devices, func(i, j int) bool {
			ni, nj := strings.ToLower(row.Devices[i].Name), strings.ToLower(row.Devices[j].Name)
			if ni != nj {
				return ni < nj
			}
			return row.Devices[i].UUID < row.Devices[j].UUID
		})
		if deviceCap > 0 && len(row.Devices) > deviceCap {
			row.Devices = row.Devices[:deviceCap]
			row.DevicesTruncated = true
		}
		row.AvailablePatchUUIDs = exposureSortedKeys(a.patches)
		row.CVEs = exposureSortedKeys(a.cves)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AffectedDevices != rows[j].AffectedDevices {
			return rows[i].AffectedDevices > rows[j].AffectedDevices
		}
		if rows[i].MaxRiskScore != rows[j].MaxRiskScore {
			return rows[i].MaxRiskScore > rows[j].MaxRiskScore
		}
		return strings.ToLower(rows[i].Customer) < strings.ToLower(rows[j].Customer)
	})
	totals.UnknownOwnerDevices = len(unknownDevices)
	return rows, totals
}

func exposureSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// exposureNote explains an empty or partial scan, and rows that could not
// be checked because they did not decode.
func exposureNote(matches int, scanned, pages int, truncated bool, rejected int) string {
	scan := ""
	switch {
	case matches == 0 && truncated:
		scan = fmt.Sprintf("no match in the first %d page(s) (%d vulnerabilities scanned) and more remain; raise --max-scan-pages or scope with --customer", pages, scanned)
	case matches == 0:
		scan = fmt.Sprintf("no device vulnerability matched (%d scanned)", scanned)
	case truncated:
		scan = fmt.Sprintf("scan stopped at --max-scan-pages (%d page(s)); results may be incomplete", pages)
	}
	bad := ""
	if rejected > 0 {
		bad = fmt.Sprintf("%d vulnerability row(s) could not be decoded and were not checked", rejected)
	}
	return exposureJoinNotes(scan, bad)
}

func newNovelExposureCmd(flags *rootFlags) *cobra.Command {
	var flagCVE, flagApp, flagVersion, flagCustomer, flagDB string
	var flagMaxScanPages int

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Which customers and devices have a given CVE or vulnerable application, with risk score",
		Long: strings.Trim(`
Find which customers and devices are affected by a CVE or a vulnerable
application. ESET Connect has no CVE filter, so this scans the live device
vulnerability list (scoped to one customer's group with --customer) and matches
client-side: --cve is an exact, case-insensitive match on the CVE of any
application, operating system or package vulnerability (comma-separate several);
--app is a case-insensitive substring of the vulnerable application's name,
optionally pinned to one --version. Each hit is attributed to its nearest
customer (COMPANY) group through the synced device-group tree, and device names
come from synced devices. ESET applies --customer's group recursively, so rows
of a customer nested under it are left out and counted in other_customer_rows.
A row whose device group is not yet in the synced tree (created after the
last sync) is attributed through its device's synced group when there is one;
otherwise its owner is unknown: it is reported as "(group not in synced tree)"
(or "(parent group not visible)" when its group is synced but a parent is not
visible to the API user; see 'fleet health'), and under --customer it is left
out of the customer's rows (it may be the customer's) and counted in
unattributed_rows (devices in unattributed_device_uuids and
totals.unknown_owner_devices), never in
other_customer_rows. Re-sync device-groups to attribute them, except rows under
a parent group the API user cannot see, which no sync attributes. The MSP's own
devices are listed under "(unassigned)"; totals.customers counts customers
(COMPANY groups, or the one "(tenant)" of a single-tenant account) and never
that row.

With no synced device-group tree the scan still runs: every hit is listed under
one "(unsynced device tree)" row by device UUID, synced_hint says what to sync,
and --customer accepts only a device-group UUID.

Per customer: distinct affected devices (up to 50 listed), highest risk score,
worst severity, earliest first detection, whether a patch is available, and the
available patch UUIDs. The scan stops at --max-scan-pages pages of 1000; the
output reports scanned_vulnerabilities, truncated and rejected_rows (rows that
could not be decoded) so a partial scan is never mistaken for a clean bill of
health.

Use this command to find which customers and devices are affected by a specific
CVE or vulnerable application. Do NOT use it for a customer's overall
vulnerability rollup; use 'report vulnerabilities' instead. Do NOT use it to
plan which patches to deploy; use 'patches backlog' instead.`, "\n"),
		Example: strings.Trim(`
  eset-protect-cli exposure --cve CVE-2024-3400 --agent
  eset-protect-cli exposure --cve CVE-2024-3400,CVE-2023-4863 --customer "Acme Dental" --json
  eset-protect-cli exposure --app chrome --version 126.0.6478.127 --max-scan-pages 40`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exposure")
			}
			q := exposureQuery{CVEs: exposureParseCVEs(flagCVE), App: strings.TrimSpace(flagApp), Version: strings.TrimSpace(flagVersion)}
			if (len(q.CVEs) > 0) == (q.App != "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one of --cve <CVE-ID[,CVE-ID...]> or --app <name>"))
			}
			if q.Version != "" && q.App == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--version narrows --app; it cannot be used with --cve"))
			}
			if len(q.CVEs) == 0 {
				q.CVEs = nil
			}
			q.Customer = strings.TrimSpace(flagCustomer)
			view := exposureView{Query: q, Customers: make([]exposureCustomerRow, 0)}

			db, scope, hint, err := exposureOpenScope(cmd, flags, flagDB, "device-groups", "devices")
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
			}
			view.SyncedHint = hint
			params := map[string]string{}
			target := ""
			if q.Customer != "" {
				cust, rerr := exposureResolveTarget(scope, q.Customer)
				if rerr != nil {
					return rerr
				}
				target = cust.UUID
				params["deviceGroupUuid"] = cust.UUID
				view.Query.Customer = cust.Name
			}
			if q.App != "" {
				params["vulnerabilityScope"] = exposureScopeApplication
			}

			live, err := exposureNewLive(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			page, err := live.list(ctx, exposureVulnsURL(), params, "vulnerabilities", exposureMaxPages(flagMaxScanPages))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var rej esetRejects
			vulns := exposureDecodeVulns(page.Items, &rej)
			exposureAssemble(&view, q, vulns, scope, target)
			view.ScannedVulnerabilities = len(page.Items)
			view.Pages = page.Pages
			view.Truncated = page.Truncated
			view.RejectedRows = rej.N
			unsynced := ""
			if scope.unsynced {
				unsynced = exposureUnsyncedNote(target)
			}
			view.Note = exposureJoinNotes(unsynced,
				exposureNote(len(view.Customers), view.ScannedVulnerabilities, view.Pages, view.Truncated, rej.N),
				exposureOtherNote(view.OtherCustomerRows, view.Query.Customer))
			rej.warn(cmd)
			return exposureRender(cmd.OutOrStdout(), flags, view)
		},
	}
	cmd.Flags().StringVar(&flagCVE, "cve", "", "CVE ID to look for, exact and case-insensitive (comma-separate several, e.g. CVE-2024-3400,CVE-2023-4863)")
	cmd.Flags().StringVar(&flagApp, "app", "", "Vulnerable application name substring, case-insensitive (e.g. chrome)")
	cmd.Flags().StringVar(&flagVersion, "version", "", "With --app: only this exact application version name (e.g. 126.0.6478.127)")
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Scope the scan to one customer (COMPANY group name or UUID; only a group UUID when no device-group tree is synced)")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", exposureDefaultMaxScanPages, "Maximum pages of 1000 device vulnerabilities to scan (1 under the dogfood harness)")
	cmd.Flags().StringVar(&flagDB, "db", "", esetDBFlagUsage)
	return cmd
}

// exposureCustomerErr maps a customer-resolution failure to an exit code:
// not found (3) when nothing matched, usage (2) otherwise.
func exposureCustomerErr(err error) error {
	if strings.HasPrefix(err.Error(), "no customer matches") {
		return notFoundErr(err)
	}
	return usageErr(err)
}

func exposureRender(w io.Writer, flags *rootFlags, view exposureView) error {
	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, view, flags)
	}
	unattributed := esetUnattributedLine(view.UnattributedRows, "matching row(s)", view.UnattributedDeviceUUIDs, view.Query.Customer, esetUnattributedResync, view.parentNotVisible)
	if len(view.Customers) == 0 {
		if unattributed == "" {
			fmt.Fprintln(w, "No device matched.")
		} else {
			// Never an all-clear while a match could be this customer's.
			fmt.Fprintln(w, "No device the mirror can attribute matched.")
			esetFprintln(w, unattributed)
		}
		if view.Note != "" {
			esetFprintln(w, view.Note)
		}
		if view.SyncedHint != "" {
			esetFprintln(w, view.SyncedHint)
		}
		return nil
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, "CUSTOMER\tDEVICES\tMAX RISK\tSEVERITY\tPATCH\tFIRST SEEN\tCVES")
	for _, r := range view.Customers {
		patch := "no"
		if r.PatchAvailable {
			patch = "yes"
		}
		cves := strings.Join(r.CVEs, ",")
		esetFprintf(tw, "%s\t%d\t%d\t%s\t%s\t%s\t%s\n", r.Customer, r.AffectedDevices, r.MaxRiskScore, r.Severity, patch, r.EarliestFirstDetect, truncate(cves, 60))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	unknown := ""
	if view.Totals.UnknownOwnerDevices > 0 {
		unknown = fmt.Sprintf(" (+%d device(s) whose customer is unknown)", view.Totals.UnknownOwnerDevices)
	}
	esetFprintf(w, "\n%d device(s) across %d customer(s)%s; %d vulnerabilities scanned in %d page(s)\n", view.Totals.AffectedDevices, view.Totals.Customers, unknown, view.ScannedVulnerabilities, view.Pages)
	if unattributed != "" {
		esetFprintln(w, unattributed)
	}
	if view.Note != "" {
		esetFprintln(w, view.Note)
	}
	if view.SyncedHint != "" {
		esetFprintln(w, view.SyncedHint)
	}
	return nil
}
