// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Open P1s from the 2026-10-24 factory Codex review round 2.

import (
	"encoding/json"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// [P1 with-history snapshot] a history clear ("") survives the store merge.
func TestR2HistoryClearSurvivesTheStoreMerge(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"results":[{"id":"1","properties":{"dealname":"A","dealstage":"qualifiedtobuy"},"propertiesWithHistory":{"hubspot_owner_id":[{"value":"","timestamp":"2026-10-24T14:00:00Z","sourceType":"CRM_UI"},{"value":"7","timestamp":"2026-10-01T00:00:00Z","sourceType":"CRM_UI"}]}}]}`)
	s, path := r1Store(t)
	r1Deal(t, s, "1", map[string]any{"dealname": "A", "dealstage": "qualifiedtobuy", "hubspot_owner_id": "7"})
	s.Close()
	if _, err := r1Run(t, "", "sync", "--resources", "hubspot-deals-crm", "--with-history", "hubspot_owner_id", "--db", path, "--json"); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var owner string
	if err := s2.DB().QueryRow(`SELECT COALESCE(json_extract(data,'$.properties.hubspot_owner_id'),'<null>') FROM hubspot_deals_crm WHERE id='1'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "" {
		t.Fatalf("owner = %q after HubSpot cleared it", owner)
	}
}

// Class: an authoritative value from HubSpot is lost when the local store
// merges it with the stored row. Every member below seeds deal 1 owned by an
// ACTIVE owner 7 (so before the fix `deals unowned` correctly hides it), syncs
// a response that clears the owner, and asserts the clear reached the generic
// row, the typed row and the `deals unowned` answer an MSP acts on. The
// response omits amount, so each member also proves a property absent from the
// response is still kept (the reason the keep-richer merge exists).

func r2SeedOwnedDeal(t *testing.T) string {
	t.Helper()
	s, path := r1Store(t)
	r1Pipeline(t, s)
	r1Upsert(t, s.UpsertHubspotOwnersCrm, map[string]any{"id": "7", "email": "rep@example.com", "archived": false})
	r1Deal(t, s, "1", map[string]any{"dealname": "A", "amount": "500", "dealstage": "qualifiedtobuy", "pipeline": "default", "hubspot_owner_id": "7"})
	s.Close()
	return path
}

func r2AssertOwnerCleared(t *testing.T, path, want string) {
	t.Helper()
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT COALESCE(json_extract(data,'$.properties.hubspot_owner_id'),'<null>'), COALESCE(json_extract(data,'$.properties.amount'),'') FROM resources WHERE resource_type='hubspot-deals-crm' AND id='1'`,
		`SELECT COALESCE(json_extract(data,'$.properties.hubspot_owner_id'),'<null>'), COALESCE(json_extract(data,'$.properties.amount'),'') FROM hubspot_deals_crm WHERE id='1'`,
	} {
		var owner, amount string
		if err := s.DB().QueryRow(q).Scan(&owner, &amount); err != nil {
			s.Close()
			t.Fatalf("%s: %v", q, err)
		}
		if owner != want {
			t.Errorf("owner = %q after HubSpot cleared it, want %q (%s)", owner, want, q)
		}
		if amount != "500" {
			t.Errorf("amount = %q, want the stored 500 kept for a property the response omitted (%s)", amount, q)
		}
	}
	s.Close()
	out, err := r1Run(t, "", "deals", "unowned", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Deals []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"deals"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode deals unowned: %v\n%s", err, out)
	}
	for _, d := range view.Deals {
		if d.ID == "1" && d.Reason == "unassigned" {
			return
		}
	}
	t.Fatalf("deals unowned omits deal 1 after HubSpot cleared its owner: %s", out)
}

// Member: the --with-history fill ("" newest entry) through the batch sync path.
func TestR2HistoryClearShowsInDealsUnowned(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"results":[{"id":"1","properties":{"dealname":"A","dealstage":"qualifiedtobuy","pipeline":"default"},"propertiesWithHistory":{"hubspot_owner_id":[{"value":"","timestamp":"2026-10-24T14:00:00Z","sourceType":"CRM_UI"},{"value":"7","timestamp":"2026-10-01T00:00:00Z","sourceType":"CRM_UI"}]}}]}`)
	path := r2SeedOwnedDeal(t)
	if _, err := r1Run(t, "", "sync", "--resources", "hubspot-deals-crm", "--with-history", "hubspot_owner_id", "--db", path, "--json"); err != nil {
		t.Fatal(err)
	}
	r2AssertOwnerCleared(t, path, "")
}

// Members: a regular sync (no history) where HubSpot returns the property in
// properties as "" (its documented clear) or null (a nullable string value).
func TestR2ReturnedPropertyClearSurvivesTheStoreMerge(t *testing.T) {
	for _, tc := range []struct{ name, owner, want string }{
		{"empty-string", `""`, ""},
		{"null", `null`, "<null>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			newR1Server(t, 200, `{"results":[{"id":"1","properties":{"dealname":"A","dealstage":"qualifiedtobuy","pipeline":"default","hubspot_owner_id":`+tc.owner+`}}]}`)
			path := r2SeedOwnedDeal(t)
			if _, err := r1Run(t, "", "sync", "--resources", "hubspot-deals-crm", "--db", path, "--json"); err != nil {
				t.Fatal(err)
			}
			r2AssertOwnerCleared(t, path, tc.want)
		})
	}
}

// Member: the write-through of a mutation response. HubSpot documents that a
// PATCH clears a property passed as "", and the CLI caches the PATCH response
// into the local mirror; the cached clear must reach the stored deal.
func TestR2PatchClearReachesTheLocalMirror(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"id":"1","archived":false,"properties":{"hubspot_owner_id":"","hs_lastmodifieddate":"2026-10-24T14:00:00Z"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-10-24T14:00:00Z"}`)
	path := defaultDBPath("hubspot-cli")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r1Pipeline(t, s)
	r1Upsert(t, s.UpsertHubspotOwnersCrm, map[string]any{"id": "7", "email": "rep@example.com", "archived": false})
	r1Deal(t, s, "1", map[string]any{"dealname": "A", "amount": "500", "dealstage": "qualifiedtobuy", "pipeline": "default", "hubspot_owner_id": "7"})
	s.Close()
	if _, err := r1Run(t, "", "hubspot-deals-crm", "patch-v3-objects-0-3-deal-id-update", "1", "--properties", `{"hubspot_owner_id":""}`, "--yes", "--json"); err != nil {
		t.Fatal(err)
	}
	r2AssertOwnerCleared(t, path, "")
}
