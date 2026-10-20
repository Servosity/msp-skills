// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build !darwin && !linux && !windows

package cli

import (
	"errors"
	"os"
)

// renameNoReplace has no atomic form here; the caller falls back to
// claiming the final name exclusively.
func renameNoReplace(*os.Root, *os.File, string, string, string) error {
	return errors.ErrUnsupported
}
