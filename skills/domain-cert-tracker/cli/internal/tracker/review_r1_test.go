// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// RFC 9083 section 5.3 makes "status" optional on a domain object. A
// response without it says nothing about the transfer lock, so it must
// never read as "unlocked" (no registrar.unlocked event, no unlocked row).
func TestRDAPMissingStatusIsUnknownNotUnlocked(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	body := `{"objectClassName":"domain","ldhName":"example.com","events":[{"eventAction":"expiration","eventDate":"2027-08-13T04:00:00Z"}]}`
	obs, err := ParseRDAPDomain([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	src := Sources{RDAP: &fakeRDAP{obs: map[string]DomainObs{"example.com": obs}}, Guard: netguard.New()}
	res, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindRDAP: true}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(res.Events); got[EventRegistrarUnlocked] != 0 {
		t.Fatalf("status-less RDAP answer raised registrar.unlocked: %v", got)
	}
	rep, err := BuildReport(ctx, st, "", 30, now.AddDate(0, 0, -7), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Clients[0].Unlocked) != 0 {
		t.Fatalf("status-less RDAP answer listed as unlocked: %+v", rep.Clients[0].Unlocked)
	}
	unknown := false
	for _, u := range rep.Clients[0].Unverified {
		if u.Kind == KindRDAP && u.Subject == "example.com" && strings.Contains(u.Reason, "status") {
			unknown = true
		}
	}
	if !unknown {
		t.Fatalf("unknown lock state not listed as unverified: %+v", rep.Clients[0].Unverified)
	}
}

// crt.sh (certwatch_db fnc/web_apis.fnc, c_resultLimit = 10000) caps the
// matching identities and warns about truncation in HTML output only. A
// JSON answer whose result_count reaches the cap is a partial, random
// subset and must not be read as the domain's certificate history.
func TestCrtShTruncatedResultIsAnError(t *testing.T) {
	row := func(id, count int) string {
		return fmt.Sprintf(`{"issuer_ca_id":1,"issuer_name":"C=US, O=Let's Encrypt, CN=R11","common_name":"a.example.com","name_value":"a.example.com","id":%d,"not_before":"2026-10-01T00:00:00","not_after":"2027-01-01T00:00:00","serial_number":"0%d","result_count":%d}`, id, id, count)
	}
	for name, body := range map[string]string{
		"one row at the cap":    "[" + row(1, 10000) + "]",
		"rows summing to cap":   "[" + row(1, 6000) + "," + row(2, 4000) + "]",
		"rows past the cap":     "[" + row(1, 9999) + "," + row(2, 2) + "]",
		"capped row, any shape": "[" + row(1, 12000) + "]",
	} {
		if _, err := ParseCrtSh([]byte(body)); err == nil {
			t.Errorf("%s: truncated crt.sh answer accepted as complete", name)
		}
	}
	if c, err := ParseCrtSh([]byte("[" + row(1, 2) + "," + row(2, 3) + "]")); err != nil || len(c) != 2 {
		t.Errorf("small answer: %v %v", c, err)
	}
}

// The successor certificate's not_before is not the time it was logged
// (crt.sh identity search returns no entry timestamp). deploy_stalled may
// only be claimed once this tool itself saw the successor in CT more than
// the grace period before a probe that still showed the old certificate.
func TestRenewalsDeployStalledNeedsOwnSighting(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	day := 24 * time.Hour
	f := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "emudhra-lab.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	em := "C=NL, O=eMudhra BV, CN=EM DV TLS CA - G3B"
	oldNB, oldNA := now.Add(-57*day), now.Add(65*day)
	served := TLSObs{Host: "emudhra-lab.example.com", Port: 443, Serial: "aa", Issuer: "CN=EM DV TLS CA - G3B,O=eMudhra BV,C=NL",
		NotBefore: f(oldNB), NotAfter: f(oldNA), DaysLeft: 65, Trusted: true, HostnameMatch: true}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": {
		ctCert(1, em, f(oldNB), f(oldNA), "aa", "emudhra-lab.example.com"),
		ctCert(30003111654, em, f(now.Add(-7*day)), f(now.Add(277*day)), "298e86054c63f6553cb78d46aeb08b0f75658494", "emudhra-lab.example.com"),
	}}}
	src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"emudhra-lab.example.com:443": served}}, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Now: func() time.Time { return now }}
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	findings, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Successor == nil {
		t.Fatalf("renewals: %+v", findings)
	}
	if findings[0].Status == RenewalDeployStalled {
		t.Fatalf("successor first seen in the same sweep as the probe reported deploy_stalled: %+v", findings[0])
	}
	// Two days later the host still serves the old certificate: now it is a
	// stalled deploy, measured from this tool's own first sighting.
	later := now.Add(2 * day)
	opt.Now = func() time.Time { return later }
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	if findings, err = Renewals(ctx, st, "", 14, later); err != nil {
		t.Fatal(err)
	}
	if findings[0].Status != RenewalDeployStalled {
		t.Fatalf("successor seen two days before the probe: %s, want deploy_stalled", findings[0].Status)
	}
}

// crt.sh identifies a certificate by issuer CA and serial (its deduplicate
// join, certwatch_db fnc/web_apis.fnc). A CT row with the served serial from
// another issuer, with other validity dates, is a different certificate.
func TestRenewalsServedCertIdentityIsNotSerialAlone(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	sw, _ := st.BeginSweep(ctx, "")
	tlsData, _ := json.Marshal(TLSObs{Host: "www.example.com", Port: 443, Serial: "01", Issuer: "CN=R11,O=Let's Encrypt,C=US",
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2027-01-01T00:00:00Z", DaysLeft: 65, Trusted: true, HostnameMatch: true})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindTLS, Subject: "www.example.com:443", OK: true, Data: tlsData})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "example.com", OK: true, Data: []byte("[]")})
	_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{{Domain: "example.com", CrtID: 7, IssuerName: "C=US, O=DigiCert Inc, CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1",
		NotBefore: "2026-09-01T00:00:00Z", NotAfter: "2026-12-01T00:00:00Z", Serial: "01", Names: []string{"www.example.com"}}})
	_ = st.FinishSweep(ctx, sw, "ok")
	findings, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].ServedInCT == nil || *findings[0].ServedInCT || findings[0].Status == RenewalOK {
		t.Fatalf("another issuer's certificate with the same serial counted as the served one: %+v", findings)
	}
}

// Two issuers can use the same serial; the cert.expiring event for a host
// must not be swallowed because a different certificate shared the serial.
func TestCertExpiringEventIDIsCertificateIdentity(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	tlsf := &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": {Host: "www.example.com", Port: 443, Serial: "01", Issuer: "CN=A1,O=Issuer A,C=US",
		NotBefore: "2026-08-12T00:00:00Z", NotAfter: "2026-11-10T00:00:00Z", DaysLeft: 12}}}
	src := Sources{TLS: tlsf, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindTLS: true}, Now: func() time.Time { return now }}
	r1, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	tlsf.obs["www.example.com:443"] = TLSObs{Host: "www.example.com", Port: 443, Serial: "01", Issuer: "CN=B1,O=Issuer B,C=US",
		NotBefore: "2026-08-22T00:00:00Z", NotAfter: "2026-11-20T00:00:00Z", DaysLeft: 22}
	r2, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if eventTypes(r1.Events)[EventCertExpiring] != 1 || eventTypes(r2.Events)[EventCertExpiring] != 1 {
		t.Fatalf("cert.expiring events: sweep 1 %v, sweep 2 %v", r1.Events, r2.Events)
	}
	// The same certificate again raises nothing new.
	r3, err := RunSweep(ctx, st, src, opt)
	if err != nil || len(r3.Events) != 0 {
		t.Fatalf("repeat sweep: %v %v", r3.Events, err)
	}
}

// RFC 8659 section 3: the relevant CAA RRset is found by climbing from each
// certificate name, so a subdomain's own CAA overrides its parent's. The
// eMudhra certificate 30003111654 in testdata/crtsh-example.com.json names
// emudhra-lab.example.com only; with that name publishing
// 0 issue "emudhra.com" it is permitted, whatever example.com says.
func TestIssuerAuditUsesEachNamesOwnCAA(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	em := ctCert(30003111654, "C=NL, O=eMudhra BV, CN=EM DV TLS CA - G3B", "2026-10-20T21:19:03Z", "2027-08-01T21:19:03Z",
		"298e86054c63f6553cb78d46aeb08b0f75658494", "emudhra-lab.example.com")
	dns := &fakeDNS{raw: map[string][][]byte{
		"example.com|CAA":             {caaRData(0, "issue", "letsencrypt.org")},
		"emudhra-lab.example.com|CAA": {caaRData(0, "issue", "emudhra.com")},
	}}
	sweepAuditFixture(t, st, now, dns, em)
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.CrtID == 30003111654 && f.Status == "not_in_caa" {
			t.Fatalf("subdomain's own CAA ignored: %+v", f)
		}
	}
	// The reverse: the child forbids what the parent allows.
	dns.raw["emudhra-lab.example.com|CAA"] = [][]byte{caaRData(0, "issue", "letsencrypt.org")}
	dns.raw["example.com|CAA"] = [][]byte{caaRData(0, "issue", "emudhra.com")}
	sweepAuditFixture(t, st, now, dns, em)
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14)); err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].Status != "not_in_caa" {
		t.Fatalf("subdomain CAA forbidding the issuer not reported: %+v", res.Findings)
	}
}

// RFC 8659 section 4.3: issuewild governs wildcard names only. Certificate
// 29581277802 (testdata/crtsh-example.com.json) names *.example.com and
// example.com; with issue "letsencrypt.org" and issuewild "ssl.com" its SSL
// Corporation issuer is forbidden for the apex name.
func TestIssuerAuditChecksEveryNameWithItsOwnProperty(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	cf := ctCert(29581277802, "C=US, O=SSL Corporation, CN=Cloudflare TLS Issuing ECC CA 3", "2026-09-26T22:49:11Z", "2026-12-25T22:56:35Z",
		"01eee6aabb521d5e14fc315fd9985690", "*.example.com", "example.com")
	dns := &fakeDNS{raw: map[string][][]byte{"example.com|CAA": {caaRData(0, "issue", "letsencrypt.org"), caaRData(0, "issuewild", "ssl.com")}}}
	sweepAuditFixture(t, st, now, dns, cf)
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -40))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].Status != "not_in_caa" || !strings.Contains(res.Findings[0].Detail, "example.com (CAA at example.com)") {
		t.Fatalf("apex name forbidden by issue was passed by issuewild: %+v routine %d", res.Findings, res.Routine)
	}
	// A wildcard-only certificate from the same issuer is permitted.
	wild := ctCert(29581277803, "C=US, O=SSL Corporation, CN=Cloudflare TLS Issuing ECC CA 3", "2026-09-27T00:00:00Z", "2026-12-26T00:00:00Z", "02", "*.example.com")
	sweepAuditFixture(t, st, now, dns, cf, wild)
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.CrtID == 29581277803 && f.Status == "not_in_caa" {
			t.Fatalf("issuewild-permitted wildcard reported: %+v", f)
		}
	}
	// A CAA lookup that fails is unverified, never "permitted".
	dns.fail = map[string]bool{"example.com": true}
	sweepAuditFixture(t, st, now, dns, cf, wild)
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	unknown := false
	for _, u := range res.Unverified {
		if u.Kind == KindDNS && u.Subject == "example.com" {
			unknown = true
		}
	}
	if !unknown {
		t.Fatalf("failed CAA lookup not listed as unverified: %+v", res.Unverified)
	}
}

// RFC 8659 section 3 climbs to, not past, the root: a name with no CAA up
// to its registered domain still takes the TLD's set.
func TestRelevantCAAClimbsPastRegisteredDomain(t *testing.T) {
	dns := &fakeDNS{raw: map[string][][]byte{"test|CAA": {caaRData(0, "issue", "ca.example")}}}
	set, src, err := RelevantCAA(context.Background(), dns, "*.www.shop.test")
	if err != nil || src != "test" || len(set) != 1 {
		t.Fatalf("climb: %v %q %v", set, src, err)
	}
	o := CheckDNS(context.Background(), dns, "shop.test")
	if o.CAASource != "test" {
		t.Fatalf("CheckDNS climb stopped early: %q", o.CAASource)
	}
}

// The crt.sh and crypto/x509 spellings of one issuer fold to one key.
func TestCertKeyIssuerSpellings(t *testing.T) {
	a := CertKey("C=US, ST=Arizona, L=Scottsdale, O=GoDaddy.com, Inc., OU=http://certs.godaddy.com/repository/, CN=Go Daddy Secure Certificate Authority - G2",
		"0A:BC", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z")
	b := CertKey(`CN=Go Daddy Secure Certificate Authority - G2,OU=http://certs.godaddy.com/repository/,O=GoDaddy.com\, Inc.,L=Scottsdale,ST=Arizona,C=US`,
		"abc", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z")
	if a != b {
		t.Fatalf("same certificate, two spellings:\n%s\n%s", a, b)
	}
	if CertKey("O=Issuer A", "01", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z") == CertKey("O=Issuer B", "01", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z") {
		t.Fatal("two issuers with one serial share a key")
	}
	if issuerOrg("Issuer Not Found") != "" {
		t.Fatal("non-DN issuer text read as an organization")
	}
}

// sweepAuditFixture runs one CT sweep of example.com over certs with dns
// answering CAA queries; each certificate downloads with its listed names
// as its full SAN list.
func sweepAuditFixture(t *testing.T, st *store.Store, now time.Time, dns *fakeDNS, certs ...CTCert) {
	t.Helper()
	ctx := context.Background()
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"example.com": certs}, autoDER: true}, DNS: dns, Guard: netguard.New()}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
}
