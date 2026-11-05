// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: organization-token routing for requests that still carry the
// reseller token (MCP typed tools have no --org), plus the --org hint on
// organization-scoped 401s. Own file so generate --force preserves it.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/config"
)

// defaultOrgEnv names one organization whose token the organization-scoped
// requests without an organization id in their path (/accounts, /onedrives,
// ...) use when they would otherwise go out with the reseller token. It is
// how an MCP server reads one client's backup data.
const defaultOrgEnv = "DROPSUITE_ORG"

// orgScopedPath reports whether an API path (relative to the base URL) needs
// an organization token rather than the reseller admin token, and returns the
// organization id the path names (/users/{id}/...), if any.
func orgScopedPath(rel string) (scoped bool, pathOrgID string) {
	rel = "/" + strings.Trim(rel, "/")
	switch rel {
	case "/status", "/countries", "/timezones", "/locales", "/users", "/users/generate_support_sso_link":
		return false, ""
	case "/users/sub_users", "/users/generate_redirect_link", "/users/hosted_exchange":
		return true, ""
	}
	if strings.HasPrefix(rel, "/users/") {
		parts := strings.Split(strings.TrimPrefix(rel, "/users/"), "/")
		if len(parts) < 2 {
			return false, "" // /users/{id}: the admin subscription read
		}
		id, err := url.PathUnescape(parts[0])
		if err != nil {
			id = parts[0]
		}
		return true, id
	}
	return true, ""
}

// orgTokenDirectory resolves organization tokens from GET /users (reseller
// token) and keeps them in memory only, one set per API origin and reseller
// credential pair, so a client with other credentials never sees them. A
// successful list is reused for orgTokenTTL (a long-lived MCP server picks up
// new organizations); a failed load is not cached, so a transient error does
// not stick.
type orgTokenDirectory struct {
	mu   sync.Mutex
	sets map[string]*orgTokenSet
}

// orgTokenSet is one loaded list; it is never modified after load.
type orgTokenSet struct {
	loadedAt time.Time
	byID     map[string]string
	orgs     []dsOrg
}

const orgTokenTTL = 10 * time.Minute

var orgTokens orgTokenDirectory

// get returns the token set for origin (the resolved scheme://host/base of
// the request being routed) and the credentials that request carries; the
// list is loaded with exactly those credentials (config headers can override
// the configured fields).
func (d *orgTokenDirectory) get(ctx context.Context, origin, accessToken, resellerToken string, cfgClient *client.Client, next http.RoundTripper) (*orgTokenSet, error) {
	sum := sha256.Sum256([]byte(origin + "\x00" + accessToken + "\x00" + resellerToken))
	key := hex.EncodeToString(sum[:])
	d.mu.Lock()
	defer d.mu.Unlock()
	if s := d.sets[key]; s != nil && time.Since(s.loadedAt) < orgTokenTTL {
		return s, nil
	}
	admin := client.New(cfgClient.Config, 60*time.Second, 0)
	admin.NoCache = true
	admin.HTTPClient.Transport = pinCredentials{next: next, origin: origin, access: accessToken, reseller: resellerToken}
	installRedaction(admin)
	lctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	orgs, err := fetchOrgsRaw(lctx, admin)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]string, len(orgs))
	for _, o := range orgs {
		if o.ID != "" && o.token != "" {
			byID[o.ID] = o.token
		}
	}
	s := &orgTokenSet{loadedAt: time.Now(), byID: byID, orgs: orgs}
	if d.sets == nil {
		d.sets = map[string]*orgTokenSet{}
	}
	d.sets[key] = s
	return s, nil
}

// tokenFor picks the organization token for a request: the organization the
// path names, else the DROPSUITE_ORG organization. ok is false when neither
// applies (the request keeps the reseller token).
func (s *orgTokenSet) tokenFor(pathOrgID, defaultRef string) (token string, ok bool, err error) {
	if pathOrgID != "" {
		token, ok = s.byID[pathOrgID]
		return token, ok, nil
	}
	if defaultRef == "" {
		return "", false, nil
	}
	org, err := matchOrg(s.orgs, defaultRef)
	if err != nil {
		return "", false, fmt.Errorf("%s=%q: %w", defaultOrgEnv, defaultRef, err)
	}
	if org.token == "" {
		return "", false, fmt.Errorf("%s=%q: organization %s has no authentication_token", defaultOrgEnv, defaultRef, org.label())
	}
	return org.token, true, nil
}

// orgRoutingTransport swaps the reseller token for the right organization's
// token on organization-scoped paths. Requests that already carry another
// token (the fan-out code's organization clients, --org) are left alone.
type orgRoutingTransport struct {
	next       http.RoundTripper
	owner      *client.Client
	adminToken string
	basePath   string
}

// isAdminToken reports whether h is the reseller's own credential: the
// effective configured value, or the credential field the generated redirect
// handler re-stamps on a same-origin hop.
func (t orgRoutingTransport) isAdminToken(h string) bool {
	return h != "" && (h == t.adminToken || h == t.owner.Config.AuthHeader())
}

func (t orgRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rel := strings.TrimPrefix(req.URL.Path, t.basePath)
	scoped, pathOrgID := orgScopedPath(rel)
	defaultRef := strings.TrimSpace(os.Getenv(defaultOrgEnv))
	if !scoped || !t.isAdminToken(req.Header.Get("X-Access-Token")) || (pathOrgID == "" && defaultRef == "") {
		return t.next.RoundTrip(req)
	}
	// The directory always loads with the effective reseller credential, even
	// on a redirect hop that re-stamped the credential field.
	set, err := orgTokens.get(req.Context(), req.URL.Scheme+"://"+req.URL.Host+t.basePath,
		t.adminToken, req.Header.Get("X-Reseller-Token"), t.owner, t.next)
	if err != nil {
		return nil, fmt.Errorf("resolving the organization token for %s: %w", rel, err)
	}
	token, ok, err := set.tokenFor(pathOrgID, defaultRef)
	if err != nil {
		return nil, err
	}
	if !ok {
		return t.next.RoundTrip(req) // unknown organization id: let the API answer
	}
	req = req.Clone(req.Context())
	req.Header.Set("X-Access-Token", token)
	return t.next.RoundTrip(req)
}

// pinCredentials sends exactly the credentials of the request that triggered
// a directory load, and only to that request's origin: a redirect elsewhere
// goes out with no credentials.
type pinCredentials struct {
	next             http.RoundTripper
	origin           string // scheme://host + base path
	access, reseller string
}

func (p pinCredentials) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Del("X-Access-Token")
	req.Header.Del("X-Reseller-Token")
	host := req.URL.Scheme + "://" + req.URL.Host
	if p.origin != host && !strings.HasPrefix(p.origin, host+"/") {
		return p.next.RoundTrip(req)
	}
	req.Header.Set("X-Access-Token", p.access)
	if p.reseller != "" {
		req.Header.Set("X-Reseller-Token", p.reseller)
	}
	return p.next.RoundTrip(req)
}

// basePathOf returns the path part of a base URL by text, because the
// default base URL is a template (https://{host}/api) that url.Parse rejects.
func basePathOf(base string) string {
	if i := strings.Index(base, "://"); i >= 0 {
		base = base[i+3:]
	}
	j := strings.Index(base, "/")
	if j < 0 {
		return ""
	}
	return strings.TrimRight(base[j:], "/")
}

// effectiveAccessToken is the X-Access-Token value the client actually sends:
// a configured header overrides the credential fields.
func effectiveAccessToken(cfg *config.Config) string {
	for k, v := range cfg.Headers {
		if strings.EqualFold(k, "X-Access-Token") {
			return v
		}
	}
	return cfg.AuthHeader()
}

func installOrgRouting(c *client.Client) {
	if c == nil || c.HTTPClient == nil || c.Config == nil || orgTokenOverride != "" {
		return
	}
	if _, ok := c.HTTPClient.Transport.(orgRoutingTransport); ok {
		return
	}
	basePath := basePathOf(c.BaseURL)
	next := c.HTTPClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.HTTPClient.Transport = orgRoutingTransport{next: next, owner: c, adminToken: effectiveAccessToken(c.Config), basePath: basePath}
	// A routed response depends on the swapped organization token and the
	// reseller token, neither of which the response cache key includes.
	c.NoCache = true
}

// wrapOrgScopedErrors adds an --org hint when an organization-scoped command
// fails because it went out with the reseller token.
func wrapOrgScopedErrors(root *cobra.Command) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		top := topLevelName(cmd)
		if !orgScopedCommands[top] || cmd.RunE == nil || cmd.HasSubCommands() {
			return
		}
		run := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			err := run(c, args)
			if err == nil || orgTokenOverride != "" || os.Getenv(defaultOrgEnv) != "" {
				return err
			}
			if !strings.Contains(err.Error(), "account is not found") && !strings.Contains(err.Error(), "not authorized to perform this action") {
				return err
			}
			wrapped := fmt.Errorf("%w\nhint: this command reads one organization's data and needs that organization's token; add --org <organization id or owner email> (or set %s)", err, defaultOrgEnv)
			var ce *cliError
			if errors.As(err, &ce) {
				return &cliError{code: ce.code, err: wrapped}
			}
			return wrapped
		}
	}
	walk(root)
}

func init() {
	// installOrgRouting runs from the one client hook in ds_guard.go.
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		wrapOrgScopedErrors(root)
	})
}
