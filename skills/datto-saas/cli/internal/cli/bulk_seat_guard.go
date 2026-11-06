// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Local validation for the one write in this CLI, PUT .../bulkSeatChange.
// Datto's enums are case-sensitive and it recommends at most 100 ids per call
// (S1), but the generated command sends whatever it is given. The guard is a
// client hook, so it covers the CLI command and the MCP tool, and it survives
// regeneration because it lives outside generated files. It forwards only the
// canonical payload it validated, never the caller's original bytes.

package cli

import (
	"bytes"
	"datto-saas-pp-cli/internal/credkit"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"datto-saas-pp-cli/internal/client"
	"datto-saas-pp-cli/internal/cliutil"
)

func init() {
	registerClientHook(installBulkSeatGuard)
}

func installBulkSeatGuard(c *client.Client) error {
	if c == nil || c.HTTPClient == nil {
		return nil
	}
	if _, ok := c.HTTPClient.Transport.(bulkSeatGuard); ok {
		return nil
	}
	next := c.HTTPClient.Transport
	if next == nil {
		next = credkit.Transport(nil) // Hand-fix (handfixes.json credkit-wiring)
	}
	c.HTTPClient.Transport = bulkSeatGuard{next: next}
	// http.Client follows 307/308 by resending a PUT with its body, which for
	// bulkSeatChange could repeat a billing change at another path. Refuse.
	prev := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && isBulkSeatPut(via[0]) {
			return fmt.Errorf("refusing to follow a redirect to %s for bulkSeatChange; the change was not resent", req.URL.Redacted())
		}
		if prev != nil {
			return prev(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return nil
}

func isBulkSeatPut(req *http.Request) bool {
	return req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/bulkSeatChange")
}

type bulkSeatGuard struct{ next http.RoundTripper }

func (g bulkSeatGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if isBulkSeatPut(req) {
		// An idempotency header makes the generated client treat the PUT as
		// replay-safe after a 5xx; Datto documents no idempotency, so a retry
		// could repeat a billing change.
		for _, h := range []string{"Idempotency-Key", "X-Idempotency-Key"} {
			if req.Header.Get(h) != "" {
				return nil, fmt.Errorf("bulkSeatChange refused before sending: remove the %s header; Datto documents no idempotency for this change, so it must never be replayed", h)
			}
		}
		var body []byte
		if req.Body != nil {
			b, err := io.ReadAll(req.Body)
			_ = req.Body.Close()
			if err != nil {
				return nil, err
			}
			body = b
		}
		canonical, err := canonicalBulkSeatBody(body)
		if err != nil {
			return nil, fmt.Errorf("bulkSeatChange refused before sending: %w", err)
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(canonical))
		req.ContentLength = int64(len(canonical))
		// No GetBody: it would let Go's HTTP/2 transport resend the PUT after a
		// stream reset, below every retry control; redirects are refused too.
		req.GetBody = nil
	}
	return g.next.RoundTrip(req)
}

// canonicalBulkSeatBody parses the body with exact key matching (Go's struct
// decoder folds case and Unicode, so "idſ" would alias "ids"), rejects
// duplicate or unknown keys, validates the values, and returns the only bytes
// that will be sent. A comma-separated "ids" string (what the generated MCP
// tool sends, comma-separated or a JSON array string) is parsed with the
// generated CLI's list parser and becomes the JSON array Datto expects (S1).
func canonicalBulkSeatBody(body []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("body must be a JSON object with seat_type, action_type and ids")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("body must be JSON: %w", err)
		}
		key, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("body must be JSON: %w", err)
		}
		if _, dup := fields[key]; dup {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		switch key {
		case "seat_type", "action_type", "ids":
		default:
			return nil, fmt.Errorf("unexpected key %q; only seat_type, action_type and ids are sent", key)
		}
		fields[key] = v
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, fmt.Errorf("body must be JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("body has trailing data after the JSON object")
	}
	str := func(key string) (string, error) {
		raw, ok := fields[key]
		if !ok {
			return "", fmt.Errorf("%s is required", key)
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("%s must be a string", key)
		}
		return s, nil
	}
	seatType, err := str("seat_type")
	if err != nil {
		return nil, err
	}
	if err := validateEnumSet("seat-type", map[string]bool{seatType: true}, saasSeatTypes); err != nil {
		return nil, err
	}
	actionType, err := str("action_type")
	if err != nil {
		return nil, err
	}
	if err := validateEnumSet("action-type", map[string]bool{actionType: true}, bulkSeatActions); err != nil {
		return nil, err
	}
	raw, ok := fields["ids"]
	if !ok {
		return nil, errors.New("ids is required")
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		var list string
		if json.Unmarshal(raw, &list) != nil {
			return nil, errors.New("ids must be an array of remoteId strings or a comma-separated string")
		}
		// Same parser as the generated CLI flag, so MCP and CLI agree.
		if ids, err = cliutil.ParseStringList(list); err != nil {
			return nil, fmt.Errorf("ids: %w", err)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("ids must list at least one seat remoteId")
	}
	if len(ids) > maxBulkSeatIDs {
		return nil, fmt.Errorf("%d ids exceeds the %d-per-call limit this CLI enforces from Datto's recommendation; split with 'seats bulk-plan'", len(ids), maxBulkSeatIDs)
	}
	for i, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("ids[%d] is empty", i)
		}
	}
	return json.Marshal(struct {
		SeatType   string   `json:"seat_type"`
		ActionType string   `json:"action_type"`
		IDs        []string `json:"ids"`
	}{seatType, actionType, ids})
}
