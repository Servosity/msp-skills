// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"testing"
	"time"
)

// A program that writes its own proxy CONNECT asks the kit first, and gets
// the transport's answer (proxyUnit): the proxy's own credentials go only to
// an https proxy, and no other held value goes to a proxy at all.
func TestProxyWriteJudgesAConnectTheKitDidNotWrite(t *testing.T) {
	const token = "exoTOKEN7fb3189bQ6u3sZxW4o1n7R8v52"
	connect := func(host, user, pass string) []byte {
		u := "CONNECT " + host + ":443 HTTP/1.1\r\nHost: " + host + ":443\r\nUser-Agent: Go-http-client/1.1\r\n"
		if user != "" {
			u += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
		}
		return []byte(u + "\r\n")
	}
	for _, c := range []struct {
		name, proxy string
		unit        []byte
		send        bool
	}{
		{"no credentials", "http://127.0.0.1:3128", connect("api.example.test", "", ""), true},
		{"own credentials, https proxy", "https://ops:Prx-91zq7Wsink@proxy.example.test:3128", connect("api.example.test", "ops", "Prx-91zq7Wsink"), true},
		{"own credentials, plain http proxy", "http://ops:Prx-91zq7Wsink@127.0.0.1:3128", connect("api.example.test", "ops", "Prx-91zq7Wsink"), false},
		{"held token as the password, https proxy", "https://ops:" + token + "@proxy.example.test:3128", connect("api.example.test", "ops", token), false},
		{"held token in the target", "https://proxy.example.test:3128", connect(token+".example.test", "", ""), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			resetForTest(Declarations{})
			Hold("env:EXCHANGE_ONLINE_TOKEN", token)
			pu, err := url.Parse(c.proxy)
			if err != nil {
				t.Fatal(err)
			}
			var w bytes.Buffer
			err = ProxyWrite(&w, pu, c.unit)
			if (err == nil) != c.send {
				t.Errorf("ProxyWrite = %v, want sent %v", err, c.send)
			}
			if want := map[bool]string{true: string(c.unit), false: ""}[c.send]; w.String() != want {
				t.Errorf("proxy received %q, want %q", w.String(), want)
			}
		})
	}
}

// holdDuringWrite holds a value while ProxyWrite is writing, and records
// whether the hold returned before the write did.
type holdDuringWrite struct{ heldFirst bool }

func (h *holdDuringWrite) Write(p []byte) (int, error) {
	done := make(chan struct{})
	go func() { Hold("response:later", "Go-http-client/1.1"); close(done) }()
	select {
	case <-done:
		h.heldFirst = true
	case <-time.After(200 * time.Millisecond):
	}
	return len(p), nil
}

// A value held while the CONNECT is on its way waits for the write (the
// verdict and the write are one step), so no verdict goes stale.
func TestProxyWriteHoldsTheBarrierUntilWritten(t *testing.T) {
	resetForTest(Declarations{})
	pu, _ := url.Parse("http://127.0.0.1:3128")
	var w holdDuringWrite
	if err := ProxyWrite(&w, pu, []byte("CONNECT api.example.test:443 HTTP/1.1\r\nUser-Agent: Go-http-client/1.1\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	if w.heldFirst {
		t.Error("a value was held while ProxyWrite wrote: its verdict was stale")
	}
}
