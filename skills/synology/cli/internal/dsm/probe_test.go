// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// recordingHost serves body for every request and records everything a
// request carried (method, URL, headers, body).
type recordingHost struct {
	mu   sync.Mutex
	seen []string
	srv  *httptest.Server
}

func newRecordingHost(t *testing.T, h http.HandlerFunc) *recordingHost {
	t.Helper()
	rh := &recordingHost{}
	rh.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(b))
		var hdr strings.Builder
		for k, vs := range r.Header {
			hdr.WriteString(k + "=" + strings.Join(vs, ",") + ";")
		}
		rh.mu.Lock()
		rh.seen = append(rh.seen, r.Method+" "+r.URL.String()+" "+hdr.String()+" "+string(b))
		rh.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(rh.srv.Close)
	return rh
}

func (rh *recordingHost) requests() []string {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	return append([]string(nil), rh.seen...)
}

// TestLoginProbesBeforeSendingPassword: a host that does not answer
// SYNO.API.Info as DSM (the client's router on :5001, a web server, a proxy)
// gets one unauthenticated GET and never the account or password; a DSM still
// logs in, one-time code and trusted-device token included.
func TestLoginProbesBeforeSendingPassword(t *testing.T) {
	const pw = "PWSENTINEL-4471"
	creds := Credentials{Account: "msp-admin", Password: pw, OTPCode: "123456"}
	notDSM := map[string]http.HandlerFunc{
		"html page": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "<!doctype html><html><body>router login</body></html>")
		},
		"json without envelope": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"status":"ok","data":{"SYNO.API.Auth":{}}}`)
		},
		"envelope without SYNO.API.Auth": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"data":{"SYNO.Other":{"path":"x.cgi"}}}`)
		},
		"failure envelope": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"success":false,"error":{"code":102}}`)
		},
		"404": func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login.html", http.StatusFound)
		},
	}
	for name, h := range notDSM {
		t.Run(name, func(t *testing.T) {
			host := newRecordingHost(t, h)
			_, err := Login(context.Background(), host.srv.Client(), host.srv.URL, creds)
			var nd *NotDSMError
			if !errors.As(err, &nd) || !strings.Contains(err.Error(), "the password was not sent") {
				t.Fatalf("Login to a host that is not DSM: err = %v, want a NotDSMError saying the password was not sent", err)
			}
			reqs := host.requests()
			if len(reqs) != 1 || !strings.HasPrefix(reqs[0], "GET /webapi/query.cgi?api=SYNO.API.Info") {
				t.Fatalf("requests = %q, want only the SYNO.API.Info probe", reqs)
			}
			for _, r := range reqs {
				for _, secret := range []string{pw, "msp-admin", "123456", "passwd"} {
					if strings.Contains(r, secret) {
						t.Fatalf("the probe carried %q: %s", secret, r)
					}
				}
			}
		})
	}

	t.Run("dsm logs in with a one-time code", func(t *testing.T) {
		host := newRecordingHost(t, func(w http.ResponseWriter, r *http.Request) {
			if dsmtest.ServeAPIInfo(w, r) {
				return
			}
			_ = r.ParseForm()
			if r.Method != http.MethodPost || r.PostForm.Get("passwd") != pw || r.PostForm.Get("otp_code") != "123456" {
				_, _ = io.WriteString(w, `{"success":false,"error":{"code":400}}`)
				return
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"sid":"s1","synotoken":"t1","did":"dev-1"}}`)
		})
		s, err := Login(context.Background(), host.srv.Client(), host.srv.URL, creds)
		if err != nil || s.SID != "s1" || s.DeviceID != "dev-1" {
			t.Fatalf("Login to a DSM: session=%+v err=%v", s, err)
		}
		reqs := host.requests()
		if len(reqs) != 2 || !strings.HasPrefix(reqs[0], "GET /webapi/query.cgi") || !strings.HasPrefix(reqs[1], "POST /webapi/entry.cgi") {
			t.Fatalf("requests = %q, want the probe then the login POST", reqs)
		}
		if strings.Contains(reqs[0], pw) {
			t.Fatal("the probe carried the password")
		}
	})

	t.Run("dsm logs in with a trusted-device token", func(t *testing.T) {
		host := newRecordingHost(t, func(w http.ResponseWriter, r *http.Request) {
			if dsmtest.ServeAPIInfo(w, r) {
				return
			}
			_ = r.ParseForm()
			if r.PostForm.Get("device_id") != "dev-1" || r.PostForm.Get("otp_code") != "" {
				_, _ = io.WriteString(w, `{"success":false,"error":{"code":403}}`)
				return
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"sid":"s2","synotoken":"t2"}}`)
		})
		withDevice := creds
		withDevice.DeviceID = "dev-1"
		if s, err := Login(context.Background(), host.srv.Client(), host.srv.URL, withDevice); err != nil || s.SID != "s2" {
			t.Fatalf("device-token login: session=%+v err=%v", s, err)
		}
		if n := len(host.requests()); n != 2 {
			t.Fatalf("%d requests, want one probe and one login (the device token is tried first and accepted)", n)
		}
	})
}

// TestProbeDSMSharedVerdicts pins the reasons doctor prints.
func TestProbeDSMSharedVerdicts(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !dsmtest.ServeAPIInfo(w, r) {
			http.NotFound(w, r)
		}
	}))
	defer ok.Close()
	if err := ProbeDSM(context.Background(), ok.Client(), ok.URL); err != nil {
		t.Fatalf("a DSM answer was refused: %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	err := ProbeDSM(context.Background(), http.DefaultClient, url)
	var pe *ProbeTransportError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "cannot reach the NAS") {
		t.Fatalf("unreachable host: err = %v", err)
	}
}

// TestPinMismatchIsItsOwnErrorKind: a pinned NAS that presents another
// certificate fails with ErrCertificatePinMismatch and a description that
// names the mismatch and the re-pin remedy, not "connection error".
func TestPinMismatchIsItsOwnErrorKind(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !dsmtest.ServeAPIInfo(w, r) {
			_, _ = io.WriteString(w, `{"success":true,"data":{"sid":"s"}}`)
		}
	}))
	defer srv.Close()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if err := ApplyTLS(cfg, strings.Repeat("ab", 32), false); err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
	_, err := Login(context.Background(), hc, srv.URL, Credentials{Account: "a", Password: "p"})
	if !errors.Is(err, ErrCertificatePinMismatch) {
		t.Fatalf("err = %v, want ErrCertificatePinMismatch", err)
	}
	for _, want := range []string{"does not match the pinned SHA-256 fingerprint", "re-pin", "SYNOLOGY_TLS_FINGERPRINT", "the password was not sent",
		// The remedy is: check the presented fingerprint in DSM, then pin
		// exactly that value (finding 9: never a blind --pin-cert).
		"verify that fingerprint in DSM (Control Panel > Security > Certificate)", "--tls-fingerprint <the value you checked>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("pin mismatch text lacks %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "--pin-cert") {
		t.Fatalf("pin mismatch hint recommends --pin-cert, which trusts whatever certificate is presented (a man-in-the-middle looks exactly like this): %v", err)
	}
	if strings.Contains(err.Error(), "connection error or malformed response") {
		t.Fatalf("pin mismatch still reads as a generic failure: %v", err)
	}
}
