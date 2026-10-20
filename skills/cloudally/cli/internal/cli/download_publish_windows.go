// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build windows

package cli

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// renameNoReplace moves oldBase to newBase inside dirRel under root and
// fails if newBase exists: MoveFileEx without MOVEFILE_REPLACE_EXISTING
// refuses an existing target in the same step as the move. It is reached
// only on drives without hard links (exFAT, FAT32, some shares); the
// folders on the path were each checked to be plain folders just before.
func renameNoReplace(root *os.Root, _ *os.File, dirRel, oldBase, newBase string) error {
	from, err := windows.UTF16PtrFromString(filepath.Join(root.Name(), dirRel, oldBase))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(root.Name(), dirRel, newBase))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}
