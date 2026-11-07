// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class rule: every corkFetchPages caller keeps the page-cap result. Discarding it is how a truncated roster
// read as a complete sweep ("none past their cure period" with scan_cap_hit=false).
func TestNoCorkFetchPagesCallerDiscardsTheCap(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Rhs) != 1 || len(as.Lhs) != 4 {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "corkFetchPages" {
				return true
			}
			if id, ok := as.Lhs[2].(*ast.Ident); ok && id.Name == "_" {
				t.Errorf("%s: corkFetchPages cap result discarded; a truncated read must reach scan_cap_hit", fset.Position(as.Pos()))
			}
			return true
		})
	}
}

// Documented PaginatedClientResponse: 100 clients on page 1 of 2, read with --max-scan-pages 1.
func TestComplianceOverdueRosterCapIsReported(t *testing.T) {
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/compliance/event-types", func(w http.ResponseWriter, r *http.Request) {
		send(w, []any{map[string]any{"event_type": "mfa_disabled", "cure_period_hours": 72}})
	})
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		items := make([]any, 0, 100)
		for i := 0; i < 100; i++ {
			items = append(items, map[string]any{"uuid": fmt.Sprintf("00000000-0000-0000-0000-%012d", i), "name": fmt.Sprintf("c%d", i)})
		}
		send(w, map[string]any{"items": items, "pagination": map[string]any{"page": 1, "page_size": 100, "total_pages": 2, "total_items": 101}})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{}, "pagination": map[string]any{"page": 1, "total_pages": 1}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy")
	cmd := RootCmd()
	cmd.SetArgs([]string{"compliance", "overdue", "--max-clients", "0", "--max-scan-pages", "1", "--db", t.TempDir() + "/absent.db", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	_ = cmd.Execute()
	var v struct {
		ScanCapHit bool `json:"scan_cap_hit"`
	}
	s := out.String()
	if i := strings.Index(s, "{"); i < 0 || json.Unmarshal([]byte(s[i:]), &v) != nil {
		t.Fatalf("no JSON view:\n%s\n%s", s, errOut.String())
	}
	if !v.ScanCapHit {
		t.Fatalf("roster stopped at page 1 of 2 but scan_cap_hit=false:\n%s", s)
	}
}
