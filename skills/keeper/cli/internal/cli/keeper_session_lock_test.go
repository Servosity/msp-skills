// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"testing"

	"keeper-pp-cli/internal/cliutil"
	"keeper-pp-cli/internal/cliutil/testenv"
)

// keeper handfix mc-context-serialized (class cross-tenant-context-race): the session
// lock is per user and per Service Mode instance. --home, KEEPER_HOME and the per-kind
// variables relocate this CLI's own files and must not relocate the lock, or two runs
// against one global session stop excluding each other. internal/keeperblackbox drives
// the race itself on the built binary.
func TestSessionLockFileIgnoresHomeAndLoopbackSpelling(t *testing.T) {
	testenv.Isolate(t)
	const base = "https://keeper.example.com:9090"
	want, err := sessionLockFile(base)
	if err != nil {
		t.Fatal(err)
	}
	lockUnder := func(t *testing.T, relocate func(t *testing.T)) string {
		t.Helper()
		relocate(t)
		got, err := sessionLockFile(base)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for name, relocate := range map[string]func(t *testing.T){
		"--home A": func(t *testing.T) {
			restore, err := cliutil.SetHomeOverride(filepath.Join(t.TempDir(), "a"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restore)
		},
		"--home B": func(t *testing.T) {
			restore, err := cliutil.SetHomeOverride(filepath.Join(t.TempDir(), "b"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restore)
		},
		"XDG_CACHE_HOME A":  func(t *testing.T) { t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "a")) },
		"XDG_CACHE_HOME B":  func(t *testing.T) { t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "b")) },
		"HOME":              func(t *testing.T) { t.Setenv("HOME", t.TempDir()) },
		"TMPDIR":            func(t *testing.T) { t.Setenv("TMPDIR", t.TempDir()) },
		"KEEPER_HOME A":     func(t *testing.T) { t.Setenv("KEEPER_HOME", filepath.Join(t.TempDir(), "a")) },
		"KEEPER_HOME B":     func(t *testing.T) { t.Setenv("KEEPER_HOME", filepath.Join(t.TempDir(), "b")) },
		"KEEPER_DATA_DIR":   func(t *testing.T) { t.Setenv("KEEPER_DATA_DIR", t.TempDir()) },
		"KEEPER_STATE_DIR":  func(t *testing.T) { t.Setenv("KEEPER_STATE_DIR", t.TempDir()) },
		"KEEPER_CACHE_DIR":  func(t *testing.T) { t.Setenv("KEEPER_CACHE_DIR", t.TempDir()) },
		"KEEPER_CONFIG_DIR": func(t *testing.T) { t.Setenv("KEEPER_CONFIG_DIR", t.TempDir()) },
	} {
		t.Run(name, func(t *testing.T) {
			if got := lockUnder(t, relocate); got != want {
				t.Errorf("the lock moved: %s, want %s", got, want)
			}
		})
	}
	loopback, err := sessionLockFile("https://localhost:9090")
	if err != nil {
		t.Fatal(err)
	}
	for _, spelling := range []string{"https://127.0.0.1:9090", "https://[::1]:9090", "https://LOCALHOST:9090", "https://localhost:9090/some/path"} {
		got, err := sessionLockFile(spelling)
		if err != nil {
			t.Fatal(err)
		}
		if got != loopback {
			t.Errorf("%s has its own lock %s, want %s", spelling, got, loopback)
		}
	}
	for _, other := range []string{base, "https://keeper-two.example.com:9090", "https://localhost:9091", "http://localhost:9090"} {
		got, err := sessionLockFile(other)
		if err != nil {
			t.Fatal(err)
		}
		if got == loopback {
			t.Errorf("%s shares the loopback instance's lock", other)
		}
	}
	if got, err := sessionLockFile("https://keeper-two.example.com:9090"); err != nil || got == want {
		t.Errorf("another host shares the lock: %s, %v", got, err)
	}
	for _, bad := range []string{"", "://bad", "https://"} {
		if got, err := sessionLockFile(bad); err == nil {
			t.Errorf("sessionLockFile(%q) = %s: no lock must mean no read", bad, got)
		}
	}
}

// Two spellings of one instance take one lock, or two runs against one global session
// do not exclude each other. A different port or host is another instance.
func TestSessionLockFileIsOnePerInstance(t *testing.T) {
	testenv.Isolate(t)
	lock := func(u string) string {
		t.Helper()
		got, err := sessionLockFile(u)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, same := range [][2]string{
		{"https://localhost", "https://localhost:443"},
		{"https://127.0.0.1", "https://localhost:443"},
		{"https://[::1]:9090", "https://LOCALHOST:9090"},
		{"https://localhost.:9090", "https://127.0.0.1:9090"},
		{"http://localhost", "http://localhost:80"},
		{"https://keeper.example.com.", "https://keeper.example.com"},
		{"https://KEEPER.example.com:443", "https://keeper.example.com/"},
	} {
		if lock(same[0]) != lock(same[1]) {
			t.Errorf("%s and %s take different locks", same[0], same[1])
		}
	}
	for _, other := range [][2]string{
		{"https://localhost:9090", "https://localhost:9091"},
		{"https://a.example.com", "https://b.example.com"},
	} {
		if lock(other[0]) == lock(other[1]) {
			t.Errorf("%s and %s share a lock", other[0], other[1])
		}
	}
}
