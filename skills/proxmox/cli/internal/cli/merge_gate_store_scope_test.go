// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/cliutil"
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

// hand-fix (per-cluster-store): a malformed config.toml must not drop every
// cluster into the one unscoped store; the cluster scope is decided first.
func TestConfigErrorKeepsClusterStoresApart(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	bad := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(bad, []byte("this is = = not toml [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pathWithBadConfig := func(cluster string) string {
		selectedCluster = cluster
		configureDefaultDBScope(bad)
		return defaultDBPath("proxmox-cli")
	}
	acme, beta, none := pathWithBadConfig("acme"), pathWithBadConfig("beta"), pathWithBadConfig("")
	if acme == beta || acme == none || beta == none {
		t.Fatalf("a config error merged cluster stores: acme=%q beta=%q default=%q", acme, beta, none)
	}
	if acme != storePathFor("acme") {
		t.Fatalf("a config error changed acme's store: %q vs %q", acme, storePathFor("acme"))
	}
}

// hand-fix (cluster-store-no-fallback): with a cluster selected, a legacy
// data.db whose ownership is unproven (empty marker) never becomes the store.
func TestClusterStoreIgnoresLegacyUnscopedDB(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	unscoped := filepath.Join(dir, "data.db")
	for _, p := range []string{unscoped, unscoped + ".owner"} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	acme, beta := storePathFor("acme"), storePathFor("beta")
	if acme == unscoped || beta == unscoped || acme == beta {
		t.Fatalf("a selected cluster fell back to the shared store: acme=%q beta=%q unscoped=%q", acme, beta, unscoped)
	}
}

// hand-fix (cluster-store-no-fallback): help and parse-failure paths never
// run pre-run, so the path must follow the selected cluster on its own.
func TestClusterStoreWithoutPreRun(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	want := storePathFor("acme")
	selectedCluster = ""
	configureDefaultDBScope("") // pre-run state of a process that never scoped
	selectedCluster = "acme"    // what flag parsing sets before --help returns
	if got := learnDBPath(""); got != want {
		t.Fatalf("learn store without pre-run = %q, want acme's %q", got, want)
	}
}

// hand-fix (cluster-store-no-fallback): --db names a file any cluster could
// share, so a selected cluster refuses it, including from a saved profile.
func TestDBFlagRefusedWithCluster(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	shared := filepath.Join(t.TempDir(), "shared.db")
	_, _, err := runCLI(t, "--cluster", "acme", "learnings", "stats", "--db", shared)
	if err == nil || !strings.Contains(err.Error(), "--db cannot be combined") {
		t.Fatalf("--cluster with --db: err=%v", err)
	}
	selectedCluster = ""
	if err := saveProfileStore(&profileStore{Profiles: map[string]Profile{
		"shared": {Name: "shared", Values: map[string]string{"cluster": "acme", "db": shared}},
	}}); err != nil {
		t.Fatal(err)
	}
	_, _, err = runCLI(t, "--profile", "shared", "learnings", "stats")
	if err == nil || !strings.Contains(err.Error(), "--db cannot be combined") {
		t.Fatalf("profile cluster+db: err=%v", err)
	}
	selectedCluster = ""
	if _, _, err := runCLI(t, "--cluster", "acme", "learnings", "stats", "--db", " "); err == nil || !strings.Contains(err.Error(), "--db cannot be combined") {
		t.Fatalf("--cluster with a whitespace --db: err=%v", err)
	}
	selectedCluster = ""
	if _, stderr, err := runCLI(t, "learnings", "stats", "--db", shared); err != nil {
		t.Fatalf("--db without a cluster must still work: %v\n%s", err, stderr)
	}
}

// hand-fix (cluster-store-no-fallback): SQLite ends a DSN filename at '?' or
// '#', so a selected cluster refuses a store path that contains one.
func TestClusterStorePathWithQueryCharRefused(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	t.Setenv("PROXMOX_DATA_DIR", filepath.Join(t.TempDir(), "data?tenant"))
	selectedCluster = "acme"
	if !strings.Contains(defaultDBPath("proxmox-cli"), "?") {
		t.Skip("PROXMOX_DATA_DIR does not reach the store path")
	}
	if _, err := MCPStorePath(); err == nil {
		t.Fatal("MCPStorePath accepted a '?' store path for a selected cluster")
	}
	selectedCluster = ""
	if _, _, err := runCLI(t, "--cluster", "acme", "learnings", "stats"); err == nil || !strings.Contains(err.Error(), "contains '?' or '#'") {
		t.Fatalf("a '?' store path was not refused: err=%v", err)
	}
}

// A clusters entry with only a PBS namespace names no cluster, so it gets a
// per-process store like any other unresolvable entry.
func TestNamespaceOnlyEntryIsUnresolved(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	saved := unresolvedStoreNonce
	defer func() { unresolvedStoreNonce = saved }()
	writeClustersFile(t, "[clusters.acme]\npbs_namespace = \"backups\"\n")
	first := storePathFor("acme")
	unresolvedStoreNonce = "next-invocation"
	if got := storePathFor("acme"); got == first {
		t.Fatalf("a namespace-only entry reopened an earlier invocation's store %q", got)
	}
}
