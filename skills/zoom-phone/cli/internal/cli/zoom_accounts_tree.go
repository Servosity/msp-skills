// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// Storage-tree integrity for client-account profiles.

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// An account runs with ZOOM_PHONE_HOME=<account dir> and
// ZOOM_PHONE_CONFIG=<account dir>/config.toml, so every file the generated
// code reads or writes for it lives under that directory:
//
//	config.toml                     the profile (config.Load, save)
//	data/credentials.toml(.lock)    token mint and reload (cliutil credentials)
//	data/data-*.db                  local store (defaultDBPath)
//	config/  state/  cache/         ConfigDir, StateDir (learn journal), CacheDir (http)
//
// A symlink anywhere in that tree, or at the accounts directory or the account
// directory, would let one profile read or overwrite another account's files
// (accounts/acme/data -> accounts/globex/data makes acme authenticate as
// globex). acctVerifyStorageTree refuses that whole class before any API use
// or credential write.

// acctStoragePaths are the account-relative paths the generated code reads or
// writes; each is verified even before the recursive walk reaches it.
var acctStoragePaths = []string{
	acctConfigFileName,
	"data",
	filepath.Join("data", "credentials.toml"),
	filepath.Join("data", "credentials.toml.lock"),
	"config",
	"state",
	"cache",
}

func acctUnsafeStorage(name, path, why string) error {
	return configErr(fmt.Errorf("refusing to use the %q account: %s %s; an account's storage must be plain files and directories inside its own account directory. Inspect and remove the link (and check which account's files it reached) before using this account", name, path, why))
}

// acctVerifyStorageTree refuses (configErr, exit 10) an account whose storage
// could reach outside its own directory: a symlink at the accounts directory,
// the account directory or anywhere below it; any regular file with more than
// one hard link; or a storage path that resolves outside the account
// directory. A tree that does not exist yet is fine.
func acctVerifyStorageTree(name, home string) error {
	home = filepath.Clean(home)
	root := filepath.Dir(home)
	for _, p := range []string{root, home} {
		st, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return configErr(fmt.Errorf("checking the %q account's storage at %s: %w", name, p, err))
		}
		if st.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			return acctUnsafeStorage(name, p, fmt.Sprintf("is a symlink (to %s)", target))
		}
		if !st.IsDir() {
			return acctUnsafeStorage(name, p, "is not a directory")
		}
	}
	canonHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return configErr(fmt.Errorf("resolving the %q account directory %s: %w", name, home, err))
	}
	walkErr := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return configErr(fmt.Errorf("checking the %q account's storage at %s: %w", name, p, err))
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			return acctUnsafeStorage(name, p, fmt.Sprintf("is a symlink (to %s)", target))
		}
		// Any regular file with a second hard link (a database, its -wal/-shm
		// sidecar, a lock, a cache entry, credentials, the profile) shares its
		// contents with a path outside this account.
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return configErr(fmt.Errorf("checking the %q account's storage at %s: %w", name, p, err))
			}
			if n := acctLinkCount(p, info); n > 1 {
				return acctUnsafeStorage(name, p, fmt.Sprintf("has %d hard links (it shares its contents with a path outside this account)", n))
			}
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	for _, rel := range acctStoragePaths {
		p := filepath.Join(home, rel)
		st, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return configErr(fmt.Errorf("checking the %q account's storage at %s: %w", name, p, err))
		}
		if st.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			return acctUnsafeStorage(name, p, fmt.Sprintf("is a symlink (to %s)", target))
		}
		if st.Mode().IsRegular() {
			if n := acctLinkCount(p, st); n > 1 {
				return acctUnsafeStorage(name, p, fmt.Sprintf("has %d hard links (it shares its contents with a path outside this account)", n))
			}
		}
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return configErr(fmt.Errorf("resolving the %q account's storage at %s: %w", name, p, err))
		}
		if !acctResolvedWithin(resolved, canonHome) {
			return acctUnsafeStorage(name, p, fmt.Sprintf("resolves to %s, outside %s", resolved, canonHome))
		}
	}
	return nil
}

// acctResolvedWithin reports whether an already-resolved path is dir or below
// it (case-insensitively: a false match only refuses a run).
func acctResolvedWithin(path, dir string) bool {
	p := strings.ToLower(filepath.Clean(path))
	d := strings.ToLower(filepath.Clean(dir))
	return p == d || strings.HasPrefix(p, strings.TrimSuffix(d, string(filepath.Separator))+string(filepath.Separator))
}
