// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests for sync completeness, Codex round 17
// (handfixes.json: sync-non-object-item-is-incomplete,
// sync-short-page-follows-continuation, sync-has-more-null-is-unknown,
// sync-seen-ids-are-stored-ids).

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/store"
)

func syncR17Count(t *testing.T, dbPath, resource string) int {
	t.Helper()
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n, err := db.Count(resource)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func syncR17Page(q url.Values) int {
	page := 1
	if p := q.Get("page[number]"); p != "" {
		fmt.Sscan(p, &page)
	}
	return page
}

func syncR17Agent(id string) string {
	return `{"id":"` + id + `","type":"user_agents","attributes":{"hostname":"PC-` + id + `"}}`
}

// A list item that is not a JSON object (42, "x", null) is a row the API
// returned that was not stored: the walk is not complete, and --full must not
// prune the stored row that item stands for.
func TestSyncNonObjectItemIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	second := ""
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		if syncR17Page(q) != 1 {
			return `{"data":[]}`, true
		}
		if second != "" {
			return `{"data":[` + syncR17Agent("a0") + `,` + second + `]}`, true
		}
		return `{"data":[` + syncR17Agent("a0") + `,` + syncR17Agent("a1") + `]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	full := []string{"sync", "--full", "--resources", "user-agents", "--db", dbPath}
	syncHFRunOK(t, full...)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "first walk")
	for _, item := range []string{`42`, `"a1"`, `null`, `[1]`} {
		second = item
		out, _ := syncHFRun(full...)
		if n := syncR17Count(t, dbPath, "user-agents"); n != 2 {
			t.Fatalf("item %s: --full pruned a row the API still returns (%d rows left, want 2):\n%s", item, n, out)
		}
		syncHFWantComplete(t, dbPath, "user-agents", 0, "a page with a non-object item "+item)
	}
}

// A null child element is a failure for that parent, never a panic.
func TestSyncDependentNullChildIsAFailure(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if syncR17Page(q) != 1 {
			return `{"data":[]}`, true
		}
		switch path {
		case "/v1/networks/all":
			return `{"data":[{"id":"2","type":"networks","attributes":{"name":"HQ"}}]}`, true
		case "/v1/networks/2/lan_ips":
			return `{"data":[{"id":"2000","type":"lan_ips","attributes":{"ip":"10.0.0.1"}},null]}`, true
		}
		return "", false
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out, _ := syncHFRun("sync", "--full", "--resources", "networks,lan_ips", "--db", dbPath)
	if n := syncR17Count(t, dbPath, "lan_ips"); n != 1 {
		t.Fatalf("stored %d lan_ips, want the 1 object child:\n%s", n, out)
	}
	if complete, _ := syncHFComplete(t, dbPath, "lan_ips"); complete != 0 {
		t.Fatalf("a parent walk with a null child was certified complete:\n%s", out)
	}
}

// A short page that carries an explicit continuation (links.next) is not the
// end: the walk follows it, flat and per parent, and a --max-pages cap on that
// short page is a truncation, not a complete walk.
func TestSyncShortPageFollowsContinuation(t *testing.T) {
	testenv.Isolate(t)
	api := syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		page := syncR17Page(q)
		switch path {
		case "/v1/user_agents/all":
			switch page {
			case 1:
				return `{"data":[` + syncR17Agent("a0") + `,` + syncR17Agent("a1") + `],"links":{"self":"/v1/user_agents/all?page[number]=1","next":"/v1/user_agents/all?page[number]=2&page[size]=100"}}`, true
			case 2:
				return `{"data":[` + syncR17Agent("a2") + `],"links":{"self":"/v1/user_agents/all?page[number]=2"}}`, true
			}
			return `{"data":[]}`, true
		case "/v1/networks/all":
			if page != 1 {
				return `{"data":[]}`, true
			}
			return `{"data":[{"id":"2","type":"networks","attributes":{"name":"HQ"}}]}`, true
		case "/v1/networks/2/lan_ips":
			lan := func(id string) string {
				return `{"id":"` + id + `","type":"lan_ips","attributes":{"ip":"10.0.0.` + id + `"}}`
			}
			switch page {
			case 1:
				return `{"data":[` + lan("1") + `,` + lan("2") + `],"links":{"next":"https://api.dnsfilter.com/v1/networks/2/lan_ips?page%5Bnumber%5D=2"}}`, true
			case 2:
				return `{"data":[` + lan("3") + `]}`, true
			}
			return `{"data":[]}`, true
		}
		return "", false
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out, err := syncHFRun("sync", "--full", "--resources", "user-agents,networks,lan_ips", "--db", dbPath)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := api.last("/v1/user_agents/all").Get("page[number]"); got != "2" {
		t.Fatalf("the continuation on the short first page was not followed (last page requested %q):\n%s", got, out)
	}
	if n := syncR17Count(t, dbPath, "user-agents"); n != 3 {
		t.Fatalf("flat walk stored %d user-agents, want 3:\n%s", n, out)
	}
	if n := syncR17Count(t, dbPath, "lan_ips"); n != 3 {
		t.Fatalf("per-parent walk stored %d lan_ips, want 3:\n%s", n, out)
	}
	syncHFWantComplete(t, dbPath, "user-agents", 1, "walk that followed the continuation")
	capped := filepath.Join(t.TempDir(), "data.db")
	_, _ = syncHFRun("sync", "--resources", "user-agents", "--max-pages", "1", "--db", capped)
	syncHFWantComplete(t, capped, "user-agents", 0, "--max-pages 1 on a short page with a continuation")
}

// has_more: null (or a string) is unknown, never a proven end: a full page
// that says has_more null is followed by the page-number fallback.
func TestSyncHasMoreNullIsUnknown(t *testing.T) {
	testenv.Isolate(t)
	hasMore := `null`
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		switch syncR17Page(q) {
		case 1:
			items := make([]string, 0, 100)
			for i := 0; i < 100; i++ {
				items = append(items, syncR17Agent(fmt.Sprintf("a%d", i)))
			}
			return `{"data":[` + strings.Join(items, ",") + `],"has_more":` + hasMore + `}`, true
		case 2:
			return `{"data":[` + syncR17Agent("last") + `]}`, true
		}
		return `{"data":[]}`, true
	})
	for _, v := range []string{`null`, `"false"`, `0`} {
		hasMore = v
		dbPath := filepath.Join(t.TempDir(), "data.db")
		out, err := syncHFRun("sync", "--full", "--resources", "user-agents", "--db", dbPath)
		if err != nil {
			t.Fatalf("has_more %s: %v\n%s", v, err, out)
		}
		if n := syncR17Count(t, dbPath, "user-agents"); n != 101 {
			t.Fatalf("has_more %s was read as the end: stored %d user-agents, want 101:\n%s", v, n, out)
		}
	}
	// An explicit false still ends the walk on the full page.
	hasMore = `false`
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--db", dbPath)
	if n := syncR17Count(t, dbPath, "user-agents"); n != 100 {
		t.Fatalf("has_more false: stored %d user-agents, want 100", n)
	}
	syncHFWantComplete(t, dbPath, "user-agents", 1, "has_more false")
}

// The reconcile seen set is the ids the writer stored, so an ID-bearing
// envelope item ({"result":{"id":"7",...}}), which the store unwraps and keys
// by the inner id, is never deleted by the --full reconcile that follows it,
// flat or per parent.
func TestSyncFullKeepsEnvelopeRows(t *testing.T) {
	testenv.Isolate(t)
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if syncR17Page(q) != 1 {
			return `{"data":[]}`, true
		}
		switch path {
		case "/v1/user_agents/all":
			return `{"data":[{"result":` + syncR17Agent("7") + `}]}`, true
		case "/v1/networks/all":
			return `{"data":[{"id":"2","type":"networks","attributes":{"name":"HQ"}}]}`, true
		case "/v1/networks/2/lan_ips":
			return `{"data":[{"result":{"id":"2000","type":"lan_ips","attributes":{"ip":"10.0.0.1"}}}]}`, true
		}
		return "", false
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out, err := syncHFRun("sync", "--full", "--resources", "user-agents,networks,lan_ips", "--db", dbPath)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if n := syncR17Count(t, dbPath, "user-agents"); n != 1 {
		t.Fatalf("flat --full reconcile deleted the envelope row it stored (%d left):\n%s", n, out)
	}
	if n := syncR17Count(t, dbPath, "lan_ips"); n != 1 {
		t.Fatalf("per-parent --full reconcile deleted the envelope row it stored (%d left):\n%s", n, out)
	}
}

// A singleton response's seen id is the id its writer stored it under: a
// typed writer keys an id-less parameter row by its fingerprint, not by the
// resource name.
func TestSyncSingletonSeenIDIsTheStoredID(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, tc := range []struct{ resource, body string }{
		{"billing", `{"plan":"msp","seats":12}`},
		{"distributors", `{"id":1,"name":"Distributor"}`},
		{"numeric_ids", `{"name":"no id"}`},
	} {
		id, err := upsertSingleObjectStoredID(db, tc.resource, json.RawMessage(tc.body))
		if err != nil {
			t.Fatalf("%s: %v", tc.resource, err)
		}
		ids, err := db.ListIDs(tc.resource)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 || store.BareResourceID(ids[0]) != id {
			t.Fatalf("%s: returned seen id %q, stored ids %q", tc.resource, id, ids)
		}
	}
}
