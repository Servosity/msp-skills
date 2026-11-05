// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

// Codex review round 1 (domotz 4.34.2 reprint) class mask-shaped-credential
// (holder-incomplete): a credential whose value reads like a mask ("hidden",
// "masked", "********", "withheld", "redacted", "[redacted]", bullets) was
// taken for one the API had already hidden, so it was neither held nor
// redacted, and a preview printed it. A value the user sends is the credential
// whatever it reads as. What a RocketCyber user sends: the API token
// (ROCKETCYBER_API_TOKEN, Authorization: Bearer <token>) and a
// credential-named query value (sync --param password=...). The fake echoes
// the token it received and the query value, under a credential name and a
// plain one; each value runs in its own processes (a held "[redacted]"
// withholds every later redacted text of its session).

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// maskShapedValues are credentials that read like masks: mask words, the
// program's own redaction marker and journal class, and glyph runs.
var maskShapedValues = []string{"hidden", "masked", "********", "withheld", "redacted", "[redacted]", "•••••", "<hidden>", "(masked)"}

// kitNoticeText is the fixed text of the kit's withhold notices and its startup
// refusal (credential leak contract section 1, public text 1), which carry
// the word "withheld" themselves. The source a refusal names is a variable
// name, never the value.
var kitNoticeText = regexp.MustCompile(`\[credkit: (withheld from [a-z ]+: it carried a credential this program holds|refusing to start: a startup credential occurs in public text(: [A-Za-z0-9_:.-]+)?)\]`)

// withoutNotices drops the kit's notices.
func withoutNotices(s string) string {
	return kitNoticeText.ReplaceAllString(s, "")
}

// bearerToken is the token a request carried as Authorization: Bearer <token>.
func bearerToken(r *http.Request) string {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return tok
}

func TestMaskShapedCredentialsNeverPrint(t *testing.T) {
	for i, v := range maskShapedValues {
		v := v
		// Named by index: a subtest name is in its temp paths, which the
		// program prints (config_path), and "hidden" there is the test's
		// text, not the credential.
		t.Run(fmt.Sprintf("value-%d", i), func(t *testing.T) {
			sv := shapeServer(t, func(p string, r *http.Request) (string, bool) {
				switch p {
				case "/agents":
					return fmt.Sprintf(`{"totalCount":1,"currentPage":1,"totalPages":1,"data":[{"id":"asda12341aasdafg1ag1llk412a","hostname":"HQ-Mask agent","connectivity":"online","note":%q,"token":%q}]}`,
						bearerToken(r), bearerToken(r)), true
				case "/apps":
					pw := r.URL.Query().Get("password")
					return fmt.Sprintf(`{"totalCount":1,"currentPage":1,"totalPages":1,"data":[{"id":60,"name":"HQ-Mask app","status":"active","note":%q,"secret":%q}]}`, pw, pw), true
				case "/suppression/rule/42":
					return fmt.Sprintf(`{"ruleId":42,"ruleName":"HQ-Mask rule","status":"active","comment":%q,"token":%q}`, bearerToken(r), bearerToken(r)), true
				}
				return "", false
			})
			e := newEnv(t, sv, credEnv)
			e.env = append(withoutEnv(e.env, "ROCKETCYBER_API_TOKEN"), "ROCKETCYBER_API_TOKEN="+v)
			var out strings.Builder
			for _, args := range [][]string{
				{"agents", "--json"},
				{"agents", "--csv"},
				{"suppression", "rule", "42", "--json"},
				{"sync", "--resources", "apps", "--param", "password=" + v, "--json"},
				{"sync", "--resources", "apps", "--param", "password=" + v, "--dry-run"},
				{"agents", "--hostname", v, "--dry-run"},
				{"auth", "status"},
				{"doctor"},
			} {
				out.WriteString(e.run(t, args...))
			}
			out.WriteString(e.mcpCalls(t, []map[string]any{
				tool("agents_list", map[string]any{}),
				tool("suppression_rule", map[string]any{"ruleId": 42}),
				tool("sync", map[string]any{"resources": []any{"apps"}, "param": []any{"password=" + v}}),
			}))
			got := out.String()
			if n, _ := sv.sawKey(v); n == 0 {
				// A value that occurs in the kit's public text ("withheld" is in
				// its own notices) is refused at startup (exit 78): nothing runs,
				// so nothing can repeat it. Anything else must have been sent.
				if !strings.Contains(got, "[credkit: refusing to start: a startup credential occurs in public text") {
					t.Fatalf("no request carried the token %q and the kit did not refuse to start; the case did not run:\n%.3000s", v, got)
				}
			}
			if strings.Contains(withoutNotices(got), v) {
				t.Errorf("output repeats the credential %q:\n%.4000s", v, got)
			}
		})
	}
}
