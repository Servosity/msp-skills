// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

type onePage struct{ body string }

func (p onePage) GetWithHeaders(context.Context, string, map[string]string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(p.body), nil
}

// Handfix jsm-single-page-says-more: a one-page read (no --all) of a JSM or
// Assets list whose page says more follow (isLastPage / isLast false) reports
// the truncation, the same event the generated pager emits for other APIs, so
// "request sla list" never reads as the complete list.
func TestSinglePageMarkedNotLastIsReportedTruncated(t *testing.T) {
	for _, body := range []string{
		`{"start":0,"limit":50,"size":1,"isLastPage":false,"values":[{"id":"1","name":"TTFR"}]}`,
		`{"startAt":0,"maxResults":25,"total":30,"isLast":false,"values":[{"id":"13"}]}`,
	} {
		r, w, _ := os.Pipe()
		old := os.Stderr
		os.Stderr = w
		_, err := paginatedGet(context.Background(), onePage{body}, "/request/SD-1/sla", map[string]string{}, nil, false, "offset", "offset", "limit", 0, "", "")
		w.Close()
		os.Stderr = old
		got, _ := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "truncated") {
			t.Errorf("page %s: no truncation signal on stderr (got %q)", body, got)
		}
	}
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	_, _ = paginatedGet(context.Background(), onePage{`{"start":0,"limit":50,"size":1,"isLastPage":true,"values":[{"id":"1"}]}`}, "/request/SD-1/sla", map[string]string{}, nil, false, "offset", "offset", "limit", 0, "", "")
	w.Close()
	os.Stderr = old
	if got, _ := io.ReadAll(r); strings.Contains(string(got), "truncated") {
		t.Errorf("a last page was reported truncated: %q", got)
	}
}
