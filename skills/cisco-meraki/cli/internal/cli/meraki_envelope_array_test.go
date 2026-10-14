// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/config"
)

// TestSyncUnwrapsMerakiEnvelopeArray guards the hand-fix
// meraki-sync-envelope-array: an ARRAY of {items, meta} envelopes yields the
// records inside it, while an ordinary array of records is left alone.
func TestSyncUnwrapsMerakiEnvelopeArray(t *testing.T) {
	page := json.RawMessage(`[{"items":[{"serial":"Q234-ABCD-0001"},{"serial":"Q234-ABCD-0002"}],"meta":{"counts":{"items":{"total":2}}}}]`)
	items, _, _ := extractPageItemsWithPagination(page, "", "")
	if len(items) != 2 {
		t.Fatalf("got %d items from an envelope array, want the 2 records inside it", len(items))
	}
	plain := json.RawMessage(`[{"serial":"Q234-ABCD-0001","items":["x"]},{"serial":"Q234-ABCD-0002"}]`)
	if items, _, _ := extractPageItemsWithPagination(plain, "", ""); len(items) != 2 {
		t.Fatalf("an ordinary record array was altered: %d items", len(items))
	}
}

// TestMerakiGetAllCountsEnvelopeRecords: fleet fanout reports items_returned
// from merakiGetAll, so an [{items, meta}] page must count its records.
func TestMerakiGetAllCountsEnvelopeRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"items":[{"serial":"A"},{"serial":"B"},{"serial":"C"}],"meta":{"counts":{"items":{"total":3}}}}]`))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(cfg, 10*time.Second, 0)
	c.NoCache = true
	res, err := merakiGetAll(context.Background(), c, "/organizations/1/wireless/rfProfiles/assignments/byDevice", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.ItemsReturned != 3 || len(res.Items) != 3 {
		t.Fatalf("items_returned = %d (%d items), want the 3 records inside the envelope", res.ItemsReturned, len(res.Items))
	}
}
