// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
)

func seedResolveFixture(t *testing.T) string {
	db, path := newTestStore(t)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","name":"jane.doe@contoso.com","kind":"user","external_id":"ext-user-1"}`)
	seed(t, db, "tenants", "ten1", `{"id":"ten1","name":"Contoso US","kind":"o365","external_id":"ext-tenant-1","region":"us"}`)
	seed(t, db, "tenants", "ten1eu", `{"id":"ten1eu","name":"Contoso EU","kind":"o365","external_id":"ext-tenant-1","region":"eu"}`)
	seed(t, db, "orgs", "org1", `{"id":"org1","name":"Contoso Holdings","kind":"customer","external_id":"ext-org-1"}`)
	return path
}

func TestResolveByExternalID(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "ext-user-1", "--db", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	if len(view.Matches) != 1 || view.Matches[0].ID != "res1" || view.Matches[0].MatchedOn != "external_id" {
		t.Errorf("external_id match wrong: %s", out)
	}
	if view.Matches[0].ObjectType != "resource" {
		t.Errorf("object_type = %q, want resource", view.Matches[0].ObjectType)
	}
}

func TestResolveByNameSubstring(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "jane.doe", "--db", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	if len(view.Matches) != 1 || view.Matches[0].ID != "res1" || view.Matches[0].MatchedOn != "name" {
		t.Errorf("name match wrong: %s", out)
	}
}

func TestResolveMultiGeoFanout(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "ext-tenant-1", "--db", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	if len(view.Matches) != 2 {
		t.Fatalf("multi-geo should return both per-region tenants: %s", out)
	}
	if !view.MultiGeo {
		t.Errorf("multi_geo flag should be set: %s", out)
	}
}

func TestResolveByAfiID(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "org1", "--db", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	if len(view.Matches) != 1 || view.Matches[0].ObjectType != "org" || view.Matches[0].MatchedOn != "id" {
		t.Errorf("afi-id match wrong: %s", out)
	}
}

func TestResolveKindFilter(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "Contoso", "--db", path, "--kind", "o365")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	for _, m := range view.Matches {
		if m.Kind != "o365" {
			t.Errorf("kind filter leaked %+v", m)
		}
	}
	if len(view.Matches) != 2 {
		t.Errorf("expected the two o365 tenants, got %d: %s", len(view.Matches), out)
	}
}

func TestResolveNoMatchNote(t *testing.T) {
	path := seedResolveFixture(t)
	out, err := runNovel(t, newNovelResolveCmd, "zzz-nope", "--db", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	view := decodeView[resolveView](t, out)
	if len(view.Matches) != 0 || view.Note == "" {
		t.Errorf("no-match should set note: %s", out)
	}
}

func TestResolveMissingArg(t *testing.T) {
	_, path := newTestStore(t)
	_, err := runNovel(t, newNovelResolveCmd, "--db", path)
	if err == nil {
		t.Fatal("expected usage error when identifier missing")
	}
}
