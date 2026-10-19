// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "fmt"

// jsmSkipDependent gates request and comment dependents in sync (hook in
// syncDependentResources). comment and comment_attachment run only when named
// in --resources: one call per request, for every request ever synced. The
// other request dependents never run: the generator keys them on issue_id,
// which request rows lack, so they would print a false "parent table is
// empty" and store nothing (sla and transition ids would also collide).
// Naming one returns an error result so the summary and exit policy see it.
// ponytail: drop the second case when the press keys them on issueId.
func jsmSkipDependent(name, parent string, allow map[string]bool) (bool, []syncResult) {
	switch {
	case name == "comment" || parent == "comment":
		return !allow[name], nil
	case parent == "request":
		if allow[name] {
			return true, []syncResult{{Resource: name, Err: fmt.Errorf("sync does not store request %s rows; read them live per request (request --help)", name)}}
		}
		return true, nil
	}
	return false, nil
}
