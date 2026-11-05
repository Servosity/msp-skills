// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the SuperOps tenant keys every sync
// checkpoint and every local store (superops-tenant-scope).

package cli

import (
	"context"
	"sync"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/store"
)

// The generated syncScopeTemplateVars lists the base-URL placeholders that
// distinguish one tenant's checkpoint from another's; SuperOps has none, so
// the list was empty and every tenant shared one checkpoint per resource. The
// client reports the tenant under this name (client.CurrentTemplateVars).
//
// Every store open asks for the running tenant (store.enforceStoreTenant).
// An open made inside a command execution or an MCP tool call carries that
// run's snapshot in its context (runSnapshot.storeContext); any other open
// asks storeTenantOfThisCommand. Installing it here puts the check in both
// binaries: superops-cli runs this package, and superops-mcp imports it for
// MCPStoreRun.
func init() {
	syncScopeTemplateVars = append(syncScopeTemplateVars, config.SuperOpsTenantScopeVar)
	store.SetTenantSource(storeTenantOfThisCommand)
}

// storeAdoptsPreTenantStores reports whether the default store path may adopt
// a store written before the tenant-scoped key: the token-named data-<hash>.db
// or the unscoped data.db a 4.24 install kept. Neither records which tenant
// filled it, so adopting one shows tenant A's rows under tenant B. superops
// never adopts: an upgraded install starts a fresh tenant-scoped store and the
// old files are left in place (one re-sync; the pre-reprint store also held
// only the first 50 rows of each resource).
func storeAdoptsPreTenantStores() bool { return false }

// runSnapshot is the configuration of one run: one command execution of
// superops-cli, or one MCP tool call. It is loaded once, by whichever of the
// default-store scope (configureDefaultDBScopeFrom), the client
// (rootFlags.newClient) or a store's tenant check asks first, and every
// later caller in the run gets the same *config.Config. So the tenant a
// store is checked against and recorded for is always the tenant of the
// configuration the run's client sends requests with: an edit to --config or
// the default config file in the middle of a run changes nothing until the
// next run.
type runSnapshot struct {
	configPath string
	once       sync.Once
	cfg        *config.Config
	err        error
}

func newRunSnapshot(configPath string) *runSnapshot {
	return &runSnapshot{configPath: configPath}
}

// config is the run's one configuration load.
func (r *runSnapshot) config() (*config.Config, error) {
	r.once.Do(func() { r.cfg, r.err = config.Load(r.configPath) })
	return r.cfg, r.err
}

// tenant is the running tenant for every store this run opens. ok is false
// when the configuration did not load; then no store opens.
func (r *runSnapshot) tenant() (string, bool) {
	cfg, err := r.config()
	if err != nil || cfg == nil {
		return "", false
	}
	return cfg.SuperOpsTenant(), true
}

// storeContext is ctx carrying this run's tenant for every store opened with
// it.
func (r *runSnapshot) storeContext(ctx context.Context) context.Context {
	return store.WithTenantSource(ctx, r.tenant)
}

// detachedStoreContext is storeContext for a store the run opens outside its
// command's context (the post-run learn pass in Execute).
func (r *runSnapshot) detachedStoreContext() context.Context {
	return r.storeContext(context.Background())
}

var runSnapshotMu sync.Mutex

// beginRunSnapshot starts the run of one command execution: a fresh snapshot
// on flags (for newClient) and in the returned context (for every store open
// the command makes). The root pre-run calls it once per execution, so a
// later command in the same process, as in tests, never sees this one's.
func (f *rootFlags) beginRunSnapshot(ctx context.Context) context.Context {
	runSnapshotMu.Lock()
	f.snapshot = newRunSnapshot(f.configPath)
	snapshot := f.snapshot
	runSnapshotMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	return snapshot.storeContext(ctx)
}

// runSnapshot is this execution's snapshot, started on first use when the
// pre-run has not run (a rootFlags is built per execution, so it never
// outlives one).
func (f *rootFlags) runSnapshot() *runSnapshot {
	runSnapshotMu.Lock()
	defer runSnapshotMu.Unlock()
	if f.snapshot == nil {
		f.snapshot = newRunSnapshot(f.configPath)
	}
	return f.snapshot
}

// MCPStoreRun is the local store of one MCP tool call: its path and a context
// whose store opens are checked against the tenant of the same configuration
// snapshot the path was scoped by. The path is the one MCPStorePath returns.
func MCPStoreRun(ctx context.Context) (context.Context, string, error) {
	snapshot := newRunSnapshot("")
	configureDefaultDBScopeFrom(snapshot)
	if ctx == nil {
		ctx = context.Background()
	}
	dir, err := cliutil.DataDir()
	if err != nil {
		return ctx, "", err
	}
	return snapshot.storeContext(ctx), defaultDBPathInDir(dir), nil
}

var storeTenantConfig struct {
	sync.RWMutex
	path string
}

// recordStoreTenantConfig remembers the --config path of the latest run.
// configureDefaultDBScopeFrom calls it; storeTenantOfThisCommand reads it.
func recordStoreTenantConfig(configPath string) {
	storeTenantConfig.Lock()
	storeTenantConfig.path = configPath
	storeTenantConfig.Unlock()
}

// storeTenantOfThisCommand is the process-wide tenant source: the tenant for
// a store open whose context carries no run snapshot, which is code outside
// any command execution or MCP tool call (tests, and the store's own direct
// callers). It loads a snapshot of its own from the --config path the latest
// run recorded. A config that does not load gives no tenant, and then no
// store opens. Every open inside a run carries that run's snapshot instead
// (TestEveryShippedStoreOpenPassesAContext keeps the shipped code that way).
func storeTenantOfThisCommand() (string, bool) {
	storeTenantConfig.RLock()
	path := storeTenantConfig.path
	storeTenantConfig.RUnlock()
	return newRunSnapshot(path).tenant()
}
