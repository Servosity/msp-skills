// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 (appdirect 4.34.2 reprint), wrong results an MSP would
// act on, from response shapes AppDirect documents (spec.yaml). Each test was
// written to fail on the code before its fix:
//   - sync-read-partition-mismatch: every generated endpoint command named
//     its command group as the store partition, so the company list read
//     (and its automatic reads wrote) the partition sync fills with
//     /account/v2/users (PagedCompanyAccountV2 vs PagedUserAccountV2).
//   - incomplete-paging-read-as-complete: nine partitions that page with
//     number/size (OpportunityPage and the other PageMetadata lists) were
//     synced as one page and declared complete, and a short page ended any
//     walk even when the page metadata said more pages exist.
//   - wrong-field-read-from-a-documented-response: company show attributed
//     users by a `company` member UserAccountV2 does not have, so every
//     customer reported zero users.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/cliutil/testenv"
	"appdirect-pp-cli/internal/store"
)

// Codex r1 failing input: sync account and account-v2-companies, then
// `account resource-company-read-all-companies-get --data-source local --json`.
func TestR1CompanyListReadsTheCompanyPartition(t *testing.T) {
	testenv.Isolate(t)
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("appdirect-cli"))
	if err != nil {
		t.Fatal(err)
	}
	seedNovelResource(t, db, "account", "user-uuid-71a2", map[string]any{"uuid": "user-uuid-71a2", "email": "ann@customer.example", "firstName": "Ann"})
	seedNovelResource(t, db, "account-v2-companies", "company-uuid-5c3e", map[string]any{"uuid": "company-uuid-5c3e", "name": "Customer Company"})
	db.Close()

	for _, c := range []struct {
		args       []string
		want, deny string
	}{
		{[]string{"account", "resource-company-read-all-companies-get", "--data-source", "local", "--json"}, "company-uuid-5c3e", "user-uuid-71a2"},
		{[]string{"account", "resource-user-read-all-users-get", "--data-source", "local", "--json"}, "user-uuid-71a2", "company-uuid-5c3e"},
	} {
		cmd := RootCmd()
		var out, errOut strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(c.args)
		err := cmd.Execute()
		if !strings.Contains(out.String(), c.want) || strings.Contains(out.String(), c.deny) {
			t.Errorf("%s: printed %q (err %v, stderr %q); want %s and never %s", strings.Join(c.args[:2], " "), out.String(), err, errOut.String(), c.want, c.deny)
		}
	}
}

// pagedFake serves AppDirect PageMetadata lists ({"content":[...],"page":
// {"number","size","totalElements","totalPages"}}) from a fixed set of
// records, reading the page from the number (or page) parameter and the size
// from size. base is the first page's number (1 for OpportunityPage). maxSize
// caps the page size the way a server does; ignorePaging serves page one for
// every request.
type pagedFake struct {
	total, base, maxSize int
	pageParam            string
	ignorePaging         bool
	defaultSize          int
	gets                 int
}

func (f *pagedFake) RateLimit() float64 { return 0 }

func (f *pagedFake) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	f.gets++
	if f.gets > 50 {
		return nil, fmt.Errorf("runaway paging")
	}
	size := f.defaultSize
	if v, err := strconv.Atoi(params["size"]); err == nil && v > 0 {
		size = v
	}
	if f.maxSize > 0 && size > f.maxSize {
		size = f.maxSize
	}
	number := f.base
	if v, err := strconv.Atoi(params[f.pageParam]); err == nil && !f.ignorePaging {
		number = v
	}
	start := (number - f.base) * size
	var items []string
	for i := start; i < start+size && i < f.total && i >= 0; i++ {
		items = append(items, fmt.Sprintf(`{"id":"rec-%03d","uuid":"rec-%03d","name":"Record %d","status":"OPEN"}`, i, i, i))
	}
	pages := (f.total + size - 1) / size
	return json.RawMessage(fmt.Sprintf(`{"content":[%s],"page":{"number":%d,"size":%d,"totalElements":%d,"totalPages":%d}}`,
		strings.Join(items, ","), number, size, f.total, pages)), nil
}

func syncPagedFake(t *testing.T, resource string, f *pagedFake, required ...string) (syncResult, int) {
	t.Helper()
	db, _ := newNovelTestDB(t)
	var specs []string
	for _, k := range required {
		specs = append(specs, resource+":"+k+"=required-value")
	}
	userParams, err := parseSyncUserParams(nil, specs, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := syncResource(context.Background(), f, db, resource, "", true, 0, false, false, userParams, io.Discard)
	n, err := db.Count(resource)
	if err != nil {
		t.Fatal(err)
	}
	return res, n
}

// Codex r1 failing input: eleven OPEN opportunities through the documented
// OpportunityPage (number from 1, size 10), then `pipeline`: ten were stored.
func TestR1NumberPagedPartitionsSyncEveryPage(t *testing.T) {
	testenv.Isolate(t)
	// The app-reseller lists name required query params sync cannot fill;
	// they sync when the operator passes them (--resource-param).
	for _, c := range []struct {
		resource string
		base     int
		required []string
	}{
		{"assisted-sales-v1-opportunities", 1, nil},
		{"app-reseller-v1-orders", 1, []string{"context", "customerCompanyId", "resellerCompanyId"}},
		{"app-reseller-v1-subscriptions", 1, []string{"context", "customerCompanyId", "resellerCompanyId"}},
		{"app-reseller-v1-payment-methods", 1, []string{"companyId", "ownerId"}},
		{"app-reseller-v1-company-associations", 0, []string{"customerCompanyId", "resellerCompanyId"}},
		{"channel-v1-company-groups", 0, nil},
		{"checkout", 1, nil},
		{"reporting-v2-reports", 1, nil},
		{"reseller", 1, nil},
	} {
		f := &pagedFake{total: 123, base: c.base, pageParam: "number", defaultSize: 10}
		res, n := syncPagedFake(t, c.resource, f, c.required...)
		if res.Err != nil || res.Warn != nil || n != f.total {
			t.Errorf("%s: stored %d of %d (err %v, warn %v)", c.resource, n, f.total, res.Err, res.Warn)
		}
	}
}

// The class: whatever ends a walk, a page whose documented metadata says more
// records exist never reads as complete.
func TestR1PageMetadataOverridesAShortPage(t *testing.T) {
	testenv.Isolate(t)
	for _, c := range []struct {
		name, resource string
		f              *pagedFake
	}{
		{"server caps the page size below sync's", "account", &pagedFake{total: 45, base: 0, pageParam: "page", maxSize: 20, defaultSize: 50}},
		{"server ignores the page number", "checkout", &pagedFake{total: 11, base: 1, pageParam: "number", ignorePaging: true, defaultSize: 10, maxSize: 5}},
		{"server ignores the page number on a full page", "reseller", &pagedFake{total: 300, base: 1, pageParam: "number", ignorePaging: true, defaultSize: 10}},
	} {
		res, n := syncPagedFake(t, c.resource, c.f)
		if n < c.f.total && res.Err == nil && res.Warn == nil {
			t.Errorf("%s: stored %d of %d and reported the sync complete", c.name, n, c.f.total)
		}
	}
}

// Codex r1 failing input: documented UserAccountV2 records (no company
// member), then `company show <uuid> --json`: zero users.
func TestR1CompanyShowNeverReportsUsersItCannotJoin(t *testing.T) {
	db, path := newNovelTestDB(t)
	const company = "d689b970-f3f9-42a6-a974-90e2bdadf860"
	seedNovelResource(t, db, rtCompanies, company, map[string]any{"uuid": company, "name": "Customer Company"})
	seedNovelResource(t, db, rtUsers, "2f93cb24-f7b9-433f-875e-bb0dd9b0f1c1", map[string]any{
		"uuid": "2f93cb24-f7b9-433f-875e-bb0dd9b0f1c1", "email": "john.lee@company.example", "firstName": "John", "lastName": "Lee"})
	out, err := runNovelCmd(t, newNovelCompanyShowCmd, company, "--db", path)
	if err != nil {
		t.Fatal(err)
	}
	m := decodeNovelJSON(t, out)
	if counts, _ := m["counts"].(map[string]any); counts != nil {
		if v, ok := counts["users"]; ok {
			t.Errorf("counts.users = %v: synced users carry no company (UserAccountV2), so no user count can be joined", v)
		}
	}
	if users, ok := m["users"]; ok && users != nil {
		t.Errorf("users = %v: a list joined on a member UserAccountV2 does not have", users)
	}
}

// Codex r1 P2 local-source-contract-unenforced (one line per command): the six
// local analytics declared `pp:data-source local` only in a comment, so
// `pipeline --data-source live` ran on local SQLite as if it were live.
func TestR1LocalAnalyticsRefuseLiveDataSource(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"reconcile", "--since", "30d"}, {"subs", "changed", "--since", "7d"}, {"payments", "unpaid", "--since", "7d"},
		{"pipeline", "--group-by", "status"}, {"pipeline", "stale", "--days", "30"}, {"company", "show", "co-1"},
	} {
		cmd := RootCmd()
		var out, errOut strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(append(append([]string{}, args...), "--data-source", "live", "--json"))
		if err := cmd.Execute(); err == nil {
			t.Errorf("%s --data-source live ran on the local store: %q", strings.Join(args, " "), out.String())
		}
	}
}

// The partition comes from the endpoint the path is, not from its shape: a
// documented literal child of a synced collection
// (/assistedSales/v1/opportunities/pricingPlanCosts) is not a record of it,
// and is neither read from nor cached into the opportunities partition.
func TestR1PartitionFollowsTheEndpointTemplate(t *testing.T) {
	for _, c := range []struct{ label, path, want string }{
		{"account", "/account/v2/companies", "account-v2-companies"},
		{"account", "/account/v2/companies/0f9c2a1e-6b1d-4c55-9a2e-1d2c3b4a5f60", "account-v2-companies"},
		{"account", "/account/v2/users", "account"},
		{"account", "/account/v2/companies/0f9c2a1e-6b1d-4c55-9a2e-1d2c3b4a5f60/memberships", appdirectUnsyncedPartition},
		{"assisted-sales", "/assistedSales/v1/opportunities", "assisted-sales-v1-opportunities"},
		{"assisted-sales", "/assistedSales/v1/opportunities/pricingPlanCosts", appdirectUnsyncedPartition},
		{"notification", "/notification/v1/templates/common", "notification-v1-templates-common"},
		{"app-market", "/appMarket/v2/paymentMethods/defaults", "app-market-v2-payment-methods-defaults"},
		{"billing", "/billing/v1/no-such-endpoint", appdirectUnsyncedPartition},
	} {
		if got, _ := appdirectStorePartition(c.label, c.path, false); got != c.want {
			t.Errorf("%s %s: partition %q, want %q", c.label, c.path, got, c.want)
		}
	}
}
