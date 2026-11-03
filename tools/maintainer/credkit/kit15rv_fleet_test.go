// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fleetTexts is every Go string literal (unquoted) and every testdata or
// golden file under skills/*/cli, with whether it comes from a test file.
func fleetTexts(t *testing.T) (texts []string, files []string, tests []bool) {
	t.Helper()
	filepath.WalkDir("../../../skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains(p, "/cli/") {
			return nil
		}
		isTest := strings.HasSuffix(p, "_test.go")
		switch {
		case strings.HasSuffix(p, ".go"):
			f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
			if err != nil {
				return nil
			}
			ast.Inspect(f, func(x ast.Node) bool {
				if bl, ok := x.(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if s, err := strconv.Unquote(bl.Value); err == nil {
						texts, files, tests = append(texts, s), append(files, p), append(tests, isTest)
					}
				}
				return true
			})
		case strings.Contains(p, "/testdata/") || strings.HasSuffix(p, ".golden"):
			if b, err := os.ReadFile(p); err == nil {
				texts, files, tests = append(texts, string(b)), append(files, p), append(tests, false)
			}
		}
		return nil
	})
	if len(texts) == 0 {
		t.Skip("no skills tree beside the kit")
	}
	return texts, files, tests
}

var (
	fleetQuery  = regexp.MustCompile(`(?:^|[\s"'(=])((?:https?://[^\s"'<>]*|/[A-Za-z0-9_{}.~%/-]*)\?([^\s"'<>#]*))`)
	fleetConfig = regexp.MustCompile(`(?m)^(?:[A-Za-z_][A-Za-z0-9_.-]*[ \t]*=[ \t]*\S|\[[A-Za-z0-9_.-]+\][ \t]*$)`)
	fleetHeader = regexp.MustCompile(`\.Header\[|\.Header\.(?:Set|Add)\("(?:Content-Encoding|Transfer-Encoding|Content-Transfer-Encoding)"`)
	fleetCS     = regexp.MustCompile(`(?i)charset=([A-Za-z0-9_-]+)`)
)

// Loop ruling 8 proof: the blunt request rules fire on no request the
// fleet's connectors build. Every query in a connector's code, testdata or
// golden files is canonical (a {placeholder} path template or an example
// value with $$ is not a query sent as written; connector test files hold
// deliberately malformed redaction fixtures, logged, not judged); no code
// sets a header map key directly or a coding header on a request; every
// charset literal is UTF-8; every ordinary-output unit is a whole JSON body.
func TestKit15FleetRequestsAreWhole(t *testing.T) {
	texts, files, tests := fleetTexts(t)
	queries, logged := 0, 0
	for i, s := range texts {
		for _, m := range fleetQuery.FindAllStringSubmatch(s, -1) {
			queries++
			if canonicalQuery(m[2]) {
				continue
			}
			if tests[i] || strings.ContainsAny(m[2], "{$") {
				logged++
				t.Logf("not a sent query (test fixture or template): %s: %q", files[i], m[1])
				continue
			}
			t.Errorf("%s: a fleet query the kit would refuse: %q", files[i], m[1])
		}
		if !tests[i] {
			for _, m := range fleetCS.FindAllStringSubmatch(s, -1) {
				if cs := strings.ToLower(m[1]); cs != "utf-8" && cs != "us-ascii" {
					t.Errorf("%s: a charset the kit would refuse on a request body: %q", files[i], m[0])
				}
			}
		}
	}
	t.Logf("queries checked: %d (%d logged as test fixtures or templates)", queries, logged)
	filepath.WalkDir("../../../skills", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") && strings.Contains(p, "/cli/") {
			b, _ := os.ReadFile(p)
			for _, line := range strings.Split(string(b), "\n") {
				if fleetHeader.MatchString(line) && !strings.Contains(line, "resp.") && !strings.Contains(line, "view.") && !strings.Contains(line, "preview.") {
					t.Errorf("%s: a request header the kit would refuse: %s", p, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	h := http.Header{"Content-Type": {"application/json"}}
	for name, unit := range ordinaryCorpus(t) {
		if !bodyWhole(unit, h) {
			t.Errorf("%s: an ordinary unit as a JSON request body is not whole", name)
		}
	}
}

// Loop ruling 9 proof: every config file text in the fleet's connectors
// (every Go string literal that is whole config lines, tests included, and
// every testdata file) is in the canonical subset, so the blanket hold
// never fires on what connectors and their docs write. The docs and
// READMEs carry no config file example (they name the file's path only).
func TestKit15FleetConfigsArePrecise(t *testing.T) {
	texts, files, _ := fleetTexts(t)
	n := 0
outer:
	for i, s := range texts {
		if !strings.HasSuffix(s, "\n") || strings.Contains(s, "%") || !fleetConfig.MatchString(s) {
			continue // a format string or a fragment is not a file
		}
		for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
			if !fleetConfig.MatchString(l) && !canonBlank.MatchString(l) {
				continue outer
			}
		}
		n++
		if !preciseConfig([]byte(s)) {
			t.Errorf("%s: a fleet config text outside the canonical subset: %q", files[i], s)
		}
	}
	t.Logf("config texts checked: %d", n)
}
