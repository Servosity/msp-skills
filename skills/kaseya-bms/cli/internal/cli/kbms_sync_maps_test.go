// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-bms handfixes envelope-unwrapped-for-sync, lookups-stay-in-default-sync and sync-partition-names-pinned:
// the 4.34.2 render of the vendor spec synced nothing by default (every list's item type was the
// {Success, Error, Result, TotalRecords} envelope, which has no id), dropped the 37 lookup lists, and re-pointed 16
// partitions (tickets left `servicedesk`, which became hardware assets). The novels and every upgraded store read the
// 0.1.x names. These tests fail on any regeneration that loses one of the three repairs.

import (
	"sort"
	"testing"
)

func TestDefaultSyncKeepsTheShippedPartitions(t *testing.T) {
	got := defaultSyncResources()
	if len(got) != 67 {
		t.Fatalf("defaultSyncResources has %d partitions, want the shipped 67: %v", len(got), got)
	}
	set := map[string]bool{}
	for _, r := range got {
		set[r] = true
	}
	for _, want := range []string{"servicedesk", "timelogs", "crm", "finance-contracts-summary", "system",
		"system-priorities-lookup", "system-queues-lookup", "system-statuses-lookup", "hr", "hr-assignees-lookup",
		"admin", "admin-finance-accountcodes-lookup", "project", "inventory", "integrations-surveyhistory"} {
		if !set[want] {
			sorted := append([]string(nil), got...)
			sort.Strings(sorted)
			t.Errorf("default sync lacks %q: %v", want, sorted)
		}
	}
}

func TestSyncPartitionNamesAreTheShippedOnes(t *testing.T) {
	for name, want := range map[string]string{
		"servicedesk":                            "/v2/servicedesk/tickets",
		"servicedesk-hardwareasset-searchselect": "/v2/servicedesk/hardwareasset/searchselect",
		"crm":                                    "/v2/crm/opportunities",
		"crm-accounts-lookup":                    "/v2/crm/accounts/lookup",
		"timelogs":                               "/v2/timelogs",
		"finance-contracts-summary":              "/v2/finance/contracts/summary",
		"system":                                 "/v2/system/views",
		"system-accounttypes-lookup":             "/v2/system/accounttypes/lookup",
		"hr":                                     "/v2/hr/assignees",
		"hr-assignees-lookup":                    "/v2/hr/assignees/lookup",
		"admin":                                  "/v2/admin/k1/modules",
		"admin-finance-accountcodes-lookup":      "/v2/admin/finance/accountcodes/lookup",
		"project":                                "/v2/project/projects/lookup",
		"project-projects-summary":               "/v2/project/projects/summary",
		"inventory":                              "/v2/inventory/warehouses/lookup",
		"inventory-purchaseorders-lookup":        "/v2/inventory/purchaseorders/lookup",
		"integrations":                           "/v2/integrations/itglue/status",
		"integrations-surveyhistory":             "/v2/integrations/surveyhistory",
		"servicedesk-templates-tickets-lookup":   "/v2/servicedesk/templates/tickets/lookup",
		"admin-finance-chartofaccounts-types":    "/v2/admin/finance/chartofaccounts/types",
		"admin-finance-chartofaccounts-types-lookup": "/v2/admin/finance/chartofaccounts/types/lookup",
	} {
		got, err := syncResourcePath(name)
		if err != nil || got != want {
			t.Errorf("syncResourcePath(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestTicketSyncPagesWithPageNumberAndUnwrapsResult(t *testing.T) {
	p := determinePaginationDefaults("servicedesk")
	if p.cursorParam != "PageNumber" || p.limitParam != "PageSize" || p.cursorType != "page" || p.limit != 100 {
		t.Fatalf("servicedesk paging = %+v, want PageNumber/PageSize page walk of 100", p)
	}
	if !resourceSupportsPagination("servicedesk") || !resourceSupportsPagination("timelogs") {
		t.Fatal("tickets and time logs must page")
	}
	if got := responsePathForResource("servicedesk", "/v2/servicedesk/tickets"); len(got) != 1 || got[0] != "Result" {
		t.Fatalf("tickets response path = %v, want [Result]", got)
	}
}
