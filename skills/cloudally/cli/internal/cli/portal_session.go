// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// Resolution strategies for turning a requested date into a backup.
//
// resolveLastBackup uses only the documented API, which reports one date per
// mailbox and sub-source: the most recent backup. It can confirm that the
// latest backup is at or before a date, but it cannot find an older one.
//
// resolveSnapshots reads the customer portal's per-mailbox backup calendar
// (see package portal), which lists every past run. This is what an
// end-of-year restore needs.
const (
	resolveAuto       = "auto"
	resolveSnapshots  = "snapshots"
	resolveLastBackup = "last-backup"
)

// resolveModes lists the accepted --resolve values for help and validation.
var resolveModes = []string{resolveAuto, resolveSnapshots, resolveLastBackup}

// Provenance recorded on each plan row, so a resolved date is never
// mistaken for a different kind of lookup.
const (
	dateSourceSnapshots  = "snapshots"
	dateSourceLastBackup = "last-backup"
)

// resolveFlagHelp documents --resolve.
const resolveFlagHelp = "How to find each mailbox's backup: snapshots (read the portal's per-mailbox backup calendar; needed for any past date), last-backup (use only the latest backup date the documented API reports), or auto (snapshots when the scope allows it, otherwise last-backup)"

// portalURLFlagHelp documents the override used for testing.
const portalURLFlagHelp = "Customer portal origin to read backup calendars from (default: CLOUDALLY_PORTAL_URL, else https://app.cloudally.com when the API is api.cloudally.com)"

// errPortalNeedsAccount explains why snapshot resolution cannot run.
var errPortalNeedsAccount = errors.New("reading the portal's backup calendar needs a partner login and a named customer: pass --account <id> (with --reseller for a reseller's customer)")

// errPortalInHarness blocks portal reads under a Printing Press harness.
var errPortalInHarness = errors.New("portal reads are disabled under a test harness")

// handoffTokenResponse is the body of the partner-access-token endpoint. The
// token is opaque and single-use; it is never logged or printed.
type handoffTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

// exportPath is the create-export (download job) endpoint for this scope.
func (s restoreScope) exportPath() string { return s.basePath() + "/recovery/export" }

// partnerAccessTokenPath is the endpoint that mints a customer handoff
// token. It exists only under the partner path families, so an
// account-scoped login has nowhere to call.
func (s restoreScope) partnerAccessTokenPath() (string, error) {
	if s.account == "" {
		return "", usageErr(errPortalNeedsAccount)
	}
	return s.basePath() + "/partner-access-token", nil
}

// mintPortalHandoff asks the documented API for a single-use handoff token
// for the customer named by scope.
func mintPortalHandoff(ctx context.Context, c *client.Client, scope restoreScope) (string, error) {
	path, err := scope.partnerAccessTokenPath()
	if err != nil {
		return "", err
	}
	raw, status, err := c.Post(ctx, path, map[string]any{})
	if err != nil {
		return "", err
	}
	if status < 200 || status > 299 {
		return "", fmt.Errorf("minting a portal handoff token: POST %s returned %d", path, status)
	}
	var body handoffTokenResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("decoding the portal handoff token response: %w", err)
	}
	if strings.TrimSpace(body.AccessToken) == "" {
		return "", fmt.Errorf("POST %s returned no accessToken", path)
	}
	return body.AccessToken, nil
}

// openPortalSession mints a handoff token and opens a customer portal
// session with it. Everything it does is read-only, but it is still refused
// under a Printing Press harness so a shipcheck or dogfood run never reaches
// a real customer's portal. The portal origin is settled before the token is
// minted, so a refusal spends nothing.
func openPortalSession(ctx context.Context, c *client.Client, scope restoreScope, portalURL string) (*portal.Session, error) {
	if cliutil.IsAnyHarness() {
		return nil, errPortalInHarness
	}
	resolved, err := resolvePortalURL(portalURL, c.BaseURL)
	if err != nil {
		return nil, err
	}
	token, err := mintPortalHandoff(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	sess, err := openPortal(ctx, portal.Config{BaseURL: resolved}, token)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// openPortal is portal.Open, held in a variable so a test can see which
// origin a handoff token would be sent to without any network.
var openPortal = portal.Open

// cloudallyPortalURLEnv overrides the customer portal origin. It is read
// here, in internal/cli, and never in internal/config or internal/client.
const cloudallyPortalURLEnv = "CLOUDALLY_PORTAL_URL"

// cloudallyDefaultAPIHost is the documented API host. The production portal
// (portal.DefaultBaseURL) is assumed only for this API.
const cloudallyDefaultAPIHost = "api.cloudally.com"

// resolvePortalURL picks the customer portal origin. An explicit --portal-url
// wins, then CLOUDALLY_PORTAL_URL, then the production portal, but the last
// only when the API itself is production. A handoff token minted by any other
// API (CLOUDALLY_BASE_URL pointing at a test stub, a proxy or another
// deployment) is never sent to the production portal by default: the run is
// refused and names the variable to set. --portal-url at its default value
// counts as unset, because the flag cannot tell "not passed" from "passed
// the default".
//
// Whatever is chosen must be https, except on a loopback host (a local test
// portal): the single-use handoff token travels in the URL, so a plain-http
// portal anywhere else would send it in cleartext. This is checked here,
// before the token is minted.
func resolvePortalURL(flagValue, apiBaseURL string) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" && !strings.EqualFold(strings.TrimRight(v, "/"), portal.DefaultBaseURL) {
		return checkPortalURL(v, "--portal-url")
	}
	if v := strings.TrimSpace(cliutil.EnvOverride(cloudallyPortalURLEnv)); v != "" {
		return checkPortalURL(v, cloudallyPortalURLEnv)
	}
	if isDefaultCloudallyAPI(apiBaseURL) {
		return portal.DefaultBaseURL, nil
	}
	return "", usageErr(fmt.Errorf("the API base URL (CLOUDALLY_BASE_URL or base_url in the config) points at %s, not %s, so the customer portal cannot be assumed to be %s: the portal handoff token would be minted by one service and spent at another. Set %s (or --portal-url) to the portal that belongs to this API; set it to %s only if that really is the matching portal",
		describeOrigin(apiBaseURL), cloudallyDefaultAPIHost, portal.DefaultBaseURL, cloudallyPortalURLEnv, portal.DefaultBaseURL))
}

// checkPortalURL accepts an https portal origin, or http on a loopback
// host. source names where the value came from, for the message.
func checkPortalURL(raw, source string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", usageErr(fmt.Errorf("%s must be a portal origin such as https://app.cloudally.com, got %q", source, describeOrigin(raw)))
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return raw, nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return raw, nil
		}
		return "", usageErr(fmt.Errorf("%s is %s: the portal handoff token is sent in the URL, so the portal must use https (plain http is accepted only on a loopback test host)", source, describeOrigin(raw)))
	default:
		return "", usageErr(fmt.Errorf("%s has scheme %q; the portal must use https", source, u.Scheme))
	}
}

// isLoopbackHost reports whether host is localhost or a loopback address.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isDefaultCloudallyAPI reports whether baseURL is the production API over
// https (any path). An empty value is the built-in default.
func isDefaultCloudallyAPI(baseURL string) bool {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), cloudallyDefaultAPIHost) {
		return false
	}
	return u.Port() == "" || u.Port() == "443"
}

// describeOrigin renders scheme://host for a message, never a path or query.
func describeOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "a non-default URL"
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// openPortalResolver opens a portal session and wraps it in a month-caching
// resolver.
func openPortalResolver(ctx context.Context, c *client.Client, scope restoreScope, portalURL string, lookback int) (*portal.Resolver, error) {
	sess, err := openPortalSession(ctx, c, scope, portalURL)
	if err != nil {
		return nil, err
	}
	return portal.NewResolver(sess, lookback), nil
}

// validateResolveMode checks --resolve and reports whether the portal is
// reachable for this scope. mode is normalised to lower case.
func validateResolveMode(mode string, scope restoreScope) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "":
		mode = resolveAuto
	case resolveAuto, resolveSnapshots, resolveLastBackup:
	default:
		return "", usageErr(fmt.Errorf("--resolve %q is not one of %s", mode, strings.Join(resolveModes, ", ")))
	}
	if mode == resolveSnapshots && scope.account == "" {
		return "", usageErr(errPortalNeedsAccount)
	}
	return mode, nil
}

// wantsSnapshotResolution reports whether a run should read the portal.
// auto does so only when the scope names a customer and no harness is in
// play, so an account-token login keeps working without a new flag.
func wantsSnapshotResolution(mode string, scope restoreScope) bool {
	switch mode {
	case resolveSnapshots:
		return true
	case resolveLastBackup:
		return false
	default:
		return scope.account != "" && !cliutil.IsAnyHarness()
	}
}

// addResolveFlags attaches the resolution flags shared by plan and submit.
func addResolveFlags(cmd *cobra.Command, mode *string, portalURL *string, lookback *int) {
	cmd.Flags().StringVar(mode, "resolve", resolveAuto, resolveFlagHelp)
	cmd.Flags().StringVar(portalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	cmd.Flags().IntVar(lookback, "lookback-months", portal.DefaultMaxLookbackMonths,
		"How many months back a snapshot search may walk before reporting NO_BACKUP")
	// The portal origin is an escape hatch for a white-label host or a
	// test server, not part of the everyday surface.
	_ = cmd.Flags().MarkHidden("portal-url")
}
