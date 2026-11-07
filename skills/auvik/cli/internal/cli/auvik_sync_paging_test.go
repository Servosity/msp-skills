// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written: sync-pagination class (4.35.1 reprint review, round 1).
// Every Auvik list answers at most one page and links to the rest through
// links.next carrying page[after] (auvik-v1 JSON:API paging). A sync that stops
// at page one stores a truncated mirror and, under --full, prunes every row
// beyond page one as if the vendor had deleted it.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func pagedDevice(id string) map[string]any {
	return map[string]any{"type": "device", "id": id,
		"attributes": map[string]any{"deviceName": "sw-" + id, "deviceType": "switch"}}
}

// fakePagedDeviceList answers page one (d1, d2) with links.next to
// page[after]=cur2, and page two (d3) with no next link.
func fakePagedDeviceList(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var reqs []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.URL.Path != "/v1/inventory/device/info" {
			http.NotFound(w, r)
			return
		}
		links := map[string]any{"first": srv.URL + "/v1/inventory/device/info"}
		data := []any{pagedDevice("d3")}
		if r.URL.Query().Get("page[after]") == "" {
			links["next"] = srv.URL + "/v1/inventory/device/info?page%5Bafter%5D=cur2&page%5Bfirst%5D=100"
			data = []any{pagedDevice("d1"), pagedDevice("d2")}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "links": links})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), reqs...) }
}

func TestFullSyncFollowsLinksNextAndKeepsLaterPages(t *testing.T) {
	srv, requests := fakePagedDeviceList(t)
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	// d3 is already mirrored; a full sync that stops at page one prunes it.
	seedRawRows(t, dbPath, []rawSeed{
		{"inventory-device-info", "d3", `{"id":"d3","type":"device","attributes":{"deviceName":"sw-d3"}}`},
	})

	runCLI(t, "sync", "--resources", "inventory-device-info", "--full", "--db", dbPath, "--agent", "--no-cache")

	if ids := readDeviceIDs(t, dbPath); ids != "d1,d2,d3" {
		t.Fatalf("devices after a two-page full sync = %q, want d1,d2,d3; requests=%q", ids, requests())
	}
	reqs := requests()
	if len(reqs) != 2 || !strings.Contains(reqs[1], "page%5Bafter%5D=cur2") {
		t.Fatalf("requests = %q, want page one then page[after]=cur2", reqs)
	}
	for _, q := range reqs {
		// page[last] pages backward from the end; sent beside page[first] it
		// asks for both ends at once.
		if strings.Contains(q, "page%5Blast%5D") {
			t.Fatalf("request %q carries page[last]; a forward walk must not", q)
		}
	}
}
