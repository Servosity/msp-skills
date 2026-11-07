// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

// Codex review r1 class "incomplete paging read as complete" (datto-rmm
// handfix fleet-live-reads-walk-every-page): `fleet sprawl --refresh` read one
// SoftwarePage per device and ignored its documented
// pageDetails.nextPageUrl, so software on later pages vanished from the
// report; a failed fetch was skipped silently. Every page is read, and a
// device whose pages cannot all be read keeps its last synced software and
// is reported.
func TestSprawlRefreshReadsEverySoftwarePage(t *testing.T) {
	var mu sync.Mutex
	var asks []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asks = append(asks, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v2/audit/device/d1/software":
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write([]byte(`{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":null},"software":[{"name":"VulnerableApp","version":"2.0"}]}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"pageDetails":{"count":1,"totalCount":2,"prevPageUrl":null,"nextPageUrl":"https://merlot-api.centrastage.net/api/v2/audit/device/d1/software?page=1&max=250"},"software":[{"name":"OtherApp","version":"1"}]}`)
		case "/api/v2/audit/device/d2/software":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	db := filepath.Join(t.TempDir(), "fleet.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch(fleetDevicesResource, []json.RawMessage{
		json.RawMessage(`{"id":1,"uid":"d1","hostname":"PC1","siteName":"Acme"}`),
		json.RawMessage(`{"id":2,"uid":"d2","hostname":"PC2","siteName":"Acme"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(fleetSoftwareResource, "d2", json.RawMessage(`{"software":[{"name":"VulnerableApp","version":"1.0"}]}`)); err != nil {
		t.Fatal(err)
	}
	s.Close()

	stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "sprawlToken-5d5d"},
		"fleet", "sprawl", "--refresh", "--name", "VulnerableApp", "--json", "--db", db)
	if err != nil {
		t.Fatalf("fleet sprawl --refresh: %v\n%s", err, stderr)
	}
	var view []sprawlView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("output: %v\n%s", err, stdout)
	}
	versions := map[string]int{}
	for _, v := range view {
		versions[v.Version] += v.DeviceCount
	}
	if versions["2.0"] != 1 {
		t.Fatalf("the installation on the linked software page is missing (requests %v): %+v", asks, view)
	}
	if versions["1.0"] != 1 {
		t.Fatalf("a device whose refresh failed lost its last synced software: %+v", view)
	}
	if !strings.Contains(stderr, "d2") && !strings.Contains(stderr, "1 of 2") {
		t.Fatalf("the failed refresh was not reported:\n%s", stderr)
	}
}
