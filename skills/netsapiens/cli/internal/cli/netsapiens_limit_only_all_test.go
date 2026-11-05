// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json netsapiens-limit-only-all-is-not-silent).

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// limitOnlyServer answers like NetSapiens /phones: it takes only limit, so
// every request returns the first min(limit, rows) rows.
type limitOnlyServer struct {
	rows  int
	calls int
}

func (s *limitOnlyServer) GetWithHeaders(_ context.Context, _ string, params map[string]string, _ map[string]string) (json.RawMessage, error) {
	s.calls++
	n, _ := strconv.Atoi(params["limit"])
	if n <= 0 || n > s.rows {
		n = s.rows
	}
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"mac":"%012d"}`, i)
	}
	return json.RawMessage("[" + strings.Join(items, ",") + "]"), nil
}

// A limit-only endpoint (no offset parameter) whose first page is full may
// have rows --all cannot reach: it must fail, never return 100 of 1000 rows
// with a complete event and exit 0.
func TestLimitOnlyAllFailsOnAFullPage(t *testing.T) {
	srv := &limitOnlyServer{rows: 1000}
	data, err := paginatedGet(context.Background(), srv, "/phones", map[string]string{}, nil, true, "", "offset", "limit", 100, "", "")
	if err == nil {
		var got []json.RawMessage
		_ = json.Unmarshal(data, &got)
		t.Fatalf("--all on a limit-only endpoint returned %d of 1000 rows with no error", len(got))
	}
	if !strings.Contains(err.Error(), "--limit") {
		t.Fatalf("error should tell the operator to raise --limit: %v", err)
	}
}

// A short page is the whole collection: no error, every row.
func TestLimitOnlyAllShortPageIsComplete(t *testing.T) {
	srv := &limitOnlyServer{rows: 42}
	data, err := paginatedGet(context.Background(), srv, "/phones", map[string]string{}, nil, true, "", "offset", "limit", 100, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var got []json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil || len(got) != 42 || srv.calls != 1 {
		t.Fatalf("got %d rows in %d calls (%v)", len(got), srv.calls, err)
	}
}
