// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// A concrete path matches its declared template, a {catchAll} segment takes
// the rest of the path, and a query never hides a match (handfixes.json
// mutating-gets-are-writes).
func TestAMutatingGETPathMatchesEveryConcreteForm(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/v1/maintenance-emails/0f8c/jobs/postpone": true,
		"/API/v1/Maintenance-Sessions/7/Cancel?x=1":     true,
		"/plugins/api/v1/5/tickets":                     true,
		"/plugins/api/v1/5/tickets/42/close":            true,
		"/plugins/api/v1/5":                             false,
		"/api/v1/maintenance-sessions/7":                false,
		"/api/v1/maintenance-sessions//cancel":          false,
	} {
		if got := IsMutatingGETPath(path); got != want {
			t.Errorf("IsMutatingGETPath(%q) = %v, want %v", path, got, want)
		}
	}
}
