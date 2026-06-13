// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored regression tests for two sync-resource-scoping hand-fixes:
//   - flatSyncResources: a dependent-resource name passed via --resources must
//     not be enqueued as a flat resource (it has no flat endpoint and would
//     fail, aborting the run under --strict). See handfixes.json:
//     sync-skip-dependent-flat-names.
//   - describeFailedResources: a --strict failure must NAME the failing
//     resource, not just count it. See handfixes.json:strict-error-names-resources.

package cli

import (
	"strings"
	"testing"
)

func TestFlatSyncResources_DropsDependentNames(t *testing.T) {
	// A user may name a dependent explicitly (e.g. `--resources clients,device,
	// autoverify`, which older docs/other tooling may suggest); the filter must
	// drop it from the flat enqueue while the dependent pass still runs it.
	got := flatSyncResources([]string{"clients", "device", "autoverify"})
	want := []string{"clients", "device"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("flatSyncResources = %v, want %v (dependent autoverify must be dropped from flat enqueue)", got, want)
	}

	// Every dependent name is filtered; flat names always survive.
	for _, dep := range dependentResourceDefs() {
		out := flatSyncResources([]string{"device", dep.Name})
		for _, r := range out {
			if r == dep.Name {
				t.Fatalf("flatSyncResources kept dependent %q in the flat list: %v", dep.Name, out)
			}
		}
	}

	// A dependent named alone leaves an empty flat list (dependent pass still
	// runs it via parentFilter against an already-synced parent table).
	if out := flatSyncResources([]string{"autoverify"}); len(out) != 0 {
		t.Fatalf("flatSyncResources([autoverify]) = %v, want [] (no flat work)", out)
	}

	// Unknown-but-flat names are NOT filtered (only known dependents are).
	if out := flatSyncResources([]string{"device", "user"}); strings.Join(out, ",") != "device,user" {
		t.Fatalf("flatSyncResources dropped a flat resource: %v", out)
	}
}

// TestSyncResourcePath_RejectsDependentNames documents the root cause the
// filter guards against: a dependent name has no flat path.
func TestSyncResourcePath_RejectsDependentNames(t *testing.T) {
	for _, dep := range dependentResourceDefs() {
		if _, err := syncResourcePath(dep.Name); err == nil {
			t.Fatalf("syncResourcePath(%q) returned nil error; a dependent name must not resolve to a flat path (this is why flatSyncResources must drop it)", dep.Name)
		}
	}
}

func TestDescribeFailedResources_NamesResources(t *testing.T) {
	if got := describeFailedResources(nil); got != "" {
		t.Fatalf("describeFailedResources(nil) = %q, want empty", got)
	}
	if got := describeFailedResources([]string{"autoverify"}); got != ": autoverify" {
		t.Fatalf("describeFailedResources([autoverify]) = %q, want %q", got, ": autoverify")
	}
	if got := describeFailedResources([]string{"autoverify", "restore_point"}); got != ": autoverify, restore_point" {
		t.Fatalf("describeFailedResources(two) = %q, want %q", got, ": autoverify, restore_point")
	}
}
