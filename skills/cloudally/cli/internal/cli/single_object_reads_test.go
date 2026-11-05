// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/cliutil/testenv"
)

// A single-object read returns the whole object. The press inferred a
// response path from one array field of these responses (reportEmails on
// GET /v2/user, entities on a backup task), so the user, the task's own
// status, name and source were dropped and only that array was printed.
func TestSingleObjectReadsReturnTheWholeObject(t *testing.T) {
	user := `{"id":"u-1","name":"Example MSP","email":"ops@example.com","status":"ACTIVE","reportEmails":["reports@example.com"]}`
	task := `{"id":"64b0e9","name":"Contoso MS365","source":"MS365","status":"PAUSED","numOfEntities":2,` +
		`"entities":[{"entity":"jane@contoso.com","status":"ACTIVE"},{"entity":"bob@contoso.com","status":"ACTIVE"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/backup-tasks/") {
			_, _ = w.Write([]byte(task))
			return
		}
		_, _ = w.Write([]byte(user))
	}))
	defer srv.Close()

	cases := []struct {
		name string
		args []string
		want map[string]string
	}{
		{"user me", []string{"user", "me"}, map[string]string{"name": "Example MSP", "email": "ops@example.com"}},
		{"tasks get", []string{"tasks", "get", "64b0e9"}, map[string]string{"status": "PAUSED", "source": "MS365"}},
		{"distributor-account-tasks get", []string{"distributor-account-tasks", "get", "acct-1", "64b0e9"}, map[string]string{"status": "PAUSED", "name": "Contoso MS365"}},
		{"reseller-account-tasks get", []string{"reseller-account-tasks", "get", "acct-1", "64b0e9"}, map[string]string{"status": "PAUSED", "name": "Contoso MS365"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
			t.Setenv("CLOUDALLY_BASE_URL", srv.URL)
			stdout, stderr, err := runRootArgs(t, append(tc.args, "--data-source", "live", "--json")...)
			if err != nil {
				t.Fatalf("exit error: %v (stderr=%q)", err, stderr)
			}
			var env struct {
				Results json.RawMessage `json:"results"`
			}
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
			}
			var obj map[string]any
			if err := json.Unmarshal(env.Results, &obj); err != nil {
				t.Fatalf("results is not the object: %v\n%s", err, env.Results)
			}
			for k, v := range tc.want {
				if obj[k] != v {
					t.Errorf("results.%s = %v, want %q\n%s", k, obj[k], v, env.Results)
				}
			}
		})
	}

	// The documented tasks get example selects into the task's entities.
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)
	stdout, stderr, err := runRootArgs(t, "tasks", "get", "64b0e9", "--data-source", "live", "--agent", "--select", "entities.entity,entities.status")
	if err != nil || !strings.Contains(stdout, "jane@contoso.com") {
		t.Fatalf("documented example failed: err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
}
