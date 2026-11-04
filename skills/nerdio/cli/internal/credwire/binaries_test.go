// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The tests in this package drive the built binaries (handfixes.json
// credkit-wiring). Set NERDIO_LEAK_BIN_DIR to run them against binaries built
// elsewhere: that is how each was seen failing on the code before its fix.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

// exe is the suffix a built binary carries on this platform.
func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// binaries builds nerdio-cli and nerdio-mcp once per test run.
func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("NERDIO_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "nerdio-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"nerdio-cli", "../../cmd/nerdio-cli"}, {"nerdio-mcp", "../../cmd/nerdio-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out+exe()), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, "nerdio-cli"+exe()), filepath.Join(binDir, "nerdio-mcp"+exe())
}

// sandboxEnv is a closed environment under one home: nothing is inherited from
// the caller but PATH, so no real credential or config reaches a run. The
// network fence: every request for a host but a loopback fake (which Go never
// proxies) goes to a proxy on a closed loopback port, so no run reaches a real
// host.
func sandboxEnv(home string, extra ...string) []string {
	env := []string{
		"HTTPS_PROXY=http://127.0.0.1:9", "HTTP_PROXY=http://127.0.0.1:9",
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "NERDIO_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1", "NERDIO_NO_LEARN=1",
	}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "TEMP", "TMP", "APPDATA", "LOCALAPPDATA"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
	}
	return append(env, extra...)
}

// runCode runs bin with env and args, stdout and stderr together, and returns
// what it printed and its exit code. stdin is empty.
func runCode(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader([]byte(stdin))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s %v: did not exit within 60s; printed %q", filepath.Base(bin), args, out.String())
	}
	return out.String(), cmd.ProcessState.ExitCode()
}
