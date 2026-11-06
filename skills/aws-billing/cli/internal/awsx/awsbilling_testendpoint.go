// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build awsbilling_blackbox

package awsx

// Compiled only into the binaries the black-box suite builds
// (internal/awsbillingblackbox, go build -tags awsbilling_blackbox); a
// production build has no such file, so blackboxTransport stays nil there.
//
// AWS_BILLING_BLACKBOX_AWS_ENDPOINT names ONE fake AWS origin: https, a
// loopback IP literal and an explicit port, nothing else (no userinfo, path,
// query or fragment); AWS_BILLING_BLACKBOX_AWS_CA names the PEM file of its
// certificate. Every https connection to port 443 then dials the fake, which
// answers for every AWS host: the SDK, the pin (awsEndpointAllowed) and the
// credential kit all see the real AWS host names, so the production origin
// rules are what the suite exercises. Nothing else changes: a custom endpoint
// is still refused, and so is every redirect and every request under
// --dry-run.

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// BlackboxEndpointEnv names the one loopback fake AWS origin a black-box build
// dials; BlackboxCAEnv names the PEM file of the fake's certificate.
const (
	BlackboxEndpointEnv = "AWS_BILLING_BLACKBOX_AWS_ENDPOINT"
	BlackboxCAEnv       = "AWS_BILLING_BLACKBOX_AWS_CA"
)

func init() {
	addr, ok := blackboxOrigin(os.Getenv(BlackboxEndpointEnv))
	if !ok {
		return
	}
	pem, err := os.ReadFile(os.Getenv(BlackboxCAEnv))
	if err != nil {
		return
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return
	}
	blackboxTransport = func(tr *http.Transport) {
		dial := tr.DialContext
		tr.DialContext = func(ctx context.Context, network, a string) (net.Conn, error) {
			if strings.HasSuffix(a, ":443") {
				a = addr
			}
			return dial(ctx, network, a)
		}
		tr.TLSClientConfig.RootCAs = roots
		tr.TLSClientConfig.ServerName = "example.com" // the httptest certificate's name
		// The pin (pinTransport's proxy hook) still runs; its proxy answer is
		// dropped, since the fake is on loopback.
		pin := tr.Proxy
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			if _, err := pin(req); err != nil {
				return nil, err
			}
			return nil, nil
		}
	}
}

// blackboxOrigin parses the fake's origin; anything but https://<loopback
// IP>:<port>[/] names no endpoint at all.
func blackboxOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return "", false
	}
	return u.Host, true
}
