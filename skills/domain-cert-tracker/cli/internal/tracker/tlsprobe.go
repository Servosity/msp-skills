// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
)

// oidMustStaple is the TLS Feature extension (RFC 7633); value 5 is
// status_request, i.e. OCSP must-staple.
var oidMustStaple = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 24}

// ChainCert summarizes one certificate in the served chain.
type ChainCert struct {
	Subject  string `json:"subject"`
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
}

// TLSObs is what one TLS handshake showed.
type TLSObs struct {
	Host          string      `json:"host"`
	Port          int         `json:"port"`
	Address       string      `json:"address"`
	TLSVersion    string      `json:"tls_version"`
	CipherSuite   string      `json:"cipher_suite"`
	Subject       string      `json:"subject"`
	Issuer        string      `json:"issuer"`
	IssuerOrg     string      `json:"issuer_org,omitempty"`
	Serial        string      `json:"serial"`
	NotBefore     string      `json:"not_before"`
	NotAfter      string      `json:"not_after"`
	DaysLeft      int         `json:"days_left"`
	Expired       bool        `json:"expired"`
	SANs          []string    `json:"sans"`
	IPSANs        []string    `json:"ip_sans,omitempty"`
	KeyType       string      `json:"key_type"`
	KeyBits       int         `json:"key_bits,omitempty"`
	HostnameMatch bool        `json:"hostname_match"`
	Trusted       bool        `json:"trusted"`
	VerifyError   string      `json:"verify_error,omitempty"`
	SelfSigned    bool        `json:"self_signed"`
	MustStaple    bool        `json:"ocsp_must_staple"`
	OCSPStapled   bool        `json:"ocsp_stapled"`
	Chain         []ChainCert `json:"chain"`
}

// TLSProber performs live handshakes through a guard.
type TLSProber struct {
	Guard *netguard.Guard
	// Roots overrides the system trust store (tests).
	Roots *x509.CertPool
	Now   func() time.Time
}

// Probe handshakes with host:port. The connection goes through the guard, so
// a non-public address or a port outside the allowed set is refused before
// any packet is sent. The handshake verifies the chain and host name as
// usual; when that verification fails, the certificates the server presented
// are taken from the verification error and reported (no application data
// is ever sent), so an expired, self-signed or wrong-host certificate is
// still reported instead of only failing.
func (p *TLSProber) Probe(ctx context.Context, host string, port int) (TLSObs, error) {
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	g := p.Guard
	if g == nil {
		g = netguard.New()
	}
	conn, err := g.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return TLSObs{}, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(now().Add(20 * time.Second))
	}
	cfg := &tls.Config{
		RootCAs:    p.Roots,
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		Time:       now,
	}
	tc := tls.Client(conn, cfg)
	var peers []*x509.Certificate
	if err := tc.HandshakeContext(ctx); err != nil {
		var cve *tls.CertificateVerificationError
		if !errors.As(err, &cve) {
			return TLSObs{}, fmt.Errorf("TLS handshake with %s:%d: %w", host, port, err)
		}
		peers = cve.UnverifiedCertificates
	}
	cs := tc.ConnectionState()
	if peers == nil {
		peers = cs.PeerCertificates
	}
	if len(peers) == 0 {
		return TLSObs{}, fmt.Errorf("TLS handshake with %s:%d: no certificate presented", host, port)
	}
	leaf := peers[0]
	t := now().UTC()
	obs := TLSObs{
		Host:        host,
		Port:        port,
		Address:     conn.RemoteAddr().String(),
		TLSVersion:  tls.VersionName(cs.Version),
		CipherSuite: tls.CipherSuiteName(cs.CipherSuite),
		Subject:     leaf.Subject.String(),
		Issuer:      leaf.Issuer.String(),
		Serial:      DisplaySerial(leaf.SerialNumber.Text(16)),
		NotBefore:   leaf.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:    leaf.NotAfter.UTC().Format(time.RFC3339),
		DaysLeft:    DaysUntil(t, leaf.NotAfter),
		Expired:     t.After(leaf.NotAfter),
		SANs:        append([]string{}, leaf.DNSNames...),
		OCSPStapled: len(cs.OCSPResponse) > 0,
		Chain:       []ChainCert{},
	}
	if len(leaf.Issuer.Organization) > 0 {
		obs.IssuerOrg = leaf.Issuer.Organization[0]
	}
	for _, ip := range leaf.IPAddresses {
		obs.IPSANs = append(obs.IPSANs, ip.String())
	}
	obs.KeyType, obs.KeyBits = keyInfo(leaf)
	for _, ext := range leaf.Extensions {
		if ext.Id.Equal(oidMustStaple) {
			var features []int
			if _, err := asn1.Unmarshal(ext.Value, &features); err == nil {
				for _, f := range features {
					if f == 5 {
						obs.MustStaple = true
					}
				}
			}
		}
	}
	for _, c := range peers {
		obs.Chain = append(obs.Chain, ChainCert{Subject: c.Subject.String(), Issuer: c.Issuer.String(), NotAfter: c.NotAfter.UTC().Format(time.RFC3339)})
	}
	// CheckSignatureFrom would demand the CA flag, which most self-signed
	// server certificates lack; check the signature over the TBS directly.
	obs.SelfSigned = bytes.Equal(leaf.RawIssuer, leaf.RawSubject) &&
		leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil
	obs.HostnameMatch = leaf.VerifyHostname(host) == nil
	inter := x509.NewCertPool()
	for _, c := range peers[1:] {
		inter.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: p.Roots, Intermediates: inter, CurrentTime: t}); err != nil {
		obs.VerifyError = err.Error()
	} else {
		obs.Trusted = true
	}
	return obs, nil
}

func keyInfo(c *x509.Certificate) (string, int) {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA", k.N.BitLen()
	case *ecdsa.PublicKey:
		return "ECDSA " + k.Curve.Params().Name, k.Curve.Params().BitSize
	case ed25519.PublicKey:
		return "Ed25519", 256
	default:
		return c.PublicKeyAlgorithm.String(), 0
	}
}

// DaysUntil returns whole days from now to t, negative once t has passed.
func DaysUntil(now, t time.Time) int {
	d := t.Sub(now)
	days := int(d / (24 * time.Hour))
	if d < 0 && d%(24*time.Hour) != 0 {
		days--
	}
	return days
}
