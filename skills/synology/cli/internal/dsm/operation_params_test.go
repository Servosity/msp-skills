// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

func TestCheckOperationParams(t *testing.T) {
	const path = "/webapi/entry.cgi?api=SYNO.Core.Share&method=list&version=1&additional=x"
	for _, bad := range []map[string]string{
		{"api": "SYNO.ActiveBackup.Task"},
		{"METHOD": "backup"},
		{"version": "9"},
		{"_sid": "other"},
		{"SynoToken": "t"},
		{"additional": "y"},
	} {
		if err := CheckOperationParams(path, bad); err == nil || !strings.Contains(err.Error(), "nothing was sent") {
			t.Fatalf("%v: err = %v, want a refusal", bad, err)
		}
	}
	if err := CheckOperationParams(path+"&method=backup", nil); err == nil {
		t.Fatal("a path naming method twice was accepted")
	}
	if err := CheckOperationParams(path, map[string]string{"offset": "0", "limit": "100", "filter": "{}"}); err != nil {
		t.Fatalf("ordinary filters refused: %v", err)
	}
}

// TestFleetClientRefusesOperationOverride: the per-NAS client the fleet
// layer uses applies the same rule, before it logs in.
func TestFleetClientRefusesOperationOverride(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", url.Values{"method": {"backup"}})
	if err == nil || atomic.LoadInt32(&requests) != 0 {
		t.Fatalf("err=%v requests=%d, want a refusal before any request", err, requests)
	}
}
