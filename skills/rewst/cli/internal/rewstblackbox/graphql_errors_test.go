// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// graphql-errors-are-failures, padded form (an adversarial shape the class
// fix already covers): Rewst's not-authorized answer is HTTP 200 with an
// errors array, which internal/cli/rewst_graphql_errors.go turns into a
// failure. The same answer behind a UTF-8 byte-order mark is read the way the
// client reads it (the mark dropped), so a typed read still fails with the
// AUTH_ERR text, never exits 0 as if the API had answered no data.
func TestGraphQLErrorsBehindAByteOrderMarkAreFailures(t *testing.T) {
	budget(t)
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "\xef\xbb\xbf"+apolloAuthError("myAccessibleOrganizations", nil, `{"note":"echo"}`))
	}))
	sv.url, sv.caFile = srv.URL, caFileFor(t, srv)
	e := newEnv(t, sv, credEnv)
	cmd := exec.Command(bbCLI, "organizations", "get", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("organizations get exited 0 on a BOM-prefixed AUTH_ERR answer (the API said Not Authorized):\n%.1500s", out)
	}
	if !strings.Contains(string(out), "AUTH_ERR") {
		t.Errorf("organizations get lost the AUTH_ERR answer's text:\n%.1500s", out)
	}
}
