// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// P2 (kit-findings): an OrgVariable's value is a credential when its category
// is secret, under the generic name value that every other category uses for
// Rewst data. The credential kit holds a response field by its name alone, so
// it cannot hold "value when category is secret", and the connector builds no
// holder of its own beside the kit (Rewst's own maskSecrets argument is the
// vendor-side answer). The same holds for a credential pair in general (a
// member named value beside one that says password or secret) and for a
// secret value the user sends, which a write's preview repeats. Runs only
// with REWST_BB_P2=1; fails while the P2 stands.
func TestOrgVariableSecretValueHeldKnownP2(t *testing.T) {
	budget(t)
	if os.Getenv("REWST_BB_P2") == "" {
		t.Skip("P2 an org variable value of category secret is not held (the kit holds by name); set REWST_BB_P2=1 to reproduce")
	}
	sv := echoServer(t)
	sv.quiet, sv.bare = true, true
	e := newEnv(t, sv, credEnv)
	out := e.run(t, "org-variables", "get", "--visible-for-org-id", bbOrgID, "--json") +
		e.mcpCalls(t, []map[string]any{read("org-variables.get", map[string]any{"visibleForOrgId": bbOrgID})})
	if strings.Contains(out, bbOrgSecret) {
		t.Errorf("the secret org variable's value printed:\n%.2000s", out)
	}
	const sentSecret = "48261937"
	out = e.mcpCalls(t, []map[string]any{execute("org-variables.create", map[string]any{"name": "hq_numeric", "value": 48261937,
		"category": "secret", "orgId": bbOrgID, "cascade": false})})
	if strings.Contains(out, sentSecret) {
		t.Errorf("the preview printed the secret org variable value it would send:\n%.2000s", out)
	}
}

// P2 (kit-findings): a Zstandard frame is outside the leak contract's section 3
// (the kit cannot inflate it), so an unknown command spelled as a frame of a
// held value prints back. Input shape: KLUv/SAgAwEAWg== (an RLE frame). Runs
// only with REWST_BB_P2=1; fails while the P2 stands.
func TestZstdFrameOfHeldValueKnownP2(t *testing.T) {
	budget(t)
	if os.Getenv("REWST_BB_P2") == "" {
		t.Skip("P2 a Zstandard frame of a held value is out of contract and prints; set REWST_BB_P2=1 to reproduce")
	}
	e := newEnv(t, echoServer(t), credEnv)
	frame := base64.StdEncoding.EncodeToString(zstdRLEFrame(bbToken))
	if out := e.run(t, frame); strings.Contains(out, frame) {
		t.Errorf("the Zstandard frame of the held token printed:\n%.1000s", out)
	}
}
