// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// organization_devices fetches /v2/organization/{id}/devices once per
// organization. Its parent partition "organization" holds
// /v2/organization/documents rows keyed on documentId (spec.yaml
// ClientDocumentWithAttributeValuesPublicApiDTO), so fanning out over that
// partition's ids requested document ids as organization ids and skipped every
// organization without a document. The organization ids are the
// "organizations" partition's (/v2/organizations).
func TestOrganizationDevicesFanOutOverOrganizations(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		w.Header().Set("Content-Type", "application/json")
		var org int
		if _, err := fmt.Sscanf(r.URL.Path, "/v2/organization/%d/devices", &org); err != nil {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `[{"id":%d,"organizationId":%d,"systemName":"PC-%d"}]`, org*100, org, org)
	}))
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("organizations", []json.RawMessage{
		json.RawMessage(`{"id":3,"name":"Acme"}`), json.RawMessage(`{"id":4,"name":"Globex"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("organization", []json.RawMessage{
		json.RawMessage(`{"documentId":900,"organizationId":3,"documentName":"Runbook"}`),
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	runNinjaSyncArgsAt(t, srv.URL, dbPath, "--resources", "organization_devices")
	var paths []string
	for _, r := range f.requests {
		paths = append(paths, strings.SplitN(r, "?", 2)[0])
	}
	sort.Strings(paths)
	want := []string{"/v2/organization/3/devices", "/v2/organization/4/devices"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("organization_devices requested %v, want %v", paths, want)
	}
	if n := storedRows(t, dbPath, "organization_devices"); n != 2 {
		t.Fatalf("organization_devices holds %d rows, want 2", n)
	}
}
