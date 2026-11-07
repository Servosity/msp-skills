// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written shared plumbing for the novel commands (handfixes.json
// tracker-novels): store opening, the outbound guard, and the CLI-only gates
// that an MCP tool call must never pass.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"domain-cert-tracker-pp-cli/internal/client"
	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
	"domain-cert-tracker-pp-cli/internal/tracker"
)

const trackerCLIName = "domain-cert-tracker-cli"

// Every client the generated commands build (CLI and MCP) dials through the
// guard: public addresses only, port 443 only, https-only redirects. The one
// exception is the operator's own origin, and only for a client whose base
// URL has that origin: a private RDAP server or a test stand-in there is
// reached as configured. Under MCP (the server process or a mirrored child)
// that origin comes only from the DOMAIN_CERT_TRACKER_BASE_URL environment
// variable; on the operator's own command line a config-file base_url counts
// too. No other client (crt.sh, IANA) can be redirected into it.
func init() {
	registerClientHook(func(c *client.Client) error {
		trackerClientGuard(c.BaseURL).Install(c.HTTPClient)
		return nil
	})
	// sweep copies RDAP, CT, TLS and DNS results into the store, so the
	// operator's DOMAIN_CERT_TRACKER_NO_LOCAL_DATA=1 switch refuses it the
	// same way it refuses the generated sync.
	localDataWriterCommands["sweep"] = true
}

// trackerClientGuard is the policy for generated clients. A base URL that
// does not parse grants nothing; the client itself then reports it.
func trackerClientGuard(baseURL string) *netguard.Guard {
	g := netguard.New()
	env := strings.TrimSpace(cliutil.EnvOverride("DOMAIN_CERT_TRACKER_BASE_URL"))
	if env == "" && !mcpServerProcess && !cliutil.IsMCPChildEnv() {
		env = baseURL // the config file read by the operator's own command line
	}
	if env == "" || urlOrigin(env) == "" || urlOrigin(env) != urlOrigin(baseURL) {
		return g
	}
	if og, err := g.WithOperatorOrigin(env); err == nil {
		return og
	}
	return g
}

// mcpServerProcess is set by the MCP server binary before it serves a tool.
var mcpServerProcess bool

// MarkMCPServer records that this process is the MCP server, so a config-file
// base_url never widens the outbound guard for tool calls.
func MarkMCPServer() { mcpServerProcess = true }

// MCPServerMarked reports whether MarkMCPServer ran.
func MCPServerMarked() bool { return mcpServerProcess }

// urlOrigin is scheme://host:port with the default port filled in, or ""
// when raw is not an http(s) URL with a host.
func urlOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	switch {
	case scheme == "https" && port == "":
		port = "443"
	case scheme == "http" && port == "":
		port = "80"
	case scheme != "https" && scheme != "http":
		return ""
	}
	return scheme + "://" + strings.TrimSuffix(strings.ToLower(u.Hostname()), ".") + ":" + port
}

// refuseUnderMCP blocks an operator-only capability when the MCP server
// spawned this process. The MCP server always sets the marker itself and
// strips any inherited copy, so a tool call cannot clear it.
func refuseUnderMCP(what string) error {
	if cliutil.IsMCPChildEnv() {
		return usageErr(fmt.Errorf("%s is available on the command line only, not through MCP", what))
	}
	return nil
}

// trackerGuard builds this invocation's outbound policy. allowPorts comes
// only from the --allow-port flag typed on the command line.
func trackerGuard(allowPorts []int) (*netguard.Guard, error) {
	g := netguard.New()
	if len(allowPorts) == 0 {
		return g, nil
	}
	if err := refuseUnderMCP("--allow-port"); err != nil {
		return nil, err
	}
	for _, p := range allowPorts {
		if p < 1 || p > 65535 {
			return nil, usageErr(fmt.Errorf("--allow-port %d is not a TCP port", p))
		}
	}
	return g.WithPorts(allowPorts...), nil
}

func trackerSources(g *netguard.Guard) tracker.Sources {
	return tracker.Sources{
		RDAP:  &tracker.RDAP{Web: tracker.NewWeb(g, 1, 4<<20)},
		CT:    &tracker.CT{Web: tracker.NewWeb(g, 0.5, 48<<20)},
		TLS:   &tracker.TLSProber{Guard: g},
		DNS:   &tracker.SystemDNS{},
		Guard: g,
	}
}

func openTrackerWrite(ctx context.Context) (*store.Store, error) {
	return store.OpenWithContext(ctx, defaultDBPath(trackerCLIName))
}

// openTrackerRead returns nil, nil when no store exists yet.
func openTrackerRead(ctx context.Context) (*store.Store, error) {
	return openStoreForRead(ctx, trackerCLIName)
}

// parseHostTarget validates "host[:port]" for the inventory. An IP literal
// must be public; a name is checked when it is resolved.
func parseHostTarget(s string) (string, int, error) {
	host, port, err := netguard.SplitHostPort(s)
	if err != nil {
		return "", 0, err
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if err := netguard.CheckAddr(a); err != nil {
			return "", 0, err
		}
		return a.String(), port, nil
	}
	norm, err := tracker.NormalizeDomain(host)
	if err != nil {
		return "", 0, err
	}
	return norm, port, nil
}

func splitList(values []string) []string {
	var out []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// parseDaysFlag accepts "30", "30d", "4w" or "720h" and returns whole days.
func parseDaysFlag(name, v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, usageErr(fmt.Errorf("--%s is empty", name))
	}
	if !strings.ContainsAny(v, "hdwms") {
		v += "d"
	}
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a number of days (for example 30 or 30d)", name, v))
	}
	// Downstream a zero window means "use the default", so a value under one
	// day is refused here rather than silently becoming 30.
	days := int(d / (24 * time.Hour))
	if days < 1 {
		return 0, usageErr(fmt.Errorf("--%s %q is under one day; pass at least 1", name, v))
	}
	return days, nil
}

// trackerErr maps tracker sentinel errors to typed exit codes.
func trackerErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, tracker.ErrUnknownClient) {
		return usageErr(err)
	}
	return err
}
