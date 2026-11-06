// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex r1 (4.34.2 reprint) class wrong-field-read-from-a-documented-response:
// Abnormal's case lists answer only list rows (spec.json PaginatedCases.cases[]
// is AbnormalCase: no affectedEmployee, no case_status; PaginatedVendorCases.
// vendorCases[] is VendorCases: vendorCaseId only). The fields the novels
// match on live in the detail schemas (AbnormalCaseDetails, VendorCaseDetails),
// so every listed case is read through its detail GET before it is matched.
// One helper for both novels; every detail failure is reported, never read as
// "no match".

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"abnormal-pp-cli/internal/client"
)

// caseDetailWorkers bounds concurrent detail GETs (the API rate-limits).
const caseDetailWorkers = 4

// listedIDs returns the identifier of every list row under key, as text (the
// spec types caseId a string with a numeric example and vendorCaseId an
// integer), in list order; a row without one is skipped and counted.
func listedIDs(rows []map[string]json.RawMessage, key string) (ids []string, missing int) {
	for _, r := range rows {
		if id := idText(r[key]); id != "" {
			ids = append(ids, id)
		} else {
			missing++
		}
	}
	return ids, missing
}

// idText is a JSON string or number as text; "" for anything else.
func idText(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// listedCaseDetails GETs prefix+id for every id, caseDetailWorkers at a time,
// and returns the bodies in id order (nil where the GET failed). Each failure
// goes to fail with the case it belongs to.
func listedCaseDetails(ctx context.Context, c *client.Client, prefix string, ids []string, fail func(source string, err error)) []json.RawMessage {
	out := make([]json.RawMessage, len(ids))
	sem := make(chan struct{}, caseDetailWorkers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			data, err := c.Get(ctx, prefix+escapeCaseID(id), nil)
			if err != nil {
				fail(fmt.Sprintf("%s%s", prefix, id), err)
				return
			}
			out[i] = data
		}(i, id)
	}
	wg.Wait()
	return out
}

func escapeCaseID(id string) string {
	return strings.NewReplacer("/", "%2F", "?", "%3F", "#", "%23", "%", "%25").Replace(id)
}

// caseIsOpen reports whether a case_status (spec.json CaseStatusEnum) still
// needs action; an absent or unknown status counts as open (fail open: an
// unreadable status never hides a case).
func caseIsOpen(status string) bool {
	switch strings.TrimSpace(status) {
	case "Acknowledged (Attack resolved)", "Acknowledged (Not an attack)":
		return false
	}
	return true
}
