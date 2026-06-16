// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// HAND-AUTHORED regression test for issue #114 (NOT generated — do not delete on reprint).
//
// Bug: every typed list/get query requested association/enum fields
// (accountManager, client, status, site, ...) with object sub-selections
// "{ ... }", but the live SuperOps GraphQL schema returns those fields as
// scalar leaf types (JSON/String). The server rejected every typed command
// with `SubSelectionNotAllowed`. The fix strips those sub-selections so each
// field is requested as the bare scalar it already unmarshals into — note
// every one of these fields is declared `string` (or json.RawMessage) in
// internal/types, so the bare-scalar form is the only shape consistent with
// the generated output contract.
//
// These tests pin that contract so a future reprint cannot silently
// re-introduce the sub-selections (the source spec.yaml carried the spurious
// `selection:` keys that produced them).

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"superops-pp-cli/internal/config"
)

// allListQueries is every typed query constant the bug touched.
var allListQueries = map[string]string{
	"AlertsListQuery":       AlertsListQuery,
	"AssetsGetQuery":        AssetsGetQuery,
	"AssetsListQuery":       AssetsListQuery,
	"ClientsGetQuery":       ClientsGetQuery,
	"ClientsListQuery":      ClientsListQuery,
	"ContractsListQuery":    ContractsListQuery,
	"InvoicesGetQuery":      InvoicesGetQuery,
	"InvoicesListQuery":     InvoicesListQuery,
	"ItDocsListQuery":       ItDocsListQuery,
	"KbListQuery":           KbListQuery,
	"ServiceItemsListQuery": ServiceItemsListQuery,
	"SitesListQuery":        SitesListQuery,
	"TasksGetQuery":         TasksGetQuery,
	"TasksListQuery":        TasksListQuery,
	"TechniciansListQuery":  TechniciansListQuery,
	"TicketsGetQuery":       TicketsGetQuery,
	"TicketsListQuery":      TicketsListQuery,
	"UsersListQuery":        UsersListQuery,
	"WorklogsListQuery":     WorklogsListQuery,
}

// scalarFields are the association/enum fields the live schema returns as
// leaf scalars (5 confirmed by the reporter in #114; the rest are declared
// scalar in internal/types and so must be requested bare too).
var scalarFields = []string{
	"accountManager", "primaryContact", "hqSite", "client", "site", "status",
	"priority", "requester", "technician", "techGroup", "sla", "asset",
	"category", "designation", "role", "team", "ticket",
}

// TestQueries_NoScalarSubSelections asserts no scalar field is requested with
// an object sub-selection. A field followed by optional whitespace and "{" is
// the exact shape that triggered SubSelectionNotAllowed.
func TestQueries_NoScalarSubSelections(t *testing.T) {
	for _, f := range scalarFields {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(f) + `\s*\{`)
		for name, q := range allListQueries {
			if re.MatchString(q) {
				t.Errorf("%s requests scalar field %q with a sub-selection { ... } "+
					"— this re-introduces the issue #114 SubSelectionNotAllowed bug", name, f)
			}
		}
	}
}

// TestQueries_PreserveStructuralWrappers guards against an over-strip: the
// listInfo pagination object and the nodes: entity alias must remain.
func TestQueries_PreserveStructuralWrappers(t *testing.T) {
	for name, q := range allListQueries {
		isList := strings.HasSuffix(name, "ListQuery")
		if isList && !strings.Contains(q, "listInfo {") {
			t.Errorf("%s lost its `listInfo { ... }` pagination wrapper", name)
		}
		if isList && !strings.Contains(q, "nodes:") {
			t.Errorf("%s lost its `nodes:` entity alias", name)
		}
	}
}

// captureRoundTripper records the last request body and returns a canned 200.
type captureRoundTripper struct {
	lastBody []byte
	respBody string
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		c.lastBody, _ = io.ReadAll(req.Body)
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(c.respBody)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// TestClientsList_ScalarResponseRoundTrips proves the corrected ClientsListQuery
// (a) sends accountManager/primaryContact/hqSite as bare scalars, and (b) a
// scalar-shaped getClientList response parses cleanly via the normal Query path.
func TestClientsList_ScalarResponseRoundTrips(t *testing.T) {
	// A realistic SuperOps response: associations are JSON-scalar blobs, not
	// typed objects; status is a String.
	resp := `{"data":{"getClientList":{"nodes":[{` +
		`"accountId":"1","name":"Acme","stage":"ACTIVE","status":"ACTIVE",` +
		`"emailDomains":["acme.com"],` +
		`"accountManager":{"userId":"u1","name":"Pat"},` +
		`"primaryContact":{"userId":"u2","name":"Sam"},` +
		`"hqSite":{"id":"s1","name":"HQ"}}],` +
		`"listInfo":{"totalCount":1,"hasMore":false,"page":1,"pageSize":100}}}}`

	rt := &captureRoundTripper{respBody: resp}
	cfg := &config.Config{BaseURL: "http://superops.test"}
	c := New(cfg, time.Second, 0)
	c.HTTPClient = &http.Client{Transport: rt}
	c.NoCache = true

	data, err := c.Query(context.Background(), ClientsListQuery, map[string]any{"first": 100})
	if err != nil {
		t.Fatalf("Query returned error on a scalar response: %v", err)
	}

	// (a) the outgoing query must request the scalars bare.
	sent := string(rt.lastBody)
	for _, f := range []string{"accountManager", "primaryContact", "hqSite"} {
		if !strings.Contains(sent, f) {
			t.Errorf("outgoing query missing field %q", f)
		}
		if regexp.MustCompile(`\b` + f + `\s*\{`).MatchString(sent) {
			t.Errorf("outgoing query still sub-selects %q (would 400 with SubSelectionNotAllowed)", f)
		}
	}

	// (b) the scalar payload survives the normal decode path intact.
	if !bytes.Contains(data, []byte("Acme")) {
		t.Errorf("parsed data missing client name; got %s", data)
	}
	var probe struct {
		GetClientList struct {
			Nodes []json.RawMessage `json:"nodes"`
		} `json:"getClientList"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("unmarshal getClientList: %v", err)
	}
	if len(probe.GetClientList.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(probe.GetClientList.Nodes))
	}
}
