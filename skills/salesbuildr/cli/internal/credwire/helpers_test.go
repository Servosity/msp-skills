// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// Shared helpers of the credential leak tests on the built binaries
// (handfixes.json credkit-wiring). Set SALESBUILDR_LEAK_BIN_DIR to run them
// against binaries built elsewhere.

import (
	"bytes"
	"context"
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
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

// exe is the file name a built binary has on this platform.
func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// binaries builds salesbuildr-cli and salesbuildr-mcp once per test process.
func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("SALESBUILDR_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "salesbuildr-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"salesbuildr-cli", "../../cmd/salesbuildr-cli"}, {"salesbuildr-mcp", "../../cmd/salesbuildr-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("salesbuildr-cli")), filepath.Join(binDir, exe("salesbuildr-mcp"))
}

// closedEnv is a closed environment under one sandbox home: no variable of
// the operator's reaches the binary but PATH and what Windows needs to start
// one. SALESBUILDR_HOME roots the config, data, state and cache directories in
// the sandbox, so no run reads or writes the operator's real profile.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_STATE_HOME=", "XDG_CACHE_HOME=",
		"SALESBUILDR_HOME=" + filepath.Join(home, "salesbuildr-home")}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "SYSTEMDRIVE", "TEMP", "TMP", "COMSPEC", "PATHEXT"} {
			env = append(env, k+"="+os.Getenv(k))
		}
		env = append(env, "APPDATA="+filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"))
	}
	return append(env, extra...)
}

// runSplit runs bin with args under env and returns what it wrote to stdout
// and to stderr, and its exit code.
func runSplit(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runSplitIn(t, bin, env, "", args...)
}

// runSplitIn is runSplit with stdin.
func runSplitIn(t *testing.T, bin string, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if ctx.Err() != nil {
		t.Fatalf("%s %s: timed out", filepath.Base(bin), strings.Join(args, " "))
	}
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

// filesUnder is every regular file under root with its content.
func filesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, rerr := os.ReadFile(path); rerr == nil {
			files[path] = string(data)
		}
		return nil
	})
	return files
}

// fakeAPI is an https Salesbuildr fake that answers every request with body,
// and the PEM file of its certificate for SALESBUILDR_CA_CERT. The kit sends a
// credential only over https, so the fake is TLS. seen counts the requests
// that carried the api-key header with the value key (when key is not empty).
func fakeAPI(t *testing.T, body, key string) (baseURL, caFile string, seen func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if key != "" && r.Header.Get("api-key") == key {
			mu.Lock()
			n++
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv.URL + "/public-api", caFile, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}
