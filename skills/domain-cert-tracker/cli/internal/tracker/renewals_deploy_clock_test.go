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

// The deploy clock runs from the earliest-sighted successor, whatever order
// crt.sh lists the certificates in, and the finding names both the newest
// successor and the one whose grace ran out.
func TestRenewalsDeployClockNamesOverdueAndNewestSuccessor(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	day := 24 * time.Hour
	probe := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	a := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	// The newer certificate carries the higher crt.sh id here, so neither
	// list order nor id order may pick the clock.
	b := ctCert(9, le, "2026-10-24T00:00:00Z", "2027-01-22T00:00:00Z", "bb", "www.example.com")
	c := ctCert(5, le, "2026-10-27T00:00:00Z", "2027-01-25T00:00:00Z", "cc", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 3, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": {a, b}}}
	src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Now: func() time.Time { return probe.Add(-4 * day) }}
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	ct.certs["example.com"] = []CTCert{c, b, a}
	opt.Now = func() time.Time { return probe }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, probe)
	if err != nil {
		t.Fatal(err)
	}
	f := got[0]
	if f.Status != RenewalDeployStalled {
		t.Fatalf("status %s (%s), want deploy_stalled", f.Status, f.Detail)
	}
	// Checked through the JSON the renewals command and MCP tool print.
	raw, _ := json.Marshal(f)
	var out struct {
		Successor        map[string]any `json:"successor"`
		OverdueSuccessor map[string]any `json:"overdue_successor"`
		OverdueSince     string         `json:"overdue_since"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Successor["crt_id"] != float64(5) {
		t.Fatalf("successor is not the newest certificate: %s", raw)
	}
	if out.OverdueSuccessor["crt_id"] != float64(9) || out.OverdueSuccessor["first_seen_in_ct"] != "2026-10-24T12:00:00Z" {
		t.Fatalf("overdue_successor is not the earliest-sighted one: %s", raw)
	}
	if out.OverdueSince != "2026-10-25T12:00:00Z" {
		t.Fatalf("overdue_since %q, want first sighting plus the one-day grace", out.OverdueSince)
	}
}

// A successor whose first sighting this tool cannot date may be the one
// that has gone undeployed longest: the host is unverified, never ok, even
// when another successor was seen within the grace.
func TestRenewalsUndatedSuccessorIsNotOK(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	stamp := now.Format(time.RFC3339Nano)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	// Sweep 1 stores certificate 2 but records no CT read for example.com,
	// so its first sighting has no time.
	sw1, _ := st.BeginSweep(ctx, "")
	_, _ = st.AddCTCerts(ctx, sw1, []store.CTCertRow{{Domain: "example.com", CrtID: 2, IssuerName: le,
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2026-12-30T00:00:00Z", Serial: "bb", Names: []string{"www.example.com"}}})
	_ = st.FinishSweep(ctx, sw1, "ok")
	// Sweep 2 probes the host and first sees certificate 3, just now.
	sw2, _ := st.BeginSweep(ctx, "")
	tlsData, _ := json.Marshal(TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US",
		NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 3, Trusted: true, HostnameMatch: true})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw2, Kind: KindTLS, Subject: "www.example.com:443", OK: true, Data: tlsData, ObservedAt: stamp})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw2, Kind: KindCT, Subject: "example.com", OK: true, Data: []byte(`{"certs":[],"caa_by_name":{}}`), ObservedAt: stamp})
	_, _ = st.AddCTCerts(ctx, sw2, []store.CTCertRow{{Domain: "example.com", CrtID: 3, IssuerName: le,
		NotBefore: "2026-10-27T00:00:00Z", NotAfter: "2027-01-25T00:00:00Z", Serial: "cc", Names: []string{"www.example.com"}}})
	_ = st.FinishSweep(ctx, sw2, "ok")
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalCTUnverified {
		t.Fatalf("successor with no dated sighting judged %s (%s), want ct_unverified", got[0].Status, got[0].Detail)
	}
}

// A successor first seen after the host's last probe has not been checked
// against what the host serves: the host is unverified, never "within a day
// of the last probe".
func TestRenewalsSuccessorSeenAfterLastProbeIsNotOK(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	day := 24 * time.Hour
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	a := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	b := ctCert(2, le, "2026-10-24T00:00:00Z", "2027-01-22T00:00:00Z", "bb", "www.example.com")
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 7, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": {a}}}
	src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": served}}, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Now: func() time.Time { return now.Add(-5 * day) }}
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	// CT-only sweeps from here on: the host is never probed again.
	ct.certs["example.com"] = []CTCert{a, b}
	opt.Kinds = map[string]bool{KindCT: true}
	opt.Now = func() time.Time { return now.Add(-3 * day) }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalTLSUnverified {
		t.Fatalf("successor never checked against a probe judged %s (%s), want tls_unverified", got[0].Status, got[0].Detail)
	}
	// The next probe still shows the old certificate, three days after the
	// sighting: a stalled deploy.
	opt.Kinds = map[string]bool{KindCT: true, KindTLS: true}
	opt.Now = func() time.Time { return now }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	if got, err = Renewals(ctx, st, "", 14, now); err != nil {
		t.Fatal(err)
	}
	if got[0].Status != RenewalDeployStalled {
		t.Fatalf("probe three days after the sighting: %s, want deploy_stalled", got[0].Status)
	}
}
