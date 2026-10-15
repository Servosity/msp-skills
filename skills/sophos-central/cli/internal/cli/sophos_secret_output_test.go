// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "secret-responses-never-persisted" entry.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestExportAndTailDropMigrationToken: GET /endpoint/v1/migrations returns
// each job's token; the bulk commands an MCP caller can reach must not print it.
func TestExportAndTailDropMigrationToken(t *testing.T) {
	const secret = "MIGRATION-TOKEN-SENTINEL"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"id":"m1","status":"pending","token":"`+secret+`"}],"pages":{}}`)
	}))
	defer srv.Close()
	t.Setenv("SOPHOS_CENTRAL_HOME", t.TempDir())
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "")

	for _, args := range [][]string{
		{"export", "migrations", "--no-cache"},
		{"export", "migrations", "m1", "--no-cache"},
		{"tail", "migrations", "--follow=false"},
	} {
		// export and tail write records to os.Stdout directly.
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := os.Stdout
		os.Stdout = w
		cmd := RootCmd()
		cmd.SetOut(w)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		runErr := cmd.Execute()
		os.Stdout = orig
		_ = w.Close()
		var out bytes.Buffer
		_, _ = io.Copy(&out, r)
		if runErr != nil {
			t.Fatalf("%v: %v", args, runErr)
		}
		if !strings.Contains(out.String(), "m1") {
			t.Fatalf("%v printed no record: %q", args, out.String())
		}
		if strings.Contains(out.String(), secret) {
			t.Fatalf("%v printed the migration token: %q", args, out.String())
		}
	}
}
