// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"riverside-fm-pp-cli/internal/client"
)

// listAll reads every page of an offset/limit list. It stops on the first page
// that adds nothing new, not on a short page: Riverside's largest page size is
// undocumented, so a page shorter than the limit proves nothing, and a server that
// ignores offset ends after one repeated page instead of looping. A failed page
// fails the whole read, so a partial list is never taken for a complete one.
func listAll[T any](ctx context.Context, c *client.Client, path string, params map[string]string, pageSize int, extract func(json.RawMessage) []T, id func(T) string) ([]T, error) {
	var all []T
	seen := map[string]bool{}
	for offset := 0; ; {
		q := map[string]string{"offset": strconv.Itoa(offset), "limit": strconv.Itoa(pageSize)}
		for k, v := range params {
			q[k] = v
		}
		data, err := c.Get(ctx, path, q)
		if err != nil {
			return nil, err
		}
		page := extract(data)
		fresh := 0
		for _, it := range page {
			k := id(it)
			if k == "" {
				k = fmt.Sprint(it)
			}
			if !seen[k] {
				seen[k] = true
				all = append(all, it)
				fresh++
			}
		}
		if fresh == 0 {
			return all, nil
		}
		offset += len(page)
	}
}

func listStudioProjects(ctx context.Context, c *client.Client, studio string) ([]projectSummary, error) {
	return listAll(ctx, c, "/api/v4/projects/studio/"+url.PathEscape(studio),
		map[string]string{"sortBy": "createdAt", "orderBy": "desc"}, 200, extractProjects,
		func(p projectSummary) string { return p.ID })
}

func listProjectTakes(ctx context.Context, c *client.Client, projectID string) ([]takeSummary, error) {
	return listAll(ctx, c, "/api/v4/projects/"+url.PathEscape(projectID)+"/takes", nil, 100, extractTakes,
		func(t takeSummary) string { return t.SessionID })
}
