// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"cisco-duo-pp-cli/internal/client"
)

// Codex merge review (P2): fleet --only dropped selectors that matched no
// subaccount, so a typo produced a successful report covering fewer customers
// than requested (or none at all, still exit 0). Every unmatched selector must
// be refused as a usage error that names it, before any per-account fetch.
func TestSelectFleetAccountsRefusesUnmatchedSelectors(t *testing.T) {
	accounts := []client.DuoSubaccount{
		{AccountID: "DA11111111111111111A", Name: "Acme Dental"},
		{AccountID: "DA22222222222222222B", Name: "Bayside Law"},
	}

	t.Run("all accounts when --only is empty", func(t *testing.T) {
		got, err := selectFleetAccounts(accounts, "  ")
		if err != nil || len(got) != 2 {
			t.Fatalf("got %d accounts, err %v; want 2, nil", len(got), err)
		}
	})

	t.Run("id and name match case-insensitively", func(t *testing.T) {
		got, err := selectFleetAccounts(accounts, " da11111111111111111a , BAYSIDE LAW ")
		if err != nil || len(got) != 2 {
			t.Fatalf("got %d accounts, err %v; want 2, nil", len(got), err)
		}
	})

	for name, only := range map[string]string{
		"mixed valid and misspelled": "Acme Dental,Baysde Law",
		"entirely misspelled":        "Acme Dentl",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := selectFleetAccounts(accounts, only)
			if err == nil {
				t.Fatalf("selector %q returned %d accounts and no error; an unmatched selector must be refused", only, len(got))
			}
			if code := ExitCode(err); code != 2 {
				t.Fatalf("exit code %d, want 2 (usage error): %v", code, err)
			}
			for _, part := range strings.Split(only, ",") {
				matched := strings.EqualFold(part, "Acme Dental")
				if !matched && !strings.Contains(err.Error(), part) {
					t.Fatalf("error %q does not name the unmatched selector %q", err, part)
				}
				if matched && strings.Contains(err.Error(), `"`+part+`"`) {
					t.Fatalf("error %q names the matched selector %q as unmatched", err, part)
				}
			}
		})
	}
}
