// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// refusingClient fails the test if any connection is attempted.
func refusingClient(t *testing.T, dials *int32) *http.Client {
	t.Helper()
	return &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			atomic.AddInt32(dials, 1)
			return nil, errors.New("dial attempted")
		},
	}, Timeout: time.Second}
}

// TestPlainHTTPToAnotherHostIsRefused: press 4.34.2's client refuses a session
// over http:// to a host that is not this computer, so DSM login must refuse the
// password there too (it used to warn and send it, then every request failed).
// A cached session is refused the same way. Nothing is dialed.
func TestPlainHTTPToAnotherHostIsRefused(t *testing.T) {
	creds := Credentials{Account: "admin", Password: "PLAIN-HTTP-PW-5512"}
	for _, origin := range []string{"http://192.0.2.9:5000", "http://nas.lan:5000", "HTTP://10.0.0.20:5000", "http://[fd00::1]:5000"} {
		var dials int32
		_, err := Login(context.Background(), refusingClient(t, &dials), origin, creds)
		if !errors.Is(err, ErrPlainHTTPCredentials) || strings.Contains(err.Error(), creds.Password) {
			t.Fatalf("Login(%s) = %v, want ErrPlainHTTPCredentials without the password", origin, err)
		}
		c := &Client{HTTP: refusingClient(t, &dials), BaseURL: origin, Creds: creds}
		c.sess = Session{SID: "CACHED-SID-5512"}
		if _, err := c.Get(context.Background(), "/webapi/entry.cgi?api=SYNO.Core.System&method=info&version=1", nil); !errors.Is(err, ErrPlainHTTPCredentials) {
			t.Fatalf("Get over %s with a cached session = %v, want ErrPlainHTTPCredentials", origin, err)
		}
		if dials != 0 {
			t.Fatalf("%s: %d dial(s) attempted; nothing may be sent", origin, dials)
		}
		if !PlainHTTPRefused(origin) {
			t.Fatalf("PlainHTTPRefused(%s) = false", origin)
		}
	}
}

// TestPlainHTTPOnThisComputerStillLogsIn: localhost and loopback addresses keep
// the warning-only behaviour (a local tunnel or a test server), like the press.
func TestPlainHTTPOnThisComputerStillLogsIn(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:5000", "http://localhost:5000", "http://[::1]:5000"} {
		if PlainHTTPRefused(origin) || RefusePlainHTTP(origin) != nil {
			t.Fatalf("%s refused; loopback http:// only warns", origin)
		}
	}
	for _, origin := range []string{"https://192.0.2.9:5001", "https://nas.lan:5001"} {
		if PlainHTTPRefused(origin) {
			t.Fatalf("%s refused; it is https://", origin)
		}
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if PlainHTTPRefused(srv.URL) {
		t.Fatalf("the loopback test server %s was refused", srv.URL)
	}
}
