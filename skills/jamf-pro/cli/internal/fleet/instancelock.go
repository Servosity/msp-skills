// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: instance lifetimes versus removal and re-registration.

package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Every process acting as an instance holds that instance's lock shared for
// its whole life; removing the instance or pruning its old homes needs the
// lock exclusively. So a removal never purges a home a running command is
// writing a token into, and a command never starts on an entry that was
// just removed or re-registered (HoldInstance re-checks the entry after it
// holds the lock).

var held struct {
	sync.Mutex
	files []*os.File
}

func lockFile(base, name string) (*os.File, error) {
	dir := filepath.Join(base, "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Clean(filepath.Join(dir, name+".lock")), os.O_CREATE|os.O_RDWR, 0o600) // name is a validated instance slug
}

// HoldInstance takes p's lock shared for the life of this process and
// verifies the registry still holds exactly p.
func HoldInstance(base string, p Instance) error {
	if !slugPattern.MatchString(p.Name) {
		return fmt.Errorf("invalid instance name %q", p.Name)
	}
	f, err := lockFile(base, p.Name)
	if err != nil {
		return err
	}
	if err := lockShared(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("instance %q is being removed or re-registered; try again", p.Name)
	}
	reg, err := Load()
	if err != nil {
		_ = f.Close()
		return err
	}
	if cur, ok := reg.Get(p.Name); !ok || cur.Fingerprint() != p.Fingerprint() {
		_ = f.Close()
		return fmt.Errorf("instance %q was removed or changed in the registry after this command started; run it again", p.Name)
	}
	held.Lock()
	held.files = append(held.files, f)
	held.Unlock()
	return nil
}

// WithInstanceExclusive runs fn (given the fleet directory) while no process
// acts as the instance. It does not wait: a busy instance is an error.
func WithInstanceExclusive(name string, fn func(base string) error) error {
	if !slugPattern.MatchString(name) {
		return fmt.Errorf("invalid instance name %q", name)
	}
	base, err := BaseDir()
	if err != nil {
		return err
	}
	f, err := lockFile(base, name)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockExclusive(f); err != nil {
		return fmt.Errorf("instance %q is in use by a running command or MCP server; try again when it finishes", name)
	}
	return fn(base)
}

// ReleaseHeld drops the shared locks this process holds. The CLI calls it
// when an execution starts (none are held yet in a real process); tests call
// it between phases that stand for separate processes.
func ReleaseHeld() { ReleaseHeldForTest() }

// ReleaseHeldForTest drops the shared locks this process holds.
func ReleaseHeldForTest() {
	held.Lock()
	defer held.Unlock()
	for _, f := range held.files {
		_ = f.Close()
	}
	held.files = nil
}
