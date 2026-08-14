// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the POST list-endpoint path. See
// skills/threatlocker/handfixes.json (sync-post-list-endpoints).

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// fakePostClient models ThreatLocker's POST list endpoints: a paged
// {"action":"post","data":[...]} envelope whose rows key on a per-entity id
// field rather than a bare id.
type fakePostClient struct {
	total  int
	idKey  string
	calls  []map[string]any
	header []map[string]string
}

func (f *fakePostClient) PostWithHeaders(_ context.Context, _ string, body any, headers map[string]string) (json.RawMessage, int, error) {
	b, _ := body.(map[string]any)
	f.calls = append(f.calls, b)
	f.header = append(f.header, headers)

	page, size := 1, tenantSyncPageSize
	if v, ok := b["pageNumber"].(int); ok {
		page = v
	}
	if v, ok := b["pageSize"].(int); ok {
		size = v
	}
	start := (page - 1) * size
	end := start + size
	if start > f.total {
		start = f.total
	}
	if end > f.total {
		end = f.total
	}
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, fmt.Sprintf(`{%q:"guid-%d","name":"row %d"}`, f.idKey, i, i))
	}
	return json.RawMessage(fmt.Sprintf(
		`{"action":"post","data":[%s],"success":true}`, strings.Join(rows, ","),
	)), 200, nil
}

// TestTenantSyncSpecsUseRealListEndpoints pins the endpoints. A reprint that
// restores the profiler's dropdown helpers fails here.
func TestTenantSyncSpecsUseRealListEndpoints(t *testing.T) {
	for resource, want := range map[string]string{
		"computers":     "/Computer/ComputerGetByAllParameters",
		"organizations": "/Organization/OrganizationGetChildOrganizationsByParameters",
	} {
		spec, ok := tenantSyncSpecs[resource]
		if !ok {
			t.Fatalf("%s has no POST spec; sync would fall back to the id-less dropdown endpoint", resource)
		}
		if spec.path != want {
			t.Errorf("%s path = %q, want %q", resource, spec.path, want)
		}
		if strings.Contains(strings.ToLower(spec.path), "dropdown") ||
			strings.Contains(spec.path, "GetForNewComputer") ||
			strings.Contains(spec.path, "GetForMoveComputers") {
			t.Errorf("%s is pointed at a dropdown helper: %s", resource, spec.path)
		}
	}
	if got := tenantSyncSpecs["computers"].idField; got != "computerId" {
		t.Errorf("computers idField = %q, want computerId", got)
	}
	if got := tenantSyncSpecs["organizations"].idField; got != "organizationId" {
		t.Errorf("organizations idField = %q, want organizationId", got)
	}
}

// TestComputersRequestCoversChildOrganizations pins the tenancy decision: one
// request with childOrganizations=true covers the whole tree, which is why the
// path does not fan out per organization.
func TestComputersRequestCoversChildOrganizations(t *testing.T) {
	body := tenantSyncSpecs["computers"].body(1)
	if body["childOrganizations"] != true {
		t.Fatalf("computers body childOrganizations = %v, want true; without it sync sees only the authenticating organization", body["childOrganizations"])
	}
}

// TestTenantSyncRowsWalksEveryPage covers pagination across the POST envelope.
func TestTenantSyncRowsWalksEveryPage(t *testing.T) {
	f := &fakePostClient{total: tenantSyncPageSize + 37, idKey: "computerId"}
	rows, err := tenantSyncRows(context.Background(), f, tenantSyncSpecs["computers"], 0)
	if err != nil {
		t.Fatalf("tenantSyncRows: %v", err)
	}
	if len(rows) != f.total {
		t.Fatalf("fetched %d rows, want %d", len(rows), f.total)
	}
	if len(f.calls) != 2 {
		t.Fatalf("made %d requests, want 2 (a full page then a short one)", len(f.calls))
	}
	if f.calls[1]["pageNumber"] != 2 {
		t.Errorf("second request pageNumber = %v, want 2", f.calls[1]["pageNumber"])
	}
}

// TestTenantExtractRowsHandlesEnvelopes covers the response shapes the portal
// returns for these endpoints.
func TestTenantExtractRowsHandlesEnvelopes(t *testing.T) {
	for name, raw := range map[string]string{
		"data envelope":    `{"action":"post","data":[{"computerId":"a"}],"success":true}`,
		"results envelope": `{"results":[{"computerId":"a"}]}`,
		"bare array":       `[{"computerId":"a"}]`,
	} {
		rows, ok := tenantExtractRows(json.RawMessage(raw))
		if !ok || len(rows) != 1 {
			t.Errorf("%s: extracted ok=%v rows=%d, want ok=true rows=1", name, ok, len(rows))
		}
	}
	if _, ok := tenantExtractRows(json.RawMessage(`"not an object"`)); ok {
		t.Error("a scalar body should not extract as rows")
	}
}
