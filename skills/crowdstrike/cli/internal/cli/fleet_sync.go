// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature logic (Printing Press transcendence): cross-tenant
// Flight Control sync. Mints a member_cid-scoped OAuth2 token per child CID and
// pulls hosts/alerts/vulns/policies into the CID-keyed local store.

package cli

import (
	"context"
	"crowdstrike-pp-cli/internal/client"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// fleetSyncMaxPages caps offset pagination per kind/CID so a misconfigured
// filter can't loop forever. fleetSyncPageLimit is the per-page fetch size.
const (
	fleetSyncMaxPages   = 10
	fleetSyncPageLimit  = 500
	defaultVulnFilter   = "status:'open'"
	defaultAlertFilter  = "" // empty = all alerts the client can see
	defaultDeviceFilter = ""
)

// pp:data-source live
func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var allCids bool
	var cidsCSV string
	var memberCID string
	var kindsCSV string
	var dbPath string
	var vulnFilter string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull hosts, alerts, vulnerabilities, policies, and the Flight Control fabric from every child tenant into the local store",
		Long: strings.Trim(`
Sync the MSP fleet into the CID-keyed local store. For each Flight Control child
CID, fleet sync mints a member_cid-scoped OAuth2 token from your parent API
client and pulls hosts, alerts, Spotlight vulnerabilities (with CVE, host, and
remediation facets), and prevention policies, tagging every row with its source
CID. The fabric kind additionally pulls the Flight Control objects themselves
(child CIDs, CID groups, user groups, role grants) from the parent client. The
fleet rollups (scorecard, vulns, stale, policy-drift, alerts, tenants,
remediate, trend) then read this store offline. Each sync also records a
per-CID posture snapshot that powers 'fleet trend'.

Requires a parent-CID API client with Flight Control (MSSP) scope. With --cids
you can target specific tenants; with neither flag, only the authenticated CID
is synced.`, "\n"),
		Example: strings.Trim(`
  crowdstrike-cli fleet sync --all-cids
  crowdstrike-cli fleet sync --cids abc123...,def456...
  crowdstrike-cli fleet sync --kinds hosts,vulns --all-cids`, "\n"),
		// NOT read-only: writes the synced fleet into the local SQLite store
		// (mutates local state across every targeted CID), mirroring the generic
		// `sync` command which is likewise unannotated so agents get prompted.
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return emitFleetSyncPlan(cmd, flags, allCids, cidsCSV, memberCID, kindsCSV)
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			return runFleetSync(cmd, flags, fleetSyncOptions{
				dbPath:     dbPath,
				allCids:    allCids,
				cidsCSV:    cidsCSV,
				memberCID:  memberCID,
				kindsCSV:   kindsCSV,
				vulnFilter: vulnFilter,
			})
		},
	}
	cmd.Flags().BoolVar(&allCids, "all-cids", false, "Discover and sync every Flight Control child CID (MSSP parent client required)")
	cmd.Flags().StringVar(&cidsCSV, "cids", "", "Comma-separated child CIDs to sync (overrides --all-cids discovery)")
	cmd.Flags().StringVar(&memberCID, "member-cid", "", "Sync a single child CID (Flight Control member_cid); shorthand for --cids <cid>")
	cmd.Flags().StringVar(&kindsCSV, "kinds", "hosts,alerts,vulns,policies,fabric", "Comma-separated entity kinds to sync (fabric = Flight Control CIDs/groups/roles)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&vulnFilter, "vuln-filter", defaultVulnFilter, "FQL filter for Spotlight vulnerabilities (Spotlight requires one)")
	return cmd
}

type fleetSyncOptions struct {
	dbPath     string
	allCids    bool
	cidsCSV    string
	memberCID  string
	kindsCSV   string
	vulnFilter string
}

type fleetSyncSummary struct {
	CIDsSynced int            `json:"cids_synced"`
	Entities   int            `json:"entities"`
	ByKind     map[string]int `json:"by_kind"`
	Warnings   []string       `json:"warnings,omitempty"`
}

func emitFleetSyncPlan(cmd *cobra.Command, flags *rootFlags, allCids bool, cidsCSV, memberCID, kindsCSV string) error {
	scope := "authenticated CID only"
	switch {
	case memberCID != "":
		scope = "CID: " + memberCID
	case cidsCSV != "":
		scope = "CIDs: " + cidsCSV
	case allCids:
		scope = "all Flight Control child CIDs"
	}
	plan := map[string]any{
		"event":  "plan",
		"action": "fleet sync",
		"scope":  scope,
		"kinds":  splitCSV(kindsCSV),
	}
	if flags.asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "would sync %s (kinds: %s)\n", scope, kindsCSV)
	return nil
}

func runFleetSync(cmd *cobra.Command, flags *rootFlags, opts fleetSyncOptions) error {
	ctx := cmd.Context()
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return configErr(err)
	}
	clientID, clientSecret := cfg.ClientID, cfg.ClientSecret
	if clientID == "" || clientSecret == "" {
		return authErr(fmt.Errorf("fleet sync needs a parent API client: set FALCON_CLIENT_ID and FALCON_CLIENT_SECRET"))
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = strings.TrimRight(cfg.BaseURL, "/") + "/oauth2/token"
	}
	kinds := splitCSV(opts.kindsCSV)
	if len(kinds) == 0 {
		kinds = []string{"hosts", "alerts", "vulns", "policies", "fabric"}
	}
	// fabric (Flight Control CIDs/groups/roles) is parent-scoped: it syncs once
	// via the parent client below, not per child CID.
	kinds, wantFabric := splitFabricKind(kinds)

	dbPath := opts.dbPath
	if dbPath == "" {
		dbPath = defaultDBPath("crowdstrike-cli")
	}

	// Resolve the CID list.
	var cids []string
	switch {
	case opts.memberCID != "":
		cids = []string{opts.memberCID}
	case opts.cidsCSV != "":
		cids = splitCSV(opts.cidsCSV)
	case opts.allCids:
		parent, perr := flags.newClient()
		if perr != nil {
			return perr
		}
		cids, err = fleetQueryIDs(ctx, parent, "/mssp/queries/children/v1", nil)
		if err != nil {
			return classifyAPIError(fmt.Errorf("discovering child CIDs: %w", err), flags)
		}
		if len(cids) == 0 {
			return apiErr(fmt.Errorf("no child CIDs returned; is this a Flight Control parent with MSSP scope?"))
		}
	default:
		cids = []string{""} // authenticated CID; member_cid empty
	}

	summary := fleetSyncSummary{ByKind: map[string]int{}}
	var allEnts []fleetEntity
	if len(kinds) > 0 {
		for _, cid := range cids {
			c, cerr := fleetClientForCID(ctx, cfg, flags, tokenURL, clientID, clientSecret, cid)
			label := labelCID(cid)
			if cerr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("cid %s: token mint failed: %v", label, cerr))
				continue
			}
			ents, warns := syncOneCID(ctx, c, label, kinds, opts.vulnFilter)
			summary.Warnings = append(summary.Warnings, warns...)
			if err := persistFleetEntities(ctx, dbPath, ents, "cid "+label, &summary); err != nil {
				return configErr(err)
			}
			summary.CIDsSynced++
			allEnts = append(allEnts, ents...)
			if !flags.asJSON {
				fmt.Fprintf(cmd.ErrOrStderr(), "synced %s: %d entities\n", label, len(ents))
			}
		}
	}

	if wantFabric {
		parent, perr := flags.newClient()
		if perr != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("fabric: parent client unavailable: %v", perr))
		} else {
			fents, warns := syncFabric(ctx, parent)
			summary.Warnings = append(summary.Warnings, warns...)
			if len(fents) > 0 {
				if err := persistFleetEntities(ctx, dbPath, fents, "fabric", &summary); err != nil {
					return configErr(err)
				}
				if !flags.asJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "synced fabric: %d entities\n", len(fents))
				}
			}
		}
	}

	// Record sync state (drives the generated hintIfUnsynced/hintIfStale
	// helpers in the rollups) and per-CID posture snapshots (drives fleet
	// trend) once per completed sync.
	if summary.CIDsSynced > 0 || summary.Entities > 0 {
		st, oerr := openFleetStore(ctx, dbPath)
		if oerr != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("sync-state write failed: %v", oerr))
		} else {
			if serr := st.SaveSyncState(fleetSyncStateKey, "", summary.Entities); serr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("sync-state write failed: %v", serr))
			}
			if snaps := snapshotFromEntities(allEnts); len(snaps) > 0 {
				if serr := writeFleetSnapshots(ctx, st.DB(), snaps, time.Now()); serr != nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("snapshot write failed: %v", serr))
				}
			}
			_ = st.Close() // best-effort cleanup; warnings already recorded above
		}
	}

	return flags.printJSON(cmd, summary)
}

// splitFabricKind removes the fabric pseudo-kind from the per-CID kind list
// and reports whether it was requested.
func splitFabricKind(kinds []string) ([]string, bool) {
	out := kinds[:0:0]
	fabric := false
	for _, k := range kinds {
		if strings.ToLower(strings.TrimSpace(k)) == "fabric" {
			fabric = true
			continue
		}
		out = append(out, k)
	}
	return out, fabric
}

// persistFleetEntities groups entities by kind and upserts each group in its
// own transaction, accumulating counts into the summary. labelPrefix names the
// source ("cid <label>" or "fabric") in store-write warnings.
func persistFleetEntities(ctx context.Context, dbPath string, ents []fleetEntity, labelPrefix string, summary *fleetSyncSummary) error {
	if len(ents) == 0 {
		return nil
	}
	byKind := map[string][]fleetEntity{}
	for _, e := range ents {
		byKind[e.Kind] = append(byKind[e.Kind], e)
	}
	st, err := openFleetStore(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	for k, group := range byKind {
		n, uerr := upsertFleetEntities(ctx, st.DB(), group)
		if uerr != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: store write (%s) failed: %v", labelPrefix, k, uerr))
			continue
		}
		summary.ByKind[k] += n
		summary.Entities += n
	}
	return nil
}

// syncFabric pulls the Flight Control objects (child CIDs, CID groups, CID
// group members, user groups, user group members, role grants) from the
// parent-scoped client into fabric-kind fleet entities. Each fetch failure is
// a warning, not a fatal error, so a partially-scoped parent client still
// syncs what it can.
func syncFabric(ctx context.Context, c *client.Client) ([]fleetEntity, []string) {
	now := time.Now()
	var out []fleetEntity
	var warns []string

	// Child CID details: queries/children -> entities/children/GET/v2.
	childIDs, err := fleetQueryIDs(ctx, c, "/mssp/queries/children/v1", nil)
	if err != nil {
		warns = append(warns, fmt.Sprintf("fabric: querying child CIDs: %v", err))
	}
	if len(childIDs) > 0 {
		data, _, derr := c.PostQueryWithParams(ctx, "/mssp/entities/children/GET/v2", nil, map[string]any{"ids": childIDs})
		if derr != nil {
			warns = append(warns, fmt.Sprintf("fabric: hydrating child CIDs: %v", derr))
			// Fall back to bare ID rows so fleet tenants still lists tenants.
			for _, id := range childIDs {
				out = append(out, fleetEntity{CID: "self", Kind: kindChildCID, ID: id, Name: id, SyncedAt: now})
			}
		} else {
			for _, o := range extractObjectResources(data) {
				id := firstString(o, "child_cid", "cid", "id")
				out = append(out, fleetEntity{
					CID: "self", Kind: kindChildCID,
					ID:       id,
					Name:     firstString(o, "name", "company", "child_cid"),
					Status:   firstString(o, "status"),
					SyncedAt: now,
					Raw:      mustRaw(o),
				})
			}
		}
	}

	// Generic query->hydrate pairs for the remaining fabric objects.
	type fabricSource struct {
		kind       string
		queryPath  string
		entityPath string
		idsInQuery bool // GET entity endpoint taking ids as query param
	}
	sources := []fabricSource{
		{kind: kindCIDGroup, queryPath: "/mssp/queries/cid-groups/v1", entityPath: "/mssp/entities/cid-groups/v2", idsInQuery: true},
		{kind: kindCIDGroupMember, queryPath: "/mssp/queries/cid-group-members/v1", entityPath: "/mssp/entities/cid-group-members/v2", idsInQuery: true},
		{kind: kindUserGroup, queryPath: "/mssp/queries/user-groups/v1", entityPath: "/mssp/entities/user-groups/v2", idsInQuery: true},
		{kind: kindUserGroupMember, queryPath: "/mssp/queries/user-group-members/v1", entityPath: "/mssp/entities/user-group-members/v2", idsInQuery: true},
		{kind: kindMSSPRole, queryPath: "/mssp/queries/mssp-roles/v1", entityPath: "/mssp/entities/mssp-roles/v1", idsInQuery: true},
	}
	for _, src := range sources {
		ids, qerr := fleetQueryIDs(ctx, c, src.queryPath, nil)
		if qerr != nil {
			warns = append(warns, fmt.Sprintf("fabric: querying %s: %v", src.kind, qerr))
			continue
		}
		if len(ids) == 0 {
			continue
		}
		data, derr := c.Get(ctx, src.entityPath, map[string]string{"ids": strings.Join(ids, ",")})
		if derr != nil {
			warns = append(warns, fmt.Sprintf("fabric: hydrating %s: %v", src.kind, derr))
			continue
		}
		for _, o := range extractObjectResources(data) {
			id := firstString(o, "cid_group_id", "user_group_id", "id")
			if id == "" {
				id = firstString(o, "cid", "uuid")
			}
			out = append(out, fleetEntity{
				CID: "self", Kind: src.kind,
				ID:       fabricEntityID(src.kind, o, id),
				Name:     firstString(o, "name", "description"),
				SyncedAt: now,
				Raw:      mustRaw(o),
			})
		}
	}
	return out, warns
}

// fabricEntityID builds a stable primary-key ID for fabric rows whose natural
// payload has no single unique id (e.g. role grants are unique per
// user_group+cid_group+role triple).
func fabricEntityID(kind string, o map[string]any, fallback string) string {
	switch kind {
	case kindMSSPRole:
		ug := firstString(o, "user_group_id")
		cg := firstString(o, "cid_group_id")
		role := firstString(o, "role_id", "role_name", "id")
		if ug != "" || cg != "" || role != "" {
			return ug + ":" + cg + ":" + role
		}
	case kindCIDGroupMember:
		if cg := firstString(o, "cid_group_id"); cg != "" {
			return cg
		}
	case kindUserGroupMember:
		if ug := firstString(o, "user_group_id"); ug != "" {
			return ug
		}
	}
	if fallback != "" {
		return fallback
	}
	return firstString(o, "id")
}

// fleetQueryIDs pages through a Falcon queries/* endpoint returning string IDs.
func fleetQueryIDs(ctx context.Context, c *client.Client, path string, params map[string]string) ([]string, error) {
	var all []string
	offset := 0
	for page := 0; page < fleetSyncMaxPages; page++ {
		p := map[string]string{"limit": strconv.Itoa(fleetSyncPageLimit), "offset": strconv.Itoa(offset)}
		for k, v := range params {
			if v != "" {
				p[k] = v
			}
		}
		data, err := c.Get(ctx, path, p)
		if err != nil {
			return nil, err
		}
		ids := extractStringResources(data)
		all = append(all, ids...)
		if len(ids) < fleetSyncPageLimit {
			break
		}
		offset += len(ids)
	}
	return all, nil
}

// fleetClientForCID returns an API client scoped to a child CID. For an empty
// cid it returns the parent-scoped client (the authenticated CID). Otherwise it
// mints a member_cid token and injects it so the client authenticates as that
// tenant.
func fleetClientForCID(ctx context.Context, cfg *config.Config, flags *rootFlags, tokenURL, clientID, clientSecret, cid string) (*client.Client, error) {
	if cid == "" {
		return flags.newClient()
	}
	// Bound the token mint by the same --timeout the API client honors; a
	// hung token endpoint must not stall the whole fleet sync.
	mintClient := &http.Client{Timeout: flags.timeout}
	tok, err := mintMemberCIDToken(ctx, mintClient, tokenURL, clientID, clientSecret, cid)
	if err != nil {
		return nil, err
	}
	return client.New(scopedChildConfig(cfg, tok.AccessToken), flags.timeout, flags.rateLimit), nil
}

// scopedChildConfig builds the config for a child-CID-scoped client so the
// ONLY credential it can ever present is the freshly minted member_cid token.
// Every invariant here closes a path back to the parent credential:
//   - ClientID/ClientSecret cleared: no config-sourced client_credentials.
//   - AuthHeaderVal cleared: a static auth_header (e.g. `auth set-token`)
//     wins over AccessToken in Config.AuthHeader() and would silently read
//     the PARENT tenant's data into rows tagged with the child CID.
//   - TokenExpiry zero: the generated client's needsClientCredentialsMint
//     re-mints within 60s of expiry via an env-var fallback
//     (FALCON_CLIENT_ID/SECRET) — WITHOUT member_cid, which would again be a
//     parent-scoped token. Zero expiry disables that gate; a genuinely
//     expired token surfaces as an honest per-CID 401 warning instead.
//   - Path cleared: a re-mint's SaveTokens would otherwise persist a parent
//     token over the user's real config.toml.
func scopedChildConfig(cfg *config.Config, accessToken string) *config.Config {
	scoped := *cfg // config.Config has no locks; value copy is safe
	scoped.AccessToken = accessToken
	scoped.ClientID = ""
	scoped.ClientSecret = ""
	scoped.AuthHeaderVal = ""
	scoped.TokenExpiry = time.Time{}
	scoped.Path = ""
	return &scoped
}

// mintMemberCIDToken mints an OAuth2 client_credentials token scoped to a child
// CID via the Flight Control member_cid form field.
func mintMemberCIDToken(ctx context.Context, httpClient *http.Client, tokenURL, clientID, clientSecret, memberCID string) (*tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	if memberCID != "" {
		form.Set("member_cid", memberCID)
	}
	if scope := resolveClientCredentialsScope(); scope != "" {
		form.Set("scope", scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling token endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(string(body)))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	return &tok, nil
}

// syncOneCID fetches the requested kinds for a single (already-scoped) client
// and returns the extracted entities plus any non-fatal warnings.
func syncOneCID(ctx context.Context, c *client.Client, cid string, kinds []string, vulnFilter string) ([]fleetEntity, []string) {
	var out []fleetEntity
	var warns []string
	now := time.Now()
	for _, kind := range kinds {
		var ents []fleetEntity
		var err error
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "hosts", "host", "devices":
			ents, err = fetchHosts(ctx, c, cid, now)
		case "alerts", "alert", "detects":
			ents, err = fetchAlerts(ctx, c, cid, now)
		case "vulns", "vulnerabilities", "spotlight":
			ents, err = fetchVulns(ctx, c, cid, vulnFilter, now)
		case "policies", "policy", "prevention":
			ents, err = fetchPolicies(ctx, c, cid, now)
		default:
			warns = append(warns, fmt.Sprintf("cid %s: unknown kind %q (skipped)", cid, kind))
			continue
		}
		if err != nil {
			warns = append(warns, fmt.Sprintf("cid %s: %s sync failed: %v", cid, kind, err))
			continue
		}
		out = append(out, ents...)
	}
	return out, warns
}

// ---- per-kind fetchers (combined endpoints: query + details in one call) ----

func fetchHosts(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedGetResources(ctx, c, "/devices/combined/devices/v1", map[string]string{"filter": defaultDeviceFilter})
	if err != nil {
		return nil, err
	}
	out := make([]fleetEntity, 0, len(objs))
	for _, o := range objs {
		out = append(out, fleetEntity{
			CID: cid, Kind: kindHost,
			ID:       firstString(o, "device_id", "id"),
			Name:     firstString(o, "hostname", "device_id"),
			Status:   firstString(o, "status", "reduced_functionality_mode"),
			LastSeen: parseFalconTime(firstString(o, "last_seen", "last_login_timestamp")),
			SyncedAt: now,
			Raw:      mustRaw(o),
		})
	}
	return out, nil
}

func fetchAlerts(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedPostResources(ctx, c, "/alerts/combined/alerts/v1", map[string]any{"filter": defaultAlertFilter})
	if err != nil {
		return nil, err
	}
	out := make([]fleetEntity, 0, len(objs))
	for _, o := range objs {
		out = append(out, fleetEntity{
			CID: cid, Kind: kindAlert,
			ID:       firstString(o, "composite_id", "id"),
			Name:     firstString(o, "display_name", "name", "description"),
			Severity: alertSeverity(o),
			Status:   firstString(o, "status"),
			LastSeen: parseFalconTime(firstString(o, "created_timestamp", "timestamp")),
			SyncedAt: now,
			Raw:      mustRaw(o),
		})
	}
	return out, nil
}

func fetchVulns(ctx context.Context, c *client.Client, cid, vulnFilter string, now time.Time) ([]fleetEntity, error) {
	if vulnFilter == "" {
		vulnFilter = defaultVulnFilter
	}
	// Repeated facet params ride the path; client params merge on top of the
	// parsed query, preserving the repeats. cve powers severity/CVE naming,
	// host_info powers fleet remediate's host counts, remediation powers the
	// remediation worklist grouping.
	objs, err := pagedGetResources(ctx, c, "/spotlight/combined/vulnerabilities/v1?facet=cve&facet=host_info&facet=remediation", map[string]string{"filter": vulnFilter})
	if err != nil {
		return nil, err
	}
	out := make([]fleetEntity, 0, len(objs))
	for _, o := range objs {
		out = append(out, fleetEntity{
			CID: cid, Kind: kindVuln,
			ID:       firstString(o, "id"),
			Name:     vulnName(o),
			Severity: vulnSeverity(o),
			Status:   firstString(o, "status"),
			SyncedAt: now,
			Raw:      mustRaw(o),
		})
	}
	return out, nil
}

func fetchPolicies(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedGetResources(ctx, c, "/policy/combined/prevention/v1", map[string]string{})
	if err != nil {
		return nil, err
	}
	out := make([]fleetEntity, 0, len(objs))
	for _, o := range objs {
		platform := firstString(o, "platform_name")
		name := firstString(o, "name", "id")
		enabled := "false"
		if b, ok := o["enabled"].(bool); ok && b {
			enabled = "true"
		}
		out = append(out, fleetEntity{
			CID: cid, Kind: kindPolicy,
			ID: firstString(o, "id"),
			// Name carries "<platform>:<name>" so policyDrift can compare
			// enabled-policy signatures across tenants.
			Name:     strings.TrimPrefix(platform+":"+name, ":"),
			Status:   enabled,
			SyncedAt: now,
			Raw:      mustRaw(o),
		})
	}
	return out, nil
}

// ---- HTTP plumbing for combined endpoints ----

func pagedGetResources(ctx context.Context, c *client.Client, path string, params map[string]string) ([]map[string]any, error) {
	var all []map[string]any
	offset := 0
	for page := 0; page < fleetSyncMaxPages; page++ {
		p := map[string]string{"limit": strconv.Itoa(fleetSyncPageLimit), "offset": strconv.Itoa(offset)}
		for k, v := range params {
			if v != "" {
				p[k] = v
			}
		}
		data, err := c.Get(ctx, path, p)
		if err != nil {
			return nil, err
		}
		objs := extractObjectResources(data)
		all = append(all, objs...)
		if len(objs) < fleetSyncPageLimit {
			break
		}
		offset += len(objs)
	}
	return all, nil
}

func pagedPostResources(ctx context.Context, c *client.Client, path string, body map[string]any) ([]map[string]any, error) {
	var all []map[string]any
	offset := 0
	for page := 0; page < fleetSyncMaxPages; page++ {
		b := map[string]any{"limit": fleetSyncPageLimit, "offset": offset}
		for k, v := range body {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			b[k] = v
		}
		data, _, err := c.Post(ctx, path, b)
		if err != nil {
			return nil, err
		}
		objs := extractObjectResources(data)
		all = append(all, objs...)
		if len(objs) < fleetSyncPageLimit {
			break
		}
		offset += len(objs)
	}
	return all, nil
}

// ---- response/field helpers ----

type resourcesEnvelope struct {
	Resources json.RawMessage `json:"resources"`
}

// extractObjectResources pulls the `resources` array of objects from a Falcon
// envelope, falling back to a bare top-level array.
func extractObjectResources(data json.RawMessage) []map[string]any {
	var env resourcesEnvelope
	if json.Unmarshal(data, &env) == nil && len(env.Resources) > 0 {
		var objs []map[string]any
		if json.Unmarshal(env.Resources, &objs) == nil {
			return objs
		}
	}
	var bare []map[string]any
	if json.Unmarshal(data, &bare) == nil {
		return bare
	}
	return nil
}

// extractStringResources pulls the `resources` array of strings (e.g. CID or
// entity-ID lists) from a Falcon envelope.
func extractStringResources(data json.RawMessage) []string {
	var env resourcesEnvelope
	if json.Unmarshal(data, &env) == nil && len(env.Resources) > 0 {
		var ids []string
		if json.Unmarshal(env.Resources, &ids) == nil {
			return ids
		}
	}
	var bare []string
	if json.Unmarshal(data, &bare) == nil {
		return bare
	}
	return nil
}

func firstString(o map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := o[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// alertSeverity prefers a named severity, else buckets Falcon's 0-100 score.
func alertSeverity(o map[string]any) string {
	if s := firstString(o, "severity_name"); s != "" {
		return strings.ToLower(s)
	}
	if v, ok := o["severity"]; ok {
		if f, ok := v.(float64); ok {
			return bucketScore(f)
		}
	}
	return ""
}

// bucketScore maps a Falcon 0-100 severity score to a named bucket.
func bucketScore(f float64) string {
	switch {
	case f >= 90:
		return "critical"
	case f >= 70:
		return "high"
	case f >= 40:
		return "medium"
	case f >= 1:
		return "low"
	default:
		return "informational"
	}
}

func vulnSeverity(o map[string]any) string {
	if cve, ok := o["cve"].(map[string]any); ok {
		if s := firstString(cve, "severity"); s != "" {
			return strings.ToLower(s)
		}
	}
	return strings.ToLower(firstString(o, "severity"))
}

func vulnName(o map[string]any) string {
	if cve, ok := o["cve"].(map[string]any); ok {
		if s := firstString(cve, "id"); s != "" {
			return s
		}
	}
	if app, ok := o["apps"].([]any); ok && len(app) > 0 {
		if a0, ok := app[0].(map[string]any); ok {
			if s := firstString(a0, "product_name_version", "product_name_normalized"); s != "" {
				return s
			}
		}
	}
	return firstString(o, "id")
}

func parseFalconTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func mustRaw(o map[string]any) json.RawMessage {
	b, err := json.Marshal(o)
	if err != nil {
		return nil
	}
	return b
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func labelCID(cid string) string {
	if cid == "" {
		return "self"
	}
	return cid
}
