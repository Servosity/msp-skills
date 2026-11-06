// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// The documented AWS answer shapes the read paths must not misread, against
// the fake (quiet: it repeats no credential here):
//   - ce-dimension-values-paged: GetDimensionValues answers in two pages;
//     dimensions lists the values of both;
//   - inventory-failures-reported: a CloudWatch GetMetricStatistics failure
//     leaves the idle check undone, so sync records the inventory layer as
//     failed and waste does not report the running instance as checked idle;
//   - inventory-scan-replaces-scope: a volume the second complete scan no
//     longer returns is no longer listed as waste.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDimensionsReadsEveryPage(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.quiet = true
	out := e.run(t, "dimensions", "SERVICE", "--json")
	if !strings.Contains(out, "HQ-Dimension-Page2") || !strings.Contains(out, `"HQ-Dimension"`) {
		t.Errorf("dimensions did not list both pages:\n%.1500s", out)
	}
	if n := e.aws.count("GetDimensionValues"); n != 2 {
		t.Errorf("want two GetDimensionValues calls (one per page), got %d", n)
	}
}

// syncLayers is sync --json's per-layer outcome.
func syncLayers(t *testing.T, out string) map[string]bool {
	t.Helper()
	var sum struct {
		Layers []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"layers"`
	}
	start := strings.Index(out, "{")
	if start < 0 || json.NewDecoder(strings.NewReader(out[start:])).Decode(&sum) != nil || len(sum.Layers) == 0 {
		t.Fatalf("sync --json printed no layer outcomes:\n%.1500s", out)
	}
	layers := map[string]bool{}
	for _, l := range sum.Layers {
		layers[l.Name] = l.OK
	}
	return layers
}

func TestCloudWatchFailureFailsTheInventoryLayer(t *testing.T) {
	t.Parallel()
	for _, cwDeny := range []bool{false, true} {
		e := newEnv(t, srcEnv)
		e.aws.set(func(f *fakeAWS) { f.quiet, f.cwDeny = true, cwDeny })
		layers := syncLayers(t, e.run(t, "sync", "--json"))
		idle := e.run(t, "waste", "idle", "--json")
		if e.aws.count("GetMetricStatistics") == 0 {
			t.Fatalf("cwDeny=%v: sync never asked CloudWatch: %v", cwDeny, e.aws.seen())
		}
		if cwDeny {
			if ok, seen := layers["inventory"]; !seen || ok {
				t.Errorf("CloudWatch refused, yet the inventory layer reads as synced: %v", layers)
			}
			if strings.Contains(idle, "i-0hq0001") && strings.Contains(idle, "avg CPU") {
				t.Errorf("CloudWatch refused, yet waste reports the instance as checked idle:\n%.1500s", idle)
			}
			continue
		}
		// Control: CloudWatch answers 1.5% average CPU; the layer syncs and
		// the instance is idle waste.
		if !layers["inventory"] {
			t.Errorf("control: the inventory layer failed: %v", layers)
		}
		if !strings.Contains(idle, "i-0hq0001") || !strings.Contains(idle, "avg CPU") {
			t.Errorf("control: waste idle does not list the idle instance:\n%.1500s", idle)
		}
	}
}

func TestSecondSyncPrunesAVolumeAWSNoLongerReturns(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	e.aws.quiet = true
	if layers := syncLayers(t, e.run(t, "sync", "--json")); !layers["inventory"] {
		t.Fatalf("the first sync's inventory layer failed: %v", layers)
	}
	if rank := e.run(t, "waste", "rank", "--json"); !strings.Contains(rank, "vol-0hq0001") {
		t.Fatalf("the first sync stored no vol-0hq0001; the test did not run:\n%.1500s", rank)
	}
	e.aws.set(func(f *fakeAWS) { f.hideVol = "vol-0hq0001" })
	if layers := syncLayers(t, e.run(t, "sync", "--json")); !layers["inventory"] {
		t.Fatalf("the second sync's inventory layer failed: %v", layers)
	}
	for _, args := range [][]string{{"waste", "rank", "--json"}, {"waste", "gp2-gp3", "--json"}} {
		if out := e.run(t, args...); strings.Contains(out, "vol-0hq0001") {
			t.Errorf("%v still lists the volume AWS no longer returns:\n%.1500s", args, out)
		}
	}
	// The rest of the scan is still there.
	if rank := e.run(t, "waste", "rank", "--json"); !strings.Contains(rank, "snap-0hq0001") {
		t.Errorf("the second sync lost the rows AWS still returns:\n%.1500s", rank)
	}
}
