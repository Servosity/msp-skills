// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written: Sophos Central request routing.
//
// Sophos Central splits its API across two kinds of host and three ID headers:
//   - /whoami/v1, /partner/v1, /organization/v1 live on the global host
//     (api.central.sophos.com). Partner calls need X-Partner-ID, organization
//     calls need X-Organization-ID; both IDs come from GET /whoami/v1.
//   - Every other service (endpoint, common, live-discover, xdr-query,
//     account-health-check, siem) lives on the tenant's regional apiHost
//     (https://api-<region>.central.sophos.com) and needs X-Tenant-ID.
//
// The generated client knows none of this, so this file installs a client hook
// that wraps the HTTP transport. The transport reads the request path, resolves
// the caller identity once (cached on disk), maps the tenant to its apiHost, and
// rewrites host and headers. No generated file is edited.
//
// Cache safety: the generated response cache keys on BaseURL, the auth token
// hash, and tenant-selecting headers (cfg.Headers plus per-call overrides). The
// active tenant therefore always travels as an X-Tenant-ID header (from
// --tenant / SOPHOS_CENTRAL_TENANT_ID via cfg.Headers, or a per-call override in
// the fleet commands), never as hidden transport state.

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/config"
	"sophos-central-pp-cli/internal/store"
)

const (
	sophosGlobalHost     = "api.central.sophos.com"
	sophosHostSuffix     = ".central.sophos.com"
	sophosTenantEnv      = "SOPHOS_CENTRAL_TENANT_ID"
	sophosContextTTL     = 24 * time.Hour
	headerTenantID       = "X-Tenant-ID"
	headerPartnerID      = "X-Partner-ID"
	headerOrganizationID = "X-Organization-ID"
)

// sophosTenantFlag backs the root --tenant flag.
var sophosTenantFlag string

// tenantFlagValue rejects a --tenant that names no tenant ID ("", " ", ","):
// an explicit selector must never fall back to "all tenants".
type tenantFlagValue struct{ p *string }

func (v tenantFlagValue) String() string { return *v.p }
func (v tenantFlagValue) Type() string   { return "string" }
func (v tenantFlagValue) Set(s string) error {
	for _, id := range strings.Split(s, ",") {
		if strings.TrimSpace(id) != "" {
			*v.p = s
			return nil
		}
	}
	return fmt.Errorf("%q names no tenant ID", s)
}

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		if root.PersistentFlags().Lookup("tenant") == nil {
			// Clear the target so one invocation's tenant never carries into
			// the next root (StringVar also resets it; kept explicit).
			sophosTenantFlag = ""
			resetSophosClientScope()
			// Declared with StringVar so static flag scanners (verify-skill)
			// see --tenant, then the value is swapped for tenantFlagValue so
			// an empty selector is still rejected at parse time.
			root.PersistentFlags().StringVar(&sophosTenantFlag, "tenant", "",
				"Sophos tenant ID for tenant-scoped calls (endpoints, alerts, policies, ...). Required with a partner or organization credential (env: "+sophosTenantEnv+")")
			root.PersistentFlags().Lookup("tenant").Value = tenantFlagValue{&sophosTenantFlag}
		}
	})
	registerClientHook(installSophosRouting)

	// Generic `sync --full` reconciles flat resources. Tenant-scoped rows (for
	// example endpoints and alerts from fleet refresh) belong to many tenants,
	// so prune them per tenant (their tenant.id field) instead of across the
	// whole table. With no known tenant the generated sync skips the prune.
	// The same applies to every other tenant-scoped resource: a row that
	// carries no tenant.id is never in a partition, so it is never pruned.
	// Only partner/organization resources (one caller, one partition) keep
	// whole-table reconcile.
	for rt, mode := range flatReconcileModes {
		if mode != "flat_global" {
			continue
		}
		if path, err := syncResourcePath(rt); err == nil && (strings.HasPrefix(path, "/partner/") || strings.HasPrefix(path, "/organization/")) {
			continue
		}
		flatReconcileModes[rt] = "flat"
		flatReconcileDefs[rt] = flatReconcileDefT{BodyField: "tenant.id"}
	}
	resolveTenantID = sophosActiveTenant

	// Pin the default local store to the credential's client ID. The
	// generated default keys the store on the access token until one exists,
	// so the first `fleet refresh` (no token yet) and every later command
	// (token saved) would use different database files.
	config.StoreScopeIdentity = sophosStoreScope
}

// sophosStoreScope keys the default store on base URL + client ID, stable
// across token mints and secret rotation. "" keeps the generated behavior.
func sophosStoreScope(c *config.Config) string {
	// The credential requests actually carry (configured Authorization header,
	// static auth header, a full client-credential pair, else the saved
	// token). A full pair keeps the store stable once a token is saved; a
	// client ID without a secret cannot mint, so the token decides. The
	// generated code hashes the key into the path, never using it verbatim.
	key := sophosCredentialKey(c)
	if key == "" {
		return ""
	}
	base := "sophos-store|" + strings.TrimRight(c.BaseURL, "/") + "|"
	if id, ok := strings.CutPrefix(key, "client|"); ok {
		return base + id
	}
	return base + key
}

// sophosActiveTenant is the tenant generic sync is working on: the selected
// --tenant (single ID) or, for a tenant credential, its own tenant once
// whoami has run. "" means unknown.
// sophosSyncStateKey scopes a tenant-scoped resource's sync checkpoint and
// incremental watermark to the selected tenant: one store holds many
// tenants, and one tenant's resume key or since-watermark must never drive
// another tenant's sync. Partner/organization resources keep the plain key.
func sophosSyncStateKey(resource string) string {
	path, err := syncResourcePath(resource)
	if err != nil || !sophosTenantScopedPath(path) {
		return resource
	}
	if t := sophosExplicitTenant(); t != "" {
		return resource + "@" + t
	}
	// No selector: the request tenant is the credential's own (a partner
	// credential cannot reach tenant data without one), so key by credential.
	// The identity is not known before the first request, so it is not used.
	clientScopeMu.Lock()
	defer clientScopeMu.Unlock()
	if activeCredScope != "" {
		return resource + "@cred-" + activeCredScope
	}
	return resource
}

// sophosScopeParentRows drops dependent-sync parents stored for another
// tenant, so a --tenant T2 sync never requests T1's user IDs under T2. Rows
// that do not record a tenant are kept (the API rejects them loudly).
func sophosScopeParentRows(db *store.Store, dep dependentResourceDef, rows []map[string]string) ([]map[string]string, error) {
	if db == nil || !sophosTenantScopedPath(dep.PathTemplate) {
		return rows, nil
	}
	t := sophosActiveTenant() // selector, configured header, or the credential's resolved tenant
	kept := make([]map[string]string, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		tid := ""
		if raw, err := db.Get(dep.ParentTable, r["id"]); err == nil {
			tid = rowTenantID(raw)
		}
		if tid != "" {
			seen[tid] = true
		}
		if t != "" && tid != "" && tid != t {
			continue
		}
		kept = append(kept, r)
	}
	// Unknown tenant over parents from several tenants: requests would send
	// one tenant's IDs under another's header, so refuse.
	if t == "" && len(seen) > 1 {
		return nil, usageErr(fmt.Errorf("%s holds %s rows for %d tenants; pass --tenant <tenant-id> to sync %s for one of them", dep.ParentTable, dep.ParentTable, len(seen), dep.Name))
	}
	return kept, nil
}

// The tenant and credential the current invocation's client sends, recorded
// when the client is built (requests honor a configured X-Tenant-ID header
// even without --tenant). Reset per root so nothing leaks across invocations.
var (
	clientScopeMu      sync.Mutex
	activeHeaderTenant string
	activeCredScope    string
)

func recordSophosClientScope(cfg *config.Config) {
	tenant := ""
	for k, v := range cfg.Headers {
		if strings.EqualFold(strings.TrimSpace(k), headerTenantID) {
			tenant = strings.TrimSpace(v)
		}
	}
	cred := ""
	if key := sophosCredentialKey(cfg); key != "" {
		sum := sha256.Sum256([]byte("sophos-scope|" + key))
		cred = hex.EncodeToString(sum[:6])
	}
	clientScopeMu.Lock()
	activeHeaderTenant, activeCredScope = tenant, cred
	clientScopeMu.Unlock()
}

func resetSophosClientScope() {
	clientScopeMu.Lock()
	activeHeaderTenant, activeCredScope = "", ""
	clientScopeMu.Unlock()
	activeRouterMu.Lock()
	activeRouter = nil
	activeRouterMu.Unlock()
}

// sophosExplicitTenant is the single tenant this invocation selects: --tenant
// or the env var, else a configured X-Tenant-ID header. A list selects none.
func sophosExplicitTenant() string {
	if t := configuredSophosTenant(); t != "" {
		if strings.Contains(t, ",") {
			return ""
		}
		return t
	}
	clientScopeMu.Lock()
	defer clientScopeMu.Unlock()
	if strings.Contains(activeHeaderTenant, ",") {
		return ""
	}
	return activeHeaderTenant
}

func sophosActiveTenant() string {
	if t := sophosExplicitTenant(); t != "" {
		return t
	}
	activeRouterMu.Lock()
	r := activeRouter
	activeRouterMu.Unlock()
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastIdentity != nil && r.lastIdentity.IDType == "tenant" {
		return r.lastIdentity.ID
	}
	return ""
}

// sophosIdentity is the cached GET /whoami/v1 answer.
type sophosIdentity struct {
	ID       string `json:"id"`
	IDType   string `json:"idType"`
	APIHosts struct {
		Global     string `json:"global"`
		DataRegion string `json:"dataRegion"`
	} `json:"apiHosts"`
}

// identityEntry is one cached whoami answer.
type identityEntry struct {
	Identity  sophosIdentity `json:"identity"`
	FetchedAt time.Time      `json:"fetched_at"`
}

// sophosContextFile is the on-disk cache. Identities are keyed by a hash of
// the exact Authorization header they were fetched with, so an identity is
// only ever reused for the credential that produced it. Tenant -> apiHost
// entries are facts about tenants, not about the caller.
type sophosContextFile struct {
	Identities  map[string]identityEntry `json:"identities,omitempty"`
	TenantHosts map[string]string        `json:"tenant_hosts,omitempty"`
}

type sophosRouter struct {
	c         *client.Client
	base      http.RoundTripper
	cachePath string

	mu           sync.Mutex
	identities   map[string]identityEntry // auth fingerprint -> identity
	lastIdentity *sophosIdentity
	tenantHosts  map[string]string
}

// authOverride passes an Authorization value as a per-call header override
// (nil when empty, so the client applies its own credential).
func authOverride(auth string) map[string]string {
	if strings.TrimSpace(auth) == "" {
		return nil
	}
	return map[string]string{"Authorization": auth}
}

// authFingerprint is a one-way key for an Authorization header value.
func authFingerprint(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("sophos-auth|" + header))
	return hex.EncodeToString(sum[:12])
}

// activeSophosRouter is the router of the most recently built client. Fleet
// commands use it to seed tenant hosts they learned from the tenant list.
var (
	activeRouterMu sync.Mutex
	activeRouter   *sophosRouter
)

func configuredSophosTenant() string {
	if v := strings.TrimSpace(sophosTenantFlag); v != "" {
		return v
	}
	return strings.TrimSpace(cliutil.EnvOverride(sophosTenantEnv))
}

func installSophosRouting(c *client.Client) error {
	if c == nil || c.Config == nil || c.HTTPClient == nil {
		return nil
	}
	// The environment's client ID is the credential in use; material saved
	// for another client ID is never sent (handfixes.json:
	// env-credential-wins-over-saved).
	c.Config.AdoptEnvClientCredential()
	// The response cache keys on the configured auth, not on a per-config
	// Authorization header; with one present, caching could serve another
	// credential's response, so disable it.
	for k := range c.Config.Headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			c.NoCache = true
		}
	}
	if err := normalizeSophosHeaders(c.Config); err != nil {
		return err
	}
	pinClientPairAuth(c.Config)
	tenant := configuredSophosTenant()
	if tenant != "" {
		if c.Config.Headers == nil {
			c.Config.Headers = map[string]string{}
		}
		// Drop any configured tenant header under another spelling
		// (x-tenant-id): two keys would race in the outgoing request, and
		// the request could target a different tenant than the one selected.
		for k := range c.Config.Headers {
			if strings.EqualFold(strings.TrimSpace(k), headerTenantID) {
				delete(c.Config.Headers, k)
			}
		}
		c.Config.Headers[headerTenantID] = tenant
	}
	recordSophosClientScope(c.Config)
	// A BaseURL pointing anywhere but Sophos (verify/dogfood mock servers,
	// a local proxy) gets the tenant header and nothing else: there is no
	// whoami or regional host to resolve there.
	if !isSophosAPIHost(hostOf(c.BaseURL)) {
		return nil
	}
	base := c.HTTPClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if _, already := base.(*sophosRouter); already {
		return nil
	}
	r := &sophosRouter{c: c, base: base, cachePath: sophosContextPath(c), identities: map[string]identityEntry{}, tenantHosts: map[string]string{}}
	r.loadDisk()
	c.HTTPClient.Transport = r
	activeRouterMu.Lock()
	activeRouter = r
	activeRouterMu.Unlock()
	return nil
}

// normalizeSophosHeaders folds case/space variants of the tenant and
// Authorization headers into one canonical key. Variants with different
// values are refused: map order would decide which one a request sends, and
// tenant scoping (sync reconcile, local reads) could pick the other.
func normalizeSophosHeaders(cfg *config.Config) error {
	for _, name := range []string{headerTenantID, "Authorization"} {
		value, n := "", 0
		for k, v := range cfg.Headers {
			if !strings.EqualFold(strings.TrimSpace(k), name) {
				continue
			}
			v = strings.TrimSpace(v)
			if n > 0 && v != value {
				return usageErr(fmt.Errorf("configured headers set %s more than once with different values; keep one", name))
			}
			value, n = v, n+1
			delete(cfg.Headers, k)
		}
		if n > 0 {
			cfg.Headers[name] = value
		}
	}
	return nil
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// isSophosAPIHost guards every rewrite: the bearer token is only ever sent to
// a *.central.sophos.com host, whatever a cached or API-returned value says.
func isSophosAPIHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == sophosGlobalHost || (strings.HasPrefix(host, "api-") && strings.HasSuffix(host, sophosHostSuffix))
}

// sophosContextPath names the on-disk identity cache for this credential.
// It is keyed by client ID, or for token-only setups (auth set-token) by a
// hash of the token, so two credentials never share an identity or tenant
// map. With nothing to key on, identity is not cached on disk at all.
func sophosContextPath(c *client.Client) string {
	dir, err := cliutil.CacheDir()
	if err != nil {
		return ""
	}
	key := sophosCredentialKey(c.Config)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("sophos-context|" + key))
	return filepath.Join(dir, "sophos-context-"+hex.EncodeToString(sum[:6])+".json")
}

// sophosCredentialKey identifies the credential requests will actually carry,
// in the generated client's precedence: an explicit auth header wins; then a
// token minted from client ID + secret (only when both are set); then a
// stored token (auth set-token). Empty when there is no credential.
// sophosClientPair resolves the pair the client mints with: the
// environment's client ID when it differs from the saved one (the hook has
// already adopted it), else saved config first, then the environment. The provider-specific central_*
// fields are not used to mint, so they are not a credential here.
func sophosClientPair(cfg *config.Config) (string, string) {
	id, secret := cfg.ClientID, cfg.ClientSecret
	// The environment's client ID wins over a different saved one, and the
	// saved secret then belongs to another credential (env-credential-wins-over-saved).
	if envID := strings.TrimSpace(cliutil.EnvOverride("SOPHOS_CENTRAL_CLIENT_ID")); envID != "" && envID != id {
		id, secret = envID, ""
	}
	if id == "" {
		id = cliutil.EnvOverride("SOPHOS_CENTRAL_CLIENT_ID")
	}
	if secret == "" {
		secret = cliutil.EnvOverride("SOPHOS_CENTRAL_CLIENT_SECRET")
	}
	return id, secret
}

// pinClientPairAuth makes a configured client pair the credential every
// request carries: a static header would win until the next mint clears it,
// and a token saved without a client ID (auth set-token) may belong to
// another account. Both would make the store and checkpoint scope disagree
// with what is sent.
func pinClientPairAuth(cfg *config.Config) {
	if id, secret := sophosClientPair(cfg); id == "" || secret == "" {
		return
	}
	cfg.AuthHeaderVal = ""
	if cfg.ClientID == "" && cfg.AccessToken != "" {
		cfg.AccessToken, cfg.TokenExpiry = "", time.Time{}
	}
}

func sophosCredentialKey(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	// A configured Authorization header is applied after the normal auth,
	// so it is the credential requests actually carry.
	for k, v := range cfg.Headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") && strings.TrimSpace(v) != "" {
			return "header|" + strings.TrimSpace(v)
		}
	}
	// A full client pair mints its own token (installSophosRouting drops a
	// stale static header or a token not minted from a saved pair, so this
	// holds for every request), then a static header, then a saved token.
	if id, secret := sophosClientPair(cfg); id != "" && secret != "" {
		return "client|" + id
	}
	if cfg.AuthHeaderVal != "" {
		return "header|" + cfg.AuthHeaderVal
	}
	if cfg.AccessToken != "" {
		return "token|" + cfg.AccessToken
	}
	return ""
}

func (r *sophosRouter) loadDisk() {
	if r.cachePath == "" {
		return
	}
	data, err := os.ReadFile(r.cachePath)
	if err != nil {
		return
	}
	var f sophosContextFile
	if json.Unmarshal(data, &f) != nil {
		return
	}
	for fp, e := range f.Identities {
		if fp != "" && e.Identity.ID != "" && time.Since(e.FetchedAt) < sophosContextTTL {
			r.identities[fp] = e
		}
	}
	for k, v := range f.TenantHosts {
		if isSophosAPIHost(v) {
			r.tenantHosts[k] = v
		}
	}
}

// saveDiskLocked persists identity and tenant hosts. Caller holds r.mu.
func (r *sophosRouter) saveDiskLocked() {
	if r.cachePath == "" {
		return
	}
	for fp, e := range r.identities {
		if time.Since(e.FetchedAt) >= sophosContextTTL {
			delete(r.identities, fp)
		}
	}
	f := sophosContextFile{Identities: r.identities, TenantHosts: r.tenantHosts}
	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(r.cachePath), 0o700)
	tmp := r.cachePath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, r.cachePath)
	}
}

// rememberTenantHost records a tenant's regional host learned from a tenant list.
func (r *sophosRouter) rememberTenantHost(tenantID, apiHost string) {
	host := hostOf(apiHost)
	if tenantID == "" || !isSophosAPIHost(host) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tenantHosts[tenantID] == host {
		return
	}
	r.tenantHosts[tenantID] = host
	r.saveDiskLocked()
}

// rememberSophosTenantHost seeds the active router (no-op without one).
func rememberSophosTenantHost(tenantID, apiHost string) {
	activeRouterMu.Lock()
	r := activeRouter
	activeRouterMu.Unlock()
	if r != nil {
		r.rememberTenantHost(tenantID, apiHost)
	}
}

// rememberIdentity records a whoami answer for the credential (auth
// fingerprint) it was fetched with.
func (r *sophosRouter) rememberIdentity(fp string, id sophosIdentity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idCopy := id
	r.lastIdentity = &idCopy
	if id.IDType == "tenant" && isSophosAPIHost(hostOf(id.APIHosts.DataRegion)) {
		r.tenantHosts[id.ID] = hostOf(id.APIHosts.DataRegion)
	}
	if fp == "" {
		return
	}
	r.identities[fp] = identityEntry{Identity: id, FetchedAt: time.Now()}
	r.saveDiskLocked()
}

// whoami returns the identity behind the credential with fingerprint fp,
// fetching it when this credential has not been seen.
func (r *sophosRouter) whoami(ctx context.Context, auth string) (*sophosIdentity, error) {
	fp := authFingerprint(auth)
	if fp != "" {
		r.mu.Lock()
		e, ok := r.identities[fp]
		if ok {
			idCopy := e.Identity // a cache hit is still this run's identity
			r.lastIdentity = &idCopy
		}
		r.mu.Unlock()
		if ok {
			id := e.Identity
			return &id, nil
		}
	}
	// The nested call goes back through this transport as a /whoami path,
	// which is routed to the global host without needing an identity.
	// Forward the outer request's Authorization so the identity answers for
	// the credential that request will actually carry.
	raw, err := r.c.GetWithHeadersNoCache(ctx, "/whoami/v1", nil, authOverride(auth))
	if err != nil {
		return nil, upstreamErr("resolving Sophos caller identity (GET /whoami/v1)", err)
	}
	var id sophosIdentity
	if err := json.Unmarshal(raw, &id); err != nil || id.ID == "" || id.IDType == "" {
		return nil, &routingError{status: http.StatusBadGateway, code: "upstreamLookupFailed", msg: "resolving Sophos caller identity: /whoami/v1 returned no id/idType"}
	}
	r.rememberIdentity(fp, id)
	return &id, nil
}

// tenantHost returns the regional host for a tenant, looking it up through the
// partner or organization API on a cache miss.
func (r *sophosRouter) tenantHost(ctx context.Context, tenantID, auth string) (string, error) {
	r.mu.Lock()
	host, ok := r.tenantHosts[tenantID]
	r.mu.Unlock()
	if ok {
		return host, nil
	}
	id, err := r.whoami(ctx, auth)
	if err != nil {
		return "", err
	}
	var lookup string
	switch id.IDType {
	case "tenant":
		if id.ID != tenantID {
			return "", routingErr("wrongTenant", "this login is a tenant account for %s; it cannot act on tenant %s", id.ID, tenantID)
		}
		host = hostOf(id.APIHosts.DataRegion)
	case "partner":
		lookup = "/partner/v1/tenants/" + url.PathEscape(tenantID)
	case "organization":
		lookup = "/organization/v1/tenants/" + url.PathEscape(tenantID)
	default:
		return "", routingErr("unsupportedCallerType", "unsupported Sophos account type %q", id.IDType)
	}
	if lookup != "" {
		raw, err := r.c.GetWithHeadersNoCache(ctx, lookup, nil, authOverride(auth))
		if err != nil {
			return "", upstreamErr("looking up apiHost for tenant "+tenantID, err)
		}
		var t struct {
			APIHost string `json:"apiHost"`
		}
		if err := json.Unmarshal(raw, &t); err != nil {
			return "", &routingError{status: http.StatusBadGateway, code: "upstreamLookupFailed", msg: "looking up apiHost for tenant " + tenantID + ": " + err.Error()}
		}
		host = hostOf(t.APIHost)
	}
	if !isSophosAPIHost(host) {
		return "", routingErr("noApiHost", "tenant %s has no usable Sophos apiHost (got %q)", tenantID, host)
	}
	r.rememberTenantHost(tenantID, "https://"+host)
	return host, nil
}

// defaultTenant answers "which tenant?" when the request carries no X-Tenant-ID.
func (r *sophosRouter) defaultTenant(ctx context.Context, auth string) (string, error) {
	id, err := r.whoami(ctx, auth)
	if err != nil {
		return "", err
	}
	if id.IDType == "tenant" {
		return id.ID, nil
	}
	return "", routingErr("tenantRequired", "this %s account needs a tenant for tenant-scoped calls: pass --tenant <tenant-id> or set %s (list tenants with `sophos-central-cli tenants list` or `org-tenants list`)", id.IDType, sophosTenantEnv)
}

// routingError is a request the CLI refused or could not route. Messages
// avoid words like "credential", "key" and "token": the generated error
// classifier reads those as auth failures (exit 4) and would tell a user who
// only forgot --tenant to check their credentials. RoundTrip turns
// it into a synthetic HTTP response instead of a Go error: the generated
// client retries transport errors with backoff, and "pass --tenant" is not
// something a retry can fix.
type routingError struct {
	status int
	code   string
	msg    string
}

func (e *routingError) Error() string { return e.msg }

func routingErr(code, format string, args ...any) error {
	return &routingError{status: http.StatusBadRequest, code: code, msg: fmt.Sprintf(format, args...)}
}

// upstreamErr keeps the HTTP status of a failed nested whoami/tenant lookup
// (401 bad credentials, 403 tenant not managed) so exit codes stay truthful.
func upstreamErr(what string, err error) error {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 {
		return &routingError{status: apiErr.StatusCode, code: "upstreamLookupFailed", msg: what + ": " + apiErr.Error()}
	}
	return &routingError{status: http.StatusBadGateway, code: "upstreamLookupFailed", msg: what + ": " + err.Error()}
}

func (r *sophosRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || !isSophosAPIHost(req.URL.Hostname()) {
		return r.base.RoundTrip(req)
	}
	out, err := r.route(req)
	if err != nil {
		var re *routingError
		if !errors.As(err, &re) {
			return nil, err
		}
		body, _ := json.Marshal(map[string]string{"error": re.code, "message": re.msg})
		return &http.Response{
			StatusCode: re.status,
			Status:     fmt.Sprintf("%d %s", re.status, http.StatusText(re.status)),
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    req,
		}, nil
	}
	resp, err := r.base.RoundTrip(out)
	if err == nil && resp.StatusCode == http.StatusOK && strings.HasPrefix(out.URL.Path, "/whoami/") {
		r.captureWhoami(resp, authFingerprint(out.Header.Get("Authorization")))
	}
	return resp, err
}

// captureWhoami records a whoami answer seen on the wire, for the credential
// that sent it, and hands the caller an intact body.
func (r *sophosRouter) captureWhoami(resp *http.Response, fp string) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return
	}
	var id sophosIdentity
	if json.Unmarshal(body, &id) == nil && id.ID != "" && id.IDType != "" {
		r.rememberIdentity(fp, id)
	}
}

// route returns a clone of req with the Sophos host and ID headers applied.
func (r *sophosRouter) route(req *http.Request) (*http.Request, error) {
	ctx := req.Context()
	out := req.Clone(ctx)
	auth := req.Header.Get("Authorization")
	service := strings.SplitN(strings.TrimPrefix(out.URL.Path, "/"), "/", 2)[0]
	clearIDs := func() {
		out.Header.Del(headerTenantID)
		out.Header.Del(headerPartnerID)
		out.Header.Del(headerOrganizationID)
	}
	switch service {
	case "", "whoami":
		// "/" is the doctor reachability probe: global host, no identity needed.
		r.setHost(out, sophosGlobalHost)
		clearIDs()
	case "partner", "organization":
		id, err := r.whoami(ctx, auth)
		if err != nil {
			return nil, err
		}
		if id.IDType != service {
			return nil, routingErr("wrongCallerType", "/%s/v1 calls need a %s account; this login is a %s account", service, service, id.IDType)
		}
		r.setHost(out, globalHostFor(id))
		clearIDs()
		if service == "partner" {
			out.Header.Set(headerPartnerID, id.ID)
		} else {
			out.Header.Set(headerOrganizationID, id.ID)
		}
	case "licenses", "audit":
		// Global-host tenant APIs: the tenant ID when one is selected,
		// otherwise the caller's own ID header.
		tenantID := strings.TrimSpace(out.Header.Get(headerTenantID))
		id, err := r.whoami(ctx, auth)
		if err != nil {
			return nil, err
		}
		r.setHost(out, globalHostFor(id))
		clearIDs()
		switch {
		case tenantID != "":
			out.Header.Set(headerTenantID, tenantID)
		case id.IDType == "tenant":
			out.Header.Set(headerTenantID, id.ID)
		case id.IDType == "partner":
			out.Header.Set(headerPartnerID, id.ID)
		case id.IDType == "organization":
			out.Header.Set(headerOrganizationID, id.ID)
		}
	default:
		tenantID := strings.TrimSpace(out.Header.Get(headerTenantID))
		if tenantID == "" {
			t, err := r.defaultTenant(ctx, auth)
			if err != nil {
				return nil, err
			}
			tenantID = t
		}
		host, err := r.tenantHost(ctx, tenantID, auth)
		if err != nil {
			return nil, err
		}
		r.setHost(out, host)
		clearIDs()
		out.Header.Set(headerTenantID, tenantID)
	}
	return out, nil
}

func globalHostFor(id *sophosIdentity) string {
	if h := hostOf(id.APIHosts.Global); isSophosAPIHost(h) {
		return h
	}
	return sophosGlobalHost
}

func (r *sophosRouter) setHost(req *http.Request, host string) {
	req.URL.Scheme = "https"
	req.URL.Host = host
	req.Host = host
}
