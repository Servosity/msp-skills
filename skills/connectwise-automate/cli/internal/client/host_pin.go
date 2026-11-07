// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import "fmt"

// Hand-owned (handfixes.json: base-url-pinned-at-construction). The base URL
// carries a {server} placeholder that Config.TemplateVars fills, and the
// bearer token and clientId go wherever it resolves. The client resolves it
// once, when it is built from the operator's configuration, and refuses any
// request whose base resolves anywhere else afterwards (sync --path-context,
// or any later write to TemplateVars), so a caller-chosen value can never
// move the credentials to another host.

func (c *Client) pinBaseURL() {
	if c.Config == nil {
		return
	}
	vars := make(map[string]string, len(c.Config.TemplateVars))
	for k, v := range c.Config.TemplateVars {
		vars[k] = v
	}
	if base, err := buildURL(c.BaseURL, "", vars); err == nil {
		c.pinnedBaseURL = base
	}
}

func (c *Client) checkPinnedBaseURL(vars map[string]string) error {
	if c.pinnedBaseURL == "" {
		return nil
	}
	base, err := buildURL(c.BaseURL, "", vars)
	if err != nil {
		return err
	}
	if base != c.pinnedBaseURL {
		return fmt.Errorf("refusing to send credentials to %s: the configured server is %s (set CONNECTWISE_AUTOMATE_SERVER or CONNECTWISE_AUTOMATE_BASE_URL to change it)", base, c.pinnedBaseURL)
	}
	return nil
}
