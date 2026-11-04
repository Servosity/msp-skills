// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (secret-field-persistence class): fleet_rows never holds a
// response secret, even one the decoded row does not show.

package cli

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/cliutil/testenv"
)

func TestFleetRowsSeeEveryKeyOccurrence(t *testing.T) {
	testenv.Isolate(t)
	const pw = "breached-secret-123"
	path := filepath.Join(t.TempDir(), "f.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureFleetSchema(db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	rows := []fleetRow{
		{ID: "e1", Data: json.RawMessage(`{"id":"e1","password":"` + pw + `","password":null}`)},
		{ID: "e2", Data: json.RawMessage(`{"id":"e2"} {"password":"` + pw + `"}`)},
		{ID: "e3", Data: json.RawMessage(`{"id":"e3"}garbage"password":"` + pw + `"`)},
		{ID: "e4", Data: json.RawMessage(`{"id":"e4","password":"` + pw + `"`)},
	}
	at := time.Now()
	if err := replaceFleetScope(db, fleetKindDevice, "t1", rows, at); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := mergeFleetRows(db, fleetKindDevice, "t2", rows, at, ""); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + "-wal", path + "-journal"} {
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), pw) {
			t.Fatalf("%s holds the response password", filepath.Base(p))
		}
	}
}
