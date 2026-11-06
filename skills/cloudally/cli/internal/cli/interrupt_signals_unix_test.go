//go:build !windows

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

const signalHelperEnv = "CLOUDALLY_SIGNAL_HELPER"

// TestSignalHelperProcess is not a test on its own: the signal tests re-run
// the test binary with signalHelperEnv set, and this then runs the real
// Execute (as main does) on a download watch whose status check blocks, so
// the parent can deliver a real signal to a real run.
func TestSignalHelperProcess(t *testing.T) {
	if os.Getenv(signalHelperEnv) != "1" {
		t.Skip("helper process for TestInterruptSignalsCancelTheRun")
	}
	isolateAuthState(t)
	s := newWatchStub(t, "block")
	t.Setenv("CLOUDALLY_BASE_URL", s.api.URL)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "partner-token")
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	go func() {
		<-s.listed
		fmt.Println("helper: ready")
	}()
	os.Args = []string{"cloudally-cli", "download", "watch", "--account", "5f1c2a", "--job", "job-9", "--portal-url", s.portal.URL}
	err := Execute()
	fmt.Printf("helper: exit=%d\n", ExitCode(err))
	if err != nil {
		fmt.Printf("helper: error=%s\n", strings.ReplaceAll(err.Error(), "\n", " "))
	}
}

// Closing a terminal or dropping an SSH session sends SIGHUP. Like Ctrl-C
// and SIGTERM it must cancel the run's context, so a run in the middle of
// creating jobs stops at a safe point (download watch then exits 8 with
// the command to run again) instead of being killed outright.
func TestInterruptSignalsCancelTheRun(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestSignalHelperProcess$", "-test.v", "-test.count=1")
			cmd.Env = append(os.Environ(), signalHelperEnv+"=1")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			lines := make(chan string, 64)
			go func() {
				sc := bufio.NewScanner(stdout)
				for sc.Scan() {
					lines <- sc.Text()
				}
				close(lines)
			}()
			waitFor := func(prefix string) (string, bool) {
				deadline := time.After(30 * time.Second)
				for {
					select {
					case l, ok := <-lines:
						if !ok {
							return "", false
						}
						if strings.HasPrefix(l, prefix) {
							return l, true
						}
					case <-deadline:
						return "", false
					}
				}
			}
			if _, ok := waitFor("helper: ready"); !ok {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("the helper run never reached its status check")
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			exitLine, ok := waitFor("helper: exit=")
			errLine, _ := waitFor("helper: error=")
			waitErr := cmd.Wait()
			if !ok {
				t.Fatalf("%s killed the run instead of cancelling it (wait: %v)", sig, waitErr)
			}
			if exitLine != fmt.Sprintf("helper: exit=%d", ExitJobPending) {
				t.Fatalf("%s: %s, want exit=%d (%s)", sig, exitLine, ExitJobPending, errLine)
			}
			if !strings.Contains(errLine, "download watch --account 5f1c2a --job job-9") {
				t.Fatalf("%s: the run did not print how to watch again: %s", sig, errLine)
			}
		})
	}
}
