// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Class holder-incomplete (Codex round 2 on the 4.34.2 reprint): runZero
// answers an error as an x-www-form-urlencoded body whose token field holds a
// URL-shaped credential. The value is a vendor response credential (the
// API key's token, contract section 1), so no output of the program may carry
// it: not stdout, not stderr, not any file the run writes.

import (
	"bytes"
	"encoding/pem"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hiFormToken is the credential the fake answers in its form-encoded error.
const hiFormToken = "https://vault.example/private-key-material"

// hiForms are the spellings of hiFormToken a sink could carry.
func hiForms() []string {
	return []string{
		hiFormToken,
		"private-key-material",
		"vault.example/private",
		url.QueryEscape(hiFormToken),
		strings.ReplaceAll(hiFormToken, "/", `\/`),
	}
}

func TestHolderIncompleteFormBodyTokenNeverPrinted(t *testing.T) {
	const keyID = "e77602e0-3fb8-4734-aef9-fbc6fdcb0fa8" // gitleaks:allow synthetic test fixture
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != apiBase+"/account/keys/"+keyID {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("token=" + hiFormToken))
	}))
	srv.StartTLS()
	t.Cleanup(srv.Close)

	home := t.TempDir()
	ca := filepath.Join(home, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(home, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"RUNZERO_BASE_URL=" + srv.URL + apiBase, "RUNZERO_CA_CERT=" + ca, "SSL_CERT_FILE=" + ca,
		"RUNZERO_API_KEY=" + "CThiFormKeyq7Zp2Wm4Xv9Lr6Ts3Nb8Qd",
		"HTTPS_PROXY=", "HTTP_PROXY=", "https_proxy=", "http_proxy=", "NO_PROXY=*", "no_proxy=*",
	}
	cmd := exec.Command(bbCLI, "account", "get-key", keyID, "--json")
	cmd.Env = env
	cmd.Dir = home
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("account get-key timed out")
	}
	if cmd.ProcessState.ExitCode() == 0 {
		t.Errorf("account get-key exited 0 on a 400 answer")
	}
	for name, out := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
		for _, form := range hiForms() {
			if strings.Contains(out, form) {
				t.Errorf("%s carries the form body's token (%q):\n%s", name, form, out)
			}
		}
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == ca {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		for _, form := range hiForms() {
			if bytes.Contains(b, []byte(form)) {
				rel, _ := filepath.Rel(home, path)
				t.Errorf("file %s carries the form body's token (%q)", rel, form)
			}
		}
		return nil
	})
}
