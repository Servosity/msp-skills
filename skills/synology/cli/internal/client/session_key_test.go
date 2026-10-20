// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-session-key) in the generated session manager.

package client

import (
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestTrustedDeviceTokenBehindAProxyPath is the review's pp repro: with
// SYNOLOGY_BASE_URL = <proxy>/nas1 (a DSM behind a reverse proxy), the file
// recorded the full address as its origin but the reload compared it with
// scheme://host, so after the session ended every relogin demanded a fresh
// one-time code. An upper-case scheme broke the same way. Every spelling of
// the address is now one session key, and the token is sent.
func TestTrustedDeviceTokenBehindAProxyPath(t *testing.T) {
	testenv.Isolate(t)
	dsm.ForgetVerifiedAddresses()
	nas := dsmtest.NewTwoFactorNAS(t, "/nas1")
	login := func(base, otp string) error {
		c := New(&config.Config{BaseURL: base, SynologyAccount: dsmtest.TwoFactorAccount, SynologyPassword: nas.Password, SynologyOtpCode: otp}, 5*time.Second, 0)
		c.Session.Invalidate()
		_, err := c.Session.EnsureToken()
		if err == nil {
			err = c.Session.SaveError()
		}
		return err
	}
	upper := "HTTP" + nas.URL()[len("http"):] + "/"
	if err := login(upper, dsmtest.TwoFactorOTP); err != nil {
		t.Fatalf("login with a code at %s: %v", upper, err)
	}
	nas.Forget() // the session ends on the NAS (timeout or session logout)
	for _, base := range []string{nas.URL(), upper} {
		if err := login(base, ""); err != nil {
			t.Fatalf("relogin at %s without a code: %v (the trusted-device token was not reused)", base, err)
		}
		logins := nas.Logins()
		if did := logins[len(logins)-1].Get("device_id"); did != dsmtest.TwoFactorDeviceID {
			t.Fatalf("relogin at %s sent device_id %q", base, did)
		}
	}
	// The token belongs to /nas1 only: another path at the same proxy gets none.
	m := newSessionManager(time.Second, "")
	for base, want := range map[string]string{nas.URL(): dsmtest.TwoFactorDeviceID, nas.Server.URL + "/nas2": "", nas.Server.URL: ""} {
		m.ConfigureDSM(base, dsm.Credentials{Account: dsmtest.TwoFactorAccount, Password: "x"})
		if m.deviceID != want {
			t.Errorf("ConfigureDSM(%s) loaded device token %q, want %q", base, m.deviceID, want)
		}
	}
	// The fleet cache (dsm.SaveDeviceID) and the session manager share the key.
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := dsm.SaveDeviceID(dir, "HTTPS://PROXY.test:443/nas7/", "fleet-acct", "did-fleet-7"); err != nil {
		t.Fatal(err)
	}
	m.ConfigureDSM("https://proxy.test/nas7", dsm.Credentials{Account: "fleet-acct", Password: "x"})
	if m.deviceID != "did-fleet-7" {
		t.Fatalf("the session manager did not load the fleet layer's token for the same NAS (got %q)", m.deviceID)
	}
}
