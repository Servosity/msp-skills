// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Codex review r1 (4.34.2 reprint), class credential sent to another host:
// the origin pin lowercased the whole host, an IPv6 zone included, and zones
// are case-sensitive interface names (Linux lan0 and LAN0 are two
// interfaces), so a 307 from https://[fe80::1%lan0] to https://[fe80::1%LAN0]
// passed the pin and the replayed body (two access tokens of provisioning
// link-tenant) reached another peer. The pin may only ever conflate spellings
// the transport dials identically: a host carrying a zone is no origin at
// all, so it pins nothing and matches nothing (fail closed). No request is
// sent: the pins are checked directly.
func TestZonedHostPinsNothing(t *testing.T) {
	newReq := func(method, raw string) *http.Request {
		t.Helper()
		req, err := http.NewRequest(method, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	pins := newNerdioPins("https://[fe80::1%25lan0]", "https://[fe80::1%25lan0]/tenant-1/oauth2/v2.0/token")
	first := newReq(http.MethodPost, "https://[fe80::1%25lan0]/rest-api/v1/accountprovisioning/linktenant")
	other := newReq(http.MethodPost, "https://[fe80::1%25LAN0]/rest-api/v1/accountprovisioning/linktenant")
	for _, req := range []*http.Request{first, other, newReq(http.MethodPost, "https://[fe80::1%25LAN0]/tenant-1/oauth2/v2.0/token")} {
		if err := pins.check(req); err == nil {
			t.Errorf("%s %s passed the pin; a zoned host must match no pinned origin", req.Method, req.URL)
		}
	}
	hc := &http.Client{}
	installPins(hc, pins, false)
	if err := hc.CheckRedirect(other, []*http.Request{first}); err == nil {
		t.Errorf("a redirect from zone lan0 to zone LAN0 was followed")
	}
	// The same check on an ordinary pinned origin still passes (control), and
	// a host's ASCII case still folds (DNS names are case-insensitive).
	plain := newNerdioPins("https://nmm.contoso.test", "")
	if err := plain.check(newReq(http.MethodGet, "https://NMM.Contoso.test/rest-api/v1/accounts")); err != nil {
		t.Errorf("control: a request to the pinned API origin was refused: %v", err)
	}
	for _, raw := range []string{"https://[fe80::1%25lan0]/", "https://[fe80::1%25LAN0]:443/"} {
		req := newReq(http.MethodGet, raw)
		if o := normalizedOrigin(req.URL); o != "" {
			t.Errorf("normalizedOrigin(%s) = %q, want \"\" (a zoned host is no origin)", raw, o)
		}
	}
}

// Codex review r1 P2 one-liners (fixed, not P1s):
//   - fleet host-estate: an AVD session host status "Unavailable" read as
//     running ("unavailable" holds "available");
//   - job wait: --timeout 1s --interval 1h slept the whole interval after a
//     Running poll before checking the deadline; the last wait now ends at the
//     timeout.
func TestHostEstateUnavailableIsNotRunning(t *testing.T) {
	m := newFakeInstance(t, "estateClientId-5d1a", "estateSecret-8c3f", "estateMinted-2e7b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/accounts"):
			_, _ = io.WriteString(w, `[{"id":101,"name":"Contoso"}]`)
		case strings.HasSuffix(r.URL.Path, "/host-pool"):
			_, _ = io.WriteString(w, `[{"name":"pool1","subscriptionId":"sub-1","resourceGroup":"rg-1"}]`)
		default:
			_, _ = io.WriteString(w, `[{"name":"host-up","status":"Available"},{"name":"host-down","status":"Unavailable"},{"name":"vm-on","powerState":"VM running"}]`)
		}
	})
	fence := fenceNetwork(t)
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	var out bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"fleet", "host-estate", "--json", "--data-source", "live"})
	_ = cmd.Execute()
	text := out.String()
	if !strings.Contains(text, "host-down") {
		t.Fatalf("host-estate did not list the hosts; the test did not run:\n%.2000s", text)
	}
	var view struct {
		Items []struct {
			Host    string `json:"host"`
			Running *bool  `json:"running"`
		} `json:"items"`
	}
	start := strings.Index(text, "{")
	if start < 0 || json.Unmarshal([]byte(text[start:]), &view) != nil {
		t.Fatalf("host-estate --json is not one JSON object:\n%.2000s", text)
	}
	want := map[string]bool{"host-up": true, "host-down": false, "vm-on": true}
	for _, it := range view.Items {
		if w, ok := want[it.Host]; ok && (it.Running == nil || *it.Running != w) {
			t.Errorf("host %s: running %v, want %v", it.Host, it.Running, w)
		}
	}
	if _, escaped := fence.result(); len(escaped) != 0 {
		t.Errorf("non-loopback dials escaped: %v", escaped)
	}
}

func TestJobWaitTimeoutEndsTheLastWait(t *testing.T) {
	m := newFakeInstance(t, "waitClientId-6b2c", "waitSecret-1f8e", "waitMinted-4a9d", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":4821,"status":"Running"}`)
	})
	fence := fenceNetwork(t)
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	var out bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"job", "wait", "4821", "--timeout", "1s", "--interval", "1h", "--json"})
	done := make(chan struct{})
	start := time.Now()
	go func() { _ = cmd.Execute(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("job wait --timeout 1s --interval 1h was still waiting after 20s:\n%s", out.String())
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("job wait --timeout 1s took %s", took)
	}
	if _, escaped := fence.result(); len(escaped) != 0 {
		t.Errorf("non-loopback dials escaped: %v", escaped)
	}
}
