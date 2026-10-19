// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// jsmCompleteRequestSLA fills in a synced request's embedded SLA when JSM
// marked that page incomplete (sla.isLastPage false): sync asks for
// expand=sla, which returns one page of metrics, and sla-report and client
// read only what sync stored. The rest is read from /request/{key}/sla. A
// request whose SLA cannot be completed keeps its partial page, and the
// reports count it as sla_incomplete instead of silently undercounting.
func jsmCompleteRequestSLA(ctx context.Context, c any, resource string, items []json.RawMessage) []json.RawMessage {
	if resource != "request" {
		return items
	}
	g, ok := c.(jsmGetter)
	if !ok {
		return items
	}
	for i, raw := range items {
		var head struct {
			IssueKey string `json:"issueKey"`
			SLA      *struct {
				IsLastPage *bool `json:"isLastPage"`
			} `json:"sla"`
		}
		if json.Unmarshal(raw, &head) != nil || head.IssueKey == "" || !slaPageIncomplete(head.SLA) {
			continue
		}
		seg, err := jsmPathSeg(head.IssueKey)
		if err != nil {
			continue
		}
		values, truncated, err := jsmWalk(ctx, g, "/request/"+seg+"/sla", nil, 0, jsonKey("id"))
		if err != nil || truncated {
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		sla, err := json.Marshal(map[string]any{"size": len(values), "start": 0, "limit": len(values), "isLastPage": true, "values": values})
		if err != nil {
			continue
		}
		obj["sla"] = sla
		if b, err := json.Marshal(obj); err == nil {
			items[i] = b
		}
	}
	return items
}

func slaPageIncomplete(sla *struct {
	IsLastPage *bool `json:"isLastPage"`
}) bool {
	return sla != nil && sla.IsLastPage != nil && !*sla.IsLastPage
}

// jsmPruneUnfiltered turns off --full pruning for a resource whose walk a
// --param, --global-param or --resource-param shaped (requestStatus=OPEN_REQUESTS,
// say): rows the filter left out were never seen, so they must not be deleted.
func jsmPruneUnfiltered(prune bool, userParams *syncUserParams, resource string, dependent bool, events io.Writer) bool {
	if !prune {
		return false
	}
	probe := map[string]string{}
	userParams.applyTo(resource, probe, dependent)
	if len(probe) == 0 {
		return true
	}
	if events != nil {
		fmt.Fprintf(events, `{"event":"reconcile_skipped","resource":%q,"scope":"*","reason":"user_params"}`+"\n", resource)
	}
	return false
}
