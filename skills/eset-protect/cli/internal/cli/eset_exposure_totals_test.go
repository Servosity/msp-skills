// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestExposureTotalsAgreeWithUnattributedRows (adversarial round 4, P2):
// under --customer the totals were built from the kept rows only, so
// totals.unknown_owner_devices read 0 beside unattributed_rows 1. On random
// mirrors (missing groups, cycles, hidden roots, MSP groups), random
// vulnerability rows (some on devices the mirror lacks, some with no group
// of their own, some not matching the CVE) and every target (each COMPANY
// and none), totals.unknown_owner_devices is the number of distinct devices
// of matching owner-unknown rows, exactly the unattributed_device_uuids.
func TestExposureTotalsAgreeWithUnattributedRows(t *testing.T) {
	r := rand.New(rand.NewSource(404))
	q := exposureQuery{CVEs: []string{"CVE-2024-3400"}}
	for trial := 0; trial < 2000; trial++ {
		synced, ids := ancestryGen(r)
		var devices []esetDevice
		for i, id := range ids {
			devices = append(devices, esetDevice{UUID: fmt.Sprintf("d%d", i), ParentGroupUUID: id})
		}
		scope := newExposureScope(synced, devices)
		var rows []map[string]any
		for i := 0; i < 1+r.Intn(12); i++ {
			dev := fmt.Sprintf("d%d", r.Intn(len(ids)+2)) // some not in the mirror
			group := ""
			if r.Intn(2) == 0 {
				group = ids[r.Intn(len(ids))]
			}
			cve := "CVE-2024-3400"
			if r.Intn(5) == 0 {
				cve = "CVE-1999-0001"
			}
			rows = append(rows, exposureVulnJSON(dev, group, "Google Chrome", cve, 50, "SEVERITY_LEVEL_HIGH", ""))
		}
		vulns := exposureDecodeFixture(t, rows...)
		for _, target := range append(scope.tree.companies(), esetGroup{}) {
			var view exposureView
			exposureAssemble(&view, q, vulns, scope, target.UUID)
			want := map[string]bool{}
			for _, v := range vulns {
				if !exposureMatch(q, v) {
					continue
				}
				if _, known := scope.owner(v.DeviceGroupUUID, v.DeviceUUID); !known && v.DeviceUUID != "" {
					want[v.DeviceUUID] = true
				}
			}
			var wantIDs []string
			for id := range want {
				wantIDs = append(wantIDs, id)
			}
			sort.Strings(wantIDs)
			if view.Totals.UnknownOwnerDevices != len(wantIDs) || len(view.UnattributedDeviceUUIDs) != len(wantIDs) {
				t.Fatalf("trial %d target %q: totals.unknown_owner_devices %d, unattributed_device_uuids %v, want %v\ntree %+v",
					trial, target.UUID, view.Totals.UnknownOwnerDevices, view.UnattributedDeviceUUIDs, wantIDs, synced)
			}
		}
	}
}

// TestHelpNeverFilesOwnerUnknownRowsAsOtherCustomers (adversarial round 4,
// P2): exposure and patches backlog --help said an owner-unknown row is
// "also counted in other_customer_rows", a field whose note says the rows
// belong to another customer; since rounds 2 and 3 they are counted in
// unattributed_rows. No command's help may say so, and the two help texts
// name unattributed_rows.
func TestHelpNeverFilesOwnerUnknownRowsAsOtherCustomers(t *testing.T) {
	var walk func(c *cobra.Command)
	seen := map[string]string{}
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		long := strings.Join(strings.Fields(c.Long), " ")
		seen[c.CommandPath()] = long
		if strings.Contains(long, "also counted in other_customer_rows") {
			t.Errorf("%s --help files owner-unknown rows under other_customer_rows", c.CommandPath())
		}
	}
	walk(RootCmd())
	for _, path := range []string{esetCLIName + " exposure", esetCLIName + " patches backlog"} {
		long, ok := seen[path]
		if !ok {
			t.Fatalf("no %s command", path)
		}
		if !strings.Contains(long, "counted in unattributed_rows") || !strings.Contains(long, "never in other_customer_rows") {
			t.Errorf("%s --help does not say owner-unknown rows are counted in unattributed_rows: %s", path, long)
		}
	}
}
