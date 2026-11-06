// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex review round 1 (domotz 4.34.2 reprint) class mask-shaped-credential
// (holder-incomplete): a credential whose value reads like a mask ("hidden",
// "masked", "********", "withheld", "redacted", "[redacted]", bullets) was
// taken for one the API had already hidden, so it was neither held nor
// redacted, and a preview printed it. A value the user sends is the credential
// whatever it reads as. What a runZero user sends: the API key
// (RUNZERO_API_KEY, in Authorization: Bearer) and a credential-named query
// value (sync --param password=...). The fake echoes the key it received and
// the query value, under a credential name and a plain one; each value runs
// in its own processes (a held "[redacted]" withholds every later redacted
// text of its session), once as the key and once as the query value.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// maskShapedValues are credentials that read like masks: mask words, the
// program's own redaction marker and journal class, and glyph runs.
var maskShapedValues = []string{"hidden", "masked", "********", "withheld", "redacted", "[redacted]", "•••••", "<hidden>", "(masked)"}

// withoutNotices drops the credential kit's notices and the program's own
// response field mask, which carry the word "withheld" themselves.
func withoutNotices(s string) string {
	for _, n := range []string{
		kitNotice("stdout"), kitNotice("stderr"), kitNotice("mcp"), kitNotice("outbound request"),
		"[credential withheld]",
	} {
		s = strings.ReplaceAll(s, n, "")
	}
	return s
}

func TestMaskShapedCredentialsNeverPrint(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	for i, v := range maskShapedValues {
		for _, as := range []string{"key", "query-value"} {
			v, as := v, as
			// The subtest name (and so the temp home every path names) never
			// spells the value: a query value is held only by the processes
			// given it, and doctor prints its paths.
			t.Run(fmt.Sprintf("%s-%d", as, i), func(t *testing.T) {
				sv := shapeServer(t, func(p string, r *http.Request) (string, bool) {
					echo := bearerKey(r)
					switch p {
					case "/account/keys":
						return fmt.Sprintf(`[{"id":%q,"organization_id":%q,"comment":%q,"last_used_ip":%q,"token":%q}]`,
							bbKeyID, bbOrgID, echo, bbLastUsedIP, echo), true
					case "/org/assets":
						pw := r.URL.Query().Get("password")
						return fmt.Sprintf(`[{"id":%q,"names":["hq-mask"],"addresses":[%q],"comments":%q,"secret":%q}]`,
							bbAssetID, bbAddr, pw, pw), true
					}
					return "", false
				})
				e := newEnv(t, sv, credEnv)
				key, pw := v, "QueryPassw0rd-mask1"
				if as == "query-value" {
					key, pw = bbKey, v
				}
				e.env = append(withoutEnv(e.env, "RUNZERO_API_KEY"), "RUNZERO_API_KEY="+key)
				// Every process given the value runs and is checked: all of them
				// for the key, the ones that send the query value for it (a
				// process that never had "[redacted]" prints the program's own
				// marker for an API key's token, which is not the credential).
				var out strings.Builder
				for _, args := range [][]string{
					{"account", "get-keys", "--json"},
					{"account", "get-keys", "--csv"},
					{"sync", "--resources", "org-assets", "--param", "password=" + pw, "--json"},
					{"sync", "--resources", "org-assets", "--param", "password=" + pw, "--dry-run"},
					{"auth", "status"},
					{"doctor"},
				} {
					if as == "key" || strings.Contains(strings.Join(args, " "), "password=") {
						out.WriteString(e.run(t, args...))
					}
				}
				calls := []map[string]any{tool("sync", map[string]any{"resources": []any{"org-assets"}, "param": []any{"password=" + pw}})}
				if as == "key" {
					calls = append([]map[string]any{read("account.get-keys", nil)}, calls...)
				}
				out.WriteString(e.mcpCalls(t, calls))
				got := out.String()
				// A key that occurs in the program's own public text ("********"
				// is how a preview writes Authorization) is refused at startup by
				// the credential kit: nothing is sent or printed, which closes
				// the class too.
				if n, _ := sv.sawKey(key); n == 0 && !strings.Contains(got, "[credkit: refusing to start: a startup credential occurs in public text") {
					t.Fatalf("no request carried the key %q; the case did not run:\n%.3000s", key, got)
				}
				if strings.Contains(withoutNotices(got), v) {
					t.Errorf("output repeats the credential %q:\n%.4000s", v, got)
				}
			})
		}
	}
}
