// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"slices"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/credkit"
)

// recordHolds swaps Hold for a recorder for the rest of the test.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var held []string
	was := Hold
	Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { Hold = was })
	return &held
}

// tenant-secrets-shown-never-stored: a value a tenant-secret response carried
// is registered and never held by the credential kit, so stdout and MCP
// results show it to the caller who asked, while a file line and an MCP
// child's argv that repeat it are withheld. In an MCP child, whose stdout is
// the server's input, it is held, so the kit withholds it everywhere there.
func TestTenantSecretShownToTheCallerWithheldElsewhere(t *testing.T) {
	held := recordHolds(t)
	t.Setenv(MCPChildEnvVar, "")
	const laps, varValue = "unitLapsPassw0rd-8c1e5a", "unitSecureVarValue-3d9b7f"
	ShowTenantSecrets([]byte(`{"accountName":"localadmin","password":"` + laps + `"}`))
	ShowTenantSecrets([]byte(`[{"name":"DomainJoin","value":"`+varValue+`"}]`), "value")
	if len(*held) != 0 {
		t.Errorf("a tenant secret was held, so the caller could not see it: %q", *held)
	}
	out := string(WithholdTenantSecrets([]byte("id,password\n1," + laps + "\nkept line\n")))
	if strings.Contains(out, laps) || !strings.Contains(out, "kept line") {
		t.Errorf("want the line with the tenant secret withheld and the rest kept:\n%s", out)
	}
	if !TenantSecretEchoes("log: rotated to "+varValue) || TenantSecretEchoes("log: nothing secret") {
		t.Error("a secure variable value was not recognized inside text, or plain text was")
	}
	if !ArgvRepeatsSecret("teach\n--notes\n" + laps) {
		t.Error("a child argv repeating the LAPS password was not refused")
	}
	t.Setenv(MCPChildEnvVar, "1")
	ShowTenantSecrets([]byte(`{"password":"unitChildLaps-4f2a"}`))
	if !slices.Contains(*held, "unitChildLaps-4f2a") {
		t.Error("an MCP child did not hold the tenant secret it was answered")
	}
}

// A secret no single line repeats (one that spans lines) is withheld whole.
func TestTenantSecretSplitAcrossLinesWithheld(t *testing.T) {
	recordHolds(t)
	ShowTenantSecrets([]byte(`{"password":"unitSplit\nSecret-77c1"}`))
	if out := string(WithholdTenantSecrets([]byte("a unitSplit\nSecret-77c1 b\n"))); out != TenantWithheldNotice+"\n" {
		t.Errorf("a tenant secret spanning lines was kept:\n%s", out)
	}
}
