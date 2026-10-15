// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"watchguard-pp-cli/internal/store"
)

// hand-fix (handfixes.json endpoint-security-odata-paging): sync of the
// Endpoint Security lists against a server that, like the API, rejects the
// plain query names (HTTP 400) succeeds, pages with $skip, and stores every
// row under its device identity.
func TestEndpointSecuritySyncUsesODataAndStoresRows(t *testing.T) {
	const total = 150
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		q := r.URL.Query()
		for _, plain := range []string{"count", "skip", "top"} {
			if _, ok := q[plain]; ok {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"unknown query option; use $top/$skip"}`))
				return
			}
		}
		top, _ := strconv.Atoi(q.Get("$top"))
		skip, _ := strconv.Atoi(q.Get("$skip"))
		rows := []string{}
		for i := skip; i < total && i < skip+top; i++ {
			if strings.HasSuffix(r.URL.Path, "/patchavailability") {
				rows = append(rows, fmt.Sprintf(`{"device_id":"d%d","host_name":"h%d","patch_id":"p1","patch_name":"KB1"}`, i, i))
			} else {
				rows = append(rows, fmt.Sprintf(`{"device_id":"d%d","host_name":"h%d"}`, i, i))
			}
		}
		_, _ = w.Write([]byte(`{"data":[` + strings.Join(rows, ",") + `],"total_items":` + strconv.Itoa(total) + `}`))
	}))
	defer srv.Close()
	spEnv(t, srv)
	_, code := runJSON(t, "sync", "--resources", "endpoint_devices-devicesprotectionstatus,endpoint_patches,endpoint_unmanaged_devices", "--full", "--json")
	if code != 0 {
		t.Fatalf("endpoint security sync: exit %d", code)
	}
	db, err := store.OpenReadOnly(defaultDBPath("watchguard-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, rt := range []string{"endpoint_devices-devicesprotectionstatus", "endpoint_patches", "endpoint_unmanaged_devices"} {
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != total {
			t.Errorf("%s: stored %d rows, want %d", rt, n, total)
		}
	}
	var raw string
	_ = db.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'endpoint_patches' AND id LIKE 'd149%'`).Scan(&raw)
	if !json.Valid([]byte(raw)) {
		t.Fatalf("patch row for d149 not keyed on device_id+patch: %q", raw)
	}
}
