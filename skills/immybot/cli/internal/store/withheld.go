// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Hand-fix (handfixes.json sync-skips-withheld-row). A row the credential check withholds
// from the store is skipped and named, so one row never aborts a batch (#612).

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"immybot-pp-cli/internal/credkit"
)

// WithheldRowsError names the rows of a batch the credential check refused
// whole: each carried a credential this program holds, or its check ran out
// of budget. The batch's other rows were stored. errors.Is(err,
// credkit.ErrWithheld) holds for it.
type WithheldRowsError struct {
	Resource string
	IDs      []string
}

func (e *WithheldRowsError) Error() string {
	return fmt.Sprintf("%s: %d row(s) not stored, withheld by the credential check (each carries a secret a vendor response returned); the next sync fetches them again: %s",
		e.Resource, len(e.IDs), strings.Join(e.IDs, ", "))
}

func (e *WithheldRowsError) Is(target error) bool { return target == credkit.ErrWithheld }

func isWithheldRow(err error) bool { return errors.Is(err, credkit.ErrWithheld) }

// WithheldEvent is the sync_warning event line that names a page's withheld rows.
func WithheldEvent(resource string, ids []string) string {
	b, _ := json.Marshal(map[string]any{"event": "sync_warning", "resource": resource, "reason": "rows_withheld", "ids": ids})
	return string(b)
}

// WithheldSummary is the sync_summary event's "withheld" member (resource to
// row ids), or "" when no row was withheld.
func WithheldSummary(rows map[string][]string) string {
	if len(rows) == 0 {
		return ""
	}
	b, _ := json.Marshal(rows)
	return `,"withheld":` + string(b)
}
