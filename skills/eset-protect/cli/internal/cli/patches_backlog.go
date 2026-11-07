// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
//
// Hand-authored. `patches backlog`: available-but-not-installed patches ranked
// by the devices and CVE risk they fix, per customer, plus failed patch
// attempts. Device vulnerabilities are live-only (no identifier to mirror);
// their availablePatchUuids are joined to device-patches rows by uuid, read
// from the local mirror first and fetched live when the mirror has none
// (--data-source auto) or always live (--data-source live). Failed attempts
// come from live /v1/patching-process-details over --since. Rows are
// attributed to their nearest COMPANY group; with no synced device-group tree
// they land in one "(unsynced device tree)" bucket instead of disappearing.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	patchesBacklogCategorySecurity = "PATCH_CATEGORY_SECURITY_UPDATE"
	patchesBacklogCVEIDCap         = 5
)

// patchesBacklogPatchCore holds the fields shared by application, OS and
// package patches (v1ApplicationPatch / v1OsPatch / v1PackagePatch).
type patchesBacklogPatchCore struct {
	CatalogID            flexInt  `json:"catalogId"`
	Category             string   `json:"category"`
	DeveloperDisplayName string   `json:"developerDisplayName"`
	FixedCves            []string `json:"fixedCves"`
	RestartRequired      string   `json:"restartRequired"`
	ResultVersionName    string   `json:"resultVersionName"`
	Severity             string   `json:"severity"`
}

type patchesBacklogOSPatch struct {
	patchesBacklogPatchCore
	OSFamilyID flexInt `json:"osFamilyId"`
}

type patchesBacklogPackagePatch struct {
	patchesBacklogPatchCore
	Package exposurePackage `json:"package"`
}

// patchesBacklogDevicePatch is one v1DevicePatch: one available patch on one
// device. Exactly one of App, OS or Package is set.
type patchesBacklogDevicePatch struct {
	UUID            string                      `json:"uuid"`
	DeviceUUID      string                      `json:"deviceUuid"`
	DeviceGroupUUID string                      `json:"deviceGroupUuid"`
	App             *patchesBacklogPatchCore    `json:"applicationPatch"`
	OS              *patchesBacklogOSPatch      `json:"osPatch"`
	Package         *patchesBacklogPackagePatch `json:"packagePatch"`
}

func (p patchesBacklogDevicePatch) kind() string {
	switch {
	case p.App != nil:
		return "application"
	case p.OS != nil:
		return "os"
	case p.Package != nil:
		return "package"
	}
	return ""
}

func (p patchesBacklogDevicePatch) core() patchesBacklogPatchCore {
	switch {
	case p.App != nil:
		return *p.App
	case p.OS != nil:
		return p.OS.patchesBacklogPatchCore
	case p.Package != nil:
		return p.Package.patchesBacklogPatchCore
	}
	return patchesBacklogPatchCore{}
}

func (p patchesBacklogDevicePatch) isSecurity() bool {
	return p.core().Category == patchesBacklogCategorySecurity
}

// patchesBacklogDecodePatches decodes stored or live device-patch rows. A
// row that is not a JSON object, does not decode, or has no uuid to join on
// is counted in rej.
func patchesBacklogDecodePatches(items []json.RawMessage, rej *esetRejects) []patchesBacklogDevicePatch {
	out := make([]patchesBacklogDevicePatch, 0, len(items))
	for i, raw := range items {
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("device-patch row %d", i), errors.New("not a JSON object"))
			continue
		}
		var p patchesBacklogDevicePatch
		if err := json.Unmarshal(raw, &p); err != nil {
			rej.add(fmt.Sprintf("device-patch row %d", i), err)
			continue
		}
		if p.UUID == "" {
			rej.add(fmt.Sprintf("device-patch row %d", i), fmt.Errorf("no uuid"))
			continue
		}
		out = append(out, p)
	}
	return out
}

// patchesBacklogPatchKey is the attribution key of a device-patch row.
func patchesBacklogPatchKey(p patchesBacklogDevicePatch) (string, string) {
	return p.DeviceGroupUUID, p.DeviceUUID
}

// patchesBacklogProcessKey is the attribution key of a patching-details row,
// which carries a device but no group.
func patchesBacklogProcessKey(p patchesBacklogProcess) (string, string) {
	return "", p.DeviceUUID
}

// patchesBacklogInCustomer keeps local (account-wide) rows owned by one
// customer. A row whose group (and device) the synced tree does not know is
// "(group not in synced tree)", never the target; callers count those with
// patchesBacklogUnattributedPatches before filtering.
func patchesBacklogInCustomer(patches []patchesBacklogDevicePatch, scope exposureScope, customerUUID string) []patchesBacklogDevicePatch {
	out := make([]patchesBacklogDevicePatch, 0, len(patches))
	for _, p := range patches {
		if scope.customerFor(p.DeviceGroupUUID, p.DeviceUUID).UUID == customerUUID {
			out = append(out, p)
		}
	}
	return out
}

// patchesBacklogUnattributedPatches counts pending device-patch rows (security
// ones only when securityOnly) whose owner is unknown (handfixes.json:
// customer-views-count-unattributed-rows).
func patchesBacklogUnattributedPatches(patches []patchesBacklogDevicePatch, scope exposureScope, securityOnly bool, u *esetUnattributed) {
	for _, p := range patches {
		if securityOnly && !p.isSecurity() {
			continue
		}
		if _, known := scope.owner(p.DeviceGroupUUID, p.DeviceUUID); !known {
			u.add(p.DeviceUUID)
		}
	}
}

// patchesBacklogUnattributedVulns counts vulnerability rows that reference an
// available patch and whose owner is unknown.
func patchesBacklogUnattributedVulns(vulns []exposureVuln, scope exposureScope, u *esetUnattributed) {
	for _, v := range vulns {
		if len(v.AvailablePatchUUIDs) == 0 {
			continue
		}
		if _, known := scope.owner(v.DeviceGroupUUID, v.DeviceUUID); !known {
			u.add(v.DeviceUUID)
		}
	}
}

// patchesBacklogUnattributedDetails counts failed patch attempts whose owner
// is unknown.
func patchesBacklogUnattributedDetails(details []patchesBacklogProcess, scope exposureScope, u *esetUnattributed) {
	for _, d := range details {
		if !patchesBacklogIsFailure(d.status()) {
			continue
		}
		if _, known := scope.owner("", d.DeviceUUID); !known {
			u.add(d.DeviceUUID)
		}
	}
}

type patchesBacklogCVEs struct {
	Count int      `json:"count"`
	IDs   []string `json:"ids"`
}

type patchesBacklogRow struct {
	Name            string             `json:"name"`
	Version         string             `json:"version"`
	Kind            string             `json:"kind"`
	Category        string             `json:"category"`
	Severity        string             `json:"severity"`
	RestartRequired string             `json:"restart_required"`
	FixedCVEs       patchesBacklogCVEs `json:"fixed_cves"`
	AffectedDevices int                `json:"affected_devices"`
	MaxRiskScore    int                `json:"max_risk_score"`
	Score           int                `json:"score"`
	CatalogID       int64              `json:"catalog_id,omitempty"`
	Resolved        bool               `json:"resolved"`
}

type patchesBacklogCustomer struct {
	Customer     string              `json:"customer"`
	CustomerUUID string              `json:"customer_uuid"`
	TotalPatches int                 `json:"total_patches"`
	Patches      []patchesBacklogRow `json:"patches"`
}

// patchesBacklogBuild groups available patches by identity per customer and
// ranks them by affected devices x the highest risk score of the
// vulnerabilities they fix. Vulnerability patch references with no matching
// device-patch row are kept as unresolved rows so a stale mirror never hides
// a fix. limit caps patches per customer (0 = no cap).
func patchesBacklogBuild(vulns []exposureVuln, patches []patchesBacklogDevicePatch, scope exposureScope, securityOnly bool, limit int) []patchesBacklogCustomer {
	type link struct {
		cve  string
		risk int
		name string
	}
	links := map[string][]link{}
	for _, v := range vulns {
		core := v.core()
		for _, pu := range v.AvailablePatchUUIDs {
			if pu == "" {
				continue
			}
			links[pu] = append(links[pu], link{cve: strings.ToUpper(strings.TrimSpace(core.CVENumber)), risk: int(core.RiskScore), name: v.name()})
		}
	}

	type acc struct {
		cust         esetGroup
		row          patchesBacklogRow
		nameFromVuln bool
		devices      map[string]bool
		cveRisk      map[string]int
		sevRank      int
		restart      map[string]bool
	}
	groups := map[string]*acc{}
	get := func(cust esetGroup, key string, seed patchesBacklogRow) *acc {
		k := exposureCustomerKey(cust) + "\x00" + key
		a := groups[k]
		if a == nil {
			a = &acc{cust: cust, row: seed, devices: map[string]bool{}, cveRisk: map[string]int{}, restart: map[string]bool{}, sevRank: -1}
			groups[k] = a
		}
		return a
	}
	addLinks := func(a *acc, ls []link) {
		for _, l := range ls {
			if l.risk > a.row.MaxRiskScore {
				a.row.MaxRiskScore = l.risk
			}
			if l.cve != "" && l.risk >= a.cveRisk[l.cve] {
				a.cveRisk[l.cve] = l.risk
			}
			// The patch catalog has no application name, so application
			// patches borrow it from a vulnerability they fix; OS and package
			// patches keep their own (more specific) names.
			if l.name != "" && !a.nameFromVuln && a.row.Kind == "application" {
				a.row.Name, a.nameFromVuln = l.name, true
			}
		}
	}

	known := map[string]bool{}
	for _, p := range patches {
		known[p.UUID] = true
		if securityOnly && !p.isSecurity() {
			continue
		}
		core := p.core()
		cust := scope.customerFor(p.DeviceGroupUUID, p.DeviceUUID)
		name := patchesBacklogPatchName(p, scope)
		if p.App != nil {
			for _, l := range links[p.UUID] {
				if l.name != "" {
					name = l.name
					break
				}
			}
		}
		ident := strconv.FormatInt(int64(core.CatalogID), 10)
		if core.CatalogID == 0 {
			ident = "name:" + strings.ToLower(name)
		}
		if p.OS != nil {
			ident += "|family:" + strconv.FormatInt(int64(p.OS.OSFamilyID), 10)
		}
		if p.Package != nil {
			ident += "|pkg:" + strings.ToLower(p.Package.Package.Name)
		}
		key := "patch|" + p.kind() + "|" + ident + "|" + core.ResultVersionName
		a := get(cust, key, patchesBacklogRow{
			Name: name, Version: core.ResultVersionName, Kind: p.kind(),
			Category: shortEnum(core.Category, "PATCH_CATEGORY_"), CatalogID: int64(core.CatalogID), Resolved: true,
		})
		if p.DeviceUUID != "" {
			a.devices[p.DeviceUUID] = true
		}
		if r := exposureSeverityRank(core.Severity); r > a.sevRank {
			a.sevRank = r
			a.row.Severity = exposureSeverityShort(core.Severity)
		}
		a.restart[shortEnum(core.RestartRequired, "RESTART_REQUIREMENT_")] = true
		for _, cve := range core.FixedCves {
			if id := strings.ToUpper(strings.TrimSpace(cve)); id != "" {
				if _, ok := a.cveRisk[id]; !ok {
					a.cveRisk[id] = 0
				}
			}
		}
		addLinks(a, links[p.UUID])
	}

	// Patch references whose device-patch row is not in hand.
	for _, v := range vulns {
		cust := scope.customerFor(v.DeviceGroupUUID, v.DeviceUUID)
		for _, pu := range v.AvailablePatchUUIDs {
			if pu == "" || known[pu] {
				continue
			}
			name := v.name()
			if d, ok := scope.devices[v.DeviceUUID]; ok && v.OS != nil && d.OperatingSystem.DisplayName != "" {
				name = d.OperatingSystem.DisplayName + " update"
			}
			key := "unresolved|" + v.kind() + "|" + strings.ToLower(name)
			a := get(cust, key, patchesBacklogRow{Name: name, Kind: v.kind(), Resolved: false})
			if v.DeviceUUID != "" {
				a.devices[v.DeviceUUID] = true
			}
			core := v.core()
			if r := exposureSeverityRank(core.Severity); r > a.sevRank {
				a.sevRank = r
				a.row.Severity = exposureSeverityShort(core.Severity)
			}
			addLinks(a, []link{{cve: strings.ToUpper(strings.TrimSpace(core.CVENumber)), risk: int(core.RiskScore), name: v.name()}})
		}
	}

	byCustomer := map[string]*patchesBacklogCustomer{}
	for _, a := range groups {
		row := a.row
		row.AffectedDevices = len(a.devices)
		row.Score = row.AffectedDevices * row.MaxRiskScore
		row.RestartRequired = patchesBacklogRestart(a.restart)
		row.FixedCVEs = patchesBacklogTopCVEs(a.cveRisk, patchesBacklogCVEIDCap)
		ck := exposureCustomerKey(a.cust)
		c := byCustomer[ck]
		if c == nil {
			c = &patchesBacklogCustomer{Customer: a.cust.Name, CustomerUUID: a.cust.UUID, Patches: make([]patchesBacklogRow, 0)}
			byCustomer[ck] = c
		}
		c.Patches = append(c.Patches, row)
	}
	out := make([]patchesBacklogCustomer, 0, len(byCustomer))
	for _, c := range byCustomer {
		sort.Slice(c.Patches, func(i, j int) bool {
			a, b := c.Patches[i], c.Patches[j]
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			if a.AffectedDevices != b.AffectedDevices {
				return a.AffectedDevices > b.AffectedDevices
			}
			if a.MaxRiskScore != b.MaxRiskScore {
				return a.MaxRiskScore > b.MaxRiskScore
			}
			if strings.ToLower(a.Name) != strings.ToLower(b.Name) {
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
			return a.Version < b.Version
		})
		c.TotalPatches = len(c.Patches)
		if limit > 0 && len(c.Patches) > limit {
			c.Patches = c.Patches[:limit]
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		si, sj := patchesBacklogTopScore(out[i]), patchesBacklogTopScore(out[j])
		if si != sj {
			return si > sj
		}
		return strings.ToLower(out[i].Customer) < strings.ToLower(out[j].Customer)
	})
	return out
}

func patchesBacklogTopScore(c patchesBacklogCustomer) int {
	if len(c.Patches) == 0 {
		return 0
	}
	return c.Patches[0].Score
}

// patchesBacklogPatchName names a patch: the patch catalog carries no
// application name, so application patches borrow it later from the
// vulnerabilities they fix; this is the fallback.
func patchesBacklogPatchName(p patchesBacklogDevicePatch, scope exposureScope) string {
	core := p.core()
	switch {
	case p.Package != nil:
		if p.Package.Package.DisplayName != "" {
			return p.Package.Package.DisplayName
		}
		if p.Package.Package.Name != "" {
			return p.Package.Package.Name
		}
	case p.OS != nil:
		if d, ok := scope.devices[p.DeviceUUID]; ok && d.OperatingSystem.DisplayName != "" {
			return d.OperatingSystem.DisplayName + " update"
		}
		return "Operating system update"
	}
	if core.DeveloperDisplayName != "" {
		return core.DeveloperDisplayName + " update"
	}
	if core.CatalogID != 0 {
		return fmt.Sprintf("catalog patch %d", int64(core.CatalogID))
	}
	return "unnamed patch"
}

// patchesBacklogRestart folds per-device restart requirements: YES wins,
// then any unknown (UNSPECIFIED), else NO ("" when unknown, e.g. unresolved rows).
func patchesBacklogRestart(seen map[string]bool) string {
	// Hand-fix profile-restart-keeps-unknown (patches backlog twin): any YES
	// wins; any unknown (UNSPECIFIED or omitted) makes the row UNSPECIFIED;
	// NO only when every affected device said NO.
	switch {
	case seen["YES"]:
		return "YES"
	case len(seen) == 0:
		return ""
	}
	for k := range seen {
		if k != "NO" {
			return "UNSPECIFIED"
		}
	}
	return "NO"
}

// patchesBacklogTopCVEs counts CVEs and lists up to max, riskiest first.
func patchesBacklogTopCVEs(risk map[string]int, max int) patchesBacklogCVEs {
	ids := make([]string, 0, len(risk))
	for id := range risk {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if risk[ids[i]] != risk[ids[j]] {
			return risk[ids[i]] > risk[ids[j]]
		}
		return ids[i] > ids[j] // newer CVE years first on ties
	})
	out := patchesBacklogCVEs{Count: len(ids), IDs: ids}
	if max > 0 && len(out.IDs) > max {
		out.IDs = out.IDs[:max]
	}
	return out
}

// patchesBacklogProcess is one v1DevicePatchingDetails row.
type patchesBacklogProcess struct {
	DeviceUUID  string `json:"deviceUuid"`
	DisplayName string `json:"displayName"`
	Application *struct {
		DisplayName string `json:"displayName"`
		Status      string `json:"status"`
	} `json:"application"`
	OperatingSystem *struct {
		Status string `json:"status"`
	} `json:"operatingSystem"`
	Package *struct {
		PackageReference string `json:"packageReference"`
		Status           string `json:"status"`
	} `json:"package"`
}

func (p patchesBacklogProcess) status() string {
	switch {
	case p.Application != nil && p.Application.Status != "":
		return p.Application.Status
	case p.OperatingSystem != nil && p.OperatingSystem.Status != "":
		return p.OperatingSystem.Status
	case p.Package != nil && p.Package.Status != "":
		return p.Package.Status
	}
	return ""
}

func (p patchesBacklogProcess) name() string {
	switch {
	case p.DisplayName != "":
		return p.DisplayName
	case p.Application != nil && p.Application.DisplayName != "":
		return p.Application.DisplayName
	case p.OperatingSystem != nil:
		return "Operating system"
	case p.Package != nil && p.Package.PackageReference != "":
		return p.Package.PackageReference
	}
	return "(unknown)"
}

// patchesBacklogStatusShort trims both status enum families
// (APPLICATION_PATCHING_STATUS_* and PATCHING_STATUS_*).
func patchesBacklogStatusShort(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "APPLICATION_"), "PATCHING_STATUS_")
}

// patchesBacklogIsFailure keeps the FAILED_* statuses; NOT_APPLICABLE means
// already patched or uninstalled and is not a failure to act on.
func patchesBacklogIsFailure(status string) bool {
	return strings.HasPrefix(patchesBacklogStatusShort(status), "FAILED_")
}

// patchesBacklogDecodeProcesses decodes live patching-details rows, counting
// the ones that are not JSON objects or do not decode in rej.
func patchesBacklogDecodeProcesses(items []json.RawMessage, rej *esetRejects) []patchesBacklogProcess {
	out := make([]patchesBacklogProcess, 0, len(items))
	for i, raw := range items {
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("patching-detail row %d", i), errors.New("not a JSON object"))
			continue
		}
		var p patchesBacklogProcess
		if err := json.Unmarshal(raw, &p); err != nil {
			rej.add(fmt.Sprintf("patching-detail row %d", i), err)
			continue
		}
		out = append(out, p)
	}
	return out
}

type patchesBacklogFailure struct {
	Status      string   `json:"status"`
	Application string   `json:"application"`
	Count       int      `json:"count"`
	Devices     int      `json:"devices"`
	Customers   []string `json:"customers"`
}

// patchesBacklogFailures groups failed patch attempts by status and
// application, most frequent first. Customers are keyed by COMPANY UUID (or
// bucket name), so two customers that share a display name stay two entries;
// such names are disambiguated with a UUID prefix, as 'detections spread' does.
func patchesBacklogFailures(details []patchesBacklogProcess, scope exposureScope) []patchesBacklogFailure {
	type acc struct {
		row       patchesBacklogFailure
		devices   map[string]bool
		customers map[string]esetGroup // customer identity -> customer
	}
	label := detectionsSpreadCustomerLabels(scope.tree)
	groups := map[string]*acc{}
	for _, d := range details {
		st := d.status()
		if !patchesBacklogIsFailure(st) {
			continue
		}
		short := patchesBacklogStatusShort(st)
		name := d.name()
		k := short + "\x00" + strings.ToLower(name)
		a := groups[k]
		if a == nil {
			a = &acc{row: patchesBacklogFailure{Status: short, Application: name}, devices: map[string]bool{}, customers: map[string]esetGroup{}}
			groups[k] = a
		}
		a.row.Count++
		if d.DeviceUUID != "" {
			a.devices[d.DeviceUUID] = true
		}
		cust := scope.customerFor("", d.DeviceUUID)
		a.customers[detectionsSpreadCustomerID(cust)] = cust
	}
	out := make([]patchesBacklogFailure, 0, len(groups))
	for _, a := range groups {
		row := a.row
		row.Devices = len(a.devices)
		row.Customers = make([]string, 0, len(a.customers))
		for _, c := range a.customers {
			row.Customers = append(row.Customers, label(c))
		}
		sort.Strings(row.Customers)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		return strings.ToLower(out[i].Application) < strings.ToLower(out[j].Application)
	})
	return out
}

// patchesBacklogFetchFailure is one live fetch that failed without failing
// the command; its data is excluded from the output.
type patchesBacklogFetchFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type patchesBacklogView struct {
	Customers              []patchesBacklogCustomer `json:"customers"`
	FailedAttempts         []patchesBacklogFailure  `json:"failed_attempts"`
	FailedSince            string                   `json:"failed_since,omitempty"`
	PatchSource            string                   `json:"patch_source"`
	ScannedVulnerabilities int                      `json:"scanned_vulnerabilities"`
	Truncated              bool                     `json:"truncated"`
	RejectedRows           int                      `json:"rejected_rows"`
	OtherCustomerRows      int                      `json:"other_customer_rows"`
	// UnattributedRows counts pending patches, patch-bearing vulnerabilities
	// and failed attempts whose owner is unknown (no synced device or group
	// ties them to a customer). Under --customer they are left out of the
	// customer's backlog; without it they sit under "(group not in synced tree)".
	UnattributedRows        int                          `json:"unattributed_rows"`
	UnattributedDeviceUUIDs []string                     `json:"unattributed_device_uuids,omitempty"`
	FetchFailures           []patchesBacklogFetchFailure `json:"fetch_failures"`
	SyncedHint              string                       `json:"synced_hint,omitempty"`
	Note                    string                       `json:"note,omitempty"`

	customer string // --customer name for the human output
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

func patchesBacklogEmptyView() patchesBacklogView {
	return patchesBacklogView{
		Customers:      make([]patchesBacklogCustomer, 0),
		FailedAttempts: make([]patchesBacklogFailure, 0),
		FetchFailures:  make([]patchesBacklogFetchFailure, 0),
	}
}

// patchesBacklogTimePeriod prefixes ESET's dotted wire names
// (timePeriod.startTime / timePeriod.endTime). Kept apart so the Printing
// Press novel-host scanner does not read a dotted literal as a hostname.
const patchesBacklogTimePeriod = "timePeriod."

// patchesBacklogWindowParams builds the timePeriod query for patching details.
func patchesBacklogWindowParams(groupUUID string, since, until time.Time) map[string]string {
	p := map[string]string{patchesBacklogTimePeriod + "endTime": until.UTC().Format(time.RFC3339)}
	if !since.IsZero() {
		p[patchesBacklogTimePeriod+"startTime"] = since.UTC().Format(time.RFC3339)
	}
	if groupUUID != "" {
		p["deviceGroupUuid"] = groupUUID
	}
	return p
}

func patchesBacklogDevicePatchesURL() string {
	return esetHost("patch-management", "/v1/device-patches")
}

func patchesBacklogProcessURL() string {
	return esetHost("patch-management", "/v1/patching-process-details")
}

// patchesBacklogDataSourceErr rejects --data-source local before any IO:
// device vulnerabilities exist only live.
func patchesBacklogDataSourceErr(dataSource string) error {
	if dataSource == "local" {
		return usageErr(fmt.Errorf("patches backlog needs live vulnerability data; use auto or live"))
	}
	return nil
}

func newNovelPatchesBacklogCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer, flagSince, flagDB string
	var flagSecurityOnly bool
	var flagMaxScanPages, flagLimit int

	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "Available-but-not-installed patches ranked by the devices and CVE risk they fix per customer",
		Long: strings.Trim(`
Rank the patches that are available but not installed, per customer, by how many
devices need them times the highest risk score of the vulnerabilities they fix.
Live device vulnerabilities (scoped to --customer's group when given) supply the
risk; their availablePatchUuids are joined to device-patches rows. With
--data-source auto (the default) those rows come from the local mirror, or are
fetched live when the mirror has none; --data-source live always fetches them
live. --data-source local is refused: vulnerabilities exist only live. Each
patch shows its name, result version, category, severity, restart requirement
and the CVEs it fixes. A vulnerability that points at a patch with no
device-patch row in hand is kept as an unresolved row rather than dropped.
--security-only keeps PATCH_CATEGORY_SECURITY_UPDATE patches (and unresolved
rows, which by definition fix a detected vulnerability).

Rows are attributed to their nearest customer (COMPANY) group. ESET applies
--customer's group recursively, so rows of a customer nested under it are left
out and counted in other_customer_rows. A row whose device group is not yet in
the synced tree (created after the last sync) is attributed through its
device's synced group when there is one; otherwise its owner is unknown: it
is reported as "(group not in synced tree)" (or "(parent group not visible)"
when its group is synced but a parent is not visible to the API user; see
'fleet health'), and under --customer it is left out of the customer's backlog
(it may be the customer's) and counted in unattributed_rows (devices in
unattributed_device_uuids), never in
other_customer_rows. Re-sync device-groups to attribute them, except rows under
a parent group the API user cannot see, which no sync attributes. With
no synced device-group tree the live scan still runs, every row lands in one
"(unsynced device tree)" customer, synced_hint says what to sync, and
--customer accepts only a device-group UUID. Rows that cannot be decoded
(including null rows and vulnerability rows without exactly one variant) are
counted in rejected_rows.

Failed patch attempts over --since (live /v1/patching-process-details, FAILED_*
statuses) are grouped by status and application. When that fetch fails the
backlog is still printed and the failure is listed under fetch_failures.

Use this command to list patches that are available but not installed, and
failed patch attempts, ranked by the CVEs they fix. Do NOT use it to look up who
is affected by one CVE; use 'exposure' instead. Do NOT use it for a customer's
vulnerability summary; use 'report vulnerabilities' instead.`, "\n"),
		Example: strings.Trim(`
  eset-protect-cli patches backlog --security-only --since 30d --agent
  eset-protect-cli patches backlog --customer "Acme Dental" --limit 10 --json
  eset-protect-cli patches backlog --since 7d --max-scan-pages 40 --data-source live`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "patches backlog")
			}
			if err := patchesBacklogDataSourceErr(flags.dataSource); err != nil {
				_ = cmd.Usage()
				return err
			}
			if flagLimit < 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be 0 (no cap) or more"))
			}
			since, err := esetSince(flagSince)
			if err != nil {
				return err
			}
			until := time.Now()
			view := patchesBacklogEmptyView()
			if !since.IsZero() {
				view.FailedSince = since.UTC().Format(time.RFC3339)
			}

			forceLive := flags.dataSource == "live"
			resources := []string{"device-groups", "devices"}
			if !forceLive {
				resources = append(resources, "device-patches")
			}
			db, scope, hint, err := exposureOpenScope(cmd, flags, flagDB, resources...)
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
			}
			view.SyncedHint = hint
			var patchRows []json.RawMessage
			if db != nil && !forceLive {
				if patchRows, err = loadEsetRaw(db, "device-patches"); err != nil {
					return fmt.Errorf("reading synced device patches: %w", err)
				}
			}
			groupUUID := ""
			var cust esetGroup
			if c := strings.TrimSpace(flagCustomer); c != "" {
				if cust, err = exposureResolveTarget(scope, c); err != nil {
					return err
				}
				groupUUID = cust.UUID
			}

			live, err := patchesBacklogNewLive(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			maxPages := exposureMaxPages(flagMaxScanPages)
			vpage, err := live.list(ctx, exposureVulnsURL(), map[string]string{"deviceGroupUuid": groupUUID}, "vulnerabilities", maxPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			var rej esetRejects
			var un esetUnattributed
			other := 0
			allVulns := exposureDecodeVulns(vpage.Items, &rej)
			patchesBacklogUnattributedVulns(allVulns, scope, &un)
			vulns, n, _ := exposureFilterRows(allVulns, exposureVulnKey, scope, groupUUID)
			other += n
			view.ScannedVulnerabilities = len(vpage.Items)
			view.Truncated = vpage.Truncated

			// Local rows are account-wide; without a synced tree they cannot be
			// narrowed to --customer, so that case reads the scoped live list.
			var patches []patchesBacklogDevicePatch
			if len(patchRows) > 0 && !(scope.unsynced && groupUUID != "") {
				view.PatchSource = "local"
				// The mirror's patch rows are the answer now, so an unfinished
				// device-patches sync makes it partial.
				if h := esetMirrorHint(db, "device-patches"); h != "" {
					view.SyncedHint = esetJoinHints(view.SyncedHint, h)
					esetFprintf(cmd.ErrOrStderr(), "hint: %s\n", h)
				}
				patches = patchesBacklogDecodePatches(patchRows, &rej)
				patchesBacklogUnattributedPatches(patches, scope, flagSecurityOnly, &un)
				if groupUUID != "" {
					patches = patchesBacklogInCustomer(patches, scope, groupUUID)
				}
			} else {
				view.PatchSource = "live"
				ppage, perr := live.list(ctx, patchesBacklogDevicePatchesURL(), map[string]string{"deviceGroupUuid": groupUUID}, "devices", maxPages)
				if perr != nil {
					view.FetchFailures = append(view.FetchFailures, patchesBacklogFetchFailure{Source: "device-patches", Error: perr.Error()})
				} else {
					livePatches := patchesBacklogDecodePatches(ppage.Items, &rej)
					patchesBacklogUnattributedPatches(livePatches, scope, flagSecurityOnly, &un)
					patches, n, _ = exposureFilterRows(livePatches, patchesBacklogPatchKey, scope, groupUUID)
					other += n
					view.Truncated = view.Truncated || ppage.Truncated
				}
			}
			view.Customers = patchesBacklogBuild(vulns, patches, scope, flagSecurityOnly, flagLimit)
			view.parentNotVisible = scope.tree.parentNotVisible()
			view.ParentNotVisible = scope.tree.parentNotVisibleGroups()

			fpage, ferr := live.list(ctx, patchesBacklogProcessURL(), patchesBacklogWindowParams(groupUUID, since, until), "patchingDetails", maxPages)
			if ferr != nil {
				view.FetchFailures = append(view.FetchFailures, patchesBacklogFetchFailure{Source: "patching-process-details", Error: ferr.Error()})
			} else {
				allDetails := patchesBacklogDecodeProcesses(fpage.Items, &rej)
				patchesBacklogUnattributedDetails(allDetails, scope, &un)
				details, n, _ := exposureFilterRows(allDetails, patchesBacklogProcessKey, scope, groupUUID)
				other += n
				view.FailedAttempts = patchesBacklogFailures(details, scope)
				view.Truncated = view.Truncated || fpage.Truncated
			}
			view.RejectedRows = rej.N
			view.OtherCustomerRows = other
			view.UnattributedRows, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()
			if n := len(view.FetchFailures); n > 0 {
				srcs := make([]string, 0, n)
				for _, f := range view.FetchFailures {
					srcs = append(srcs, f.Source)
				}
				esetFprintf(cmd.ErrOrStderr(), "warning: %d of %d live fetches failed (%s); their data is excluded\n", n, patchesBacklogLiveFetchCount(view.PatchSource), strings.Join(srcs, ", "))
			}
			unsynced := ""
			if scope.unsynced {
				unsynced = exposureUnsyncedNote(groupUUID)
			}
			view.Note = exposureJoinNotes(unsynced, patchesBacklogNote(view), exposureOtherNote(other, cust.Name))
			view.customer = cust.Name
			rej.warn(cmd)
			// meta.source is "live" only when no row came from the mirror
			// (handfixes.json: agent-meta-source-matches-results).
			if view.PatchSource == "local" {
				flags.agentSource = "local"
			}
			return patchesBacklogRender(cmd.OutOrStdout(), flags, view)
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Only this customer (COMPANY group name or UUID; only a group UUID when no device-group tree is synced)")
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Window for failed patch attempts (e.g. 7d, 30d, 2w)")
	cmd.Flags().BoolVar(&flagSecurityOnly, "security-only", false, "Only PATCH_CATEGORY_SECURITY_UPDATE patches")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", exposureDefaultMaxScanPages, "Maximum pages of 1000 rows per live list (1 under the dogfood harness)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum patches per customer (0 = no cap)")
	cmd.Flags().StringVar(&flagDB, "db", "", esetDBFlagUsage)
	return cmd
}

// patchesBacklogLiveFetchCount is how many live lists a run makes.
func patchesBacklogLiveFetchCount(patchSource string) int {
	if patchSource == "live" {
		return 3
	}
	return 2
}

func patchesBacklogNote(view patchesBacklogView) string {
	empty := len(view.Customers) == 0
	scan := ""
	switch {
	case empty && view.Truncated:
		scan = fmt.Sprintf("no available patch found in the scanned pages (%d vulnerabilities scanned) and more remain; raise --max-scan-pages or scope with --customer", view.ScannedVulnerabilities)
	case empty:
		scan = "no available patches found"
	case view.Truncated:
		scan = "a live list stopped at --max-scan-pages; the backlog may be incomplete"
	}
	bad := ""
	if view.RejectedRows > 0 {
		bad = fmt.Sprintf("%d row(s) could not be decoded and were left out", view.RejectedRows)
	}
	return exposureJoinNotes(scan, bad)
}

func patchesBacklogRender(w io.Writer, flags *rootFlags, view patchesBacklogView) error {
	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, view, flags)
	}
	unattributed := esetUnattributedLine(view.UnattributedRows, "patch row(s)", view.UnattributedDeviceUUIDs, view.customer, esetUnattributedResync, view.parentNotVisible)
	if len(view.Customers) == 0 {
		if unattributed == "" {
			fmt.Fprintln(w, "No available patches found.")
		} else {
			// Never an all-clear while a pending patch could be this customer's.
			fmt.Fprintln(w, "No available patches found on devices the mirror can attribute.")
		}
	}
	for _, c := range view.Customers {
		esetFprintf(w, "%s (%d patch(es))\n", c.Customer, c.TotalPatches)
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "  PATCH\tVERSION\tCATEGORY\tDEVICES\tMAX RISK\tCVES\tRESTART")
		for _, p := range c.Patches {
			name := p.Name
			if !p.Resolved {
				name += " (unresolved)"
			}
			cves := fmt.Sprintf("%d", p.FixedCVEs.Count)
			if len(p.FixedCVEs.IDs) > 0 {
				cves += " " + strings.Join(p.FixedCVEs.IDs, ",")
			}
			esetFprintf(tw, "  %s\t%s\t%s\t%d\t%d\t%s\t%s\n", truncate(name, 40), p.Version, p.Category, p.AffectedDevices, p.MaxRiskScore, truncate(cves, 50), p.RestartRequired)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if len(view.FailedAttempts) > 0 {
		fmt.Fprintln(w, "\nFailed patch attempts")
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "  STATUS\tAPPLICATION\tATTEMPTS\tDEVICES")
		for _, f := range view.FailedAttempts {
			esetFprintf(tw, "  %s\t%s\t%d\t%d\n", f.Status, truncate(f.Application, 40), f.Count, f.Devices)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	for _, f := range view.FetchFailures {
		esetFprintf(w, "fetch failed: %s: %s\n", f.Source, truncate(f.Error, 160))
	}
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
