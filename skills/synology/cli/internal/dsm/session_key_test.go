// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-session-key): one canonical session key.

package dsm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionKeyIsOneCanonicalKey: the session file name, the origin recorded
// in it and every comparison against it go through SessionKey, so every
// spelling of one NAS address (upper-case scheme or host, a default port
// written or not, a trailing slash) is one session, and a different scheme,
// port or reverse-proxy path is a different one.
func TestSessionKeyIsOneCanonicalKey(t *testing.T) {
	same := [][]string{
		{"https://proxy.test/nas1", "HTTPS://PROXY.test:443/nas1/", "https://proxy.test:443/nas1"},
		{"http://127.0.0.1:5000", "HTTP://127.0.0.1:5000/", " http://127.0.0.1:5000 "},
		{"https://nas.lan", "https://NAS.lan:443", "https://nas.lan/"},
	}
	for _, group := range same {
		for _, a := range group[1:] {
			if !SameSession(group[0], a) || SessionFileName(a) != SessionFileName(group[0]) || SessionKey(a) == "" {
				t.Errorf("%q and %q must be one session (keys %q, %q; files %q, %q)", group[0], a, SessionKey(group[0]), SessionKey(a), SessionFileName(group[0]), SessionFileName(a))
			}
		}
	}
	different := [][2]string{
		{"https://proxy.test/nas1", "https://proxy.test/nas2"},
		{"https://proxy.test/nas1", "https://proxy.test"},
		{"https://proxy.test/nas1", "http://proxy.test/nas1"},
		{"https://nas.lan:5001", "https://nas.lan:5000"},
		{"https://nas.lan", "http://nas.lan"},
	}
	for _, d := range different {
		if SameSession(d[0], d[1]) || SessionFileName(d[0]) == SessionFileName(d[1]) {
			t.Errorf("%q and %q must be different sessions", d[0], d[1])
		}
	}
	for _, bad := range []string{"", "nas:5001", "https://admin:pw@nas:5001", "ftp://nas", "https://"} {
		if SessionKey(bad) != "" || SessionFileName(bad) != "" || SameSession(bad, bad) {
			t.Errorf("%q has no session key (got %q)", bad, SessionKey(bad))
		}
	}
	if name := SessionFileName("https://proxy.test/a b/<nas>|*?"); strings.ContainsAny(name, `/\:<>|*?" `) || !strings.HasPrefix(name, "session-") {
		t.Errorf("file name %q is not safe on every OS", name)
	}

	// The trusted-device token cache follows the same key.
	dir := t.TempDir()
	if err := SaveDeviceID(dir, "HTTPS://PROXY.test:443/nas1/", "admin", "did-nas1"); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceID(dir, "https://proxy.test/nas1", "admin"); got != "did-nas1" {
		t.Fatalf("a token saved for HTTPS://PROXY.test:443/nas1/ was not found for https://proxy.test/nas1 (got %q)", got)
	}
	for _, other := range []string{"https://proxy.test/nas2", "https://proxy.test", "http://proxy.test/nas1"} {
		if got := CachedDeviceID(dir, other, "admin"); got != "" {
			t.Fatalf("nas1's token leaked to %s", other)
		}
	}
	raw, _ := os.ReadFile(SessionFilePath(dir, "https://proxy.test/nas1"))
	if !strings.Contains(string(raw), `"origin": "https://proxy.test:443/nas1"`) {
		t.Fatalf("the recorded origin is not the session key: %s", raw)
	}
	// A record whose origin is another spelling of the same key still matches.
	path := filepath.Join(dir, SessionFileName("https://nas.lan"))
	if err := os.WriteFile(path, []byte(`{"origin":"HTTPS://NAS.LAN/","device_ids":{"admin":"did-lan"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CachedDeviceID(dir, "https://nas.lan:443", "admin"); got != "did-lan" {
		t.Fatalf("the comparison is not canonical on both sides (got %q)", got)
	}
}
