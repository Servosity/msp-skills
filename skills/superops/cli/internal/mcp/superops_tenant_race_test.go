// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/store"
)

func mcpStampRecorded(t *testing.T, path string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tenant string
	return db.QueryRow(`SELECT tenant FROM `+mcpStampTable+` WHERE id = 1`).Scan(&tenant) == nil
}

// Member 1 (round 2 P1), MCP side: the sql and search tools opened a store
// that records no tenant read-only whenever it held no synced rows, and
// another tenant could fill it before the query ran. Such a store is refused,
// and the refusal says to sync this tenant first.
func TestMCPSQLAndSearchRefuseAStoreThatRecordsNoTenant(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, path string)
	}{
		{"empty file", func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"schema without rows or tenant", func(t *testing.T, path string) {
			s, err := store.Open(path)
			if err != nil {
				t.Fatalf("plant: %v", err)
			}
			if _, err := s.DB().Exec(`DROP TABLE IF EXISTS ` + mcpStampTable); err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetMCPPathEnv(t)
			t.Setenv("SUPEROPS_API_TOKEN", "sop-own-tenant-token")
			t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
			path, err := mcpDBPath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			tc.plant(t, path)

			sqlResult := callMCPSQL(t, context.Background(), "SELECT name FROM sqlite_master")
			assertMCPRefusal(t, "sql on a store that records no tenant", sqlResult)
			if text := mcpTextContent(t, sqlResult); !strings.Contains(text, "superops-cli sync") {
				t.Fatalf("sql refusal does not name superops-cli sync:\n%s", text)
			}
			assertMCPRefusal(t, "search on a store that records no tenant", mcpSearchResult(t, "ticket"))
			if mcpStampRecorded(t, path) {
				t.Fatal("an MCP read recorded a tenant in the store")
			}
		})
	}
}

// Positive path: on a fresh install the tools say to sync, and once any
// command of this tenant has opened the store for writing (which records the
// tenant), sql and search answer with an empty result instead of an error.
func TestMCPSQLAndSearchOnAFreshInstall(t *testing.T) {
	for _, token := range []string{"", "sop-own-tenant-token"} {
		t.Run("token="+map[bool]string{true: "set", false: "unset"}[token != ""], func(t *testing.T) {
			mcpFreshInstall(t, token)
		})
	}
}

func mcpFreshInstall(t *testing.T, token string) {
	t.Helper()
	resetMCPPathEnv(t)
	t.Setenv("SUPEROPS_API_TOKEN", token)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	result := callMCPSQL(t, context.Background(), "SELECT COUNT(*) AS n FROM resources")
	if text := mcpTextContent(t, result); !result.IsError || !strings.Contains(text, "superops-cli sync") {
		t.Fatalf("token %v: sql with no store file (IsError %v):\n%s", token != "", result.IsError, text)
	}

	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("token %v: first read-write open: %v", token != "", err)
	}
	_ = s.Close()

	result = callMCPSQL(t, context.Background(), "SELECT COUNT(*) AS n FROM resources")
	if text, envelope := decodeMCPSQLEnvelope(t, result); result.IsError || envelope.Count != 1 {
		t.Fatalf("token %v: sql on this tenant's empty store (IsError %v):\n%s", token != "", result.IsError, text)
	}
	search := mcpSearchResult(t, "ticket")
	if text := mcpTextContent(t, search); search.IsError || !strings.Contains(text, "store_status") || !strings.Contains(text, "empty") {
		t.Fatalf("token %v: search on this tenant's empty store (IsError %v):\n%s", token != "", search.IsError, text)
	}
}
