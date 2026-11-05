// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package umbrellablackbox

import (
	"strings"
	"testing"
)

// The first sync on a fresh home is the store every later run with the same key
// reads: the first mint saves the key pair as client_id and client_secret, and that
// must not move the run to another database, with the variables still set or unset.
func TestFirstSyncIsTheStoreLaterRunsRead(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{networksPath: `[{"originId":222,"name":"Branch","ipAddress":"198.51.100.8","isVerified":false}]`})
	if out := e.run(t, "sync", "--resources", "networks", "--no-learn"); !strings.Contains(out, `"sync_complete"`) {
		t.Fatalf("sync:\n%.800s", out)
	}
	hygiene := []string{"hygiene", "--kind", "network-unverified", "--json", "--no-learn"}
	if out := e.run(t, hygiene...); !strings.Contains(out, "222") {
		t.Errorf("with the key still in the environment, hygiene does not see the first sync:\n%.800s", out)
	}
	if out := e.without("CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET").run(t, hygiene...); !strings.Contains(out, "222") {
		t.Errorf("with the key only in the credential store, hygiene does not see the first sync:\n%.800s", out)
	}
}
