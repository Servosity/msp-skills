// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package umbrellablackbox

import (
	"os"
	"path/filepath"
	"testing"
)

// An MSP switches customers by naming another key. The key named on this run (the
// environment, or the config file it loads) wins over the key and bearer a previous
// run stored, so a write never lands in the previous customer's organization.
func TestNamedKeyBeatsTheStoredOne(t *testing.T) {
	t.Parallel()
	const keyA, secretA = "tenantKeyA-" + "3f9c1e7a5d2b", "tenantSecretA-" + "8d2b6f4a1c9e7e31"
	const keyB, secretB = "tenantKeyB-" + "6a1d9e3c7f5b", "tenantSecretB-" + "2c7e5a9d3b1f6e84"
	read := []string{"networks", "list", "--data-source", "live", "--json"}
	pair := func(k, s string) []string {
		return []string{"CISCO_UMBRELLA_API_KEY=" + k, "CISCO_UMBRELLA_API_SECRET=" + s}
	}
	mintedWith := func(t *testing.T, sv *server, want, out string) {
		t.Helper()
		if u, p := sv.mintBasic(); u != want {
			t.Errorf("the second run minted with %q/%q, want key %q (the stored bearer was reused):\n%.800s", u, p, want, out)
		}
	}
	t.Run("environment, CLI write", func(t *testing.T) {
		t.Parallel()
		sv, base := honestEnv(t, nil)
		e := base.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET")
		e.with(pair(keyA, secretA)...).run(t, read...)
		mintedWith(t, sv, keyA, "")
		writes := sv.writes.Load()
		out := e.with(pair(keyB, secretB)...).run(t, "networks", "delete", "111", "--agent", "--yes")
		if sv.writes.Load() == writes {
			t.Fatalf("the delete was not sent:\n%.800s", out)
		}
		mintedWith(t, sv, keyB, out)
	})
	t.Run("environment, MCP", func(t *testing.T) {
		t.Parallel()
		sv, base := honestEnv(t, nil)
		e := base.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET")
		e.with(pair(keyA, secretA)...).run(t, read...)
		out := e.with(pair(keyB, secretB)...).mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "networks.list"}}})
		mintedWith(t, sv, keyB, out)
	})
	t.Run("named config files", func(t *testing.T) {
		t.Parallel()
		sv, base := honestEnv(t, nil)
		e := base.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET")
		conf := func(name, k, s string) string {
			p := filepath.Join(e.home, name)
			if err := os.WriteFile(p, []byte("umbrella_api_key = \""+k+"\"\numbrella_api_secret = \""+s+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return p
		}
		a, b := conf("customerA.toml", keyA, secretA), conf("customerB.toml", keyB, secretB)
		e.run(t, append([]string{"--config", a}, read...)...)
		mintedWith(t, sv, keyA, "")
		out := e.run(t, append([]string{"--config", b}, read...)...)
		mintedWith(t, sv, keyB, out)
	})
	t.Run("the same key keeps its bearer", func(t *testing.T) {
		t.Parallel()
		sv, e := honestEnv(t, nil)
		e.run(t, read...)
		hits := sv.hits.Load()
		e.run(t, read...)
		if n := sv.hits.Load() - hits; n != 1 {
			t.Errorf("the second run with the same key sent %d requests, want 1 (no new mint)", n)
		}
	})
}
