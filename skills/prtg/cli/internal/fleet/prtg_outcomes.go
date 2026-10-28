// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

// prtg handfix (bulk-action-outcomes): the one reader of a PRTG bulk action
// response (an ActionResult list: {node, failure} per object, prtg.api.yaml).
// The fleet action engine, the generated multi-* commands (through
// detectPartialFailure) and MCP prtg_execute (through the transport's action
// sink) all use it, so they cannot disagree. Any failure signal from any item
// fails that item: a failure member that is not null (whatever it holds), a
// populated error/errors/err member, success/ok false, a status-like member
// that is an HTTP status of 400 or more or carries failure wording, and an
// ActionResult entry that names neither a node nor a failure.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ItemOutcome is one object of a bulk action result.
type ItemOutcome struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	Failed bool   `json:"failed"`
	Reason string `json:"reason,omitempty"`
}

// failureWording marks a status-like value that reports a failure.
var failureWording = regexp.MustCompile(`(?i)(fail|error|denied|refus|reject|invalid|forbidden|unauthori|not[ _-]?found|bad[ _-]?request|conflict|timed?[ _-]?out|unable|cannot|abort|unavailable)`)

var (
	errorKeys  = []string{"failure", "error", "errors", "err"}
	falseKeys  = []string{"success", "ok", "succeeded"}
	statusKeys = []string{"status", "state", "result", "outcome", "status_code", "http_status", "code"}
)

// ActionOutcomes reads body as a bulk action result. isList is false when
// body is not a JSON array of objects (then items is nil and nothing is
// judged); an array element that is not an object is a failed item.
func ActionOutcomes(body []byte) (items []ItemOutcome, isList bool) {
	var raw []json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return nil, false
	}
	actionShaped := false
	objs := make([]map[string]any, len(raw))
	for i, r := range raw {
		dec := json.NewDecoder(strings.NewReader(string(r)))
		dec.UseNumber()
		if dec.Decode(&objs[i]) == nil && objs[i] != nil {
			_, hasNode := objs[i]["node"]
			_, hasFailure := objs[i]["failure"]
			actionShaped = actionShaped || hasNode || hasFailure
		}
	}
	for i, obj := range objs {
		if obj == nil {
			items = append(items, ItemOutcome{Failed: true, Reason: "unreadable entry: " + truncate(string(raw[i]), 80)})
			continue
		}
		it := ItemOutcome{}
		if node, ok := obj["node"].(map[string]any); ok {
			it.ID, it.Name, it.Type = str(node["id"]), str(node["name"]), str(node["type"])
		}
		it.Failed, it.Reason = itemFailure(obj)
		if !it.Failed && actionShaped && it.ID == "" {
			it.Failed, it.Reason = true, "entry names neither a node nor a failure"
		}
		items = append(items, it)
	}
	return items, true
}

// itemFailure reports the first failure signal in one result object.
func itemFailure(obj map[string]any) (bool, string) {
	for _, k := range errorKeys {
		v, ok := obj[k]
		if !ok || v == nil || isEmpty(v) && k != "failure" {
			continue
		}
		if m, ok := v.(map[string]any); ok {
			if why := strings.TrimSpace(str(m["code"]) + " " + str(m["message"])); why != "" {
				return true, why
			}
		}
		return true, k + ": " + truncate(compact(v), 120)
	}
	for _, k := range falseKeys {
		switch v := obj[k].(type) {
		case bool:
			if !v {
				return true, k + ": false"
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(v), "false") {
				return true, k + ": false"
			}
		}
	}
	for _, k := range statusKeys {
		v, ok := obj[k]
		if !ok || v == nil {
			continue
		}
		s := str(v)
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			if n >= 400 && n < 600 {
				return true, fmt.Sprintf("%s: %s", k, s)
			}
			continue
		}
		if failureWording.MatchString(s) {
			return true, k + ": " + truncate(s, 120)
		}
	}
	return false, ""
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func compact(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// FailedSummary is "<failed> of <total> item(s) failed: <reasons>".
func FailedSummary(items []ItemOutcome) (failed int, summary string) {
	var reasons []string
	for _, it := range items {
		if !it.Failed {
			continue
		}
		failed++
		if len(reasons) < 5 {
			who := it.Name
			if it.ID != "" {
				if who != "" {
					who += " (" + it.ID + ")"
				} else {
					who = it.ID
				}
			}
			reasons = append(reasons, strings.TrimSpace(who+" "+it.Reason))
		}
	}
	if failed == 0 {
		return 0, ""
	}
	return failed, fmt.Sprintf("%d of %d item(s) failed: %s", failed, len(items), strings.Join(reasons, "; "))
}

// ResponseFailure judges a whole 2xx write response: a list by its items, an
// object as one item (its own failure, error or status members).
func ResponseFailure(body []byte) (items []ItemOutcome, failed int, summary string) {
	items, isList := ActionOutcomes(body)
	if !isList {
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.UseNumber()
		var obj map[string]any
		if dec.Decode(&obj) != nil || obj == nil {
			return nil, 0, ""
		}
		it := ItemOutcome{}
		it.Failed, it.Reason = itemFailure(obj)
		items = []ItemOutcome{it}
	}
	failed, summary = FailedSummary(items)
	return items, failed, summary
}
