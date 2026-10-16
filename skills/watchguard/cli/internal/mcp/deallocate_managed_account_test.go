// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json deallocate-managed-account-query.

package mcp

import (
	"strings"
	"testing"
)

// watchguard_execute sends managedAccountId as the deallocate accountId query
// parameter; accountId itself stays the Service Provider path segment.
func TestCodeOrchDeallocateManagedAccountQuery(t *testing.T) {
	ep := findCodeOrchEndpoint("allocations.deallocate")
	if ep == nil {
		t.Fatal("allocations.deallocate missing")
	}
	params := map[string]any{"managedAccountId": "ACC-A", "productName": "WES_EDR"}
	got := codeOrchSplitQuery("/rest/platform/allocation/v2/ACC-SP/assets", ep.QueryParams, params)
	if !strings.Contains(got, "accountId=ACC-A") || len(params) != 0 {
		t.Fatalf("query = %q, leftover params %v", got, params)
	}
}
