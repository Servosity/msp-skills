// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"sonicwall-pp-cli/internal/cliutil/testenv"
)

// Codex r2 (2026-11-02): a registry whose only products are non-firewalls compares
// no firewall at all and must not read as "every registered firewall is visible".
func TestBlindSpotsNoComparedFirewallIsNotAllClear(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	srv := newR1Mock(t, map[string]string{
		"/api/users/productlist-customer": `{"operationStatus":"Success","content":{"totalProducts":"1","filterResultCount":"1","prodList":[{"serNo":"AP1","productName":"SonicWave 231c"}]}}`,
		"/devices/tenant":                 `[]`,
	}, nil)
	out, err := runCLI(t, srv, "blind-spots")
	if err == nil || ExitCode(err) != 3 || !strings.Contains(out+err.Error(), "nothing was compared") {
		t.Fatalf("no firewall compared: want exit 3 'nothing was compared', got err=%v out=%s", err, out)
	}
}

// The same registry with --all-products does compare the access point, so it is
// listed as a blind spot instead of refused (the guard counts what is compared).
func TestBlindSpotsAllProductsComparesNonFirewall(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	srv := newR1Mock(t, map[string]string{
		"/api/users/productlist-customer": `{"operationStatus":"Success","content":{"totalProducts":"1","filterResultCount":"1","prodList":[{"serNo":"AP1","productName":"SonicWave 231c"}]}}`,
		"/devices/tenant":                 `[]`,
	}, nil)
	out, err := runCLI(t, srv, "blind-spots", "--all-products")
	if err != nil || !strings.Contains(out, "AP1") {
		t.Fatalf("--all-products: want AP1 listed with exit 0, got err=%v out=%s", err, out)
	}
}
