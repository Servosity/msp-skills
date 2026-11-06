// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// Two held-value sources the connector's own layer once held in process
// (internal/cli TestResponseCredentialsHeld and TestArgvCredentialsHeld,
// retired with that layer), proven on the built binaries through the kit.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// response-credentials-held: a credential a response carries in a declared
// field (Declarations.ResponseFields), nested in a record and deflate-encoded,
// is held before the response reaches any sink, so a later call that repeats
// it prints neither. The control answers the same values under a field that
// is not a credential, and the later call prints them, so the case proves the
// field name, not the values, is what the kit held.
func TestResponseCredentialsHeldAtDepthAndDeflated(t *testing.T) {
	_, mcpBin := binaries(t)
	const nested, top = "rhnest-kqzwmkqzwm", "rhtop-wmqzkwmqzk"
	session := func(field string) string {
		base, ca := fakeAPIFunc(t, func(h http.Header, r *http.Request) string {
			if strings.HasSuffix(r.URL.Path, "/me") {
				return fmt.Sprintf(`{"id":"user-1","name":"HQ-Me","note":"rotated %s and %s"}`, nested, top)
			}
			h.Set("Content-Encoding", "deflate")
			return deflate(`{"items":[{"id":"ds-1","name":"HQ-Record","` + field + `":"` + top + `","script":{"parameters":{"` + field + `":"` + nested + `"}}}],"total_items":1,"next_page":""}`)
		})
		env := closedEnv(t.TempDir(), "ACTION1_BASE_URL="+base, "ACTION1_CA_CERT="+ca, "ACTION1_OAUTH2=rhtok-zqkwmzqkwmzqkw")
		out, _, _ := mcpRun(t, mcpBin, env,
			call("action1_read", map[string]any{"endpoint_id": "organizations.get"}),
			call("action1_read", map[string]any{"endpoint_id": "me.me"}))
		return out
	}
	if out := session("label"); !strings.Contains(reply(out, 3), nested) || !strings.Contains(reply(out, 3), top) {
		t.Fatalf("control: the later call did not print values no credential field carried, so this test proves nothing:\n%.3000s", out)
	}
	for _, field := range []string{"access_token", "client_secret"} {
		out := session(field)
		if reply(out, 3) == "" {
			t.Fatalf("%s: the later call went unanswered:\n%.3000s", field, out)
		}
		for _, v := range []string{nested, top} {
			if strings.Contains(out, v) {
				t.Errorf("%s: a response credential reached the client (%q):\n%.3000s", field, v, out)
			}
		}
	}
}

// deflate is body compressed as HTTP's deflate content coding (zlib).
func deflate(body string) string {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write([]byte(body))
	_ = zw.Close()
	return z.String()
}

// Finding (.review/kit/test-helper-findings.md): auth set-token reads its
// token from stdin and takes no argument, so a token typed as an argument by
// mistake is refused, and Cobra's refusal repeats it ("unknown command
// <token>"). The command's argument check holds each argument before the
// refusal prints (auth.go), so the kit withholds the unit that repeats it.
func TestTokenTypedAsSetTokenArgumentIsWithheld(t *testing.T) {
	cliBin, _ := binaries(t)
	for _, args := range [][]string{
		{"auth", "set-token", "--json", "argvposzkqwmzkq"},
		{"auth", "set-token", "--", "-argvdashqzkwmzq"},
	} {
		tok := strings.TrimPrefix(args[len(args)-1], "-")
		out, errOut, _ := runSplit(t, cliBin, closedEnv(t.TempDir()), args...)
		// The refusal, or the kit's notice standing in for the stderr unit that carried it, proves the case ran.
		if !strings.Contains(out+errOut, "set-token") && !strings.Contains(errOut, "[credkit: withheld from stderr") {
			t.Fatalf("%v printed no refusal, so this case proves nothing:\n%s%s", args, out, errOut)
		}
		if strings.Contains(out+errOut, tok) {
			t.Errorf("%v: the refusal repeats the token typed as an argument:\n%s%s", args, out, errOut)
		}
	}
}
