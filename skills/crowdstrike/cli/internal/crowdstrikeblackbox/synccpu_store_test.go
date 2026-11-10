// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

import "strings"

// synccpuStoreFile: a SQLite store file. The kit no longer checks what the
// local store holds (credential leak contract section 5, owner GO
// 2026-11-09), so a scan for a credential at rest skips it; every other
// file is still scanned, and output and requests are checked as before.
func synccpuStoreFile(path string) bool {
	for _, s := range []string{".db", ".db-wal", ".db-shm", ".db-journal"} {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
