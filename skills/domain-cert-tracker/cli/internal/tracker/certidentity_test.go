// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// SameCertificate is the one "same certificate" rule: the whole issuer DN in
// either spelling, the serial and the validity window, failing closed when
// any part cannot be read.
func TestSameCertificateWholeIssuerDNFailsClosed(t *testing.T) {
	const nb, na = "2026-10-01T00:00:00Z", "2027-01-01T00:00:00Z"
	ref := func(issuer, serial string) CertRef {
		return CertRef{Issuer: issuer, Serial: serial, NotBefore: nb, NotAfter: na}
	}
	// What crypto/x509 prints for an issuer carrying a type it has no name
	// for: the dotted OID and the DER value in hex.
	thawte := pkix.Name{Country: []string{"ZA"}, Organization: []string{"Thawte Consulting cc"}, CommonName: "Thawte Premium Server CA",
		Names: []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}, Value: "premium-server@thawte.com"}}}
	same := []struct{ name, a, b string }{
		{"two spellings of one issuer", "CN=R11,O=Let's Encrypt,C=US", "C=US, O=Let's Encrypt, CN=R11"},
		{"attribute order", "CN=R11,C=US,O=Let's Encrypt", "C=US, O=Let's Encrypt, CN=R11"},
		{"escaped and bare commas", `CN=Go Daddy Secure Certificate Authority - G2,OU=http://certs.godaddy.com/repository/,O=GoDaddy.com\, Inc.,L=Scottsdale,ST=Arizona,C=US`,
			"C=US, ST=Arizona, L=Scottsdale, O=GoDaddy.com, Inc., OU=http://certs.godaddy.com/repository/, CN=Go Daddy Secure Certificate Authority - G2"},
		{"quoted value", `CN=X1,O=GoDaddy.com\, Inc.,C=US`, `C=US, O="GoDaddy.com, Inc.", CN=X1`},
		{"multi-valued RDN", "CN=CA1,OU=a+OU=b,O=Org,C=US", "C=US, O=Org, OU=a + OU=b, CN=CA1"},
		{"RFC 4514 hex escape", "CN=Zertifizierungsstelle Süd,O=Org,C=DE", `C=DE, O=Org, CN=Zertifizierungsstelle S\C3\BCd`},
		{"case and white space", "CN=R11,O=Let's  Encrypt,C=US", "c=us, o=LET'S ENCRYPT, cn=r11"},
		{"Go #hex value for an unnamed type", thawte.String(),
			"C=ZA, O=Thawte Consulting cc, CN=Thawte Premium Server CA, emailAddress=premium-server@thawte.com"},
	}
	for _, c := range same {
		if !SameCertificate(ref(c.a, "0A:BC"), ref(c.b, "abc")) {
			t.Errorf("%s: one certificate read as two:\n%q\n%q", c.name, c.a, c.b)
		}
		if CertKey(c.a, "0abc", nb, na) != CertKey(c.b, "abc", nb, na) {
			t.Errorf("%s: one certificate, two event keys", c.name)
		}
	}
	different := []struct{ name, a, b string }{
		{"two intermediates of one CA", "CN=R10,O=Let's Encrypt,C=US", "C=US, O=Let's Encrypt, CN=R11"},
		{"an attribute only one side has", "CN=R11,O=Let's Encrypt,C=US", "C=US, O=Let's Encrypt, OU=Ops, CN=R11"},
		{"two organizations", "CN=CA1,O=Issuer A,C=US", "C=US, O=Issuer B, CN=CA1"},
	}
	for _, c := range different {
		if SameCertificate(ref(c.a, "01"), ref(c.b, "01")) {
			t.Errorf("%s: two certificates read as one:\n%q\n%q", c.name, c.a, c.b)
		}
		if CertKey(c.a, "01", nb, na) == CertKey(c.b, "01", nb, na) {
			t.Errorf("%s: two certificates share an event key", c.name)
		}
	}
	// Fail closed: identical text that cannot be read is not evidence of
	// one certificate.
	unreadable := []struct {
		name string
		ref  CertRef
	}{
		{"empty issuer", ref("", "01")},
		{"issuer text that is not a DN", ref("Issuer Not Found", "01")},
		{"text before the first attribute", ref("garbage, O=Let's Encrypt, CN=R11", "01")},
		{"unterminated quote", ref(`C=US, O="Let's Encrypt, CN=R11`, "01")},
		{"trailing backslash", ref(`C=US, O=Let's Encrypt, CN=R11\`, "01")},
		{"undecodable #hex value", ref("CN=R11,1.2.3.4=#zz", "01")},
		{"bad attribute type", ref("C N=R11, O=Let's Encrypt, C=US", "01")},
		{"empty serial", ref("C=US, O=Let's Encrypt, CN=R11", "")},
		{"non-hex serial", ref("C=US, O=Let's Encrypt, CN=R11", "xyz")},
		{"missing validity", CertRef{Issuer: "C=US, O=Let's Encrypt, CN=R11", Serial: "01"}},
	}
	for _, c := range unreadable {
		if SameCertificate(c.ref, c.ref) {
			t.Errorf("%s: an unreadable certificate matched itself", c.name)
		}
	}
}

// A served certificate whose identity cannot be read is never "in CT" on a
// guess, even when a CT row carries the same unreadable text.
func TestRenewalsUnreadableServedIssuerIsNotMatched(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	sw, _ := st.BeginSweep(ctx, "")
	tlsData, _ := json.Marshal(TLSObs{Host: "www.example.com", Port: 443, Serial: "01", Issuer: "",
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2027-01-01T00:00:00Z", DaysLeft: 65, Trusted: true, HostnameMatch: true})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindTLS, Subject: "www.example.com:443", OK: true, Data: tlsData})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "example.com", OK: true, Data: []byte(`{"certs":[],"caa_by_name":{}}`)})
	_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{{Domain: "example.com", CrtID: 7, IssuerName: "",
		NotBefore: "2026-10-01T00:00:00Z", NotAfter: "2027-01-01T00:00:00Z", Serial: "01", Names: []string{"www.example.com"}}})
	_ = st.FinishSweep(ctx, sw, "ok")
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ServedInCT != nil || got[0].Status != RenewalServedNotLogged {
		t.Fatalf("unreadable served certificate matched on a guess: %+v", got)
	}
}

// renewalsFixture stores one TLS observation of www.example.com and the
// given CT rows for example.com, then runs Renewals.
func renewalsFixture(t *testing.T, served TLSObs, rows ...store.CTCertRow) RenewalFinding {
	t.Helper()
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}, {Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	sw, _ := st.BeginSweep(ctx, "")
	tlsData, _ := json.Marshal(served)
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindTLS, Subject: "www.example.com:443", OK: true, Data: tlsData})
	_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "example.com", OK: true, Data: []byte(`{"certs":[],"caa_by_name":{}}`)})
	for i := range rows {
		rows[i].Domain, rows[i].Names = "example.com", []string{"www.example.com"}
	}
	_, _ = st.AddCTCerts(ctx, sw, rows)
	_ = st.FinishSweep(ctx, sw, "ok")
	got, err := Renewals(ctx, st, "", 14, now)
	if err != nil || len(got) != 1 {
		t.Fatalf("renewals: %+v %v", got, err)
	}
	return got[0]
}

// "Same issuer organization" (the renewal lineage) fails closed too: two
// issuers with no organization are one lineage only when their whole DNs
// agree, so another CA's newer certificate cannot pose as the successor and
// hide a stalled issuance. Intermediates of one organization stay one
// lineage.
func TestRenewalsLineageIssuerWithoutOrganization(t *testing.T) {
	served := TLSObs{Host: "www.example.com", Port: 443, Serial: "aa", Issuer: "CN=Foo CA",
		NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z", DaysLeft: 4, Trusted: true, HostnameMatch: true}
	logged := store.CTCertRow{CrtID: 1, IssuerName: "CN=Foo CA", Serial: "aa", NotBefore: "2026-08-01T00:00:00Z", NotAfter: "2026-11-01T00:00:00Z"}
	newer := func(issuer string) store.CTCertRow {
		return store.CTCertRow{CrtID: 2, IssuerName: issuer, Serial: "bb", NotBefore: "2026-10-20T00:00:00Z", NotAfter: "2027-01-18T00:00:00Z"}
	}
	f := renewalsFixture(t, served, logged, newer("CN=Bar CA"))
	if f.Status != RenewalIssuanceStalled || f.Successor != nil || len(f.OtherCovering) != 1 {
		t.Fatalf("another CA without an organization counted as the successor: %+v", f)
	}
	if f = renewalsFixture(t, served, logged, newer("CN=Foo CA")); f.Successor == nil {
		t.Fatalf("same CA without an organization lost its successor: %+v", f)
	}
	served.Issuer = "CN=R10,O=Let's Encrypt,C=US"
	logged.IssuerName = "C=US, O=Let's Encrypt, CN=R10"
	if f = renewalsFixture(t, served, logged, newer("C=US, O=Let's Encrypt, CN=R11")); f.Successor == nil || f.ServedInCT == nil || !*f.ServedInCT {
		t.Fatalf("R10 to R11 rotation is one organization's lineage: %+v", f)
	}
}

// A cert.expiring event is keyed by the whole certificate identity: the
// certificate a rotated intermediate signed with the same serial and
// validity is a different certificate and raises its own event.
func TestCertExpiringEventIDSeparatesIntermediates(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "host", Value: "www.example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	obs := TLSObs{Host: "www.example.com", Port: 443, Serial: "01", Issuer: "CN=R10,O=Let's Encrypt,C=US",
		NotBefore: "2026-08-12T00:00:00Z", NotAfter: "2026-11-10T00:00:00Z", DaysLeft: 12}
	tlsf := &fakeTLS{obs: map[string]TLSObs{"www.example.com:443": obs}}
	src := Sources{TLS: tlsf, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindTLS: true}, Now: func() time.Time { return now }}
	r1, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	obs.Issuer = "CN=R11,O=Let's Encrypt,C=US"
	tlsf.obs["www.example.com:443"] = obs
	r2, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if eventTypes(r1.Events)[EventCertExpiring] != 1 || eventTypes(r2.Events)[EventCertExpiring] != 1 {
		t.Fatalf("cert.expiring events: sweep 1 %v, sweep 2 %v", r1.Events, r2.Events)
	}
}

// "First-ever issuer" fails closed: an issuer with no organization, or one
// that does not read as a DN, never makes a later certificate from another
// such issuer routine.
func TestIssuerAuditNewIssuerWithoutReadableOrganization(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct{ name, before, after string }{
		{"no organization", "CN=Old CA", "CN=New CA"},
		{"not a DN", "Issuer Not Found", "Unknown issuer"},
	}
	for _, c := range cases {
		ctx := context.Background()
		st := openTestStore(t)
		_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "acme.test", Port: 443}})
		sw, _ := st.BeginSweep(ctx, "")
		ctObs, _ := json.Marshal(CTObs{Certs: []CTCert{}, CAA: map[string]NameCAA{"acme.test": {Source: "acme.test"}}})
		_ = st.PutObservation(ctx, store.Observation{SweepID: sw, Kind: KindCT, Subject: "acme.test", OK: true, Data: ctObs})
		_, _ = st.AddCTCerts(ctx, sw, []store.CTCertRow{
			{Domain: "acme.test", CrtID: 1, IssuerName: c.before, NotBefore: "2026-06-01T00:00:00Z", Names: []string{"acme.test"}},
			{Domain: "acme.test", CrtID: 2, IssuerName: c.after, NotBefore: "2026-10-25T00:00:00Z", Names: []string{"acme.test"}},
		})
		// The in-window certificate's names came from the certificate itself,
		// so the audit reaches the first-ever-issuer rule (a crt.sh-only name
		// list is names_partial before that rule is asked).
		if err := st.PutCertNames(ctx, []store.CertNames{{CrtID: 2, Status: store.CertNamesOK, Names: []string{"acme.test"}}}); err != nil {
			t.Fatal(err)
		}
		_ = st.FinishSweep(ctx, sw, "ok")
		res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -7))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Findings) != 1 || res.Findings[0].Status != "new_issuer" || res.Routine != 0 {
			t.Errorf("%s: a new issuer read as routine: %+v routine %d", c.name, res.Findings, res.Routine)
		}
	}
}
