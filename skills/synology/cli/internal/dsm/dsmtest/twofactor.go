// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsmtest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// The two-factor stand-in's one account (its password is per instance,
// TwoFactorNAS.Password).
const (
	TwoFactorAccount  = "admin"
	TwoFactorOTP      = "771122"
	TwoFactorDeviceID = "did-two-factor-1"
)

// TwoFactorNAS stands in for a DSM whose account has two-factor sign-in, the
// way DSM behaves: a login needs the password plus either a valid one-time
// code (the answer then carries a trusted-device token, "did") or that
// trusted-device token; a password alone gets error 403 (code required). It
// serves reads only to a session id it issued and still holds, forgets a
// session id on logout, and can sit behind a reverse-proxy path prefix
// (every request must start with it). Every login's form is recorded.
type TwoFactorNAS struct {
	*httptest.Server
	Prefix string
	// Password is the account's password, random per instance.
	Password string

	mu     sync.Mutex
	sids   map[string]bool
	n      int
	logins []url.Values
}

// NewTwoFactorNAS starts the stand-in; prefix is "" or a path such as "/nas1".
func NewTwoFactorNAS(t *testing.T, prefix string) *TwoFactorNAS {
	t.Helper()
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	n := &TwoFactorNAS{Prefix: prefix, Password: "pw-" + hex.EncodeToString(b), sids: map[string]bool{}}
	n.Server = httptest.NewServer(http.HandlerFunc(n.serve))
	t.Cleanup(n.Close)
	return n
}

// URL is the base address a client is pointed at (server URL plus prefix).
func (n *TwoFactorNAS) URL() string { return n.Server.URL + n.Prefix }

// Logins returns a copy of every login form received, in order.
func (n *TwoFactorNAS) Logins() []url.Values {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]url.Values(nil), n.logins...)
}

// Forget drops every session id issued so far (as DSM does on a timeout).
func (n *TwoFactorNAS) Forget() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sids = map[string]bool{}
}

func (n *TwoFactorNAS) serve(w http.ResponseWriter, r *http.Request) {
	if n.Prefix != "" {
		if !strings.HasPrefix(r.URL.Path, n.Prefix+"/") {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = strings.TrimPrefix(r.URL.Path, n.Prefix)
	}
	if ServeAPIInfo(w, r) {
		return
	}
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	for k, v := range r.URL.Query() {
		form[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	switch api, method := form.Get("api"), form.Get("method"); {
	case api == "SYNO.API.Auth" && method == "login":
		n.login(w, form)
	case api == "SYNO.API.Info":
		_, _ = io.WriteString(w, `{"success":true,"data":{"SYNO.API.Auth":{"path":"entry.cgi","minVersion":1,"maxVersion":7},"SYNO.DSM.Info":{"path":"entry.cgi","minVersion":1,"maxVersion":2}}}`)
	case method == "logout":
		n.mu.Lock()
		delete(n.sids, form.Get("_sid"))
		n.mu.Unlock()
		_, _ = io.WriteString(w, `{"success":true}`)
	default:
		n.mu.Lock()
		ok := n.sids[form.Get("_sid")]
		n.mu.Unlock()
		switch {
		case !ok:
			_, _ = io.WriteString(w, `{"success":false,"error":{"code":119}}`)
		case api == "SYNO.DSM.Info":
			_, _ = io.WriteString(w, `{"success":true,"data":{"model":"DS1621+","version_string":"DSM 7.2.2"}}`)
		default:
			_, _ = io.WriteString(w, `{"success":false,"error":{"code":102}}`)
		}
	}
}

func (n *TwoFactorNAS) login(w http.ResponseWriter, form url.Values) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.logins = append(n.logins, form)
	if form.Get("account") != TwoFactorAccount || form.Get("passwd") != n.Password {
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":400}}`)
		return
	}
	did := ""
	switch {
	case form.Get("device_id") == TwoFactorDeviceID:
	case form.Get("otp_code") == TwoFactorOTP:
		did = TwoFactorDeviceID
	case form.Get("otp_code") != "":
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":404,"errors":{"types":[{"type":"otp"}]}}}`)
		return
	default:
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":403,"errors":{"token":"tok-otp","types":[{"type":"otp"}]}}}`)
		return
	}
	n.n++
	sid := fmt.Sprintf("sid-2fa-%d", n.n)
	n.sids[sid] = true
	_, _ = fmt.Fprintf(w, `{"success":true,"data":{"sid":%q,"synotoken":"tok-2fa-%d","did":%q}}`, sid, n.n, did)
}
