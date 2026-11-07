// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// datto-rmm handfix next-link-query-followed (Codex review r1 class "next-link
// query dropped"): Datto's pageDetails.nextPageUrl carries the whole state of
// the next page in its query: page=N with max for the lists, and for
// /v2/activity-logs page=next|previous with the searchAfter pointer (vendor
// spec: "It is not advised to set this parameter manually. Instead, ... utilize
// the prevPage and nextPage"). The generated pagers kept only the page value,
// so an activity walk asked page=next with no pointer and repeated or lost
// records. One token now carries the link's whole query from the one place
// every walk reduces a next link (cursorFromNextURL: sync, list --all, export,
// the fleet live reads), and the client expands it at its one request
// chokepoint. Only the query travels: scheme, host and path stay the
// configured ones, so a next link never moves a credential (api-origin-pinned
// still guards the wire).

import (
	"net/url"
	"sort"
	"strings"
)

const dattoNextQueryPrefix = "datto-next-query:"

// DattoNextQueryToken reduces a next-page link to one cursor token carrying
// every parameter of its query; ok is false for a link with no query.
func DattoNextQueryToken(nextURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(nextURL))
	if err != nil || u.RawQuery == "" {
		return "", false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) == 0 {
		return "", false
	}
	return dattoNextQueryPrefix + q.Encode(), true
}

// dattoExpandNextQuery replaces a next-query token among the request
// parameters with the parameters it carries: each one the link names takes
// the link's value (repeated values joined by commas, Datto's documented form
// for list parameters); the request's other parameters stay.
func dattoExpandNextQuery(params map[string]string) map[string]string {
	var tokens []string
	for k, v := range params {
		if strings.HasPrefix(v, dattoNextQueryPrefix) {
			tokens = append(tokens, k)
		}
	}
	if len(tokens) == 0 {
		return params
	}
	sort.Strings(tokens)
	out := make(map[string]string, len(params))
	for k, v := range params {
		if !strings.HasPrefix(v, dattoNextQueryPrefix) {
			out[k] = v
		}
	}
	for _, k := range tokens {
		q, err := url.ParseQuery(strings.TrimPrefix(params[k], dattoNextQueryPrefix))
		if err != nil {
			out[k] = params[k] // never sent as a different page: the API refuses the token
			continue
		}
		for name, vals := range q {
			out[name] = strings.Join(vals, ",")
		}
	}
	return out
}
