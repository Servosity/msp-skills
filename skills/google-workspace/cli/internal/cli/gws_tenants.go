// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: multi-client tenant registry.
//
// One Google Workspace client = one service account with domain-wide
// delegation, impersonating that client's super admin (GAM's model). The
// registry maps a short tenant name to {domain, admin, key file, scopes}.
// Selecting a tenant (--tenant, GOOGLE_WORKSPACE_TENANT, or the only
// registered tenant) mints a bearer token for every request and pins the
// local SQLite store to that tenant, so client data never mixes.
//
// Wiring is entirely hook-based (registerNovelCommand, registerClientHook,
// config.StoreScopeIdentity) so regeneration never touches this file.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"

	"google-workspace-pp-cli/internal/client"
	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/config"
)

const scopePrefix = "https://www.googleapis.com/auth/"

// #nosec G101 -- public Google OAuth2 token endpoint, not a credential
const googleTokenURL = "https://oauth2.googleapis.com/token"

// tenantScopeProfiles are the scope sets an MSP pastes into each client's
// Admin console > Security > API controls > Domain-wide delegation. Minting
// requests exactly the tenant's configured set: DWD rejects the whole token
// if any requested scope is not authorized, so least privilege is the default.
var tenantScopeProfiles = map[string][]string{
	"read": {
		"admin.directory.user.readonly",
		"admin.directory.group.readonly",
		"admin.directory.group.member.readonly",
		"admin.directory.orgunit.readonly",
		"admin.directory.domain.readonly",
		"admin.directory.customer.readonly",
		"admin.directory.device.chromeos.readonly",
		"admin.directory.device.mobile.readonly",
		"admin.directory.rolemanagement.readonly",
		"admin.directory.user.security", // tokens/asps listing has no readonly variant
		"admin.reports.audit.readonly",
		"admin.reports.usage.readonly",
		"apps.licensing",
		"admin.datatransfer.readonly", // leavers: did the Drive/Calendar transfer complete
	},
	// full adds what offboarding writes: suspend/sign-out/OU move, group
	// removal, Drive/Calendar ownership transfer.
	"full": {
		"admin.directory.user",
		"admin.directory.group.member",
		"admin.datatransfer",
	},
	// policy adds the Cloud Identity Policy API (current 2SV enforcement,
	// Drive external sharing, Gmail auto-forwarding) for 'posture'.
	"policy": {
		"cloud-identity.policies.readonly",
	},
	// reseller is for the MSP's own reseller domain, never a client tenant.
	"reseller": {
		"apps.order.readonly",
	},
}

// scopesForProfile expands a profile name (or a CSV of profile names and raw
// scopes) into full scope URLs. "full" includes "read".
func scopesForProfile(spec string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if !strings.HasPrefix(s, "https://") {
			s = scopePrefix + s
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, part := range splitTenantCSV(spec) {
		switch part {
		case "read", "reseller", "policy":
			for _, s := range tenantScopeProfiles[part] {
				add(s)
			}
		case "full":
			for _, s := range tenantScopeProfiles["read"] {
				add(s)
			}
			for _, s := range tenantScopeProfiles["full"] {
				add(s)
			}
		default:
			if !strings.Contains(part, ".") {
				return nil, fmt.Errorf("unknown scope profile %q (use read, full, policy, reseller, or a scope like admin.directory.user)", part)
			}
			add(part)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no scopes selected")
	}
	return out, nil
}

type gwsTenant struct {
	Name       string   `toml:"name" json:"name"`
	Domain     string   `toml:"domain" json:"domain"`
	Domains    []string `toml:"domains,omitempty" json:"domains,omitempty"`
	Admin      string   `toml:"admin" json:"admin"`
	KeyFile    string   `toml:"key_file" json:"key_file"`
	CustomerID string   `toml:"customer_id,omitempty" json:"customer_id,omitempty"`
	Scopes     []string `toml:"scopes" json:"scopes"`
	Reseller   bool     `toml:"reseller,omitempty" json:"reseller,omitempty"`
	// OwnershipMismatch is set by tenants test when the admin and key resolve
	// to an organization that does not own Domain; the tenant is refused
	// until it is re-registered or passes tenants test again.
	OwnershipMismatch string `toml:"ownership_mismatch,omitempty" json:"ownership_mismatch,omitempty"`
}

// customer returns the Directory customer key for this tenant.
func (t gwsTenant) customer() string {
	if t.CustomerID != "" {
		return t.CustomerID
	}
	return "my_customer"
}

// ownsDomain reports whether an email domain belongs to this tenant.
func (t gwsTenant) ownsDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	if strings.EqualFold(t.Domain, domain) {
		return true
	}
	for _, d := range t.Domains {
		if strings.EqualFold(d, domain) {
			return true
		}
	}
	return false
}

type tenantRegistry struct {
	Tenants []gwsTenant `toml:"tenants"`
}

var tenantNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

func tenantRegistryDir() (string, error) {
	return cliutil.ConfigDir()
}

func tenantRegistryPath() (string, error) {
	dir, err := tenantRegistryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tenants.toml"), nil
}

func loadTenantRegistry() (*tenantRegistry, error) {
	path, err := tenantRegistryPath()
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- fixed file under the CLI's own config dir
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &tenantRegistry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading tenant registry: %w", err)
	}
	var reg tenantRegistry
	if err := toml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing tenant registry %s: %w", path, err)
	}
	return &reg, nil
}

// updateTenantRegistry runs a read-modify-write of the registry under a
// cross-process file lock so concurrent tenants add/remove/test cannot
// overwrite each other's changes.
func updateTenantRegistry(fn func(reg *tenantRegistry) error) error {
	path, err := tenantRegistryPath()
	if err != nil {
		return err
	}
	// The lock file lives beside the registry, so the directory must exist
	// before the first tenant is ever added.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return cliutil.WithFileLock(path+".lock", func() error {
		reg, err := loadTenantRegistry()
		if err != nil {
			return err
		}
		if err := fn(reg); err != nil {
			return err
		}
		return saveTenantRegistry(reg)
	})
}

func saveTenantRegistry(reg *tenantRegistry) error {
	path, err := tenantRegistryPath()
	if err != nil {
		return err
	}
	sort.Slice(reg.Tenants, func(i, j int) bool { return reg.Tenants[i].Name < reg.Tenants[j].Name })
	data, err := toml.Marshal(reg)
	if err != nil {
		return err
	}
	return cliutil.AtomicWritePrivateFile(path, data, 0o600, 0o700)
}

func (r *tenantRegistry) find(name string) (gwsTenant, bool) {
	for _, t := range r.Tenants {
		if t.Name == name {
			return t, true
		}
	}
	return gwsTenant{}, false
}

// forEmail returns the tenant owning an email address's domain.
func (r *tenantRegistry) forEmail(email string) (gwsTenant, bool) {
	_, domain, ok := strings.Cut(email, "@")
	if !ok {
		return gwsTenant{}, false
	}
	for _, t := range r.Tenants {
		if t.ownsDomain(domain) {
			return t, true
		}
	}
	return gwsTenant{}, false
}

// clients returns every non-reseller tenant (the fan-out set).
func (r *tenantRegistry) clients() []gwsTenant {
	var out []gwsTenant
	for _, t := range r.Tenants {
		if !t.Reseller {
			out = append(out, t)
		}
	}
	return out
}

// --- tenant selection -------------------------------------------------------

var (
	tenantFlag  string
	tenantState struct {
		sync.Mutex
		resolved bool
		tenant   *gwsTenant
		err      error
	}
)

// RefreshTenantSelection re-reads the tenant registry on the next selection.
// A CLI command resolves its tenant once, so its store and its credential
// always belong to the same client. The MCP server is long-lived and calls
// this at the start of each tool call, so a tenant added from a terminal
// ends the single-tenant auto-selection it resolved earlier.
func RefreshTenantSelection() {
	mcpTenantMu.Lock()
	defer mcpTenantMu.Unlock()
	resetTenantSelection()
}

// mcpTenantMu serializes MCP calls' refresh-to-store-path sequence: the tenant
// selection and store scope are process-wide, and the HTTP transport serves
// calls concurrently.
var mcpTenantMu sync.Mutex

// tenantStoreIdentity keys a tenant's local store to its name and domain, so
// re-registering a name for a different client never reopens the previous
// client's mirror.
func tenantStoreIdentity(t *gwsTenant) string {
	return "tenant:" + t.Name + "/" + strings.ToLower(strings.TrimSpace(t.Domain))
}

// selectedTenant resolves --tenant > GOOGLE_WORKSPACE_TENANT > the only
// registered client tenant. Returns nil (no error) when no tenant applies, in
// which case the raw bearer token env var / auth set-token path is used.
func selectedTenant() (*gwsTenant, error) {
	tenantState.Lock()
	defer tenantState.Unlock()
	if tenantState.resolved {
		return tenantState.tenant, tenantState.err
	}
	tenantState.resolved, tenantState.tenant, tenantState.err = true, nil, nil
	name := strings.TrimSpace(tenantFlag)
	if name == "" {
		name = getenvTenant()
	}
	reg, err := loadTenantRegistry()
	if err != nil {
		tenantState.err = configErr(err)
		return nil, tenantState.err
	}
	if name == "" {
		if c := reg.clients(); len(c) == 1 && len(reg.Tenants) == 1 {
			tenantState.tenant = &c[0]
		}
		return tenantState.tenant, nil
	}
	t, ok := reg.find(name)
	if !ok {
		tenantState.err = usageErr(fmt.Errorf("unknown tenant %q (run 'google-workspace-cli tenants list')", name))
		return nil, tenantState.err
	}
	tenantState.tenant = &t
	return tenantState.tenant, nil
}

// confirmed reports an explicit --yes on the command line. A saved run
// profile can set flags.yes without the operator typing it, which must never
// authorize a destructive action.
func confirmed(cmd *cobra.Command, flags *rootFlags) bool {
	if !flags.yes {
		return false
	}
	f := cmd.Flags().Lookup("yes")
	if f == nil {
		f = cmd.Root().PersistentFlags().Lookup("yes")
	}
	return f != nil && f.Changed
}

// getenvTenant goes through EnvOverride so an MCPB host that leaves an unset
// optional field as the literal "${user_config.google_workspace_tenant}"
// reads as no tenant, not as a tenant with that name.
func getenvTenant() string {
	return strings.TrimSpace(cliutil.EnvOverride("GOOGLE_WORKSPACE_TENANT"))
}

// tenantPinnedStore reports whether the local store is keyed to a registered
// tenant (see config.StoreScopeIdentity below) rather than to a credential.
func tenantPinnedStore() bool {
	t, err := selectedTenant()
	return err == nil && t != nil
}

// localStoreTenantErr refuses a local-store read that would open the shared,
// unscoped data.db while tenants are registered and none is selected. No
// client's mirror lives there (a tenant never claims it), so an empty answer
// would read as a clean one. A bearer token keys the store to its own
// organization, the same one generated API commands then act on.
func localStoreTenantErr() error {
	t, err := selectedTenant()
	if err != nil {
		return err
	}
	hash, _, _ := currentDefaultDBScope()
	if t != nil {
		// Hand-fix (handfixes.json tenants-test-checks-domain): a quarantined
		// tenant's mirror may hold another organization's data.
		if t.OwnershipMismatch != "" {
			return configErr(fmt.Errorf("tenant %s is quarantined: %s", t.Name, t.OwnershipMismatch))
		}
		if hash != hashDefaultDBScope(tenantStoreIdentity(t)) {
			return configErr(fmt.Errorf("the local store for tenant %q could not be resolved; check the CLI config file", t.Name))
		}
		return nil
	}
	return noTenantSelectedErr()
}

// registeredTenantCount is the number of registered tenants (0 when the
// registry cannot be read).
func registeredTenantCount() int {
	reg, err := loadTenantRegistry()
	if err != nil {
		return 0
	}
	return len(reg.Tenants)
}

// noTenantSelectedErr refuses an API call or local read while client tenants
// are registered and none is selected, instead of falling back to an ambient
// GOOGLE_WORKSPACE_TOKEN (or a saved token): that token's organization is not
// a client the operator picked for this call. Hand-fix (handfixes.json
// no-ambient-token-with-tenants).
func noTenantSelectedErr() error {
	reg, err := loadTenantRegistry()
	if err != nil || len(reg.Tenants) == 0 {
		return nil
	}
	return usageErr(fmt.Errorf("%d tenants are registered and none is selected: pass --tenant <name> (MCP server: set GOOGLE_WORKSPACE_TENANT in its env); cross-client reports take --all-tenants. GOOGLE_WORKSPACE_TOKEN is not used once tenants are registered", len(reg.Tenants)))
}

// AuthErrorHint is authSetupHint for a 401 or 403 error, for the MCP execute
// tool, which reports raw client errors.
func AuthErrorHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "HTTP 401"):
		return "\nhint:" + strings.TrimPrefix(authSetupHint(), "\n     ")
	case strings.Contains(msg, "HTTP 403") && !googleQuotaExceeded(msg):
		lead := "\nhint: permission denied: the credential was accepted but lacks access (a missing scope, admin role, or API not enabled)."
		if reg, err := loadTenantRegistry(); err != nil || len(reg.Tenants) == 0 {
			return lead
		}
		return lead + authSetupHint()
	}
	return ""
}

// googleQuotaExceeded reports a Google 403 that is a rate or quota limit, not
// a permission problem.
func googleQuotaExceeded(msg string) bool {
	for _, reason := range []string{"rateLimitExceeded", "userRateLimitExceeded", "quotaExceeded", "dailyLimitExceeded", "RATE_LIMIT_EXCEEDED", "RESOURCE_EXHAUSTED"} {
		if strings.Contains(msg, reason) {
			return true
		}
	}
	return false
}

// authSetupHint is the credential line of the 400/401/403 hints. With client
// tenants registered the fix is a tenant or its delegation, not a bearer
// token (the docs say to unset that once tenants exist).
func authSetupHint() string {
	reg, err := loadTenantRegistry()
	if err != nil || len(reg.Tenants) == 0 {
		return "\n      Set one of: export GOOGLE_WORKSPACE_TOKEN=\"your-token-here\" or export ADMIN_ACCESS_TOKEN=\"your-token-here\""
	}
	t, err := selectedTenant()
	if err != nil || t == nil {
		return fmt.Sprintf("\n      %d tenants are registered and none is selected, so this call used no tenant credential: pass --tenant <name> (or set GOOGLE_WORKSPACE_TENANT); cross-client reports take --all-tenants.", len(reg.Tenants))
	}
	return fmt.Sprintf("\n      Tenant %s: check its domain-wide delegation with 'google-workspace-cli tenants test %s' and its scopes with 'google-workspace-cli tenants scopes %s' (writes need the full profile).", t.Name, t.Name, t.Name)
}

// resetTenantSelection is for tests.
func resetTenantSelection() {
	tenantState.Lock()
	tenantState.resolved, tenantState.tenant, tenantState.err = false, nil, nil
	tenantState.Unlock()
}

// --- token minting -----------------------------------------------------------

var tokenSources sync.Map // key: tenant name + scopes -> oauth2.TokenSource

// tenantTokenSource returns a cached, auto-refreshing token source that signs
// a JWT with the tenant's service-account key and impersonates its admin.
func tenantTokenSource(t gwsTenant, scopes []string) (oauth2.TokenSource, error) {
	keyPath := expandTenantPath(t.KeyFile)
	// #nosec G304 -- the operator-registered service-account key path for this tenant
	keyJSON, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, configErr(fmt.Errorf("tenant %s: reading service-account key: %w", t.Name, err))
	}
	// Keyed to the key's content too: a rotated key must not keep refreshing
	// through a token source built from the old one in a long-running server.
	keySum := sha256.Sum256(keyJSON)
	cacheKey := t.Name + "|" + t.Admin + "|" + strings.Join(scopes, " ") + "|" + hex.EncodeToString(keySum[:8])
	if ts, ok := tokenSources.Load(cacheKey); ok {
		return ts.(oauth2.TokenSource), nil
	}
	var key struct {
		ClientEmail  string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		PrivateKeyID string `json:"private_key_id"`
	}
	if err := json.Unmarshal(keyJSON, &key); err != nil || key.ClientEmail == "" || key.PrivateKey == "" {
		return nil, configErr(fmt.Errorf("tenant %s: %s is not a service-account key", t.Name, keyPath))
	}
	// Google's token endpoint is pinned: a key file's token_uri must not
	// redirect the signed assertion anywhere else.
	cfg := &jwt.Config{
		Email:        key.ClientEmail,
		PrivateKey:   []byte(key.PrivateKey),
		PrivateKeyID: key.PrivateKeyID,
		Scopes:       scopes,
		TokenURL:     googleTokenURL,
		Subject:      t.Admin,
	}
	// The token source outlives one request, so it gets its own bounded
	// HTTP client instead of a caller context: a stalled token endpoint
	// cannot hold a fan-out worker forever.
	tokCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 30 * time.Second})
	ts := oauth2.ReuseTokenSource(nil, cfg.TokenSource(tokCtx))
	tokenSources.Store(cacheKey, ts)
	return ts, nil
}

func mintTenantToken(ctx context.Context, t gwsTenant, scopes []string) (string, error) {
	if len(scopes) == 0 {
		scopes = t.Scopes
	}
	ts, err := tenantTokenSource(t, scopes)
	if err != nil {
		return "", err
	}
	tok, err := ts.Token()
	if err != nil {
		return "", authErr(fmt.Errorf("tenant %s: minting token as %s: %w%s", t.Name, t.Admin, err, dwdHint(err)))
	}
	return tok.AccessToken, nil
}

func dwdHint(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unauthorized_client"):
		return " (domain-wide delegation is missing a requested scope; run 'tenants scopes <tenant>' and paste them into Admin console > Security > API controls > Domain-wide delegation, then 'tenants test <tenant>')"
	case strings.Contains(msg, "invalid_grant"):
		return " (check the admin email exists, is a super admin, and the service-account key is not revoked)"
	}
	return ""
}

// hasScopes returns the short names of scopes in need that t lacks.
func hasScopes(t gwsTenant, need []string) []string {
	have := map[string]bool{}
	for _, s := range t.Scopes {
		have[s] = true
	}
	var missing []string
	for _, s := range need {
		if !have[s] {
			missing = append(missing, strings.TrimPrefix(s, scopePrefix))
		}
	}
	return missing
}

func expandTenantPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// applyTenantClient points a freshly built client at one tenant by minting
// its bearer token. Dry-run never mints (no network, no key read).
func applyTenantClient(ctx context.Context, c *client.Client, t gwsTenant) error {
	// Hand-fix (handfixes.json tenants-test-checks-domain): a tenant whose
	// credentials belong to another organization is never used.
	if t.OwnershipMismatch != "" {
		return configErr(fmt.Errorf("tenant %s is quarantined: %s", t.Name, t.OwnershipMismatch))
	}
	// The implicit "default" tenant (no key file) keeps the configured bearer.
	if c == nil || c.Config == nil || c.DryRun || t.KeyFile == "" {
		return nil
	}
	tok, err := mintTenantToken(ctx, t, nil)
	if err != nil {
		return err
	}
	bindTenantToken(c.Config, t, tok)
	// Long runs (tail, a large sync) outlive the hour-long token; the cached
	// token source re-mints once it expires.
	if ts, err := tenantTokenSource(t, t.Scopes); err == nil {
		c.Config.AuthHeaderSource = refreshingAuthHeader(ts)
	}
	return nil
}

// refreshingAuthHeader returns the current bearer from an auto-refreshing
// token source, or "" (the bound token then applies) if it cannot refresh.
func refreshingAuthHeader(ts oauth2.TokenSource) func() string {
	return func() string {
		tok, err := ts.Token()
		if err != nil || tok.AccessToken == "" {
			return ""
		}
		return "Bearer " + tok.AccessToken
	}
}

// bindTenantToken makes the tenant's minted bearer authoritative. The
// generated AuthHeader prefers AuthHeaderVal, then GOOGLE_WORKSPACE_TOKEN,
// then AccessToken, so a stray env token would otherwise authenticate a
// --tenant B request as whoever owns that token.
func bindTenantToken(cfg *config.Config, t gwsTenant, tok string) {
	// Configured [headers] are applied after auth, so a static Authorization
	// entry would also win; drop it for tenant-bound clients.
	for k := range cfg.Headers {
		if strings.EqualFold(k, "Authorization") {
			delete(cfg.Headers, k)
		}
	}
	cfg.AuthHeaderVal = "Bearer " + tok
	cfg.GoogleWorkspaceToken = ""
	cfg.AccessToken = tok
	cfg.AuthSource = "tenant:" + t.Name
}

// fanoutBound is set once a command builds explicitly tenant-bound clients;
// the selected-tenant hook then stays out of the way, so a broken
// GOOGLE_WORKSPACE_TENANT cannot fail an unrelated --tenants b,c sweep.
var fanoutBound atomic.Bool

// newTenantClient builds a client bound to a specific tenant (fan-out path).
func (f *rootFlags) newTenantClient(ctx context.Context, t gwsTenant) (*client.Client, error) {
	fanoutBound.Store(true)
	c, err := f.newClient()
	if err != nil {
		return nil, err
	}
	if err := applyTenantClient(ctx, c, t); err != nil {
		return nil, err
	}
	return c, nil
}

func init() {
	// Every client (CLI and MCP) for the selected tenant authenticates as
	// that tenant. A fan-out command overrides per tenant afterwards.
	registerClientHook(func(c *client.Client) error {
		if fanoutBound.Load() {
			return nil
		}
		t, err := selectedTenant()
		if err != nil {
			return err
		}
		if t == nil {
			return noTenantSelectedErr()
		}
		return applyTenantClient(context.Background(), c, *t)
	})
	// doctor only knows generated credentials; report registered tenants
	// too so a tenant-only setup is not shown as "auth: not configured".
	doctorAuthConfiguredHook = func() (bool, string) {
		reg, err := loadTenantRegistry()
		if err != nil || len(reg.Tenants) == 0 {
			return false, ""
		}
		return true, fmt.Sprintf("tenant registry: %d tenant(s); verify delegation with 'tenants test --all-tenants'", len(reg.Tenants))
	}
	// Pin the local store to the tenant name, not the hourly-rotating token,
	// so each client keeps one stable database.
	config.StoreScopeIdentity = func(*config.Config) string {
		if t, err := selectedTenant(); err == nil && t != nil {
			return tenantStoreIdentity(t)
		}
		return ""
	}
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if root.PersistentFlags().Lookup("tenant") == nil {
			root.PersistentFlags().StringVar(&tenantFlag, "tenant", "", "Client tenant to act as (see 'tenants list'; env: GOOGLE_WORKSPACE_TENANT)")
		}
		root.AddCommand(newTenantsCmd(flags))
		// The tenant (and its store scope) is resolved before run profiles
		// are applied, and a saved profile silently choosing which client to
		// act on is unsafe anyway: refuse profiles that carry a tenant.
		if inner := root.PersistentPreRunE; inner != nil {
			root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
				if err := inner(cmd, args); err != nil {
					return err
				}
				// Checked after inner so --home is honored; still before RunE,
				// so nothing has authenticated or opened the tenant store.
				if flags.runProfileName != "" {
					if p, err := GetProfile(flags.runProfileName); err == nil && p != nil {
						if _, ok := p.Values["tenant"]; ok {
							return usageErr(fmt.Errorf("run profile %q sets tenant; tenants are chosen per invocation with --tenant or GOOGLE_WORKSPACE_TENANT", flags.runProfileName))
						}
					}
				}
				return nil
			}
		}
	})
}

func splitTenantCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
