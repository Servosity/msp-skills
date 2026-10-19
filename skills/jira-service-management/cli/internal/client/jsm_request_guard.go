// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// jsmBeforeRequest runs on every real send (dry runs return earlier).
//
// 1. Request types, desk customers and request feedback are experimental
// endpoints that answer HTTP 412 without X-ExperimentalApi: opt-in; sync and
// the MCP execute tool do not send it per call, so every servicedeskapi
// request carries it (Atlassian ignores it on stable endpoints).
//
// 2. One Atlassian token reaches every client site its account belongs to, so
// an Assets cloudId from another client (an exported
// JIRA_SERVICE_MANAGEMENT_CLOUD_ID left over from that client, for example)
// would silently read or delete that client's CMDB. Every Assets request is
// refused unless its cloudId is the current site's (from the site's public
// tenant_info) and its workspaceId is one the site itself lists (from the
// site's /rest/servicedeskapi/assets/workspace, sent with the same
// Authorization header to the same <site>.atlassian.net host). Both answers
// are cached per process; concurrent cold-cache callers may each fetch once.
func jsmBeforeRequest(ctx context.Context, c *Client, req *http.Request) error {
	if strings.Contains(req.URL.Path, "/rest/servicedeskapi/") && req.Header.Get("X-ExperimentalApi") == "" {
		req.Header.Set("X-ExperimentalApi", "opt-in")
	}
	return jsmCheckAssetsCloud(ctx, c, req)
}

func jsmCheckAssetsCloud(ctx context.Context, c *Client, req *http.Request) error {
	u := req.URL
	if strings.TrimSuffix(strings.ToLower(u.Hostname()), ".") != "api.atlassian.com" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) < 4 || parts[0] != "ex" || parts[1] != "jira" || parts[3] != "jsm" {
		return nil
	}
	cloudID := parts[2]
	site := ""
	if c != nil && c.Config != nil {
		site = strings.TrimSpace(c.Config.TemplateVars["site"])
	}
	if site == "" || !jsmSafeTemplateValue.MatchString(site) || strings.Contains(site, "..") {
		return fmt.Errorf("Assets call refused: JIRA_SERVICE_MANAGEMENT_SITE is not set to a site subdomain, so cloudId %s cannot be confirmed to belong to your site", cloudID)
	}
	want, err := jsmSiteCloudID(ctx, c, site)
	if err != nil {
		return fmt.Errorf("Assets call refused: could not confirm site %s's cloudId: %w", site, err)
	}
	if want != cloudID {
		return fmt.Errorf("Assets call refused: cloudId %s is not site %s's cloudId (%s). A JIRA_SERVICE_MANAGEMENT_CLOUD_ID from another client site is probably exported: unset JIRA_SERVICE_MANAGEMENT_CLOUD_ID and _WORKSPACE_ID (environment values win over saved ones), then run 'cmdb discover --save' if this site has no saved ids", cloudID, site, want)
	}
	if len(parts) < 7 || parts[4] != "assets" || parts[5] != "workspace" {
		return nil
	}
	workspaceID := parts[6]
	ids, err := jsmSiteWorkspaceIDs(ctx, c, site, req.Header.Get("Authorization"))
	if err != nil {
		return fmt.Errorf("Assets call refused: could not confirm site %s's Assets workspaceId: %w", site, err)
	}
	for _, id := range ids {
		if id == workspaceID {
			return nil
		}
	}
	return fmt.Errorf("Assets call refused: workspaceId %s is not an Assets workspace of site %s (it lists %s). A JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID from another client site is probably exported: unset it (environment values win over saved ones), then run 'cmdb discover --save' if this site has no saved ids", workspaceID, site, strings.Join(ids, ", "))
}

var jsmTenantCloudIDs sync.Map // site -> cloudId, per process

func jsmSiteCloudID(ctx context.Context, c *Client, site string) (string, error) {
	if v, ok := jsmTenantCloudIDs.Load(site); ok {
		return v.(string), nil
	}
	id, err := jsmFetchTenantCloudID(ctx, c.HTTPClient, site)
	if err != nil {
		return "", err
	}
	jsmTenantCloudIDs.Store(site, id)
	return id, nil
}

// jsmFetchTenantCloudID reads https://<site>.atlassian.net/_edge/tenant_info,
// which is unauthenticated: no credential is sent. A variable so tests can
// stand in for the network.
var jsmFetchTenantCloudID = func(ctx context.Context, hc *http.Client, site string) (string, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+site+".atlassian.net/_edge/tenant_info", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tenant_info answered with status %d", resp.StatusCode)
	}
	var t struct {
		CloudID string `json:"cloudId"`
	}
	if json.Unmarshal(body, &t) != nil || t.CloudID == "" {
		return "", fmt.Errorf("tenant_info returned no cloudId")
	}
	return t.CloudID, nil
}

var jsmWorkspaceIDs sync.Map // site -> []string, per process

func jsmSiteWorkspaceIDs(ctx context.Context, c *Client, site, authorization string) ([]string, error) {
	if v, ok := jsmWorkspaceIDs.Load(site); ok {
		return v.([]string), nil
	}
	ids, err := jsmFetchSiteWorkspaceIDs(ctx, c.HTTPClient, site, authorization)
	if err != nil {
		return nil, err
	}
	jsmWorkspaceIDs.Store(site, ids)
	return ids, nil
}

// jsmFetchSiteWorkspaceIDs lists the site's Assets workspaces. The request goes
// only to https://<site>.atlassian.net, the host that already receives this
// Authorization header for every request-API call. A variable so tests can
// stand in for the network.
// ponytail: one page of 50; a site has one Assets workspace in practice.
var jsmFetchSiteWorkspaceIDs = func(ctx context.Context, hc *http.Client, site, authorization string) ([]string, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+site+".atlassian.net/rest/servicedeskapi/assets/workspace?limit=50", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-ExperimentalApi", "opt-in")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		// "HTTP <code>" so callers classify these as auth (exit 4) or rate-limit errors.
		return nil, fmt.Errorf("the site's Assets workspace list returned HTTP %d", resp.StatusCode)
	case http.StatusNotFound:
		return nil, fmt.Errorf("the site's Assets workspace list answered with status 404 (check that the site has Assets)")
	default:
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("the site's Assets workspace list answered with status %d (try again; a 5xx is usually transient)", resp.StatusCode)
		}
		return nil, fmt.Errorf("the site's Assets workspace list answered with status %d", resp.StatusCode)
	}
	var page struct {
		Values []struct {
			WorkspaceID string `json:"workspaceId"`
		} `json:"values"`
	}
	if json.Unmarshal(body, &page) != nil {
		return nil, fmt.Errorf("the site's Assets workspace list was not JSON")
	}
	ids := make([]string, 0, len(page.Values))
	for _, v := range page.Values {
		if v.WorkspaceID != "" {
			ids = append(ids, v.WorkspaceID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("the site lists no Assets workspace (Assets needs JSM Premium or Enterprise)")
	}
	return ids, nil
}
