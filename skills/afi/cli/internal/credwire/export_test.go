// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-file-closed-on-error):
// export --output opens its file through the credential kit before the first
// page; an API error after that must still close it and end the run.

package credwire

import (
	"net/http"
	"path/filepath"
	"testing"
)

func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	e := newEnv(t, func(w http.ResponseWriter, _ *http.Request, _ seen) {
		http.Error(w, `{"code":16,"message":"unauthorized"}`, http.StatusUnauthorized)
	}, "AFI_API_KEY="+dryRunKey)
	r := e.run(t, "", "export", "applications", "--output", filepath.Join(e.home, "applications.jsonl"))
	if r.code == 0 {
		t.Fatalf("export --output succeeded on a 401:\n%s", r.out)
	}
	if got := e.api.all(); len(got) != 1 || got[0].Path != "/api/v1/applications/me/installations" {
		t.Fatalf("want the one GET /applications/me/installations, got %+v\n%s", got, r.out)
	}
}
