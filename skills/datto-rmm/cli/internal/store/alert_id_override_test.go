package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// alertItem mirrors the shape /v2/account/alerts/{open,resolved} returns: the
// primary key is the camelCase `alertUid` and there is no id/uid/uuid/name/
// slug/key/code field anywhere on the object (see internal/types.Alert). Before
// the resourceIDFieldOverrides entries, every generic fallback missed, the
// suffix fallback probed "account-alerts-open_id"-style keys that do not exist,
// and sync consumed N alerts while storing 0 of them (#258).
const alertItem = `{
  "alertUid": "16c466e9-9dfd-4ad1-a9cf-c02ea18eebcd",
  "alertContext": {"@class": "diskusage_ctx"},
  "priority": "High",
  "resolved": false,
  "muted": false,
  "ticketNumber": "",
  "timestamp": "1787069191000"
}`

func TestExtractResourceID_AlertUidOverride(t *testing.T) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(alertItem), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	const want = "16c466e9-9dfd-4ad1-a9cf-c02ea18eebcd"
	for _, resource := range []string{"account-alerts-open", "account-alerts-resolved"} {
		if got := ExtractResourceID(resource, obj); got != want {
			t.Errorf("ExtractResourceID(%q) = %q, want %q", resource, got, want)
		}
	}
}

// The snake_case override key must resolve the API's camelCase rendering;
// this is the only reason a "alert_uid" override can match "alertUid".
func TestLookupFieldValue_AlertUidCamelCase(t *testing.T) {
	obj := map[string]any{"alertUid": "abc"}
	if got := LookupFieldValue(obj, "alert_uid"); got != "abc" {
		t.Errorf("LookupFieldValue(alert_uid) = %v, want abc", got)
	}
}

// A paginated batch must report consumed == stored with zero extraction
// failures — the exact counter pair the #258 sync_anomaly reported as 250/0.
func TestUpsertBatch_AlertsStoreEveryRow(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	for _, resource := range []string{"account-alerts-open", "account-alerts-resolved"} {
		items := make([]json.RawMessage, 0, 25)
		for i := 0; i < 25; i++ {
			items = append(items, json.RawMessage(
				`{"alertUid": "uid-`+resource+`-`+string(rune('a'+i%26))+`", "priority": "High"}`))
		}
		stored, extractFailures, err := s.UpsertBatch(resource, items)
		if err != nil {
			t.Fatalf("UpsertBatch(%s): %v", resource, err)
		}
		if stored != len(items) {
			t.Errorf("%s: stored = %d, want %d", resource, stored, len(items))
		}
		if extractFailures != 0 {
			t.Errorf("%s: extractFailures = %d, want 0", resource, extractFailures)
		}
	}
}

// The override is scoped to the two alert resources; every other resource must
// keep resolving through the untouched generic fallback list.
func TestExtractResourceID_NonAlertResourcesUnchanged(t *testing.T) {
	cases := []struct {
		resource string
		obj      map[string]any
		want     string
	}{
		{"account-devices", map[string]any{"uid": "dev-uid", "id": "7"}, "7"},
		{"account", map[string]any{"uid": "site-uid", "name": "HQ"}, "site-uid"},
		{"account-components", map[string]any{"uid": "comp-uid"}, "comp-uid"},
		{"activity-logs", map[string]any{"id": "log-1", "uid": "ignored"}, "log-1"},
		// An alert-shaped object under a resource with no override still has
		// no extractable id — the override must not leak into the generic list.
		{"account-users", map[string]any{"alertUid": "leaked"}, ""},
	}
	for _, c := range cases {
		if got := ExtractResourceID(c.resource, c.obj); got != c.want {
			t.Errorf("ExtractResourceID(%q) = %q, want %q", c.resource, got, c.want)
		}
	}
}
