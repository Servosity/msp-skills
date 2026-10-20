// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"testing"
)

// With an unusable auth-context.json the ledger commands must stop, not
// open an empty per-token store and report "no batches".
func TestRestoreLedgerCommandsFailClosedOnUnusableRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission refusal is a POSIX mode check")
	}
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)
	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	path, _ := authContextPath()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"restore", "history"},
		{"restore", "watch", "--batch", "20261001-000000-abcd", "--once"},
		{"restore", "submit", "--task", "64b0e9", "--date", "2026-10-01", "--yes"},
	} {
		resetStoreScope(t)
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); !errors.Is(err, errStoreScopeUnbound) {
			t.Errorf("%v: err %v, want the fail-closed store-scope error", args, err)
		}
	}
}
