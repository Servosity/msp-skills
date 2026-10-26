// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strconv"
	"strings"

	"ninjaone-pp-cli/internal/ninjapage"
)

// syncRequestedPageLimit is the page size every sync continuation decision
// compares a page against: the limit parameter as sent (after --param /
// --resource-param overrides), capped by the spec default. Both sync loops set
// pageSize.limit from it once the request params are final, so the after-id
// fallback on a full bare-array page, the end of an object-offset walk on a
// short page and the rendered short-page checks never read a full page of an
// overridden smaller size as the last page (a 10-row page under pageSize=10 is
// full, not short). Capping by the default keeps an oversized request safe: a
// page NinjaOne clamps below an out-of-range pageSize still counts as full
// unless it is short of the default too, so the walk asks once more instead of
// stopping early. Class sync-effective-page-size (reapply/pagesize.py); the rule is
// ninjapage.Limit, shared by every NinjaOne paging reader.
func syncRequestedPageLimit(params map[string]string, d paginationDefaults) int {
	requested := 0
	if d.limitParam != "" {
		requested, _ = strconv.Atoi(strings.TrimSpace(params[d.limitParam]))
	}
	if limit := ninjapage.Limit(requested, d.limit); limit > 0 {
		return limit
	}
	return d.limit
}
