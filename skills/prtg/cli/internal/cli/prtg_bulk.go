// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "prtg-pp-cli/internal/fleet"

// prtgActionFailure judges a 2xx write response with the shared reader
// (prtg handfix: bulk-action-outcomes); the generated detectPartialFailure
// calls it first, so every generated write command reports success only when
// every item succeeded, and lists each item otherwise.
func prtgActionFailure(data []byte) *partialFailureReport {
	items, failed, summary := fleet.ResponseFailure(data)
	if failed == 0 {
		return nil
	}
	r := &partialFailureReport{Field: "failure", Message: summary, Details: items}
	for _, it := range items {
		if !it.Failed && it.ID != "" {
			r.ResourceNames = append(r.ResourceNames, it.ID)
		}
	}
	return r
}
