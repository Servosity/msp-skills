// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestFleetSyncDryRunNamesEachNAS: fleet sync --dry-run printed only "would
// run fleet sync". It names each NAS it would contact (name, address without
// user info, TLS mode, the credential env-var NAMES and whether each is set,
// never a value), honours --nas, sends nothing and creates no mirror.
func TestFleetSyncDryRunNamesEachNAS(t *testing.T) {
	home := sweepIsolate(t)
	var hits atomic.Int32
	nas := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer nas.Close()
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", nas.URL, "--client", "Acme Corp", "--json"); code != 0 {
		t.Fatalf("fleet add acme: %d %s", code, errText)
	}
	if _, errText, code := runCLIErr(t, "fleet", "add", "lab", "--url", "http://192.0.2.20:5000", "--env-prefix", "LAB_NAS", "--json"); code != 0 {
		t.Fatalf("fleet add lab: %d %s", code, errText)
	}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "PWDRYRUN-4410")

	out, errText, code := runCLIErr(t, "fleet", "sync", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("fleet sync --dry-run: exit %d %s %s", code, out, errText)
	}
	var plan fleetSyncPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("plan: %v %s", err, out)
	}
	if !plan.DryRun || len(plan.NAS) != 2 || !strings.Contains(plan.Would, "nothing was sent") {
		t.Fatalf("plan does not name both NAS: %s", out)
	}
	byName := map[string]fleetSyncPlanNAS{}
	for _, n := range plan.NAS {
		byName[n.Name] = n
	}
	acme, lab := byName["acme"], byName["lab"]
	if acme.URL != nas.URL || acme.Client != "Acme Corp" || !acme.CredentialsReady ||
		!acme.CredentialVars["SYNOLOGY_ACME_ACCOUNT"] || !acme.CredentialVars["SYNOLOGY_ACME_PASSWORD"] {
		t.Fatalf("acme row: %+v", acme)
	}
	if lab.URL != "http://192.0.2.20:5000" || lab.CredentialsReady || lab.CredentialVars["LAB_NAS_PASSWORD"] ||
		lab.TLS != "none (plain http)" || !strings.Contains(lab.TransportWarning, "plain http://") {
		t.Fatalf("lab row: %+v", lab)
	}
	if strings.Contains(out, "PWDRYRUN-4410") || strings.Contains(out, "msp-admin") {
		t.Fatalf("the plan printed a credential value: %s", out)
	}

	out, _, code = runCLIErr(t, "fleet", "sync", "--dry-run", "--nas", "lab", "--json")
	if code != 0 || !strings.Contains(out, `"lab"`) || strings.Contains(out, `"acme"`) {
		t.Fatalf("--nas lab: exit %d %s", code, out)
	}
	if _, errText, code := runCLIErr(t, "fleet", "sync", "--dry-run", "--nas", "nope"); code == 0 || !strings.Contains(errText, "no registered NAS") {
		t.Fatalf("--nas nope: exit %d %s", code, errText)
	}
	out, _, code = runCLIErr(t, "fleet", "sync", "--dry-run")
	if code != 0 || !strings.Contains(out, "acme") || !strings.Contains(out, "lab") {
		t.Fatalf("human plan: exit %d %s", code, out)
	}

	if n := hits.Load(); n != 0 {
		t.Fatalf("a dry run sent %d request(s) to the NAS", n)
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(filepath.Base(p), "fleet.db") {
			t.Errorf("a dry run created the mirror: %s", p)
		}
		return nil
	})
}
