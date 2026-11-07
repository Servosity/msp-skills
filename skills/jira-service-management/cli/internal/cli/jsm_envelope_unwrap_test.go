// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Handfix jsm-paged-envelope-unwrap: JSM lists answer PagedDTO
// {start, limit, size, isLastPage, values} (servicedeskapi) or
// {startAt, maxResults, total, isLast, values} (Assets). Export, tail and sync
// must read the rows out of values, never store or write the envelope as one
// record. Press 4.35.1 stopped unwrapping both shapes (the siblings are not on
// its envelope-metadata list) until the spec repair in bin/prep.py.
func TestJSMPagedEnvelopesUnwrapValues(t *testing.T) {
	pages := map[string]string{
		"servicedesk":  `{"start":0,"limit":50,"size":2,"isLastPage":true,"values":[{"id":"1","projectName":"Acme"},{"id":"2","projectName":"Beta"}]}`,
		"request":      `{"start":0,"limit":50,"size":2,"isLastPage":false,"values":[{"issueId":"10"},{"issueId":"11"}]}`,
		"organization": `{"start":0,"limit":50,"size":2,"isLastPage":true,"values":[{"id":"7"},{"id":"8"}]}`,
		"objectschema": `{"startAt":0,"maxResults":25,"total":2,"isLast":true,"values":[{"id":"13"},{"id":"14"}]}`,
	}
	for resource, body := range pages {
		items, _, _ := extractResourcePage(json.RawMessage(body), resourceReadConfigFor(resource))
		if len(items) != 2 || strings.Contains(string(items[0]), "values") {
			t.Errorf("export/tail %s: got %d items %s, want the 2 rows inside values", resource, len(items), items)
		}
		if p := responsePathForResource(resource, ""); strings.Join(p, ".") != "values" {
			t.Errorf("sync %s: response path %v, want [values]", resource, p)
		}
	}
}
