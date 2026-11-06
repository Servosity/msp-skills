// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

var (
	testCertKeyOnce sync.Once
	testCertKey     *ecdsa.PrivateKey
)

// mustCertDER builds a PEM certificate (the form crt.sh's ?d= download
// returns) matching crt.sh row c: issuer O/CN/C from c.IssuerName, serial
// and validity from the row, and names as its SAN dNSNames.
func mustCertDER(c CTCert, names ...string) []byte {
	testCertKeyOnce.Do(func() {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(err)
		}
		testCertKey = k
	})
	serial, ok := new(big.Int).SetString(NormalizeSerial(c.Serial), 16)
	if !ok {
		panic("bad test serial " + c.Serial)
	}
	nb, err1 := time.Parse(time.RFC3339, c.NotBefore)
	na, err2 := time.Parse(time.RFC3339, c.NotAfter)
	if err1 != nil || err2 != nil {
		panic(fmt.Sprintf("bad test validity %q %q", c.NotBefore, c.NotAfter))
	}
	issuer := pkix.Name{CommonName: dnAttr(c.IssuerName, "CN")}
	if o := dnAttr(c.IssuerName, "O"); o != "" {
		issuer.Organization = []string{o}
	}
	if cc := dnAttr(c.IssuerName, "C"); cc != "" {
		issuer.Country = []string{cc}
	}
	parent := &x509.Certificate{Subject: issuer, PublicKey: testCertKey.Public()}
	cn := ""
	if len(names) > 0 {
		cn = names[0]
	}
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: cn}, NotBefore: nb, NotAfter: na, DNSNames: names}
	der, err := x509.CreateCertificate(rand.Reader, leaf, parent, testCertKey.Public(), testCertKey)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// The r2 setup with the certificate itself available: crt.sh showed only
// shop.example.com, the certificate also names *.example.com, and
// example.com's issuewild permits only DigiCert. The audit must judge the
// wildcard and flag the Let's Encrypt certificate.
func TestIssuerAuditJudgesWildcardReadFromTheCertificate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	body := `[{"issuer_ca_id":295815,"issuer_name":"C=US, O=Let's Encrypt, CN=R11","common_name":"shop.example.com","name_value":"shop.example.com","id":42,"not_before":"2026-10-20T00:00:00","not_after":"2027-01-18T00:00:00","serial_number":"0abc","result_count":2}]`
	certs, err := ParseCrtSh([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	dns := func() *fakeDNS {
		return &fakeDNS{raw: map[string][][]byte{
			"shop.example.com|CAA": {caaRData(0, "issue", "letsencrypt.org")},
			"example.com|CAA":      {caaRData(0, "issuewild", "digicert.com")},
		}}
	}
	run := func(t *testing.T, ct *fakeCT) IssuerAudit {
		t.Helper()
		st := openTestStore(t)
		if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "shop.example.com", Port: 443}}); err != nil {
			t.Fatal(err)
		}
		src := Sources{CT: ct, DNS: dns(), Guard: netguard.New()}
		if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
			t.Fatal(err)
		}
		res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	t.Run("certificate downloaded", func(t *testing.T) {
		ct := &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs},
			der: map[int64][]byte{42: mustCertDER(certs[0], "shop.example.com", "*.example.com")}}
		res := run(t, ct)
		if res.Routine != 0 || len(res.Findings) != 1 {
			t.Fatalf("findings %+v routine %d", res.Findings, res.Routine)
		}
		f := res.Findings[0]
		if f.Status != IssuerNotInCAA || f.NamesSource != store.NamesFromCertificate || !strings.Contains(f.Detail, "*.example.com (CAA at example.com)") {
			t.Fatalf("wildcard read from the certificate not judged against example.com's issuewild: %+v", f)
		}
		if res.Totals[IssuerNotInCAA] != 1 || res.Totals[IssuerRoutine] != 0 {
			t.Fatalf("totals: %v", res.Totals)
		}
	})
	t.Run("no download", func(t *testing.T) {
		res := run(t, &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs}})
		if res.Routine != 0 || len(res.Findings) != 1 || res.Findings[0].Status != IssuerNamesPartial || res.Totals[IssuerNamesPartial] != 1 {
			t.Fatalf("certificate with crt.sh's names only: %+v routine %d totals %v", res.Findings, res.Routine, res.Totals)
		}
	})
	t.Run("download is another certificate", func(t *testing.T) {
		other := certs[0]
		other.Serial = "0abd"
		ct := &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs},
			der: map[int64][]byte{42: mustCertDER(other, "shop.example.com")}}
		res := run(t, ct)
		if res.Routine != 0 || len(res.Findings) != 1 || res.Findings[0].Status != IssuerNamesPartial || !strings.Contains(res.Findings[0].Detail, "does not match") {
			t.Fatalf("a download with another serial was trusted: %+v routine %d", res.Findings, res.Routine)
		}
	})
	t.Run("download omits a name crt.sh listed", func(t *testing.T) {
		ct := &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs},
			der: map[int64][]byte{42: mustCertDER(certs[0], "www.example.com")}}
		if res := run(t, ct); res.Routine != 0 || res.Findings[0].Status != IssuerNamesPartial {
			t.Fatalf("a download that drops crt.sh's name was trusted: %+v", res.Findings)
		}
	})
	t.Run("download is not a certificate", func(t *testing.T) {
		ct := &fakeCT{certs: map[string][]CTCert{"shop.example.com": certs}, der: map[int64][]byte{42: []byte("<html>busy</html>")}}
		if res := run(t, ct); res.Routine != 0 || res.Findings[0].Status != IssuerNamesPartial {
			t.Fatalf("an HTML answer was read as a certificate: %+v", res.Findings)
		}
	})
}

// The one rule has two halves: complete names are not enough when the CAA
// set for one of them was not read.
func TestIssuerAuditCompleteNamesWithUnreadCAAAreNotRoutine(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	c := ctCert(7, "C=US, O=Let's Encrypt, CN=R11", "2026-10-20T00:00:00Z", "2027-01-18T00:00:00Z", "07", "www.example.com", "api.example.com")
	dns := &fakeDNS{raw: map[string][][]byte{"example.com|CAA": {caaRData(0, "issue", "letsencrypt.org")}}, fail: map[string]bool{"api.example.com": true}}
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"example.com": {c}}, autoDER: true}, DNS: dns, Guard: netguard.New()}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil {
		t.Fatal(err)
	}
	if res.Routine != 0 || len(res.Findings) != 1 || res.Findings[0].Status != IssuerCAAUnverified || res.Totals[IssuerCAAUnverified] != 1 {
		t.Fatalf("certificate with an unread CAA set counted as routine: %+v routine %d", res.Findings, res.Routine)
	}
	// Once CAA is readable the same certificate is routine.
	dns.fail = nil
	if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14)); err != nil {
		t.Fatal(err)
	}
	if res.Routine != 1 || len(res.Findings) != 0 {
		t.Fatalf("fully checked certificate: %+v routine %d", res.Findings, res.Routine)
	}
}

// Downloads are capped per sweep, newest first; certificates past the cap
// stay partial and are fetched by a later sweep; a verified certificate is
// never downloaded again.
func TestSweepCertificateDownloadsAreCappedAndOncePerID(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}}); err != nil {
		t.Fatal(err)
	}
	le := "C=US, O=Let's Encrypt, CN=R11"
	var certs []CTCert
	for i := 1; i <= 5; i++ {
		certs = append(certs, ctCert(int64(i), le, fmt.Sprintf("2026-10-%02dT00:00:00Z", 20+i), "2027-01-18T00:00:00Z", fmt.Sprintf("%02x", i), fmt.Sprintf("h%d.example.com", i)))
	}
	ct := &fakeCT{certs: map[string][]CTCert{"example.com": certs}, autoDER: true}
	src := Sources{CT: ct, DNS: &fakeDNS{}, Guard: netguard.New()}
	opt := SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true}, CertDownloads: 2, Now: func() time.Time { return now }}
	r1, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := ct.downloads(); len(got) != 2 || got[0] != 5 || got[1] != 4 {
		t.Fatalf("sweep 1 downloads %v, want the two newest (5, 4)", got)
	}
	if r1.CertNames != (CertNameStats{Complete: 2, Downloaded: 2, Partial: 3, Deferred: 3}) {
		t.Fatalf("sweep 1 cert_names: %+v", r1.CertNames)
	}
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals[IssuerNamesPartial] != 3 || res.Routine != 2 {
		t.Fatalf("audit after a capped sweep: totals %v routine %d", res.Totals, res.Routine)
	}
	for _, f := range res.Findings {
		if !strings.Contains(f.Detail, "cap") {
			t.Fatalf("partial certificate does not say why: %+v", f)
		}
	}
	r2, err := RunSweep(ctx, st, src, opt)
	if err != nil {
		t.Fatal(err)
	}
	if got := ct.downloads(); len(got) != 4 || got[2] != 3 || got[3] != 2 {
		t.Fatalf("sweep 2 downloads %v: verified certificates fetched again, or the next newest skipped", got)
	}
	if r2.CertNames.Complete != 4 || r2.CertNames.Downloaded != 2 {
		t.Fatalf("sweep 2 cert_names: %+v", r2.CertNames)
	}
	// A failed download is retried after every certificate never tried.
	ct.autoDER = false
	ct.der = map[int64][]byte{}
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	if got := ct.downloads(); len(got) != 5 || got[4] != 1 {
		t.Fatalf("sweep 3 downloads %v", got)
	}
	ct.der[1] = mustCertDER(certs[0], certs[0].Names...)
	if _, err := RunSweep(ctx, st, src, opt); err != nil {
		t.Fatal(err)
	}
	if res, err = AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14)); err != nil {
		t.Fatal(err)
	}
	if res.Routine != 5 || len(res.Findings) != 0 {
		t.Fatalf("every certificate downloaded: %+v routine %d", res.Findings, res.Routine)
	}
	// Two inventoried domains sharing a certificate download it once.
	st2 := openTestStore(t)
	_ = st2.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "a.example.com", Port: 443}, {Kind: "domain", Value: "b.example.com", Port: 443}})
	shared := ctCert(99, le, "2026-10-25T00:00:00Z", "2027-01-18T00:00:00Z", "63", "a.example.com", "b.example.com")
	ct2 := &fakeCT{certs: map[string][]CTCert{"a.example.com": {shared}, "b.example.com": {shared}}, autoDER: true}
	r, err := RunSweep(ctx, st2, Sources{CT: ct2, DNS: &fakeDNS{}, Guard: netguard.New()}, SweepOptions{Kinds: map[string]bool{KindCT: true}, Concurrency: 2, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if got := ct2.downloads(); len(got) != 1 || r.CertNames.Complete != 2 {
		t.Fatalf("shared certificate downloads %v, cert_names %+v", got, r.CertNames)
	}
}

// A source that claims its names came from the certificate is not trusted:
// only a verified download marks a list complete.
func TestSweepIgnoresANamesSourceTheCTSearchClaims(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	_ = st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "example.com", Port: 443}})
	c := ctCert(3, "C=US, O=Let's Encrypt, CN=R11", "2026-10-20T00:00:00Z", "2027-01-18T00:00:00Z", "03", "www.example.com")
	c.NamesSource = store.NamesFromCertificate
	src := Sources{CT: &fakeCT{certs: map[string][]CTCert{"example.com": {c}}}, DNS: &fakeDNS{}, Guard: netguard.New()}
	if _, err := RunSweep(ctx, st, src, SweepOptions{Kinds: map[string]bool{KindCT: true}, Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	res, err := AuditIssuers(ctx, st, "", now.AddDate(0, 0, -14))
	if err != nil || res.Routine != 0 || res.Findings[0].Status != IssuerNamesPartial {
		t.Fatalf("claimed names source trusted: %+v %v", res.Findings, err)
	}
}

// A store created before tracker_cert_names, opened read-only (no
// migration), still lists its certificates: all partial, none missing.
func TestListCTCertsWithoutCertNamesTableIsPartialNotEmpty(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	sw, _ := st.BeginSweep(ctx, "")
	if _, err := st.AddCTCerts(ctx, sw, []store.CTCertRow{{Domain: "example.com", CrtID: 1, Names: []string{"www.example.com"}}}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutCertNames(ctx, []store.CertNames{{CrtID: 1, Status: store.CertNamesOK, Names: []string{"*.example.com", "www.example.com"}}}); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListCTCerts(ctx, "example.com")
	if err != nil || len(rows) != 1 || rows[0].NamesSource != store.NamesFromCertificate || len(rows[0].Names) != 2 {
		t.Fatalf("with names: %+v %v", rows, err)
	}
	// A verified record is final.
	if err := st.PutCertNames(ctx, []store.CertNames{{CrtID: 1, Status: store.CertNamesFailed, Error: "HTTP 502"}}); err != nil {
		t.Fatal(err)
	}
	if rows, _ = st.ListCTCerts(ctx, "example.com"); rows[0].NamesSource != store.NamesFromCertificate {
		t.Fatalf("a later failure replaced a verified name list: %+v", rows[0])
	}
	if _, err := st.DB().ExecContext(ctx, `DROP TABLE tracker_cert_names`); err != nil {
		t.Fatal(err)
	}
	rows, err = st.ListCTCerts(ctx, "example.com")
	if err != nil || len(rows) != 1 || rows[0].NamesSource != store.NamesFromCrtSh {
		t.Fatalf("without the table: %+v %v", rows, err)
	}
}

type urlRecorder struct {
	mu   sync.Mutex
	urls []string
	next http.RoundTripper
}

func (r *urlRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.urls = append(r.urls, req.URL.String())
	r.mu.Unlock()
	return r.next.RoundTrip(req)
}

// The certificate download reaches https://crt.sh on port 443 only, through
// the guard, whatever the id; a non-positive id sends nothing.
func TestCertificateDownloadTargetsOnlyCrtSh443(t *testing.T) {
	var mu sync.Mutex
	var hosts, dials []string
	g := &netguard.Guard{
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			mu.Lock()
			hosts = append(hosts, host)
			mu.Unlock()
			return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			mu.Lock()
			dials = append(dials, addr)
			mu.Unlock()
			return nil, errors.New("test dialer: no network")
		},
	}
	w := NewWeb(g, 1000, 1<<20)
	w.Attempts = 1
	rec := &urlRecorder{next: w.HTTP.Transport}
	w.HTTP.Transport = rec
	ct := &CT{Web: w}
	for _, id := range []int64{0, -5} {
		if _, err := ct.Certificate(context.Background(), id); err == nil {
			t.Fatalf("id %d accepted", id)
		}
	}
	if len(rec.urls) != 0 || len(dials) != 0 {
		t.Fatalf("a non-positive id sent a request: %v %v", rec.urls, dials)
	}
	for _, id := range []int64{42, math.MaxInt64} {
		if _, err := ct.Certificate(context.Background(), id); err == nil {
			t.Fatal("test dialer reached")
		}
	}
	want := []string{"https://crt.sh/?d=42", fmt.Sprintf("https://crt.sh/?d=%d", int64(math.MaxInt64))}
	if fmt.Sprint(rec.urls) != fmt.Sprint(want) {
		t.Fatalf("requested %v, want %v", rec.urls, want)
	}
	for _, h := range hosts {
		if h != "crt.sh" {
			t.Fatalf("resolved %q", h)
		}
	}
	for _, d := range dials {
		if d != "93.184.215.14:443" {
			t.Fatalf("dialed %q", d)
		}
	}
	if len(dials) == 0 {
		t.Fatal("the download did not go through the guard's dialer")
	}
}

// A real crt.sh download (https://crt.sh/?d=29581277802, captured
// 2026-10-28, PEM as served) checked against that certificate's row in
// testdata/crtsh-example.com.json.
func TestCertificateNamesFromCrtShCapture(t *testing.T) {
	body, err := os.ReadFile("testdata/crtsh-d-29581277802.txt")
	if err != nil {
		t.Fatal(err)
	}
	certs, err := ParseCrtSh(readFixture(t, "crtsh-example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	var row CTCert
	for _, c := range certs {
		if c.ID == 29581277802 {
			row = c
		}
	}
	names, err := CertificateNames(body, row)
	if err != nil || fmt.Sprint(names) != "[*.example.com example.com]" {
		t.Fatalf("names %v err %v", names, err)
	}
	row.NotAfter = "2026-12-25T22:56:36Z"
	if _, err := CertificateNames(body, row); err == nil {
		t.Fatal("a validity mismatch was accepted")
	}
}

// Renewals may only use a crt.sh-matched name list to prove a successor,
// never to rule one out, and "served in CT" is the served certificate's
// identity, not its names. Host shop.example.com is covered by
// *.example.com on both certificates, which crt.sh's match for
// shop.example.com does not show.
func TestRenewalsPartialNamesNeverRuleOutASuccessor(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	le := "C=US, O=Let's Encrypt, CN=R11"
	old := ctCert(1, le, "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", "aa", "api.shop.example.com")
	succ := ctCert(2, le, "2026-10-20T00:00:00Z", "2027-01-18T00:00:00Z", "bb", "api.shop.example.com")
	served := TLSObs{Host: "shop.example.com", Port: 443, Serial: "aa", Issuer: "CN=R11,O=Let's Encrypt,C=US", NotBefore: old.NotBefore, NotAfter: old.NotAfter, DaysLeft: 3, Trusted: true, HostnameMatch: true}
	run := func(t *testing.T, ct *fakeCT) RenewalFinding {
		t.Helper()
		st := openTestStore(t)
		if err := st.AddTargets(ctx, "Acme", []store.Target{{Kind: "domain", Value: "shop.example.com", Port: 443}, {Kind: "host", Value: "shop.example.com", Port: 443}}); err != nil {
			t.Fatal(err)
		}
		src := Sources{CT: ct, TLS: &fakeTLS{obs: map[string]TLSObs{"shop.example.com:443": served}}, DNS: &fakeDNS{}, Guard: netguard.New()}
		if _, err := RunSweep(ctx, st, src, SweepOptions{Days: 30, Kinds: map[string]bool{KindCT: true, KindTLS: true}, Now: func() time.Time { return now }}); err != nil {
			t.Fatal(err)
		}
		got, err := Renewals(ctx, st, "", 14, now)
		if err != nil || len(got) != 1 {
			t.Fatalf("renewals %+v %v", got, err)
		}
		return got[0]
	}
	t.Run("crt.sh names only", func(t *testing.T) {
		f := run(t, &fakeCT{certs: map[string][]CTCert{"shop.example.com": {old, succ}}})
		if f.Status != RenewalCTUnverified || f.ServedInCT == nil || !*f.ServedInCT {
			t.Fatalf("a successor hidden by crt.sh's matched names was ruled out: %+v", f)
		}
	})
	t.Run("certificates downloaded", func(t *testing.T) {
		ct := &fakeCT{certs: map[string][]CTCert{"shop.example.com": {old, succ}}, der: map[int64][]byte{
			1: mustCertDER(old, "api.shop.example.com", "*.example.com"),
			2: mustCertDER(succ, "api.shop.example.com", "*.example.com"),
		}}
		f := run(t, ct)
		if f.Successor == nil || f.Successor["crt_id"].(int64) != 2 || f.Status == RenewalIssuanceStalled || f.Status == RenewalCTUnverified {
			t.Fatalf("successor read from the certificate not found: %+v", f)
		}
	})
}
