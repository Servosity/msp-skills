// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/config"
)

func TestLinkNextToken(t *testing.T) {
	cases := []struct {
		name, header, want string
	}{
		{"unquoted rel", `<https://api.meraki.com/api/v1/organizations?perPage=3&startingAfter=1000>; rel=first, <https://api.meraki.com/api/v1/organizations?perPage=3&startingAfter=1003>; rel=next`, "1003"},
		{"quoted rel", `<https://api.meraki.com/api/v1/x?startingAfter=Q2XX-AAAA-BBBB>; rel="next"`, "Q2XX-AAAA-BBBB"},
		{"last page has no next", `<https://api.meraki.com/api/v1/x?startingAfter=a>; rel=first, <https://api.meraki.com/api/v1/x?endingBefore=z>; rel=prev`, ""},
		{"empty", "", ""},
		{"escaped token", `<https://api.meraki.com/api/v1/x?startingAfter=2026-10-13T00%3A00%3A00Z>; rel=next`, "2026-10-13T00:00:00Z"},
	}
	for _, tc := range cases {
		if got := linkNextToken(tc.header, merakiCursorParam); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestMerakiLinkPaginationDefaults(t *testing.T) {
	got := merakiLinkPagination(paginationDefaults{cursorType: "offset", limitParam: "perPage", limit: 100})
	if got.cursorParam != merakiCursorParam || got.cursorType != "cursor" {
		t.Fatalf("perPage-only default not converted: %+v", got)
	}
	other := merakiLinkPagination(paginationDefaults{cursorParam: "page", cursorType: "page", limitParam: "perPage"})
	if other.cursorParam != "page" {
		t.Fatalf("explicit cursor overwritten: %+v", other)
	}
}

// fakeMeraki serves total items in pages of perPage, linking pages with a
// Link header the way the Dashboard API does, and counts requests.
func fakeMeraki(t *testing.T, total int, requests *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if r.Header.Get("X-Cisco-Meraki-API-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":["No valid authentication method found"]}`))
			return
		}
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		if perPage <= 0 {
			perPage = 3
		}
		start := 0
		if sa := r.URL.Query().Get("startingAfter"); sa != "" {
			n, _ := strconv.Atoi(sa)
			start = n
		}
		end := start + perPage
		if end > total {
			end = total
		}
		items := make([]map[string]any, 0)
		for i := start; i < end; i++ {
			items = append(items, map[string]any{"id": strconv.Itoa(100 + i), "name": fmt.Sprintf("Org %d", i)})
		}
		link := fmt.Sprintf(`<http://%s%s?perPage=%d>; rel=first`, r.Host, r.URL.Path, perPage)
		if end < total {
			link += fmt.Sprintf(`, <http://%s%s?perPage=%d&startingAfter=%d>; rel=next`, r.Host, r.URL.Path, perPage, end)
		}
		w.Header().Set("Link", link)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
}

func testMerakiClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	cfg := &config.Config{BaseURL: baseURL, MerakiDashboardApiKey: "test-key"}
	c := client.New(cfg, 10*time.Second, 0)
	c.NoCache = true
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMerakiGetAllWalksEveryLinkPage(t *testing.T) {
	requests := 0
	srv := fakeMeraki(t, 7, &requests)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)

	res, err := merakiGetAll(context.Background(), c, "/organizations", map[string]string{"perPage": "3"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.ItemsReturned != 7 || len(res.Items) != 7 || !res.Complete || res.PagesFetched != 3 {
		t.Fatalf("want 7 items over 3 complete pages, got %+v", res)
	}
	if requests != 3 {
		t.Fatalf("want 3 requests, got %d", requests)
	}
}

func TestMerakiGetAllStopsAtPageCapWithResumeCursor(t *testing.T) {
	requests := 0
	srv := fakeMeraki(t, 7, &requests)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)

	res, err := merakiGetAll(context.Background(), c, "/organizations", map[string]string{"perPage": "3"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Complete || res.ItemsReturned != 6 || res.NextCursor != "6" || res.LimitReason == "" {
		t.Fatalf("capped walk must report incomplete with resume cursor, got %+v", res)
	}
}

func TestPaginatedGetAllFollowsLinkHeader(t *testing.T) {
	requests := 0
	srv := fakeMeraki(t, 8, &requests)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	c.NoCache = false // --all must still see every page when the cache is on

	data, err := paginatedGet(context.Background(), c, "/organizations", map[string]string{"perPage": "3"}, nil, true, "", "offset", "perPage", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("not an array: %s", data)
	}
	if len(items) != 8 {
		t.Fatalf("--all returned %d of 8 items", len(items))
	}
}

func TestPaginatedGetWithoutAllReturnsOnePage(t *testing.T) {
	requests := 0
	srv := fakeMeraki(t, 8, &requests)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)

	data, err := paginatedGet(context.Background(), c, "/organizations", map[string]string{"perPage": "3"}, nil, false, "", "offset", "perPage", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	_ = json.Unmarshal(data, &items)
	if len(items) != 3 || requests != 1 {
		t.Fatalf("single-page read changed: %d items, %d requests", len(items), requests)
	}
}

// fakeEnvelope serves {"clientId":..,"records":[...]} pages linked by Link,
// the clients/search shape.
func fakeEnvelope(t *testing.T, total int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startingAfter"))
		end := start + 2
		if end > total {
			end = total
		}
		recs := make([]map[string]any, 0)
		for i := start; i < end; i++ {
			recs = append(recs, map[string]any{"network": map[string]any{"id": fmt.Sprintf("N_%d", i)}})
		}
		link := fmt.Sprintf(`<http://%s%s>; rel=first`, r.Host, r.URL.Path)
		if end < total {
			link += fmt.Sprintf(`, <http://%s%s?startingAfter=%d>; rel=next`, r.Host, r.URL.Path, end)
		}
		w.Header().Set("Link", link)
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "k1", "mac": "aa:bb:cc:dd:ee:ff", "records": recs})
	}))
}

func TestMerakiGetAllMergesEnvelopePages(t *testing.T) {
	srv := fakeEnvelope(t, 5)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	res, err := merakiGetAll(context.Background(), c, "/organizations/1/clients/search", map[string]string{"mac": "aa:bb:cc:dd:ee:ff"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		ClientID string           `json:"clientId"`
		Records  []map[string]any `json:"records"`
	}
	if len(res.Items) != 1 || json.Unmarshal(res.Items[0], &env) != nil {
		t.Fatalf("want one merged envelope, got %d items", len(res.Items))
	}
	if env.ClientID != "k1" || len(env.Records) != 5 || res.ItemsReturned != 5 || !res.Complete || res.PagesFetched != 3 {
		t.Fatalf("envelope pages not merged: client=%q records=%d res=%+v", env.ClientID, len(env.Records), res)
	}
}

func TestPaginatedGetAllFollowsEnvelopeLinks(t *testing.T) {
	srv := fakeEnvelope(t, 5)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	data, err := paginatedGet(context.Background(), c, "/organizations/1/clients/search", map[string]string{"mac": "aa:bb:cc:dd:ee:ff"}, nil, true, "", "offset", "perPage", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Records []map[string]any `json:"records"`
	}
	var arr []map[string]any
	got := 0
	if json.Unmarshal(data, &env) == nil && env.Records != nil {
		got = len(env.Records)
	} else if json.Unmarshal(data, &arr) == nil {
		got = len(arr)
	}
	if got != 5 {
		t.Fatalf("--all on an envelope returned %d of 5 records: %s", got, data)
	}
}

func TestLinkProvenEnd(t *testing.T) {
	p := merakiLinkPagination(paginationDefaults{limitParam: "perPage", limit: 2})
	s := &linkSink{}
	if linkProvenEnd(s, p) {
		t.Fatal("no live response seen yet must not prove the end")
	}
	s.record(http.Header{"Link": []string{`<https://x/y?perPage=2>; rel=first`}})
	if !linkProvenEnd(s, p) {
		t.Fatal("live page without rel=next is the proven end")
	}
	s.record(http.Header{"Link": []string{`<https://x/y?startingAfter=2>; rel=next`}})
	if linkProvenEnd(s, p) {
		t.Fatal("rel=next present means more pages")
	}
}

func TestMerakiGetAllRepeatedCursorIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?startingAfter=stuck>; rel=next`, r.Host, r.URL.Path))
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "1"}})
	}))
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	if _, err := merakiGetAll(context.Background(), c, "/organizations", nil, 0); err == nil {
		t.Fatal("a repeated cursor must be an error, not a silent partial list")
	}
}

func TestPaginatedGetAllStopsOnRepeatedLinkCursor(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?startingAfter=same>; rel=next`, r.Host, r.URL.Path))
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": strconv.Itoa(requests)}})
	}))
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	if _, err := paginatedGet(context.Background(), c, "/organizations", map[string]string{"perPage": "3"}, nil, true, "", "offset", "perPage", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if requests > 3 {
		t.Fatalf("repeated Link cursor walked %d pages instead of stopping", requests)
	}
}

func TestPaginatedGetResponsePathBypassesCache(t *testing.T) {
	srv := fakeEnvelope(t, 5)
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	c.NoCache = false
	params := map[string]string{"mac": "aa:bb:cc:dd:ee:ff"}
	// Warm the cache with page 1, then walk every page through the response-path wrapper.
	if _, err := c.GetWithHeaders(context.Background(), "/organizations/1/clients/search", params, nil); err != nil {
		t.Fatal(err)
	}
	data, err := paginatedGetWithResponsePath(context.Background(), c, "/organizations/1/clients/search", params, nil, true, "", "offset", "perPage", 0, "", "", "records")
	if err != nil {
		t.Fatal(err)
	}
	var arr []map[string]any
	var env map[string]json.RawMessage
	got := 0
	if json.Unmarshal(data, &arr) == nil {
		got = len(arr)
	} else if json.Unmarshal(data, &env) == nil {
		for _, v := range env {
			var items []map[string]any
			if json.Unmarshal(v, &items) == nil && len(items) > got {
				got = len(items)
			}
		}
	}
	if got != 5 {
		t.Fatalf("response-path --all after a warm cache returned %d of 5 records: %s", got, data)
	}
}

func TestLinkAdvanceStopsCursorCycle(t *testing.T) {
	p := merakiLinkPagination(paginationDefaults{limitParam: "perPage", limit: 2})
	page := func(sent, next string) *linkSink {
		s := &linkSink{}
		u, _ := url.Parse("https://api.meraki.com/api/v1/organizations/1/devices")
		if sent != "" {
			u.RawQuery = "startingAfter=" + sent
		}
		s.recordRequest(u)
		s.record(http.Header{"Link": []string{"<https://api.meraki.com/api/v1/organizations/1/devices?startingAfter=" + next + ">; rel=next"}})
		return s
	}
	last := ""
	for i, step := range []struct{ sent, next, want string }{{"", "A", "A"}, {"A", "B", "B"}, {"B", "A", "B"}} {
		got, _ := linkAdvance(page(step.sent, step.next), p, "", false)
		if got != step.want {
			t.Fatalf("step %d: got %q want %q", i, got, step.want)
		}
		if i == 2 && got != last {
			t.Fatalf("cycle must return the previous cursor so the stuck-cursor guard fires")
		}
		last = got
	}
}

func TestPollActionBatchSurfacesStatusReadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["forbidden"]}`))
	}))
	defer srv.Close()
	c := testMerakiClient(t, srv.URL)
	start := time.Now()
	got := pollActionBatch(context.Background(), c, "/organizations/1/actionBatches/9", time.Minute)
	if !strings.HasPrefix(got, "unknown: status read failed") || time.Since(start) > 10*time.Second {
		t.Fatalf("403 on status read must be reported at once, got %q after %s", got, time.Since(start))
	}
}
