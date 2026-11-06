// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

import (
	"encoding/json"
	"strings"
	"testing"
)

// Class false-withheld-own-metadata: agent-context describes the CLI's own
// auth (mode, env var names, a hint) under an "auth" key; the chokepoint must
// redact that container without withholding the whole description, with no
// credential configured, with one in the environment and with one in a config
// file.
func TestAgentContextIsNeverWithheld(t *testing.T) {
	sv := echoServer(t)
	for _, mode := range []credMode{credStored, credEnv, credConfigFile} {
		var e *bbEnv
		if mode == credStored {
			// a clean environment: nothing configured at all
			h := t.TempDir()
			e = &bbEnv{home: h, env: cleanEnv(h, "KNOWBE4_BASE_URL="+sv.url, "KNOWBE4_CA_CERT="+sv.caFile)}
		} else {
			e = newEnv(t, sv, mode)
		}
		out := e.run(t, "agent-context")
		var doc struct {
			CLI      map[string]any `json:"cli"`
			Commands []any          `json:"commands"`
		}
		if err := json.NewDecoder(strings.NewReader(out)).Decode(&doc); err != nil || len(doc.Commands) == 0 {
			t.Fatalf("%v: agent-context is not the CLI's description (err %v):\n%.600s", mode, err, out)
		}
		assertNoEcho(t, "agent-context ("+mode.String()+")", out)
	}
}
