// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: clusters.toml is read and permission-checked through one file
// descriptor, so the bytes parsed are the bytes whose owner and mode were
// verified (no swap between the check and the read).

//go:build !windows

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func readClustersFile(path string) ([]byte, error) {
	refuse := func(why string) error { return fmt.Errorf("refusing to use %s: %s", path, why) }
	if err := verifyClustersDir(filepath.Dir(path)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, refuse(err.Error())
	}
	// O_NONBLOCK: a FIFO planted at the path must not block the open; the
	// regular-file check below then rejects it.
	// #nosec G304 -- path is this CLI's own clusters.toml (config dir or PROXMOX_CLUSTERS_FILE); type, mode and owner are checked on this descriptor.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, refuse("not a regular file")
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return nil, refuse(fmt.Sprintf("file is group/world-accessible (mode %#o); run: chmod 600 %s", perm, path))
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return nil, refuse("file is owned by another user")
	}
	return io.ReadAll(f)
}

// verifyClustersDir rejects a parent directory other users could use to swap
// the file (group/world-writable without the sticky bit, or owned by someone
// other than this user or root).
func verifyClustersDir(dir string) error {
	di, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if di.Mode().Perm()&0o022 != 0 && di.Mode()&os.ModeSticky == 0 {
		return errors.New("its directory is writable by other users")
	}
	if st, ok := di.Sys().(*syscall.Stat_t); ok && st.Uid != 0 && int(st.Uid) != os.Getuid() {
		return errors.New("its directory is owned by another user")
	}
	return nil
}
