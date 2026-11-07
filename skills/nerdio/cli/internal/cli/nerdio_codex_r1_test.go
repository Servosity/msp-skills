// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 (appdirect 4.34.2 reprint, ported) class
// sync-read-partition-mismatch, as it reaches Nerdio: every generated endpoint
// command names its command group as the store partition, and one group holds
// several endpoints (scripted-actions list is the MSP-level
// /rest-api/v1/scripted-actions sync fills; scripted-actions account-list is
// one account's /rest-api/v1/accounts/{id}/scripted-actions, which sync does
// not store). The account read with --data-source local returned the MSP
// list, and an automatic account read would have been cached into it. The
// data-source chokepoint now picks the partition by the endpoint the path is
// (nerdio_partitions.go), and a tenant-secret path (LAPS, BitLocker keys,
// secure variables) is never a partition at all.
//
// Not ported (no Nerdio counterpart): the AppDirect number-paging tests (NMM
// lists are bare arrays with no paging), company show's user join and the six
// AppDirect local-store views' --data-source live refusal (Nerdio's novels
// are live fan-outs).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/cliutil/testenv"
	"nerdio-pp-cli/internal/store"
)

// Codex r1 failing input, Nerdio shape: sync scripted-actions, then
// `scripted-actions account-list 101 --data-source local --json`.
func TestR1AccountListNeverReadsTheMSPPartition(t *testing.T) {
	testenv.Isolate(t)
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("nerdio-cli"))
	if err != nil {
		t.Fatal(err)
	}
	for id, rec := range map[string]map[string]any{
		"7": {"id": 7, "name": "MSP-Level-Script-5c3e"},
	} {
		data, _ := json.Marshal(rec)
		if err := db.Upsert("scripted-actions", id, data); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	for _, c := range []struct {
		args       []string
		want, deny string
	}{
		{[]string{"scripted-actions", "list", "--data-source", "local", "--json"}, "MSP-Level-Script-5c3e", ""},
		{[]string{"scripted-actions", "account-list", "101", "--data-source", "local", "--json"}, "no local data", "MSP-Level-Script-5c3e"},
	} {
		cmd := RootCmd()
		var out, errOut strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(c.args)
		err := cmd.Execute()
		got := out.String() + errOut.String()
		if err != nil {
			got += err.Error()
		}
		if !strings.Contains(got, c.want) || (c.deny != "" && strings.Contains(got, c.deny)) {
			t.Errorf("%s: printed %q; want %q and never %q", strings.Join(c.args[:2], " "), got, c.want, c.deny)
		}
	}
}

// The partition comes from the endpoint the path is, not from its command
// group: a synced collection and one record under it read their partition;
// another endpoint of the same group, an account-scoped list, a literal child
// of a synced collection, and every tenant-secret path have none.
func TestR1PartitionFollowsTheEndpointTemplate(t *testing.T) {
	for _, c := range []struct{ label, path, want string }{
		{"accounts", "/rest-api/v1/accounts", "accounts"},
		{"scripted-actions", "/rest-api/v1/scripted-actions", "scripted-actions"},
		{"scripted-actions", "/rest-api/v1/accounts/101/scripted-actions", nerdioUnsyncedPartition},
		{"autoscale-profiles", "/rest-api/v1/autoscale-profiles", "autoscale-profiles"},
		{"autoscale-profiles", "/rest-api/v1/autoscale-profiles/12", "autoscale-profiles"},
		{"autoscale-profiles", "/rest-api/v1/accounts/101/autoscale-profiles", nerdioUnsyncedPartition},
		{"schedules", "/rest-api/v1/schedules/9", "schedules"},
		{"schedules", "/rest-api/v1/schedules/9/configurations", nerdioUnsyncedPartition},
		{"cost-estimator", "/rest-api/v1/costestimator/list", "cost-estimator"},
		{"resource-groups", "/rest-api/v1/resource-group", "resource-groups"},
		{"resource-groups", "/rest-api/v1/accounts/101/resource-group", nerdioUnsyncedPartition},
		{"devices", "/rest-api/v1-beta/accounts/101/devices", nerdioUnsyncedPartition},
		{"devices", "/rest-api/v1-beta/accounts/101/devices/dev-1/local-admin-password", nerdioUnsyncedPartition},
		{"devices", "/rest-api/v1-beta/accounts/101/devices/dev-1/bitlocker-keys", nerdioUnsyncedPartition},
		{"secure-variables", "/rest-api/v1/secure-variables", nerdioUnsyncedPartition},
		{"secure-variables", "/rest-api/v1/secure-variables?name=x", nerdioUnsyncedPartition},
		{"secure-variables", "/rest-api/v1/accounts/101/secure-variables", nerdioUnsyncedPartition},
		{"accounts", "/rest-api/v1/no-such-endpoint", nerdioUnsyncedPartition},
	} {
		if got, _ := nerdioStorePartition(c.label, c.path, false); got != c.want {
			t.Errorf("%s %s: partition %q, want %q", c.label, c.path, got, c.want)
		}
	}
}

// tenant-secrets-shown-never-stored, at the guard: a GET of a tenant-secret
// path (LAPS, BitLocker keys, secure variables, MSP-level or per account) is
// recognized; nothing else is, a write to the same path included.
func TestTenantSecretPaths(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/rest-api/v1-beta/accounts/101/devices/550e8400-e29b-41d4-a716-446655440000/local-admin-password", true},
		{"GET", "/rest-api/v1-beta/accounts/101/devices/550e8400-e29b-41d4-a716-446655440000/bitlocker-keys/", true},
		{"GET", "/rest-api/v1/secure-variables", true},
		{"GET", "/rest-api/v1/accounts/101/secure-variables", true},
		{"POST", "/rest-api/v1/secure-variables", false},
		{"PUT", "/rest-api/v1/accounts/101/secure-variables", false},
		{"GET", "/rest-api/v1-beta/accounts/101/devices/550e8400-e29b-41d4-a716-446655440000", false},
		{"GET", "/rest-api/v1/accounts", false},
		{"GET", "/rest-api/v1/scripted-actions", false},
	} {
		if got := nerdioTenantSecretPath(c.method, c.path); got != c.want {
			t.Errorf("%s %s: tenant secret path = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
