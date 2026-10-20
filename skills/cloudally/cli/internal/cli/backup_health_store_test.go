// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/store"
)

func seedHealthRows(t *testing.T, rows ...string) {
	t.Helper()
	s, err := store.Open(defaultDBPath(restoreCLIName))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		items[i] = json.RawMessage(r)
	}
	if n, _, err := s.UpsertBatch(healthResourceDistributor, items); err != nil || n != len(rows) {
		t.Fatalf("seeded %d rows, err %v", n, err)
	}
}

func runHealth(args ...string) (string, error) {
	cmd := RootCmd()
	cmd.SetArgs(append([]string{"backup", "health"}, args...))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

// --agent compaction must keep the error text a failing row exists to show,
// and a filter that matches nothing must not read as "healthy".
func TestBackupHealthAgentKeepsErrorAndEmptyFilterFails(t *testing.T) {
	testenv.Isolate(t)
	seedHealthRows(t,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"bad@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z","backupStatus":[{"subSource":"mail","status":"FAILED","error":"QUOTA_EXCEEDED","errFAQLink":"https://example.com/faq"}]}`,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"a@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"b@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"c@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
		`{"userId":"u1","taskId":"t1","source":"MS365","entityName":"d@contoso.com","lastBackupDate":"2020-01-01T00:00:00Z"}`,
	)
	out, err := runHealth("--agent")
	if err != nil {
		t.Fatalf("health --agent: %v", err)
	}
	if !strings.Contains(out, "QUOTA_EXCEEDED") || !strings.Contains(out, "https://example.com/faq") {
		t.Fatalf("--agent dropped the failing row's error: %s", out)
	}
	_, err = runHealth("--account", "nobody", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 3 {
		t.Fatalf("unmatched --account: err %v, want exit 3", err)
	}
}
