// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (winrm-transport).

package winrm

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"windows-server-pp-cli/internal/cliutil"
)

// fakeWSMan answers the four WinRS actions the client sends.
type fakeWSMan struct {
	mu        sync.Mutex
	bodies    []string
	stdout    string
	timeouts  int // Receive faults with the timeout code before answering
	authCalls int
	deleted   bool
	signalled bool
}

func (f *fakeWSMan) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCalls++
	if !ok || user != "mspcli" || pass != "s3cret-Pa55" {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	body := string(raw)
	f.bodies = append(f.bodies, body)
	reply := func(inner string) {
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:x="http://schemas.xmlsoap.org/ws/2004/09/transfer"><s:Body>`+inner+`</s:Body></s:Envelope>`)
	}
	switch {
	case strings.Contains(body, actionCreate):
		reply(`<x:ResourceCreated><a:ReferenceParameters><w:SelectorSet><w:Selector Name="ShellId">SHELL-1</w:Selector></w:SelectorSet></a:ReferenceParameters></x:ResourceCreated>`)
	case strings.Contains(body, actionCommand):
		reply(`<rsp:CommandResponse><rsp:CommandId>CMD-1</rsp:CommandId></rsp:CommandResponse>`)
	case strings.Contains(body, actionReceive):
		if f.timeouts > 0 {
			f.timeouts--
			w.WriteHeader(http.StatusInternalServerError)
			reply(`<s:Fault><s:Reason><s:Text>timeout</s:Text></s:Reason><s:Detail><f:WSManFault xmlns:f="http://schemas.microsoft.com/wbem/wsman/1/wsmanfault" Code="2150858793"><f:Message>The WS-Management service cannot complete the operation within the time specified in OperationTimeout.</f:Message></f:WSManFault></s:Detail></s:Fault>`)
			return
		}
		half := len(f.stdout) / 2
		reply(`<rsp:ReceiveResponse><rsp:Stream Name="stdout" CommandId="CMD-1">` + base64.StdEncoding.EncodeToString([]byte(f.stdout[:half])) + `</rsp:Stream><rsp:Stream Name="stdout" CommandId="CMD-1">` + base64.StdEncoding.EncodeToString([]byte(f.stdout[half:])) + `</rsp:Stream><rsp:Stream Name="stderr" CommandId="CMD-1" End="true"></rsp:Stream><rsp:CommandState CommandId="CMD-1" State="http://schemas.microsoft.com/wbem/wsman/1/windows/shell/CommandState/Done"><rsp:ExitCode>0</rsp:ExitCode></rsp:CommandState></rsp:ReceiveResponse>`)
	case strings.Contains(body, actionSignal):
		f.signalled = true
		reply(`<rsp:SignalResponse/>`)
	case strings.Contains(body, actionDelete):
		f.deleted = true
		reply(``)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func endpointFor(t *testing.T, srv *httptest.Server) Endpoint {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	p, _ := strconv.Atoi(port)
	sum := sha256.Sum256(srv.Certificate().Raw)
	return Endpoint{Host: host, Port: p, CertSHA256: hex.EncodeToString(sum[:])}
}

func basic() Auth { return Auth{Method: "basic", Username: "mspcli", Password: "s3cret-Pa55"} }

func TestRunReturnsStdoutAndPassesEnvironment(t *testing.T) {
	f := &fakeWSMan{stdout: `{"ok":true,"data":[1,2,3]}`, timeouts: 2}
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1") // a proxy would break the call
	c, err := New(endpointFor(t, srv), basic(), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Run(context.Background(), "powershell.exe", []string{"-NoProfile", "-EncodedCommand", "QQA="}, map[string]string{"WSPP_ARGS": `{"name":"a<b&c"}`})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Stdout) != f.stdout || res.ExitCode != 0 {
		t.Fatalf("stdout %q exit %d", res.Stdout, res.ExitCode)
	}
	create := f.bodies[0]
	if !strings.Contains(create, `<rsp:Variable Name="WSPP_ARGS">{&#34;name&#34;:&#34;a&lt;b&amp;c&#34;}</rsp:Variable>`) {
		t.Fatalf("environment not escaped into the create body: %s", create)
	}
	if !f.deleted || !f.signalled {
		t.Fatal("shell was not signalled and deleted")
	}
}

func TestPinMismatchFailsHandshake(t *testing.T) {
	srv := httptest.NewTLSServer(&fakeWSMan{stdout: "x"})
	defer srv.Close()
	ep := endpointFor(t, srv)
	ep.CertSHA256 = strings.Repeat("ab", 32)
	c, err := New(ep, basic(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), "x", nil, nil); err == nil || !strings.Contains(err.Error(), "does not match the pinned") {
		t.Fatalf("want pin mismatch, got %v", err)
	}
}

// A resumed TLS session skips VerifyPeerCertificate, so the pinned config
// turns resumption off and checks the pin again on every connection.
func TestPinnedConfigChecksEveryConnection(t *testing.T) {
	srv := httptest.NewTLSServer(&fakeWSMan{stdout: "x"})
	defer srv.Close()
	ep := endpointFor(t, srv)
	tc, err := tlsConfig(ep)
	if err != nil {
		t.Fatal(err)
	}
	if !tc.SessionTicketsDisabled || tc.ClientSessionCache != nil {
		t.Fatal("pinned config must not resume sessions")
	}
	if tc.VerifyConnection == nil {
		t.Fatal("pinned config must verify every connection")
	}
	good := tls.ConnectionState{PeerCertificates: []*x509.Certificate{srv.Certificate()}}
	if err := tc.VerifyConnection(good); err != nil {
		t.Fatalf("pinned certificate refused: %v", err)
	}
	if err := tc.VerifyConnection(tls.ConnectionState{DidResume: true}); err == nil {
		t.Fatal("a connection with no certificate passed")
	}
	ep.CertSHA256 = strings.Repeat("ab", 32)
	other, err := tlsConfig(ep)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.VerifyConnection(tls.ConnectionState{DidResume: true, PeerCertificates: []*x509.Certificate{srv.Certificate()}}); err == nil || !strings.Contains(err.Error(), "does not match the pinned") {
		t.Fatalf("resumed connection with another certificate: want pin mismatch, got %v", err)
	}
}

func TestUntrustedCertificateFailsWithoutPin(t *testing.T) {
	srv := httptest.NewTLSServer(&fakeWSMan{stdout: "x"})
	defer srv.Close()
	ep := endpointFor(t, srv)
	ep.CertSHA256 = "" // system roots do not know the test CA
	c, err := New(ep, basic(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), "x", nil, nil); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("want certificate error, got %v", err)
	}
}

func TestRedirectIsRefusedAndCredentialStays(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect target was contacted with auth header %q", r.Header.Get("Authorization"))
	}))
	defer other.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/wsman", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c, err := New(endpointFor(t, srv), basic(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), "x", nil, nil); !errors.Is(err, ErrRefused) {
		t.Fatalf("want refused redirect, got %v", err)
	}
}

func TestWrongPasswordIsAuthError(t *testing.T) {
	srv := httptest.NewTLSServer(&fakeWSMan{stdout: "x"})
	defer srv.Close()
	a := basic()
	a.Password = "wrong"
	c, err := New(endpointFor(t, srv), a, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), "x", nil, nil); !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
}

func TestNTLMDoesNotDowngradeToBasic(t *testing.T) {
	f := &fakeWSMan{stdout: "x"}
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	a := basic()
	a.Method = "ntlm"
	c, err := New(endpointFor(t, srv), a, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), "x", nil, nil); !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth (no Basic fallback), got %v", err)
	}
	if len(f.bodies) != 0 {
		t.Fatal("an NTLM client sent the password as Basic")
	}
}

func TestEndpointRefusals(t *testing.T) {
	cases := []Endpoint{
		{Host: "https://fs01"},
		{Host: "fs01/wsman"},
		{Host: "fs01:5986"},
		{Host: "fs01", HTTP: true},
		{Host: "fs01", Port: 70000},
		{Host: "fs01", CertSHA256: "14C7487FC08DE66AB3EF9E823036FF3AAD309396"},
	}
	for _, ep := range cases {
		if _, err := New(ep, basic(), time.Second); err == nil {
			t.Errorf("endpoint %+v was accepted", ep)
		}
	}
	if _, err := New(Endpoint{Host: "fs01", HTTP: true, AllowHTTP: true}, basic(), time.Second); err == nil {
		t.Error("Basic over plain HTTP accepted: the password would cross the wire in clear text")
	}
	if _, err := New(Endpoint{Host: "fs01", HTTP: true, AllowHTTP: true}, Auth{Method: "ntlm", Username: "u", Password: "Passw0rd-long"}, time.Second); err != nil {
		t.Errorf("explicit --allow-http with NTLM refused: %v", err)
	}
	if _, err := New(Endpoint{Host: "fs01"}, Auth{Username: "u"}, time.Second); err == nil {
		t.Error("missing password accepted")
	}
}

func TestParseFingerprint(t *testing.T) {
	got, err := ParseFingerprint("4B:EF:91:35:FE:FA:8B:33:94:8D:5A:13:6A:86:E6:4D:77:CA:64:8D:06:13:0C:F6:D9:00:82:71:56:DC:35:C2")
	if err != nil || got != "4bef9135fefa8b33948d5a136a86e64d77ca648d06130cf6d900827156dc35c2" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestThrottlingIsATypedRateLimitError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c, err := New(endpointFor(t, srv), basic(), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Run(context.Background(), "cmd.exe", nil, nil)
	var rl *cliutil.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("want a RateLimitError, got %v", err)
	}
}
