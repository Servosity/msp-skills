// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseRDAPDomainRFC9083Figure24(t *testing.T) {
	obs, err := ParseRDAPDomain(readFixture(t, "rfc9083-figure24-domain.json"))
	if err != nil {
		t.Fatal(err)
	}
	if obs.Domain != "xn--fo-5ja.example" {
		t.Errorf("domain = %q", obs.Domain)
	}
	if obs.Expires != "2016-12-31T23:59:59Z" || obs.Registered != "1990-12-31T23:59:59Z" {
		t.Errorf("events: registered %q expires %q", obs.Registered, obs.Expires)
	}
	if !obs.TransferLocked {
		t.Error(`status "transfer prohibited" must read as transfer-locked`)
	}
	if obs.DelegationSigned == nil || !*obs.DelegationSigned {
		t.Error("secureDNS.delegationSigned not read")
	}
	if len(obs.Nameservers) != 2 || obs.Nameservers[0] != "ns1.example.com" {
		t.Errorf("nameservers = %v", obs.Nameservers)
	}
}

func TestParseRDAPDomainVerisignCapture(t *testing.T) {
	obs, err := ParseRDAPDomain(readFixture(t, "rdap-verisign-example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	if obs.Domain != "example.com" || obs.Expires != "2027-08-13T04:00:00Z" {
		t.Errorf("got %q expires %q", obs.Domain, obs.Expires)
	}
	if obs.Registrar != "RESERVED-Internet Assigned Numbers Authority" || obs.RegistrarIANAID != "376" {
		t.Errorf("registrar = %q (%q)", obs.Registrar, obs.RegistrarIANAID)
	}
	if !obs.TransferLocked {
		t.Error("client transfer prohibited not read as locked")
	}
	if obs.DelegationSigned == nil || !*obs.DelegationSigned {
		t.Error("DS at registry not read")
	}
}

func TestParseRDAPDomainLockSpellingsAndErrors(t *testing.T) {
	unlocked := `{"objectClassName":"domain","ldhName":"a.com","status":["active","client update prohibited"],"events":[{"eventAction":"expiration","eventDate":"2030-01-02T03:04:05.123Z"}]}`
	obs, err := ParseRDAPDomain([]byte(unlocked))
	if err != nil || obs.TransferLocked {
		t.Fatalf("unlocked domain read as locked (%v)", err)
	}
	if obs.Expires != "2030-01-02T03:04:05Z" {
		t.Errorf("fractional-second date not normalized: %q", obs.Expires)
	}
	epp := `{"objectClassName":"domain","ldhName":"a.com","status":["clientTransferProhibited"]}`
	if obs, err := ParseRDAPDomain([]byte(epp)); err != nil || !obs.TransferLocked || !obs.ExpiryNotListed {
		t.Fatalf("EPP-spelled lock not detected or missing expiry not flagged: %+v %v", obs, err)
	}
	for name, body := range map[string]string{
		"error body": `{"errorCode":404,"title":"Not Found"}`,
		"not domain": `{"objectClassName":"entity","handle":"X"}`,
		"html":       `<html>502 Bad Gateway</html>`,
	} {
		if _, err := ParseRDAPDomain([]byte(body)); err == nil {
			t.Errorf("%s parsed as a domain", name)
		}
	}
}

func TestBootstrapServerFor(t *testing.T) {
	b, err := ParseBootstrap(readFixture(t, "iana-dns-bootstrap-subset.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s, err := b.ServerFor("example.com"); err != nil || s != "https://rdap.verisign.com/com/v1/" {
		t.Errorf("com server = %q, %v", s, err)
	}
	if _, err := b.ServerFor("shop.kg"); !errors.Is(err, ErrRDAPUnavailable) || !strings.Contains(err.Error(), "non-https") {
		t.Errorf("http-only registry not refused: %v", err)
	}
	if _, err := b.ServerFor("example.zz"); !errors.Is(err, ErrRDAPUnavailable) {
		t.Errorf("unknown TLD: %v", err)
	}
}

func TestNormalizeAndRegisteredDomain(t *testing.T) {
	if d, err := NormalizeDomain("Bücher.Example."); err != nil || d != "xn--bcher-kva.example" {
		t.Errorf("IDN = %q, %v", d, err)
	}
	for _, bad := range []string{"", "localhost", "http://a.com", "a.com/x", "a b.com", "user@a.com"} {
		if _, err := NormalizeDomain(bad); err == nil {
			t.Errorf("NormalizeDomain(%q) accepted", bad)
		}
	}
	if r := RegisteredDomain("www.shop.example.co.uk"); r != "example.co.uk" {
		t.Errorf("eTLD+1 = %q", r)
	}
}

func TestParseCrtSh(t *testing.T) {
	certs, err := ParseCrtSh(readFixture(t, "crtsh-example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) == 0 {
		t.Fatal("no certificates parsed")
	}
	seen := map[int64]bool{}
	for _, c := range certs {
		if seen[c.ID] {
			t.Fatalf("crt.sh id %d not deduplicated", c.ID)
		}
		seen[c.ID] = true
		if !strings.HasSuffix(c.NotAfter, "Z") || c.Serial == "" || len(c.Names) == 0 {
			t.Errorf("row not normalized: %+v", c)
		}
	}
	multi := `[{"id":5,"issuer_ca_id":1,"issuer_name":"C=US, O=Let's Encrypt, CN=R11","common_name":"a.example.com","name_value":"a.example.com\nB.example.com","not_before":"2026-01-01T00:00:00","not_after":"2026-04-01T00:00:00","serial_number":"00AB"},
	           {"id":5,"issuer_ca_id":1,"issuer_name":"C=US, O=Let's Encrypt, CN=R11","common_name":"a.example.com","name_value":"*.example.com","not_before":"2026-01-01T00:00:00","not_after":"2026-04-01T00:00:00","serial_number":"00AB"}]`
	got, err := ParseCrtSh([]byte(multi))
	if err != nil || len(got) != 1 {
		t.Fatalf("rows of one id not folded: %v %v", got, err)
	}
	if strings.Join(got[0].Names, ",") != "*.example.com,a.example.com,b.example.com" || got[0].Serial != "00ab" {
		t.Errorf("folded row = %+v", got[0])
	}
	for name, body := range map[string]string{"html": "<html>502</html>", "empty": "", "object": `{"error":"x"}`, "bad id": `[{"id":"x"}]`} {
		if _, err := ParseCrtSh([]byte(body)); err == nil {
			t.Errorf("%s parsed as certificates", name)
		}
	}
	if c, err := ParseCrtSh([]byte("[]")); err != nil || len(c) != 0 {
		t.Errorf("empty array: %v %v", c, err)
	}
}

func TestCoversHostAndDomain(t *testing.T) {
	names := []string{"*.example.com", "example.com"}
	if !CoversHost(names, "www.example.com") || !CoversHost(names, "example.com") {
		t.Error("wildcard or apex not covered")
	}
	if CoversHost(names, "a.b.example.com") {
		t.Error("wildcard must cover one label only")
	}
	if CoversDomain([]string{"notexample.com"}, "example.com") {
		t.Error("suffix without a dot matched")
	}
}

func TestNormalizeSerial(t *testing.T) {
	if DisplaySerial("FFBC4") != "0ffbc4" || NormalizeSerial(DisplaySerial("FFBC4")) != NormalizeSerial("00:0F:FB:C4") {
		t.Error("display serial must pad to even length and still compare equal")
	}
	if NormalizeSerial("00:AB:cd") != "abcd" || NormalizeSerial("0") != "0" {
		t.Error("serial normalization")
	}
}

func TestCrtShURL(t *testing.T) {
	if got, want := crtShURL(12345678901), "https://crt.sh/?id=12345678901"; got != want {
		t.Fatalf("crtShURL = %q, want %q", got, want)
	}
}
