// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-response-path-keeps-object".

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyResponsePathKeepsMerakiObjects(t *testing.T) {
	batch := json.RawMessage(`{"id":"1000","confirmed":true,"status":{"completed":false,"failed":true,"errors":["boom"]},"actions":[{"resource":"/x","operation":"update"}]}`)
	if got := string(applyResponsePath(batch, "actions")); !strings.Contains(got, `"failed":true`) {
		t.Fatalf("action batch status dropped: %s", got)
	}
	envelope := json.RawMessage(`{"items":[{"id":"a"}],"meta":{"counts":{"items":{"total":1}}}}`)
	if got := string(applyResponsePath(envelope, "items")); got != `[{"id":"a"}]` {
		t.Fatalf("items+meta envelope not unwrapped: %s", got)
	}
}

// TestPagedResponsePathStillUnwrapsTheList: a paged list command (no --all)
// prints its array even when the page carries pagination fields beside it,
// and never leaks the internal response-path wrapper key.
func TestPagedResponsePathStillUnwrapsTheList(t *testing.T) {
	for _, tc := range []struct{ body, path string }{
		{`{"items":[{"id":"a"},{"id":"b"}],"meta":{"counts":{"items":{"total":2}}}}`, "items"},
		{`{"message":null,"pageStartAt":"2026-10-01T00:00:00Z","pageEndAt":"2026-10-02T00:00:00Z","events":[{"id":"a"},{"id":"b"}]}`, "events"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(tc.body))
		}))
		c := testMerakiClient(t, srv.URL)
		data, err := paginatedGetWithResponsePath(context.Background(), c, "/x", map[string]string{}, nil, false, "startingAfter", "cursor", "perPage", 0, "", "", tc.path)
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		var items []map[string]any
		if json.Unmarshal(data, &items) != nil || len(items) != 2 || strings.Contains(string(data), responsePathItemsKey) {
			t.Fatalf("%s page not unwrapped to its list: %s", tc.path, data)
		}
	}
}
