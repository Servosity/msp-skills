// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): an atomic replace through the credential kit
// (credkit-wiring), for a file that is not a declared credential source.

package cliutil

import (
	"fmt"
	"os"
	"path/filepath"

	"superops-pp-cli/internal/credkit"
)

// HeldAtomicWrite replaces path with data, checked by the kit as one record, so
// a reader never sees part of it. A withheld write leaves path as it was.
func HeldAtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating private file dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary private file: %w", err)
	}
	name := tmp.Name()
	_ = tmp.Close()
	if err := credkit.WriteFile(name, data, 0o600); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := renamePrivateFileWithRetry(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("publishing private file: %w", err)
	}
	return nil
}
