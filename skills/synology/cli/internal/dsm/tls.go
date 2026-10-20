// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"synology-pp-cli/internal/cliutil"
)

// Most client NAS units present DSM's self-signed certificate. Two opt-ins:
//
//   - pin: trust exactly one certificate by the SHA-256 of its DER bytes. The
//     chain is not checked, the fingerprint is. This is the safe choice.
//   - skip: trust anything. Only for a network the operator controls.
//
// Pinning wins when both are set.

// NormalizeFingerprint lowercases a hex SHA-256 fingerprint and strips colons
// and spaces, so "AB:CD:..." and "abcd..." compare equal.
func NormalizeFingerprint(fp string) string {
	return strings.ToLower(strings.NewReplacer(":", "", " ", "", "sha256/", "").Replace(strings.TrimSpace(fp)))
}

// ApplyTLS adjusts cfg (cloned by the caller) for a pin or a skip. It returns
// an error for a pin that is not a 64-character hex SHA-256.
func ApplyTLS(cfg *tls.Config, pin string, skip bool) error {
	supplied := strings.TrimSpace(pin) != ""
	pin = NormalizeFingerprint(pin)
	if supplied && pin == "" {
		return errors.New("TLS fingerprint is empty after normalization (got only separators or a prefix)")
	}
	if pin != "" {
		if b, err := hex.DecodeString(pin); err != nil || len(b) != sha256.Size {
			return fmt.Errorf("TLS fingerprint %q is not a SHA-256 hex digest", pin)
		}
		// The pin replaces chain verification: VerifyPeerCertificate checks the
		// leaf against the fingerprint the operator recorded.
		cfg.InsecureSkipVerify = true // #nosec G402 -- replaced by the fingerprint check below
		// #nosec G123 -- every caller passes a fresh tls.Config and nothing in this module sets ClientSessionCache, so the client never resumes a session and this pin check runs on every handshake
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("NAS presented no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			if hex.EncodeToString(sum[:]) != pin {
				return &PinMismatchError{Presented: hex.EncodeToString(sum[:]), Pinned: pin}
			}
			return nil
		}
		return nil
	}
	if skip {
		cfg.InsecureSkipVerify = true // #nosec G402 -- explicit operator opt-in for a self-signed NAS certificate
	}
	return nil
}

// ErrCertificatePinMismatch is what errors.Is matches for a NAS certificate
// that is not the pinned one.
var ErrCertificatePinMismatch = errors.New("the NAS certificate does not match the pinned SHA-256 fingerprint")

// PinMismatchError is a NAS that presented a certificate other than the
// pinned one: a renewed certificate (DSM's self-signed one, or Let's Encrypt
// every ~90 days) or another host. Both fingerprints are hex SHA-256 digests
// computed here, never text the server chose.
type PinMismatchError struct {
	Presented, Pinned string
}

func (e *PinMismatchError) Error() string {
	return fmt.Sprintf("NAS certificate fingerprint %s does not match the pinned %s", e.Presented, e.Pinned)
}

// Is makes errors.Is(err, ErrCertificatePinMismatch) true.
func (e *PinMismatchError) Is(target error) bool { return target == ErrCertificatePinMismatch }

// PinMismatchHint is the remedy every surface (transport errors, fleet sync
// rows, live commands, doctor) prints for a pin mismatch. It never suggests
// --pin-cert: that trusts whatever certificate is presented right now, and a
// pin mismatch is also exactly what a man-in-the-middle looks like. The
// operator checks the presented fingerprint in DSM first, then pins that value.
const PinMismatchHint = "verify that fingerprint in DSM (Control Panel > Security > Certificate); if it matches (the NAS renewed its certificate), re-pin it with 'synology-cli fleet add <name> --tls-fingerprint <the value you checked>' for a registered NAS (keeps its URL, settings and mirror), or set SYNOLOGY_TLS_FINGERPRINT to that value for the single NAS; if it does not match, do not re-pin: something else is answering at the NAS address"

// RefuseAllTLS returns a config that fails every TLS handshake with err: a
// pin the operator asked for but mistyped must not silently fall back to
// ordinary certificate validation.
func RefuseAllTLS(err error) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- every handshake is rejected below
		// #nosec G123 -- a fresh config with no ClientSessionCache whose callback rejects every handshake, so no session ever exists to resume
		VerifyPeerCertificate: func([][]byte, [][]*x509.Certificate) error {
			return fmt.Errorf("TLS refused: %w", err)
		},
	}
}

// EnvTLS reads SYNOLOGY_TLS_FINGERPRINT and SYNOLOGY_SKIP_TLS_VERIFY through
// cliutil.EnvOverride, so an unresolved ${user_config...} placeholder that an
// MCPB host left for a skipped optional prompt reads as unset instead of a pin
// that refuses every certificate. HAND-FIX(mcp-http-token-placeholder).
func EnvTLS() (pin string, skip bool) {
	v := strings.ToLower(strings.TrimSpace(cliutil.EnvOverride("SYNOLOGY_SKIP_TLS_VERIFY")))
	return strings.TrimSpace(cliutil.EnvOverride("SYNOLOGY_TLS_FINGERPRINT")), v == "1" || v == "true" || v == "yes" || v == "on"
}

// FetchFingerprint connects to baseURL's host without verifying the chain and
// returns the SHA-256 of the leaf certificate, for pinning at registration.
func FetchFingerprint(ctx context.Context, baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid NAS URL %s", DisplayAddress(baseURL))
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("certificate pinning needs an https:// URL, got %s", DisplayAddress(baseURL))
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	d := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config:    &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()}, // #nosec G402 -- reading the certificate to pin it, no data is sent
	}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", host, err)
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", errors.New("NAS presented no certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}
