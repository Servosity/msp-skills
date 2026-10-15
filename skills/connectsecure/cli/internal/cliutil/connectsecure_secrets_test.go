// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

func TestConnectsecureSecretTable(t *testing.T) {
	for path, want := range map[string]bool{
		"/r/company/credentials":                 true,
		"/r/company/credentials/7":               true,
		"/r/integration/integration_credentials": true,
		"/r/company/get_uninstall_secret":        true,
		"/r/company/agent_credentials_mapping":   false,
		"/r/company/credentialsx":                false,
		"/r/company/scheduler":                   true,
		"/r/company/scheduler/3":                 true,
		"/r/company/attack_surface_results":      true,
		"/r/integration/integration_rules":       true,
		"/r/company/custom_domains":              true,
	} {
		if got := ConnectsecureSecretPath(path); got != want {
			t.Errorf("ConnectsecureSecretPath(%q) = %v, want %v", path, got, want)
		}
	}
	if !ConnectsecureSecretResource("credentials") || ConnectsecureSecretResource("agent_credentials_mapping") || ConnectsecureSecretResource("") {
		t.Fatal("resource check wrong")
	}
	if len(ConnectsecureSecretResources()) != 6 || !ConnectsecureSecretResource("scheduler") || !ConnectsecureSecretResource("attack_surface_results") || !ConnectsecureSecretResource("integration_rules") || !ConnectsecureSecretResource("custom_domains") {
		t.Fatalf("want 6 store resources incl. scheduler, attack_surface_results, integration_rules, custom_domains, got %v", ConnectsecureSecretResources())
	}
}
