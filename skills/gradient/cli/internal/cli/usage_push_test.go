// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParsePushFile(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		content  string
		wantRows int
		wantErr  bool
	}{
		{
			name: "csv with header", file: "counts.csv",
			content:  "accountId,serviceId,unitCount\na1,s1,10\na2,s1,5.5\n",
			wantRows: 2,
		},
		{
			name: "csv header aliases", file: "counts.csv",
			content:  "account_id,service_id,unit_count\na1,s1,10\n",
			wantRows: 1,
		},
		{
			name: "json array", file: "counts.json",
			content:  `[{"accountId":"a1","serviceId":"s1","unitCount":10},{"accountId":"a2","serviceId":"s2","unitCount":0}]`,
			wantRows: 2,
		},
		{name: "empty file", file: "counts.csv", content: "", wantRows: 0},
		{name: "missing header column", file: "counts.csv", content: "accountId,unitCount\na1,10\n", wantErr: true},
		{name: "non-numeric count", file: "counts.csv", content: "accountId,serviceId,unitCount\na1,s1,abc\n", wantErr: true},
		{name: "negative count", file: "counts.csv", content: "accountId,serviceId,unitCount\na1,s1,-3\n", wantErr: true},
		{name: "empty serviceId", file: "counts.json", content: `[{"accountId":"a1","serviceId":"","unitCount":1}]`, wantErr: true},
		{name: "empty accountId", file: "counts.json", content: `[{"accountId":"","serviceId":"s1","unitCount":1}]`, wantErr: true},
		{name: "invalid json", file: "counts.json", content: `[{"accountId":`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := parsePushFile(writeTemp(t, tc.file, tc.content))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d rows", len(rows))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rows) != tc.wantRows {
				t.Errorf("rows = %d, want %d", len(rows), tc.wantRows)
			}
		})
	}
}

func TestParsePushFileCSVValues(t *testing.T) {
	p := writeTemp(t, "counts.csv", "accountId,serviceId,unitCount\nacct-9, svc-7 ,42\n")
	rows, err := parsePushFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].AccountID != "acct-9" || rows[0].ServiceID != "svc-7" || rows[0].UnitCount != 42 {
		t.Errorf("row mismatch: %+v", rows[0])
	}
}

func TestNovelUsagePushCommandShape(t *testing.T) {
	cmd := newNovelUsagePushCmd(&rootFlags{})
	if cmd.Flags().Lookup("file") == nil || cmd.Flags().Lookup("no-build") == nil {
		t.Error("usage push must declare --file and --no-build")
	}
	if cmd.Annotations["mcp:read-only"] != "false" {
		t.Error("usage push mutates upstream state; mcp:read-only must be false")
	}
}

func TestNewPushRunIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newPushRunID()
		if seen[id] {
			t.Fatalf("duplicate run id within same second: %s", id)
		}
		seen[id] = true
	}
}
