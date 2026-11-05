// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "sync-json-only-bounded" entry. See docs/reprint-survival.md.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"servosity-msp-pp-cli/internal/client"
)

// refuseBinaryEndpointSync refuses the installer endpoint before any request.
// The spec cannot name it: its response has no schema and no produces, the same
// as JSON reads like /stats/ and /users/. The Content-Type refusal covers any
// other endpoint like it.
func refuseBinaryEndpointSync(resources []string) error {
	for _, r := range resources {
		if r == "download" {
			return fmt.Errorf("download is not synced: it is a Windows installer, not records")
		}
	}
	return nil
}

// syncBodyRefusalReason names the sync_warning reason when a response was
// refused for what its body is, not for what the API said about access.
func syncBodyRefusalReason(err error) (string, bool) {
	switch {
	case errors.Is(err, client.ErrNonJSONResponse):
		return "non_json_response", true
	case errors.Is(err, client.ErrOffOriginRedirect):
		return "redirect_off_origin", true
	case errors.Is(err, client.ErrResponseTooLarge), errors.Is(err, client.ErrDecodedBodyTooLarge):
		return "response_too_large", true
	}
	return "", false
}

// syncWarnedSummaryField is the sync_summary field naming the warned resources,
// empty when there are none so an all-clear summary keeps its old shape.
func syncWarnedSummaryField(names []string) string {
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	raw, _ := json.Marshal(names)
	return `,"warned_resources":` + string(raw)
}
