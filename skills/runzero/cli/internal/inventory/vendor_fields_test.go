// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"strconv"
	"testing"
)

// Class inventory-vendor-row-fields (runzero handfix): rows shaped as runZero's
// own OpenAPI documents its exports (Service, Software, Vulnerability,
// Certificate, Asset). A service or vulnerability row embeds its asset, so its
// `id` is the ASSET's id and its own key is <prefix>_id; the asset link is
// <prefix>_asset_id; the finding's CVE, severity and score, the software's
// product and version, a certificate's expiry (validity_end) and an asset's
// end-of-life date (eol_os) are prefixed or renamed too. Reading the generic
// names collapsed every service of an asset into one row, linked no service,
// software or finding to its asset, and left `affected`, `triage`, `software
// rollup` and `certs-expiring` empty or wrong on real data.
func TestVendorShapedExportRows(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	soon := strconv.FormatInt(clock().Add(10*24*60*60*1e9).Unix(), 10)
	g := &fakeGetter{payloads: map[string]string{
		"/export/org/assets.json": `[{"id":"asset-1","names":["web01"],"addresses":["10.0.0.5"],"os":"Ubuntu",` +
			`"eol_os":1600000000,"risk":"critical","alive":true}]`,
		"/export/org/services.json": `[` +
			`{"id":"asset-1","names":["web01"],"risk":"critical","service_id":"svc-22","service_asset_id":"asset-1",` +
			`"service_address":"10.0.0.5","service_transport":"tcp","service_port":22,"service_protocol":"ssh"},` +
			`{"id":"asset-1","names":["web01"],"risk":"critical","service_id":"svc-443","service_asset_id":"asset-1",` +
			`"service_address":"10.0.0.5","service_transport":"tcp","service_port":443,"service_protocol":"tls"}]`,
		"/export/org/software.json": `[{"id":"sw-row-1","software_id":"sw-1","software_asset_id":"asset-1",` +
			`"software_vendor":"OpenSSL Project","software_product":"OpenSSL","software_version":"1.1.1"}]`,
		"/export/org/vulnerabilities.json": `[{"id":"asset-1","names":["web01"],"risk":"low","vulnerability_id":"v-1",` +
			`"vulnerability_asset_id":"asset-1","vulnerability_name":"OpenSSH regreSSHion","vulnerability_cve":"CVE-2024-6387",` +
			`"vulnerability_severity":"Critical","vulnerability_cvss3_base_score":8.1}]`,
		"/export/org/certificates.json": `[{"id":"cert-1","organization_id":"org-1","subject":"CN=web01","cn":"web01",` +
			`"issuer":"CN=R3","validity_end":` + soon + `}]`,
	}}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var services int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inv_services WHERE asset_id = 'asset-1'`).Scan(&services); err != nil {
		t.Fatal(err)
	}
	if services != 2 {
		t.Errorf("services linked to asset-1 = %d, want 2 (one row per service_id)", services)
	}
	var eol int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(os_eol,0) FROM inv_assets WHERE id = 'asset-1'`).Scan(&eol); err != nil || eol != 1600000000 {
		t.Errorf("asset os_eol = %d (err %v), want eol_os 1600000000", eol, err)
	}

	affected, err := Affected(ctx, db, "CVE-2024-6387")
	if err != nil {
		t.Fatal(err)
	}
	if len(affected) != 1 || affected[0].AssetID != "asset-1" || affected[0].Severity != "Critical" {
		t.Errorf("affected CVE-2024-6387 = %+v, want asset-1 with the finding's own severity Critical", affected)
	}

	triage, err := Triage(ctx, db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(triage) != 1 || triage[0].ServiceCount != 2 || triage[0].VulnCount != 1 || triage[0].TopCVE != "CVE-2024-6387" {
		t.Errorf("triage = %+v, want asset-1 with 2 services, 1 finding, top CVE-2024-6387", triage)
	}

	rollup, err := SoftwareRollup(ctx, db, "openssl")
	if err != nil {
		t.Fatal(err)
	}
	if len(rollup) != 1 || rollup[0].Product != "OpenSSL" || rollup[0].Version != "1.1.1" || rollup[0].AssetCount != 1 {
		t.Errorf("software rollup = %+v, want OpenSSL 1.1.1 on 1 asset", rollup)
	}

	certs, err := CertsExpiring(ctx, db, 30, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) != 1 || certs[0].CertID != "cert-1" || certs[0].DaysLeft != 10 {
		t.Errorf("certs expiring = %+v, want cert-1 with 10 days left (validity_end)", certs)
	}
}
