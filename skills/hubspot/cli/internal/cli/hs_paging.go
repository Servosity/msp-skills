// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// hsNextCursorPath is where a HubSpot list keeps its next-page cursor (hand-fix
// handfixes.json hubspot-paging-next-after): every CRM list paged by "after"
// answers "paging":{"next":{"after":...}} (CollectionResponse...ForwardPaging,
// press/tools-manifest.json). A path the caller names wins; the generic
// fallbacks still run when this one is absent.
func hsNextCursorPath(cursorParam, nextCursorPath string) string {
	if nextCursorPath == "" && cursorParam == "after" {
		return "paging.next.after"
	}
	return nextCursorPath
}
