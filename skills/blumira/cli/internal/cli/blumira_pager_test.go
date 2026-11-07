// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// pageGetterFunc serves fetchAllBlumiraPages from a function of path and page.
type pageGetterFunc func(path string, page int) (string, error)

func (f pageGetterFunc) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	page, _ := strconv.Atoi(params["page"])
	body, err := f(path, page)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func TestFetchAllBlumiraPagesFailsClosed(t *testing.T) {
	full := make([]string, blumiraPageSize)
	for i := range full {
		full[i] = `{"id":"` + strconv.Itoa(i) + `"}`
	}
	fullPage := "[" + strings.Join(full, ",") + "]"
	cases := []struct {
		name    string
		pages   map[int]string
		want    int
		wantErr string
	}{
		{"links.next drives the walk", map[int]string{
			1: `{"status":"OK","data":[{"a":1}],"links":{"next":"/v1/x?page=2"},"meta":{"page":1}}`,
			2: `{"status":"OK","data":[{"a":2}],"links":{"next":null},"meta":{"page":2}}`,
		}, 2, ""},
		{"total_pages wins over a null next", map[int]string{
			1: `{"status":"OK","data":[{"a":1}],"links":{"next":null},"meta":{"page":1,"total_pages":2}}`,
			2: `{"status":"OK","data":[{"a":2}],"links":{"next":null},"meta":{"page":2,"total_pages":2}}`,
		}, 2, ""},
		{"total_items above rows read, then an empty page", map[int]string{
			1: `{"status":"OK","data":[{"a":1}],"links":{"next":null},"meta":{"page":1,"total_items":3}}`,
			2: `{"status":"OK","data":[],"links":{"next":null},"meta":{"page":2,"total_items":3}}`,
		}, 0, "empty page while the response says more rows follow"},
		{"non-OK status", map[int]string{1: `{"status":"ERROR","data":[]}`}, 0, "status"},
		{"error field", map[int]string{1: `{"data":[],"error":"denied"}`}, 0, "error"},
		{"no data array", map[int]string{1: `{"status":"OK","links":{"next":null}}`}, 0, "no data array"},
		{"data not an array", map[int]string{1: `{"status":"OK","data":{"a":1}}`}, 0, "not an array"},
		{"not an object", map[int]string{1: `[{"a":1}]`}, 0, "not a JSON object"},
		{"page parameter ignored", map[int]string{
			1: `{"status":"OK","data":[{"a":1}],"links":{"next":"/v1/x?page=2"}}`,
			2: `{"status":"OK","data":[{"a":1}],"links":{"next":"/v1/x?page=3"}}`,
		}, 0, "repeats page"},
		{"no paging metadata, short page ends", map[int]string{1: `{"status":"OK","data":[{"a":1}]}`}, 1, ""},
		{"no paging metadata, full page continues", map[int]string{
			1: `{"status":"OK","data":` + fullPage + `}`,
			2: `{"status":"OK","data":[]}`,
		}, blumiraPageSize, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			get := pageGetterFunc(func(_ string, page int) (string, error) {
				if body, ok := c.pages[page]; ok {
					return body, nil
				}
				return `{"status":"OK","data":[]}`, nil
			})
			rows, err := fetchAllBlumiraPages(context.Background(), get, "/x", 10)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) || rows != nil {
					t.Fatalf("want error containing %q and no rows, got rows=%d err=%v", c.wantErr, len(rows), err)
				}
				return
			}
			if err != nil || len(rows) != c.want {
				t.Fatalf("want %d rows, got %d err=%v", c.want, len(rows), err)
			}
		})
	}
	endless := pageGetterFunc(func(_ string, page int) (string, error) {
		return `{"status":"OK","data":[{"p":` + strconv.Itoa(page) + `}],"links":{"next":"/v1/x?page=` + strconv.Itoa(page+1) + `"}}`, nil
	})
	if rows, err := fetchAllBlumiraPages(context.Background(), endless, "/x", 3); err == nil || rows != nil {
		t.Fatalf("a walk cut off at maxPages must fail, got rows=%d err=%v", len(rows), err)
	}
}
