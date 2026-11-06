// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The restore and download workflows on the built binaries, under the
// credential kit (handfixes.json portal-kit-wiring): the portal session goes
// through the kit's outbound guard, its handoff and CSRF tokens reach only the
// origins credwire.Declarations allows them, a cross-origin redirect never
// carries a credential, and an interrupt still reaches the command's context.
//
// These need an https fake the binaries trust. Go reads SSL_CERT_FILE on Linux
// only (macOS asks the system's trust store), so they run on Linux: in CI's
// Linux row and in the release gate's container.

import (
	"bufio"
	"context"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Portal fixtures, each with letters and digits in every 8-character window.
const (
	handoffTok = "ho_T4k9W" + "2m7Q5x1Z8"
	csrfTok    = "cf_J8d3N" + "6r1V4b7K2"
	sessionCk  = "sc_Y5u2P" + "9e4G7h1M6"
)

// tlsFixture is an https fake whose certificate the built binaries trust
// through SSL_CERT_FILE.
func tlsFixture(t *testing.T, h http.Handler) (*httptest.Server, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the binaries trust a test certificate only through SSL_CERT_FILE, which Go reads on Linux")
	}
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, ca
}

// recorder counts what one fake received and which credentials it saw.
type recorder struct {
	mu   sync.Mutex
	hits atomic.Int64
	seen []string
}

func (r *recorder) note(req *http.Request) {
	r.hits.Add(1)
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, req.Method+" "+req.URL.String()+" "+fmt.Sprint(req.Header)+" "+string(body))
}

func (r *recorder) carried(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.seen {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

// portalMux serves the partner-access-token mint, the portal bootstrap page
// and a download status check that blocks until the request ends.
func portalMux(rec *recorder, listed chan<- struct{}) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rec.note(r)
		switch {
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"accessToken":%q}`, handoffTok)
		case r.URL.Path == "/newapp":
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: sessionCk, Path: "/"})
			fmt.Fprintf(w, `<html><head><meta name="_csrf" content=%q/><meta name="_csrf_header" content="X-CSRF-TOKEN"/></head></html>`, csrfTok)
		case r.URL.Path == "/application/service/getRestoreDownloadStatus":
			select {
			case listed <- struct{}{}:
			default:
			}
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return mux
}

func portalEnv(home, ca, api, portal string) []string {
	return closedEnv(home, "SSL_CERT_FILE="+ca, "CLOUDALLY_BASE_URL="+api, "CLOUDALLY_PORTAL_URL="+portal,
		"CLOUDALLY_ACCESS_TOKEN="+accessToken, "CLOUDALLY_CLIENT_ID="+clientID, "CLOUDALLY_CLIENT_SECRET="+clientSecret)
}

// TestPortalSessionThroughTheKitStopsOnInterrupt: with the portal on the API's
// own origin (a declared origin for the portal credentials), the handoff token,
// the CSRF token and the session cookie all go out through the kit and the
// session works; a SIGINT in the middle of the run reaches the command's
// context through credkit.Start, so the run stops at its safe point, exits 8 and
// prints how to watch again. No credential reaches the output.
func TestPortalSessionThroughTheKitStopsOnInterrupt(t *testing.T) {
	cliBin, _ := binaries(t)
	rec := &recorder{}
	listed := make(chan struct{}, 1)
	srv, ca := tlsFixture(t, portalMux(rec, listed))
	home := t.TempDir()
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, cliBin, "download", "watch", "--account", "5f1c2a", "--job", "job-9")
			cmd.Env = portalEnv(home, ca, srv.URL, srv.URL)
			var out strings.Builder
			pr, pw := io.Pipe()
			cmd.Stdout, cmd.Stderr = pw, pw
			done := make(chan struct{})
			go func() {
				sc := bufio.NewScanner(pr)
				for sc.Scan() {
					out.WriteString(sc.Text() + "\n")
				}
				close(done)
			}()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-listed:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				_ = pw.Close()
				<-done
				t.Fatalf("the run never reached its status check:\n%s", out.String())
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			_ = pw.Close()
			<-done
			if code := cmd.ProcessState.ExitCode(); code != 8 {
				t.Fatalf("%s: exit %d, want 8 (the job is still pending):\n%s", sig, code, out.String())
			}
			if !strings.Contains(out.String(), "download watch --account 5f1c2a --job job-9") {
				t.Fatalf("%s: the run did not print how to watch again:\n%s", sig, out.String())
			}
			assertAbsent(t, "download watch", out.String(), handoffTok, csrfTok, sessionCk, accessToken, clientID, clientSecret)
		})
	}
	if !rec.carried("accessToken="+handoffTok) || !rec.carried(csrfTok) || !rec.carried(sessionCk) {
		t.Fatalf("the portal session did not run through the kit: %d request(s) seen", rec.hits.Load())
	}
	assertNoFileCarries(t, home, nil, handoffTok, csrfTok, sessionCk, accessToken, clientSecret)
}

// TestPortalCredentialsStayOffOtherOrigins: a portal on an origin that is
// neither the customer portal nor the API never receives the handoff token: the
// kit refuses the request and the run says why.
func TestPortalCredentialsStayOffOtherOrigins(t *testing.T) {
	cliBin, _ := binaries(t)
	apiRec, otherRec := &recorder{}, &recorder{}
	api, ca := tlsFixture(t, portalMux(apiRec, make(chan struct{}, 1)))
	other := httptest.NewTLSServer(portalMux(otherRec, make(chan struct{}, 1)))
	t.Cleanup(other.Close)
	home := t.TempDir()
	out := run(t, cliBin, portalEnv(home, ca, api.URL, other.URL), "", "download", "watch", "--account", "5f1c2a", "--job", "job-9")
	if n := otherRec.hits.Load(); n != 0 {
		t.Fatalf("the other origin received %d request(s)", n)
	}
	if !strings.Contains(out, "portal handoff token was not sent") {
		t.Fatalf("the refusal does not say why:\n%s", out)
	}
	assertAbsent(t, "refused portal", out, handoffTok, accessToken, clientID, clientSecret)
}

// TestAuthRedirectToAnotherOriginIsRefused replaces the wip branch's own
// redirect refusals: a login, or a generated session login, answered with a
// redirect to another origin never sends the password, the client secret or
// anything else held there, and no token that origin would return is saved.
func TestAuthRedirectToAnotherOriginIsRefused(t *testing.T) {
	cliBin, _ := binaries(t)
	trapRec := &recorder{}
	trap := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trapRec.note(r)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"accessToken":"trapped-token-9x8y","refreshToken":"trapped-refresh-7w6v","expiresIn":3600}`)
	}))
	t.Cleanup(trap.Close)
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			api, ca := tlsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, trap.URL+r.URL.Path, status)
			}))
			home := t.TempDir()
			env := closedEnv(home, "SSL_CERT_FILE="+ca, "CLOUDALLY_BASE_URL="+api.URL, "CLOUDALLY_CLIENT_ID="+clientID, "CLOUDALLY_CLIENT_SECRET="+clientSecret)
			out := run(t, cliBin, env, partnerPass+"\n", "auth", "login", "--partner", "--email", "ops@example.com", "--password-stdin")
			out += run(t, cliBin, env, "", "session", "partner-login", "--email", "ops@example.com", "--password", partnerPass, "--json")
			if n := trapRec.hits.Load(); n != 0 {
				t.Fatalf("the redirect target received %d request(s)", n)
			}
			assertAbsent(t, "redirected login", out, partnerPass, clientID, clientSecret)
			assertNoFileCarries(t, home, nil, "trapped-token-9x8y", partnerPass)
		})
	}
}
