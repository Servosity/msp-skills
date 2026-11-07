// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): publish pass 3 round 1 review inputs.

package tracker

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/netguard"
)

// certwatch_db fnc/web_apis.fnc: with no match parameter a value holding a
// hyphen gets match=Single, which returns only identities equal to the query,
// so every subdomain certificate of acme-dental.com (or an xn-- name) is lost.
func TestCTSearchAsksForSubdomainsOfAHyphenatedDomain(t *testing.T) {
	g := &netguard.Guard{
		Resolve: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("test dialer: no network")
		},
	}
	w := NewWeb(g, 1000, 1<<20)
	w.Attempts = 1
	rec := &urlRecorder{next: w.HTTP.Transport}
	w.HTTP.Transport = rec
	for _, domain := range []string{"acme-dental.com", "example.com", "xn--bcher-kva.example"} {
		rec.urls = nil
		_, _ = (&CT{Web: w}).Search(context.Background(), domain)
		if len(rec.urls) != 1 || !strings.Contains(rec.urls[0], "match=ILIKE") {
			t.Fatalf("%s: requested %v, want match=ILIKE", domain, rec.urls)
		}
	}
}

// RFC 9083 section 10.2.2: "locked: Changes to the object instance cannot be
// made". A locked domain is transfer locked; it must never read as unlocked.
func TestParseRDAPLockedStatusIsATransferLock(t *testing.T) {
	obs, err := ParseRDAPDomain([]byte(`{"objectClassName":"domain","ldhName":"locked.example","status":["active","locked"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !obs.TransferLocked || !obs.LockKnown() {
		t.Fatalf("status locked: %+v", obs)
	}
}

// web_apis.fnc identity search covers rfc822Name: an S/MIME address is not a
// host name, and a certificate that names only addresses is not this domain's.
func TestParseCrtShDropsEmailIdentities(t *testing.T) {
	certs, err := ParseCrtSh([]byte(`[{"id":1,"issuer_ca_id":2,"issuer_name":"O=X","common_name":"jane@mail.example.com","name_value":"jane@mail.example.com\nwww.example.com","not_before":"2026-01-01T00:00:00","not_after":"2027-01-01T00:00:00","serial_number":"01"},
{"id":3,"issuer_ca_id":2,"issuer_name":"O=X","common_name":"Jane Doe","name_value":"jane@mail.example.com","not_before":"2026-01-01T00:00:00","not_after":"2027-01-01T00:00:00","serial_number":"02"}]`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range certs {
		if strings.Contains(strings.Join(c.Names, " "), "@") {
			t.Fatalf("an email address reads as a host name: %+v", c)
		}
		if c.ID == 3 && CoversDomain(c.Names, "example.com") {
			t.Fatalf("a certificate naming only an address covers the domain: %+v", c)
		}
	}
}
