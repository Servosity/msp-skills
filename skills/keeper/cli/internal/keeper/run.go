// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

// keeper handfix commander-allowlist (runner) and commander-response-strict:
// one POST per Command to /api/v1/executecommand (served in both Service Mode
// modes: legacy direct and queue compatibility, keepercommander/service/api/
// routes.py), and a response parser that fails closed. A report is accepted
// only as {"status":"success","data":[{...},...]}; every other shape (an
// error, a warning, partial_success, a log message, a security-audit error
// report, an encrypted body) is an error, never "zero rows".

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"keeper-pp-cli/internal/client"
	"keeper-pp-cli/internal/recordshape"
)

// ExecutePath is the Service Mode command endpoint.
const ExecutePath = "/api/v1/executecommand"

// Poster is the generated client's POST (internal/client.Client.Post): it
// carries the api-key header, the origin pin and the output chokepoint.
type Poster interface {
	Post(ctx context.Context, path string, body any) (json.RawMessage, int, error)
}

// Row is one projected report row: only the command's allowlisted fields.
type Row map[string]any

// ErrRecordContent is returned when Keeper answers with anything shaped like
// vault record content; the whole response is withheld.
var ErrRecordContent = recordshape.Err

// ErrSecurityAuditErrorReport is security-audit-report's error report
// (commands/security_audit.py: vault_owner, error_message rows), returned
// instead of scores when some vaults have invalid security data.
var ErrSecurityAuditErrorReport = errors.New("Keeper returned the security-audit error report, not scores: some vaults have invalid security data (an admin fixes it in Keeper with 'security-audit-report --attempt-fix'; this CLI never runs it)")

type envelope struct {
	Status  string          `json:"status"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
	Message json.RawMessage `json:"message"`
	Error   json.RawMessage `json:"error"`
}

func send(ctx context.Context, p Poster, c Command) (envelope, error) {
	if err := c.validate(); err != nil {
		return envelope{}, err
	}
	raw, status, err := p.Post(ctx, ExecutePath, map[string]string{"command": c.String()})
	if err != nil {
		// A non-2xx answer still carries Service Mode's {"status":"error",
		// "error":...} body; read it so callers see Keeper's reason.
		var apiErr *client.APIError
		if !errors.As(err, &apiErr) {
			return envelope{}, fmt.Errorf("%s: %w", c.name, err)
		}
		raw, status = []byte(apiErr.Body), apiErr.StatusCode
		var env envelope
		if json.Unmarshal(bytes.TrimSpace(raw), &env) != nil {
			return envelope{}, fmt.Errorf("%s: %w", c.name, err)
		}
		return env, fmt.Errorf("%s: %s", c.name, failureText(env, status))
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return envelope{}, fmt.Errorf("%s: Service Mode answered HTTP %d without a JSON object (response encryption, -ek, is not supported; turn it off for this key's host)", c.name, status)
	}
	var env envelope
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return envelope{}, fmt.Errorf("%s: unreadable Service Mode response: %w", c.name, err)
	}
	if status < 200 || status > 299 || env.Status != "success" || present(env.Error) {
		return env, fmt.Errorf("%s: %s", c.name, failureText(env, status))
	}
	return env, nil
}

// present reports a JSON value that is not absent and not null.
func present(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && !bytes.Equal(t, []byte("null"))
}

// withheldText stands for an error or message member that is not plain text, or that
// holds a credential. Service Mode's own are plain strings, so nothing is lost.
const withheldText = "(withheld: Keeper's text here is not plain text or holds a credential)"

// text is the one reader of a Service Mode error or message member: errors, the store
// and MCP results print what it returns.
func text(raw json.RawMessage) string {
	if !present(raw) {
		return ""
	}
	var lines []string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		lines = []string{s}
	} else if json.Unmarshal(raw, &lines) != nil || len(lines) == 0 {
		// A multi-line log arrives as a list of lines
		// (service/util/parse_keeper_response.py _format_multiline_message).
		return withheldText
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if secretValue.MatchString(l) || strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
			return withheldText
		}
	}
	return strings.Join(lines, "\n")
}

func failureText(env envelope, status int) string {
	msg := text(env.Error)
	if msg == "" {
		msg = text(env.Message)
	}
	if msg == "" {
		msg = "no error text"
	}
	st := env.Status
	if st == "" {
		st = "missing status"
	}
	return fmt.Sprintf("Service Mode returned %s (HTTP %d): %s", st, status, msg)
}

// Run sends a report command and returns its projected rows.
func Run(ctx context.Context, p Poster, c Command) ([]Row, error) {
	if c.kind != kindRows {
		return nil, fmt.Errorf("refused: %s is not a report", c.name)
	}
	env, err := send(ctx, p, c)
	if err != nil {
		return nil, err
	}
	if !present(env.Data) || bytes.TrimSpace(env.Data)[0] != '[' {
		return nil, fmt.Errorf("%s: unexpected Service Mode response: expected a JSON report, got %s", c.name, describe(env))
	}
	var raws []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(env.Data))
	dec.UseNumber()
	if err := dec.Decode(&raws); err != nil {
		return nil, fmt.Errorf("%s: unreadable report: %w", c.name, err)
	}
	out := make([]Row, 0, len(raws))
	for i, r := range raws {
		var obj map[string]any
		d := json.NewDecoder(bytes.NewReader(r))
		d.UseNumber()
		if err := d.Decode(&obj); err != nil || obj == nil {
			return nil, fmt.Errorf("%s: report row %d is not an object", c.name, i+1)
		}
		row, err := project(c, obj)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func describe(env envelope) string {
	if m := text(env.Message); m != "" {
		return "the message " + quoteShort(m)
	}
	if present(env.Data) {
		return "a non-list data value"
	}
	return "no data"
}

func quoteShort(s string) string {
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return fmt.Sprintf("%q", s)
}

// errorReportKeys mark security-audit-report's error report
// (commands/security_audit.py: vault_owner, error_message), which replaces
// the score rows when vault security data cannot be read.
var errorReportKeys = []string{"vault_owner", "error_message"}

// recordContent and secretValue are the one definition the client also applies.
var (
	recordContent = recordshape.Keys
	secretValue   = recordshape.Values
)

func project(c Command, obj map[string]any) (Row, error) {
	for _, k := range errorReportKeys {
		if _, ok := obj[k]; ok {
			return nil, fmt.Errorf("%s: %w", c.name, ErrSecurityAuditErrorReport)
		}
	}
	allowed := map[string]bool{}
	for _, f := range c.fields {
		allowed[f] = true
	}
	row := Row{}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := obj[k]
		if recordContent.MatchString(strings.TrimSpace(k)) && !allowed[k] {
			return nil, ErrRecordContent
		}
		if hasSecretValue(v) {
			return nil, ErrRecordContent
		}
		if !allowed[k] {
			continue
		}
		if keys, ok := c.nested[k]; ok {
			if kept, ok := nestedKeep(v, keys); ok {
				row[k] = kept
			}
			continue
		}
		if sv, ok := scalarOrList(v); ok {
			row[k] = sv
		}
	}
	return row, nil
}

func hasSecretValue(v any) bool {
	switch t := v.(type) {
	case string:
		return secretValue.MatchString(t)
	case []any:
		for _, e := range t {
			if hasSecretValue(e) {
				return true
			}
		}
	case map[string]any:
		for _, e := range t {
			if hasSecretValue(e) {
				return true
			}
		}
	}
	return false
}

// nestedKeep keeps an allowlisted nested field: an object cut to keys, or a
// list whose elements are such objects or scalars. Anything else drops the
// field.
func nestedKeep(v any, keys []string) (any, bool) {
	cut := func(obj map[string]any) map[string]any {
		kept := map[string]any{}
		for _, nk := range keys {
			if sv, ok := scalarOrList(obj[nk]); ok {
				kept[nk] = sv
			}
		}
		return kept
	}
	switch t := v.(type) {
	case map[string]any:
		return cut(t), true
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			switch et := e.(type) {
			case map[string]any:
				out = append(out, cut(et))
			case string, json.Number, bool:
				out = append(out, et)
			default:
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// scalarOrList keeps strings, numbers, booleans and lists of those; nested
// objects are not part of any allowlisted field and are dropped.
func scalarOrList(v any) (any, bool) {
	switch t := v.(type) {
	case string, json.Number, bool:
		return t, true
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			switch e.(type) {
			case string, json.Number, bool:
				out = append(out, e)
			default:
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// Message sends a context command and returns Keeper's log message.
func Message(ctx context.Context, p Poster, c Command) (string, error) {
	if c.kind != kindContext {
		return "", fmt.Errorf("refused: %s is not a context command", c.name)
	}
	env, err := send(ctx, p, c)
	if err != nil {
		return "", err
	}
	return text(env.Message), nil
}
