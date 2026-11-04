// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The go/ast rule for rule 3: in files.go, every branch on an error in a
// function that writes, truncates or places a file write marks the stream
// unknown (fileState.fail, or a stream's lost) before it returns; the
// branches that come before anything is written or truncated are named
// here with why.
var kit8ErrorExits = map[string]string{
	"WriteFile: err != nil":                                     "os.OpenFile failed: nothing opened, nothing truncated",
	"WriteFile: err != nil#2":                                   "lockedState failed: the file was not truncated",
	"OpenFile: err != nil":                                      "os.OpenFile failed (without O_TRUNC): nothing truncated",
	"OpenFile: err != nil#2":                                    "f.Stat failed before the truncate",
	"OpenFile: err != nil#3":                                    "lockedState failed before the truncate",
	"AppendFile: err != nil":                                    "OpenFile failed: OpenFile's own branches decide",
	"AppendFile: err != nil#2":                                  "w.Write failed: Write's own branches decide",
	"lockedState: err != nil":                                   "a stat, before any write: its callers decide",
	"readBack: err == nil || err == io.EOF && int64(n) == size": "the read succeeded",
	"readBack: err != nil":                                      "a read: placed fails on its error",
	"readBack: err != nil#2":                                    "a read: placed fails on its error",
	"readBack: err != nil || !os.SameFile(fi, ri)":              "a read: placed fails on its error",
	"faultAt: err == nil && fileFault != nil":                   "the test fault: its caller decides",
	"put: err != nil":                                           "put's callers (atEnd, placed) fail on its error",
	"put: err != nil#2":                                         "put's callers (atEnd, placed) fail on its error",
	"fileKey: err != nil":                                       "path resolution, before any write",
	"fileKey: err == nil":                                       "path resolution, before any write",
	"fileKey: err == nil#2":                                     "path resolution, before any write",
	"NewWriter: err != nil":                                     "a stream that cannot be identified starts lost (st.lost)",
	"named: err == nil && os.SameFile(fi, fs.fi)":               "a stat of a name, before any write: a name that does not resolve does not name the file",
	"reconcile: err == nil && bytes.Equal(b, fs.ctx.b)":         "the tail read back matches: anything else reads the file again (readFound), which marks the stream unknown on its own error",
	"readTail: int64(k) == n && (err == nil || err == io.EOF)":  "the read succeeded",
	"readTail: err != nil":                                      "a read: readFound marks the stream unknown on its error",
	"readTail: err != nil#2":                                    "a read: readFound marks the stream unknown on its error",
	"readTail: err != nil || !os.SameFile(fi, ri)":              "a read: readFound marks the stream unknown on its error",
}

func TestKit8EveryErrorBranchMarksTheStream(t *testing.T) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "files.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	marks := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(x ast.Node) bool {
			switch x := x.(type) {
			case *ast.CallExpr:
				if s, ok := x.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "fail" {
					found = true
				}
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if s, ok := l.(*ast.SelectorExpr); ok && (s.Sel.Name == "lost" || s.Sel.Name == "unknown") && i < len(x.Rhs) {
						if id, ok := x.Rhs[i].(*ast.Ident); ok && id.Name == "true" {
							found = true
						}
					}
				}
			}
			return true
		})
		return found
	}
	seen := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		count := map[string]int{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			is, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}
			cond := exprText(fs, is.Cond)
			if !strings.Contains(cond, "err") {
				return true
			}
			returns := false
			ast.Inspect(is.Body, func(x ast.Node) bool {
				if _, ok := x.(*ast.ReturnStmt); ok {
					returns = true
				}
				return true
			})
			if !returns {
				return true
			}
			key := fd.Name.Name + ": " + cond
			if count[key]++; count[key] > 1 {
				key += fmt.Sprintf("#%d", count[key])
			}
			seen[key] = true
			if !marks(is.Body) {
				if _, ok := kit8ErrorExits[key]; !ok {
					t.Errorf("files.go:%d %s returns on an error without marking the stream unknown",
						fs.Position(is.Pos()).Line, key)
				}
			} else if _, ok := kit8ErrorExits[key]; ok {
				t.Errorf("%s is named as an exit but marks the stream: drop it from the list", key)
			}
			return true
		})
	}
	for k := range kit8ErrorExits {
		if !seen[k] {
			t.Errorf("a named error exit is gone: %s", k)
		}
	}
}

func exprText(fs *token.FileSet, e ast.Expr) string {
	b, err := os.ReadFile("files.go")
	if err != nil {
		return ""
	}
	return string(b[fs.Position(e.Pos()).Offset:fs.Position(e.End()).Offset])
}
