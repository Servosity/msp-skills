// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"jira-service-management-pp-cli/internal/config"
)

func TestRequestGuard(t *testing.T) {
	calls := 0
	prev := jsmFetchTenantCloudID
	jsmFetchTenantCloudID = func(_ context.Context, _ *http.Client, site string) (string, error) {
		calls++
		return "cloud-" + site, nil
	}
	wsCalls := 0
	prevWS := jsmFetchSiteWorkspaceIDs
	jsmFetchSiteWorkspaceIDs = func(_ context.Context, _ *http.Client, site, auth string) ([]string, error) {
		wsCalls++
		if auth != "Basic test-auth" {
			t.Fatalf("workspace lookup got Authorization %q, want the request's own", auth)
		}
		return []string{"w"}, nil
	}
	t.Cleanup(func() {
		jsmFetchTenantCloudID, jsmFetchSiteWorkspaceIDs = prev, prevWS
		jsmTenantCloudIDs.Clear()
		jsmWorkspaceIDs.Clear()
	})
	c := &Client{Config: &config.Config{TemplateVars: map[string]string{"site": "acme"}}}
	send := func(raw string) (*http.Request, error) {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		req.Header.Set("Authorization", "Basic test-auth")
		return req, jsmBeforeRequest(context.Background(), c, req)
	}

	req, err := send("https://acme.atlassian.net/rest/servicedeskapi/requesttype")
	if err != nil || req.Header.Get("X-ExperimentalApi") != "opt-in" {
		t.Fatalf("request API call: err=%v header=%q", err, req.Header.Get("X-ExperimentalApi"))
	}
	if _, err := send("https://api.atlassian.com/ex/jira/cloud-acme/jsm/assets/workspace/w/v1/object/1"); err != nil {
		t.Fatalf("own site's cloudId refused: %v", err)
	}
	req, err = send("https://api.atlassian.com/ex/jira/cloud-beta/jsm/assets/workspace/w/v1/object/1")
	if err == nil || !strings.Contains(err.Error(), "not site acme's cloudId") {
		t.Fatalf("another site's cloudId allowed: %v", err)
	}
	if req.Header.Get("X-ExperimentalApi") != "" {
		t.Fatal("Assets call got the request-API header")
	}
	if _, err := send("https://api.atlassian.com/ex/jira/cloud-acme/jsm/assets/workspace/other/v1/object/1"); err == nil || !strings.Contains(err.Error(), "workspaceId other is not an Assets workspace of site acme") {
		t.Fatalf("another site's workspaceId allowed: %v", err)
	}
	if _, err := send("https://api.atlassian.com./ex/jira/cloud-beta/jsm/assets/workspace/w/v1/object/1"); err == nil {
		t.Fatal("trailing-dot host skipped the guard")
	}
	if calls != 1 || wsCalls != 1 {
		t.Fatalf("tenant_info fetched %d times, workspace list %d times, want once per site", calls, wsCalls)
	}
	c.Config.TemplateVars["site"] = ""
	if _, err := send("https://api.atlassian.com/ex/jira/cloud-acme/jsm/assets/workspace/w/v1/object/1"); err == nil {
		t.Fatal("Assets call allowed with no site to check against")
	}
}

type jsmRoundTrip func(*http.Request) (*http.Response, error)

func (f jsmRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchSiteWorkspaceIDs(t *testing.T) {
	status, body := 0, ""
	hc := &http.Client{Transport: jsmRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "acme.atlassian.net" || r.Header.Get("Authorization") != "Basic test-auth" {
			t.Fatalf("workspace list went to %s with Authorization %q", r.URL.Host, r.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	fetch := func() ([]string, error) {
		return jsmFetchSiteWorkspaceIDs(context.Background(), hc, "acme", "Basic test-auth")
	}

	status, body = 200, `{"values":[{"workspaceId":"w1"},{"workspaceId":""}]}`
	if ids, err := fetch(); err != nil || len(ids) != 1 || ids[0] != "w1" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	status, body = 200, `{"values":[]}`
	if _, err := fetch(); err == nil {
		t.Fatal("empty workspace list accepted")
	}
	status, body = 401, ""
	if _, err := fetch(); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("401 must read as an auth error: %v", err)
	}
	status = 404
	if _, err := fetch(); err == nil || strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("404 must not read as resource-not-found: %v", err)
	}
}
