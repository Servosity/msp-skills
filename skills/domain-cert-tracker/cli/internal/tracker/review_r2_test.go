// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// renewal-age/newest-successor-reset: successor B was first seen three days
// before the probe and never deployed; a newer successor C seen today must
// not reset the grace and hide B.
func TestR2RenewalsOldestUndeployedSuccessorCounts(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	day := 24 * time.Hour
	probe := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	a := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	b := ctCert(2, le, "2026-10-24T00:00:00Z", "2027-01-22T00:00:00Z", "bb", "www.example.com")
	c := ctCert(3, le, "2026-10-27T00:00:00Z", "2027-01-25T00:00:00Z", "cc", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 3, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": {a, b}}}
	src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}}
	for _, at := range []time.Time{probe.Add(-4 * day), probe.Add(-3 * day)} {
		at := at
		opt.Now = func() time.Time { return at }
		if _, err := RunSweep(ctx, st, src, opt); err != nil {
			t.Fatal(err)
		}
	}
	ct.certs["example.com"] = []CTCert{a, b, c}
	opt.Now = func() time.Time { return probe }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, probe)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalDeployStalled {
		t.Fatalf("successor seen 3 days before the probe hidden by a newer one: %s (%s)", got[0].Status, got[0].Detail)
	}
}

// certificate-identity/same-organization-issuers: two Let's Encrypt
// intermediates (R10, R11) with the same serial and validity are two
// certificates; only R11's is in CT.
func TestR2CertKeyDistinguishesIntermediates(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	sw, _ := st.BeginSweep(ctx, "")
	tlsData, _ := json.Marshal(TLSObs{Host: "www.example.com", Port: 443, Serial: "01", Issuer: "CN=R10,O=Let's Encrypt,C=US",
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2027-01-01T00:00:00Z", DaysLeft: 65, Trusted: true, HostnameMatch: true})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindTLS, Subject: "www.example.com:443", OK: true, Data: tlsData})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "example.com", OK: true, Data: []byte(`{"certs":[],"caa_by_name":{}}`)})
	_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{{Domain: "example.com", CrtID: 7, IssuerName: "C=US, O=Let's Encrypt, CN=R11",
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2027-01-01T00:00:00Z", Serial: "01", Names: []string{"www.example.com"}}})
	_ = st.FinishSweep(ctx, sw, "ok")
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ServedInCT == nil || *got[0].ServedInCT {
		t.Fatalf("R10 certificate matched to R11's CT entry: %+v", got[0])
	}
}

// caa-policy/incomplete-certificate-names: crt.sh's default ILIKE match
// (certwatch_db fnc/web_apis.fnc) builds name_value from the identities that
// contain the query only, so a certificate found for shop.example.com that
// also names *.example.com shows shop.example.com alone. The audit judged
// only the names it saw and called the certificate routine.
func TestR2IssuerAuditNamesFromCrtShAreNotTheFullSANList(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "shop.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	body := `[{"issuer_ca_id":295815,"issuer_name":"C=US, O=Let's Encrypt, CN=R11","common_name":"shop.example.com","name_value":"shop.example.com","id":42,"not_before":"2026-10-20T00:00:00","not_after":"2027-01-18T00:00:00","serial_number":"0abc","result_count":2}]`
	certs, err := ParseCrtSh([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	dns := &fakeDNS{raw: map[string][][]byte{
		"shop.example.com|CAA": {caaRData(0, "issue", "letsencrypt.org")},
		"example.com|CAA":      {caaRData(0, "issuewild", "digicert.com")},
	}}
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs}}, DNS: dns, Guard: netguard.New()}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil {
		t.Fatal(err)
	}
	if res.Routine != 0 {
		t.Fatalf("certificate judged on crt.sh's filtered name list counted as routine: %+v routine %d", res.Findings, res.Routine)
	}
}

// Codex publish pass 4 round 1: RFC 8555 section 7.4 lets an order ask for its
// notAfter, and crt.sh returns the real bounds, so a later issuance can expire
// sooner than the certificate it replaces. Still valid, it is a successor.
func TestRenewalsShorterLivedReplacementIsASuccessor(t *testing.T) {
	ctx := context.Background()
	day := 24 * time.Hour
	le := "C=US, O=Let's Encrypt, CN=R11"
	old := ctCert(1, le, "2026-10-01T00:00:00Z", "2026-12-30T00:00:00Z", "aa", "www.example.com")
	short := ctCert(2, le, "2026-11-01T00:00:00Z", "2026-11-07T00:00:00Z", "bb", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2026-12-30T00:00:00Z", DaysLeft: 56, Trusted: true, HostnameMatch: true}
	sweep := func(st *store.Store, seen, probe time.Time) {
		t.Helper()
		ct := &fakeCT{certs: map[string][]CTCert{"example.com": {old, short}}}
		src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, Guard: netguard.New()}
		opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return seen }}
		if _, err := RunSweep(ctx, st, src, opt); err != nil {
			t.Fatal(err)
		}
		opt.Kinds, opt.Now = map[string]bool{KindTLS: true}, func() time.Time { return probe }
		if _, err := RunSweep(ctx, st, src, opt); err != nil {
			t.Fatal(err)
		}
	}
	newStore := func() *store.Store {
		st := openTestStore(t)
		if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
			t.Fatal(err)
		}
		return st
	}
	seen := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	st := newStore()
	probe := seen.Add(3 * day)
	sweep(st, seen, probe)
	got, err := Renewals(ctx, st, "", 14, probe)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalDeployStalled {
		t.Errorf("a still-valid shorter-lived replacement was not judged: %s (%s)", got[0].Status, got[0].Detail)
	}

	// Once it has expired it can no longer be deployed: the older certificate
	// still being served is not a stalled deployment.
	st = newStore()
	probe = seen.Add(10 * day)
	sweep(st, seen, probe)
	got, err = Renewals(ctx, st, "", 14, probe)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status == RenewalDeployStalled {
		t.Errorf("an expired replacement called a stalled deployment: %s", got[0].Detail)
	}
}

// Adversarial publish pass 4 round 2: a shorter-lived certificate within its
// deploy grace is no evidence a host past its renewal point was renewed.
func TestRenewalsShorterLivedDoesNotHideIssuanceStalled(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	old := ctCert(1, le, "2026-08-12T00:00:00Z", "2026-11-10T00:00:00Z", "aa", "www.example.com")
	short := ctCert(2, le, "2026-11-01T06:00:00Z", "2026-11-07T06:00:00Z", "bb", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-12T00:00:00Z", NotAfter: "2026-11-10T00:00:00Z", DaysLeft: 8, Trusted: true, HostnameMatch: true}
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"example.com": {old, short}}}, TLS: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, Guard: netguard.New()}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Now: func() time.Time { return now }}
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalIssuanceStalled {
		t.Errorf("past the renewal point with only a shorter-lived certificate: %s (%s)", got[0].Status, got[0].Detail)
	}
}
