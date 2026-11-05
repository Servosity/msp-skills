// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch r1 (handfixes.json hubspot-reports-read-only-collected-properties):
// HubSpot returns only the properties a read names, or the object's defaults
// when it names none (press/tools-manifest.json, the "properties" query
// parameter of every /crm/v3/objects list and read). So sync and the live
// reads request every property a report reads, and a stored record that
// lacks one was never collected: the report refuses and names the full sync,
// never reading the absence as empty.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

// nbDefaultProperties are what HubSpot answers when a read names no
// properties (probed live 2026-10-27, .review/nb-probe-props.txt).
var nbDefaultProperties = map[string][]string{
	"0-3":      {"amount", "closedate", "createdate", "dealname", "dealstage", "hs_lastmodifieddate", "hs_object_id", "pipeline"},
	"contacts": {"createdate", "email", "firstname", "hs_object_id", "lastmodifieddate", "lastname"},
	"meetings": {"hs_createdate", "hs_lastmodifieddate", "hs_object_id"},
}

type nbPortal struct {
	mu        sync.Mutex
	requested map[string][]string // object -> every properties= it was asked for
}

// newNBPortal serves records the way HubSpot does: a list or read returns
// each requested property (null when the record has no value) or, when none
// are requested, the object's defaults.
func newNBPortal(t *testing.T, records map[string][]map[string]string) *nbPortal {
	t.Helper()
	p := &nbPortal{requested: map[string][]string{}}
	shape := func(obj string, rec map[string]string, props string) string {
		names := nbDefaultProperties[obj]
		if props != "" {
			names = strings.Split(props, ",")
		}
		out := map[string]any{}
		for _, n := range names {
			if v, ok := rec[n]; ok {
				out[n] = v
			} else {
				out[n] = nil
			}
		}
		b, _ := json.Marshal(map[string]any{"id": rec["hs_object_id"], "properties": out, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-10-20T00:00:00Z", "archived": false})
		return string(b)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// HubSpot takes properties comma-separated or repeated (form explode).
		props := strings.Join(r.URL.Query()["properties"], ",")
		switch {
		case r.URL.Path == "/crm/v3/owners":
			_, _ = fmt.Fprint(w, `{"results":[{"id":"7","email":"owner@example.com","firstName":"Olive","lastName":"Owner","archived":false}]}`)
			return
		case strings.HasPrefix(r.URL.Path, "/crm/v4/objects/contacts/1/associations/meetings"):
			_, _ = fmt.Fprint(w, `{"results":[{"toObjectId":31,"associationTypes":[]}]}`)
			return
		case strings.HasPrefix(r.URL.Path, "/crm/v3/objects/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/crm/v3/objects/"), "/")
			obj := parts[0]
			p.mu.Lock()
			p.requested[obj] = append(p.requested[obj], props)
			p.mu.Unlock()
			if len(parts) == 2 {
				for _, rec := range records[obj] {
					if rec["hs_object_id"] == parts[1] {
						_, _ = fmt.Fprint(w, shape(obj, rec, props))
						return
					}
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"status":"error","message":"not found"}`)
				return
			}
			recs := make([]string, 0, len(records[obj]))
			for _, rec := range records[obj] {
				recs = append(recs, shape(obj, rec, props))
			}
			_, _ = fmt.Fprint(w, `{"results":[`+strings.Join(recs, ",")+`]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"results":[]}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a32")
	return p
}

var nbOwnedDeal = map[string][]map[string]string{
	"0-3": {{"hs_object_id": "1", "dealname": "Owned", "amount": "10000", "dealstage": "qualifiedtobuy", "pipeline": "default", "hubspot_owner_id": "7", "hs_is_closed": "false"}},
	"contacts": {
		{"hs_object_id": "1", "firstname": "Ada", "lifecyclestage": "customer", "hubspot_owner_id": "7"},
		{"hs_object_id": "2", "firstname": "Bo", "lifecyclestage": "lead", "hubspot_owner_id": "7"},
	},
	"meetings": {{"hs_object_id": "31", "hs_meeting_title": "Quarterly review", "hs_meeting_outcome": "COMPLETED"}},
}

// The Codex scenario: an open $10,000 deal owned by 7, synced into a fresh
// store, is not "unassigned"; contacts keep their lifecycle stage. A user
// --param properties= adds to what sync collects, never narrows it.
func TestNBSyncCollectsWhatReportsRead(t *testing.T) {
	for name, extra := range map[string][]string{
		"default":    nil,
		"user-param": {"--param", "properties=description"},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			newNBPortal(t, nbOwnedDeal)
			path := filepath.Join(t.TempDir(), "fresh.db")
			args := append([]string{"sync", "--resources", "hubspot-owners-crm,hubspot-deals-crm,hubspot-contacts-crm", "--db", path, "--json"}, extra...)
			if _, stderr, err := r3dRun(t, args...); err != nil {
				t.Fatalf("sync: %v\n%s", err, stderr)
			}
			out, stderr, err := r3dRun(t, "deals", "unowned", "--db", path, "--json")
			if err != nil {
				t.Fatalf("deals unowned: %v\n%s", err, stderr)
			}
			var view unownedView
			if err := json.Unmarshal([]byte(out), &view); err != nil {
				t.Fatalf("decode %q: %v", out, err)
			}
			if len(view.Deals) != 0 || view.TotalExposure != 0 {
				t.Fatalf("deals unowned = %+v, want none: deal 1 is owned by 7\n%s", view, out)
			}
			out, stderr, err = r3dRun(t, "contacts", "funnel", "--db", path, "--json")
			if err != nil {
				t.Fatalf("contacts funnel: %v\n%s", err, stderr)
			}
			if !strings.Contains(out, `"stage": "customer"`) || !strings.Contains(out, `"stage": "lead"`) {
				t.Fatalf("contacts funnel lost the lifecycle stages:\n%s", out)
			}
		})
	}
}

// A live timeline names the engagement properties it shows.
func TestNBLiveEngagementsReadTheirTitles(t *testing.T) {
	testenv.Isolate(t)
	newNBPortal(t, nbOwnedDeal)
	_, path := r1Store(t)
	out, stderr, err := r3dRun(t, "engagements", "of", "contact:1", "--type", "meetings", "--db", path, "--json", "--rate-limit", "0")
	if err != nil {
		t.Fatalf("engagements of: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, "Quarterly review") || !strings.Contains(out, "COMPLETED") {
		t.Fatalf("live timeline lost the meeting title and outcome:\n%s", out)
	}
}

// A stored record without a property a report reads (an older build's sync,
// a narrow live read) was never collected: the report refuses and names the
// full sync instead of reporting the deal unassigned.
func TestNBReportRefusesAnUncollectedProperty(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	if err := s.UpsertHubspotDealsCrm(json.RawMessage(`{"id":"1","archived":false,"properties":{"dealname":"Owned","amount":"10000","dealstage":"qualifiedtobuy","pipeline":"default","hs_lastmodifieddate":"2026-10-20T00:00:00Z"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-10-20T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	out, stderr, err := r3dRun(t, "deals", "unowned", "--db", path, "--json")
	if err == nil {
		t.Fatalf("deals unowned answered over an uncollected hubspot_owner_id:\n%s", out)
	}
	msg := err.Error() + stderr
	for _, want := range []string{"hubspot_owner_id", "hubspot-cli sync --resources hubspot-deals-crm --full"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q does not name %q", msg, want)
		}
	}
}

// nbAsSynced is a fixture record as this build's sync stores it: every
// property sync collects (defaults included) is present, null when HubSpot
// has no value. A null
// and an absent key read the same through json_extract, so only the
// collected-property guard can tell them apart.
func nbAsSynced(props map[string]any) map[string]any {
	for resource := range hsSyncProperties {
		for _, n := range strings.Split(hsCollectedProperties(resource, ""), ",") {
			if _, ok := props[n]; !ok {
				props[n] = nil
			}
		}
	}
	return props
}

// nbHonourProperties answers a stub read like HubSpot: every property the
// request names is on each returned record, null when it has no value.
func nbHonourProperties(r *http.Request, body string) string {
	props := r.URL.Query().Get("properties")
	if props == "" {
		return body
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v map[string]any
	if dec.Decode(&v) != nil {
		return body
	}
	recs, ok := v["results"].([]any)
	if !ok {
		recs = []any{v}
	}
	for _, rec := range recs {
		m, _ := rec.(map[string]any)
		p, _ := m["properties"].(map[string]any)
		for _, n := range strings.Split(props, ",") {
			if _, ok := p[n]; p != nil && !ok {
				p[n] = nil
			}
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

var nbPropertyRead = regexp.MustCompile(`\$\.properties\.([a-z0-9_]+)|PropSQL\([^,]+, "([a-z0-9_]+)"\)|Properties\["([a-z0-9_]+)"\]`)

// nbNeverDefined are read only as a fallback and are not a property of any
// HubSpot object, so no read can collect them.
var nbNeverDefined = map[string]bool{"hs_engagement_title": true}

// Structural: every property a report reads is one sync collects for some
// object, so a new report reading an uncollected property fails here.
func TestNBEveryReadPropertyIsCollected(t *testing.T) {
	collected := map[string]bool{}
	for resource := range hsSyncProperties {
		for _, p := range strings.Split(hsCollectedProperties(resource, ""), ",") {
			collected[p] = true
		}
	}
	for resource, props := range hsReportProperties {
		for _, p := range props {
			if !strings.Contains(","+hsCollectedProperties(resource, "")+",", ","+p+",") {
				t.Errorf("%s: report property %s is not requested by sync", resource, p)
			}
		}
	}
	files, _ := filepath.Glob("*.go")
	var missing []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range nbPropertyRead.FindAllStringSubmatch(string(src), -1) {
			p := m[1] + m[2] + m[3]
			if !collected[p] && !nbNeverDefined[p] {
				missing = append(missing, f+": "+p)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("reports read properties no sync collects:\n%s", strings.Join(missing, "\n"))
	}
}

// A --filter names a property too: on records synced without it, NOT_HAS
// would match every record and an equality none. The report refuses and
// names the full sync that adds it; a collected property filters as before.
func TestNBFilterOnAnUncollectedPropertyRefuses(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "1", "archived": false, "properties": map[string]any{"firstname": "Ada", "lifecyclestage": "customer", "notes_last_contacted": "2026-01-01T00:00:00Z"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"})
	s.Close()
	out, stderr, err := r3dRun(t, "stale", "contacts", "--days", "30", "--filter", "lifecyclestage=customer !hs_email_optout", "--db", path, "--json")
	if err == nil {
		t.Fatalf("stale contacts filtered on an uncollected hs_email_optout:\n%s", out)
	}
	msg := err.Error() + stderr
	if !strings.Contains(msg, "hubspot-cli sync --resources hubspot-contacts-crm --full --param properties=hs_email_optout") {
		t.Errorf("refusal %q does not name the sync that collects the filter property", msg)
	}
	out, stderr, err = r3dRun(t, "stale", "contacts", "--days", "30", "--filter", "lifecyclestage=customer !hubspot_owner_id", "--db", path, "--json")
	if err != nil || !strings.Contains(out, `"id": "1"`) {
		t.Fatalf("a filter on collected properties: %v\n%s\n%s", err, out, stderr)
	}
}

// Nextfix2: a default property is read like any other. In a fresh store with
// owners collected, a projected live read (press/tools-manifest.json:1624,
// the "properties" of get-v3-objects-0-3-deal-id-get-by-id) stores the deal
// without amount, one of HubSpot's defaults: owner-load refuses instead of
// summing the missing amount as $0, and the sync it names clears it.
func TestNB2ReportRefusesAnUncollectedDefaultProperty(t *testing.T) {
	testenv.Isolate(t)
	newNBPortal(t, nbOwnedDeal)
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-owners-crm", "--json"); err != nil {
		t.Fatalf("sync owners: %v\n%s", err, stderr)
	}
	if _, stderr, err := r3dRun(t, "hubspot-deals-crm", "get-v3-objects-0-3-deal-id-get-by-id", "1", "--properties", "pipeline,dealstage,hs_is_closed,hubspot_owner_id,notes_last_contacted,notes_last_updated", "--json"); err != nil {
		t.Fatalf("projected read: %v\n%s", err, stderr)
	}
	out, stderr, err := r3dRun(t, "owner-load", "--pipeline", "default", "--json")
	if err == nil {
		t.Fatalf("owner-load summed a deal stored without amount:\n%s", out)
	}
	msg := err.Error() + stderr
	for _, want := range []string{"amount (1 records)", "hubspot-cli sync --resources hubspot-deals-crm --full"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q does not name %q", msg, want)
		}
	}
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--full", "--json"); err != nil {
		t.Fatalf("full sync: %v\n%s", err, stderr)
	}
	if out, stderr, err := r3dRun(t, "owner-load", "--pipeline", "default", "--json"); err != nil || !strings.Contains(out, `"total_amount": 10000`) {
		t.Fatalf("owner-load after the named sync: %v\n%s\n%s", err, out, stderr)
	}
}
