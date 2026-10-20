// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestFleetClientNeverFollowsARedirect: the fleet client (fleet sync, fleet
// add --check, abb triage) follows no redirect on an authenticated read or a
// logout, whatever http.Client it is handed (nas.Client() follows redirects
// by default), and Login never re-sends the password form to one. Each
// refusal names the status, never the Location.
func TestFleetClientNeverFollowsARedirect(t *testing.T) {
	nas := dsmtest.NewRedirectingNAS(t, nil)
	c := &Client{HTTP: nas.Client(), BaseURL: nas.URL, Creds: Credentials{Account: "a", Password: "pw-redirect-2"}, RatePerSec: 1000}
	_, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1", nil)
	if err == nil || !errors.Is(err, ErrRedirectRefused) || !strings.Contains(err.Error(), "status 302") {
		t.Fatalf("read: err = %v, want a refused redirect naming status 302", err)
	}
	if strings.Contains(err.Error(), "backup") || strings.Contains(err.Error(), "SID1") {
		t.Fatalf("read: the error quoted the Location: %v", err)
	}
	if err := c.Logout(context.Background()); err == nil || !strings.Contains(err.Error(), "status 302") {
		t.Fatalf("logout: err = %v, want a refused redirect naming status 302", err)
	}
	if n := nas.Count("method=backup"); n != 0 {
		t.Fatalf("the fleet client followed a redirect into a backup start: %v", nas.Requests())
	}

	var posts atomic.Int32
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
			http.Redirect(w, r, "/webapi/entry.cgi", http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{}}`)
	}))
	defer login.Close()
	if _, err := Login(context.Background(), login.Client(), login.URL, Credentials{Account: "a", Password: "pw-redirect-2"}); err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("login: err = %v, want a refused redirect naming status 307", err)
	}
	if n := posts.Load(); n != 1 {
		t.Fatalf("the login form was sent %d times; a redirect must never re-send the password", n)
	}

	// The probe refuses a redirect the same way (it is not DSM's answer).
	if err := ProbeDSM(context.Background(), nas.Client(), nas.URL+"/redirect-me"); err == nil {
		t.Fatal("the probe accepted a redirect")
	}
}

// TestEveryHTTPClientRefusesRedirects walks the module: every CheckRedirect
// assignment (the generated client, its session manager, the fleet client)
// is dsm.RefuseRedirect, so a regenerated client that brings back a
// "same origin is fine" redirect policy fails here, not in the field.
func TestEveryHTTPClientRefusesRedirects(t *testing.T) {
	root := filepath.Join("..", "..")
	found := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel, _ := filepath.Rel(root, path); strings.HasPrefix(rel, ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			var rhs ast.Expr
			switch x := n.(type) {
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "CheckRedirect" && i < len(x.Rhs) {
						rhs = x.Rhs[i]
					}
				}
			case *ast.KeyValueExpr:
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == "CheckRedirect" {
					rhs = x.Value
				}
			}
			if rhs == nil {
				return true
			}
			found++
			ok := false
			switch v := rhs.(type) {
			case *ast.SelectorExpr:
				ok = v.Sel.Name == "RefuseRedirect"
			case *ast.Ident:
				ok = v.Name == "RefuseRedirect"
			}
			if !ok {
				t.Errorf("%s sets CheckRedirect to something other than dsm.RefuseRedirect; no DSM request may follow a redirect", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 3 { // generated client, session manager, fleet client
		t.Fatalf("found %d CheckRedirect assignments, want at least 3 (the walk is broken)", found)
	}
}
