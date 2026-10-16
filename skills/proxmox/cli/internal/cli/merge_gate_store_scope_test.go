// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func storePathFor(cluster string) string {
	selectedCluster = cluster
	configureDefaultDBScope("")
	return defaultDBPath("proxmox-cli")
}

// hand-fix (per-cluster-store): a cluster name is not an identity. Another
// clusters file, or `clusters add --force`, can reuse "acme" for a different
// client, and that client must not open the first one's store.
func TestReusedClusterNameGetsItsOwnStore(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	first := storePathFor("acme")
	if again := storePathFor("acme"); again != first {
		t.Fatalf("one cluster entry must keep one store: %q then %q", first, again)
	}

	other := filepath.Join(t.TempDir(), "clusters.toml")
	body := strings.Replace(acmeClusters(f, f.pbs.URL, ""), `client = "Acme Dental"`, `client = "Other Clinic"`, 1)
	if err := os.WriteFile(other, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOX_CLUSTERS_FILE", other)
	if got := storePathFor("acme"); got == first {
		t.Fatalf("a second clusters file reusing the name shares the first tenant's store: %q", got)
	}

	// Same file, entry replaced (clusters add --force) with another endpoint.
	writeClustersFile(t, strings.Replace(acmeClusters(f, f.pbs.URL, ""), f.pve.URL, "https://192.0.2.10:8006", 1))
	replaced := storePathFor("acme")
	writeClustersFile(t, body)
	if replaced == storePathFor("acme") {
		t.Fatalf("replacing the entry's endpoint kept the old tenant's store: %q", replaced)
	}

	// The client label is display text: renaming it keeps local-only data
	// (taught notes, amended playbooks) that a sync cannot restore.
	labelled := storePathFor("acme")
	writeClustersFile(t, strings.Replace(body, `client = "Other Clinic"`, `client = "Other Clinic LLC"`, 1))
	if got := storePathFor("acme"); got != labelled {
		t.Fatalf("a client label edit moved the store: %q -> %q", labelled, got)
	}
}

// An entry with no endpoint to bind to (undefined, unreadable, or empty) never
// reopens a store an earlier invocation wrote: two tenants could each have
// owned that name.
func TestUnresolvedClusterNeverReopensAStore(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	saved := unresolvedStoreNonce
	defer func() { unresolvedStoreNonce = saved }()
	for _, body := range []string{"", "[clusters.acme]\nclient = \"Acme Dental\"\n"} {
		if body != "" {
			writeClustersFile(t, body)
		}
		first := storePathFor("acme")
		unresolvedStoreNonce = "next-invocation" // a later process
		if got := storePathFor("acme"); got == first {
			t.Fatalf("an unresolvable acme reopened an earlier invocation's store %q (clusters file %q)", got, body)
		}
		if storePathFor("beta") == storePathFor("acme") {
			t.Fatal("two unresolvable cluster names share one store")
		}
		unresolvedStoreNonce = saved
	}
	_ = f
}

// hand-fix (profile-store-scope): a saved run profile's --cluster is applied
// after the store was first scoped; the command must use that cluster's store.
func TestSavedProfileClusterGetsItsOwnStore(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	if err := saveProfileStore(&profileStore{Profiles: map[string]Profile{
		"acme-run": {Name: "acme-run", Values: map[string]string{"cluster": "acme"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runCLI(t, "--profile", "acme-run", "clusters", "list"); err != nil {
		t.Fatalf("clusters list via profile: %v\n%s", err, stderr)
	}
	got := defaultDBPath("proxmox-cli")
	if want := storePathFor("acme"); got != want {
		t.Fatalf("profile with --cluster acme used store %q, want acme's %q (default store is %q)", got, want, storePathFor(""))
	}
}
