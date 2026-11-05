// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-probe-before-session) end to end, and the
// type-preserving, any-length whole-field masking of secret values.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// stubNAS is a DSM stand-in answering the probe, any login (sid sid-1,
// SynoToken tok-1), logout and the reads in body; it records every request.
type stubNAS struct {
	*httptest.Server
	mu  sync.Mutex
	log []string
}

func newStubNAS(t *testing.T, body map[string]string) *stubNAS {
	t.Helper()
	n := &stubNAS{}
	n.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.log = append(n.log, r.Method+" "+r.URL.String()+" cookie="+r.Header.Get("Cookie")+" body="+string(b))
		n.mu.Unlock()
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		form, _ := neturl.ParseQuery(string(b))
		q := r.URL.Query()
		api, method := firstOf(q.Get("api"), form["api"]), firstOf(q.Get("method"), form["method"])
		switch {
		case api == "SYNO.API.Auth" && method == "login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
		case method == "logout":
			_, _ = w.Write([]byte(`{"success":true}`))
		case body[api+"/"+method] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + body[api+"/"+method] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	t.Cleanup(n.Close)
	return n
}

func (n *stubNAS) requests() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return strings.Join(n.log, "\n")
}

func firstOf(v string, vs []string) string {
	if v != "" || len(vs) == 0 {
		return v
	}
	return vs[0]
}

// TestRawSessionFromConfigIsNotSentToANonDSMHost is the review's repro: a
// private config holding only base_url and a raw auth_header sid (no account,
// no password, no cached session) made `system dsm --no-cache` send
// _sid=<sentinel> straight to the host, even one serving a router page. The
// host now gets the unauthenticated probe only, the command fails saying the
// session was not sent, and a DSM at that address still gets the sid.
func TestRawSessionFromConfigIsNotSentToANonDSMHost(t *testing.T) {
	const sid = "SID-sentinel-9917"
	writeConfig := func(t *testing.T, base string) string {
		path := filepath.Join(t.TempDir(), "config.toml") // press 4.34 config files are TOML
		if err := os.WriteFile(path, []byte("base_url = \""+base+"\"\nauth_header = \""+sid+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("router page", func(t *testing.T) {
		sweepIsolate(t)
		dsm.ForgetVerifiedAddresses()
		url, seen := notDSMHost(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><body>router login</body></html>")
		})
		cfg := writeConfig(t, url)
		for _, args := range [][]string{
			{"system", "dsm", "--no-cache", "--config", cfg, "--json"},
			{"abb", "tasks", "--no-cache", "--config", cfg, "--json", "--data-source", "live"},
		} {
			out, errText, code := runCLIErr(t, args...)
			if code == 0 || !strings.Contains(out+errText, "the session was not sent") || !strings.Contains(out+errText, "does not answer as a DSM WebAPI") {
				t.Fatalf("%v: exit %d, want a refusal naming the host as not DSM:\nout=%s\nerr=%s", args, code, out, errText)
			}
		}
		if strings.Contains(seen(), sid) {
			t.Fatalf("the raw session went to a host that is not DSM:\n%s", seen())
		}
		for _, line := range strings.Split(strings.TrimSpace(seen()), "\n") {
			if !strings.Contains(line, "SYNO.API.Info") {
				t.Fatalf("a host that is not DSM received more than the probe: %s", line)
			}
		}
	})
	t.Run("dsm", func(t *testing.T) {
		sweepIsolate(t)
		dsm.ForgetVerifiedAddresses()
		nas := newStubNAS(t, map[string]string{"SYNO.DSM.Info/getinfo": `{"model":"DS1621+","version_string":"DSM 7.2.2"}`})
		out, errText, code := runCLIErr(t, "system", "dsm", "--no-cache", "--config", writeConfig(t, nas.URL), "--json")
		if code != 0 || !strings.Contains(out, "DS1621+") {
			t.Fatalf("system dsm against a DSM: exit %d\nout=%s\nerr=%s", code, out, errText)
		}
		if !strings.Contains(nas.requests(), "_sid="+sid) || strings.Count(nas.requests(), "SYNO.API.Info") != 1 {
			t.Fatalf("want one probe, then the sid:\n%s", nas.requests())
		}
	})
}
