// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// MaskCredentials masks every configured credential, in every encoding the
// client masks in its own errors, in text that hand-written commands build
// from a response (kaseya-vsa handfix: response-text-masked).
func (c *Client) MaskCredentials(text string) string {
	return c.maskCredentialText(text)
}
