package cli

import (
	"encoding/json"
	"testing"
)

// HaloPSA answers a list endpoint with a `<Entity>_View` object carrying the
// rows next to sidecar arrays. Before the declared-items-key table, the
// generic sibling scan gave up whenever a second array was populated and the
// whole envelope fell through to the single-object path, failing the resource
// with "missing id for <resource>". /Attachment is the endpoint that always
// ships its sidecar (`folders`), which is why it broke first (#265).
func TestAttachmentEnvelopeUnwrapsRowsBesideFolders(t *testing.T) {
	body := json.RawMessage(`{"record_count":3,"page_no":1,"page_size":50,
		"attachments":[{"id":1727,"filename":"a.txt"},{"id":1577,"filename":"b.png"},{"id":4459,"filename":"c.pdf"}],
		"folders":[{"id":1,"name":"Inbox"},{"id":2,"name":"Archive"}]}`)

	items, _, _ := extractPageItems("attachment", body, "page_no")
	if len(items) != 3 {
		t.Fatalf("attachments beside a populated folders sidecar: got %d rows, want 3", len(items))
	}
	var first map[string]any
	if err := json.Unmarshal(items[0], &first); err != nil {
		t.Fatalf("unwrapped item is not an object: %v", err)
	}
	if first["filename"] != "a.txt" {
		t.Fatalf("unwrapped the wrong array: first item is %v", first)
	}
	if isEmptyPageResponse("attachment", body) {
		t.Fatal("a page with three rows must not read as empty")
	}
}

// The all-empty envelope is the shape an unscoped /Attachment returns on a
// tenant with no attachments. It has to read as an empty page: before the fix
// it reached the single-object fallback and aborted the resource.
func TestAttachmentEmptyEnvelopeIsAnEmptyPage(t *testing.T) {
	body := json.RawMessage(`{"record_count":0,"attachments":[],"folders":[]}`)

	if items, _, _ := extractPageItems("attachment", body, "page_no"); len(items) != 0 {
		t.Fatalf("empty envelope yielded %d rows", len(items))
	}
	if !isEmptyPageResponse("attachment", body) {
		t.Fatal("an empty declared items array must read as an empty page, not a single object")
	}
}

// The declared key is authoritative even when the sidecars would have made the
// old scan pick correctly by luck — this locks the resources that work today.
func TestDeclaredItemsKeyHoldsForTheSidecarHeavyViews(t *testing.T) {
	cases := []struct {
		resource string
		body     string
		want     int
		wantName string
	}{
		{"tickets", `{"record_count":2,"tickets":[{"id":11,"summary":"one"},{"id":12,"summary":"two"}],
			"columns":[{"id":1}],"statuses":[{"id":1}],"priorities":[{"id":1}],"agents":[{"id":1}]}`, 2, "summary"},
		{"clients", `{"clients":[{"id":5,"name":"Acme"}],"columns":[{"id":1},{"id":2}]}`, 1, "name"},
		{"asset", `{"assets":[{"id":7,"inventory_number":"A7"}],"columns":[{"id":1}],"xtypeunamecancreatenew":[{"id":1}]}`, 1, "inventory_number"},
		{"actions", `{"actions":[{"id":1,"ticket_id":9},{"id":2,"ticket_id":9}],"actionsdetails":[{"id":99}]}`, 2, "ticket_id"},
		{"users", `{"users":[{"id":3,"name":"Ada"}],"columns":[{"id":1}]}`, 1, "name"},
	}
	for _, c := range cases {
		items, _, _ := extractPageItems(c.resource, json.RawMessage(c.body), "page_no")
		if len(items) != c.want {
			t.Errorf("%s: got %d rows, want %d", c.resource, len(items), c.want)
			continue
		}
		var first map[string]any
		if err := json.Unmarshal(items[0], &first); err != nil {
			t.Errorf("%s: item is not an object: %v", c.resource, err)
			continue
		}
		if _, ok := first[c.wantName]; !ok {
			t.Errorf("%s: unwrapped the sidecar, not the rows: %v", c.resource, first)
		}
	}
}

// /Client/me and /Users/me return an entity, not a `_View`. Their nested
// arrays are fields of that entity, so unwrapping one would break a resource
// that works today.
func TestSingleObjectEndpointsAreNotUnwrapped(t *testing.T) {
	for _, resource := range []string{"clients-me", "users-me"} {
		body := json.RawMessage(`{"id":42,"name":"Acme","attachments":[{"id":1}],"customfields":[{"id":2}]}`)
		if items, _, _ := extractPageItems(resource, body, "page_no"); len(items) != 0 {
			t.Errorf("%s: unwrapped a single-object response into %d rows", resource, len(items))
		}
		if isEmptyPageResponse(resource, body) {
			t.Errorf("%s: a populated entity must not read as an empty page", resource)
		}
	}
}
