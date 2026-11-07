// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
)

// Codex r1 classes (4.34.2 reprint), proved against a Graph fake that answers
// the way Microsoft Learn documents it:
//   - property-not-requested: a user object comes back with Graph's default
//     projection unless $select names more ("List users": businessPhones,
//     displayName, givenName, id, jobTitle, mail, mobilePhone, officeLocation,
//     preferredLanguage, surname, userPrincipalName; "List members of a
//     directory role" returns the same shape and supports $select only);
//   - stale-snapshot-kept: a completed pull is the tenant's state, so emptied
//     arrays, deleted objects and revoked memberships leave the store;
//   - incomplete-read-as-complete: every @odata.nextLink page is followed, and a
//     resource whose read fails keeps its previous snapshot, never a partial one.
var graphDefaultUserProps = []string{"businessPhones", "displayName", "givenName", "id", "jobTitle", "mail", "mobilePhone", "officeLocation", "preferredLanguage", "surname", "userPrincipalName"}

type fakeTenant struct {
	mu     sync.Mutex
	users  map[string]map[string]any
	groups map[string][2][]string // group id -> members, owners
	roles  map[string][]string    // role id -> member ids
	refuse map[string]bool        // path -> 403
	body   map[string]string      // path -> raw 200 body
}

func newFakeTenant() *fakeTenant {
	return &fakeTenant{
		users: map[string]map[string]any{
			"u1": {"id": "u1", "displayName": "Alice", "userPrincipalName": "alice@contoso.test", "accountEnabled": true, "userType": "Member", "assignedLicenses": []any{map[string]any{"skuId": "sku1"}}},
			"u2": {"id": "u2", "displayName": "Bob", "userPrincipalName": "bob@contoso.test", "accountEnabled": false, "userType": "Member", "assignedLicenses": []any{map[string]any{"skuId": "sku1"}}},
			"u3": {"id": "u3", "displayName": "Carol", "userPrincipalName": "carol@partner.test", "accountEnabled": true, "userType": "Guest", "assignedLicenses": []any{}},
		},
		groups: map[string][2][]string{"g1": {{"u1"}, {"u1"}}},
		roles:  map[string][]string{"r1": {"u2", "u3"}},
		refuse: map[string]bool{},
		body:   map[string]string{},
	}
}

func (f *fakeTenant) project(id string, sel string) map[string]any {
	u := f.users[id]
	props := graphDefaultUserProps
	if sel != "" {
		props = strings.Split(sel, ",")
	}
	out := map[string]any{"@odata.type": "#microsoft.graph.user"}
	for _, p := range props {
		if v, ok := u[p]; ok {
			out[p] = v
		}
	}
	return out
}

func (f *fakeTenant) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1.0")
	if f.refuse[path] {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"Authorization_RequestDenied","message":"Insufficient privileges"}}`))
		return
	}
	if b, ok := f.body[path]; ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(b))
		return
	}
	q := r.URL.Query()
	sel := q.Get("$select")
	var items []any
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case path == "/users":
		for _, id := range sortedIDs(f.users) {
			items = append(items, f.project(id, sel))
		}
	case path == "/groups":
		for id := range f.groups {
			items = append(items, map[string]any{"id": id, "displayName": "Team " + id})
		}
	case len(parts) == 3 && parts[0] == "groups" && (parts[2] == "members" || parts[2] == "owners"):
		ids := f.groups[parts[1]][0]
		if parts[2] == "owners" {
			ids = f.groups[parts[1]][1]
		}
		for _, id := range ids {
			items = append(items, f.project(id, sel))
		}
	case path == "/directoryRoles":
		for id := range f.roles {
			items = append(items, map[string]any{"id": id, "displayName": "Global Administrator"})
		}
	case len(parts) == 3 && parts[0] == "directoryRoles" && parts[2] == "members":
		for _, id := range f.roles[parts[1]] {
			items = append(items, f.project(id, sel))
		}
	}
	// One object per page: every collection is read through @odata.nextLink.
	page, _ := strconv.Atoi(strings.TrimPrefix(q.Get("$skiptoken"), skiptokenPrefix))
	body := map[string]any{"value": []any{}}
	if page < len(items) {
		body["value"] = items[page : page+1]
	}
	if page+1 < len(items) {
		q.Set("$skiptoken", skiptokenPrefix+strconv.Itoa(page+1))
		body["@odata.nextLink"] = "http://" + r.Host + r.URL.Path + "?" + q.Encode()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func sortedIDs(m map[string]map[string]any) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// jsonValues collects every value of key anywhere in a JSON document.
func jsonValues(t *testing.T, doc, key string) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, doc)
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				if k == key {
					b, _ := json.Marshal(c)
					out = append(out, strings.Trim(string(b), `"`))
				}
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
	sort.Strings(out)
	return out
}

func TestPullKeepsTheTenantAsGraphReturnsIt(t *testing.T) {
	testenv.Isolate(t)
	f := newFakeTenant()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "pullSnapshotToken-4d1e")
	db := filepath.Join(t.TempDir(), "state.db")
	run := func(args ...string) string {
		t.Helper()
		out, err := runTranscripts(t, append(args, "--db", db, "--json")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out[strings.IndexAny(out, "[{"):]
	}
	pull := func() {
		t.Helper()
		out, _ := runTranscripts(t, "pull", "--db", db, "--json", "--only", "users,groups,directory-roles")
		t.Logf("pull: %s", out)
	}

	pull()
	if got := jsonValues(t, run("admins", "audit"), "risk"); strings.Join(got, ",") != "disabled-admin,guest-admin" {
		t.Errorf("admins audit risks %v, want the disabled and the guest admin flagged (role members need $select)", got)
	}
	if got := jsonValues(t, run("licenses", "orphans"), "userPrincipalName"); strings.Join(got, ",") != "bob@contoso.test" {
		t.Errorf("licensed orphans %v, want bob", got)
	}

	// Bob's license is removed, Alice is deleted, the group is emptied and Bob's role revoked.
	f.mu.Lock()
	delete(f.users, "u1")
	f.users["u2"]["assignedLicenses"] = []any{}
	f.groups["g1"] = [2][]string{{}, {}}
	f.roles["r1"] = []string{"u3"}
	f.mu.Unlock()
	pull()
	if got := jsonValues(t, run("licenses", "orphans"), "userPrincipalName"); len(got) != 0 {
		t.Errorf("licensed orphans after the license was removed: %v", got)
	}
	if got := jsonValues(t, run("tenant", "snapshot"), "users"); strings.Join(got, ",") != "2" {
		t.Errorf("tenant snapshot users %v after a deletion, want 2", got)
	}
	if got := jsonValues(t, run("admins", "audit"), "userPrincipalName"); strings.Join(got, ",") != "carol@partner.test" {
		t.Errorf("admins after Bob's role was revoked: %v", got)
	}
	if got := jsonValues(t, run("groups", "risk"), "reasons"); !strings.Contains(strings.Join(got, ","), "ownerless") || !strings.Contains(strings.Join(got, ","), "empty") {
		t.Errorf("groups risk reasons %v, want the emptied group flagged empty and ownerless", got)
	}

	// A failed read keeps the previous snapshot: no partial users, no member-less roles.
	f.mu.Lock()
	f.refuse["/users"] = true
	f.refuse["/directoryRoles/r1/members"] = true
	f.roles["r1"] = nil
	f.mu.Unlock()
	pull()
	if got := jsonValues(t, run("tenant", "snapshot"), "users"); strings.Join(got, ",") != "2" {
		t.Errorf("tenant snapshot users %v after a refused read, want the previous 2", got)
	}
	if got := jsonValues(t, run("admins", "audit"), "userPrincipalName"); strings.Join(got, ",") != "carol@partner.test" {
		t.Errorf("admins after a refused member read: %v, want the previous snapshot", got)
	}

	// A 200 without a value array is not an empty tenant (Codex r2): the snapshot stays.
	f.mu.Lock()
	f.refuse = map[string]bool{}
	f.body["/users"] = `{}`
	f.body["/directoryRoles"] = `{"value":null}`
	f.mu.Unlock()
	pull()
	if got := jsonValues(t, run("tenant", "snapshot"), "users"); strings.Join(got, ",") != "2" {
		t.Errorf("tenant snapshot users %v after a body with no value array, want the previous 2", got)
	}
	if got := jsonValues(t, run("admins", "audit"), "userPrincipalName"); strings.Join(got, ",") != "carol@partner.test" {
		t.Errorf("admins after a role list with value null: %v, want the previous snapshot", got)
	}

	// pull writes the store, so the operator's local-data off switch refuses it (Codex r1 P2).
	t.Setenv(noLocalDataEnvVar, "1")
	if _, err := runTranscripts(t, "pull", "--db", db); err == nil || !strings.Contains(err.Error(), noLocalDataEnvVar) {
		t.Errorf("pull with %s=1: want a refusal naming it, got %v", noLocalDataEnvVar, err)
	}
}

// Codex r1 P2 data-source contract: each analytic declares its strategy at
// runtime, so a contradicting --data-source is refused instead of silently
// answered from the other source.
func TestAnalyticsRefuseTheOtherDataSource(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "dataSourceToken-8b2f")
	db := filepath.Join(t.TempDir(), "state.db")
	for _, args := range [][]string{
		{"admins", "audit", "--data-source", "live"},
		{"licenses", "orphans", "--data-source", "live"},
		{"licenses", "map", "--data-source", "live"},
		{"tenant", "snapshot", "--data-source", "live"},
	} {
		if out, err := runTranscripts(t, append(args, "--db", db)...); err == nil || !strings.Contains(err.Error()+out, "data-source") {
			t.Errorf("%v: want a data-source refusal, got %v\n%s", args, err, out)
		}
	}
	if out, err := runTranscripts(t, "apps", "consent", "--data-source", "local"); err == nil || !strings.Contains(err.Error()+out, "data-source") {
		t.Errorf("apps consent --data-source local: want a data-source refusal, got %v\n%s", err, out)
	}
}
