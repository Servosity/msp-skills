// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// ancestryRefScopeRoot is the reference hidden scope root: list the parent
// of every top-level group (a group whose parent the mirror does not hold);
// when that list names exactly one parent and it is not empty, that parent
// is the scope root.
func ancestryRefScopeRoot(byID map[string]esetGroup) string {
	parents := map[string]bool{}
	for _, g := range byID {
		if _, held := byID[g.ParentUUID]; !held {
			parents[g.ParentUUID] = true
		}
	}
	if len(parents) != 1 {
		return ""
	}
	for p := range parents {
		return p
	}
	return ""
}

// ancestryRef is the reference owner, computed bottom-up instead of by
// walking: a COMPANY owns itself; an MSP group, a root group (empty parent)
// and a group whose parent is the hidden scope root are "(unassigned)"; every
// other synced group takes its parent's owner once that is settled. The
// scope root itself is "(unassigned)". Iterating to a fixpoint settles
// exactly the groups whose ancestry reaches a COMPANY, an MSP group, the
// root or the scope root through synced groups; whatever is left (a missing
// interior ancestor, a cycle) is owner-unknown. Round 5: an owner-unknown
// group whose chain of synced groups ends at a missing parent (not the root,
// not the scope root) is "(parent group not visible)"; a group that is not
// synced, an empty UUID and a cycle stay "(group not in synced tree)".
func ancestryRef(groups []esetGroup) func(string) (esetGroup, bool) {
	byID := map[string]esetGroup{}
	companies := false
	for _, g := range groups {
		if g.UUID == "" {
			continue
		}
		byID[g.UUID] = g
		if g.EntityType == esetCompanyEntityType {
			companies = true
		}
	}
	root := ancestryRefScopeRoot(byID)
	none := esetGroup{Name: esetCustomerNone}
	settled := map[string]esetGroup{}
	for changed := true; changed; {
		changed = false
		for id, g := range byID {
			if _, ok := settled[id]; ok {
				continue
			}
			switch {
			case g.EntityType == esetCompanyEntityType:
				settled[id] = g
			case g.EntityType == esetMSPEntityType, g.ParentUUID == "", g.ParentUUID == root:
				settled[id] = none
			default:
				p, ok := settled[g.ParentUUID]
				if !ok {
					continue
				}
				settled[id] = p
			}
			changed = true
		}
	}
	underHidden := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for id, g := range byID {
			if underHidden[id] {
				continue
			}
			_, parentHeld := byID[g.ParentUUID]
			if (!parentHeld && g.ParentUUID != "" && g.ParentUUID != root) || underHidden[g.ParentUUID] {
				underHidden[id] = true
				changed = true
			}
		}
	}
	return func(id string) (esetGroup, bool) {
		if !companies {
			return esetGroup{Name: esetCustomerTenant}, true
		}
		if g, ok := settled[id]; ok {
			return g, true
		}
		if id != "" && id == root {
			return none, true
		}
		if underHidden[id] {
			return esetGroup{Name: esetCustomerParentHidden}, false
		}
		return esetGroup{Name: esetCustomerUnknownGroup}, false
	}
}

// ancestryGen builds a random mirror: groups with random parents (the root,
// another group, itself, or a group that is not synced), some COMPANY, some
// MSP, deep chains, and a random subset dropped from the mirror while still
// referenced. A third of the trees are a scoped API user's view: the root
// groups name a parent ESET never returns ("hidden-root") instead of an empty
// one, so the mirror holds a hidden scope root unless a dropped group adds a
// second missing parent.
func ancestryGen(r *rand.Rand) (synced []esetGroup, ids []string) {
	n := 1 + r.Intn(14)
	all := make([]esetGroup, n)
	for i := range all {
		all[i] = esetGroup{UUID: fmt.Sprintf("g%d", i), Name: fmt.Sprintf("Group %d", i)}
	}
	chain := r.Intn(3) == 0 // a deep chain: each group under the previous
	rootParent := ""
	if r.Intn(3) == 0 {
		rootParent = "hidden-root"
	}
	for i := range all {
		switch p := r.Intn(10); {
		case chain && i > 0:
			all[i].ParentUUID = all[i-1].UUID
		case chain:
			all[i].ParentUUID = rootParent
		case p < 2:
			all[i].ParentUUID = rootParent
		case p < 3:
			all[i].ParentUUID = fmt.Sprintf("gone%d", r.Intn(3))
		default:
			all[i].ParentUUID = all[r.Intn(n)].UUID
		}
		switch k := r.Intn(12); {
		case k < 3:
			all[i].EntityType = esetCompanyEntityType
		case k < 4:
			all[i].EntityType = esetMSPEntityType
		case k < 5:
			all[i].EntityType = "DEVICE_GROUP_ENTITY_TYPE_SITE"
		}
	}
	for _, g := range all {
		if r.Intn(7) != 0 {
			synced = append(synced, g)
		}
		ids = append(ids, g.UUID)
	}
	ids = append(ids, "gone0", "gone1", "hidden-root", "")
	return synced, ids
}

// ancestryFixedTrees are the shapes the random trees reach only by chance:
// a scoped API user's MSP tree whose root ESET never returns (MSP -> COMPANY
// -> SITE, devices in each and directly in the MSP group, a non-COMPANY
// group straight under the hidden root), the same tree with a missing
// interior node (a site paged in before its new company: two missing
// parents, so no scope root, and only the MSP-type rule keeps the MSP's own
// devices known), the same tree with the MSP group not typed MSP (only the
// scope root decides), a tree whose COMPANY groups hang directly off the
// hidden root, and a full tree whose real root is visible.
func ancestryFixedTrees() [][]esetGroup {
	msp := []esetGroup{
		{UUID: "msp", Name: "Servosity MSP", ParentUUID: "hidden-root", EntityType: esetMSPEntityType},
		{UUID: "acme", Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "acme-hq", Name: "HQ", ParentUUID: "acme", EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{UUID: "beta", Name: "Beta Co", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "beta-site", Name: "Warehouse", ParentUUID: "beta", EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{UUID: "lost", Name: "Lost and found", ParentUUID: "hidden-root"},
	}
	interior := append(append([]esetGroup(nil), msp...),
		esetGroup{UUID: "paged-site", Name: "New site", ParentUUID: "gone0", EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"})
	untyped := append([]esetGroup(nil), msp...)
	untyped[0].EntityType = ""
	companyTop := []esetGroup{
		{UUID: "acme", Name: "Acme", ParentUUID: "hidden-root", EntityType: esetCompanyEntityType},
		{UUID: "acme-hq", Name: "HQ", ParentUUID: "acme", EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{UUID: "beta", Name: "Beta Co", ParentUUID: "hidden-root", EntityType: esetCompanyEntityType},
	}
	full := append([]esetGroup{{UUID: "hidden-root", Name: "All"}}, msp...)
	return [][]esetGroup{msp, interior, untyped, companyTop, full}
}

// ancestryFixedIDs lists every group of trees plus the missing ones.
func ancestryFixedIDs(groups []esetGroup) []string {
	ids := []string{"hidden-root", "gone0", ""}
	for _, g := range groups {
		ids = append(ids, g.UUID)
	}
	return ids
}

// TestHiddenScopeRootIsKnown pins the verdicts on the scoped MSP tree that
// adversarial round 4 found filed as owner-unknown: the MSP's own devices
// (in the MSP group, or in a non-COMPANY group under the hidden root) are
// "(unassigned)", known; a missing interior node and a cycle stay unknown.
func TestHiddenScopeRootIsKnown(t *testing.T) {
	trees := ancestryFixedTrees()
	msp, interior, untyped := newEsetTree(trees[0]), newEsetTree(trees[1]), newEsetTree(trees[2])
	if msp.scopeRoot != "hidden-root" || interior.scopeRoot != "" || untyped.scopeRoot != "hidden-root" || newEsetTree(trees[4]).scopeRoot != "" {
		t.Fatalf("scope roots: msp %q interior %q untyped %q", msp.scopeRoot, interior.scopeRoot, untyped.scopeRoot)
	}
	for _, tc := range []struct {
		tree      *esetTree
		group     string
		wantName  string
		wantKnown bool
	}{
		{msp, "msp", esetCustomerNone, true},
		{msp, "lost", esetCustomerNone, true},
		{msp, "hidden-root", esetCustomerNone, true},
		{msp, "acme-hq", "Acme", true},
		{msp, "gone0", esetCustomerUnknownGroup, false},
		{msp, "", esetCustomerUnknownGroup, false},
		{interior, "msp", esetCustomerNone, true},
		{interior, "acme-hq", "Acme", true},
		{interior, "paged-site", esetCustomerParentHidden, false},
		{interior, "lost", esetCustomerParentHidden, false},
		{interior, "hidden-root", esetCustomerUnknownGroup, false},
		{untyped, "msp", esetCustomerNone, true},
		{untyped, "lost", esetCustomerNone, true},
	} {
		g, k := tc.tree.ownerOf(tc.group)
		if g.Name != tc.wantName || k != tc.wantKnown {
			t.Errorf("ownerOf(%q) = %q %v, want %q %v", tc.group, g.Name, k, tc.wantName, tc.wantKnown)
		}
	}
	cycle := newEsetTree([]esetGroup{
		{UUID: "msp", ParentUUID: "hidden-root", EntityType: esetMSPEntityType},
		{UUID: "acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "a", ParentUUID: "b"}, {UUID: "b", ParentUUID: "a"},
	})
	if g, k := cycle.ownerOf("a"); k || g.Name != esetCustomerUnknownGroup {
		t.Errorf("a cycle under a hidden root = %q %v, want owner-unknown", g.Name, k)
	}
}

// TestAncestryWalkMatchesReference brute-forces the one ancestry walk
// (esetTree.resolve, through ownerOf and customerOf) against the bottom-up
// reference over random trees with missing nodes, cycles and deep chains,
// querying every id in random order on one memoizing tree and on a fresh
// tree per query.
func TestAncestryWalkMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(20261019))
	fixed := ancestryFixedTrees()
	for trial := 0; trial < 20000+len(fixed); trial++ {
		var synced []esetGroup
		var ids []string
		if trial < len(fixed) {
			synced, ids = fixed[trial], ancestryFixedIDs(fixed[trial])
		} else {
			synced, ids = ancestryGen(r)
		}
		ref := ancestryRef(synced)
		shared := newEsetTree(synced)
		r.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		for _, id := range ids {
			wantG, wantK := ref(id)
			for _, tr := range []*esetTree{shared, newEsetTree(synced)} {
				g, k := tr.ownerOf(id)
				if k != wantK || g != wantG || tr.customerOf(id) != wantG {
					t.Fatalf("trial %d: ownerOf(%q) = %+v %v, want %+v %v\ntree %+v", trial, id, g, k, wantG, wantK, synced)
				}
			}
		}
	}
}

// TestEveryAttributionCallerAgreesWithTheWalk checks each caller of the walk
// on random trees: esetDeviceOwner, exposureScope.owner/customerFor (with
// the device fallback only for a group the tree does not hold),
// reportVulnsAttribute with and without a target customer, and the fleet
// views, where every device is either the target customer's, another known
// owner's, or counted unattributed, never silently dropped.
func TestEveryAttributionCallerAgreesWithTheWalk(t *testing.T) {
	r := rand.New(rand.NewSource(77))
	now := time.Now()
	fixed := ancestryFixedTrees()
	for trial := 0; trial < 3000+len(fixed); trial++ {
		var synced []esetGroup
		var ids []string
		if trial < len(fixed) {
			synced, ids = fixed[trial], ancestryFixedIDs(fixed[trial])
		} else {
			synced, ids = ancestryGen(r)
		}
		ref := ancestryRef(synced)
		var byIDRef = map[string]esetGroup{}
		for _, g := range synced {
			if g.UUID != "" {
				byIDRef[g.UUID] = g
			}
		}
		refRoot := ancestryRefScopeRoot(byIDRef)
		tree := newEsetTree(synced)
		syncedIDs := map[string]bool{}
		for _, g := range synced {
			syncedIDs[g.UUID] = true
		}
		var devices []esetDevice
		for i, id := range ids {
			devices = append(devices, esetDevice{UUID: fmt.Sprintf("d%d", i), ParentGroupUUID: id})
		}
		byUUID := esetDevicesByUUID(devices)
		scope := newExposureScope(synced, devices)
		for _, d := range devices {
			wantG, wantK := ref(d.ParentGroupUUID)
			if g, k := esetDeviceOwner(tree, byUUID, d.UUID); g != wantG || k != wantK {
				t.Fatalf("esetDeviceOwner(%s in %q) = %+v %v, want %+v %v", d.UUID, d.ParentGroupUUID, g, k, wantG, wantK)
			}
			for _, rowGroup := range ids {
				eg, ek := wantG, wantK
				switch {
				case syncedIDs[rowGroup]:
					eg, ek = ref(rowGroup)
				case rowGroup == "" || !syncedIDs[rowGroup]:
					switch {
					case syncedIDs[d.ParentGroupUUID]:
					case refRoot != "" && (rowGroup == refRoot || d.ParentGroupUUID == refRoot):
						eg, ek = ref(refRoot)
					default:
						eg, ek = ref("")
					}
				}
				if g, k := scope.owner(rowGroup, d.UUID); g != eg || k != ek {
					t.Fatalf("scope.owner(%q, %s in %q) = %+v %v, want %+v %v", rowGroup, d.UUID, d.ParentGroupUUID, g, k, eg, ek)
				}
				if g := scope.customerFor(rowGroup, d.UUID); g != eg {
					t.Fatalf("customerFor(%q, %s) = %+v, want %+v", rowGroup, d.UUID, g, eg)
				}
			}
		}
		for _, target := range append(tree.companies(), esetGroup{}) {
			for _, id := range ids {
				wantG, wantK := ref(id)
				if target.UUID == "" {
					if g, keep := reportVulnsAttribute(tree, nil, id); g != wantG || !keep {
						t.Fatalf("reportVulnsAttribute(nil, %q) = %+v %v, want %+v kept", id, g, keep, wantG)
					}
					continue
				}
				tg := target
				_, keep := reportVulnsAttribute(tree, &tg, id)
				if want := wantK && wantG.UUID == target.UUID; keep != want {
					t.Fatalf("reportVulnsAttribute(%s, %q) keep = %v, want %v", target.UUID, id, keep, want)
				}
			}
			if target.UUID == "" {
				continue
			}
			// fleet stale --customer: every stale device is the target's
			// row, another known owner's, or unattributed.
			tg := target
			view := fleetStaleRank(devices, tree, fleetStaleOptions{Days: 1, Customer: &tg}, now)
			var rows, unknown []string
			for _, row := range view.Devices {
				rows = append(rows, row.DeviceUUID)
			}
			var wantRows, wantUnknown []string
			for _, d := range devices {
				g, k := ref(d.ParentGroupUUID)
				switch {
				case !k:
					wantUnknown = append(wantUnknown, d.UUID)
				case g.UUID == target.UUID:
					wantRows = append(wantRows, d.UUID)
				}
			}
			unknown = view.UnattributedDeviceUUIDs
			sort.Strings(rows)
			sort.Strings(wantRows)
			sort.Strings(wantUnknown)
			if !reflect.DeepEqual(rows, wantRows) || !reflect.DeepEqual(unknown, wantUnknown) || view.UnattributedDevices != len(wantUnknown) {
				t.Fatalf("fleet stale --customer %s: rows %v unattributed %v, want %v and %v\ntree %+v", target.UUID, rows, unknown, wantRows, wantUnknown, synced)
			}
		}
		// fleet health: every device lands in exactly one row, unknown ones
		// in the owner-unknown bucket.
		total, unknownRow := 0, 0
		for _, row := range fleetHealthRollup(devices, tree, now, 14) {
			total += row.Devices
			if esetOwnerUnknownBucket(row.Customer) {
				unknownRow += row.Devices
			}
		}
		wantUnknown := 0
		for _, d := range devices {
			if _, k := ref(d.ParentGroupUUID); !k {
				wantUnknown++
			}
		}
		if total != len(devices) || unknownRow != wantUnknown {
			t.Fatalf("fleet health rolled %d of %d devices, %d unknown (want %d)", total, len(devices), unknownRow, wantUnknown)
		}
	}
}

// TestMissingAncestorIsUnattributedInEveryView is the Codex round-3
// scenario end to end: a paged device-groups write put a new site group in
// the mirror before its new parent, the device-groups sync stamp is still
// complete, and a device in that site has a detection and an open incident.
// Before the fix the walk stopped at the missing parent and filed the site
// under "(unassigned)", a known owner, so report customer, triage --customer
// and fleet health --customer left the rows out with zero unattributed and
// incomplete false.
func TestMissingAncestorIsUnattributedInEveryView(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.DogfoodEnvVar, "")
	now := time.Now()
	rows := map[string][]map[string]any{
		"device-groups": append(exposureFixtureGroups(),
			map[string]any{"uuid": "site-paged", "displayName": "New site", "parentGroupUuid": "parent-not-yet-synced", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_SITE"}),
		"devices": append(exposureFixtureDevices(now),
			map[string]any{"uuid": "dev-paged", "displayName": "PAGED-PC", "parentGroupUuid": "site-paged",
				"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * reportCustomerDay).Format(time.RFC3339)}),
		"detections": {{"uuid": "det-paged", "displayName": "Win32/Agent.XYZ", "typeName": "Trojan", "occurTime": reportCustomerAgo(now, 2*reportCustomerDay),
			"severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": "dev-paged"}, "objectHashSha1": "3395856ce81f2b7382dee72602f798b642f14140"}},
		"incidents": {{"uuid": "inc-paged", "displayName": "Ransomware behavior", "deviceUuids": []string{"dev-paged"}, "status": "INCIDENT_STATUS_OPEN",
			"severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "createTime": reportCustomerAgo(now, reportCustomerDay), "updateTime": reportCustomerAgo(now, reportCustomerDay)}},
	}
	db := exposureSeedDB(t, rows)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposureScopedPage(t,
			exposureVulnJSON("dev-paged", "site-paged", "Google Chrome", "CVE-2024-3400", 70, "SEVERITY_LEVEL_HIGH", "")),
		"/v1/device-patches":           exposurePage(t, false),
		"/v1/patching-process-details": exposurePage(t, false),
		"/v1/scans/recent":             exposurePage(t, false),
	}}
	exposureStub(t, &reportCustomerNewLive, fake)

	view, _ := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if d := view.Detections; d == nil || d.UnattributedDetections != 1 || !reflect.DeepEqual(d.UnattributedDetectionUUIDs, []string{"det-paged"}) || !d.Incomplete {
		t.Fatalf("report customer detections = %+v; want det-paged unattributed and the section incomplete", d)
	}
	if in := view.Incidents; in == nil || in.UnattributedOpen != 1 || !in.Incomplete {
		t.Fatalf("report customer incidents = %+v; want inc-paged unattributed and open", in)
	}
	if h := view.Health; h == nil || h.UnattributedDevices != 1 || !reflect.DeepEqual(h.UnattributedDeviceUUIDs, []string{"dev-paged"}) {
		t.Fatalf("report customer health = %+v; want dev-paged unattributed", h)
	}
	if v := view.Vulnerabilities; v == nil || v.UnattributedRows != 1 || !v.Incomplete {
		t.Fatalf("report customer vulnerabilities = %+v; want the site-paged row unattributed", v)
	}

	out, err := fleetStaleTestRun(t, "triage", "--customer", "Acme Dental", "--db", db, "--since", "7d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var tv triageView
	fleetHealthTestDecode(t, []byte(out), &tv)
	if tv.UnattributedDetections != 1 || tv.UnattributedIncidents != 1 {
		t.Fatalf("triage --customer = %+v; want det-paged and inc-paged unattributed", tv)
	}
	out, err = fleetStaleTestRun(t, "fleet", "health", "--db", db, "--customer", "Acme Dental", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var health fleetHealthView
	fleetHealthTestDecode(t, []byte(out), &health)
	if health.UnattributedDevices != 1 {
		t.Fatalf("fleet health --customer unattributed = %d, want dev-paged", health.UnattributedDevices)
	}
	out, err = fleetStaleTestRun(t, "fleet", "health", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	health = fleetHealthView{}
	fleetHealthTestDecode(t, []byte(out), &health)
	for _, r := range health.Customers {
		if r.Customer == esetCustomerNone && r.Devices > 0 {
			t.Fatalf("a device under a missing ancestor was filed as %s: %+v", esetCustomerNone, health.Customers)
		}
	}
	vulns := exposureDecodeFixture(t, exposureVulnJSON("dev-paged", "site-paged", "Zoom", "CVE-2024-1111", 30, "SEVERITY_LEVEL_LOW", ""))
	var groups []esetGroup
	var devices []esetDevice
	exposureRoundTrip(t, rows["device-groups"], &groups)
	exposureRoundTrip(t, rows["devices"], &devices)
	scope := newExposureScope(groups, devices)
	kept, other, unknown := exposureFilterRows(vulns, exposureVulnKey, scope, "acme")
	if len(kept) != 0 || other != 0 || len(unknown) != 1 {
		t.Fatalf("exposure --customer: kept %d other %d unknown %d; want the row unknown", len(kept), other, len(unknown))
	}
	if b, _ := json.Marshal(scope.customerFor("site-paged", "dev-paged")); !strings.Contains(string(b), esetCustomerParentHidden) {
		t.Fatalf("customerFor(site-paged) = %s", b)
	}
}

// TestHiddenScopeRootIsUnassignedInEveryView is adversarial round 4's
// scoped-API-user tree end to end: ESET returns the MSP group (parent: a
// root the API user cannot see) and the customers under it, and the MSP's
// own jump box sits directly in the MSP group with a detection and an open
// incident. Before the fix the walk treated the hidden root as a missing
// node, so every customer view marked its sections incomplete and listed the
// jump box's rows as possibly the customer's, fleet health filed it under
// "(group not in synced tree)", and devices profile printed that beside a
// group path the mirror does hold. A site whose parent company is not yet
// synced (a missing interior node) must still be unattributed.
func TestHiddenScopeRootIsUnassignedInEveryView(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.DogfoodEnvVar, "")
	now := time.Now()
	groups := exposureFixtureGroups()
	groups[0]["parentGroupUuid"] = "scope-root-not-returned"
	// A static group straight under the hidden root, outside the MSP group:
	// only the scope-root rule (not the MSP-type rule) proves it.
	groups = append(groups, map[string]any{"uuid": "lost", "displayName": "Lost and found", "parentGroupUuid": "scope-root-not-returned"})
	devices := append(exposureFixtureDevices(now),
		map[string]any{"uuid": "dev-jump", "displayName": "MSP-JUMP-06", "parentGroupUuid": "msp",
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED", "lastSyncTime": now.Add(-40 * reportCustomerDay).Format(time.RFC3339)},
		map[string]any{"uuid": "dev-lost", "displayName": "SPARE-07", "parentGroupUuid": "lost",
			"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": now.Add(-40 * reportCustomerDay).Format(time.RFC3339)})
	rows := map[string][]map[string]any{
		"device-groups": groups,
		"devices":       devices,
		"detections": {{"uuid": "det-jump", "displayName": "Win32/Agent.XYZ", "typeName": "Trojan", "occurTime": reportCustomerAgo(now, 2*reportCustomerDay),
			"severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": "dev-jump"}, "objectHashSha1": "3395856ce81f2b7382dee72602f798b642f14140"}},
		"incidents": {{"uuid": "inc-jump", "displayName": "Ransomware behavior", "deviceUuids": []string{"dev-jump"}, "status": "INCIDENT_STATUS_OPEN",
			"severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "createTime": reportCustomerAgo(now, reportCustomerDay), "updateTime": reportCustomerAgo(now, reportCustomerDay)}},
	}
	db := exposureSeedDB(t, rows)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposurePage(t, false),
		"/v1/device-patches":           exposurePage(t, false),
		"/v1/patching-process-details": exposurePage(t, false),
		"/v1/scans/recent":             exposurePage(t, false),
	}}
	exposureStub(t, &reportCustomerNewLive, fake)

	view, errOut := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if view.UnattributedRows != 0 || strings.Contains(errOut, "could not be tied") {
		t.Fatalf("report customer unattributed_rows = %d (stderr %q); the MSP's jump box is no customer's", view.UnattributedRows, errOut)
	}
	if h := view.Health; h == nil || h.UnattributedDevices != 0 || h.Incomplete {
		t.Fatalf("report customer health = %+v; want complete, nothing unattributed", h)
	}
	if d := view.Detections; d == nil || d.UnattributedDetections != 0 || d.Incomplete {
		t.Fatalf("report customer detections = %+v; want complete, nothing unattributed", d)
	}
	if in := view.Incidents; in == nil || in.UnattributedOpen != 0 || in.Incomplete {
		t.Fatalf("report customer incidents = %+v; want complete, nothing unattributed", in)
	}

	out, err := fleetStaleTestRun(t, "triage", "--customer", "Acme Dental", "--db", db, "--since", "7d", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var tv triageView
	fleetHealthTestDecode(t, []byte(out), &tv)
	if tv.UnattributedDetections != 0 || tv.UnattributedIncidents != 0 {
		t.Fatalf("triage --customer = %+v; want nothing unattributed", tv)
	}
	out, err = fleetStaleTestRun(t, "fleet", "health", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var health fleetHealthView
	fleetHealthTestDecode(t, []byte(out), &health)
	unassigned := 0
	for _, r := range health.Customers {
		if esetOwnerUnknownBucket(r.Customer) {
			t.Fatalf("fleet health filed a device as owner-unknown: %+v", health.Customers)
		}
		if r.Customer == esetCustomerNone {
			unassigned = r.Devices
		}
	}
	if unassigned != 2 || health.UnattributedDevices != 0 {
		t.Fatalf("fleet health: %d device(s) in %s, %d unattributed; want the jump box and the spare unassigned", unassigned, esetCustomerNone, health.UnattributedDevices)
	}
	out, err = fleetStaleTestRun(t, "fleet", "gaps", "--db", db, "--customer", "Acme Dental", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var gaps fleetGapsView
	fleetHealthTestDecode(t, []byte(out), &gaps)
	if gaps.UnattributedDevices != 0 || gaps.UncertainGaps != 0 {
		t.Fatalf("fleet gaps --customer = %+v; want nothing unattributed or uncertain", gaps)
	}
	var gs []esetGroup
	exposureRoundTrip(t, groups, &gs)
	var ds []esetDevice
	exposureRoundTrip(t, devices, &ds)
	for _, d := range ds {
		if d.UUID != "dev-jump" {
			continue
		}
		if v := devicesProfileDeviceView(d, gs); v.Customer != esetCustomerNone || v.GroupPath != "Servosity MSP" {
			t.Fatalf("devices profile MSP-JUMP-06: customer %q group_path %q; want %q under Servosity MSP", v.Customer, v.GroupPath, esetCustomerNone)
		}
	}

	// A site paged in before its new company is a missing interior node:
	// the mirror now names two missing parents, so neither is the scope
	// root; the jump box stays known through its MSP group, the site's
	// device and the spare under the (now unprovable) root are unattributed.
	paged := append(append([]map[string]any(nil), groups...),
		map[string]any{"uuid": "site-paged", "displayName": "New site", "parentGroupUuid": "company-not-yet-synced", "linkedEntityType": "DEVICE_GROUP_ENTITY_TYPE_SITE"})
	pagedDevices := append(append([]map[string]any(nil), devices...),
		map[string]any{"uuid": "dev-paged", "displayName": "PAGED-PC", "parentGroupUuid": "site-paged", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK"})
	db2 := exposureSeedDB(t, map[string][]map[string]any{"device-groups": paged, "devices": pagedDevices})
	out, err = fleetStaleTestRun(t, "fleet", "health", "--db", db2, "--json")
	if err != nil {
		t.Fatal(err)
	}
	health = fleetHealthView{}
	fleetHealthTestDecode(t, []byte(out), &health)
	if health.UnattributedDevices != 2 || !reflect.DeepEqual(health.UnattributedDeviceUUIDs, []string{"dev-lost", "dev-paged"}) {
		t.Fatalf("fleet health with a missing interior node: unattributed %d %v; want dev-lost and dev-paged", health.UnattributedDevices, health.UnattributedDeviceUUIDs)
	}
}
