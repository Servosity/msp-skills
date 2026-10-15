// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers for live ConnectSecure calls from novel commands.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"connectsecure-pp-cli/internal/client"
)

type csFetchFailure struct {
	Section string `json:"section"`
	Error   string `json:"error"`
}

// csGetData GETs a ConnectSecure path and returns the `data` member of the
// {status, data} envelope (or the whole body when there is no envelope).
func csGetData(ctx context.Context, c *client.Client, path string, params map[string]string) (any, error) {
	raw, err := c.Get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("%s: response is not JSON: %w", path, err)
	}
	if m, ok := body.(map[string]any); ok {
		if st, ok := m["status"].(bool); ok && !st {
			return nil, fmt.Errorf("%s: API returned status false: %s", path, c.SafeServerText(csString(m["message"])))
		}
		if d, ok := m["data"]; ok {
			return d, nil
		}
	}
	return body, nil
}

// csGetRows pages a list endpoint with skip/limit until a short page or
// maxPages. It returns the rows and whether the page cap stopped the walk.
func csGetRows(ctx context.Context, c *client.Client, path string, params map[string]string, pageSize, maxPages int) ([]csRow, bool, error) {
	out := make([]csRow, 0)
	if maxPages < 1 {
		maxPages = 1
	}
	for page := 0; page < maxPages; page++ {
		p := map[string]string{"skip": strconv.Itoa(page * pageSize), "limit": strconv.Itoa(pageSize)}
		for k, v := range params {
			p[k] = v
		}
		data, err := csGetData(ctx, c, path, p)
		if err != nil {
			return out, false, err
		}
		if data == nil {
			return out, false, nil
		}
		// An unexpected shape is a fetch failure, not an empty result: callers
		// report errors in fetch_failures instead of a silent all-clear.
		list, ok := data.([]any)
		if !ok {
			return out, false, fmt.Errorf("%s: expected a list of rows, got %T", path, data)
		}
		for i, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				return out, false, fmt.Errorf("%s: row %d is %T, not an object", path, page*pageSize+i, item)
			}
			out = append(out, m)
		}
		if len(list) < pageSize {
			return out, false, nil
		}
	}
	return out, true, nil
}

// csQuote renders a string literal for a ConnectSecure `condition`.
func csQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
