// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is one a review named. Set
// ROCKETCYBER_LEAK_BIN_DIR to run them against binaries built elsewhere.

import (
	"bytes"
	"context"
	"fmt"
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

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("ROCKETCYBER_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "rocketcyber-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"rocketcyber-cli", "../../cmd/rocketcyber-cli"}, {"rocketcyber-mcp", "../../cmd/rocketcyber-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("rocketcyber-cli")), filepath.Join(binDir, exe("rocketcyber-mcp"))
}

// closedEnv is a closed environment under one sandbox home: no variable of the
// operator's reaches the binary but PATH and what Windows needs to start one.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_STATE_HOME=", "XDG_CACHE_HOME=", "ROCKETCYBER_HOME="}
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if ctx.Err() != nil {
		t.Fatalf("%s %s: timed out", filepath.Base(bin), strings.Join(args, " "))
	}
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

// unusableStateDir is a state directory the program cannot use: /dev/null on a
// Unix system (the reviewed input), a regular file on Windows.
func unusableStateDir(t *testing.T) string {
	if runtime.GOOS != "windows" {
		return "/dev/null"
	}
	p := filepath.Join(t.TempDir(), "state-is-a-file")
	if err := os.WriteFile(p, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestHalvesAcrossProcessesAreWithheld is the open round-2 class
// credential-split-across-processes, closed through the kit. Two runs each
// print one half of the API token (as an unknown command, which the error
// repeats) to one log. Each half is a piece of 8 or more characters of a held
// value, so each run withholds its own half without any state shared between
// runs: a state directory that cannot be used (the reviewed input) must still
// withhold, never fall back to a per-run check that lets a half through. A
// usable state directory is the control, and so is a value of the same shape
// that is no piece of the token: it must reach stderr, so the runs print what
// they are given. The kit may drop a withheld line without its notice, so an
// empty stderr is no failure.
func TestHalvesAcrossProcessesAreWithheld(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "QQQQQQQQQQQQQQQQZZZZZZZZZZZZZZZZ"
	halves := []string{token[:16], token[16:]}
	for _, c := range []struct{ name, stateDir string }{
		{"unusable state dir", unusableStateDir(t)},
		{"usable state dir", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := closedEnv(home, "ROCKETCYBER_API_TOKEN="+token, "ROCKETCYBER_STATE_DIR="+c.stateDir)
			var log strings.Builder
			const control = "XXXXXXXXXXXXXXXX"
			if _, stderr, _ := runSplit(t, cliBin, env, control, "--no-learn"); !strings.Contains(stderr, control) {
				t.Fatalf("rocketcyber-cli %s: stderr %q does not repeat the argument, so this run proves nothing", control[:4], stderr)
			}
			for _, half := range halves {
				_, stderr, code := runSplit(t, cliBin, env, half, "--no-learn")
				if code == 0 {
					t.Errorf("rocketcyber-cli %s: exit 0, want the unknown-command refusal", half[:4])
				}
				log.WriteString(stderr)
			}
			for _, piece := range []string{halves[0], halves[1], token[:8], token[24:]} {
				if strings.Contains(log.String(), piece) {
					t.Errorf("the combined stderr of both runs carries %q (a piece of the API token):\n%s", piece[:4]+"...", log.String())
				}
			}
		})
	}
}
