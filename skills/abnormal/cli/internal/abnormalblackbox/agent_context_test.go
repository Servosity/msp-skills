// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

import (
	"encoding/json"
	"strings"
	"testing"
)

// Class false-withheld-own-metadata: agent-context describes the CLI's own
// auth (mode, env var names, a hint) under an "auth" key; the kit must not
// withhold the description, with no credential configured and with one.
func TestAgentContextIsNeverWithheld(t *testing.T) {
	sv := echoServer(t)
	for _, mode := range []credMode{credStored, credEnv} {
		e := newEnv(t, sv, mode)
		if mode == credStored {
			// a clean environment: nothing configured at all
			e = &bbEnv{home: e.home, env: append([]string(nil), e.env...)}
		}
		out := e.run(t, "agent-context")
		var doc struct {
			CLI      map[string]any `json:"cli"`
			Commands []any          `json:"commands"`
		}
		if strings.Contains(out, "[credkit: withheld") {
			// A credential-free reason: the kit's notice names the sink, and the
			// home holds only test paths (on Windows a \001 segment there reads
			// as an octal escape; see TestAgentContextPathsCarryNoBackslashOnWindows).
			t.Fatalf("%v: the kit withheld agent-context (%d bytes printed, home %q, paths printed with a backslash: %v); first line: %.200s",
				mode, len(out), e.home, strings.Contains(out, `\\`), strings.SplitN(strings.TrimSpace(out), "\n", 2)[0])
		}
		if err := json.NewDecoder(strings.NewReader(out)).Decode(&doc); err != nil || len(doc.Commands) == 0 {
			t.Fatalf("%v: agent-context is not the CLI's description (err %v):\n%.600s", mode, err, out)
		}
		assertNoEcho(t, "agent-context ("+mode.String()+")", out)
	}
}
