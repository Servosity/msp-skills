// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: policy hygiene and golden-instance drift checks.

package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Defect names reported by policy hygiene.
const (
	DefectDisabled       = "disabled"
	DefectNoTrigger      = "no_trigger"
	DefectNoScope        = "no_scope"
	DefectExpired        = "expired"
	DefectUncategorized  = "uncategorized"
	DefectMissingScript  = "missing_script"
	DefectMissingPackage = "missing_package"
)

// HygieneRow is one defective policy.
type HygieneRow struct {
	Instance string   `json:"instance"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Defects  []string `json:"defects"`
	Detail   string   `json:"detail"`
	Error    string   `json:"error,omitempty"`
}

// Ref is an id/name pair of an object a policy points at.
type Ref struct{ ID, Name string }

// Known holds an instance's scripts and packages, by id and by name.
type Known struct {
	ScriptIDs, ScriptNames, PackageIDs, PackageNames map[string]bool
}

// NewKnown indexes Jamf Pro API scripts (id, name) and packages (id, packageName).
func NewKnown(scripts []Script, packages []Package) Known {
	k := Known{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}}
	for _, s := range scripts {
		k.ScriptIDs[s.ID], k.ScriptNames[s.Name] = true, true
	}
	for _, p := range packages {
		k.PackageIDs[p.ID], k.PackageNames[p.PackageName] = true, true
	}
	return k
}

// PolicyDefects inspects one Classic policy body ({"policy":{...}}).
// A referenced script or package is matched by id (Classic and Jamf Pro API
// objects share ids); its name is used only when the reference has no id. A
// deleted object recreated under the same name therefore still shows as
// missing, because the policy points at the old id.
func PolicyDefects(instance string, body json.RawMessage, k Known, now time.Time) (HygieneRow, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return HygieneRow{}, fmt.Errorf("unexpected policy shape: not valid JSON") // parse errors quote server bytes
	}
	p, _ := root["policy"].(map[string]any)
	if p == nil {
		return HygieneRow{}, fmt.Errorf("response has no policy object")
	}
	general, _ := p["general"].(map[string]any)
	row := HygieneRow{Instance: instance, ID: scalarString(general["id"]), Name: scalarString(general["name"])}
	var notes []string
	enabled, _ := general["enabled"].(bool)
	if !enabled {
		row.Defects = append(row.Defects, DefectDisabled)
	}
	selfService := false
	if ss, ok := p["self_service"].(map[string]any); ok {
		selfService, _ = ss["use_for_self_service"].(bool)
	}
	triggered := strings.TrimSpace(scalarString(general["trigger_other"])) != ""
	for _, t := range []string{"trigger_checkin", "trigger_enrollment_complete", "trigger_login", "trigger_logout", "trigger_network_state_changed", "trigger_startup"} {
		if on, _ := general[t].(bool); on {
			triggered = true
		}
	}
	if enabled && !triggered && !selfService {
		row.Defects = append(row.Defects, DefectNoTrigger)
		notes = append(notes, "no trigger and not in Self Service: runs only via 'jamf policy -id'")
	}
	// A policy whose scope cannot be read cannot be judged healthy.
	scope, ok := p["scope"].(map[string]any)
	if !ok {
		return HygieneRow{}, fmt.Errorf("policy has no scope object")
	}
	{
		all, _ := scope["all_computers"].(bool)
		targets := len(ObjectsUnder(scope["computers"], "computer")) + len(ObjectsUnder(scope["computer_groups"], "computer_group")) +
			len(ObjectsUnder(scope["buildings"], "building")) + len(ObjectsUnder(scope["departments"], "department"))
		if !all && targets == 0 {
			row.Defects = append(row.Defects, DefectNoScope)
		}
	}
	if exp := expiry(general); !exp.IsZero() && exp.Before(now) {
		row.Defects = append(row.Defects, DefectExpired)
		notes = append(notes, "expired "+exp.Format("2006-01-02"))
	}
	if cat, ok := general["category"].(map[string]any); !ok || uncategorized(cat) {
		row.Defects = append(row.Defects, DefectUncategorized)
	}
	for _, s := range refs(p["scripts"], "script") {
		if !known(s, k.ScriptIDs, k.ScriptNames) {
			row.Defects = appendOnce(row.Defects, DefectMissingScript)
			notes = append(notes, fmt.Sprintf("script %s (id %s) not found", s.Name, s.ID))
		}
	}
	var pkgs []Ref
	if pc, ok := p["package_configuration"].(map[string]any); ok {
		pkgs = refs(pc["packages"], "package")
	}
	for _, pk := range pkgs {
		if !known(pk, k.PackageIDs, k.PackageNames) {
			row.Defects = appendOnce(row.Defects, DefectMissingPackage)
			notes = append(notes, fmt.Sprintf("package %s (id %s) not found", pk.Name, pk.ID))
		}
	}
	row.Detail = strings.Join(notes, "; ")
	return row, nil
}

func known(r Ref, ids, names map[string]bool) bool {
	if r.ID != "" {
		return ids[r.ID]
	}
	return names[r.Name]
}

func uncategorized(cat map[string]any) bool {
	id := scalarString(cat["id"])
	name := strings.TrimSpace(scalarString(cat["name"]))
	return id == "" || id == "-1" || name == "" || strings.EqualFold(name, "No category assigned") || strings.EqualFold(name, "Unknown")
}

func refs(v any, singular string) []Ref {
	var out []Ref
	for _, o := range ObjectsUnder(v, singular) {
		out = append(out, Ref{ID: scalarString(o["id"]), Name: scalarString(o["name"])})
	}
	return out
}

// expiry reads date_time_limitations.expiration_date(_epoch); zero when unset.
func expiry(general map[string]any) time.Time {
	dtl, _ := general["date_time_limitations"].(map[string]any)
	if dtl == nil {
		return time.Time{}
	}
	if ms, ok := dtl["expiration_date_epoch"].(float64); ok && ms > 0 {
		return time.UnixMilli(int64(ms)).UTC()
	}
	if t, ok := ParseTime(scalarString(dtl["expiration_date"])); ok {
		return t
	}
	return time.Time{}
}

func appendOnce(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// Script is one /api/v1/scripts result.
type Script struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ScriptContents string `json:"scriptContents"`
}

// Package is one /api/v1/packages result.
type Package struct {
	ID          string `json:"id"`
	PackageName string `json:"packageName"`
}

// Named is any object compared by name only.
type Named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ExtensionAttribute is one /api/v1/computer-extension-attributes result.
type ExtensionAttribute struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ScriptContents string `json:"scriptContents"`
}

// Snapshot is one instance's comparable configuration: kind -> name ->
// content fingerprint ("" when only presence is compared).
type Snapshot map[string]map[string]string

// Baseline kinds.
const (
	KindScript   = "script"
	KindPackage  = "package"
	KindPolicy   = "policy"
	KindProfile  = "macos_profile"
	KindEA       = "computer_extension_attribute"
	KindSmartGrp = "smart_computer_group"
)

// Fingerprint is a short content hash. Empty content has a fingerprint too,
// so an empty golden script still differs from a non-empty copy; presence-only
// kinds use "" instead of calling this.
func Fingerprint(content string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(content, "\r\n", "\n")))
	return hex.EncodeToString(sum[:6])
}

// DriftRow is one difference from the golden instance.
type DriftRow struct {
	Instance string `json:"instance"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Error    string `json:"error,omitempty"`
}

// Drift statuses.
const (
	DriftMissing = "missing"
	DriftDiffers = "differs"
	DriftExtra   = "extra"
)

// Diff compares one instance with the golden snapshot. Duplicate names
// inside one instance compare by the last one seen.
func Diff(instance string, golden, other Snapshot, includeExtra bool) []DriftRow {
	var rows []DriftRow
	kinds := make([]string, 0, len(golden))
	for k := range golden {
		kinds = append(kinds, k)
	}
	for k := range other {
		if _, ok := golden[k]; !ok {
			kinds = append(kinds, k)
		}
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		g, o := golden[kind], other[kind]
		for _, name := range sortedKeys(g) {
			theirs, ok := o[name]
			switch {
			case !ok:
				rows = append(rows, DriftRow{Instance: instance, Kind: kind, Name: name, Status: DriftMissing, Detail: "in golden instance only"})
			case g[name] != "" && theirs != g[name]:
				rows = append(rows, DriftRow{Instance: instance, Kind: kind, Name: name, Status: DriftDiffers,
					Detail: fmt.Sprintf("content %s, golden %s", orNone(theirs), g[name])})
			}
		}
		if includeExtra {
			for _, name := range sortedKeys(o) {
				if _, ok := g[name]; !ok {
					rows = append(rows, DriftRow{Instance: instance, Kind: kind, Name: name, Status: DriftExtra, Detail: "not in golden instance"})
				}
			}
		}
	}
	return rows
}

func orNone(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
