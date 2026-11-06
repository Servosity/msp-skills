// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// Class secret-in-composed-encoding (factory Codex round 2): a held
// credential behind two decodings the credential leak contract names
// (section 3: base85, then a zlib stream) must never reach stdout or stderr.
// The carrier is the round-2 input: an Ascii85 run whose ten bytes are a zlib
// stream that inflates to the token (TestZZKF_rewst_CarrierChains in the
// kit's findings check). The shared credential kit closes it by recursion
// over every decoder; the connector's own matcher, which skipped streams
// shorter than 16 bytes, printed it.
const composedToken = "ZZZZZZZZ" + "ZZZZZZZZ" + "ZZZZZZZZ" + "ZZZZZZZZ"

// composedCarrier is Ascii85 of the zlib stream 789c8b220000b9c00b41,
// which inflates to composedToken.
const composedCarrier = "Gas[9!!'RF$UF"

func TestSecretInComposedEncodingNeverPrints(t *testing.T) {
	budget(t)
	sv := newServer()
	answer := `{"data":{"userOrganization":{"id":"msp","name":"MSP","isMsp":true,"managingOrgId":null,` +
		`"managedAndSubOrgs":[{"id":"client","name":"` + composedCarrier + `","managingOrgId":"msp","isEnabled":true,"isDeleted":false}]}}}`
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path)
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, answer)
	}))
	sv.url, sv.caFile = srv.URL, caFileFor(t, srv)
	home := t.TempDir()
	env := cleanEnv(home, "REWST_API_TOKEN="+composedToken, "REWST_BASE_URL="+sv.url, "REWST_CA_CERT="+sv.caFile,
		"REWST_NO_LEARN=1")
	for _, c := range []struct {
		where string
		args  []string
	}{
		{"stdout (orgs answer)", []string{"orgs", "--quiet", "--select", "managed_orgs.name"}},
		{"stdout (orgs --json)", []string{"orgs", "--json"}},
		{"stderr (unknown command)", []string{composedCarrier}},
	} {
		cmd := exec.Command(bbCLI, c.args...)
		cmd.Env, cmd.Dir = env, home
		count(&cliRuns, 1)
		out, _ := cmd.CombinedOutput()
		if strings.Contains(string(out), composedCarrier) {
			t.Errorf("%s: the Ascii85-then-zlib carrier of the held token printed (rewst-cli %s):\n%.1500s",
				c.where, strings.Join(c.args, " "), out)
		}
	}
}
