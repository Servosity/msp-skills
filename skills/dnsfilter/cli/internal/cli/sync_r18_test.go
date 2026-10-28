// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests for sync completeness, Codex round 18
// (handfixes.json: sync-empty-proof-is-the-response-path,
// sync-non-object-item-is-incomplete, sync-unpaginated-continuation-is-incomplete,
// sync-has-more-null-is-unknown, merge-explicit-empty-scalar-is-authoritative).

package cli

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// syncR18Seeded runs a first full walk that stores user-agents a1 and a2,
// then serves second for every later page 1, and returns the db path.
func syncR18Seeded(t *testing.T, second *string) string {
	t.Helper()
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		if syncR17Page(q) != 1 {
			return `{"data":[]}`, true
		}
		if *second != "" {
			return *second, true
		}
		return `{"data":[` + syncR17Agent("1") + `,` + syncR17Agent("2") + `]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "user-agents", 1, "first walk")
	if n := syncR17Count(t, dbPath, "user-agents"); n != 2 {
		t.Fatalf("seeded %d user-agents, want 2", n)
	}
	return dbPath
}

// Only the resource's own list (the array at its response path) proves an
// empty collection: a metadata array beside a null or missing data is
// unknown, so --full never prunes to it and the walk is not complete.
func TestSyncMetadataArrayNeverProvesEmpty(t *testing.T) {
	testenv.Isolate(t)
	second := ""
	dbPath := syncR18Seeded(t, &second)
	for _, body := range []string{
		`{"data":null,"included":[],"meta":{"total":2}}`,
		`{"included":[],"meta":{"total":2}}`,
		`{"data":null}`,
		`{"data":null,"total":0}`,
		`{"data":"none","included":[]}`,
		`{"errors":[],"data":null}`,
	} {
		second = body
		out, _ := syncHFRun("sync", "--full", "--resources", "user-agents", "--db", dbPath)
		if n := syncR17Count(t, dbPath, "user-agents"); n != 2 {
			t.Fatalf("%s: --full pruned the mirror to %d user-agents, want 2:\n%s", body, n, out)
		}
		syncHFWantComplete(t, dbPath, "user-agents", 0, "a page with no readable data: "+body)
	}
	// The resource's own empty list still proves an empty collection.
	second = `{"data":[],"included":[],"meta":{"total":0}}`
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--db", dbPath)
	if n := syncR17Count(t, dbPath, "user-agents"); n != 0 {
		t.Fatalf("an empty data list left %d user-agents", n)
	}
	syncHFWantComplete(t, dbPath, "user-agents", 1, "an empty data list")

	// A resource whose writer would key an id-less envelope by its
	// fingerprint (user-agent-releases) never stores the unreadable
	// envelope as its one record and never marks the collection complete.
	releases := `{"user_agent_releases":[{"agent_type":"windows","architecture":"x86_64","agent_version":"3.2.0","white_label":false,"release_channels":[{"value":"stable"}]}]}`
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agent_releases" {
			return "", false
		}
		return releases, true
	})
	relDB := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agent-releases", "--db", relDB)
	syncHFWantComplete(t, relDB, "user-agent-releases", 1, "a readable release feed")
	before := syncR17Count(t, relDB, "user-agent-releases")
	for _, body := range []string{`{"user_agent_releases":null}`, `{"included":[]}`, `{"user_agent_releases":null,"meta":{"total":3}}`} {
		releases = body
		out, _ := syncHFRun("sync", "--full", "--resources", "user-agent-releases", "--db", relDB)
		if n := syncR17Count(t, relDB, "user-agent-releases"); n != before {
			t.Fatalf("%s: the unreadable envelope was stored as a record (%d rows, want %d):\n%s", body, n, before, out)
		}
		syncHFWantComplete(t, relDB, "user-agent-releases", 0, "an unreadable release feed: "+body)
	}
}

// A map-keyed record the response names with an empty object is an extract
// failure: the walk is not complete and --full keeps the stored row.
func TestSyncMapKeyedEmptyRecordIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	second := ""
	dbPath := syncR18Seeded(t, &second)
	second = `{"data":{"1":{},"2":` + syncR17Agent("2") + `}}`
	out, _ := syncHFRun("sync", "--full", "--resources", "user-agents", "--db", dbPath)
	if n := syncR17Count(t, dbPath, "user-agents"); n != 2 {
		t.Fatalf("--full pruned the record the response still names (%d rows left, want 2):\n%s", n, out)
	}
	syncHFWantComplete(t, dbPath, "user-agents", 0, "a map-keyed page with an empty record")
}

// A resource this CLI reads as one page, whose response carries an explicit
// continuation, is incomplete with a warning: never complete and pruned. The
// per-parent walk does the same.
func TestSyncUnpaginatedContinuationIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	if resourceSupportsPagination("api-keys") || resourceSupportsPagination("invoices") {
		t.Fatal("the fixture resources must be classified non-paginated")
	}
	second := false
	key := func(id string) string {
		return `{"id":"` + id + `","type":"api_keys","attributes":{"name":"k` + id + `"}}`
	}
	inv := func(id string) string {
		return `{"id":"` + id + `","type":"invoices","attributes":{"number":"N` + id + `"}}`
	}
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if syncR17Page(q) != 1 {
			if path == "/v1/api_keys" {
				return `{"data":[` + key("2") + `]}`, true
			}
			return `{"data":[]}`, true
		}
		switch path {
		case "/v1/api_keys":
			if second {
				return `{"data":[` + key("1") + `],"links":{"next":"/v1/api_keys?page[number]=2"}}`, true
			}
			return `{"data":[` + key("1") + `,` + key("2") + `]}`, true
		case "/v1/organizations/all":
			return `{"data":[{"id":"7","type":"organizations","attributes":{"name":"Acme"}}]}`, true
		case "/v1/invoices":
			if second {
				return `{"data":[` + inv("1") + `],"meta":{"has_more":true}}`, true
			}
			return `{"data":[` + inv("1") + `,` + inv("2") + `]}`, true
		}
		return `{"data":[]}`, true // the organizations' other dependents
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "api-keys,organizations,invoices", "--db", dbPath)
	syncHFWantComplete(t, dbPath, "api-keys", 1, "first walk")
	syncHFWantComplete(t, dbPath, "invoices", 1, "first walk")
	second = true
	out, _ := syncHFRun("sync", "--full", "--resources", "api-keys,organizations,invoices", "--db", dbPath)
	if n := syncR17Count(t, dbPath, "api-keys"); n != 2 {
		t.Fatalf("flat: --full pruned the key on the unfollowed page (%d left, want 2):\n%s", n, out)
	}
	if n := syncR17Count(t, dbPath, "invoices"); n != 2 {
		t.Fatalf("per parent: --full pruned the invoice on the unfollowed page (%d left, want 2):\n%s", n, out)
	}
	syncHFWantComplete(t, dbPath, "api-keys", 0, "a single page with links.next")
	syncHFWantComplete(t, dbPath, "invoices", 0, "a parent page with has_more true")
	if strings.Count(out, "unfollowed_continuation") < 2 {
		t.Fatalf("no unfollowed_continuation warning for both walks:\n%s", out)
	}
	// With --max-pages 1 the per-parent cap branch is incomplete too.
	capped := filepath.Join(t.TempDir(), "data.db")
	_, _ = syncHFRun("sync", "--resources", "organizations,invoices", "--max-pages", "1", "--db", capped)
	syncHFWantComplete(t, capped, "invoices", 0, "--max-pages 1 on a parent page with has_more true")
}

// has_more inside a pagination wrapper (meta, pagination, paging) is read
// wherever a next cursor is searched: a short page that says there is more is
// never certified complete; an explicit false there still ends a full page.
func TestSyncWrapperHasMoreIsRead(t *testing.T) {
	testenv.Isolate(t)
	second := ""
	dbPath := syncR18Seeded(t, &second)
	for _, body := range []string{
		`{"data":[` + syncR17Agent("1") + `],"meta":{"has_more":true}}`,
		`{"data":[` + syncR17Agent("1") + `],"pagination":{"hasMore":true}}`,
		`{"data":[` + syncR17Agent("1") + `],"has_more":false,"meta":{"has_next":true}}`,
		`{"data":[` + syncR17Agent("1") + `],"paging":{"has_more":true}}`,
	} {
		second = body
		out, _ := syncHFRun("sync", "--full", "--resources", "user-agents", "--db", dbPath)
		if n := syncR17Count(t, dbPath, "user-agents"); n != 2 {
			t.Fatalf("%s: --full pruned the outstanding rows (%d left, want 2):\n%s", body, n, out)
		}
		syncHFWantComplete(t, dbPath, "user-agents", 0, "a short page whose wrapper says has_more: "+body)
	}
	// An explicit false in the wrapper ends a full page: no page-number
	// fallback request for page 2.
	items := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		items = append(items, syncR17Agent(fmt.Sprintf("f%d", i)))
	}
	api := syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if path != "/v1/user_agents/all" {
			return "", false
		}
		if syncR17Page(q) != 1 {
			return `{"data":[` + syncR17Agent("extra") + `]}`, true
		}
		return `{"data":[` + strings.Join(items, ",") + `],"meta":{"has_more":false}}`, true
	})
	fresh := filepath.Join(t.TempDir(), "data.db")
	syncHFRunOK(t, "sync", "--full", "--resources", "user-agents", "--db", fresh)
	if got := api.last("/v1/user_agents/all").Get("page[number]"); got == "2" {
		t.Fatal("an explicit meta.has_more false on a full page was not read as the end")
	}
	if n := syncR17Count(t, fresh, "user-agents"); n != 100 {
		t.Fatalf("stored %d user-agents, want 100", n)
	}
	syncHFWantComplete(t, fresh, "user-agents", 1, "meta.has_more false")
}

// A policy list the API now sends mistyped (blacklist_domains: false) replaces
// the stored list, so lists where names the policy as unreadable instead of
// printing its all-clear from the stale list.
func TestSyncMistypedPolicyListIsNotHiddenByTheStaleList(t *testing.T) {
	testenv.Isolate(t)
	blacklist := `[]`
	syncFleetServer(t, func(path string, q url.Values) (string, bool) {
		if syncR17Page(q) != 1 {
			return `{"data":[]}`, true
		}
		switch path {
		case "/v1/organizations/all":
			return `{"data":[{"id":"1","type":"organizations","attributes":{"name":"Org One"}}]}`, true
		case "/v1/policies/all":
			return `{"data":[{"id":"10","type":"policies","attributes":{"name":"Std","organization_id":1,"is_global_policy":false,"whitelist_domains":[],"blacklist_domains":` + blacklist + `}}]}`, true
		case "/v1/networks/all", "/v1/scheduled_policies", "/v1/scheduled_policies/all", "/v1/user_agents/all":
			return `{"data":[]}`, true
		}
		return `{"data":[]}`, true
	})
	dbPath := filepath.Join(t.TempDir(), "data.db")
	resources := "organizations,networks,policies,scheduled-policies,user-agents"
	syncHFRunOK(t, "sync", "--resources", resources, "--db", dbPath)
	blacklist = `false`
	syncHFRunOK(t, "sync", "--resources", resources, "--db", dbPath)
	out, _ := syncHFRun("lists", "where", "example.com", "--db", dbPath)
	if !strings.Contains(out, `"unreadable_policies"`) || !strings.Contains(out, `"10"`) {
		t.Fatalf("lists where did not name policy 10 as unreadable after the API sent blacklist_domains false:\n%s", out)
	}
}
