// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/credkit"
	"appdirect-pp-cli/internal/kitsql"
)

const storeDSNQuery = "?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(0)"

// explainStoreWithheld turns the kit's withheld notice on a test store open
// into a reason that names no credential. The Windows CI row withheld the
// sync-hint test store once (8d43d29299) and passed on identical code
// (4e3f3a07ea), and the notice alone does not say why. Each probe opens a
// name through the same kit SQLite driver in a directory that does not
// exist, so nothing is written: a withheld name fails with the notice, any
// other with SQLite's own error. A refused path part is reported by its
// index and length, never its text.
func explainStoreWithheld(dbPath string, err error) string {
	if err == nil || !strings.Contains(err.Error(), "credkit: withheld") {
		return ""
	}
	withheld := func(name string) bool {
		db, oerr := sql.Open(kitsql.Driver, name)
		if oerr != nil {
			return strings.Contains(oerr.Error(), "credkit: withheld")
		}
		defer db.Close()
		perr := db.PingContext(context.Background())
		return perr != nil && strings.Contains(perr.Error(), "credkit: withheld")
	}
	missing := filepath.Join(string(filepath.Separator)+"appdirect-store-probe-missing", "probe")
	parts := strings.FieldsFunc(dbPath, func(r rune) bool { return r == '/' || r == '\\' })
	var shown, refused []string
	for i, p := range parts {
		if withheld(filepath.Join(missing, p)) {
			shown = append(shown, fmt.Sprintf("<part %d, %d bytes>", i, len(p)))
			refused = append(refused, fmt.Sprint(i))
		} else {
			shown = append(shown, p)
		}
	}
	var pairs []string
	for i := 0; i+1 < len(parts); i++ {
		if withheld(filepath.Join(missing, parts[i]+`\`+parts[i+1])) || withheld(filepath.Join(missing, parts[i]+"/"+parts[i+1])) {
			pairs = append(pairs, fmt.Sprintf("%d+%d", i, i+1))
		}
	}
	return fmt.Sprintf("\nkit withheld this test's store connection name; no held value is printed:"+
		"\n  every unit withheld (fail-closed state): %v"+
		"\n  store query alone withheld: %v"+
		"\n  whole path withheld (no query): %v"+
		"\n  path parts refused on their own: [%s]; adjacent pairs refused: [%s]"+
		"\n  path: %s",
		withheld(":memory:"), withheld(filepath.Join(missing, "plain.db")+storeDSNQuery), withheld(dbPath),
		strings.Join(refused, " "), strings.Join(pairs, " "), strings.Join(shown, string(filepath.Separator)))
}

// The reason names the refused part by index and length only. The value is
// held for the rest of this test binary, so it is long and unlike any path.
func TestExplainStoreWithheldNamesNoHeldValue(t *testing.T) {
	const fixture = "zqxStoreProbeHeld-7f3e9c1a"
	credkit.Hold("test:store-probe", fixture)
	path := filepath.Join(t.TempDir(), fixture, "data.db")
	got := explainStoreWithheld(path, fmt.Errorf("open: [credkit: withheld from sqlite: it carried a credential this program holds]"))
	if strings.Contains(got, fixture) {
		t.Fatalf("the reason printed the held value: %s", got)
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	want := fmt.Sprintf("<part %d, %d bytes>", len(parts)-2, len(fixture))
	if !strings.Contains(got, want) || !strings.Contains(got, "whole path withheld (no query): true") || !strings.Contains(got, "every unit withheld (fail-closed state): false") {
		t.Fatalf("reason = %s, want %s named and the whole path withheld", got, want)
	}
	if explainStoreWithheld(path, nil) != "" || explainStoreWithheld(path, fmt.Errorf("disk full")) != "" {
		t.Fatal("a store error the kit did not cause must add no reason")
	}
}
