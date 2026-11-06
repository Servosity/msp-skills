// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: typed views and checks behind the fleet reports. Every field
// name here is taken from the vendor OpenAPI schemas in spec.yaml.

package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RSQLQuote renders s as a double-quoted RSQL argument. Backslash, the quote
// character and the '*' wildcard are escaped, per the vendor's "Filtering
// with RSQL" guide, so user text is always matched literally.
func RSQLQuote(s string) string {
	return `"` + rsqlEscape(s) + `"`
}

func rsqlEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, `*`, `\*`).Replace(s)
}

// FindFilters builds the OR'd RSQL filters for a device lookup: exact serial,
// username and email, and a contains-match on the device name.
func FindFilters(q string) (computers, mobiles string) {
	exact := RSQLQuote(q)
	contains := `"*` + rsqlEscape(q) + `*"`
	computers = strings.Join([]string{
		"hardware.serialNumber==" + exact,
		"general.name==" + contains,
		"userAndLocation.username==" + exact,
		"userAndLocation.email==" + exact,
	}, ",")
	mobiles = strings.Join([]string{
		"serialNumber==" + exact,
		"displayName==" + contains,
		"username==" + exact,
		"emailAddress==" + exact,
	}, ",")
	return computers, mobiles
}

// Computer is one /api/v4/computers-inventory result (fields the fleet uses).
type Computer struct {
	ID      string `json:"id"`
	General struct {
		Name                 string `json:"name"`
		LastContact          string `json:"lastContact"`
		ManagementID         string `json:"managementId"`
		MDMProfileExpiration string `json:"mdmProfileExpiration"`
		RemoteManagement     struct {
			Managed *bool `json:"managed"`
		} `json:"remoteManagement"`
	} `json:"general"`
	Hardware struct {
		SerialNumber string `json:"serialNumber"`
		Model        string `json:"model"`
	} `json:"hardware"`
	UserAndLocation struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"userAndLocation"`
	OperatingSystem struct {
		Version          string `json:"version"`
		FileVault2Status string `json:"fileVault2Status"`
	} `json:"operatingSystem"`
	Security struct {
		FirewallEnabled *bool `json:"firewallEnabled"`
	} `json:"security"`
}

// Mobile is one /api/v2/mobile-devices/detail result.
type Mobile struct {
	MobileDeviceID string `json:"mobileDeviceId"`
	DeviceType     string `json:"deviceType"`
	General        struct {
		DisplayName     string `json:"displayName"`
		LastContactDate string `json:"lastContactDate"`
		Managed         *bool  `json:"managed"`
		ManagementID    string `json:"managementId"`
		OSVersion       string `json:"osVersion"`
	} `json:"general"`
	Hardware struct {
		SerialNumber string `json:"serialNumber"`
		Model        string `json:"model"`
	} `json:"hardware"`
	UserAndLocation struct {
		Username     string `json:"username"`
		EmailAddress string `json:"emailAddress"`
	} `json:"userAndLocation"`
}

// Results decodes the "results" array of a Jamf Pro API page into T. A body
// without a results array (null, {}, an error object) is an error, never an
// empty collection, and every entry must be an object carrying each required
// key with a non-null value.
func Results[T any](body json.RawMessage, required ...string) ([]T, error) {
	var page map[string]json.RawMessage
	if err := json.Unmarshal(body, &page); err != nil || page == nil {
		return nil, fmt.Errorf("unexpected response shape: not a Jamf Pro API page")
	}
	raw, ok := page["results"]
	if !ok || len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("unexpected response shape: no results array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, errors.New("unexpected response shape: not valid JSON") // parse errors quote server bytes
	}
	// Every caller reads page 0 or a merged full walk, so a reported total
	// with no results is a contradiction, not an empty list.
	var total int
	if t, ok := page["totalCount"]; ok && json.Unmarshal(t, &total) == nil && total > 0 && len(entries) == 0 {
		return nil, fmt.Errorf("unexpected response: totalCount is %d but results is empty", total)
	}
	out := make([]T, 0, len(entries))
	for i, e := range entries {
		var v T
		if err := DecodeObject(e, &v, required...); err != nil {
			return nil, fmt.Errorf("result %d: %w", i, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// DecodeObject decodes a JSON object into v and requires every named key to
// be present with a non-null value, so a null, empty or error body is never
// read as zero values.
func DecodeObject(body json.RawMessage, v any, required ...string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return fmt.Errorf("unexpected response shape: not an object")
	}
	for _, k := range required {
		if raw, ok := obj[k]; !ok || string(raw) == "null" {
			return fmt.Errorf("unexpected response shape: missing %s", k)
		}
	}
	if json.Unmarshal(body, v) != nil {
		return fmt.Errorf("unexpected response shape: a field has the wrong type") // parse errors quote server bytes
	}
	return nil
}

// ClassicList returns the items of a Classic API list body {"<root>": ...};
// every item must carry a non-empty value for each required key.
func ClassicList(body json.RawMessage, root, singular string, required ...string) ([]map[string]any, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("unexpected response shape: not a Classic API object")
	}
	items, err := classicItems(obj, root, singular)
	if err != nil {
		return nil, err
	}
	for i, it := range items {
		for _, k := range required {
			if strings.TrimSpace(scalarString(it[k])) == "" {
				return nil, fmt.Errorf("unexpected response shape: %s entry %d has no %s", root, i, k)
			}
		}
	}
	return items, nil
}

// classicItems reads obj[key] as a Classic list: an array or a
// {"size":N,"<singular>":...} object. A missing, null or scalar value is an
// error, not an empty list, and a declared size must match the entries.
func classicItems(obj map[string]any, key, singular string) ([]map[string]any, error) {
	v, ok := obj[key]
	switch v.(type) {
	case []any, map[string]any:
	default:
		if !ok || v == nil {
			return nil, fmt.Errorf("unexpected response shape: no %s list", key)
		}
		return nil, fmt.Errorf("unexpected response shape: %s is not a list", key)
	}
	if err := objectMembers(v, singular); err != nil {
		return nil, fmt.Errorf("unexpected response shape: %s %w", key, err)
	}
	items := ObjectsUnder(v, singular)
	if m, isObj := v.(map[string]any); isObj {
		// An object container holds its entries under the singular key; only
		// an explicit size 0 may omit it.
		if _, has := m[singular]; !has && scalarString(m["size"]) != "0" {
			return nil, fmt.Errorf("unexpected response shape: %s has no %s entries and does not declare size 0", key, singular)
		}
		if size, has := m["size"]; has {
			if n, err := strconv.Atoi(scalarString(size)); err != nil || n != len(items) {
				return nil, fmt.Errorf("unexpected response: %s declares a size that does not match its %d entries", key, len(items))
			}
		}
	}
	return items, nil
}

// FindRow is one device match.
type FindRow struct {
	Instance     string `json:"instance"`
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Serial       string `json:"serial"`
	Model        string `json:"model"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	LastContact  string `json:"last_contact"`
	Managed      *bool  `json:"managed"`
	ManagementID string `json:"management_id"`
	Next         string `json:"next"`
	// Truncated marks rows from an instance that matched more devices of
	// this kind than --limit returned.
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ComputerFindRow turns a computer match into a row. next is the
// per-instance command an operator runs next (bin is the CLI's name); it is
// never executed.
func ComputerFindRow(bin, instance string, c Computer) FindRow {
	return FindRow{
		Instance: instance, Kind: "computer", ID: c.ID, Name: c.General.Name, Serial: c.Hardware.SerialNumber,
		Model: c.Hardware.Model, Username: c.UserAndLocation.Username, Email: c.UserAndLocation.Email,
		LastContact: c.General.LastContact, Managed: c.General.RemoteManagement.Managed, ManagementID: c.General.ManagementID,
		Next: fmt.Sprintf("%s --instance %s computer-inventory get-detail %s", bin, instance, c.ID),
	}
}

// MobileFindRow turns a mobile device match into a row.
func MobileFindRow(bin, instance string, m Mobile) FindRow {
	kind := "mobile"
	if m.DeviceType != "" {
		kind = "mobile:" + m.DeviceType
	}
	return FindRow{
		Instance: instance, Kind: kind, ID: m.MobileDeviceID, Name: m.General.DisplayName, Serial: m.Hardware.SerialNumber,
		Model: m.Hardware.Model, Username: m.UserAndLocation.Username, Email: m.UserAndLocation.EmailAddress,
		LastContact: m.General.LastContactDate, Managed: m.General.Managed, ManagementID: m.General.ManagementID,
		Next: fmt.Sprintf("%s --instance %s mobile-devices get-detail %s", bin, instance, m.MobileDeviceID),
	}
}

// PostureOptions are the compliance thresholds.
type PostureOptions struct {
	StaleDays  int    // last contact older than this many days is stale
	MinOS      string // macOS below this version fails; empty skips the check
	WithinDays int    // MDM profile expiring within this many days fails
	Now        time.Time
}

// Check names used in failed_checks.
const (
	CheckStale          = "stale_check_in"
	CheckBelowOS        = "below_min_os"
	CheckFileVaultOff   = "filevault_off"
	CheckFirewallOff    = "firewall_off"
	CheckMDMExpiring    = "mdm_profile_expiring"
	CheckNeverContacted = "never_contacted"
)

// FailedChecks evaluates one managed computer. It returns the checks that
// failed and the checks that could not be judged because inventory had no
// data for them; unknown is never treated as a pass. A computer with no last
// contact time at all fails never_contacted. The MDM profile expiry is
// optional inventory and only ever fails, never counts as unknown.
func FailedChecks(c Computer, o PostureOptions) (failed, unknown []string) {
	if t, ok := ParseTime(c.General.LastContact); ok {
		if o.Now.Sub(t) > time.Duration(o.StaleDays)*24*time.Hour {
			failed = append(failed, CheckStale)
		}
	} else {
		failed = append(failed, CheckNeverContacted)
	}
	switch {
	case o.MinOS == "":
	case c.OperatingSystem.Version == "":
		unknown = append(unknown, CheckBelowOS)
	case CompareVersions(c.OperatingSystem.Version, o.MinOS) < 0:
		failed = append(failed, CheckBelowOS)
	}
	switch c.OperatingSystem.FileVault2Status {
	case "ALL_ENCRYPTED", "BOOT_ENCRYPTED":
	case "NOT_ENCRYPTED", "SOME_ENCRYPTED":
		failed = append(failed, CheckFileVaultOff)
	default: // NOT_APPLICABLE, missing, or a status this build has never seen
		unknown = append(unknown, CheckFileVaultOff)
	}
	switch {
	case c.Security.FirewallEnabled == nil:
		unknown = append(unknown, CheckFirewallOff)
	case !*c.Security.FirewallEnabled:
		failed = append(failed, CheckFirewallOff)
	}
	if t, ok := ParseTime(c.General.MDMProfileExpiration); ok && DaysUntil(t, o.Now) <= o.WithinDays {
		failed = append(failed, CheckMDMExpiring)
	}
	return failed, unknown
}

// Unmanaged reports whether inventory explicitly marks the computer unmanaged.
// A computer whose management state is missing is not excluded.
func (c Computer) Unmanaged() bool {
	return c.General.RemoteManagement.Managed != nil && !*c.General.RemoteManagement.Managed
}

// PostureRow is the per-instance compliance rollup. compliant counts Macs
// with every check judged and passed; incomplete counts Macs with at least
// one check inventory could not answer.
type PostureRow struct {
	Instance           string `json:"instance"`
	ManagedComputers   int    `json:"managed_computers"`
	UnmanagedComputers int    `json:"unmanaged_computers"`
	ManagedDevices     int    `json:"managed_mobile_devices"`
	UnmanagedDevices   int    `json:"unmanaged_mobile_devices"`
	Evaluated          int    `json:"evaluated"`
	ManagementUnknown  int    `json:"management_unknown"`
	Compliant          int    `json:"compliant"`
	Failing            int    `json:"failing"`
	Incomplete         int    `json:"incomplete"`
	Stale              int    `json:"stale_check_in"`
	NeverContacted     int    `json:"never_contacted"`
	BelowMinOS         int    `json:"below_min_os"`
	FileVaultOff       int    `json:"filevault_off"`
	FirewallOff        int    `json:"firewall_off"`
	MDMProfileExpiring int    `json:"mdm_profile_expiring"`
	PercentCompliant   *int   `json:"percent_compliant"`
	// Coverage is set when fewer managed Macs were evaluated than
	// inventory-information reports; PercentCompliant is then withheld.
	Coverage string `json:"coverage,omitempty"`
	Error    string `json:"error,omitempty"`
}

// FailingComputer is one Mac that failed a check or could not be judged.
type FailingComputer struct {
	Instance      string   `json:"instance"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Serial        string   `json:"serial"`
	LastContact   string   `json:"last_contact"`
	OSVersion     string   `json:"os_version"`
	FailedChecks  []string `json:"failed_checks"`
	UnknownChecks []string `json:"unknown_checks"`
	Error         string   `json:"error,omitempty"`
}

// InventoryInformation is GET /api/v1/inventory-information.
type InventoryInformation struct {
	ManagedComputers   int `json:"managedComputers"`
	UnmanagedComputers int `json:"unmanagedComputers"`
	ManagedDevices     int `json:"managedDevices"`
	UnmanagedDevices   int `json:"unmanagedDevices"`
}

// InventoryInformationKeys are required in an inventory-information body.
var InventoryInformationKeys = []string{"managedComputers", "unmanagedComputers", "managedDevices", "unmanagedDevices"}

// Posture evaluates every computer of one instance that inventory does not
// mark as unmanaged.
func Posture(instance string, info InventoryInformation, computers []Computer, o PostureOptions) (PostureRow, []FailingComputer) {
	row := PostureRow{Instance: instance, ManagedComputers: info.ManagedComputers, UnmanagedComputers: info.UnmanagedComputers,
		ManagedDevices: info.ManagedDevices, UnmanagedDevices: info.UnmanagedDevices}
	var failing []FailingComputer
	for _, c := range computers {
		if c.Unmanaged() {
			continue
		}
		row.Evaluated++
		if c.General.RemoteManagement.Managed == nil {
			row.ManagementUnknown++
		}
		failed, unknown := FailedChecks(c, o)
		for _, ch := range failed {
			switch ch {
			case CheckStale:
				row.Stale++
			case CheckNeverContacted:
				row.NeverContacted++
			case CheckBelowOS:
				row.BelowMinOS++
			case CheckFileVaultOff:
				row.FileVaultOff++
			case CheckFirewallOff:
				row.FirewallOff++
			case CheckMDMExpiring:
				row.MDMProfileExpiring++
			}
		}
		switch {
		case len(failed) > 0:
			row.Failing++
		case len(unknown) > 0:
			row.Incomplete++
		default:
			row.Compliant++
		}
		if len(failed) > 0 || len(unknown) > 0 {
			failing = append(failing, FailingComputer{Instance: instance, ID: c.ID, Name: c.General.Name, Serial: c.Hardware.SerialNumber,
				LastContact: c.General.LastContact, OSVersion: c.OperatingSystem.Version, FailedChecks: nonNil(failed), UnknownChecks: nonNil(unknown)})
		}
	}
	// A percentage over part of the managed fleet would read as an all-clear
	// for Macs nobody looked at, so it is withheld until coverage is whole.
	if row.Evaluated < info.ManagedComputers {
		row.Coverage = fmt.Sprintf("incomplete: evaluated %d of %d managed computers", row.Evaluated, info.ManagedComputers)
	} else if row.Evaluated > 0 {
		pct := row.Compliant * 100 / row.Evaluated
		row.PercentCompliant = &pct
	}
	return row, failing
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// PatchTitle is one /api/v3/patch-software-title-configurations entry.
type PatchTitle struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// PatchSummary is GET .../{id}/patch-summary.
type PatchSummary struct {
	Title         string `json:"title"`
	LatestVersion string `json:"latestVersion"`
	UpToDate      int    `json:"upToDate"`
	OutOfDate     int    `json:"outOfDate"`
}

// PatchPolicy is one /api/v2/patch-policies result.
type PatchPolicy struct {
	ID                           string `json:"id"`
	PolicyName                   string `json:"policyName"`
	PolicyEnabled                bool   `json:"policyEnabled"`
	SoftwareTitleConfigurationID string `json:"softwareTitleConfigurationId"`
	Failed                       int    `json:"failed"`
}

// UpdateStatus is one managed software update status.
type UpdateStatus struct {
	Status string `json:"status"`
}

// PatchRow is one instance x patch title (kind patch_title), or the
// instance's managed OS update failures (kind os_updates).
type PatchRow struct {
	Instance       string `json:"instance"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	LatestVersion  string `json:"latest_version"`
	UpToDate       int    `json:"up_to_date"`
	OutOfDate      int    `json:"out_of_date"`
	PercentLatest  *int   `json:"percent_latest"`
	PolicyFailures int    `json:"patch_policy_failures"`
	FailedUpdates  int    `json:"failed_os_updates"`
	Error          string `json:"error,omitempty"`
}

// failedUpdateStatuses are the managed software update states that mean the
// update did not go through (vendor enum on ManagedSoftwareUpdateStatus).
var failedUpdateStatuses = map[string]bool{"ERROR": true, "DOWNLOAD_FAILED": true, "INSTALL_FAILED": true}

// FailedUpdateFilter is the RSQL filter for those states.
const FailedUpdateFilter = "status=in=(ERROR,DOWNLOAD_FAILED,INSTALL_FAILED)"

// PatchRows builds one instance's rows. summaries[i] belongs to titles[i].
func PatchRows(instance string, titles []PatchTitle, summaries []PatchSummary, policies []PatchPolicy, updates []UpdateStatus) []PatchRow {
	failures := map[string]int{}
	for _, p := range policies {
		failures[p.SoftwareTitleConfigurationID] += p.Failed
	}
	listed := map[string]bool{}
	for _, t := range titles {
		listed[t.ID] = true
	}
	rows := make([]PatchRow, 0, len(titles)+2)
	for i, t := range titles {
		s := summaries[i]
		name := FirstNonEmpty(t.DisplayName, s.Title)
		r := PatchRow{Instance: instance, Kind: "patch_title", Title: name, LatestVersion: s.LatestVersion,
			UpToDate: s.UpToDate, OutOfDate: s.OutOfDate, PolicyFailures: failures[t.ID]}
		if total := s.UpToDate + s.OutOfDate; total > 0 {
			pct := s.UpToDate * 100 / total
			r.PercentLatest = &pct
		}
		rows = append(rows, r)
	}
	failed := 0
	for _, u := range updates {
		if failedUpdateStatuses[u.Status] {
			failed++
		}
	}
	// Failures of patch policies whose title is not listed are still
	// failures: report them instead of dropping them.
	unmatched := 0
	for id, n := range failures {
		if !listed[id] {
			unmatched += n
		}
	}
	if unmatched > 0 {
		rows = append(rows, PatchRow{Instance: instance, Kind: "patch_policy_unmatched", Title: "Patch policy failures for titles not listed", PolicyFailures: unmatched})
	}
	rows = append(rows, PatchRow{Instance: instance, Kind: "os_updates", Title: "Managed software updates", FailedUpdates: failed})
	return rows
}

// Behind reports whether a row needs attention.
func (r PatchRow) Behind() bool {
	if r.Error != "" {
		return true
	}
	return r.OutOfDate > 0 || r.PolicyFailures > 0 || r.FailedUpdates > 0
}

// HealthRow is one instance x service-plane item.
type HealthRow struct {
	Instance string `json:"instance"`
	Item     string `json:"item"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	DaysLeft *int   `json:"days_left"`
	Error    string `json:"error,omitempty"`
}

// Status levels.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// DeviceEnrollment is one ADE (Automated Device Enrollment) token instance.
type DeviceEnrollment struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	TokenExpirationDate string `json:"tokenExpirationDate"`
}

// EnrollmentSync is GET /api/v1/device-enrollments/{id}/syncs/latest.
type EnrollmentSync struct {
	SyncState string `json:"syncState"`
	Timestamp string `json:"timestamp"`
}

// VPPLocation is one volume purchasing (Apps and Books) location.
type VPPLocation struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	TokenExpiration       string `json:"tokenExpiration"`
	ClientContextMismatch *bool  `json:"clientContextMismatch"` // nil: not reported
	LastSyncTime          string `json:"lastSyncTime"`
}

// ExpiryRow rates a token expiry date against the warning window.
func ExpiryRow(instance, item, name, expiry string, within int, now time.Time) HealthRow {
	r := HealthRow{Instance: instance, Item: item, Name: name, Status: StatusOK, Detail: "expires " + expiry}
	t, ok := ParseTime(expiry)
	if !ok {
		r.Status, r.Detail = StatusWarn, "no expiry date reported"
		return r
	}
	days := DaysUntil(t, now)
	r.DaysLeft = &days
	switch {
	case days < 0:
		r.Status, r.Detail = StatusFail, fmt.Sprintf("expired %s", t.Format("2006-01-02"))
	case days <= within:
		r.Status, r.Detail = StatusWarn, fmt.Sprintf("expires %s (in %d days)", t.Format("2006-01-02"), days)
	default:
		r.Detail = fmt.Sprintf("expires %s", t.Format("2006-01-02"))
	}
	return r
}

// SyncRow rates the latest ADE sync. Jamf's documented states are not
// enumerated in the schema, so any state containing "FAIL" or "ERROR" fails
// and an empty state warns.
func SyncRow(instance, name string, s EnrollmentSync) HealthRow {
	r := HealthRow{Instance: instance, Item: "ade_sync", Name: name, Status: StatusOK, Detail: strings.TrimSpace(s.SyncState + " " + s.Timestamp)}
	up := strings.ToUpper(s.SyncState)
	switch {
	case up == "":
		r.Status, r.Detail = StatusWarn, "no sync recorded"
	case strings.Contains(up, "FAIL"), strings.Contains(up, "ERROR"):
		r.Status = StatusFail
	}
	return r
}

// MarkVersionDrift flags every version row older than the newest in the fleet.
func MarkVersionDrift(rows []HealthRow) {
	newest := ""
	for _, r := range rows {
		if r.Item == "version" && r.Error == "" && (newest == "" || CompareVersions(r.Name, newest) > 0) {
			newest = r.Name
		}
	}
	for i := range rows {
		if rows[i].Item != "version" || rows[i].Error != "" {
			continue
		}
		if CompareVersions(rows[i].Name, newest) < 0 {
			rows[i].Status, rows[i].Detail = StatusWarn, "behind the newest version in the fleet ("+newest+")"
		} else {
			rows[i].Detail = "newest in the fleet"
		}
	}
}

// AppRow is one instance x installed version below the floor.
type AppRow struct {
	Instance  string `json:"instance"`
	App       string `json:"app"`
	Version   string `json:"version"`
	Computers int    `json:"computers"`
	Names     string `json:"computer_names"`
	Error     string `json:"error,omitempty"`
}

// AppVersion is one installed version and the computers that have it.
type AppVersion struct {
	Number    string
	Computers []string
}

// AppVersions reads a Classic computerapplications body. The vendor fragment
// documents the XML shape (versions > version > computers > computer); the
// JSON rendering may or may not keep the singular wrapper keys, so both are
// accepted.
func AppVersions(body json.RawMessage) ([]AppVersion, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, errors.New("unexpected response shape: not valid JSON") // parse errors quote server bytes
	}
	obj, _ := root["computer_applications"].(map[string]any)
	if obj == nil {
		return nil, fmt.Errorf("response has no computer_applications object")
	}
	versions, err := classicItems(obj, "versions", "version")
	if err != nil {
		return nil, fmt.Errorf("computer_applications: %w", err)
	}
	var out []AppVersion
	for i, v := range versions {
		computers, err := classicItems(v, "computers", "computer")
		if err != nil {
			return nil, fmt.Errorf("computer_applications version %d: %w", i, err)
		}
		av := AppVersion{Number: scalarString(v["number"])}
		for _, c := range computers {
			av.Computers = append(av.Computers, FirstNonEmpty(scalarString(c["name"]), scalarString(c["serial_number"]), scalarString(c["id"])))
		}
		out = append(out, av)
	}
	return out, nil
}

// objectMembers fails when a Classic list holds anything but objects
// (nested arrays are flattened, as ObjectsUnder does): a null or scalar entry
// would otherwise vanish and shrink the list without an error. A wrapper
// entry ({"<singular>": ...}, optionally beside "size") is checked the same
// way, and its declared size must match what it holds.
func objectMembers(v any, singular string) error {
	var check func(x any) error
	check = func(x any) error {
		switch t := x.(type) {
		case map[string]any:
			inner, has := t[singular]
			if !has || !(len(t) == 1 || (len(t) == 2 && t["size"] != nil)) {
				return nil // an entry
			}
			if err := check(inner); err != nil {
				return err
			}
			if size, ok := t["size"]; ok {
				if n, err := strconv.Atoi(scalarString(size)); err != nil || n != len(ObjectsUnder(t, singular)) {
					return fmt.Errorf("has a wrapper whose declared size does not match its %d entries", len(ObjectsUnder(t, singular)))
				}
			}
			return nil
		case []any:
			for _, y := range t {
				if err := check(y); err != nil {
					return err
				}
			}
			return nil
		}
		return fmt.Errorf("has an entry that is not an object")
	}
	if t, ok := v.(map[string]any); ok {
		if inner, has := t[singular]; has {
			return check(inner) // classicItems checks the top-level size
		}
		return nil
	}
	return check(v)
}

// BelowFloor returns the versions strictly below floor, plus installs with
// no version at all: those cannot be judged, so they are reported (Number
// "") rather than counted as current.
func BelowFloor(versions []AppVersion, floor string) []AppVersion {
	var out []AppVersion
	for _, v := range versions {
		if v.Number == "" || CompareVersions(v.Number, floor) < 0 {
			out = append(out, v)
		}
	}
	return out
}

// ObjectsUnder flattens a Classic API list in either JSON rendering:
// [obj, ...], [{"<singular>": obj}, ...], {"size":N,"<singular>":[obj, ...]}
// or {"<singular>": obj}.
func ObjectsUnder(v any, singular string) []map[string]any {
	var out []map[string]any
	var add func(x any)
	add = func(x any) {
		// The vendor's XML-derived schemas sometimes nest a list inside a
		// list (computer_applications versions.version); flatten it.
		if arr, ok := x.([]any); ok {
			for _, y := range arr {
				add(y)
			}
			return
		}
		m, ok := x.(map[string]any)
		if !ok {
			return
		}
		// A wrapper {"<singular>": ...}, optionally with the XML-derived
		// "size" beside it, holds the entries; anything else is an entry.
		if inner, ok := m[singular]; ok && (len(m) == 1 || (len(m) == 2 && m["size"] != nil)) {
			switch inner.(type) {
			case map[string]any, []any:
				add(inner)
				return
			}
		}
		out = append(out, m)
	}
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			add(x)
		}
	case map[string]any:
		switch inner := t[singular].(type) {
		case []any:
			for _, x := range inner {
				add(x)
			}
		case map[string]any:
			out = append(out, inner)
		}
	}
	return out
}

// IDString renders a JSON id (string or number) without float formatting.
func IDString(v any) string { return scalarString(v) }

func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

// FirstNonEmpty returns the first non-blank string.
func FirstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
