// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature logic (Printing Press transcendence): cross-tenant
// Flight Control sync. Mints a member_cid-scoped OAuth2 token per child CID and
// pulls hosts/alerts/vulns/policies into the CID-keyed local store.

package cli

import (
	"context"
	"crowdstrike-pp-cli/internal/client"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/config"
	"crowdstrike-pp-cli/internal/store"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
	flags.applyTLSFlags(cfg) // --ca-cert / --tls-server-name reach the member_cid mint and every child client
	clientID, clientSecret := cfg.ClientID, cfg.ClientSecret
	if clientID == "" || clientSecret == "" {
		return authErr(fmt.Errorf("fleet sync needs a parent API client: set FALCON_CLIENT_ID and FALCON_CLIENT_SECRET"))
	}
	// crowdstrike handfix per-cloud-token-url: the member_cid mints post the
	// client pair to the one token URL every other mint uses; none, no post.
	tokenURL := client.ResolveCrowdStrikeTokenURL(cfg)
	if tokenURL == "" {
		return configErr(client.ErrNoCrowdStrikeTokenURL)
	}
	kinds := splitCSV(opts.kindsCSV)
	if len(kinds) == 0 {
		kinds = []string{"hosts", "alerts", "vulns", "policies", "fabric"}
	}
	// fabric (Flight Control CIDs/groups/roles) is parent-scoped: it syncs once
	// via the parent client below, not per child CID. Each kind is kept once
	// (crowdstrike handfix fleet-selectors-deduplicated).
	kinds, wantFabric := splitFabricKind(kinds)

	dbPath := opts.dbPath
	if dbPath == "" {
		dbPath = defaultDBPath("crowdstrike-cli")
	}

	// Resolve the CID list.
	var cids []string
	var discoveryWarning string
	discoveredAll := false // --all-cids read the whole child list
	switch {
	case opts.memberCID != "":
		cids = uniqueCIDs([]string{opts.memberCID})
	case opts.cidsCSV != "":
		cids = uniqueCIDs(splitCSV(opts.cidsCSV))
	case opts.allCids:
		parent, perr := flags.newClient()
		if perr != nil {
			return perr
		}
		cids, err = fleetQueryIDs(ctx, parent, "/mssp/queries/children/v1", nil)
		cids = uniqueCIDs(cids)
		var short *fleetShortWalk
		if errors.As(err, &short) && len(cids) > 0 {
			discoveryWarning = fmt.Sprintf("discovering child CIDs: %v", err)
			err = nil
		} else if err == nil {
			discoveredAll = true
		}
		if err != nil {
			return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("discovering child CIDs: %w", err), flags)
		}
		if len(cids) == 0 {
			return apiErr(fmt.Errorf("no child CIDs returned; is this a Flight Control parent with MSSP scope?"))
		}
	default:
		cids = []string{""} // authenticated CID; member_cid empty
	}

	summary := fleetSyncSummary{ByKind: map[string]int{}}
	if discoveryWarning != "" {
		summary.Warnings = append(summary.Warnings, discoveryWarning)
	}
	var allEnts []fleetEntity
	complete := map[fleetScope]bool{} // crowdstrike handfix fleet-refresh-completeness
	var synced []string
	if len(kinds) > 0 {
		for _, cid := range cids {
			c, cerr := fleetClientForCID(ctx, cfg, flags, tokenURL, clientID, clientSecret, cid)
			label := labelCID(cid)
			if cerr != nil {
				summary.Warnings = append(summary.Warnings, fmt.Sprintf("cid %s: token mint failed: %v", label, cerr))
				continue
			}
			ents, warns, read := syncOneCID(ctx, c, label, kinds, opts.vulnFilter)
			summary.Warnings = append(summary.Warnings, warns...)
			owner := "" // a child CID names its tenant
			if cid == "" {
				owner = selfScopeOwner(clientID, tokenURL)
			}
			if err := persistFleetEntities(ctx, dbPath, ents, read, owner, "cid "+label, &summary); err != nil {
				return configErr(err)
			}
			for _, s := range read {
				complete[s] = true
			}
			synced = append(synced, label)
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
			fents, warns, read := syncFabric(ctx, parent)
			summary.Warnings = append(summary.Warnings, warns...)
			if err := persistFleetEntities(ctx, dbPath, fents, read, "", "fabric", &summary); err != nil {
				return configErr(err)
			}
			if len(fents) > 0 && !flags.asJSON {
				fmt.Fprintf(cmd.ErrOrStderr(), "synced fabric: %d entities\n", len(fents))
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
			// crowdstrike handfix fleet-refresh-completeness: a posture snapshot
			// only for a CID whose hosts, alerts and vulnerabilities were all
			// read completely in this sync.
			snaps, skipped := postureSnapshots(allEnts, complete, synced)
			if fleetKindsCoverPosture(kinds) { // a sync that asked for a posture and did not read it
				for _, cid := range skipped {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("cid %s: no posture snapshot recorded for fleet trend (hosts, alerts and vulns were not all read completely in this sync)", cid))
				}
			}
			if len(snaps) > 0 {
				if serr := writeFleetSnapshots(ctx, st.DB(), snaps, time.Now()); serr != nil {
					summary.Warnings = append(summary.Warnings, fmt.Sprintf("snapshot write failed: %v", serr))
				}
			}
			if discoveredAll {
				summary.Warnings = append(summary.Warnings, fleetCIDsNotListed(ctx, st, cids)...)
			}
			if note := fleetLegacyRowsNote(ctx, st.DB()); note != "" {
				summary.Warnings = append(summary.Warnings, note)
			}
			_ = st.Close() // best-effort cleanup; warnings already recorded above
		}
	}

	return flags.printJSON(cmd, summary)
}

// splitFabricKind removes the fabric pseudo-kind from the per-CID kind list
// and reports whether it was requested. crowdstrike handfix
// fleet-selectors-deduplicated (Codex r1 of the 0.1.11 review, class duplicate
// selections count the same entity repeatedly): each kind is kept once, under
// the first name given for it, however else it was named (an alias such as
// spotlight for vulns, another case), so `--kinds hosts,alerts,vulns,vulns`
// no longer read the vulnerabilities twice and recorded every critical one
// twice in the posture snapshot.
func splitFabricKind(kinds []string) ([]string, bool) {
	out := kinds[:0:0]
	fabric := false
	seen := map[string]bool{}
	for _, k := range kinds {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "fabric" {
			fabric = true
			continue
		}
		if ek := fleetEntityKind(key); ek != "" {
			key = ek
		} else {
			key = "?" + key // an unknown kind is warned about once
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, k)
	}
	return out, fabric
}

// uniqueCIDs is each CID once, in lower case (the form Falcon lists a CID
// in; the console shows it in upper case), so a CID named twice, or in two
// cases, is read, stored and counted once and not as two tenants. crowdstrike
// handfix fleet-selectors-deduplicated.
func uniqueCIDs(cids []string) []string {
	out := cids[:0:0]
	seen := map[string]bool{}
	for _, c := range cids {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// selfScopeOwner is the client a "self" read is bound to: a one-way digest of
// the API client ID and the token URL it signs in at (an API client belongs to
// exactly one tenant), never the client ID itself, so the fleet store holds no
// credential and the credential kit's SQL check keeps the write (a chance
// overlap with a held value would make it refuse the write, never store it).
// crowdstrike handfix fleet-replace-only-a-proven-scope.
func selfScopeOwner(clientID, tokenURL string) string {
	sum := sha256.Sum256([]byte("crowdstrike fleet self scope\x00" + strings.TrimSpace(clientID) + "\x00" + tokenURL))
	return hex.EncodeToString(sum[:8])
}

// persistFleetEntities groups entities by CID and kind and writes each group
// in its own transaction, accumulating counts into the summary. A scope in
// read (one the sync read completely) is replaced: its rows are exactly what
// the read returned, none at all for an empty read (writeFleetEntities).
// owner is the client a "self" read is bound to (selfScopeOwner; "" for a
// child CID and the fabric). labelPrefix names the source ("cid <label>" or
// "fabric") in store-write warnings.
func persistFleetEntities(ctx context.Context, dbPath string, ents []fleetEntity, read []fleetScope, owner, labelPrefix string, summary *fleetSyncSummary) error {
	if len(ents) == 0 && len(read) == 0 {
		return nil
	}
	groups := map[fleetScope][]fleetEntity{}
	for _, e := range ents {
		s := fleetScope{CID: e.CID, Kind: e.Kind}
		groups[s] = append(groups[s], e)
	}
	replace := map[fleetScope]bool{}
	for _, s := range read {
		// crowdstrike handfix fleet-replace-only-a-proven-scope (Codex r2 class
		// refresh deletes another parent's fabric; Codex r1 of 0.1.11 class
		// completed refresh never removes disappeared records): "self" names
		// whichever client authenticated, not one tenant, so two parent (or
		// single-CID) clients sharing a --db would replace each other's rows.
		// A "self" scope is replaced only bound to the client that read it
		// (owner): its rows of the scope and the pre-owner rows it supersedes
		// (fleetLegacySelfRow), and no other client's. The fabric
		// has no owner and is upserted, never replaced; per-CID scopes name
		// one tenant.
		if s.CID == "self" && owner == "" {
			continue
		}
		replace[s] = true
		if _, ok := groups[s]; !ok {
			groups[s] = nil
		}
	}
	st, err := openFleetStore(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	for s, group := range groups {
		var whole *fleetScope
		if replace[s] {
			whole = &fleetScope{CID: s.CID, Kind: s.Kind}
		}
		n, uerr := writeFleetEntities(ctx, st.DB(), group, whole, owner)
		if uerr != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: store write (%s) failed: %v", labelPrefix, s.Kind, uerr))
			continue
		}
		summary.ByKind[s.Kind] += n
		summary.Entities += n
	}
	return nil
}

// fleetKindsCoverPosture reports a --kinds list naming every kind a posture
// snapshot counts.
func fleetKindsCoverPosture(kinds []string) bool {
	have := map[string]bool{}
	for _, k := range kinds {
		have[fleetEntityKind(k)] = true
	}
	for _, k := range postureKinds {
		if !have[k] {
			return false
		}
	}
	return true
}

// fleetCIDsNotListed warns about the CIDs the store holds per-tenant rows for
// that a complete --all-cids discovery did not list (a child removed from
// Flight Control). Their rows are kept, not deleted: a store named with --db
// can hold the children of more than one parent client, and this parent's
// child list says nothing about another's.
func fleetCIDsNotListed(ctx context.Context, st *store.Store, listed []string) []string {
	rows, err := st.DB().QueryContext(ctx, `SELECT DISTINCT cid FROM fleet_entities WHERE cid <> 'self'`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	in := map[string]bool{}
	for _, c := range listed {
		in[c] = true
	}
	var gone []string
	for rows.Next() {
		var cid string
		if rows.Scan(&cid) == nil && !in[cid] {
			gone = append(gone, cid)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	sort.Strings(gone)
	return []string{fmt.Sprintf("the store holds rows for %d CID(s) Flight Control no longer lists (%s); they were not refreshed and still appear in the fleet rollups", len(gone), strings.Join(gone, ", "))}
}

// syncFabric pulls the Flight Control objects (child CIDs, CID groups, CID
// group members, user groups, user group members, role grants) from the
// parent-scoped client into fabric-kind fleet entities. Each fetch failure is
// a warning, not a fatal error, so a partially-scoped parent client still
// syncs what it can.
// The third result lists the fabric kinds read completely (the whole query
// walk, and every hydrate answered without a Falcon error): fleet sync replaces
// those kinds' rows (crowdstrike handfix fleet-refresh-completeness).
func syncFabric(ctx context.Context, c *client.Client) ([]fleetEntity, []string, []fleetScope) {
	now := time.Now()
	var out []fleetEntity
	var warns []string
	var read []fleetScope

	// Child CID details: queries/children -> entities/children/GET/v2.
	childIDs, err := fleetQueryIDs(ctx, c, "/mssp/queries/children/v1", nil)
	if err != nil {
		warns = append(warns, fmt.Sprintf("fabric: querying child CIDs: %v", err))
	} else {
		read = append(read, fleetScope{CID: "self", Kind: kindChildCID})
	}
	if len(childIDs) > 0 {
		hydrated := map[string]bool{}
		data, _, derr := c.PostQueryWithParams(ctx, "/mssp/entities/children/GET/v2", nil, map[string]any{"ids": childIDs})
		if derr == nil {
			if msg := falconErrorsOf(data); msg != "" {
				warns = append(warns, fmt.Sprintf("fabric: hydrating child CIDs: the API reported %s", msg))
			}
			for _, o := range extractObjectResources(data) {
				id := firstString(o, "child_cid", "cid", "id")
				hydrated[id] = true
				out = append(out, fleetEntity{
					CID: "self", Kind: kindChildCID,
					ID:       id,
					Name:     firstString(o, "name", "company", "child_cid"),
					Status:   firstString(o, "status"),
					SyncedAt: now,
					Raw:      mustRaw(o),
				})
			}
		} else {
			warns = append(warns, fmt.Sprintf("fabric: hydrating child CIDs: %v", derr))
		}
		// Every listed child keeps a row (a bare ID row when its details did
		// not come back) so fleet tenants still lists every tenant.
		for _, id := range childIDs {
			if !hydrated[id] {
				out = append(out, fleetEntity{CID: "self", Kind: kindChildCID, ID: id, Name: id, SyncedAt: now})
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
			if len(ids) == 0 { // a short walk keeps what it read
				continue
			}
		}
		if len(ids) == 0 {
			read = append(read, fleetScope{CID: "self", Kind: src.kind}) // a complete, empty list
			continue
		}
		data, derr := c.Get(ctx, src.entityPath, map[string]string{"ids": strings.Join(ids, ",")})
		if derr != nil {
			warns = append(warns, fmt.Sprintf("fabric: hydrating %s: %v", src.kind, derr))
			continue
		}
		if msg := falconErrorsOf(data); msg != "" {
			warns = append(warns, fmt.Sprintf("fabric: hydrating %s: the API reported %s", src.kind, msg))
		} else if qerr == nil {
			read = append(read, fleetScope{CID: "self", Kind: src.kind})
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
	return out, warns, read
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
// A walk that ends short of its documented total returns what it read and a
// *fleetShortWalk.
func fleetQueryIDs(ctx context.Context, c *client.Client, path string, params map[string]string) ([]string, error) {
	return fleetWalk(func(offset int, after string) (json.RawMessage, error) {
		return c.Get(ctx, path, fleetPageParams(params, offset, after))
	}, extractStringResources, func(id string) string { return id })
}

// crowdstrike handfix fleet-walk-complete-only-at-total (the appdirect Codex r1
// class incomplete-paging-read-as-complete, on the fleet novel): every fleet
// sync list is read through fleetWalk. A Falcon list documents its paging in
// meta.pagination (offset, limit, total, and after on the cursor-paged lists:
// Spotlight vulnerabilities, combined alerts). The walk used to end on the
// first page shorter than fleet sync's limit and called that complete, so a
// server that caps the page size below 500 kept the first page of every
// tenant's hosts, and a list that pages by after (and answers the first page
// whatever offset it is sent) kept 500 records counted ten times over. Now the
// walk follows after when the list names one, advances the offset by what the
// page held, ends when it has the documented total (or, with no total, on a
// short page) or a page brings nothing new, and returns *fleetShortWalk when
// it ends with fewer distinct records than the total.

// fleetShortWalk reports a walk that ended with fewer distinct records than
// its documented total (meta.pagination.total), that read a page whose errors
// list was not empty (apiErrors), or that the page cap ended before the list
// did (capped): an incomplete read every way. It
// keeps the records it read; fleet sync warns and replaces nothing with them
// (crowdstrike handfix fleet-refresh-completeness: any failure signal wins).
type fleetShortWalk struct {
	got, total int
	apiErrors  string
	capped     bool // fleetSyncMaxPages pages read and the list had not ended
}

func (e *fleetShortWalk) Error() string {
	if e.capped {
		return fmt.Sprintf("read %d records in %d pages, the most fleet sync reads of one list, and the list had not ended; the rest were not synced", e.got, fleetSyncMaxPages)
	}
	if e.apiErrors != "" {
		return fmt.Sprintf("the API reported %s on a page; the %d records read were kept and the list was not read completely", e.apiErrors, e.got)
	}
	return fmt.Sprintf("read %d of the %d records the API reports (meta.pagination.total); the rest were not synced", e.got, e.total)
}

// falconErrorsOf describes a Falcon envelope's errors list ("" when the list
// is absent, null or empty): its count and the first error's code and message.
// fleetEnvelopeProblem names a page that is not a CrowdStrike list response:
// a JSON object carrying a resources member (FalconPy documents
// {"meta":...,"resources":[...],"errors":[...]}), or a bare list.
func fleetEnvelopeProblem(data json.RawMessage) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) == nil && obj != nil {
		if _, ok := obj["resources"]; ok {
			return ""
		}
		return "the response carried no resources list"
	}
	var list []json.RawMessage
	if json.Unmarshal(data, &list) == nil && list != nil {
		return ""
	}
	return "the response was not a CrowdStrike list"
}

func falconErrorsOf(data json.RawMessage) string {
	var env struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(data, &env) != nil || len(env.Errors) == 0 {
		return ""
	}
	var first struct {
		Code    json.Number `json:"code"`
		Message string      `json:"message"`
	}
	_ = json.Unmarshal(env.Errors[0], &first)
	msg := first.Message
	if len(msg) > 160 {
		msg = msg[:160] + "..."
	}
	return fmt.Sprintf("%d error(s) (first: code %s %q)", len(env.Errors), first.Code.String(), msg)
}

// fleetPaging is a Falcon list's documented paging metadata.
type fleetPaging struct {
	total    int
	hasTotal bool
	after    string
}

func fleetPagingOf(data json.RawMessage) fleetPaging {
	var env struct {
		Meta struct {
			Pagination struct {
				Total *int   `json:"total"`
				After string `json:"after"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if json.Unmarshal(data, &env) != nil {
		return fleetPaging{}
	}
	p := fleetPaging{after: env.Meta.Pagination.After}
	if env.Meta.Pagination.Total != nil {
		p.total, p.hasTotal = *env.Meta.Pagination.Total, true
	}
	return p
}

// fleetPageParams is params plus the page request: limit, and after when the
// list pages by cursor, else offset.
func fleetPageParams(params map[string]string, offset int, after string) map[string]string {
	p := map[string]string{"limit": strconv.Itoa(fleetSyncPageLimit)}
	if after != "" {
		p["after"] = after
	} else {
		p["offset"] = strconv.Itoa(offset)
	}
	for k, v := range params {
		if v != "" {
			p[k] = v
		}
	}
	return p
}

// fleetWalk reads every page fetch returns: items from each page, kept once by
// key. A fetch error ends the walk with nothing (as before).
func fleetWalk[T any](fetch func(offset int, after string) (json.RawMessage, error), items func(json.RawMessage) []T, key func(T) string) ([]T, error) {
	var all []T
	seen := map[string]bool{}
	offset, after, total := 0, "", -1
	apiErrors := ""
	page := 0
	for ; page < fleetSyncMaxPages; page++ {
		data, err := fetch(offset, after)
		if err != nil {
			return nil, err
		}
		if msg := falconErrorsOf(data); msg != "" && apiErrors == "" {
			apiErrors = msg
		}
		// crowdstrike handfix fleet-replace-only-a-proven-scope (Codex r2 P2): a
		// page without CrowdStrike's resources envelope (null, {}, not JSON)
		// proves nothing about the list, so the walk is not complete.
		if msg := fleetEnvelopeProblem(data); msg != "" && apiErrors == "" {
			apiErrors = msg
		}
		got := items(data)
		fresh := 0
		for _, it := range got {
			if k := key(it); !seen[k] {
				seen[k] = true
				all = append(all, it)
				fresh++
			}
		}
		pg := fleetPagingOf(data)
		if pg.hasTotal {
			total = pg.total
		}
		if fresh == 0 {
			break
		}
		if pg.after != "" {
			after = pg.after
			continue
		}
		after = ""
		if total >= len(all) {
			if len(all) == total {
				break
			}
		} else if len(got) < fleetSyncPageLimit {
			break
		}
		offset += len(got)
	}
	if apiErrors != "" {
		return all, &fleetShortWalk{got: len(all), total: total, apiErrors: apiErrors}
	}
	if total > len(all) {
		return all, &fleetShortWalk{got: len(all), total: total}
	}
	if page == fleetSyncMaxPages { // the page cap ended the walk, not the list
		return all, &fleetShortWalk{got: len(all), total: total, capped: true}
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
	// Hand-wired (handfixes.json: member-cid-token-stays-on-origin): the form
	// carries the client secret, and a plain client replays it on a 307 to
	// another host; the token client follows same-origin hops only.
	mintClient := cliutil.OAuthTokenHTTPClient(&http.Client{Timeout: flags.timeout})
	// crowdstrike handfix fleet-child-client-guarded: the mint sends through
	// the guarded plain transport `auth login` uses (the configured CA bundle
	// and TLS server name, the dial guard); the bare default transport trusted
	// only the system roots, so a mint behind a TLS-inspecting proxy
	// (CROWDSTRIKE_CA_CERT) failed.
	plain, err := client.NewPlainHTTPClient(flags.timeout, cfg)
	if err != nil {
		return nil, configErr(err)
	}
	mintClient.Transport = plain.Transport
	// crowdstrike handfix api-origin-pinned: the mint client reaches the API
	// origin and, on another token host, only the token URL's POST; it holds
	// what it answers and sends nothing under --dry-run.
	guardTokenClient(mintClient, cfg, flags.dryRun)
	tok, err := mintMemberCIDToken(ctx, mintClient, tokenURL, clientID, clientSecret, cid)
	if err != nil {
		return nil, err
	}
	// crowdstrike handfix fleet-child-client-guarded: a child client is built
	// like every other API client (flags.newClient): --dry-run, the TLS check
	// and the client hooks, so the origin pin, the response hold and the
	// no-response-cache rule (crowdstrike_guard.go) cover it. client.New alone
	// left the on-disk response cache on, and every child tenant's raw answers
	// were written to the cache directory.
	c := client.New(scopedChildConfig(cfg, tok.AccessToken), flags.timeout, flags.rateLimit)
	if tlsErr := c.TLSConfigError(); tlsErr != nil {
		return nil, configErr(tlsErr)
	}
	if flags.timeoutExplicit {
		c.SetTimeoutExplicit(true)
	}
	c.DryRun = flags.dryRun
	c.NoCache = flags.noCache
	if err := ApplyClientHooks(c); err != nil {
		return nil, err
	}
	return c, nil
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
//     (FALCON_CLIENT_ID/SECRET), WITHOUT member_cid, which would again be a
//     parent-scoped token. Zero expiry disables that gate; a genuinely
//     expired token surfaces as an honest per-CID 401 warning instead.
//   - Path cleared: a re-mint's SaveTokens would otherwise persist a parent
//     token over the user's real config.toml.
//   - Credential headers dropped (config.WithoutCredentialHeaders, crowdstrike
//     handfix stored-credential-bound-to-its-origin): the client sets the
//     configured headers after its own bearer, so an Authorization the parent
//     configuration carried would override the member_cid token and read
//     the PARENT tenant into the child CID's rows. Load already refuses a
//     configured credential header; the child config holds none either way.
//   - RefreshToken cleared and the config pinned (client.PinToken,
//     crowdstrike handfix member-cid-token-pinned): with the parent pair in
//     FALCON_CLIENT_ID/SECRET the client's token binding would otherwise mint
//     (or reuse) the PARENT's token for this config, and the child CID's rows
//     would hold the parent tenant's data. A pinned config never mints,
//     refreshes or swaps its token.
func scopedChildConfig(cfg *config.Config, accessToken string) *config.Config {
	scoped := *cfg // config.Config has no locks; value copy is safe
	scoped.AccessToken = accessToken
	scoped.ClientID = ""
	scoped.ClientSecret = ""
	scoped.AuthHeaderVal = ""
	scoped.RefreshToken = ""
	scoped.TokenExpiry = time.Time{}
	scoped.Path = ""
	scoped.Headers = config.WithoutCredentialHeaders(cfg.Headers)
	client.PinToken(&scoped)
	return &scoped
}

// mintMemberCIDToken mints an OAuth2 client_credentials token scoped to a child
// CID via the Flight Control member_cid form field.
func mintMemberCIDToken(ctx context.Context, httpClient *http.Client, tokenURL, clientID, clientSecret, memberCID string) (*tokenResponse, error) {
	// crowdstrike handfix token-request-body-credentials: the one token form.
	form := client.ClientCredentialsTokenForm(clientID, clientSecret, memberCID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// crowdstrike handfix fleet-child-client-guarded: the plaintext refusal the
	// generated mint and `auth login` apply (an http:// token URL off loopback
	// never receives the client secret).
	req, err = client.GuardTokenRequest(httpClient, req, []byte(form.Encode()))
	if err != nil {
		return nil, err
	}
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
	cliutil.HoldValues("minted:token", tok.AccessToken) // handfixes.json credkit-wiring: fleet sync's per-child member_cid token
	return &tok, nil
}

// syncOneCID fetches the requested kinds for a single (already-scoped) client
// and returns the extracted entities, any non-fatal warnings, and the scopes
// (this CID and an entity kind) it read completely: no error, no short walk,
// no Falcon error on any page (crowdstrike handfix fleet-refresh-completeness).
func syncOneCID(ctx context.Context, c *client.Client, cid string, kinds []string, vulnFilter string) ([]fleetEntity, []string, []fleetScope) {
	var out []fleetEntity
	var warns []string
	var read []fleetScope
	now := time.Now()
	for _, kind := range kinds {
		var ents []fleetEntity
		var err error
		entKind := fleetEntityKind(kind)
		switch entKind {
		case kindHost:
			ents, err = fetchHosts(ctx, c, cid, now)
		case kindAlert:
			ents, err = fetchAlerts(ctx, c, cid, now)
		case kindVuln:
			ents, err = fetchVulns(ctx, c, cid, vulnFilter, now)
		case kindPolicy:
			ents, err = fetchPolicies(ctx, c, cid, now)
		default:
			warns = append(warns, fmt.Sprintf("cid %s: unknown kind %q (skipped)", cid, kind))
			continue
		}
		if err != nil {
			var short *fleetShortWalk
			if errors.As(err, &short) {
				warns = append(warns, fmt.Sprintf("cid %s: %s sync incomplete: %v", cid, kind, err))
				out = append(out, ents...)
				continue
			}
			warns = append(warns, fmt.Sprintf("cid %s: %s sync failed: %v", cid, kind, err))
			continue
		}
		out = append(out, ents...)
		// crowdstrike handfix fleet-replace-only-a-proven-scope (Codex r2 class
		// filtered refresh treated as complete scope): an operator's
		// --vuln-filter narrows the read, so it proves nothing about the
		// vulnerabilities it did not select; only the default filter (the scope
		// the fleet views report) replaces the CID's stored vulnerabilities.
		if entKind == kindVuln && vulnFilter != "" && vulnFilter != defaultVulnFilter {
			continue
		}
		read = append(read, fleetScope{CID: cid, Kind: entKind})
	}
	return out, warns, read
}

// fleetEntityKind is the entity kind a --kinds name syncs ("" for none).
func fleetEntityKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "hosts", "host", "devices":
		return kindHost
	case "alerts", "alert", "detects":
		return kindAlert
	case "vulns", "vulnerabilities", "spotlight":
		return kindVuln
	case "policies", "policy", "prevention":
		return kindPolicy
	}
	return ""
}

// ---- per-kind fetchers (combined endpoints: query + details in one call) ----

func fetchHosts(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedGetResources(ctx, c, "/devices/combined/devices/v1", map[string]string{"filter": defaultDeviceFilter})
	if err != nil && objs == nil {
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
	return out, err // a *fleetShortWalk keeps the records it read
}

func fetchAlerts(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedPostResources(ctx, c, "/alerts/combined/alerts/v1", map[string]any{"filter": defaultAlertFilter})
	if err != nil && objs == nil {
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
	return out, err // a *fleetShortWalk keeps the records it read
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
	if err != nil && objs == nil {
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
	return out, err // a *fleetShortWalk keeps the records it read
}

func fetchPolicies(ctx context.Context, c *client.Client, cid string, now time.Time) ([]fleetEntity, error) {
	objs, err := pagedGetResources(ctx, c, "/policy/combined/prevention/v1", map[string]string{})
	if err != nil && objs == nil {
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
	return out, err // a *fleetShortWalk keeps the records it read
}

// ---- HTTP plumbing for combined endpoints ----

func pagedGetResources(ctx context.Context, c *client.Client, path string, params map[string]string) ([]map[string]any, error) {
	return fleetWalk(func(offset int, after string) (json.RawMessage, error) {
		return c.Get(ctx, path, fleetPageParams(params, offset, after))
	}, extractObjectResources, func(o map[string]any) string { return string(mustRaw(o)) })
}

func pagedPostResources(ctx context.Context, c *client.Client, path string, body map[string]any) ([]map[string]any, error) {
	return fleetWalk(func(offset int, after string) (json.RawMessage, error) {
		b := map[string]any{"limit": fleetSyncPageLimit}
		if after != "" {
			b["after"] = after
		} else {
			b["offset"] = offset
		}
		for k, v := range body {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			b[k] = v
		}
		data, _, err := c.Post(ctx, path, b)
		return data, err
	}, extractObjectResources, func(o map[string]any) string { return string(mustRaw(o)) })
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
