// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "rule-triggers-list-all-follows-array-cursor" entry.

package cli

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

type rtPagerFake struct{ paths []string }

func (f *rtPagerFake) GetWithHeaders(_ context.Context, path string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	f.paths = append(f.paths, path)
	u, _ := url.Parse(path)
	after := u.Query()["startAfter"]
	if params["startAfter"] != "" {
		return nil, nil // the cursor must travel as repeated keys, never one comma value
	}
	if len(after) == 0 {
		return json.RawMessage(`{"ruleTriggers":[{"id":"t1"},{"id":"t2"}],"pagination":{"cursor":[1760000000002,"t2"],"hasNextPage":true}}`), nil
	}
	if len(after) == 2 && after[0] == "1760000000002" && after[1] == "t2" {
		return json.RawMessage(`{"ruleTriggers":[{"id":"t3"}],"pagination":{"cursor":[1760000000003,"t3"],"hasNextPage":false}}`), nil
	}
	return json.RawMessage(`{"ruleTriggers":[],"pagination":{"hasNextPage":false}}`), nil
}

// TestRuleTriggersListAllFollowsArrayCursor: --all walks every page of
// GET /rule-triggers instead of stopping after page 1.
func TestRuleTriggersListAllFollowsArrayCursor(t *testing.T) {
	f := &rtPagerFake{}
	data, err := paginatedGet(context.Background(), f, "/respondApi/api/v1/rule-triggers?statuses%5B%5D=PENDING", map[string]string{}, nil, true, "startAfter", "cursor", "limit", 0, "pagination.cursor", "pagination.hasNextPage")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"t1", "t2", "t3"} {
		if !strings.Contains(string(data), `"`+id+`"`) {
			t.Fatalf("missing %s after --all: %s (requests %v)", id, data, f.paths)
		}
	}
	if len(f.paths) != 2 || !strings.Contains(f.paths[1], "statuses") {
		t.Fatalf("want 2 requests keeping the status filter, got %v", f.paths)
	}
}
