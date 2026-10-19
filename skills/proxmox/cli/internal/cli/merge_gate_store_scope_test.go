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
	if _, _, err := runCLI(t, "--cluster", "acme", "learnings", "stats"); err == nil || !strings.Contains(err.Error(), "reads as URI syntax") {
		t.Fatalf("a '?' store path was not refused: err=%v", err)
	}
	for _, dir := range []string{"data%00-archive", "data#x", "data\x01x"} {
		t.Setenv("PROXMOX_DATA_DIR", filepath.Join(t.TempDir(), dir))
		selectedCluster = "acme"
		if err := storePathErr(); err == nil {
			t.Errorf("store path under %q was not refused", dir)
		}
	}
}

// One private address can be a different client behind another VPN: a new
// token reference or certificate fingerprint is a new store.
func TestReusedPrivateAddressWithNewCredentialsGetsItsOwnStore(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	base := acmeClusters(f, f.pbs.URL, "")
	writeClustersFile(t, base)
	first := storePathFor("acme")
	for _, edit := range [][2]string{{`ACME_PVE`, `BETA_PVE`}, {`ACME_PBS`, `BETA_PBS`}, {f.pveFingerprint(), strings.Repeat("AB:", 31) + "AB"}} {
		writeClustersFile(t, strings.Replace(base, edit[0], edit[1], 1))
		if got := storePathFor("acme"); got == first {
			t.Errorf("changing %q kept the previous tenant's store", edit[0])
		}
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

// hand-fix (cluster-store-no-fallback): one process binds one version of an
// entry, so `clusters add --force` mid-request cannot move the store to the
// new tenant while the client still talks to the old one.
func TestClusterEntryBoundForTheProcess(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	before := storePathFor("acme")
	cl, err := clusterByName("acme")
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite in place without the helper's reset: a concurrent `clusters add --force`.
	body := strings.Replace(acmeClusters(f, f.pbs.URL, ""), f.pve.URL, "https://192.0.2.10:8006", 1)
	if err := os.WriteFile(os.Getenv("PROXMOX_CLUSTERS_FILE"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := storePathFor("acme"); got != before {
		t.Fatalf("store moved mid-process: %q -> %q", before, got)
	}
	if again, _ := clusterByName("acme"); again.PVEBaseURL != cl.PVEBaseURL {
		t.Fatalf("client entry changed mid-process: %q -> %q", cl.PVEBaseURL, again.PVEBaseURL)
	}
}

// The token values the client sends are part of the identity: the same entry
// reused behind another client's VPN with new token values is a new store.
func TestNewTokenValuesGetTheirOwnStore(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, ""))
	t.Setenv("ACME_PVE", "auditor@pve!cli=00000000-0000-0000-0000-00000000000a")
	t.Setenv("ACME_PBS", "auditor@pbs!cli:00000000-0000-0000-0000-00000000000a")
	first := storePathFor("acme")
	for _, env := range []string{"ACME_PVE", "ACME_PBS"} {
		t.Setenv(env, "auditor@pve!cli=00000000-0000-0000-0000-00000000000b")
		if got := storePathFor("acme"); got == first {
			t.Errorf("new %s token value kept the previous tenant's store", env)
		}
	}
}

// hand-fix (env-store-endpoint-scope): without a cluster, the env target's
// store follows where it points, not only the token: the same token aimed at
// another host must not fall back to the first host's rows.
func TestEnvStoreFollowsEndpoint(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	first := storePathFor("")
	for env, value := range map[string]string{
		"PROXMOX_BASE_URL":            "https://192.0.2.10:8006/api2/json",
		"PROXMOX_PBS_HOST":            "https://192.0.2.11:8007",
		"PROXMOX_TLS_FINGERPRINT":     strings.Repeat("AB:", 31) + "AB",
		"PROXMOX_PBS_TLS_FINGERPRINT": strings.Repeat("CD:", 31) + "CD",
	} {
		saved := os.Getenv(env)
		t.Setenv(env, value)
		if got := storePathFor(""); got == first {
			t.Errorf("changing %s kept the previous target's store %q", env, got)
		}
		t.Setenv(env, saved)
	}
	if again := storePathFor(""); again != first {
		t.Fatalf("restoring the target moved its store: %q -> %q", first, again)
	}
}

// hand-fix (env-store-endpoint-scope): a scoped store never adopts an
// existing data.db, whose owner cannot be told apart.
func TestEnvStoreIgnoresLegacyUnscopedDB(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	unscoped := filepath.Join(dir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := storePathFor(""); got == unscoped {
		t.Fatalf("the env target adopted the shared data.db %q", got)
	}
}

// hand-fix (env-store-endpoint-scope): the no-cluster target fails closed the
// same ways a cluster does.
func TestEnvStoreFailsClosed(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope(""); preRunScoped = false }()

	// Two hosts with no token loaded still get separate stores.
	t.Setenv("PROXMOX_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	a := storePathFor("")
	t.Setenv("PROXMOX_BASE_URL", "https://192.0.2.10:8006/api2/json")
	if b := storePathFor(""); a == b {
		t.Errorf("two hosts with no token share %q", a)
	}

	// An unreadable config names no target: never a store another run reopens.
	bad := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(bad, []byte("this is = = not toml [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := unresolvedStoreNonce
	defer func() { unresolvedStoreNonce = saved }()
	configureDefaultDBScope(bad)
	first := defaultDBPath("proxmox-cli")
	unresolvedStoreNonce = "next-invocation"
	configureDefaultDBScope(bad)
	if filepath.Base(first) == "data.db" || defaultDBPath("proxmox-cli") == first {
		t.Errorf("a config error reused a store: %q", first)
	}
	unresolvedStoreNonce = saved

	// URI syntax in the data directory is refused without a cluster too.
	t.Setenv("PROXMOX_DATA_DIR", filepath.Join(t.TempDir(), "data?tenant"))
	configureDefaultDBScope("")
	if err := storePathErr(); err == nil {
		t.Error("a '?' store path was accepted without a cluster")
	}
}

// hand-fix (env-store-endpoint-scope): a client built from a config read that
// differs from the one pre-run scoped the store with is refused.
func TestClientRefusedWhenConfigChangedMidRun(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope(""); preRunScoped = false }()
	configureDefaultDBScope("")
	preRunScoped = true
	f := &rootFlags{}
	if _, err := f.newClient(); err != nil {
		t.Fatalf("unchanged config refused: %v", err)
	}
	t.Setenv("PROXMOX_BASE_URL", "https://192.0.2.10:8006/api2/json")
	if _, err := f.newClient(); err == nil || !strings.Contains(err.Error(), "configuration changed") {
		t.Fatalf("a changed target got a client on the old store: err=%v", err)
	}
}

// hand-fix (cluster-store-no-fallback): with no resolvable home the
// no-cluster target is scoped too, and an explicit --db is held to the same
// URI-syntax rule as the default path.
func TestEnvStoreWithoutHomeAndExplicitDBSyntax(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope(""); preRunScoped = false }()
	if _, _, err := runCLI(t, "learnings", "stats", "--db", filepath.Join(t.TempDir(), "archive?2026", "acme.db")); err == nil || !strings.Contains(err.Error(), "reads as URI syntax") {
		t.Fatalf("a '?' in --db was accepted: err=%v", err)
	}
	for _, k := range []string{"HOME", "XDG_DATA_HOME", "PROXMOX_DATA_DIR", "PROXMOX_HOME"} {
		t.Setenv(k, "")
	}
	if _, err := cliutil.DataDir(); err == nil {
		t.Skip("data dir still resolves without HOME on this platform")
	}
	// No home: the config cannot load, so each invocation gets its own scoped
	// file in the working directory, never the bare shared data.db.
	saved := unresolvedStoreNonce
	defer func() { unresolvedStoreNonce = saved }()
	a := storePathFor("")
	unresolvedStoreNonce = "next-invocation"
	if b := storePathFor(""); a == "data.db" || a == b {
		t.Fatalf("no-home fallback reused a store across invocations: %q and %q", a, b)
	}
}

// hand-fix (env-store-endpoint-scope, cluster-store-no-fallback): a template
// variable that fills the base URL is part of the no-cluster identity, and a
// clusters.toml address may not be a {placeholder} at all.
func TestTemplateVarsAreStoreIdentity(t *testing.T) {
	f := newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	write := func(site string) {
		body := "base_url = \"https://{site}:8006/api2/json\"\n[template_vars]\nsite = \"" + site + "\"\n"
		if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PROXMOX_BASE_URL", "")
	write("alpha.example")
	configureDefaultDBScope(cfgPath)
	a := defaultDBPath("proxmox-cli")
	write("beta.example")
	configureDefaultDBScope(cfgPath)
	if b := defaultDBPath("proxmox-cli"); a == b {
		t.Errorf("changing template_vars.site kept the store %q", a)
	}
	writeClustersFile(t, strings.Replace(acmeClusters(f, f.pbs.URL, ""), f.pve.URL+"/api2/json", "https://proxy.example/{tenant}/api2/json", 1))
	if _, err := clusterByName("acme"); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("a {placeholder} pve_base_url was accepted: %v", err)
	}
}
