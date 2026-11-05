// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-control-pp-cli/internal/cliutil"
)

// Shared reads for the two live novels (sessions stale, security
// access-review). Both send the positional .ashx body each operation's spec
// description documents (the community PowerShell module the spec cites sends
// the same), not the named-field body the generated commands send.

// cwcNow is the clock the novels read; tests pin it.
var cwcNow = time.Now

// cwcPost sends one POST through the configured client (pinned host, held
// credentials, --dry-run honoured by the transport guard) and returns the body.
func cwcPost(cmd *cobra.Command, flags *rootFlags, path string, body any) (json.RawMessage, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	data, _, err := c.PostQueryWithParams(cmd.Context(), path, map[string]string{}, body)
	if err != nil {
		return nil, classifyAPIError(cmd.OutOrStdout(), err, flags)
	}
	return data, nil
}

// cwcDecode decodes one JSON value; null or trailing data are
// rejected by the callers, so an odd answer is never an empty one.
func cwcDecode(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("the instance answered with something that is not JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("the instance answered with more than one JSON value")
	}
	return v, nil
}

// cwcField returns m's value for key: the exact name first, then the first
// case-insensitive match in sorted key order (the spec documents camelCase,
// the instance's own JSON is PascalCase).
func cwcField(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.EqualFold(k, key) {
			return m[k], true
		}
	}
	return nil, false
}

func cwcString(m map[string]any, key string) string {
	v, _ := cwcField(m, key)
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%v", t)
	}
	return ""
}

// cwcBool reads a boolean flag; ok is false when the field is absent or not a
// boolean (a string "true" included), so a caller can report it as unknown.
func cwcBool(m map[string]any, key string) (value, ok bool) {
	v, found := cwcField(m, key)
	b, isBool := v.(bool)
	return b, found && isBool
}

func cwcStrings(m map[string]any, key string) ([]string, bool) {
	v, found := cwcField(m, key)
	if !found || v == nil {
		return []string{}, found
	}
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// cwcObjects reads v as a list of objects; any element that is not an object
// fails the whole read.
func cwcObjects(v any, what string) ([]map[string]any, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("unrecognised %s answer: expected a JSON array", what)
	}
	out := make([]map[string]any, 0, len(items))
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unrecognised %s answer: element %d is not an object", what, i)
		}
		out = append(out, m)
	}
	return out, nil
}

// cwcSessionList reads a GetLiveData answer: the spec's bare session array,
// or the envelope the spec's wire note documents
// (ResponseInfoMap.HostSessionInfo.Sessions). Any other shape is an error, so
// an unexpected answer never reads as "no sessions".
func cwcSessionList(data []byte) ([]map[string]any, error) {
	v, err := cwcDecode(data)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		cur := m
		for _, k := range []string{"ResponseInfoMap", "HostSessionInfo"} {
			next, found := cwcField(cur, k)
			nm, isMap := next.(map[string]any)
			if !found || !isMap {
				return nil, fmt.Errorf("unrecognised session list answer: no %s object", k)
			}
			cur = nm
		}
		sessions, found := cwcField(cur, "Sessions")
		if !found {
			return nil, fmt.Errorf("unrecognised session list answer: no Sessions array")
		}
		v = sessions
	}
	return cwcObjects(v, "session list")
}

// cwcSessionTypes is the wire number of each session type (spec Session.sessionType).
var cwcSessionTypes = map[string]int{"support": 0, "meeting": 1, "access": 2}

// cwcLiveDataBody is GetLiveData's documented positional body for one
// session type and group, with no search filter, session id or limit (the
// cited module's defaults).
func cwcLiveDataBody(sessionType int, group string) []any {
	return []any{
		map[string]any{
			"HostSessionInfo": map[string]any{
				"sessionType":           sessionType,
				"sessionGroupPathParts": []string{group},
				"filter":                nil,
				"findSessionID":         nil,
				"sessionLimit":          0,
			},
			"ActionCenterInfo": map[string]any{},
		},
		0,
	}
}

// cwcTime parses a session timestamp: RFC 3339 or the .NET JSON
// "/Date(ms)/" form. ok is false for an absent or empty value.
func cwcTime(s string) (t time.Time, ok bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, nil
	}
	if t, parsed := cliutil.ParseODataDate(s); parsed {
		return t, true, nil
	}
	if t, perr := time.Parse(time.RFC3339Nano, s); perr == nil {
		return t.UTC(), true, nil
	}
	return time.Time{}, false, fmt.Errorf("unparseable timestamp %q", s)
}

// cwcEmit prints v as JSON for agents and pipes, or rows as a table on a
// terminal.
func cwcEmit(cmd *cobra.Command, flags *rootFlags, v any, rows []map[string]any, summary string) error {
	w := cmd.OutOrStdout()
	if wantsHumanTable(w, flags) {
		fmt.Fprintln(w, summary)
		if len(rows) == 0 {
			return nil
		}
		return printAutoTable(w, rows)
	}
	return printJSONFiltered(w, v, flags)
}
