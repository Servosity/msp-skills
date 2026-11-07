// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). Class nested-stream-past-decoder-bound
// (LEDGER-NOTES section 5 item 1, the print's Codex round 2): an inner
// compressed stream with bytes before and after it, inside an outer stream's
// output, was never searched by the connector's own layer, so a response that
// carried the API key that way printed it. The shared credential kit reads a
// compressed stream at every byte offset of every view (contract section 3),
// so the unit is withheld. Driven through the built CLI against an httptest
// TLS server, the way an operator runs it.

package credwire

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// nestedKey is the API key the fake answers with. No 8-character piece of it
// occurs in ordinary output, a path or a test name.
const nestedKey = "Kq7Wv3Rk9L" + "m2Xc4TbY8p"

// buildEnv is the environment the test binary started with, read before any
// test points HOME at a sandbox, so go build keeps the caller's module and
// build caches.
var buildEnv = os.Environ()

var nestedBuilt struct {
	once sync.Once
	dir  string
	cli  string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if nestedBuilt.dir != "" {
		_ = os.RemoveAll(nestedBuilt.dir)
	}
	os.Exit(code)
}

// nestedCLI builds the CLI once per test run.
func nestedCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the CLI binary")
	}
	nestedBuilt.once.Do(func() {
		dir, err := os.MkdirTemp("", "pagerduty-credwire-bin-")
		if err != nil {
			nestedBuilt.err = err
			return
		}
		nestedBuilt.dir = dir
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		out := filepath.Join(dir, "pagerduty-cli"+exe)
		cmd := exec.Command("go", "build", "-o", out, "./cmd/pagerduty-cli")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = buildEnv
		if b, err := cmd.CombinedOutput(); err != nil {
			nestedBuilt.err = fmt.Errorf("go build pagerduty-cli: %v\n%s", err, b)
			return
		}
		nestedBuilt.cli = out
	})
	if nestedBuilt.err != nil {
		t.Fatal(nestedBuilt.err)
	}
	return nestedBuilt.cli
}

func rawDeflate(t *testing.T, b []byte) []byte {
	t.Helper()
	var o bytes.Buffer
	w, err := flate.NewWriter(&o, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(b)
	_ = w.Close()
	return o.Bytes()
}

func gzipped(t *testing.T, b []byte) []byte {
	t.Helper()
	var o bytes.Buffer
	w := gzip.NewWriter(&o)
	_, _ = w.Write(b)
	_ = w.Close()
	return o.Bytes()
}

// nestedCarrier is base64 of an outer stream whose output holds a raw DEFLATE
// stream of inner with bytes before and after it.
func nestedCarrier(t *testing.T, outer func(*testing.T, []byte) []byte, inner string) string {
	t.Helper()
	var mid []byte
	mid = append(mid, "hdr:\x00\x01\x02"...)
	mid = append(mid, rawDeflate(t, []byte(inner))...)
	mid = append(mid, "\x00\x00trailer-bytes"...)
	return base64.StdEncoding.EncodeToString(outer(t, mid))
}

// nestedServer answers GET /services/PSVC001 with name as the service name,
// counting the requests that carried the key.
func nestedServer(t *testing.T, name string) (*httptest.Server, string, *int) {
	t.Helper()
	return kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/PSVC001" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"service":{"id":"PSVC001","type":"service","summary":"Checkout","name":%q}}`, name)
	})
}

// kitServer is an httptest TLS server running handler, counting the requests
// that carried the key, and the path of a PEM file holding its certificate.
func kitServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string, *int) {
	t.Helper()
	var mu sync.Mutex
	authed := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), nestedKey) {
			mu.Lock()
			authed++
			mu.Unlock()
		}
		handler(w, r)
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, ca, &authed
}

// runNested runs `services get PSVC001` against the fake with the key in the
// environment and returns everything the CLI printed.
func runNested(t *testing.T, name string) (out string, authed int) {
	t.Helper()
	srv, ca, seen := nestedServer(t, name)
	out = runKitCLI(t, srv, ca, "services", "get", "PSVC001", "--json")
	return out, *seen
}

// runKitCLI runs the built CLI with args against srv, the key in the
// environment and a fresh home, and returns everything it printed.
func runKitCLI(t *testing.T, srv *httptest.Server, ca string, args ...string) string {
	t.Helper()
	out, _ := kitRun(t, srv, ca, testenv.Isolate(t), args...)
	return out
}

// kitRun runs the built CLI with args against srv, the key in the
// environment and home as its home, and returns everything it printed and
// its exit code.
func kitRun(t *testing.T, srv *httptest.Server, ca, home string, args ...string) (string, int) {
	t.Helper()
	return kitRunIn(t, srv, ca, home, "", args...)
}

// kitEnv is the environment a built binary runs with against srv: no PagerDuty
// variable of the caller's, the key, and home as its home.
func kitEnv(srv *httptest.Server, ca, home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "PAGERDUTY_") || strings.HasPrefix(k, "PRINTING_PRESS_") || k == "CREDKIT_HELD" {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"PAGERDUTY_API_KEY="+nestedKey,
		"PAGERDUTY_BASE_URL="+srv.URL,
		"PAGERDUTY_CA_CERT="+ca,
		"PAGERDUTY_HOME="+home,
		"PAGERDUTY_NO_LEARN=1",
	)
}

// kitRunIn is kitRun with stdin.
func kitRunIn(t *testing.T, srv *httptest.Server, ca, home, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(nestedCLI(t), args...)
	cmd.Env = kitEnv(srv, ca, home)
	cmd.Dir = home
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%v timed out", args)
	}
	return stdout.String() + "\n" + stderr.String(), cmd.ProcessState.ExitCode()
}

// TestNestedStreamInsideAnOuterStreamIsWithheld is the class's own input: the
// key as raw DEFLATE with bytes before and after it, inside a gzip stream and
// inside a raw DEFLATE stream, base64 in a JSON string of an API response.
func TestNestedStreamInsideAnOuterStreamIsWithheld(t *testing.T) {
	for name, outer := range map[string]func(*testing.T, []byte) []byte{
		"gzip outer":        gzipped,
		"raw deflate outer": rawDeflate,
	} {
		t.Run(name, func(t *testing.T) {
			carrier := nestedCarrier(t, outer, "api_key="+nestedKey+";")
			out, authed := runNested(t, carrier)
			if authed == 0 {
				t.Fatalf("the CLI never sent the key to the fake, so nothing was tested:\n%s", out)
			}
			if strings.Contains(out, carrier) || strings.Contains(out, nestedKey) {
				t.Fatalf("the CLI printed a response carrying the API key in a nested stream:\n%s", out)
			}
		})
	}
}

// TestNestedStreamWithoutTheKeyPrints is the control: the same shape carrying
// no held value prints, so the test above proves a withhold, not a failure.
func TestNestedStreamWithoutTheKeyPrints(t *testing.T) {
	carrier := nestedCarrier(t, gzipped, "status=operational;region=primary")
	out, authed := runNested(t, carrier)
	if authed == 0 {
		t.Fatalf("the CLI never sent the key to the fake:\n%s", out)
	}
	if !strings.Contains(out, carrier) {
		t.Fatalf("a response carrying no held value was not printed:\n%s", out)
	}
}
