// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Class: an authoritative value from HubSpot is lost when the local store
// merges it with the stored row. A property HubSpot returned as "" (its
// documented clear) or null must replace the stored value on every write path
// that merges: the generic Upsert, the typed single-row Upsert<X>, and the
// batch path (sync pages and the write-through cache). A property the
// response omitted must still be kept.
func TestAuthoritativePropertiesSurviveEveryUpsertPath(t *testing.T) {
	paths := []struct {
		name  string
		typed bool
		write func(s *Store, data json.RawMessage) error
	}{
		{"generic-upsert", false, func(s *Store, data json.RawMessage) error { return s.Upsert("hubspot-deals-crm", "1", data) }},
		{"typed-upsert", true, func(s *Store, data json.RawMessage) error { return s.UpsertHubspotDealsCrm(data) }},
		{"batch-upsert", true, func(s *Store, data json.RawMessage) error {
			_, _, err := s.UpsertBatch("hubspot-deals-crm", []json.RawMessage{data})
			return err
		}},
	}
	clears := []struct{ name, raw, want string }{
		{"empty-string", `""`, ""},
		{"null", `null`, "<null>"},
	}
	for _, p := range paths {
		for _, c := range clears {
			t.Run(p.name+"/"+c.name, func(t *testing.T) {
				s, err := Open(filepath.Join(t.TempDir(), "data.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				seed := json.RawMessage(`{"id":"1","archived":false,"properties":{"hubspot_owner_id":"7","dealstage":"qualifiedtobuy","amount":"500"}}`)
				if err := p.write(s, seed); err != nil {
					t.Fatalf("seed: %v", err)
				}
				clear := json.RawMessage(`{"id":"1","archived":false,"properties":{"hubspot_owner_id":` + c.raw + `,"dealstage":` + c.raw + `}}`)
				if err := p.write(s, clear); err != nil {
					t.Fatalf("clear: %v", err)
				}
				queries := []string{`SELECT data FROM resources WHERE resource_type='hubspot-deals-crm' AND id='1'`}
				if p.typed {
					queries = append(queries, `SELECT data FROM hubspot_deals_crm WHERE id='1'`)
				}
				for _, q := range queries {
					var data, owner, stage, amount string
					if err := s.DB().QueryRow(q).Scan(&data); err != nil {
						t.Fatalf("%s: %v", q, err)
					}
					if err := s.DB().QueryRow(`SELECT COALESCE(json_extract(?1,'$.properties.hubspot_owner_id'),'<null>'), COALESCE(json_extract(?1,'$.properties.dealstage'),'<null>'), COALESCE(json_extract(?1,'$.properties.amount'),'')`, data).Scan(&owner, &stage, &amount); err != nil {
						t.Fatalf("extract %s: %v", data, err)
					}
					if owner != c.want || stage != c.want {
						t.Errorf("owner=%q dealstage=%q after HubSpot cleared both, want %q (%s)", owner, stage, c.want, q)
					}
					if amount != "500" {
						t.Errorf("amount=%q, want the stored 500 kept for a property the response omitted (%s)", amount, q)
					}
				}
			})
		}
	}
}

// The authoritative rule is scoped to the top-level HubSpot properties map: a
// present key wins (including "" and null), an absent key is kept, and every
// other key in the row keeps the generated keep-richer policy.
func TestMergeKeepRicherJSONHubSpotPropertiesScope(t *testing.T) {
	merged, ok := mergeKeepRicherJSON("hubspot-deals-crm",
		json.RawMessage(`{"id":"1","label":"kept","properties":{"a":"x","b":"y","c":"z"},"meta":{"properties":{"n":"keep"}}}`),
		json.RawMessage(`{"id":"1","label":"","properties":{"a":"","b":null},"meta":{"properties":{"n":""}}}`),
	)
	if !ok {
		t.Fatal("merge returned !ok")
	}
	var obj struct {
		Label      string          `json:"label"`
		Properties map[string]*any `json:"properties"`
		Meta       struct {
			Properties map[string]string `json:"properties"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(merged, &obj); err != nil {
		t.Fatal(err)
	}
	if v, present := obj.Properties["a"]; !present || v == nil || *v != "" {
		t.Errorf("returned \"\" must win, merged %s", merged)
	}
	if v, present := obj.Properties["b"]; !present || v != nil {
		t.Errorf("returned null must win, merged %s", merged)
	}
	if v := obj.Properties["c"]; v == nil || *v != "z" {
		t.Errorf("omitted property must be kept, merged %s", merged)
	}
	if obj.Label != "kept" {
		t.Errorf("a non-properties key must keep the generated policy, merged %s", merged)
	}
	if obj.Meta.Properties["n"] != "keep" {
		t.Errorf("only the top-level properties map is authoritative, merged %s", merged)
	}
}
