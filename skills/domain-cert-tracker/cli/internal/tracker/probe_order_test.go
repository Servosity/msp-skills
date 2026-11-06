// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written: Codex publish pass 2 round 2, renewal-probe-order. Whether a
// probe saw the world after a CT sighting, which probe is the latest, and
// when a certificate was first sighted are decided by the times the checks
// ran, never by sweep ids: two sweeps can overlap, and within one sweep the
// checks finish in any order.

package tracker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// fixtureCert is the row of testdata/crtsh-example.com.json with id.
func fixtureCert(t *testing.T, id int64) CTCert {
	t.Helper()
	certs, err := ParseCrtSh(readFixture(t, "crtsh-example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range certs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("crt.sh fixture has no certificate %d", id)
	return CTCert{}
}

// The reviewer's input: inventory example.com and emudhra-lab.example.com,
// then one sweep --only ct,tls. TLS finishes at 11:59:50 serving
// 30003024773; CT finishes at 12:00:10 and first returns its successor
// 30003111654 (both ordinary crt.sh rows, testdata/crtsh-example.com.json,
// certwatch_db fnc/web_apis.fnc). No probe ran after the sighting, so the
// host is tls_unverified, not ok, although both share one sweep id.
func TestRenewalsProbeBeforeSightingInSameSweepIsUnverified(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "emudhra-lab.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	certs, err := ParseCrtSh(readFixture(t, "crtsh-example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	old := fixtureCert(t, 30003024773)
	_ = fixtureCert(t, 30003111654)
	served := TLSObs{Host: "emudhra-lab.example.com", Port: 443, Serial: old.Serial, Issuer: "CN=EM DV TLS CA - G3B,O=eMudhra BV,C=NL",
		NotBefore: old.NotBefore, NotAfter: old.NotAfter, DaysLeft: 186, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": certs}}

	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	tlsAt := day.Add(11*time.Hour + 59*time.Minute + 50*time.Second)
	ctAt := day.Add(12*time.Hour + 10*time.Second)
	clk := &checkClock{cur: tlsAt.Add(-time.Minute), stamped: make(chan struct{})}
	src := Sources{
		CT:    &clockedCT{fakeCT: ct, clk: clk, at: ctAt},
		TLS:   &clockedTLS{inner: &fakeTLS{obs: map[string]TLSObs{"emudhra-lab.example.com:443": served}}, clk: clk, at: tlsAt},
		Guard: netguard.New(),
	}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Concurrency: 2, Now: clk.now}); err != nil {
		t.Fatal(err)
	}
	got, err := Renewals(ctx, st, "", 14, ctAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Successor == nil || got[0].Successor["crt_id"] != int64(30003111654) {
		t.Fatalf("renewals did not find successor 30003111654: %+v", got)
	}
	if got[0].Status != RenewalTLSUnverified {
		t.Fatalf("status %s (%s); the only probe (%s) ran before the successor was first sighted (%s), want %s",
			got[0].Status, got[0].Detail, tlsAt.Format(time.RFC3339), ctAt.Format(time.RFC3339), RenewalTLSUnverified)
	}
}

// putObs stores one observation with an explicit check time.
func putObs(t *testing.T, st *store.Store, sweep int64, kind, subject string, at time.Time, data any) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObservation(context.Background(), store.Observation{SweepID: sweep, Kind: kind, Subject: subject, OK: true,
		Data: b, ObservedAt: at.UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}
}

func ctRow(c CTCert) store.CTCertRow {
	return store.CTCertRow{Domain: "example.com", CrtID: c.ID, IssuerName: c.IssuerName, CommonName: c.CommonName, Names: c.Names,
		NotBefore: c.NotBefore, NotAfter: c.NotAfter, Serial: c.Serial}
}

// Overlapping sweeps: sweep 3 began after sweep 2 but probed the host
// first. The latest probe is the one that ran last (sweep 2, still serving
// the old certificate four days after its successor was sighted), not the
// one with the higher sweep id (sweep 3, which saw the successor served).
func TestRenewalsLatestProbeIsTheOneThatRanLast(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	a := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	b := ctCert(2, le, "2026-10-24T00:00:00Z", "2027-01-22T00:00:00Z", "bb", "www.example.com")
	serve := func(c CTCert) TLSObs {
		return TLSObs{Host: "www.example.com", Port: 443, Serial: c.Serial, Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: c.NotBefore,
			NotAfter: c.NotAfter, DaysLeft: 30, Trusted: true, HostnameMatch: true}
	}
	sighted := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	s1, _ := st.BeginSweep(ctx, "")
	putObs(t, st, s1, KindCT, "example.com", sighted, map[string]any{})
	if _, err := st.AddCTCerts(ctx, s1, []store.CTCertRow{ctRow(a), ctRow(b)}); err != nil {
		t.Fatal(err)
	}
	s2, _ := st.BeginSweep(ctx, "")
	s3, _ := st.BeginSweep(ctx, "")
	putObs(t, st, s3, KindTLS, "www.example.com:443", time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), serve(b))
	last := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	putObs(t, st, s2, KindTLS, "www.example.com:443", last, serve(a))

	got, err := Renewals(ctx, st, "", 14, last.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != RenewalDeployStalled || got[0].ServedSerial != "aa" {
		t.Fatalf("renewals judged the probe with the higher sweep id, not the one that ran last: %+v", got)
	}
}

// Overlapping sweeps: sweep 1 stored its CT result first, but sweep 2's CT
// check ran an hour earlier and returned the same successor. The deploy
// grace starts at the earliest sighting, so a probe 24.5 hours after it that
// still serves the old certificate is deploy_stalled.
func TestRenewalsFirstSightingIsTheEarliestCTCheck(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	a := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "www.example.com")
	b := ctCert(2, le, "2026-10-20T00:00:00Z", "2027-01-18T00:00:00Z", "bb", "www.example.com")
	s1, _ := st.BeginSweep(ctx, "")
	s2, _ := st.BeginSweep(ctx, "")
	putObs(t, st, s1, KindCT, "example.com", time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), map[string]any{})
	if _, err := st.AddCTCerts(ctx, s1, []store.CTCertRow{ctRow(a), ctRow(b)}); err != nil {
		t.Fatal(err)
	}
	earliest := time.Date(2026, 10, 24, 11, 0, 0, 0, time.UTC)
	putObs(t, st, s2, KindCT, "example.com", earliest, map[string]any{})
	if _, err := st.AddCTCerts(ctx, s2, []store.CTCertRow{ctRow(a), ctRow(b)}); err != nil {
		t.Fatal(err)
	}
	probe := earliest.Add(24*time.Hour + 30*time.Minute)
	putObs(t, st, s2, KindTLS, "www.example.com:443", probe, TLSObs{Host: "www.example.com", Port: 443, Serial: "aa",
		Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: a.NotBefore, NotAfter: a.NotAfter, DaysLeft: 7, Trusted: true, HostnameMatch: true})

	got, err := Renewals(ctx, st, "", 14, probe.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != RenewalDeployStalled {
		t.Fatalf("the deploy grace did not start at the earliest CT check that returned the successor: %+v", got)
	}
	if got[0].OverdueSince != earliest.Add(24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("overdue_since %q, want the earliest sighting plus the grace", got[0].OverdueSince)
	}
}

// Overlapping sweeps: sweep 1 checked the registrar lock at 10:00 and found
// it unlocked; sweep 2, which began later, checked at 09:00 and found it
// locked. Sweep 3 at 11:00 finds it unlocked. The baseline is the check that
// ran last before 11:00 (unlocked at 10:00), so nothing changed and no
// registrar.unlocked event is raised; the highest lower sweep id (locked at
// 09:00) would report an unlock that did not happen since the last look.
func TestSweepDiffBaselineIsTheCheckThatRanLastBefore(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	unlocked := DomainObs{Domain: "acme.test", Expires: "2027-11-10T00:00:00Z", Statuses: []string{"active"}, Nameservers: []string{}}
	locked := DomainObs{Domain: "acme.test", Expires: "2027-11-10T00:00:00Z", TransferLocked: true, Statuses: []string{"client transfer prohibited"}, Nameservers: []string{}}
	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	s1, _ := st.BeginSweep(ctx, "")
	s2, _ := st.BeginSweep(ctx, "")
	putObs(t, st, s1, KindRDAP, "acme.test", day.Add(10*time.Hour), unlocked)
	putObs(t, st, s2, KindRDAP, "acme.test", day.Add(9*time.Hour), locked)

	at := day.Add(11 * time.Hour)
	src := Sources{RDAP: &fakeRDAP{obs: map[string]DomainObs{"acme.test": unlocked}}, Guard: netguard.New()}
	res, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindRDAP: true}, Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(res.Events); got[EventRegistrarUnlocked] != 0 {
		t.Fatalf("the sweep diffed against the higher sweep id (locked at 09:00), not the check that ran last before it (unlocked at 10:00): %v", got)
	}
}

// Overlapping sweeps: sweep 2 finished at 11:00, sweep 1 at 12:00. The
// report's last sweep is the one that finished last.
func TestLatestSweepIsTheOneThatFinishedLast(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	s1, _ := st.BeginSweep(ctx, "")
	s2, _ := st.BeginSweep(ctx, "")
	for id, at := range map[int64]string{s1: "2026-10-28T12:00:00Z", s2: "2026-10-28T11:00:00.5Z"} {
		if _, err := st.DB().ExecContext(ctx, `UPDATE tracker_sweeps SET finished_at = ?, status = 'ok' WHERE id = ?`, at, id); err != nil {
			t.Fatal(err)
		}
	}
	sw, ok, err := st.LatestSweep(ctx)
	if err != nil || !ok {
		t.Fatalf("LatestSweep: %v %v", ok, err)
	}
	if sw.ID != s1 {
		t.Fatalf("latest sweep %d, want %d (finished at 12:00, after sweep %d at 11:00)", sw.ID, s1, s2)
	}
}
