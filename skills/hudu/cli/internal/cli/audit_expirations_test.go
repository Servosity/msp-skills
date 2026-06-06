// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution test: expiration radar runs against the real store schema.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelAuditExpirationsRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditExpirationsCmd, "--within", "30d")
	var rows []expirationRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
}

func TestNovelAuditExpirationsTypeFilterRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditExpirationsCmd, "--within", "60d", "--type", "ssl")
	var rows []expirationRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("type-filtered output is not a JSON array: %v\n%s", err, out)
	}
}
