// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/config"
)

// Cross-process lock tests run real separate processes: the test binary
// re-executes itself in TestAcctHelperProcess, either as the CLI ("cli") or
// as a holder of one profile lock ("lock", held until its stdin closes). No
// two command trees are ever built concurrently in one process.

const (
	acctHelperModeEnv = "ZOOM_ACCT_HELPER_MODE"
	acctHelperEnvBlob = "ZOOM_ACCT_HELPER_ENV"
)

// acctHelperCarried are restored in the child after TestMain's sandbox resets
// them (testenv.RunSandboxed clears HOME and the per-CLI path variables).
var acctHelperCarried = []string{
	"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	"ZOOM_PHONE_CONFIG", "ZOOM_PHONE_HOME", "ZOOM_PHONE_CONFIG_DIR", "ZOOM_PHONE_DATA_DIR", "ZOOM_PHONE_STATE_DIR", "ZOOM_PHONE_CACHE_DIR",
	"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL",
}

// TestAcctHelperProcess is not a test on its own; see the comment above.
func TestAcctHelperProcess(t *testing.T) {
	mode := os.Getenv(acctHelperModeEnv)
	if mode == "" {
		return
	}
	sandbox := os.Getenv("HOME")
	var env map[string]string
	_ = json.Unmarshal([]byte(os.Getenv(acctHelperEnvBlob)), &env)
	for _, k := range acctHelperCarried {
		if v := env[k]; v != "" {
			_ = os.Setenv(k, v)
		} else {
			_ = os.Unsetenv(k)
		}
	}
	exit := func(code int) {
		if strings.HasPrefix(filepath.Base(sandbox), "pp-cli-test-home-") {
			_ = os.RemoveAll(sandbox)
		}
		os.Exit(code)
	}
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	switch mode {
	case "cli":
		cmd := RootCmd()
		cmd.SetArgs(args)
		cmd.SetIn(os.Stdin)
		cmd.SetOut(os.Stdout)
		cmd.SetErr(os.Stderr)
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exit(ExitCode(err))
		}
		exit(0)
	case "lock":
		release, err := config.ZoomAcquireProfileLock(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			exit(9)
		}
		fmt.Println("LOCKED")
		_, _ = io.Copy(io.Discard, os.Stdin) // hold until the parent closes stdin
		release()
		exit(0)
	}
	exit(64)
}

type acctProc struct {
	stdin  io.WriteCloser
	stderr *bytes.Buffer
	done   chan int
}

// acctStartProc starts a helper process. For mode "lock" it returns once the
// lock is held; close the returned stdin (release) to let it go.
func acctStartProc(t *testing.T, mode, stdin string, args ...string) *acctProc {
	t.Helper()
	carried := map[string]string{}
	for _, k := range acctHelperCarried {
		carried[k] = os.Getenv(k)
	}
	blob, _ := json.Marshal(carried)
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestAcctHelperProcess$", "-test.count=1", "--"}, args...)...)
	cmd.Env = append(os.Environ(), acctHelperModeEnv+"="+mode, acctHelperEnvBlob+"="+string(blob))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p := &acctProc{stdin: in, stderr: &bytes.Buffer{}, done: make(chan int, 1)}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	go func() {
		r := bufio.NewReader(out)
		if mode == "lock" {
			if line, _ := r.ReadString('\n'); strings.TrimSpace(line) == "LOCKED" {
				close(ready)
			}
		}
		_, _ = io.Copy(io.Discard, r)
		code := 0
		if err := cmd.Wait(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}
		p.done <- code
	}()
	if stdin != "" {
		_, _ = io.WriteString(in, stdin)
	}
	if mode == "lock" {
		select {
		case <-ready:
		case code := <-p.done:
			t.Fatalf("lock holder exited %d before locking: %s", code, p.stderr.String())
		case <-time.After(20 * time.Second):
			t.Fatal("lock holder never locked")
		}
	} else {
		_ = in.Close()
	}
	t.Cleanup(func() {
		_ = in.Close()
		select {
		case <-p.done:
		case <-time.After(20 * time.Second):
		}
	})
	return p
}

func (p *acctProc) release() { _ = p.stdin.Close() }

// acctNotFinished asserts none of procs exits within d.
func acctNotFinished(t *testing.T, d time.Duration, procs ...*acctProc) {
	t.Helper()
	deadline := time.After(d)
	for {
		for i, p := range procs {
			select {
			case code := <-p.done:
				p.done <- code
				t.Fatalf("process %d finished (exit %d) while the profile lock was held: %s", i, code, p.stderr.String())
			default:
			}
		}
		select {
		case <-deadline:
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *acctProc) wait(t *testing.T) int {
	t.Helper()
	select {
	case code := <-p.done:
		p.done <- code
		return code
	case <-time.After(30 * time.Second):
		t.Fatal("process never finished")
		return -1
	}
}

func acctConnectArgs(name, secretTag string) []string {
	return []string{"accounts", "connect", name, "--account-id", "ACCT_" + secretTag, "--client-id", "CID_" + secretTag, "--client-secret-stdin"}
}

// Codex round 11 (item 2, race-free): two first-time connects in separate
// processes, queued behind a lock held by a third: exactly one wins.
func TestAcctProcConcurrentFirstConnectsExactlyOneWins(t *testing.T) {
	acctTestHome(t)
	path, err := acctConfigPath("newco")
	if err != nil {
		t.Fatal(err)
	}
	holder := acctStartProc(t, "lock", "", path)
	a := acctStartProc(t, "cli", "secret-1\n", acctConnectArgs("newco", "1")...)
	b := acctStartProc(t, "cli", "secret-2\n", acctConnectArgs("newco", "2")...)
	acctNotFinished(t, 1500*time.Millisecond, a, b)
	holder.release()
	codes := map[int]int{a.wait(t): 1}
	codes[b.wait(t)]++
	if codes[0] != 1 || codes[2] != 1 {
		t.Fatalf("exit codes = %v, want exactly one 0 and one 2 (a: %s / b: %s)", codes, a.stderr.String(), b.stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil || (cfg.ClientSecret != "secret-1" && cfg.ClientSecret != "secret-2") {
		t.Fatalf("winner's profile unreadable: %v", err)
	}
	if want := "secret-" + strings.TrimPrefix(cfg.ClientID, "CID_"); cfg.ClientSecret != want {
		t.Fatalf("profile mixes two writers: client %q secret %q", cfg.ClientID, cfg.ClientSecret)
	}
}

// Codex round 11 (item 1): the account directory is removed (and recreated)
// while a writer already waits on the profile lock. The lock lives outside
// the removable tree, so a writer arriving after the recreation still queues
// behind the holder, and exactly one of the two waiting writers wins.
func TestAcctProcRemoveRecreateKeepsLock(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	home := filepath.Dir(profile)

	holder := acctStartProc(t, "lock", "", profile)
	waiter := acctStartProc(t, "cli", "secret-W\n", acctConnectArgs("acme", "W")...)
	time.Sleep(1 * time.Second) // the waiter reaches the lock and opens it

	// The profile is removed while the holder owns the lock (as `accounts
	// remove` would, under the lock), then a new writer arrives.
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	late := acctStartProc(t, "cli", "secret-L\n", acctConnectArgs("acme", "L")...)
	acctNotFinished(t, 1500*time.Millisecond, waiter, late)

	lockFile := config.ZoomProfileLockPath(profile) + ".lock"
	if _, err := os.Stat(lockFile); err != nil {
		t.Fatalf("profile lock file %s did not survive the removal: %v", lockFile, err)
	}
	holder.release()
	codes := map[int]int{waiter.wait(t): 1}
	codes[late.wait(t)]++
	if codes[0] != 1 || codes[2] != 1 {
		t.Fatalf("exit codes = %v, want exactly one 0 and one 2 (waiter: %s / late: %s)", codes, waiter.stderr.String(), late.stderr.String())
	}
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "secret-" + strings.TrimPrefix(cfg.ClientID, "CID_"); cfg.ClientSecret != want || (cfg.ClientID != "CID_W" && cfg.ClientID != "CID_L") {
		t.Fatalf("profile mixes writers or was silently overwritten: client %q", cfg.ClientID)
	}
}

// `accounts remove` takes the lock in its own process and leaves the lock
// file in place.
func TestAcctProcRemoveWaitsForLockAndKeepsIt(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	holder := acctStartProc(t, "lock", "", profile)
	rm := acctStartProc(t, "cli", "", "accounts", "remove", "acme")
	acctNotFinished(t, 1500*time.Millisecond, rm)
	holder.release()
	if code := rm.wait(t); code != 0 {
		t.Fatalf("remove: exit=%d %s", code, rm.stderr.String())
	}
	if _, err := os.Stat(filepath.Dir(profile)); !os.IsNotExist(err) {
		t.Fatalf("account not removed: %v", err)
	}
	if _, err := os.Stat(config.ZoomProfileLockPath(profile) + ".lock"); err != nil {
		t.Fatalf("remove deleted the profile lock: %v", err)
	}
}
