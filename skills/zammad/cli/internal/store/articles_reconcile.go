// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): per-ticket deletion mirroring for the
// hand-authored `articles sync` (internal/cli/articles_sync.go).

package store

import (
	"fmt"
	"strings"
)

// ReconcileTicketArticles deletes the stored articles of ticketID whose bare
// id is not in seenIDs: the mark-and-sweep half of a per-ticket article
// refresh. The caller must pass the COMPLETE id set from one successful fetch
// of /ticket_articles/by_ticket/<ticketID>; an article removed upstream then
// stops counting in escalate, churn-risk and feedback-scan (Codex round 1 on
// the 4.34.0 reprint). Victims come from the generic resources table, so rows
// without a typed projection are cleaned too; each victim's typed row, FTS
// entry and generic row go in one transaction (see reconcileUnseen).
func (s *Store) ReconcileTicketArticles(ticketID string, seenIDs []string) (int, error) {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return 0, fmt.Errorf("reconcile articles: empty ticket id")
	}
	return s.reconcileUnseen("articles", seenIDs, "articles", nil,
		`SELECT id FROM resources
		 WHERE resource_type = 'articles'
		   AND (CASE WHEN json_valid(data) THEN CAST(json_extract(data, '$.ticket_id') AS TEXT) END) = ?`,
		ticketID)
}
