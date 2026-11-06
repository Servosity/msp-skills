// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Codex review round 1 (domotz 4.34.2 reprint) class mask-shaped-credential
// (holder-incomplete): a credential whose value reads like a mask ("hidden",
// "masked", "********", "withheld", "redacted", "[redacted]", bullets) was
// taken for one the API had already hidden, so it was neither held nor
// redacted, and a preview printed it. A value the user sends is the credential
// whatever it reads as. What a Rewst user sends: the API token
// (REWST_API_TOKEN, Authorization: Bearer <token>), a credential-named query
// value (sync --param password=...) and a secret org variable's value
// (org-variables create --category secret). The fake echoes the token it
// received and the query value, under a credential name and a plain one, on
// a REST-shaped read and in a GraphQL answer a novel prints (a workflow named
// with the token); each value runs in its own processes (a held "[redacted]"
// withholds every later redacted text of its session). A value that occurs in
// the credential kit's public text (its notices: "withheld", or the
// "********" it writes for a withheld header) makes the kit refuse to start
// (leak contract section 2, public text), so nothing is sent and nothing but
// the refusal prints. Subtests are numbered, not named by the value: the
// test's temporary directories are named after the subtest, and doctor
// prints their paths.

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// maskShapedValues are credentials that read like masks: mask words, the
// program's own redaction marker and journal class, and glyph runs.
var maskShapedValues = []string{"hidden", "masked", "********", "withheld", "redacted", "[redacted]", "•••••", "<hidden>", "(masked)"}

// withoutNotices drops the credential kit's notices, which carry the word
// "withheld" themselves (public text, so no held value occurs in one).
func withoutNotices(s string) string {
	return regexp.MustCompile(`\[credkit: [^\]]*\]`).ReplaceAllString(s, "")
}

// kitRefusedStart is the kit's startup refusal for a held value that occurs in
// its public text.
const kitRefusedStart = "[credkit: refusing to start: a startup credential occurs in public text"

// bearer is the token a request carried as Authorization: Bearer <token>.
func bearer(r *http.Request) string {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return tok
}

func TestMaskShapedCredentialsNeverPrint(t *testing.T) {
	budget(t)
	values := maskShapedValues
	if !fullSweep {
		values = []string{"masked", "withheld"} // one the kit holds and runs with, one that makes it refuse to start
	}
	for i, v := range values {
		v := v
		t.Run(fmt.Sprintf("value-%d", i), func(t *testing.T) {
			sv := shapeServer(t, func(s shapeReq) (string, bool) {
				tok, pw := bearer(s.r), s.q.Get("password")
				switch {
				case s.field() == "workflowExecutions":
					return fmt.Sprintf(`{"workflowExecutions":[{"id":"ac5a6e3c-7f1b-4c2e-9a4d-5b6c7d8e9fa0","status":"FAILED","createdAt":%q,`+
						`"numSuccessfulTasks":0,"numAwaitingResponseTasks":0,"workflow":{"id":"HQ-Mask-workflow","name":%q},"token":%q}]}`,
						time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), tok, tok), true
				case s.g.isDoc():
					return "", false
				case s.r.Method == http.MethodGet:
					return fmt.Sprintf(`{"myAccessibleOrganizations":[{"id":%q,"name":"HQ-Mask org","note":%q,"token":%q}],`+
						`"pages":[{"id":%q,"name":"HQ-Mask page","note":%q,"secret":%q}]}`, bbOrgID, tok, tok, bbPageID, pw, pw), true
				}
				return "", false
			})
			e := newEnv(t, sv, credEnv).lean()
			e.env = append(withoutEnv(e.env, "REWST_API_TOKEN"), "REWST_API_TOKEN="+v)
			var out strings.Builder
			for j, args := range [][]string{
				{"organizations", "get", "--json"},
				{"organizations", "get", "--csv"},
				{"failures", "--org", bbOrgID, "--json"},
				{"failures", "--org", bbOrgID},
				{"sync", "--resources", "pages", "--param", "password=" + v, "--json"},
				{"sync", "--resources", "pages", "--param", "password=" + v, "--dry-run"},
				{"org-variables", "create", "--name", "mask_secret", "--value", v, "--category", "secret", "--org-id", bbOrgID, "--cascade", "false", "--dry-run"},
				{"auth", "status"},
				{"doctor"},
			} {
				if !fullSweep && (j > 0 && v == "withheld" || args[0] == "sync" && args[len(args)-1] == "--dry-run") {
					continue // a refusal is the same refusal; a sync's preview runs in the numeric and whitespace tests
				}
				out.WriteString(e.run(t, args...))
			}
			out.WriteString(e.mcpCalls(t, withoutTools([]map[string]any{
				read("organizations.get", map[string]any{}),
				tool("failures", map[string]any{"org": bbOrgID}),
				tool("sync", map[string]any{"resources": []any{"pages"}, "param": []any{"password=" + v}}),
				execute("org-variables.create", map[string]any{"name": "mask_secret", "value": v, "category": "secret", "orgId": bbOrgID, "cascade": false}),
			}, "sync")))
			got := out.String()
			refused := strings.Contains(got, kitRefusedStart)
			if n, _ := sv.sawKey(v); n == 0 && !refused {
				t.Fatalf("no request carried the token %q; the case did not run:\n%.3000s", v, got)
			}
			if m := sv.allMethods(); refused && len(m) != 0 {
				t.Errorf("the kit refused to start with %q, yet requests were sent: %v", v, m)
			}
			if strings.Contains(withoutNotices(got), v) {
				t.Errorf("output repeats the credential %q:\n%.4000s", v, got)
			}
		})
	}
}
