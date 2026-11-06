// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/netip"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root", Organization: []string{"Test CA"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * 365 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return testCA{cert: c, key: key, pool: pool}
}

// leaf issues (or self-signs when ca is nil) a certificate for names.
func (ca *testCA) leaf(t *testing.T, serial int64, names []string, notBefore, notAfter time.Time, mustStaple bool) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if mustStaple {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: oidMustStaple, Value: []byte{0x30, 0x03, 0x02, 0x01, 0x05}}}
	}
	parent, signer := tmpl, any(key)
	if ca != nil {
		parent, signer = ca.cert, ca.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func serveTLS(t *testing.T, cert tls.Certificate) int {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = c.(*tls.Conn).Handshake()
				c.Close()
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// testProber reaches 127.0.0.1 only because the test allows it; production
// guards never set AllowAddr.
func testProber(port int, roots *x509.CertPool) *TLSProber {
	g := netguard.New().WithPorts(port)
	g.AllowAddr = func(a netip.Addr) bool { return a.IsLoopback() }
	g.Resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	return &TLSProber{Guard: g, Roots: roots}
}

func TestProbeReportsCertificateStates(t *testing.T) {
	ca := newTestCA(t)
	now := time.Now()
	cases := []struct {
		name                     string
		cert                     tls.Certificate
		trusted, hostOK, expired bool
		selfSigned, mustStaple   bool
		maxDays, minDays         int
	}{
		{"good", ca.leaf(t, 10, []string{"www.client.test"}, now.Add(-time.Hour), now.Add(60*24*time.Hour), true), true, true, false, false, true, 60, 59},
		{"expiring", ca.leaf(t, 11, []string{"www.client.test"}, now.Add(-80*24*time.Hour), now.Add(5*24*time.Hour), false), true, true, false, false, false, 5, 4},
		{"expired", ca.leaf(t, 12, []string{"www.client.test"}, now.Add(-90*24*time.Hour), now.Add(-2*24*time.Hour), false), false, true, true, false, false, -2, -3},
		{"wrong host", ca.leaf(t, 13, []string{"other.client.test"}, now.Add(-time.Hour), now.Add(30*24*time.Hour), false), true, false, false, false, false, 30, 29},
		{"self-signed", (*testCA)(nil).leaf(t, 14, []string{"www.client.test"}, now.Add(-time.Hour), now.Add(30*24*time.Hour), false), false, true, false, true, false, 30, 29},
	}
	for _, c := range cases {
		port := serveTLS(t, c.cert)
		obs, err := testProber(port, ca.pool).Probe(context.Background(), "www.client.test", port)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if obs.Trusted != c.trusted || obs.HostnameMatch != c.hostOK || obs.Expired != c.expired || obs.SelfSigned != c.selfSigned || obs.MustStaple != c.mustStaple {
			t.Errorf("%s: trusted=%v host=%v expired=%v self=%v staple=%v (verify %q)", c.name, obs.Trusted, obs.HostnameMatch, obs.Expired, obs.SelfSigned, obs.MustStaple, obs.VerifyError)
		}
		if obs.DaysLeft > c.maxDays || obs.DaysLeft < c.minDays {
			t.Errorf("%s: days_left %d, want %d..%d", c.name, obs.DaysLeft, c.minDays, c.maxDays)
		}
		// A certificate that fails verification ends the handshake early; the
		// negotiated version and suite must still be reported.
		if obs.TLSVersion == "" || obs.CipherSuite == "" {
			t.Errorf("%s: tls_version %q cipher_suite %q", c.name, obs.TLSVersion, obs.CipherSuite)
		}
		if obs.KeyType != "ECDSA P-256" || obs.Serial == "" || len(obs.Chain) != 1 {
			t.Errorf("%s: key %q serial %q chain %d", c.name, obs.KeyType, obs.Serial, len(obs.Chain))
		}
	}
}

func TestProbeRefusesLoopbackAndPortsWithoutTestAllowance(t *testing.T) {
	ca := newTestCA(t)
	port := serveTLS(t, ca.leaf(t, 20, []string{"localhost"}, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), false))
	p := &TLSProber{Guard: netguard.New().WithPorts(port)}
	if _, err := p.Probe(context.Background(), "127.0.0.1", port); !errors.Is(err, netguard.ErrRefused) {
		t.Fatalf("loopback probe: %v, want refused", err)
	}
	p = &TLSProber{Guard: netguard.New()}
	if _, err := p.Probe(context.Background(), "93.184.215.14", 8443); !errors.Is(err, netguard.ErrRefused) {
		t.Fatalf("non-443 probe: %v, want refused", err)
	}
	if _, err := p.Probe(context.Background(), "169.254.169.254", 443); !errors.Is(err, netguard.ErrRefused) {
		t.Fatalf("metadata probe: %v, want refused", err)
	}
}

func TestDaysUntil(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		t    time.Time
		want int
	}{
		{now.Add(36 * time.Hour), 1},
		{now.Add(24 * time.Hour), 1},
		{now.Add(-1 * time.Hour), -1},
		{now.Add(-24 * time.Hour), -1},
		{now.Add(-25 * time.Hour), -2},
	} {
		if got := DaysUntil(now, c.t); got != c.want {
			t.Errorf("DaysUntil(%v) = %d, want %d", c.t.Sub(now), got, c.want)
		}
	}
}
