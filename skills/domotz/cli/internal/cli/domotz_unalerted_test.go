// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// fleet unalerted (novel feature added in the 4.34.2 reprint): the devices no
// enabled alert profile is bound to, from GET /alert-profile (AlertProfile id,
// is_enabled) and each Collector's GET /alert-profile/binding/agent/{agent_id}/device
// (AlertProfileDeviceBinding alert_profile_id, device_id) and device list.

import (
	"strings"
	"testing"

	"domotz-pp-cli/internal/cliutil/testenv"
)

func TestFleetUnalertedListsDevicesNoEnabledProfileWatches(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"NAS-Acme","status":"ONLINE","importance":"VITAL"},{"id":2,"display_name":"AP-Acme","status":"ONLINE","importance":"FLOATING"}]`,
		"200": `[{"id":3,"display_name":"FW-Beta","status":"ONLINE","importance":"VITAL"},{"id":4,"display_name":"Printer-Beta","status":"ONLINE","importance":"FLOATING"}]`,
	})
	fake.bodies["/alert-profile"] = `[{"id":210,"name":"Down Alert","is_enabled":true},{"id":211,"name":"Muted","is_enabled":false}]`
	fake.bodies["/alert-profile/binding/agent/100/device"] = `[{"alert_profile_id":210,"device_id":2}]`
	fake.bodies["/alert-profile/binding/agent/200/device"] = `[{"alert_profile_id":211,"device_id":3}]`

	out := runDomotz(t, "fleet", "unalerted", "--json", "--no-learn")
	for _, w := range []string{"NAS-Acme", "FW-Beta", "Printer-Beta"} {
		if !strings.Contains(out, w) {
			t.Errorf("fleet unalerted: missing %s (no enabled profile watches it) in\n%s", w, out)
		}
	}
	if strings.Contains(out, "AP-Acme") {
		t.Errorf("fleet unalerted lists AP-Acme, which enabled profile 210 watches:\n%s", out)
	}
	if strings.Index(out, "Printer-Beta") < strings.Index(out, "FW-Beta") {
		t.Errorf("fleet unalerted lists a FLOATING device before a VITAL one:\n%s", out)
	}
	if out := runDomotz(t, "fleet", "unalerted", "--vital-only", "--json", "--no-learn"); strings.Contains(out, "Printer-Beta") {
		t.Errorf("fleet unalerted --vital-only lists a FLOATING device:\n%s", out)
	}
}
