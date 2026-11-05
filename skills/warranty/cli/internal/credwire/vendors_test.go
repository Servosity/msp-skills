// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring): the three vendors on their real
// host names, so the real binaries run under the real Origins declarations. The
// binaries reach them through HTTPS_PROXY: the proxy records each CONNECT, ends
// TLS with a certificate this run made (WARRANTY_CA_CERT trusts it) and answers
// from the documented response shapes.

package credwire

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	hostDell        = "apigtwb2c.us.dell.com"
	hostLenovo      = "supportapi.lenovo.com"
	hostAppleOAuth  = "account.apple.com"
	hostAppleAPI    = "api-business.apple.com"
	hostGateway     = "dell-gateway.example"
	stubDellToken   = "dell-bearer-AAAA1111"
	stubAppleToken  = "apple-grant-BBBB2222"
	stubDellClient  = "l7xx5f3c9a1b2d7Q2X"
	stubDellSecret  = "dell-secret-7Y6Z5X4W"
	stubLenovoKey   = "lenovo-key-3C2B1A0Z"
	stubAppleClient = "BUSINESSAPI.9703f56c-10ce-4876-8f59-e78e5e23a152"
	stubAppleUnique = "9703f56c-10ce-4876-8f59-e78e5e23a152"
	stubAppleKeyID  = "kid-test-01"
)

type seenRequest struct {
	host, method, path, query, auth, clientID, body string
}

// vendors is the fake of all three vendors behind one proxy.
type vendors struct {
	*httptest.Server
	caFile string
	tlsCfg *tls.Config

	mu   sync.Mutex
	seen []seenRequest

	dellToken, appleToken func() (int, string)
	dell, lenovo, apple   func(r *http.Request) (int, string)
}

func newVendors(t *testing.T, s sandbox) *vendors {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake vendors"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: hostDell}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), DNSNames: []string{hostDell, hostLenovo, hostAppleOAuth, hostAppleAPI, hostGateway}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	v := &vendors{
		caFile: s.writePrivate(t, "fake-vendor-ca.pem", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))),
		tlsCfg: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}},
		dellToken: func() (int, string) {
			return 200, `{"access_token":"` + stubDellToken + `","token_type":"Bearer","expires_in":3600}`
		},
		appleToken: func() (int, string) {
			return 200, `{"access_token":"` + stubAppleToken + `","token_type":"Bearer","expires_in":3600}`
		},
		dell: func(r *http.Request) (int, string) {
			return 200, `[{"id":1,"serviceTag":"` + r.URL.Query().Get("servicetags") + `","shipDate":"2021-03-14T05:00:00Z","productLineDescription":"LATITUDE 5420","invalid":false,"duplicated":false,"entitlements":[{"itemNumber":"997-8367","startDate":"2021-03-14T05:00:00Z","endDate":"2030-12-01T04:59:59.999Z","entitlementType":"INITIAL","serviceLevelCode":"ND","serviceLevelDescription":"Next Business Day"}]}]`
		},
		lenovo: func(r *http.Request) (int, string) {
			return 200, `{"Serial":"` + r.URL.Query().Get("Serial") + `","Product":"21CBCTO1WW","Warranty":[{"ID":"1EZ","Name":"Base Warranty","Type":"BASE","Start":"2020-01-20T00:00:00","End":"2030-01-19T00:00:00"}],"Contract":[]}`
		},
		apple: func(r *http.Request) (int, string) {
			return 200, `{"data":[{"type":"appleCareCoverage","id":"x1","attributes":{"status":"ACTIVE","paymentType":"ABE_SUBSCRIPTION","description":"AppleCare+ for Business","startDateTime":"2024-05-01T00:00:00Z","endDateTime":"2030-05-01T00:00:00Z","isRenewable":true}}],"links":{"self":"x"},"meta":{"paging":{"limit":1000}}}`
		},
	}
	v.Server = httptest.NewServer(http.HandlerFunc(v.connect))
	t.Cleanup(v.Close)
	return v
}

func (v *vendors) connect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
		return
	}
	host, _, _ := net.SplitHostPort(r.Host)
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\n\r\n")
	tc := tls.Server(conn, v.tlsCfg)
	defer tc.Close()
	br := bufio.NewReader(tc)
	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		body, _ := io.ReadAll(req.Body)
		v.mu.Lock()
		v.seen = append(v.seen, seenRequest{host, req.Method, req.URL.Path, req.URL.RawQuery, req.Header.Get("Authorization"), req.Header.Get("ClientID"), string(body)})
		v.mu.Unlock()
		code, out := v.answer(host, req)
		resp := &http.Response{StatusCode: code, ProtoMajor: 1, ProtoMinor: 1, Close: true, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(out)), ContentLength: int64(len(out)), Request: req}
		_ = resp.Write(tc)
		return
	}
}

func (v *vendors) answer(host string, r *http.Request) (int, string) {
	switch {
	case host == hostDell && r.URL.Path == "/auth/oauth/v2/token", host == hostGateway && r.URL.Path == "/token":
		return v.dellToken()
	case host == hostDell && strings.HasPrefix(r.URL.Path, "/PROD/sbil/eapi/v5/"):
		return v.dell(r)
	case host == hostLenovo && strings.HasPrefix(r.URL.Path, "/v2.5/"):
		return v.lenovo(r)
	case host == hostAppleOAuth && r.URL.Path == "/auth/oauth2/token":
		return v.appleToken()
	case host == hostAppleAPI && strings.HasPrefix(r.URL.Path, "/v1/"):
		return v.apple(r)
	}
	return 599, `{"error":"unexpected request"}`
}

// requests is every request a vendor received, for one host ("" for all).
func (v *vendors) requests(host string) []seenRequest {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []seenRequest
	for _, r := range v.seen {
		if host == "" || r.host == host {
			out = append(out, r)
		}
	}
	return out
}

// carried reports whether any request to host held secret anywhere.
func (v *vendors) carried(host, secret string) bool {
	for _, r := range v.requests(host) {
		if strings.Contains(r.path+r.query+r.auth+r.clientID+r.body, secret) {
			return true
		}
	}
	return false
}

func (v *vendors) env() []string {
	return []string{"HTTPS_PROXY=" + v.URL, "NO_PROXY=", "WARRANTY_CA_CERT=" + v.caFile}
}

func appleKeyPEM(t *testing.T) (string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), der
}

// allVendorEnv is every vendor's credentials, with the Apple key in a file.
func allVendorEnv(t *testing.T, s sandbox, v *vendors) []string {
	t.Helper()
	keyPEM, _ := appleKeyPEM(t)
	keyFile := s.writePrivate(t, "apple.p8", keyPEM)
	return s.env(append(v.env(),
		"WARRANTY_DELL_CLIENT_ID="+stubDellClient,
		"WARRANTY_DELL_CLIENT_SECRET="+stubDellSecret,
		"WARRANTY_LENOVO_CLIENT_KEY="+stubLenovoKey,
		"WARRANTY_APPLE_CLIENT_ID="+stubAppleClient,
		"WARRANTY_APPLE_KEY_ID="+stubAppleKeyID,
		"WARRANTY_APPLE_PRIVATE_KEY="+keyFile,
	)...)
}
