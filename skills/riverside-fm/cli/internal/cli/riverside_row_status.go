// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "fmt"

// rowStatusFailed reports whether a per-row status from media refresh or clips
// harvest means the row's work did not happen. It allowlists what is not a
// failure, so a status added later counts as one until someone says otherwise.
func rowStatusFailed(status string) bool {
	switch status {
	case "", "downloaded", "ready", "not-rendered", "metadata-only":
		return false
	}
	return true
}

// rowsIncomplete is the error a per-row command returns once its rows are
// printed: every row that failed makes the whole run exit non-zero, the same
// way bulk export and ready do.
func rowsIncomplete(command string, statuses []string) error {
	failed, first := 0, ""
	for _, s := range statuses {
		if rowStatusFailed(s) {
			if failed == 0 {
				first = s
			}
			failed++
		}
	}
	if failed == 0 {
		return nil
	}
	return apiErr(fmt.Errorf("%s incomplete: %d of %d row(s) failed. First: %s", command, failed, len(statuses), first))
}
