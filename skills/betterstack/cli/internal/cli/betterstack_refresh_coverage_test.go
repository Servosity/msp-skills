// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Class stale-dependent-resource (Codex r1 [P1], betterstack 0.1.8): a report
// is only as fresh as the staleest resource it reads. The auto-refresh hook
// refreshes the resources readCommandResources lists for a command, and the
// loaders accept any finished sync however old, so a resource a report reads
// but does not list is served from a checkpoint the hook never looks at.
// statuspage-audit listed status pages, monitors and incidents and also read
// heartbeats: a heartbeat paused in Better Stack after the last sync stayed
// unpaused in the store, and the audit said "live_check: ok" with no finding.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"betterstack-pp-cli/internal/cliutil/testenv"
	"betterstack-pp-cli/internal/store"
)

func TestStatuspageAuditRefreshesTheHeartbeatsItReads(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("BETTERSTACK_NO_AUTO_REFRESH", "")
	t.Setenv(noLearnEnvVar, "")
	var mu sync.Mutex
	paused := "false"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		p := paused
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/status-pages/184620/resources"):
			fmt.Fprint(w, bsR1Page([]string{`{"id":"1","type":"status_page_resource","attributes":{"public_name":"Backups","resource_type":"Heartbeat","resource_id":7000001}}`}, ""))
		case strings.HasSuffix(r.URL.Path, "/status-pages"):
			fmt.Fprint(w, bsR1Page([]string{`{"id":"184620","type":"status_page","attributes":{"company_name":"Client A","subdomain":"clienta","aggregate_state":"operational"}}`}, ""))
		case strings.HasSuffix(r.URL.Path, "/heartbeats"):
			fmt.Fprint(w, bsR1Page([]string{`{"id":"7000001","type":"heartbeat","attributes":{"name":"backup job","status":"up","period":3600,"grace":300,"paused":` + p + `}}`}, ""))
		default:
			fmt.Fprint(w, bsR1Page(nil, ""))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "bsRefreshUnitToken-5d81e0")
	run := func(args ...string) (string, error) {
		cmd := RootCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args) // the default store and no --no-learn: the path auto-refresh serves
		err := cmd.Execute()
		return out.String(), err
	}

	if out, err := run("sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	// Every checkpoint is now older than the freshness window.
	s, err := store.Open(defaultDBPath("betterstack-cli"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.DB().Exec(`UPDATE sync_state SET last_synced_at = ?`, time.Now().Add(-2*cachePolicy().StaleAfter).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Fatal("aged no checkpoint: the test is not reading the store sync wrote")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	paused = "true" // paused in Better Stack after the sync
	mu.Unlock()

	out, err := run("statuspage-audit", "--json")
	if err != nil || !strings.Contains(out, "paused heartbeat") {
		t.Errorf("heartbeat 7000001 was paused after the last sync and the store is past its freshness window; statuspage-audit must refresh it and flag the paused heartbeat: err %v\n%s", err, out)
	}
}

// reportArgs holds the arguments a report needs to reach its loaders, by
// command name. None of today's reports takes one; a new report that does
// adds its entry here rather than weakening the guard.
var reportArgs = map[string][]string{}

// TestEveryReportRefreshesEveryResourceItReads is the guard for the class:
// every command in this package whose body reaches requireFinishedSync
// (directly or through the loaders) is found in the source, so a new report
// needs no list update to be checked. Each must have a readCommandResources
// entry, and running it against a synced store must ask requireFinishedSync
// only about resources that entry refreshes. A new loader call, or a new
// report without its entry, fails here.
func TestEveryReportRefreshesEveryResourceItReads(t *testing.T) {
	names := storeReportNames(t)
	if len(names) == 0 || !slices.Contains(names, "statuspage-audit") {
		t.Fatalf("the source scan found %v; it must find every report, statuspage-audit included", names)
	}

	testenv.Isolate(t)
	t.Setenv("BETTERSTACK_NO_AUTO_REFRESH", "")
	t.Setenv(noLearnEnvVar, "")
	started := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	pages := map[string][]string{
		"/heartbeat-groups": {`{"id":"6000001","type":"heartbeat_group","attributes":{"name":"Client A","paused":false}}`},
		"/heartbeats":       {`{"id":"7000001","type":"heartbeat","attributes":{"name":"backup job","status":"down","period":3600,"grace":300,"paused":false,"heartbeat_group_id":6000001}}`},
		"/incidents":        {`{"id":"8000001","type":"incident","attributes":{"name":"backup job","started_at":"` + started + `","acknowledged_at":null,"resolved_at":null,"heartbeat_id":7000001}}`, `{"id":"8000002","type":"incident","attributes":{"name":"API","started_at":"` + started + `","acknowledged_at":null,"resolved_at":null,"monitor_id":2085431}}`},
		"/monitor-groups":   {`{"id":"5000001","type":"monitor_group","attributes":{"name":"Client A","paused":false}}`},
		"/monitors":         {`{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"API","url":"https://api.example.com","monitor_type":"status","status":"down","paused":false,"monitor_group_id":5000001}}`},
		"/on-calls":         {`{"id":"4000001","type":"on_call_calendar","attributes":{"name":"Primary","default_calendar":true,"on_call_users":[]}}`},
		"/policies":         {`{"id":"3000001","type":"policy","attributes":{"name":"Default"}}`},
		"/status-pages":     {`{"id":"184620","type":"status_page","attributes":{"company_name":"Client A","subdomain":"clienta","aggregate_state":"operational"}}`},
		"/status-pages/184620/resources": {
			`{"id":"1","type":"status_page_resource","attributes":{"public_name":"API","resource_type":"Monitor","resource_id":2085431}}`,
			`{"id":"2","type":"status_page_resource","attributes":{"public_name":"Backups","resource_type":"Heartbeat","resource_id":7000001}}`,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bsR1Page(pages[strings.TrimPrefix(r.URL.Path, "/api/v2")], ""))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "bsRefreshUnitToken-9a24f7")
	run := func(args ...string) (string, error) {
		cmd := RootCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	if out, err := run("sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	var seen map[string]bool
	requireFinishedSyncSeen = func(r string) { seen[r] = true }
	t.Cleanup(func() { requireFinishedSyncSeen = nil })
	root := RootCmd()
	for _, name := range names {
		cmd := findRunnableCommand(root, name)
		if cmd == nil {
			t.Errorf("%s reads the store but no runnable command of that name is registered", name)
			continue
		}
		path := cmd.CommandPath()
		refreshed, ok := readCommandResources[path]
		if !ok {
			t.Errorf("%q reads the store but has no readCommandResources entry (auto_refresh.go), so it is never refreshed; add one naming every resource it reads and record it in handfixes.json", path)
		}
		seen = map[string]bool{}
		args := append(strings.Fields(path)[1:], reportArgs[name]...)
		out, err := run(append(args, "--json", "--no-learn")...)
		if len(seen) == 0 {
			t.Errorf("%q asked requireFinishedSync about nothing, so the guard proved nothing about it (err %v); give it what it needs in reportArgs\n%s", path, err, out)
			continue
		}
		var missing []string
		for r := range seen {
			if !slices.Contains(refreshed, r) {
				missing = append(missing, r)
			}
		}
		sort.Strings(missing)
		t.Logf("%s reads %v; refreshes %v", path, slices.Sorted(maps.Keys(seen)), refreshed)
		if len(missing) > 0 {
			t.Errorf("%q reads %v from the store, but its readCommandResources entry %v does not refresh them, so it answers from a checkpoint the freshness window never checks", path, missing, refreshed)
		}
	}
}

// storeReportNames returns the Use name of every cobra.Command literal in
// this package's non-test source whose body calls requireFinishedSync or a
// package function that reaches it (a loader). A function that builds a
// cobra.Command is a command constructor, not a loader, so it does not
// spread the mark to the root that registers it.
func storeReportNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var parsed []*ast.File
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		parsed = append(parsed, af)
	}
	isCommandLit := func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return false
		}
		sel, ok := cl.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Command" {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && x.Name == "cobra"
	}
	has := func(n ast.Node, match func(ast.Node) bool) bool {
		found := false
		ast.Inspect(n, func(m ast.Node) bool {
			if !found && m != nil && match(m) {
				found = true
			}
			return !found
		})
		return found
	}
	callsInto := func(set map[string]bool) func(ast.Node) bool {
		return func(m ast.Node) bool {
			c, ok := m.(*ast.CallExpr)
			if !ok {
				return false
			}
			switch fn := c.Fun.(type) {
			case *ast.Ident:
				return set[fn.Name]
			case *ast.SelectorExpr:
				return set[fn.Sel.Name]
			}
			return false
		}
	}
	readers := map[string]bool{"requireFinishedSync": true}
	for changed := true; changed; {
		changed = false
		for _, af := range parsed {
			for _, d := range af.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil || readers[fd.Name.Name] || has(fd.Body, isCommandLit) {
					continue
				}
				if has(fd.Body, callsInto(readers)) {
					readers[fd.Name.Name] = true
					changed = true
				}
			}
		}
	}
	var names []string
	for _, af := range parsed {
		ast.Inspect(af, func(n ast.Node) bool {
			if n == nil || !isCommandLit(n) || !has(n, callsInto(readers)) {
				return true
			}
			cl := n.(*ast.CompositeLit)
			use := ""
			for _, e := range cl.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Use" {
					if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						use, _ = strconv.Unquote(lit.Value)
					}
				}
			}
			if fields := strings.Fields(use); len(fields) > 0 {
				names = append(names, fields[0])
			} else {
				t.Errorf("%s: a command reads the store but its Use is not a string literal, so the guard cannot find it", fset.Position(cl.Pos()))
			}
			return true
		})
	}
	sort.Strings(names)
	return names
}

// findRunnableCommand returns the one runnable command named name anywhere
// under root, or nil when there is none or more than one.
func findRunnableCommand(root *cobra.Command, name string) *cobra.Command {
	var hits []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == name && c.Runnable() {
			hits = append(hits, c)
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
	if len(hits) != 1 {
		return nil
	}
	return hits[0]
}
