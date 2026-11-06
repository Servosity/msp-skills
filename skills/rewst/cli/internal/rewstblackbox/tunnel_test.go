// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// homoglyphHost is the homoglyph of api.rewst.io (a Cyrillic s) the fake
// redirects bbRedirIDN to, as the transport dials it (Punycode).
const homoglyphHost = "api.xn--rewt-i9d.io"

// suiteCert is the one certificate every fake and the proxy trap present: a
// self-signed certificate for the loopback fakes and for homoglyphHost, so
// the trap can open the TLS a CONNECT tunnel carries and read the request
// inside (REWST_CA_CERT trusts it). Without it the trap sees only the CONNECT
// line, which names the host and carries no credential.
var suiteCert = sync.OnceValue(func() tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "rewst black-box fake"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost", "example.com", homoglyphHost},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
})

// tunnels are the requests the trap read inside a CONNECT to homoglyphHost:
// headers, URL and body.
var tunnels struct {
	sync.Mutex
	reqs []string
}

func tunnelRequests() []string {
	tunnels.Lock()
	defer tunnels.Unlock()
	return append([]string{}, tunnels.reqs...)
}

// trapHandler records every request a child sends to a non-loopback host
// and refuses it. A CONNECT to homoglyphHost is answered, and the request it
// carries is read and recorded, then refused.
func trapHandler(w http.ResponseWriter, r *http.Request) {
	trap.Lock()
	trap.dials = append(trap.dials, r.Method+" "+r.Host+r.URL.Path)
	trap.Unlock()
	hj, ok := w.(http.Hijacker)
	if r.Method != http.MethodConnect || r.Host != homoglyphHost+":443" || !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{suiteCert()}})
	req, err := http.ReadRequest(bufio.NewReader(tc))
	text := "unreadable tunnel: " + fmt.Sprint(err)
	if err == nil {
		body, _ := io.ReadAll(req.Body)
		text = fmt.Sprint(req.Header) + "\n" + req.URL.String() + "\n" + string(body)
	}
	tunnels.Lock()
	tunnels.reqs = append(tunnels.reqs, text)
	tunnels.Unlock()
	_, _ = io.WriteString(tc, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
}

// tunnelLeaks is every held value in any form the fake echoes, found in a
// request the trap read inside a tunnel.
func tunnelLeaks() []string {
	var out []string
	for _, req := range tunnelRequests() {
		for _, s := range secrets() {
			for _, f := range echoForms(s) {
				if strings.Contains(req, f) {
					out = append(out, fmt.Sprintf("%q in %.300q", f, req))
				}
			}
		}
	}
	return out
}
