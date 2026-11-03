// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"jira-service-management-pp-cli/internal/platform"
)

// jsmSinglePageTruncation is the one-page read's (no --all) truncation signal
// for JSM and Assets lists (handfix jsm-single-page-says-more). Their pages
// carry isLastPage / isLast instead of a has-more field or a next cursor, so the
// generated warning never fired and "request sla list" read as complete while
// JSM said more metrics follow. Same event and run metadata as the generated
// pager; jsmPageHasMore is the parser --all and sync already use.
func jsmSinglePageTruncation(ctx context.Context, data json.RawMessage) {
	if !jsmPageHasMore(data) {
		return
	}
	platform.MarkContextTruncated(ctx, platform.TruncationReason{Kind: "page_limit", Configured: 1, Observed: 1, MoreAvailable: true})
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "warning: results truncated; more pages available. Re-run with --all to fetch every page.\n")
	} else {
		fmt.Fprintf(os.Stderr, `{"event":"truncated","hint":"pass --all to fetch every page"}`+"\n")
	}
}
