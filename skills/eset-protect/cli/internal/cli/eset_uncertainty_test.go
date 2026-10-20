// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// gapsRefRule is the plain peer rule (no unknown owners) for device m in the
// peer group g: the pre-round-3 algorithm, used as the oracle.
func gapsRefRule(g []esetDevice, m esetDevice, threshold float64) map[string]bool {
	out := map[string]bool{}
	comps := func(d esetDevice) map[string]esetComponent { return fleetGapsComponents(d) }
	mine := comps(m)
	if len(g) >= fleetGapsMinGroupSize {
		prev := map[string]int{}
		for _, p := range g {
			if p.UUID == m.UUID {
				continue
			}
			for name := range comps(p) {
				prev[name]++
			}
		}
		for name, n := range prev {
			if _, has := mine[name]; !has && float64(n)/float64(len(g)-1)+1e-9 >= threshold {
				out[fleetGapsMissing+"|"+name] = true
			}
		}
	}
	newest := map[string][]int64{}
	for _, p := range g {
		for name, c := range comps(p) {
			k := fleetGapsVersionKey(c.Version)
			if cur, ok := newest[name]; !ok || fleetGapsVersionCmp(cur, k) < 0 {
				newest[name] = k
			}
		}
	}
	for name, c := range mine {
		hk := fleetGapsVersionKey(c.Version)
		if hk != nil && newest[name] != nil && fleetGapsVersionCmp(hk, newest[name]) < 0 {
			out[fleetGapsOutdated+"|"+name] = true
		}
	}
	return out
}

// TestFleetGapsUncertaintyMatchesEveryPlacement brute-forces fleet gaps in
// the customer scope: for each device of a known customer, the reference
// tries every subset of the unknown-owner devices as extra peers. A gap is
// listed exactly when some subset produces it, and marked uncertain exactly
// when not every subset does. Unknown-owner devices get only the gaps that
// need no peers, and are never pooled into a peer group of their own.
func TestFleetGapsUncertaintyMatchesEveryPlacement(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	thresholds := []float64{0.3, 0.5, 0.75, 1}
	for trial := 0; trial < 4000; trial++ {
		groups := []esetGroup{{UUID: "root", Name: "MSP"}}
		var devices []esetDevice
		nCust := 1 + r.Intn(3)
		for c := 0; c < nCust; c++ {
			groups = append(groups, esetGroup{UUID: fmt.Sprintf("c%d", c), Name: fmt.Sprintf("Customer %d", c), ParentUUID: "root", EntityType: esetCompanyEntityType})
		}
		newDevice := func(id, group string) esetDevice {
			d := esetDevice{UUID: id, ParentGroupUUID: group}
			for _, name := range []string{"A", "B", "C"} {
				if r.Intn(2) == 0 {
					d.Components = append(d.Components, esetComponent{Name: name, Version: esetVersion{Major: flexInt(1 + r.Intn(3))}})
				}
			}
			d.ActiveProducts = []json.RawMessage{json.RawMessage(`{"id":1}`)}
			return d
		}
		for c := 0; c < nCust; c++ {
			for i := r.Intn(5); i > 0; i-- {
				devices = append(devices, newDevice(fmt.Sprintf("d%d-%d", c, i), fmt.Sprintf("c%d", c)))
			}
		}
		var unknown []esetDevice
		for i := r.Intn(4); i > 0; i-- {
			d := newDevice(fmt.Sprintf("u%d", i), "group-after-sync")
			if r.Intn(3) == 0 {
				d.ActiveProducts = nil
			}
			unknown = append(unknown, d)
			devices = append(devices, d)
		}
		threshold := thresholds[r.Intn(len(thresholds))]
		tree := newEsetTree(groups)
		kinds, _ := fleetGapsParseOnly("")
		res := fleetGapsFind(devices, tree, fleetGapsOptions{Kinds: kinds, Threshold: threshold})

		got := map[string]map[string]bool{} // device -> "gap|component" -> uncertain
		for _, g := range res.Gaps {
			if got[g.DeviceUUID] == nil {
				got[g.DeviceUUID] = map[string]bool{}
			}
			got[g.DeviceUUID][g.Gap+"|"+g.Component] = g.Uncertain
		}
		for c := 0; c < nCust; c++ {
			var members []esetDevice
			for _, d := range devices {
				if d.ParentGroupUUID == fmt.Sprintf("c%d", c) {
					members = append(members, d)
				}
			}
			for _, m := range members {
				possible := map[string]int{}
				for mask := 0; mask < 1<<len(unknown); mask++ {
					g := append([]esetDevice(nil), members...)
					for i, u := range unknown {
						if mask&(1<<i) != 0 {
							g = append(g, u)
						}
					}
					for k := range gapsRefRule(g, m, threshold) {
						possible[k]++
					}
				}
				want := map[string]bool{}
				for k, n := range possible {
					want[k] = n != 1<<len(unknown)
				}
				have := map[string]bool{}
				for k, u := range got[m.UUID] {
					if strings.HasPrefix(k, fleetGapsMissing+"|") || strings.HasPrefix(k, fleetGapsOutdated+"|") {
						have[k] = u
					}
				}
				if fmt.Sprint(sortedGapKeys(have)) != fmt.Sprint(sortedGapKeys(want)) {
					t.Fatalf("trial %d threshold %v device %s: gaps %v, want %v (unknown %d, members %d)", trial, threshold, m.UUID, have, want, len(unknown), len(members))
				}
			}
		}
		for _, u := range unknown {
			for k := range got[u.UUID] {
				if !strings.HasPrefix(k, fleetGapsUnlicensed+"|") && !strings.HasPrefix(k, fleetGapsUnenrolled+"|") {
					t.Fatalf("trial %d: an unknown-owner device got a peer gap %q: it has no known peer group", trial, k)
				}
			}
			if len(u.ActiveProducts) == 0 && got[u.UUID][fleetGapsUnlicensed+"|"] {
				t.Fatalf("an unlicensed unknown-owner device's gap must not be uncertain")
			}
			if len(u.ActiveProducts) == 0 {
				if _, ok := got[u.UUID][fleetGapsUnlicensed+"|"]; !ok {
					t.Fatalf("trial %d: an unlicensed unknown-owner device was not reported", trial)
				}
			}
		}
		if res.Unattributed.N != len(unknown) {
			t.Fatalf("unattributed = %d, want %d", res.Unattributed.N, len(unknown))
		}
	}
}

func sortedGapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(out)
	return out
}

// TestSpreadThresholdsMatchEveryAttribution brute-forces detections spread's
// --min-customers / --min-devices decision. The reference enumerates every
// attribution of the unknown-owner devices (each joins a known customer or a
// new one) and of device-less detections (each sits on a counted device or a
// new one, owned by a known or new customer): a group is listed exactly when
// some attribution meets both thresholds, and thresholds_proven is true
// exactly when every attribution does. customers_known never counts an
// unknown owner. Round 5: some known devices are the MSP's own (directly in
// the root group, "(unassigned)"), which is never a customer, and every
// unknown owner may be the MSP's own too.
func TestSpreadThresholdsMatchEveryAttribution(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	when := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for trial := 0; trial < 2000; trial++ {
		nCust := 1 + r.Intn(3)
		groups := []esetGroup{{UUID: "root", Name: "MSP"}}
		for c := 0; c < nCust; c++ {
			groups = append(groups, esetGroup{UUID: fmt.Sprintf("c%d", c), Name: fmt.Sprintf("C%d", c), ParentUUID: "root", EntityType: esetCompanyEntityType})
		}
		var devices []esetDevice
		var raw []json.RawMessage
		knownOwner := map[string]int{} // device -> customer index, -1 for the MSP's own
		nKnown := r.Intn(4)
		for i := 0; i < nKnown; i++ {
			c := r.Intn(nCust + 1)
			id := fmt.Sprintf("k%d", i)
			group := fmt.Sprintf("c%d", c)
			if c == nCust {
				c, group = -1, "root"
			}
			devices = append(devices, esetDevice{UUID: id, ParentGroupUUID: group})
			knownOwner[id] = c
			raw = append(raw, json.RawMessage(detectionsSpreadTestDetection("det-"+id, detectionsSpreadTestHashB, "T", "a.exe", id, "SEVERITY_LEVEL_HIGH", when)))
		}
		nUnknown := r.Intn(3)
		for i := 0; i < nUnknown; i++ {
			id := fmt.Sprintf("u%d", i)
			if r.Intn(2) == 0 {
				devices = append(devices, esetDevice{UUID: id, ParentGroupUUID: "group-after-sync"})
			}
			raw = append(raw, json.RawMessage(detectionsSpreadTestDetection("det-"+id, detectionsSpreadTestHashB, "T", "a.exe", id, "SEVERITY_LEVEL_HIGH", when)))
		}
		nDeviceless := r.Intn(3)
		for i := 0; i < nDeviceless; i++ {
			raw = append(raw, json.RawMessage(detectionsSpreadTestDetection(fmt.Sprintf("det-x%d", i), detectionsSpreadTestHashB, "T", "a.exe", "", "SEVERITY_LEVEL_HIGH", when)))
		}
		if len(raw) == 0 {
			continue
		}
		opts := detectionsSpreadOptions{MinCustomers: r.Intn(5), MinDevices: r.Intn(5)}

		// Reference: enumerate attributions.
		devIDs := []string{}
		for i := 0; i < nKnown; i++ {
			devIDs = append(devIDs, fmt.Sprintf("k%d", i))
		}
		for i := 0; i < nUnknown; i++ {
			devIDs = append(devIDs, fmt.Sprintf("u%d", i))
		}
		// Owner choices: the MSP's own (-1, never a customer), a known
		// customer index, or a new customer (nCust+j).
		ownerChoices := nCust + nUnknown + nDeviceless
		// Device choices for a device-less detection: an existing device or
		// a new one (len(devIDs)+j).
		devChoices := len(devIDs) + nDeviceless
		total, met, proven := 0, false, true
		var rec func(i int, owners []int, devs []int, newOwners []int)
		rec = func(i int, owners []int, devs []int, newOwners []int) {
			switch {
			case i < nUnknown:
				for o := -1; o < ownerChoices; o++ {
					rec(i+1, append(owners, o), devs, newOwners)
				}
				return
			case i < nUnknown+nDeviceless:
				for d := 0; d < devChoices; d++ {
					if d < len(devIDs) {
						// On a counted device: its owner is that device's.
						rec(i+1, owners, append(devs, d), append(newOwners, -1))
						continue
					}
					for o := -1; o < ownerChoices; o++ {
						rec(i+1, owners, append(devs, d), append(newOwners, o))
					}
				}
				return
			}
			custs := map[int]bool{}
			addCust := func(o int) {
				if o >= 0 {
					custs[o] = true
				}
			}
			devSet := map[int]bool{}
			for i := 0; i < nKnown; i++ {
				addCust(knownOwner[fmt.Sprintf("k%d", i)])
				devSet[i] = true
			}
			for i := 0; i < nUnknown; i++ {
				addCust(owners[i])
				devSet[nKnown+i] = true
			}
			for j, d := range devs {
				devSet[d] = true
				switch {
				case d < nKnown:
					addCust(knownOwner[fmt.Sprintf("k%d", d)])
				case d < len(devIDs):
					addCust(owners[d-nKnown])
				default:
					addCust(newOwners[j])
				}
			}
			total++
			ok := len(custs) >= opts.MinCustomers && len(devSet) >= opts.MinDevices
			met = met || ok
			proven = proven && ok
		}
		rec(0, nil, nil, nil)

		res := detectionsSpreadBuild(raw, devices, newEsetTree(groups), nil, opts, nil)
		if len(res.Groups) > 1 {
			t.Fatalf("one hash must be one group: %+v", res.Groups)
		}
		listed := len(res.Groups) == 1
		if listed != met {
			t.Fatalf("trial %d: listed %v, want %v (known %d unknown %d deviceless %d, opts %+v): %+v", trial, listed, met, nKnown, nUnknown, nDeviceless, opts, res.Groups)
		}
		if listed {
			g := res.Groups[0]
			if g.ThresholdsProven != proven {
				t.Fatalf("trial %d: thresholds_proven %v, want %v (%+v, opts %+v)", trial, g.ThresholdsProven, proven, g, opts)
			}
			knownCusts := map[int]bool{}
			for _, c := range knownOwner {
				if c >= 0 {
					knownCusts[c] = true
				}
			}
			if g.CustomersKnown != len(knownCusts) || g.DevicesUnknownOwner != nUnknown+nDeviceless || g.CustomersPossible != len(knownCusts)+nUnknown+nDeviceless {
				t.Fatalf("trial %d: counts %+v, want known %d unknown %d", trial, g, len(knownCusts), nUnknown+nDeviceless)
			}
		}
		if res.Unattributed.N != nUnknown+nDeviceless {
			t.Fatalf("trial %d: unattributed %d, want %d before any threshold", trial, res.Unattributed.N, nUnknown+nDeviceless)
		}
	}
}

// TestSpreadReportsCrossCustomerSpreadOfDeletedDevices is the Codex round-3
// scenario: infected endpoints of two customers were deleted and devices
// fully resynced, so their detections name devices the mirror does not hold.
// They used to collapse into one "(unknown device)" customer, and
// --min-customers 2 printed "No detections matched" with
// unattributed_detections 0.
func TestSpreadReportsCrossCustomerSpreadOfDeletedDevices(t *testing.T) {
	testenv.Isolate(t)
	ago := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	path := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme Dental","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"beta","displayName":"Beta Law","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices": {`{"uuid":"a1","displayName":"FRONTDESK-01","parentGroupUuid":"acme"}`},
		"detections": {
			detectionsSpreadTestDetection("det-gone-acme", detectionsSpreadTestHashB, "Trojan", "a.exe", "f00dbabe-0000-4000-8000-0000000000a1", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det-gone-beta", detectionsSpreadTestHashB, "Trojan", "a.exe", "f00dbabe-0000-4000-8000-0000000000b1", "SEVERITY_LEVEL_HIGH", ago),
		},
	})
	v := detectionsSpreadTestRun(t, path, "--min-customers", "2")
	if len(v.Groups) != 1 || v.UnattributedDetections != 2 || v.GroupsUnproven != 1 {
		t.Fatalf("--min-customers 2 must list the unresolved spread: %+v", v)
	}
	g := v.Groups[0]
	if g.CustomersKnown != 0 || g.DevicesUnknownOwner != 2 || g.CustomersPossible != 2 || g.ThresholdsProven || len(g.Customers) != 0 {
		t.Fatalf("group = %+v", g)
	}
	// --min-customers 3 cannot be reached by two devices: no group, but the
	// window's unknown-owner detections are still counted and printed.
	v = detectionsSpreadTestRun(t, path, "--min-customers", "3")
	if len(v.Groups) != 0 || v.UnattributedDetections != 2 {
		t.Fatalf("--min-customers 3 = %+v", v)
	}
	out, err := fleetHealthTestRun(t, "detections", "spread", "--db", path, "--min-customers", "3", "--human-friendly")
	if err != nil || !bytes.Contains(out, []byte("No detections matched")) || !bytes.Contains(out, []byte("2 detection(s) in the window could not be tied to any customer")) {
		t.Fatalf("an empty result must name the unknown-owner detections: %v\n%s", err, out)
	}
	out, err = fleetHealthTestRun(t, "detections", "spread", "--db", path, "--min-customers", "2", "--human-friendly")
	if err != nil || !bytes.Contains(out, []byte("0 (+2 unknown)")) || !bytes.Contains(out, []byte("thresholds_proven false")) {
		t.Fatalf("the human table must show the unknown owners: %v\n%s", err, out)
	}
}
