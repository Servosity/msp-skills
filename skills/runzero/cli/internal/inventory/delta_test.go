// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"testing"
	"time"
)

// seedRun2Delta returns run-1 payloads plus one new exposed service on the
// critical asset, one new vulnerability on the high asset, and certificates.
func seedRun2Delta() map[string]string {
	m := seedRun1()
	m["/export/org/services.json"] = `[
		{"id":"s1","asset_id":"a1","address":"10.1.0.5","transport":"tcp","port":443,"protocol":"https","product":"nginx"},
		{"id":"s2","asset_id":"a1","address":"10.1.0.5","transport":"tcp","port":22,"protocol":"ssh"},
		{"id":"s3","asset_id":"a2","address":"10.1.0.6","transport":"tcp","port":445,"protocol":"smb2"},
		{"id":"s4","asset_id":"a1","address":"10.1.0.5","transport":"tcp","port":3389,"protocol":"rdp"},
		{"id":"s5","asset_id":"a3","address":"192.168.9.20","transport":"tcp","port":9100,"protocol":"jetdirect"}
	]`
	m["/export/org/vulnerabilities.json"] = `[
		{"id":"v1","asset_id":"a1","name":"XZ backdoor","cve":"CVE-2024-3094","severity":"critical","cvss":10.0},
		{"id":"v2","asset_id":"a2","name":"SMB issue","cve":"CVE-2020-0796","severity":"high","cvss":8.1},
		{"id":"v3","asset_id":"a2","name":"NewVuln","cve":"CVE-2026-0001","severity":"critical","cvss":9.9}
	]`
	return m
}

func TestExposureDelta(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// Fewer than two runs: empty result, no error.
	res, err := ExposureDelta(ctx, db, 0)
	if err != nil {
		t.Fatalf("ExposureDelta on empty store: %v", err)
	}
	if len(res.NewServices) != 0 || len(res.NewVulns) != 0 {
		t.Fatalf("expected empty delta on empty store, got %+v", res)
	}

	g := &fakeGetter{payloads: seedRun1()}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatalf("sync run 1: %v", err)
	}

	// One run only: still empty (needs a baseline).
	res, err = ExposureDelta(ctx, db, 0)
	if err != nil {
		t.Fatalf("ExposureDelta after one run: %v", err)
	}
	if res.SyncRunCount != 1 || len(res.NewServices) != 0 || len(res.NewVulns) != 0 {
		t.Fatalf("expected empty delta after a single run, got %+v", res)
	}

	g.payloads = seedRun2Delta()
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatalf("sync run 2: %v", err)
	}

	res, err = ExposureDelta(ctx, db, 0)
	if err != nil {
		t.Fatalf("ExposureDelta: %v", err)
	}
	if res.SyncRunCount != 2 {
		t.Fatalf("sync_run_count = %d, want 2", res.SyncRunCount)
	}
	if len(res.StaleEntities) != 0 {
		t.Fatalf("full sync should report no stale entities, got %v", res.StaleEntities)
	}
	// Exactly the two new services (s4 on a1, s5 on a3), criticality-ranked:
	// the critical asset's RDP service must come first.
	if len(res.NewServices) != 2 {
		t.Fatalf("new services = %d, want 2 (%+v)", len(res.NewServices), res.NewServices)
	}
	if res.NewServices[0].AssetID != "a1" || res.NewServices[0].Port != 3389 {
		t.Fatalf("first new service should be a1:3389 (critical asset), got %+v", res.NewServices[0])
	}
	if res.NewServices[1].AssetID != "a3" || res.NewServices[1].Port != 9100 {
		t.Fatalf("second new service should be a3:9100, got %+v", res.NewServices[1])
	}
	// Exactly the one new vulnerability; v1/v2 pre-existed.
	if len(res.NewVulns) != 1 {
		t.Fatalf("new vulns = %d, want 1 (%+v)", len(res.NewVulns), res.NewVulns)
	}
	if res.NewVulns[0].CVE != "CVE-2026-0001" || res.NewVulns[0].AssetID != "a2" {
		t.Fatalf("new vuln should be CVE-2026-0001 on a2, got %+v", res.NewVulns[0])
	}

	// A --since window with no run at/before the cutoff falls back to the
	// prior run (no-rows branch).
	winRes, err := ExposureDelta(ctx, db, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("ExposureDelta wide window: %v", err)
	}
	if winRes.BaselineRun >= winRes.LatestRun {
		t.Fatalf("wide --since window degenerated to baseline %d >= latest %d", winRes.BaselineRun, winRes.LatestRun)
	}
	if len(winRes.NewServices) != 2 || len(winRes.NewVulns) != 1 {
		t.Fatalf("wide window should report the same delta, got svc=%d vuln=%d", len(winRes.NewServices), len(winRes.NewVulns))
	}
	// A --since window NEWER than the most recent sync resolves to the latest
	// run itself; the guard must fall back to the prior run, not compare the
	// latest run against itself (silently empty).
	tinyRes, err := ExposureDelta(ctx, db, time.Millisecond)
	if err != nil {
		t.Fatalf("ExposureDelta tiny window: %v", err)
	}
	if tinyRes.BaselineRun >= tinyRes.LatestRun {
		t.Fatalf("tiny --since window degenerated to baseline %d >= latest %d", tinyRes.BaselineRun, tinyRes.LatestRun)
	}
	if len(tinyRes.NewServices) != 2 || len(tinyRes.NewVulns) != 1 {
		t.Fatalf("tiny window should fall back to the prior run's delta, got svc=%d vuln=%d", len(tinyRes.NewServices), len(tinyRes.NewVulns))
	}

	// Partial sync (--only assets): services/vulns keep last_run < latest;
	// the result must surface them as stale instead of silently empty.
	g.payloads = seedRun2Delta()
	if _, err := SyncAll(ctx, g, db, []string{"assets"}, ""); err != nil {
		t.Fatalf("sync run 3 (--only assets): %v", err)
	}
	staleRes, err := ExposureDelta(ctx, db, 0)
	if err != nil {
		t.Fatalf("ExposureDelta after partial sync: %v", err)
	}
	wantStale := map[string]bool{"services": true, "vulnerabilities": true}
	if len(staleRes.StaleEntities) != 2 || !wantStale[staleRes.StaleEntities[0]] || !wantStale[staleRes.StaleEntities[1]] {
		t.Fatalf("partial sync should flag services+vulnerabilities stale, got %v", staleRes.StaleEntities)
	}
}

func TestCertsExpiring(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	soon := time.Now().Add(10 * 24 * time.Hour).Unix()
	far := time.Now().Add(365 * 24 * time.Hour).Unix()
	expired := time.Now().Add(-5 * 24 * time.Hour).Unix()

	payloads := seedRun1()
	payloads["/export/org/certificates.json"] = `[
		{"id":"c1","asset_id":"a1","subject":"CN=web01","issuer":"CN=R3","not_after":` + itoa(soon) + `},
		{"id":"c2","asset_id":"a2","subject":"CN=db01","issuer":"CN=db01","not_after":` + itoa(far) + `,"self_signed":true},
		{"id":"c3","asset_id":"a3","subject":"CN=printer1","issuer":"CN=old-ca","not_after":` + itoa(expired) + `,"data":"x","signature_algorithm":"SHA1withRSA"}
	]`
	g := &fakeGetter{payloads: payloads}
	if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// 30-day window: c1 (expiring soon) + c3 (already expired); c2 excluded.
	rows, err := CertsExpiring(ctx, db, 30, false)
	if err != nil {
		t.Fatalf("CertsExpiring: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expiring rows = %d, want 2 (%+v)", len(rows), rows)
	}
	// Criticality ordering: critical a1 first, then low a3.
	if rows[0].CertID != "c1" || rows[0].AssetID != "a1" {
		t.Fatalf("first expiring cert should be c1 on critical a1, got %+v", rows[0])
	}
	if rows[1].CertID != "c3" || !rows[1].Expired {
		t.Fatalf("second row should be the expired c3, got %+v", rows[1])
	}
	if rows[0].DaysLeft < 8 || rows[0].DaysLeft > 10 {
		t.Fatalf("c1 days_left = %d, want ~9-10", rows[0].DaysLeft)
	}
	// Expired certs floor toward negative: 5 days past expiry reports -5,
	// so days_left < 0 is a reliable "already expired" signal even <24h out.
	if rows[1].DaysLeft != -5 {
		t.Fatalf("expired c3 days_left = %d, want -5", rows[1].DaysLeft)
	}

	// Weak-only: c2 (self-signed) + c3 (SHA-1 signature in raw data).
	weak, err := CertsExpiring(ctx, db, 30, true)
	if err != nil {
		t.Fatalf("CertsExpiring weak: %v", err)
	}
	if len(weak) != 2 {
		t.Fatalf("weak rows = %d, want 2 (%+v)", len(weak), weak)
	}
	ids := map[string]bool{}
	for _, r := range weak {
		ids[r.CertID] = true
	}
	if !ids["c2"] || !ids["c3"] {
		t.Fatalf("weak set should be {c2,c3}, got %+v", ids)
	}
}

func TestWeakSignature(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"empty", "", false},
		{"sha256", `{"signature_algorithm":"SHA256withRSA"}`, false},
		{"sha1", `{"signature_algorithm":"SHA1withRSA"}`, true},
		{"md5", `{"signature_algorithm":"MD5withRSA"}`, true},
		{"ecdsa-sha1", `{"sig":"ecdsa-with-SHA1"}`, true},
		{"bare-sha1-field", `{"alg":"sha1"}`, true},
		// SHA-1 *fingerprint* fields are digests of the cert, not its
		// signature algorithm — must NOT flag weak.
		{"fingerprint-sha1", `{"fingerprint_sha1":"ab:cd:ef:12:34","signature_algorithm":"SHA256withRSA"}`, false},
		{"bare-fingerprint-key", `{"sha1":"abcdef1234"}`, false},
		{"hash-key", `{"hash_algorithm":"sha1"}`, false},
		{"nested-sig", `{"details":{"signature_algorithm":"SHA1withRSA"}}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := weakSignature(c.data); got != c.want {
				t.Fatalf("weakSignature(%q) = %v, want %v", c.data, got, c.want)
			}
		})
	}
}
