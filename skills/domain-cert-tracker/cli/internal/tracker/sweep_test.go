// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

type fakeRDAP struct{ obs map[string]DomainObs }

func (f *fakeRDAP) Lookup(ctx context.Context, d string) (DomainObs, error) {
	o, ok := f.obs[d]
	if !ok {
		return DomainObs{}, errors.New("rdap: HTTP 503")
	}
	return o, nil
}

// fakeCT answers crt.sh searches from certs. Certificate serves der[id];
// with autoDER it serves a certificate generated from the crt.sh row
// itself (its names taken as the full SAN list). Anything else is a 404.
type fakeCT struct {
	certs   map[string][]CTCert
	fail    bool
	der     map[int64][]byte
	autoDER bool
	mu      sync.Mutex
	fetched []int64
}

func (f *fakeCT) Search(ctx context.Context, d string) ([]CTCert, error) {
	if f.fail {
		return nil, &HTTPError{URL: "https://crt.sh/", Status: 502}
	}
	return append([]CTCert(nil), f.certs[d]...), nil
}

func (f *fakeCT) Certificate(ctx context.Context, id int64) ([]byte, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, id)
	f.mu.Unlock()
	if b, ok := f.der[id]; ok {
		return b, nil
	}
	if f.autoDER {
		for _, certs := range f.certs {
			for _, c := range certs {
				if c.ID == id {
					return mustCertDER(c, c.Names...), nil
				}
			}
		}
	}
	return nil, &HTTPError{URL: fmt.Sprintf("https://crt.sh/?d=%d", id), Status: 404}
}

func (f *fakeCT) downloads() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.fetched...)
}

type fakeTLS struct {
	mu    sync.Mutex
	obs   map[string]TLSObs
	calls []string
}

func (f *fakeTLS) Probe(ctx context.Context, host string, port int) (TLSObs, error) {
	key := fmt.Sprintf("%s:%d", host, port)
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	o, ok := f.obs[key]
	if !ok {
		return TLSObs{}, errors.New("connection refused")
	}
	return o, nil
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func ctCert(id int64, issuer, notBefore, notAfter, serial string, names ...string) CTCert {
	return CTCert{ID: id, IssuerName: issuer, NotBefore: notBefore, NotAfter: notAfter, Serial: serial, Names: names, CommonName: names[0]}
}

func eventTypes(refs []EventRef) map[string]int {
	m := map[string]int{}
	for _, r := range refs {
		m[r.Type]++
	}
	return m
}

func TestSweepDiffEventsAndFailureSafety(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if err := st.AddTargets(ctx, "Acme", []store.Target{
		{Kind: "domain", Value: "acme.test", Port: 443},
		{Kind: "host", Value: "www.acme.test", Port: 443},
		{Kind: "host", Value: "legacy.acme.test", Port: 8443},
	}); err != nil {
		t.Fatal(err)
	}
	rdap := &fakeRDAP{obs: map[string]DomainObs{"acme.test": {Domain: "acme.test", Expires: "2026-11-10T00:00:00Z", TransferLocked: true, Statuses: []string{"client transfer prohibited"}, Nameservers: []string{}}}}
	le := "C=US, O=Let's Encrypt, CN=R11"
	ct := &fakeCT{certs: map[string][]CTCert{"acme.test": {ctCert(100, le, "2026-08-01T00:00:00Z", "2026-10-30T00:00:00Z", "aa", "www.acme.test")}}, autoDER: true}
	tlsf := &fakeTLS{obs: map[string]TLSObs{"www.acme.test:443": {Host: "www.acme.test", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-10-30T00:00:00Z", DaysLeft: 1, Trusted: true, HostnameMatch: true}}}
	dns := &fakeDNS{ns: map[string][]string{"acme.test": {"ns1.host.test"}}, txt: map[string][]string{"acme.test": {"v=spf1 -all"}, "_dmarc.acme.test": {"v=DMARC1; p=reject"}},
		raw: map[string][][]byte{"acme.test|CAA": {caaRData(0, "issue", "letsencrypt.org")}}}
	src := Sources{RDAP: rdap, CT: ct, TLS: tlsf, DNS: dns, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Now: clock}

	// Sweep 1: baseline. Expiring domain and certificate raise events; the
	// CT history is a baseline (no ct.new_cert); the 8443 host is refused
	// without being probed.
	r1, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(r1.Events); got[EventDomainExpiring] != 1 || got[EventCertExpiring] != 1 || got[EventCTNewCert] != 0 || got[EventRegistrarUnlocked] != 0 {
		t.Fatalf("sweep 1 events: %v", got)
	}
	if r1.Status != "partial" || len(r1.FetchFailures) != 1 || !r1.FetchFailures[0].Refused || r1.FetchFailures[0].Subject != "legacy.acme.test:8443" {
		t.Fatalf("sweep 1 failures: %s %+v", r1.Status, r1.FetchFailures)
	}
	for _, c := range tlsf.calls {
		if c == "legacy.acme.test:8443" {
			t.Fatal("a stored non-443 port was probed without --allow-port")
		}
	}

	// Sweep 2: same state. Stable event ids mean nothing repeats.
	r2, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Events) != 0 {
		t.Fatalf("re-run sweep repeated events: %+v", r2.Events)
	}

	// Sweep 3: crt.sh fails while a new certificate exists; the registrar
	// lock drops; DNS changes. The CT failure must not advance anything.
	ct.fail = true
	ct.certs["acme.test"] = append(ct.certs["acme.test"],
		ctCert(200, "C=US, O=Some Other CA, CN=X1", "2026-10-27T00:00:00Z", "2027-01-25T00:00:00Z", "bb", "www.acme.test"), // another deployment
		ctCert(201, le, "2026-10-26T00:00:00Z", "2027-01-24T00:00:00Z", "cc", "www.acme.test"))                             // the real renewal
	rdap.obs["acme.test"] = DomainObs{Domain: "acme.test", Expires: "2026-11-10T00:00:00Z", Statuses: []string{"active"}, Nameservers: []string{}}
	dns.ns["acme.test"] = []string{"ns1.other-host.test"}
	r3, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	got := eventTypes(r3.Events)
	if got[EventCTNewCert] != 0 || got[EventRegistrarUnlocked] != 1 || got[EventDNSChanged] != 1 {
		t.Fatalf("sweep 3 events: %v", got)
	}
	if rows, _ := st.ListCTCerts(ctx, "acme.test"); len(rows) != 1 {
		t.Fatalf("a failed CT read changed stored history: %d rows", len(rows))
	}
	rep, err := BuildReport(ctx, st, "", 30, now.AddDate(0, 0, -7), now)
	if err != nil {
		t.Fatal(err)
	}
	foundCT := false
	for _, u := range rep.Clients[0].Unverified {
		if u.Kind == KindCT && u.Subject == "acme.test" {
			foundCT = true
		}
	}
	if !foundCT {
		t.Fatalf("failed CT read not listed as unverified: %+v", rep.Clients[0].Unverified)
	}
	if len(rep.Clients[0].Unlocked) != 1 || len(rep.Clients[0].DNSDrift) != 1 || len(rep.Clients[0].DomainsExpiring) != 1 {
		t.Fatalf("report sections: %+v", rep.Clients[0])
	}

	// Sweep 4: crt.sh recovers; the certificate logged during the outage is
	// reported once.
	ct.fail = false
	r4, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(r4.Events); got[EventCTNewCert] != 2 {
		t.Fatalf("sweep 4 events: %v", got)
	}
	evs, _, err := st.VendorEventsSince(ctx, 0, EventCTNewCert, 10)
	if err != nil || len(evs) != 2 {
		t.Fatalf("ct.new_cert events: %v %v", evs, err)
	}
	var payload map[string]any
	_ = json.Unmarshal(evs[0].Data, &payload)
	if payload["crt_id"].(float64) != 200 || evs[0].Client != "Acme" {
		t.Fatalf("event payload: %v client %q", payload, evs[0].Client)
	}

	// The audit flags the new issuer against CAA; renewals sees the logged
	// successor that the host does not serve yet (within the 1-day grace of
	// the probe that first saw it, it is ok; two days later it is
	// deploy_stalled).
	audit, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Findings) != 1 || audit.Findings[0].Status != "unmapped" {
		t.Fatalf("issuer audit: %+v", audit.Findings)
	}
	findings, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	var www RenewalFinding
	for _, f := range findings {
		if f.Host == "www.acme.test:443" {
			www = f
		}
	}
	if www.Successor == nil || www.Successor["crt_id"].(int64) != 201 {
		t.Fatalf("renewals missed the same-CA successor: %+v", www)
	}
	if len(www.OtherCovering) != 1 || www.OtherCovering[0]["crt_id"].(int64) != 200 {
		t.Fatalf("other-CA certificate not listed as another deployment: %+v", www.OtherCovering)
	}
	if www.Status != RenewalOK {
		t.Fatalf("successor first seen in the sweep that probed the host: status %s, want ok (within grace)", www.Status)
	}
	// Sweep 5, two days on: the host still serves the old certificate.
	later := now.AddDate(0, 0, 2)
	opt.Now = func() time.Time { return later }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	if findings, err = Renewals(ctx, st, "", 14, later); err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Host == "www.acme.test:443" && f.Status != RenewalDeployStalled {
			t.Fatalf("same-CA successor seen two days before the probe: status %s, want deploy_stalled", f.Status)
		}
	}
}

func TestSweepEveryCheckFailedIsAnError(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}})
	src := Sources{RDAP: &fakeRDAP{}, CT: &fakeCT{fail: true}, DNS: &fakeDNS{fail: map[string]bool{"acme.test": true, "_dmarc.acme.test": true}}, Guard: netguard.New()}
	res, err := RunSweep(ctx, st, src, SweepOptions{})
	if !errors.Is(err, ErrAllFailed) || res.Status != "failed" || res.FirstError == "" {
		t.Fatalf("all-failed sweep: %v %s %q", err, res.Status, res.FirstError)
	}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Client: "Nobody"}); !errors.Is(err, ErrUnknownClient) {
		t.Fatalf("unknown client: %v", err)
	}
}

func TestIssuerAuditCAAMatching(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}})
	sw, _ := st.BeginSweep(ctx, "")
	set := []CAA{{Tag: "issue", Value: "letsencrypt.org"}, {Tag: "issuewild", Value: ";"}}
	ctObs, _ := json.Marshal(CTObs{Certs: []CTCert{}, CAA: map[string]NameCAA{"acme.test": {CAA: set, Source: "acme.test"}}})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "acme.test", OK: true, Data: ctObs})
	_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{
		{Domain: "acme.test", CrtID: 1, IssuerName: "C=US, O=Let's Encrypt, CN=R10", NotBefore: "2026-06-01T00:00:00Z", Names: []string{"acme.test"}},
		{Domain: "acme.test", CrtID: 2, IssuerName: "C=US, O=Let's Encrypt, CN=R11", NotBefore: "2026-10-25T00:00:00Z", Names: []string{"acme.test"}},
		{Domain: "acme.test", CrtID: 3, IssuerName: "C=US, O=Google Trust Services, CN=WR1", NotBefore: "2026-10-26T00:00:00Z", Names: []string{"acme.test"}},
		{Domain: "acme.test", CrtID: 4, IssuerName: "C=US, O=Let's Encrypt, CN=R11", NotBefore: "2026-10-27T00:00:00Z", Names: []string{"*.acme.test"}},
	})
	// Each certificate's own SAN list was read (the audit judges nothing else).
	_ = st.PutCertNames(ctx, []store.CertNames{{CrtID: 1, Status: store.CertNamesOK, Names: []string{"acme.test"}}, {CrtID: 2, Status: store.CertNamesOK, Names: []string{"acme.test"}},
		{CrtID: 3, Status: store.CertNamesOK, Names: []string{"acme.test"}}, {CrtID: 4, Status: store.CertNamesOK, Names: []string{"*.acme.test"}}})
	_ = st.FinishSweep(ctx, sw, "ok")
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	status := map[int64]string{}
	for _, f := range res.Findings {
		status[f.CrtID] = f.Status
	}
	if status[3] != "not_in_caa" || status[4] != "not_in_caa" || status[2] != "" || res.Routine != 1 {
		t.Fatalf("audit: %+v routine %d", status, res.Routine)
	}
}

// Codex publish pass 3 round 1: crt.sh's capped result (certwatch_db
// fnc/web_apis.fnc c_resultLimit) fails the CT read. A failed latest read is
// said out loud, whatever earlier reads found, and what they found stays.
func TestDiscoverHostsDisclosesLatestFailedRead(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}, {Kind: "domain", Value: "other.test", Port: 443}})
	first, _ := st.BeginSweep(ctx, "")
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	_ = st.PutObservation(ctx, store.Observation{SweepID: first, Kind: KindCT, Subject: "acme.test", OK: true, Data: []byte("[]"), ObservedAt: base.Format(time.RFC3339Nano)})
	_ = st.PutObservation(ctx, store.Observation{SweepID: first, Kind: KindCT, Subject: "other.test", OK: true, Data: []byte("[]"), ObservedAt: base.Format(time.RFC3339Nano)})
	_, _ = st.AddCTCerts(ctx, first, []store.CTCertRow{{Domain: "acme.test", CrtID: 9, IssuerName: "O=Let's Encrypt", NotAfter: "2027-01-01T00:00:00Z", Names: []string{"shop.acme.test"}}})
	second, _ := st.BeginSweep(ctx, "")
	_ = st.PutObservation(ctx, store.Observation{SweepID: second, Kind: KindCT, Subject: "acme.test", OK: false, Error: ErrCTTruncated.Error(), ObservedAt: base.Add(24 * time.Hour).Format(time.RFC3339Nano)})
	_ = st.PutObservation(ctx, store.Observation{SweepID: second, Kind: KindCT, Subject: "other.test", OK: true, Data: []byte("[]"), ObservedAt: base.Add(24 * time.Hour).Format(time.RFC3339Nano)})
	res, err := DiscoverHosts(ctx, st, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].Name != "shop.acme.test" {
		t.Fatalf("the earlier read's hosts must stay: %+v", res.Hosts)
	}
	if len(res.Unverified) != 1 || res.Unverified[0].Subject != "acme.test" || res.Unverified[0].SweepID != second ||
		!strings.Contains(res.Unverified[0].Reason, ErrCTTruncated.Error()) {
		t.Fatalf("the failed latest read was not disclosed (and only it): %+v", res.Unverified)
	}
}

func TestDiscoverHostsSubtractsInventory(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}, {Kind: "host", Value: "www.acme.test", Port: 443}})
	sw, _ := st.BeginSweep(ctx, "")
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "acme.test", OK: true, Data: []byte("[]")})
	_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{{Domain: "acme.test", CrtID: 9, IssuerName: "O=Let's Encrypt", NotAfter: "2027-01-01T00:00:00Z",
		Names: []string{"acme.test", "www.acme.test", "shop.acme.test", "*.acme.test", "acme.test.evil.example"}}})
	res, err := DiscoverHosts(ctx, st, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) != 3 || res.Hosts[0].Name != "*.acme.test" || !res.Hosts[0].Wildcard || res.Hosts[1].Name != "acme.test" || res.Hosts[2].Name != "shop.acme.test" {
		t.Fatalf("hosts: %+v", res.Hosts)
	}
}
