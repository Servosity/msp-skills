// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// edgeCookies are the cookies Xero's edge sets on API and identity responses
// (Akamai Bot Manager on api.xero.com, the device cookie on identity.xero.com).
// The kit holds every Set-Cookie value unless an Identifier names it, so a
// 452-character ak_bmsc withheld ordinary output that shared any 8-character
// piece with it: `doctor --json` printed only the withhold notice on the
// Windows smoke run against the real API (CI, PR #652).
var edgeCookies = []string{"ak_bmsc", "bm_sv", "bm_sz", "bm_mi", "_abck", "Device"}

// TestEdgeCookiesNeverWithholdOrdinaryOutput (class edge-cookie-held): a
// response that sets one of Xero's edge cookies, whose value shares a piece
// with the response body and with doctor's report, still prints in full on
// the CLI, the --json output and over MCP. Control: a cookie the Declarations
// do not name is still held, so the exemption is exact, not a blanket one.
func TestEdgeCookiesNeverWithholdOrdinaryOutput(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const shared = "EdgeCookieShared7Kq"
	const token = "xerotok-" + "Ec4Wq9Lm2Vz7"
	body := `{"Accounts":[{"AccountID":"5040915e-8ce7-4177-8d08-fde416232f18","Name":"` + shared + `","Status":"ACTIVE"}]}`
	for _, name := range edgeCookies {
		t.Run(name, func(t *testing.T) {
			var v *vendor
			v = startVendor(t, func(w http.ResponseWriter, r *http.Request) {
				// The value carries the account name and the stub's own origin, so a
				// held cookie collides with both the body and doctor's base_url.
				w.Header().Add("Set-Cookie", fmt.Sprintf("%s=~0~%s~%s~YAAQ; Path=/; Secure; HttpOnly", name, shared, v.url))
				_, _ = w.Write([]byte(body))
			})
			home := newHome(t)
			env := closedEnv(home, v.env(token)...)
			for _, args := range [][]string{{"accounts", "get"}, {"accounts", "get", "--json"}} {
				out := run(t, cliBin, env, args...)
				if !strings.Contains(out, shared) || strings.Contains(out, "credkit: withheld") {
					t.Errorf("%s with a %s cookie: output lost:\n%s", strings.Join(args, " "), name, out)
				}
			}
			if out := mcpCall(t, mcpBin, env, `{"name":"accounts_get","arguments":{}}`); !strings.Contains(out, shared) || strings.Contains(out, "credkit: withheld") {
				t.Errorf("MCP accounts_get with a %s cookie: output lost:\n%s", name, out)
			}
			out := run(t, cliBin, env, "doctor", "--json", "--timeout", "15s")
			start := strings.Index(out, "{")
			var report map[string]any
			if start < 0 || json.Unmarshal([]byte(out[start:]), &report) != nil {
				t.Errorf("doctor --json with a %s cookie: stdout is not a JSON object:\n%s", name, out)
			}
			if v.hits.Load() == 0 {
				t.Error("no request reached the API stub, so no cookie was set")
			}
		})
	}
	t.Run("undeclared-cookie-still-held", func(t *testing.T) {
		v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Set-Cookie", "session=~0~"+shared+"~YAAQ; Path=/; Secure; HttpOnly")
			_, _ = w.Write([]byte(body))
		})
		env := closedEnv(newHome(t), v.env(token)...)
		if out := run(t, cliBin, env, "accounts", "get", "--json"); strings.Contains(out, shared) {
			t.Errorf("a cookie the Declarations do not name was not held:\n%s", out)
		}
	})
}
