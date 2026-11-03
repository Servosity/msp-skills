// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json deallocate-managed-account-query.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The deallocate path names the Service Provider and the required accountId
// query parameter names the managed account; both are sent.
func TestDeallocateSendsManagedAccountQuery(t *testing.T) {
	var path, managed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		path, managed = r.URL.Path, r.URL.Query().Get("accountId")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	spEnv(t, srv)
	if _, code := runJSON(t, "allocations", "deallocate", "--managed-account-id", "ACC-A", "--product-name", "WES_EDR", "--allocation-type", "term", "--json"); code != 0 {
		t.Fatalf("deallocate exit %d", code)
	}
	if path != "/rest/platform/allocation/v2/ACC-SP/assets" || managed != "ACC-A" {
		t.Fatalf("sent path %q accountId %q; want the SP path and accountId=ACC-A", path, managed)
	}
	if _, code := runJSON(t, "allocations", "deallocate", "--product-name", "WES_EDR", "--allocation-type", "term", "--json"); code == 0 {
		t.Fatalf("deallocate without --managed-account-id must fail")
	}
}
