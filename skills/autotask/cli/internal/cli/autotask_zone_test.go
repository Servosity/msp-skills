// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"autotask-pp-cli/internal/cliutil/testenv"
)

// zone-discovery-pinned: discovery sends the API user's UserName, so it goes
// only to the discovery origin; a redirect elsewhere is refused before it is
// followed.
func TestZoneDiscoveryRefusesRedirects(t *testing.T) {
	const user = "zoneRedirUnit-3d7f1b@example.com"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"url":"https://webservices14.autotask.net/atservicesrest/"}`))
	}))
	t.Cleanup(other.Close)
	boot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/zoneInformation?"+r.URL.RawQuery, http.StatusFound)
	}))
	t.Cleanup(boot.Close)
	old := zoneBootstrapHost
	zoneBootstrapHost = boot.URL + "/atservicesrest/V1.0"
	t.Cleanup(func() { zoneBootstrapHost = old })
	_, err := discoverZone(t.Context(), user, 0)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Autotask host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the redirect target was contacted %d time(s) with the UserName", otherHits.Load())
	}
}

// The zone command caches only a webservicesN.autotask.net zone (it is where
// every later credential goes), and never prints or errors with the UserName.
func TestZoneCachesOnlyAnAutotaskZoneAndNeverEchoesTheUser(t *testing.T) {
	const user = "zoneEchoUnit-8a2c6e@example.com"
	var zone atomic.Value
	boot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user") != user {
			t.Errorf("zoneInformation got user %q", r.URL.Query().Get("user"))
		}
		_, _ = w.Write([]byte(`{"url":"` + zone.Load().(string) + `","ciLevel":1}`))
	}))
	t.Cleanup(boot.Close)
	old := zoneBootstrapHost
	zoneBootstrapHost = boot.URL + "/atservicesrest/V1.0"
	t.Cleanup(func() { zoneBootstrapHost = old })
	testenv.Isolate(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("AUTOTASK_CONFIG", cfgPath)
	t.Setenv("AUTOTASK_BASE_URL", "")
	run := func() (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"zone", "--user", user, "--json"})
		err := cmd.Execute()
		if err != nil {
			out.WriteString(err.Error())
		}
		return out.String(), err
	}
	for _, bad := range []string{"https://collector.example/atservicesrest/", "http://webservices14.autotask.net/atservicesrest/",
		"https://webservices14.autotask.net.collector.example/atservicesrest/", "https://webservices.autotask.net/atservicesrest/"} {
		zone.Store(bad)
		out, err := run()
		if err == nil {
			t.Errorf("zone %s: cached without an error (%s)", bad, out)
		}
		if b, _ := os.ReadFile(cfgPath); bytes.Contains(b, []byte("collector")) || bytes.Contains(b, []byte("http://")) || bytes.Contains(b, []byte("webservices.autotask.net")) {
			t.Fatalf("zone %s was written to config.toml:\n%s", bad, b)
		}
		if strings.Contains(out, user) {
			t.Errorf("zone %s: the UserName was printed: %s", bad, out)
		}
	}
	zone.Store("https://webservices14.autotask.net/atservicesrest/")
	out, err := run()
	if err != nil {
		t.Fatalf("a real zone: %v (%s)", err, out)
	}
	if strings.Contains(out, user) {
		t.Errorf("the UserName was printed: %s", out)
	}
	if b, _ := os.ReadFile(cfgPath); !bytes.Contains(b, []byte("https://webservices14.autotask.net/atservicesrest/V1.0")) {
		t.Errorf("the real zone was not cached:\n%s", b)
	}
}
