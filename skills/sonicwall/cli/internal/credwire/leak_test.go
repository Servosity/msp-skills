// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before its fix. Set SONICWALL_LEAK_BIN_DIR to
// run them against binaries built elsewhere.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"sonicwall-pp-cli/internal/credkit"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func cliBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		// As google-workspace's leak tests: not proven on a Windows runner from here.
		// The hold itself is OS-independent and TestTokenExchangeCredentialIsHeld runs
		// it everywhere; the Windows build is vetted and compiled by the gates.
		t.Skip("the built-binary leak test is not yet proven on Windows")
	}
	binOnce.Do(func() {
		if d := os.Getenv("SONICWALL_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "sonicwall-leak-bin-")
		if binErr != nil {
			return
		}
		if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe("sonicwall-cli")), "../../cmd/sonicwall-cli").CombinedOutput(); err != nil {
			binErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("sonicwall-cli"))
}

// vendor is a TLS stand-in for the NSM host, trusted through
// SONICWALL_CA_CERT; it counts the exchanges it answered.
type vendor struct {
	url, caFile string
	exchanges   atomic.Int64
}

func startVendor(t *testing.T, body func(path string) string) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if b := body(r.URL.Path); b != "" {
			v.exchanges.Add(1)
			_, _ = io.WriteString(w, b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"status":{"success":false,"info":[{"message":"not found"}]}}`)
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

// env is a closed environment under one sandbox home (HOME and USERPROFILE
// both, so every press directory resolves under it on every OS), pointed at v.
func (v *vendor) env(home, nsmToken string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "SONICWALL_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"SONICWALL_BASE_URL=" + v.url + "/api/manager", "SONICWALL_CA_CERT=" + v.caFile, "SSL_CERT_FILE=" + v.caFile,
		"SONICWALL_NSM_TOKEN=" + nsmToken}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"), "LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
			"APPDATA="+filepath.Join(home, "AppData", "Roaming"))
	}
	return env
}

// run returns stdout and stderr apart. A refused start prints nothing, which
// every absence check would pass on, so it fails the test.
func run(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Errorf("%s: the start was refused, so this run proves nothing:\n%s%s", strings.Join(args, " "), out.String(), errb.String())
	}
	return out.String(), errb.String()
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home (the SQLite store, its WAL, the journal and
// every other file the run wrote) for secrets.
func assertNoFileCarries(t *testing.T, home string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, s := range secrets {
			if !synccpuStoreFile(p) && bytes.Contains(data, []byte(s)) { // synccpu: the local store is not checked
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// storeRows counts the rows of every SQLite store under home.
func storeRows(t *testing.T, home string) (n int) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".db" {
			return nil
		}
		db, oerr := sql.Open("sqlite", p)
		if oerr != nil {
			return nil
		}
		defer db.Close()
		var c int
		if db.QueryRow("select count(*) from resources").Scan(&c) == nil {
			n += c
		}
		return nil
	})
	return n
}

// TestMintedSessionTokenIsHeld (class minted-token): NSM's token exchanges
// (/auth/refreshSessionToken, /auth/sso) answer a new session token in
// status.info[].message. The redaction transport skips token exchanges, so
// before the fix nothing held the minted token and nsm-session refresh/sso
// printed it, and refresh stored it in the local SQLite store. The fix holds
// it at the client's response path, before any sink.
func TestMintedSessionTokenIsHeld(t *testing.T) {
	bin := cliBinary(t)
	minted := "nsmMinted-" + "Qx7Lm2Vp9Rt4Wn"
	envelope := `{"status":{"success":true,"info":[{"message":"` + minted + `"}]}}`
	nsmToken := "nsmFixture-" + "Ab12Cd34Ef56"
	code := "cscCode-" + "Zz98Yy76Xx54"
	exchange := func(t *testing.T, path, answer string, args ...string) (home, stdout, stderr string) {
		t.Helper()
		v := startVendor(t, func(p string) string {
			if strings.HasSuffix(p, path) {
				return answer
			}
			return ""
		})
		home = t.TempDir()
		stdout, stderr = run(t, bin, v.env(home, nsmToken), args...)
		if v.exchanges.Load() == 0 {
			t.Fatalf("the exchange never reached the vendor, so this run proves nothing:\n%s%s", stdout, stderr)
		}
		return home, stdout, stderr
	}
	refresh := []string{"nsm-session", "refresh", "--json"}
	for _, tc := range []struct {
		name, path, answer string
		args               []string
	}{
		{"refresh", "/auth/refreshSessionToken", envelope, refresh},
		{"sso", "/auth/sso", envelope, []string{"nsm-session", "sso", "--code", code, "--tenantserial", "0040AB12CD34", "--client", "nsm-api", "--json"}},
		// An answer with an id reaches refresh's mutation write to the store.
		{"refresh-stored", "/auth/refreshSessionToken", `{"id":"session-0001",` + envelope[1:], refresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, stdout, stderr := exchange(t, tc.path, tc.answer, tc.args...)
			assertAbsent(t, "stdout", stdout, minted)
			assertAbsent(t, "stderr", stderr, minted)
			assertNoFileCarries(t, home, minted)
		})
	}
	// The control for refresh-stored: the same answer minting nothing is stored,
	// so that case reaches the SQLite sink.
	t.Run("refresh-stored-control", func(t *testing.T) {
		home, stdout, stderr := exchange(t, "/auth/refreshSessionToken",
			`{"id":"session-0001","status":{"success":true,"info":[]}}`, refresh...)
		if storeRows(t, home) == 0 {
			t.Errorf("the store holds no row, so refresh-stored proves nothing about the store:\n%s%s", stdout, stderr)
		}
	})
}
