// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

// fakeDNS answers from maps; a name in fail returns a SERVFAIL-style error.
type fakeDNS struct {
	ns   map[string][]string
	mx   map[string][]MX
	txt  map[string][]string
	raw  map[string][][]byte // key: name|type
	fail map[string]bool
}

func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *fakeDNS) NS(ctx context.Context, n string) ([]string, error) {
	if f.fail[n] {
		return nil, errors.New("server misbehaving")
	}
	if v, ok := f.ns[n]; ok {
		return v, nil
	}
	return nil, notFound(n)
}
func (f *fakeDNS) MX(ctx context.Context, n string) ([]MX, error) {
	if f.fail[n] {
		return nil, errors.New("server misbehaving")
	}
	if v, ok := f.mx[n]; ok {
		return v, nil
	}
	return nil, notFound(n)
}
func (f *fakeDNS) TXT(ctx context.Context, n string) ([]string, error) {
	if f.fail[n] {
		return nil, errors.New("server misbehaving")
	}
	if v, ok := f.txt[n]; ok {
		return v, nil
	}
	return nil, notFound(n)
}
func (f *fakeDNS) Raw(ctx context.Context, n string, qtype uint16) ([][]byte, error) {
	if f.fail[n] {
		return nil, errors.New("server misbehaving")
	}
	return f.raw[n+"|"+typeName(qtype)], nil
}

func caaRData(flags byte, tag, value string) []byte {
	return append(append([]byte{flags, byte(len(tag))}, tag...), value...)
}

func TestCheckDNSRecordsAndFindings(t *testing.T) {
	f := &fakeDNS{
		ns:  map[string][]string{"client.test": {"NS2.HOST.TEST.", "ns1.host.test."}},
		mx:  map[string][]MX{"client.test": {{Host: "mx2.mail.test.", Pref: 20}, {Host: "MX1.mail.test.", Pref: 10}}},
		txt: map[string][]string{"client.test": {"v=spf1 include:_spf.mail.test -all", "v=spf1 -all", "v=spf10 nope", "google-site-verification=x"}, "_dmarc.client.test": {"v=DMARC1; p=none; rua=mailto:d@client.test"}},
		raw: map[string][][]byte{"client.test|CAA": {caaRData(0, "issue", "letsencrypt.org"), caaRData(0, "iodef", "mailto:sec@client.test")}, "client.test|DNSKEY": {{1}, {2}}},
	}
	o := CheckDNS(context.Background(), f, "client.test")
	if !o.OK() {
		t.Fatalf("errors: %v", o.Errors)
	}
	if strings.Join(o.NS, ",") != "ns1.host.test,ns2.host.test" || o.MX[0].Host != "mx1.mail.test" || len(o.SPF) != 2 || o.DMARCPolicy != "none" {
		t.Fatalf("records: %+v", o)
	}
	// CAA comes back sorted (tag, then value), so record order in the DNS
	// answer cannot read as a change between sweeps.
	if len(o.CAA) != 2 || o.CAA[0].Tag != "iodef" || o.CAA[1].Tag != "issue" || o.CAA[1].Value != "letsencrypt.org" || o.CAASource != "client.test" || o.DNSKEY != 2 {
		t.Fatalf("CAA/DNSSEC: %+v", o)
	}
	joined := strings.Join(o.Findings, "|")
	for _, want := range []string{"more than one SPF record", "DMARC policy is none"} {
		if !strings.Contains(joined, want) {
			t.Errorf("finding %q missing from %v", want, o.Findings)
		}
	}
}

func TestCheckDNSCAAClimbsToRegisteredDomainAndFailuresAreErrors(t *testing.T) {
	f := &fakeDNS{
		ns:   map[string][]string{"shop.client.co.uk": {}},
		raw:  map[string][][]byte{"client.co.uk|CAA": {caaRData(128, "issue", ";")}, "co.uk|CAA": {caaRData(0, "issue", "should-not-be-read.test")}},
		fail: map[string]bool{"_dmarc.shop.client.co.uk": true},
	}
	o := CheckDNS(context.Background(), f, "shop.client.co.uk")
	if o.CAASource != "client.co.uk" || len(o.CAA) != 1 || o.CAA[0].Flags != 128 {
		t.Fatalf("CAA climb: source %q %+v", o.CAASource, o.CAA)
	}
	if o.OK() || o.Errors["dmarc"] == "" {
		t.Fatalf("a failed DMARC lookup must be an error, not an empty answer: %+v", o.Errors)
	}
	for _, f := range o.Findings {
		if strings.Contains(f, "DMARC") {
			t.Errorf("finding %q drawn from a failed lookup", f)
		}
	}
	nx := CheckDNS(context.Background(), &fakeDNS{}, "gone.test")
	if !nx.OK() || len(nx.NS) != 0 {
		t.Fatalf("NXDOMAIN must read as empty, not failed: %+v", nx.Errors)
	}
}

func TestParseCAARejectsShortData(t *testing.T) {
	if _, ok := parseCAA([]byte{0}); ok {
		t.Error("1-byte CAA accepted")
	}
	if _, ok := parseCAA([]byte{0, 9, 'i'}); ok {
		t.Error("tag length past the end accepted")
	}
}

// TestRawQueryAgainstLocalServer drives rawQuery's wire path with a UDP
// server built from dnsmessage.
func TestRawQueryAgainstLocalServer(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var p dnsmessage.Parser
			h, err := p.Start(buf[:n])
			if err != nil {
				continue
			}
			q, _ := p.Question()
			b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: h.ID, Response: true})
			_ = b.StartQuestions()
			_ = b.Question(q)
			_ = b.StartAnswers()
			if uint16(q.Type) == TypeCAA {
				_ = b.UnknownResource(dnsmessage.ResourceHeader{Name: q.Name, Type: q.Type, Class: dnsmessage.ClassINET, TTL: 60},
					dnsmessage.UnknownResource{Type: q.Type, Data: caaRData(0, "issue", "pki.goog")})
			}
			out, _ := b.Finish()
			_, _ = pc.WriteTo(out, addr)
		}
	}()
	rd, err := rawQuery(context.Background(), pc.LocalAddr().String(), "client.test", TypeCAA)
	if err != nil {
		t.Fatal(err)
	}
	if len(rd) != 1 {
		t.Fatalf("answers = %d", len(rd))
	}
	if c, ok := parseCAA(rd[0]); !ok || c.Value != "pki.goog" {
		t.Fatalf("CAA = %+v", c)
	}
	if rd, err := rawQuery(context.Background(), pc.LocalAddr().String(), "client.test", TypeDNSKEY); err != nil || len(rd) != 0 {
		t.Fatalf("DNSKEY: %v %v", rd, err)
	}
}

// A query too long for the two-byte TCP length prefix is refused before any
// dial, so the uint16 frame length cannot wrap (gosec G115).
func TestExchangeRefusesOversizedQuery(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		_, err := exchange(context.Background(), network, "127.0.0.1:1", make([]byte, 65536))
		if err == nil || !strings.Contains(err.Error(), "65535-byte message limit") {
			t.Fatalf("%s: err = %v, want the 65535-byte limit refusal", network, err)
		}
	}
}
