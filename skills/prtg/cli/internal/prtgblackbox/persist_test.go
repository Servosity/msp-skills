// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class unfiltered-persistence (prtg handfix: credkit-sqlite-every-handle): text
// an operator or agent hands the learn loop (teach --notes, a query, a playbook)
// never carries a held credential into the local SQLite store: the kit's driver
// refuses the statement, the command says it failed, and a clean row still lands.
func TestTeachNeverPersistsACredential(t *testing.T) {
	t.Parallel()
	e := bareEnv(t)
	e.env = append(e.env, "PRTG_API_KEY="+bbKey)
	r := filepath.Join(e.home, "r")
	if err := os.MkdirAll(r, 0o700); err != nil {
		t.Fatal(err)
	}
	notes := "Authentication failed for " + bbKey
	for _, args := range [][]string{
		{"--home", r, "teach", "--query", "sensor uptime", "--resource-type", "sensors", "--resource", "2322", "--notes", notes, "--no-validate"},
		{"--home", r, "teach", "--query", "uptime of " + bbKey, "--resource-type", "sensors", "--resource", "2323", "--no-validate"},
		{"--home", r, "teach-playbook", "--query", "sensor uptime", "--notes", "check " + bbKey + " then retry",
			"--playbook-json", `{"steps":["recall","fetch ` + bbKey + `"]}`},
	} {
		assertNoEcho(t, "output of teach", e.run(t, args...))
	}
	var dbs int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if synccpuStoreFile(path) { // synccpu: the local store is not checked
			dbs++
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
	if dbs == 0 {
		t.Fatal("teach wrote no local store; the test did not exercise the persistence boundary")
	}
	// The store keeps the rows carrying the key (synccpu: the local store is
	// not checked), so the list that would print them is withheld whole.
	if out := e.run(t, "--home", r, "learnings", "list", "--json"); strings.Contains(out, "2322") || !strings.Contains(out, protoNote) {
		t.Fatalf("want the list of rows carrying the key withheld whole:\n%.2000s", out)
	}
	clean := filepath.Join(e.home, "clean")
	e.run(t, "--home", clean, "teach", "--query", "sensor uptime", "--resource-type", "sensors", "--resource", "2324", "--notes", "Authentication failed for the key", "--no-validate")
	if out := e.run(t, "--home", clean, "learnings", "list", "--json"); !strings.Contains(out, "2324") {
		t.Fatalf("learnings list lost a clean row:\n%.2000s", out)
	}
}
