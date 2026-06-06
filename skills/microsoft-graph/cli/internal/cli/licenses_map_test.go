// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLicensesMapCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelLicensesMapCmd, "ENTERPRISEPACK")
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("licenses map output is not valid JSON: %v\n%s", err, out)
	}
	if res["skuPartNumber"] != "ENTERPRISEPACK" {
		t.Errorf("expected skuPartNumber ENTERPRISEPACK, got %v", res["skuPartNumber"])
	}
	// seedStore: ex@contoso.com is disabled and holds ENTERPRISEPACK.
	if !strings.Contains(out, "ex@contoso.com") || !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled consumer ex@contoso.com flagged, got: %s", out)
	}
	if got := res["reclaimableSeats"].(float64); got != 1 {
		t.Errorf("expected 1 reclaimable seat, got %v", got)
	}
	// Unrelated user must not appear.
	if strings.Contains(out, "active@contoso.com") {
		t.Errorf("user without the SKU should not be a consumer, got: %s", out)
	}
}

func TestLicensesMapCommandNotFound(t *testing.T) {
	dbPath := seedStore(t)
	flags := &rootFlags{asJSON: true}
	cmd := newNovelLicensesMapCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--db", dbPath, "NONEXISTENT_SKU"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected not-found error for unknown SKU against a synced store")
	}
	if ExitCode(err) != 3 {
		t.Errorf("expected exit code 3 (not found), got %d", ExitCode(err))
	}
}

func TestLicensesMapCommandEmptyStoreIsHonestEmpty(t *testing.T) {
	// No store file at all: empty result with note, exit 0 — matches the
	// other transcendence commands' empty-before-first-pull behavior.
	flags := &rootFlags{asJSON: true}
	cmd := newNovelLicensesMapCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--db", t.TempDir() + "/missing.db", "ENTERPRISEPACK"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected exit 0 on empty store, got: %v", err)
	}
	if !strings.Contains(buf.String(), "pull") {
		t.Errorf("expected note pointing at pull, got: %s", buf.String())
	}
}
