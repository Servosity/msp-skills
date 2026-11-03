// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Opt-in TLS verification skip for FortiGates that still use their factory
// self-signed certificate. The standard transport has no such switch, so it
// is added here through the client-hook seam: --insecure (any command) or
// FORTINET_SKIP_TLS_VERIFY=true (also honored by the MCP server). Fleet
// devices opt in individually with `insecure = true` in devices.toml.

package cli

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/client"
	"fortinet-pp-cli/internal/cliutil"
)

// ponytail: process-wide flag; set once by the root --insecure flag before any client exists.
var insecureFlag bool

func insecureRequested() bool {
	return insecureFlag || strings.EqualFold(strings.TrimSpace(cliutil.EnvOverride("FORTINET_SKIP_TLS_VERIFY")), "true")
}

// skipTLSVerify turns off certificate verification on c's transport.
func skipTLSVerify(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	rt := c.HTTPClient.Transport
	guard, guarded := rt.(*secretGuard)
	if guarded {
		rt = guard.next
	}
	tr, ok := rt.(*http.Transport)
	if !ok {
		return
	}
	tr = tr.Clone()
	cfg := &tls.Config{}
	if tr.TLSClientConfig != nil {
		cfg = tr.TLSClientConfig.Clone()
	}
	cfg.InsecureSkipVerify = true //nolint:gosec // explicit --insecure / FORTINET_SKIP_TLS_VERIFY / per-device opt-in for self-signed FortiGate certs
	tr.TLSClientConfig = cfg
	if guarded {
		c.HTTPClient.Transport = &secretGuard{next: tr}
		return
	}
	c.HTTPClient.Transport = tr
}

// refuseOffOriginRedirects wraps c's redirect policy so a hop to another
// scheme, host or port is refused. Go strips Authorization on a cross-host
// redirect but copies every other header, so a configured credential header
// (a gateway's X-API-Key) would reach the other host; a FortiGate API has no
// reason to redirect off its own origin. Same-origin hops still go through
// the generated policy.
func refuseOffOriginRedirects(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	next := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && via[0].URL != nil && req.URL != nil && !sameOrigin(req.URL, via[0].URL) {
			first := via[0].URL
			return fmt.Errorf("refusing redirect from %s://%s to %s://%s: a FortiGate API does not redirect off its own origin, and the request carries credentials", first.Scheme, first.Host, req.URL.Scheme, req.URL.Host)
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
}

// sameOrigin compares scheme, host name and port, with the scheme's default
// port filled in.
func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if strings.EqualFold(u.Scheme, "http") {
			return "80"
		}
		return "443"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func init() {
	registerClientHook(func(c *client.Client) error {
		// Every client (CLI, fleet, MCP) sends a bearer token on each request:
		// refuse plain http except to a loopback test host.
		if _, err := checkAPIURL(c.BaseURL); err != nil {
			return err
		}
		if insecureRequested() {
			skipTLSVerify(c)
		}
		guardSecrets(c)
		refuseOffOriginRedirects(c)
		return nil
	})
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("insecure") == nil {
			root.PersistentFlags().BoolVar(&insecureFlag, "insecure", false, "Skip TLS certificate verification (FortiGate/FortiManager factory self-signed certificates); also FORTINET_SKIP_TLS_VERIFY=true")
		}
	})
}
