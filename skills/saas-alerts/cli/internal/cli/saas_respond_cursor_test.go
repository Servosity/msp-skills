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

type unifyPagerFake struct {
	paths []string
	bare  bool
}

func (f *unifyPagerFake) GetWithHeaders(_ context.Context, path string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	f.paths = append(f.paths, path)
	if params["searchAfter"] != "" {
		return nil, nil // the cursor must travel as repeated keys
	}
	if f.bare {
		return json.RawMessage(`[{"id":"d1"},{"id":"d2"}]`), nil
	}
	u, _ := url.Parse(path)
	switch after := u.Query()["searchAfter"]; {
	case len(after) == 0:
		return json.RawMessage(`{"data":[{"id":"d1"},{"id":"d2"}],"meta":{"sort":[1760000000000,"d2"]}}`), nil
	case len(after) == 2 && after[1] == "d2":
		return json.RawMessage(`{"data":[{"id":"d3"}],"meta":{"sort":[1760000000001,"d3"]}}`), nil
	default:
		return json.RawMessage(`{"data":[],"meta":{}}`), nil
	}
}

// TestDevicesListAllFollowsSearchAfter: devices list --all walks every Unify
// page via meta.sort, and a full bare page is never reported complete.
func TestDevicesListAllFollowsSearchAfter(t *testing.T) {
	f := &unifyPagerFake{}
	data, err := paginatedGet(context.Background(), f, "/manageApi/api/v1/unify/devices", map[string]string{"pageSize": "2"}, nil, true, "searchAfter", "cursor", "pageSize", 0, "meta.sort", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"d1", "d2", "d3"} {
		if !strings.Contains(string(data), `"`+id+`"`) {
			t.Fatalf("missing %s: %s (requests %v)", id, data, f.paths)
		}
	}
	if !unifyFullBarePage("2", 2) || unifyFullBarePage("1000", 700) || !unifyFullBarePage("", 100) {
		t.Fatal("unifyFullBarePage thresholds wrong")
	}
}
