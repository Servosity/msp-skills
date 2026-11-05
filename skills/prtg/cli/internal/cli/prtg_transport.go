// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: PRTG transport settings applied to every CLI and MCP client.

package cli

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/client"
)

// insecureTLS is the --insecure flag for this process.
var insecureTLS bool

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		root.PersistentFlags().BoolVar(&insecureTLS, "insecure", false,
			"Skip TLS certificate verification for a core with a self-signed certificate (env: PRTG_SKIP_TLS_VERIFY=true)")
	})
	registerClientHook(applyPRTGTransport)
}

// skipTLSVerify reports whether certificate checks are off for this process:
// --insecure, or PRTG_SKIP_TLS_VERIFY set to 1/true/yes/on.
func skipTLSVerify() bool {
	if insecureTLS {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PRTG_SKIP_TLS_VERIFY"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// applyPRTGTransport turns off certificate verification only when asked
// (PRTG cores commonly ship self-signed certificates) and accepts an API key
// pasted with its "Bearer " prefix, which would otherwise be sent twice.
func applyPRTGTransport(c *client.Client) error {
	if c == nil {
		return nil
	}
	if c.Config != nil {
		key := strings.TrimSpace(c.Config.PrtgApiKey)
		if len(key) > 7 && strings.EqualFold(key[:7], "bearer ") {
			c.Config.PrtgApiKey = strings.TrimSpace(key[7:])
		}
	}
	// The pin, cache and minted-credential guards wrap the transport last,
	// over the TLS setting (prtg_guard.go).
	defer guardClient(c)
	if !skipTLSVerify() || c.HTTPClient == nil {
		return nil
	}
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		return nil
	}
	tr = tr.Clone()
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	tr.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- explicit opt-in via --insecure / PRTG_SKIP_TLS_VERIFY
	c.HTTPClient.Transport = tr
	return nil
}
