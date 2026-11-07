// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// datto-rmm handfix fleet-live-reads-walk-every-page (Codex review r1 class
// "incomplete paging read as complete"): a live Datto list read by a novel
// command walks every page the way sync does: from page 0 along the
// documented pageDetails.nextPageUrl (its whole query, on the configured
// host: client.DattoNextQueryToken) until the link is null. Any failure (a
// failed request, a page without its collection, a repeated link, the page
// cap) fails the whole read, so a caller never treats part of a list as all
// of it.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"datto-rmm-pp-cli/internal/client"
)

const dattoLiveMaxPages = 10000

// dattoReadAllPages returns every record of the collection field across all
// pages of path, asking sizeParam=size a page.
func dattoReadAllPages(ctx context.Context, c *client.Client, path, field, sizeParam string, size int) ([]json.RawMessage, error) {
	params := map[string]string{sizeParam: strconv.Itoa(size)}
	seen := map[string]bool{}
	var all []json.RawMessage
	for page := 0; ; page++ {
		if page >= dattoLiveMaxPages {
			return nil, fmt.Errorf("%s: more than %d pages", path, dattoLiveMaxPages)
		}
		raw, err := c.Get(ctx, path, params)
		if err != nil {
			return nil, err
		}
		var env struct {
			PageDetails *struct {
				NextPageURL *string `json:"nextPageUrl"`
			} `json:"pageDetails"`
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) != nil || json.Unmarshal(raw, &env) != nil {
			return nil, fmt.Errorf("%s: page %d is not a Datto list page", path, page)
		}
		if envelopeReportsFailure(body) { // datto-rmm handfix: inband-error-field-is-a-failure
			return nil, fmt.Errorf("%s: page %d reports a failure in its body", path, page)
		}
		var items []json.RawMessage
		if rawItems, ok := body[field]; !ok || json.Unmarshal(rawItems, &items) != nil {
			return nil, fmt.Errorf("%s: page %d has no %q list", path, page, field)
		}
		all = append(all, items...)
		if env.PageDetails == nil || env.PageDetails.NextPageURL == nil || *env.PageDetails.NextPageURL == "" {
			return all, nil
		}
		tok, ok := client.DattoNextQueryToken(*env.PageDetails.NextPageURL)
		if !ok || seen[tok] {
			return nil, fmt.Errorf("%s: page %d links no new page", path, page)
		}
		seen[tok] = true
		params = map[string]string{sizeParam: strconv.Itoa(size), "page": tok}
	}
}
