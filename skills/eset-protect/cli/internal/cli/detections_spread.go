// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
//
// Hand-authored. `detections spread`: how far each detected file hash (or
// detection name) has spread across devices and customers, with the synced
// executable verdict (blocked, signer, LiveGrid reputation) joined in.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

const (
	detectionsSpreadUnknownDevice = "(unknown device)"
	detectionsSpreadDefaultSince  = "7d"
	detectionsSpreadDefaultLimit  = 50
)

// detectionsSpreadSeverityRank orders v1 severityLevel values.
var detectionsSpreadSeverityRank = map[string]int{
	"SEVERITY_LEVEL_UNSPECIFIED":   0,
	"SEVERITY_LEVEL_DIAGNOSTIC":    1,
	"SEVERITY_LEVEL_INFORMATIONAL": 2,
	"SEVERITY_LEVEL_LOW":           3,
	"SEVERITY_LEVEL_MEDIUM":        4,
	"SEVERITY_LEVEL_HIGH":          5,
}

// detectionsSpreadDetection is the subset of a v1 detection this command reads.
type detectionsSpreadDetection struct {
	UUID           string `json:"uuid"`
	DisplayName    string `json:"displayName"`
	TypeName       string `json:"typeName"`
	ObjectName     string `json:"objectName"`
	ObjectHashSha1 string `json:"objectHashSha1"`
	OccurTime      string `json:"occurTime"`
	SeverityLevel  string `json:"severityLevel"`
	Context        struct {
		DeviceUUID string `json:"deviceUuid"`
	} `json:"context"`
}

// detectionsSpreadExecRecord is the subset of a synced executable this command reads.
type detectionsSpreadExecRecord struct {
	UUID         string  `json:"uuid"`
	HashSha1     string  `json:"hashSha1"`
	IsBlocked    bool    `json:"isBlocked"`
	DevicesCount flexInt `json:"devicesCount"`
	Signatures   []struct {
		SignerName          string `json:"signerName"`
		SignatureTrustLevel string `json:"signatureTrustLevel"`
	} `json:"signatures"`
	LiveGridFindings struct {
		Reputation json.RawMessage `json:"reputation"`
	} `json:"liveGridFindings"`
}

type detectionsSpreadExecutable struct {
	UUID              string   `json:"uuid"`
	IsBlocked         bool     `json:"is_blocked"`
	DevicesCount      int64    `json:"devices_count"`
	Signer            string   `json:"signer"`
	SignatureTrust    string   `json:"signature_trust"`
	Reputation        *float64 `json:"reputation"`
	ReputationVerdict string   `json:"reputation_verdict"`
}

type detectionsSpreadGroup struct {
	Key         string `json:"key"`
	SHA1        string `json:"sha1"`
	Name        string `json:"name"`
	Object      string `json:"object"`
	Occurrences int    `json:"occurrences"`
	Devices     int    `json:"devices"`
	// CustomersKnown counts the customers the mirror proves the group was
	// seen at: COMPANY groups by UUID, or the one "(tenant)" of a
	// single-tenant account. Customers names them, plus "(unassigned)" when
	// the group was seen on the MSP's own devices, which is listed but never
	// counted (handfixes.json: customer-counts-exclude-unassigned). A
	// detection whose owner is unknown is never one of them (handfixes.json:
	// spread-counts-uncertainty-before-thresholds).
	CustomersKnown int      `json:"customers_known"`
	Customers      []string `json:"customers"`
	// DevicesUnknownOwner counts the distinct devices whose customer is
	// unknown (not in the mirror, or a group whose ancestry is not synced);
	// each detection with no device counts as its own. Each could be another
	// customer (or the MSP's own), so CustomersPossible = CustomersKnown +
	// DevicesUnknownOwner.
	DevicesUnknownOwner int `json:"devices_unknown_owner"`
	CustomersPossible   int `json:"customers_possible"`
	// ThresholdsProven is true when --min-customers and --min-devices hold
	// whatever customers the unknown-owner devices belong to. A group that
	// reaches them only if they belong to other customers is still listed,
	// with false here.
	ThresholdsProven bool   `json:"thresholds_proven"`
	FirstSeen        string `json:"first_seen"`
	LastSeen         string `json:"last_seen"`
	SeverityMax      string `json:"severity_max"`
	// UnattributedDetections counts this group's detections whose customer is
	// unknown.
	UnattributedDetections int                         `json:"unattributed_detections"`
	Executable             *detectionsSpreadExecutable `json:"executable"`
	Next                   string                      `json:"next"`
}

type detectionsSpreadView struct {
	Groups             []detectionsSpreadGroup `json:"groups"`
	Since              string                  `json:"since"`
	DetectionsScanned  int                     `json:"detections_scanned"`
	DetectionsInWindow int                     `json:"detections_in_window"`
	GroupsMatched      int                     `json:"groups_matched"`
	// GroupsUnproven counts the matched groups (before --limit) whose
	// thresholds_proven is false.
	GroupsUnproven int `json:"groups_unproven"`
	RejectedRows   int `json:"rejected_rows"`
	// UnattributedDetections counts every detection in the window (after
	// --since and --sha1, before --min-customers, --min-devices and --limit)
	// whose customer is unknown, so an empty or short list never hides
	// unresolved spread (handfixes.json: spread-counts-uncertainty-before-thresholds).
	UnattributedDetections     int      `json:"unattributed_detections"`
	UnattributedDetectionUUIDs []string `json:"unattributed_detection_uuids,omitempty"`
	// SyncedHint is set when the mirror is missing or a needed resource has no
	// completed sync, so an empty groups list is not read as "no hash spread".
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

type detectionsSpreadOptions struct {
	Since        time.Time // zero = no lower bound
	SHA1         string    // lower-case; empty = every hash
	MinCustomers int
	MinDevices   int
}

type detectionsSpreadResult struct {
	Groups       []detectionsSpreadGroup
	Scanned      int
	InWindow     int
	Rejected     int // detection and executable rows that could not be used
	Unattributed esetUnattributed
	Unproven     int // matched groups whose thresholds are not proven
}

// detectionsSpreadBounds is the range a group's customer and device counts
// can take once its unknown-owner devices are attributed. Each unknown-owner
// device (and each device-less detection) may belong to a known customer, to
// a new one, or to the MSP itself ("(unassigned)", never counted), and a
// device-less detection may sit on a device already counted or on a new one;
// nothing else is uncertain.
type detectionsSpreadBounds struct {
	customersMin, customersMax int
	devicesMin, devicesMax     int
}

func detectionsSpreadBoundsOf(known, unknownOwner, devices, deviceless int) detectionsSpreadBounds {
	// An unknown owner may be the MSP's own device, so it adds no customer
	// for certain: the minimum is the known count.
	b := detectionsSpreadBounds{customersMin: known, customersMax: known + unknownOwner, devicesMin: devices, devicesMax: devices + deviceless}
	if devices == 0 && deviceless > 0 {
		b.devicesMin = 1
	}
	return b
}

// possible: some attribution of the unknown-owner devices meets both thresholds.
func (b detectionsSpreadBounds) possible(o detectionsSpreadOptions) bool {
	return b.customersMax >= o.MinCustomers && b.devicesMax >= o.MinDevices
}

// proven: every attribution meets both thresholds.
func (b detectionsSpreadBounds) proven(o detectionsSpreadOptions) bool {
	return b.customersMin >= o.MinCustomers && b.devicesMin >= o.MinDevices
}

// detectionsSpreadNormalizeSHA1 validates a --sha1 value and lower-cases it.
func detectionsSpreadNormalizeSHA1(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if len(s) != 40 {
		return "", fmt.Errorf("--sha1 expects a 40-character SHA-1 hex digest (got %d characters)", len(s))
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return "", fmt.Errorf("--sha1 expects a hex digest; %q is not a hex character", r)
		}
	}
	return s, nil
}

// detectionsSpreadKey groups by lower-case SHA-1, or by type and object name
// when the detection carries no hash.
func detectionsSpreadKey(d detectionsSpreadDetection) string {
	if h := strings.ToLower(strings.TrimSpace(d.ObjectHashSha1)); h != "" {
		return h
	}
	return "name:" + d.TypeName + "|" + d.ObjectName
}

// detectionsSpreadReputationVerdict maps LiveGrid reputation to ESET's bands:
// below 0.3 malicious, 0.3 to 0.8 suspicious, above 0.8 safe.
func detectionsSpreadReputationVerdict(r *float64) string {
	switch {
	case r == nil:
		return ""
	case *r < 0.3:
		return "malicious"
	case *r <= 0.8:
		return "suspicious"
	default:
		return "safe"
	}
}

// detectionsSpreadCustomerLabels returns the display name for a customer.
// Customers are identified by COMPANY group UUID, never by name, so two
// distinct customers called "Acme" stay apart; when a name is shared by more
// than one COMPANY (case-insensitive), or collides with a bucket name, the
// first 8 characters of the UUID are appended: "Acme (1c7a8b9d)". Buckets
// ("(tenant)", "(unassigned)", "(group not in synced tree)", "(unknown
// device)") have no UUID and keep their names.
func detectionsSpreadCustomerLabels(tree *esetTree) func(esetGroup) string {
	counts := map[string]int{}
	if tree != nil {
		for _, g := range tree.companies() {
			counts[strings.ToLower(g.Name)]++
		}
	}
	for _, b := range []string{esetCustomerTenant, esetCustomerNone, esetCustomerUnknownGroup, esetCustomerParentHidden, detectionsSpreadUnknownDevice} {
		counts[strings.ToLower(b)]++
	}
	return func(g esetGroup) string {
		if g.UUID == "" || counts[strings.ToLower(g.Name)] < 2 {
			return g.Name
		}
		id := g.UUID
		if len(id) > 8 {
			id = id[:8]
		}
		return g.Name + " (" + id + ")"
	}
}

// detectionsSpreadCustomerID is a customer's identity: its COMPANY group UUID,
// or the bucket name for the UUID-less buckets.
func detectionsSpreadCustomerID(g esetGroup) string {
	if g.UUID != "" {
		return g.UUID
	}
	return "bucket:" + g.Name
}

// detectionsSpreadIndexExecutables maps lower-case hashSha1 to the executable
// view. A row that does not decode or has no uuid is counted in rej; a row
// with no hashSha1 simply cannot be joined.
func detectionsSpreadIndexExecutables(raw []json.RawMessage, rej *esetRejects) map[string]*detectionsSpreadExecutable {
	if rej == nil {
		rej = &esetRejects{}
	}
	out := map[string]*detectionsSpreadExecutable{}
	for i, r := range raw {
		if !esetIsObject(r) {
			rej.add(fmt.Sprintf("executable row %d", i), errors.New("not a JSON object"))
			continue
		}
		var e detectionsSpreadExecRecord
		if err := json.Unmarshal(r, &e); err != nil {
			rej.add(fmt.Sprintf("executable row %d", i), err)
			continue
		}
		if e.UUID == "" {
			rej.add(fmt.Sprintf("executable row %d", i), errors.New("no uuid"))
			continue
		}
		h := strings.ToLower(strings.TrimSpace(e.HashSha1))
		if h == "" {
			continue
		}
		if _, dup := out[h]; dup {
			continue
		}
		view := &detectionsSpreadExecutable{UUID: e.UUID, IsBlocked: e.IsBlocked, DevicesCount: int64(e.DevicesCount)}
		for _, s := range e.Signatures {
			if view.Signer == "" && s.SignerName != "" {
				view.Signer = s.SignerName
				view.SignatureTrust = shortEnum(s.SignatureTrustLevel, "SIGNATURE_TRUST_LEVEL_")
			}
		}
		if view.SignatureTrust == "" && len(e.Signatures) > 0 {
			view.SignatureTrust = shortEnum(e.Signatures[0].SignatureTrustLevel, "SIGNATURE_TRUST_LEVEL_")
		}
		if rep := strings.Trim(string(e.LiveGridFindings.Reputation), `"`); rep != "" && rep != "null" {
			if f, err := strconv.ParseFloat(rep, 64); err == nil {
				view.Reputation = &f
			}
		}
		view.ReputationVerdict = detectionsSpreadReputationVerdict(view.Reputation)
		out[h] = view
	}
	return out
}

// detectionsSpreadBuild groups v1 detections in the window and joins devices,
// customers and executables. Groups are sorted by customers, devices and
// occurrences, all descending. A detection that does not decode, or whose
// occurTime cannot be placed in a --since window, is counted in rej.
func detectionsSpreadBuild(rawDetections []json.RawMessage, devices []esetDevice, tree *esetTree, rawExecutables []json.RawMessage, opts detectionsSpreadOptions, rej *esetRejects) detectionsSpreadResult {
	if rej == nil {
		rej = &esetRejects{}
	}
	startRejects := rej.N
	res := detectionsSpreadResult{Groups: make([]detectionsSpreadGroup, 0)}
	byUUID := esetDevicesByUUID(devices)
	label := detectionsSpreadCustomerLabels(tree)
	execs := detectionsSpreadIndexExecutables(rawExecutables, rej)

	type acc struct {
		group          detectionsSpreadGroup
		devices        map[string]bool
		customers      map[string]string // known owner identity -> display label (customers and "(unassigned)")
		counted        map[string]bool   // the known owners that are customers (esetCountsAsCustomer)
		unknownDevices map[string]bool   // devices whose owner is unknown
		deviceless     int               // detections that name no device
		first          time.Time
		last           time.Time
		latest         time.Time
		severity       int
		severityID     string
	}
	byKey := map[string]*acc{}
	for i, raw := range rawDetections {
		res.Scanned++
		if !esetIsObject(raw) {
			rej.add(fmt.Sprintf("detection row %d", i), errors.New("not a JSON object"))
			continue
		}
		var d detectionsSpreadDetection
		if err := json.Unmarshal(raw, &d); err != nil {
			rej.add(fmt.Sprintf("detection row %d", i), err)
			continue
		}
		when, terr := time.Parse(time.RFC3339Nano, d.OccurTime)
		if !opts.Since.IsZero() && terr != nil {
			id := d.UUID
			if id == "" {
				id = strconv.Itoa(i)
			}
			rej.add("detection row "+id, fmt.Errorf("occurTime %q is not an RFC 3339 time", d.OccurTime))
			continue
		}
		if !opts.Since.IsZero() && when.Before(opts.Since) {
			continue
		}
		key := detectionsSpreadKey(d)
		if opts.SHA1 != "" && key != opts.SHA1 {
			continue
		}
		res.InWindow++
		a, ok := byKey[key]
		if !ok {
			a = &acc{devices: map[string]bool{}, customers: map[string]string{}, counted: map[string]bool{}, unknownDevices: map[string]bool{}, severity: -1}
			a.group.Key = key
			if !strings.HasPrefix(key, "name:") {
				a.group.SHA1 = key
			}
			byKey[key] = a
		}
		a.group.Occurrences++
		name := d.DisplayName
		if name == "" {
			name = d.TypeName
		}
		if terr == nil {
			if a.first.IsZero() || when.Before(a.first) {
				a.first = when
			}
			if when.After(a.last) {
				a.last = when
			}
		}
		// The most recent detection with a value names the group.
		newer := terr == nil && (a.latest.IsZero() || !when.Before(a.latest))
		if newer {
			a.latest = when
		}
		if name != "" && (newer || a.group.Name == "") {
			a.group.Name = name
		}
		if d.ObjectName != "" && (newer || a.group.Object == "") {
			a.group.Object = d.ObjectName
		}
		if rank, known := detectionsSpreadSeverityRank[d.SeverityLevel]; known && rank > a.severity {
			a.severity, a.severityID = rank, d.SeverityLevel
		} else if !known && d.SeverityLevel != "" && a.severity < 0 {
			a.severityID = d.SeverityLevel
		}
		dev := d.Context.DeviceUUID
		if dev != "" {
			a.devices[dev] = true
		}
		// Uncertainty is counted here, before any threshold. An unknown owner
		// (a device the mirror lacks, or a group whose ancestry is not
		// synced) is never a customer: each such device, and each detection
		// with no device, could be a different one.
		cust, known := esetDeviceOwner(tree, byUUID, dev)
		if !known {
			a.group.UnattributedDetections++
			res.Unattributed.add(d.UUID)
			if dev == "" {
				a.deviceless++
			} else {
				a.unknownDevices[dev] = true
			}
			continue
		}
		a.customers[detectionsSpreadCustomerID(cust)] = label(cust)
		if esetCountsAsCustomer(cust) {
			a.counted[detectionsSpreadCustomerID(cust)] = true
		}
	}

	for _, a := range byKey {
		g := a.group
		g.Devices = len(a.devices)
		g.Customers = make([]string, 0, len(a.customers))
		for _, c := range a.customers {
			g.Customers = append(g.Customers, c)
		}
		sort.Slice(g.Customers, func(i, j int) bool { return strings.ToLower(g.Customers[i]) < strings.ToLower(g.Customers[j]) })
		g.CustomersKnown = len(a.counted)
		g.DevicesUnknownOwner = len(a.unknownDevices) + a.deviceless
		bounds := detectionsSpreadBoundsOf(g.CustomersKnown, g.DevicesUnknownOwner, g.Devices, a.deviceless)
		g.CustomersPossible = bounds.customersMax
		if !bounds.possible(opts) {
			continue
		}
		g.ThresholdsProven = bounds.proven(opts)
		if !g.ThresholdsProven {
			res.Unproven++
		}
		if !a.first.IsZero() {
			g.FirstSeen = a.first.UTC().Format(time.RFC3339)
			g.LastSeen = a.last.UTC().Format(time.RFC3339)
		}
		g.SeverityMax = shortEnum(a.severityID, "SEVERITY_LEVEL_")
		if g.SHA1 != "" {
			if e, ok := execs[g.SHA1]; ok {
				copied := *e
				g.Executable = &copied
				if !copied.IsBlocked {
					g.Next = esetCLIName + " executables block " + copied.UUID + " --dry-run"
				}
			}
		}
		res.Groups = append(res.Groups, g)
	}
	sort.Slice(res.Groups, func(i, j int) bool {
		a, b := res.Groups[i], res.Groups[j]
		if a.CustomersKnown != b.CustomersKnown {
			return a.CustomersKnown > b.CustomersKnown
		}
		if a.CustomersPossible != b.CustomersPossible {
			return a.CustomersPossible > b.CustomersPossible
		}
		if a.Devices != b.Devices {
			return a.Devices > b.Devices
		}
		if a.Occurrences != b.Occurrences {
			return a.Occurrences > b.Occurrences
		}
		return a.Key < b.Key
	})
	res.Rejected = rej.N - startRejects
	return res
}

// detectionsSpreadUnattributedLine names the window's owner-unknown
// detections; it is printed whether or not any group matched.
func detectionsSpreadUnattributedLine(v detectionsSpreadView) string {
	return esetUnattributedLine(v.UnattributedDetections, "detection(s) in the window", v.UnattributedDetectionUUIDs, "",
		"Each of their devices may be another customer, so a hash can have spread further than CUSTOMERS shows. "+esetUnattributedResync, v.parentNotVisible)
}

func detectionsSpreadPrintTable(w io.Writer, v detectionsSpreadView) error {
	if len(v.Groups) == 0 {
		_, err := esetFprintf(w, "No detections matched (scanned %d stored detections, %d in the window).\n", v.DetectionsScanned, v.DetectionsInWindow)
		// Never an all-clear while detections of unknown owner are in the window.
		if un := detectionsSpreadUnattributedLine(v); err == nil && un != "" {
			_, err = esetFprintln(w, un)
		}
		if err == nil && v.RejectedRows > 0 {
			_, err = esetFprintf(w, "%d stored row(s) could not be read and were left out.\n", v.RejectedRows)
		}
		if err == nil && v.SyncedHint != "" {
			_, err = esetFprintln(w, v.SyncedHint)
		}
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tOBJECT\tCUSTOMERS\tDEVICES\tHITS\tSEVERITY\tLAST_SEEN\tBLOCKED\tREPUTATION\tSHA1")
	for _, g := range v.Groups {
		blocked, rep := "unknown", ""
		if g.Executable != nil {
			blocked = strconv.FormatBool(g.Executable.IsBlocked)
			rep = g.Executable.ReputationVerdict
		}
		customers := strconv.Itoa(g.CustomersKnown)
		for _, c := range g.Customers {
			if c == esetCustomerNone {
				customers += " +MSP"
			}
		}
		if g.DevicesUnknownOwner > 0 {
			customers += fmt.Sprintf(" (+%d unknown)", g.DevicesUnknownOwner)
		}
		esetFprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\t%s\n", g.Name, g.Object, customers, g.Devices,
			g.Occurrences, g.SeverityMax, g.LastSeen, blocked, rep, g.SHA1)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := esetFprintf(w, "\n%d of %d groups shown; %d detections in the window\n", len(v.Groups), v.GroupsMatched, v.DetectionsInWindow)
	if err == nil && v.GroupsUnproven > 0 {
		_, err = esetFprintf(w, "%d group(s) reach --min-customers / --min-devices only if their unknown-owner devices belong to other customers (thresholds_proven false).\n", v.GroupsUnproven)
	}
	if un := detectionsSpreadUnattributedLine(v); err == nil && un != "" {
		_, err = esetFprintln(w, un)
	}
	if err == nil && v.RejectedRows > 0 {
		_, err = esetFprintf(w, "%d stored row(s) could not be read and were left out.\n", v.RejectedRows)
	}
	if err == nil && v.SyncedHint != "" {
		_, err = esetFprintln(w, v.SyncedHint)
	}
	return err
}

func newNovelDetectionsSpreadCmd(flags *rootFlags) *cobra.Command {
	var since, sha1, dbPath string
	var minCustomers, minDevices, limit int

	cmd := &cobra.Command{
		Use:   "spread",
		Short: "How far each detected file hash has spread across devices and customers in a window",
		Long: "Groups the synced v1 detections (PROTECT and Inspect endpoint detections) that occurred\n" +
			"within --since by objectHashSha1 (case-insensitive), or by typeName + objectName when a\n" +
			"detection carries no hash. Each detection's context.deviceUuid is mapped to its synced\n" +
			"device and on to the customer (the nearest COMPANY ancestor group). customers_known counts\n" +
			"only the customers the mirror proves: COMPANY groups (on a single-tenant account, the one\n" +
			"\"(tenant)\"). A hit on the MSP's own devices is listed in customers as \"(unassigned)\"\n" +
			"(shown as +MSP in the table) but is not a customer, so it never counts toward\n" +
			"customers_known or --min-customers. A detection whose device is not in the local mirror,\n" +
			"or whose device's group (or an ancestor of it, other than the hidden root group of a scoped\n" +
			"API user; see 'fleet health') is not in the synced tree, has an unknown\n" +
			"owner: it is never counted as a customer, each such device (and each detection with no\n" +
			"device) is counted in devices_unknown_owner (each may be another customer or the MSP's own),\n" +
			"and customers_possible = customers_known + devices_unknown_owner. --min-customers (default 0,\n" +
			"every group) and --min-devices keep a group that could reach\n" +
			"them once those devices are attributed; thresholds_proven is false when it reaches them\n" +
			"only that way. unattributed_detections counts every owner-unknown detection in the window\n" +
			"(with their uuids) before the thresholds and --limit apply.\n" +
			"Customers are counted by COMPANY group UUID, so two customers that share a name count\n" +
			"twice; a shared name is shown with the first 8 characters of its UUID, e.g. \"Acme (1c7a8b9d)\".\n\n" +
			"Hash groups are left-joined to synced executables on hashSha1 for isBlocked, devicesCount,\n" +
			"signer and LiveGrid reputation (below 0.3 malicious, 0.3 to 0.8 suspicious, above 0.8 safe);\n" +
			"executable is null when the hash is not in the mirror. When the executable is known and not\n" +
			"blocked, next holds the block command to preview.\n\n" +
			"Groups are sorted by known customers, then possible customers, devices and occurrences, all\n" +
			"descending. A stored detection or executable that cannot be decoded, or a detection with no\n" +
			"parseable occurTime, is left out and counted in rejected_rows.\n\n" +
			"Use this command to see how far one file hash or detection has spread across devices and customers, and whether that executable is already blocked or signed. Do NOT use it for the newest detections to work this morning; use 'detections latest' or 'triage' instead.",
		Example: strings.Trim(`
  eset-protect-cli detections spread --since 7d --min-customers 2
  eset-protect-cli detections spread --sha1 3395856ce81f2b7382dee72602f798b642f14140 --since 30d
  eset-protect-cli detections spread --since 24h --min-devices 3 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "detections spread")
			}
			start, err := esetSince(since)
			if err != nil {
				_ = cmd.Usage()
				return err
			}
			hash, err := detectionsSpreadNormalizeSHA1(sha1)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if minCustomers < 0 || minDevices < 0 || limit < 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--min-customers, --min-devices and --limit must not be negative"))
			}
			view := detectionsSpreadView{Groups: make([]detectionsSpreadGroup, 0)}
			if !start.IsZero() {
				view.Since = start.UTC().Format(time.RFC3339)
			}
			emit := func() error {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return detectionsSpreadPrintTable(cmd.OutOrStdout(), view)
			}

			resources := []string{"detections", "devices", "device-groups", "executables"}
			db, err := openEsetStore(cmd, dbPath, resources...)
			if err != nil {
				return err
			}
			// executables only add the blocked/reputation join, so a missing
			// executables sync does not make an empty result untrustworthy.
			view.SyncedHint = esetMirrorHint(db, "detections", "devices", "device-groups")
			if db == nil {
				view.SyncedHint = esetMirrorHint(nil, resources...)
				return emit()
			}
			defer db.Close()
			// The stderr hint judges the same required resources as synced_hint.
			esetSyncHints(cmd, db, flags.maxAge, "detections", "devices", "device-groups")

			detections, err := loadEsetRaw(db, "detections")
			if err != nil {
				return fmt.Errorf("reading synced detections: %w", err)
			}
			devices, err := loadEsetDevices(db)
			if err != nil {
				return fmt.Errorf("reading synced devices: %w", err)
			}
			groups, err := loadEsetGroups(db)
			if err != nil {
				return fmt.Errorf("reading synced device groups: %w", err)
			}
			executables, err := loadEsetRaw(db, "executables")
			if err != nil {
				return fmt.Errorf("reading synced executables: %w", err)
			}
			var rej esetRejects
			tree := newEsetTree(groups)
			view.parentNotVisible = tree.parentNotVisible()
			view.ParentNotVisible = tree.parentNotVisibleGroups()
			res := detectionsSpreadBuild(detections, devices, tree, executables, detectionsSpreadOptions{
				Since: start, SHA1: hash, MinCustomers: minCustomers, MinDevices: minDevices,
			}, &rej)
			rej.warn(cmd)
			view.RejectedRows = res.Rejected
			view.DetectionsScanned = res.Scanned
			view.DetectionsInWindow = res.InWindow
			view.GroupsMatched = len(res.Groups)
			view.GroupsUnproven = res.Unproven
			view.Groups = res.Groups
			view.UnattributedDetections, view.UnattributedDetectionUUIDs = res.Unattributed.N, res.Unattributed.UUIDs()
			if limit > 0 && len(view.Groups) > limit {
				view.Groups = view.Groups[:limit]
			}
			return emit()
		},
	}
	cmd.Flags().StringVar(&since, "since", detectionsSpreadDefaultSince, "Only detections that occurred within this window (e.g. 24h, 7d, 2w)")
	cmd.Flags().StringVar(&sha1, "sha1", "", "Only this file hash (40-character SHA-1, case-insensitive)")
	cmd.Flags().IntVar(&minCustomers, "min-customers", 0, "Only groups seen at this many customers (COMPANY groups; the MSP's own devices do not count) or more")
	cmd.Flags().IntVar(&minDevices, "min-devices", 1, "Only groups seen on this many distinct devices or more")
	cmd.Flags().IntVar(&limit, "limit", detectionsSpreadDefaultLimit, "Maximum groups to show (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}
