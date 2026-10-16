// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// refused unless its cloudId is the current site's, read once per process
// from the site's public tenant_info. A workspaceId outside that cloud is
// rejected by Atlassian itself.
func jsmBeforeRequest(ctx context.Context, c *Client, req *http.Request) error {
	if strings.Contains(req.URL.Path, "/rest/servicedeskapi/") && req.Header.Get("X-ExperimentalApi") == "" {
		req.Header.Set("X-ExperimentalApi", "opt-in")
	}
	return jsmCheckAssetsCloud(ctx, c, req.URL)
}

func jsmCheckAssetsCloud(ctx context.Context, c *Client, u *url.URL) error {
	if !strings.EqualFold(u.Hostname(), "api.atlassian.com") {
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
		return fmt.Errorf("Assets call refused: cloudId %s is not site %s's cloudId (%s). A JIRA_SERVICE_MANAGEMENT_CLOUD_ID / _WORKSPACE_ID from another client site is set: unset them, or run 'cmdb discover --save' for this site", cloudID, site, want)
	}
	return nil
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
		return "", fmt.Errorf("tenant_info returned HTTP %d", resp.StatusCode)
	}
	var t struct {
		CloudID string `json:"cloudId"`
	}
	if json.Unmarshal(body, &t) != nil || t.CloudID == "" {
		return "", fmt.Errorf("tenant_info returned no cloudId")
	}
	return t.CloudID, nil
}
