// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/client"
	"microsoft-graph-pp-cli/internal/cliutil"
	"microsoft-graph-pp-cli/internal/store"
)

// pullResource describes one Graph collection to pull into the local store.
type pullResource struct {
	group    string            // user-facing name for --only
	storeKey string            // generic resource_type for sync state
	path     string            // Graph path to list
	params   map[string]string // initial query params
	// embed enriches each item with association data before it is stored
	// (directory-roles: members; groups: members + owners). nil = no embed.
	embed func(ctx context.Context, c *client.Client, item json.RawMessage) (json.RawMessage, error)
	// skipPaged: Graph pages this collection with $skip, not only nextLink
	// (graphGetAllSkipPages; handfix teams-phone-reads-every-page).
	skipPaged bool
}

type pullSummary struct {
	Resource string `json:"resource"`
	Count    int    `json:"count"`
	Error    string `json:"error,omitempty"`
}

type pullResult struct {
	Total     int           `json:"total"`
	Resources []pullSummary `json:"resources"`
}

func newPullCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var only []string

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Sync the MSP Graph surface into the local store (follows @odata.nextLink)",
		Long: strings.Trim(`
Pulls the MSP-relevant Microsoft Graph surface into the local SQLite store,
following @odata.nextLink to completion so the offline analytics (licenses
waste/orphans, admins audit, security triage, managed-devices drift, tenant
snapshot) see every record, not just the first page.

Pulled resources: users (with assignedLicenses), groups (with members and
owners embedded), directory-roles (with their members embedded), security
alerts and incidents, licenses (subscribedSkus), devices, managed-devices,
and Teams Phone: phone-numbers (telephone number assignments) and teams-users
(per-user voice configuration and effective Teams policy assignments). The
Teams resources need TeamsTelephoneNumber.Read.All and
TeamsUserConfiguration.Read.All; without them they report a per-resource error
and the rest of the pull continues. Requires a valid token — run
'microsoft-graph-cli auth login ...' or set MICROSOFT_GRAPH_TOKEN first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli pull
  microsoft-graph-cli pull --only users,licenses
  microsoft-graph-cli pull --only security
  microsoft-graph-cli pull --only phone-numbers,teams-users`, "\n"),
		// Not read-only: pull replaces the local store's snapshots, so
		// MICROSOFT_GRAPH_MCP_READ_ONLY drops it as it drops sync (it is on the
		// MCP local-only list: no tenant state changes).
		Annotations: map[string]string{},
		RunE: func(cmd *cobra.Command, args []string) error {
			// The verify harness drives every command with no credentials; a
			// real pull would hit the network and fail. Short-circuit to a
			// clean no-op so verify/validate-narrative full-example runs stay
			// green without making live calls.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "pull: verify mode — skipping live sync")
				return nil
			}
			if dryRunOK(flags) {
				return nil
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			st, err := store.OpenWithContext(cmd.Context(), graphDBPath(dbPath))
			if err != nil {
				return configErr(fmt.Errorf("opening local store: %w", err))
			}
			defer st.Close()

			resources := []pullResource{
				{group: "users", storeKey: "users", path: "/users", params: map[string]string{
					"$top":    "999",
					"$select": "id,displayName,userPrincipalName,mail,jobTitle,department,accountEnabled,userType,createdDateTime,assignedLicenses",
				}},
				{group: "groups", storeKey: "groups", path: "/groups", params: map[string]string{"$top": "999"}, embed: embedGroupAssociations},
				{group: "directory-roles", storeKey: "directory-roles", path: "/directoryRoles", embed: embedRoleMembers},
				{group: "security", storeKey: "security", path: "/security/alerts_v2", params: map[string]string{"$top": "999"}},
				{group: "security", storeKey: "security", path: "/security/incidents", params: map[string]string{"$top": "999"}},
				{group: "licenses", storeKey: "licenses", path: "/subscribedSkus"},
				{group: "devices", storeKey: "devices", path: "/devices", params: map[string]string{"$top": "999"}},
				{group: "managed-devices", storeKey: "managed-devices", path: "/deviceManagement/managedDevices", params: map[string]string{"$top": "999"}},
				// Teams Phone reads. numberAssignments documents $top up to 1000; userConfigurations
				// documents no page-size maximum, so it takes the server's page size.
				{group: "phone-numbers", storeKey: "phone-numbers", path: "/admin/teams/telephoneNumberManagement/numberAssignments", params: map[string]string{"$top": teamsSkipPageSize}, skipPaged: true},
				{group: "teams-users", storeKey: "teams-users", path: "/admin/teams/userConfigurations"},
			}

			result := pullResult{}
			ctx := cmd.Context()
			// A completed pull replaces each resource's stored snapshot
			// (handfix pull-replaces-snapshot): emptied arrays, deleted objects
			// and revoked memberships leave the store. A resource is replaced
			// only when every path behind its store key (security reads alerts
			// and incidents) and every embedded association was read in full;
			// otherwise its previous snapshot is kept and the error reported.
			var keys []string
			byKey := map[string][]pullResource{}
			for _, r := range resources {
				if !pullSelected(only, r.group) {
					continue
				}
				if _, seen := byKey[r.storeKey]; !seen {
					keys = append(keys, r.storeKey)
				}
				byKey[r.storeKey] = append(byKey[r.storeKey], r)
			}
			for _, key := range keys {
				var items []json.RawMessage
				var paths []string
				var failed error
				for _, r := range byKey[key] {
					paths = append(paths, r.path)
					read := graphGetAllPages
					if r.skipPaged {
						read = graphGetAllSkipPages
					}
					got, err := read(ctx, c, r.path, r.params)
					for i := 0; err == nil && r.embed != nil && i < len(got); i++ {
						if got[i], err = r.embed(ctx, c, got[i]); err != nil {
							err = fmt.Errorf("embedding associations: %w", err)
						}
					}
					if err != nil {
						failed = fmt.Errorf("%s: %w", r.path, err)
						break
					}
					items = append(items, got...)
				}
				if failed == nil {
					failed = st.ReplaceResources(key, items)
				}
				resource := strings.Join(paths, ",")
				if failed != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "pull: %s failed; the previous %s snapshot is kept: %v\n", resource, key, failed)
					result.Resources = append(result.Resources, pullSummary{Resource: resource, Error: failed.Error()})
					continue
				}
				_ = st.SaveSyncState(key, "", len(items))
				result.Total += len(items)
				result.Resources = append(result.Resources, pullSummary{Resource: resource, Count: len(items)})
				if !flags.asJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "pull: %-32s %d\n", resource, len(items))
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Limit to these groups: users,groups,directory-roles,security,licenses,devices,managed-devices,phone-numbers,teams-users")
	return cmd
}

// pullSelected reports whether a resource group should be pulled given the
// --only filter (empty filter pulls everything).
func pullSelected(only []string, group string) bool {
	if len(only) == 0 {
		return true
	}
	for _, o := range only {
		if strings.EqualFold(strings.TrimSpace(o), group) {
			return true
		}
	}
	return false
}

// graphGetAllPages fetches every page of a Graph collection, following the
// absolute @odata.nextLink URL each page returns. The generated client's Get
// joins a relative path to the base URL, so the nextLink is reduced to its
// version-relative path plus query before each follow-up call — reusing the
// client's auth, retry, and rate-limit handling, and keeping every page on the
// configured host. A collection that cannot be read to its end is an error,
// never a short list (Codex r1 class incomplete-read-as-complete): a page
// after the first that is not a collection, a nextLink that does not parse, or
// the page cap. A first response that is a bare array is its whole collection;
// one that is not a collection at all (a dry-run preview) yields no items.
func graphGetAllPages(ctx context.Context, c *client.Client, path string, params map[string]string) ([]json.RawMessage, error) {
	return graphGetAllPagesWithHeaders(ctx, c, path, params, nil)
}

func graphGetAllPagesWithHeaders(ctx context.Context, c *client.Client, path string, params, headers map[string]string) ([]json.RawMessage, error) {
	return graphReadPages(ctx, c, path, params, headers, false, nil)
}

// graphReadPages is the page walk behind graphGetAllPages under a read
// policy: nil is the policy of pull, GetCollection and every other reader;
// sync passes its own (graphRead, handfix sync-reads-every-page), under which
// every page must be a collection, every nextLink must name the configured
// host and a page not yet read, and --max-pages may stop the walk. continued
// marks a walk that continues a collection already begun (GetCollection's
// nextLink, a later $skip window): its first response is a page like any
// other, so a bare array there is not the collection's end (class
// incomplete-read-as-complete).
func graphReadPages(ctx context.Context, c *client.Client, path string, params, headers map[string]string, continued bool, rd *graphRead) ([]json.RawMessage, error) {
	const maxPages = 100000
	var all []json.RawMessage
	curPath := path
	curParams := params
	for page := 1; page <= maxPages; page++ {
		data, err := rd.get(ctx, c, curPath, curParams, headers)
		if err != nil {
			return nil, err
		}
		var env struct {
			Value *[]json.RawMessage `json:"value"`
			Next  *string            `json:"@odata.nextLink"`
		}
		// A page without a value array is not an empty collection: `pull`
		// replaces the snapshot with what it read, so {} or {"value":null} would
		// erase it (Codex r2 class incomplete-read-as-complete).
		if err := json.Unmarshal(data, &env); err != nil || env.Value == nil {
			if page == 1 && !continued && rd == nil {
				var bare []json.RawMessage
				if json.Unmarshal(data, &bare) == nil && bare != nil {
					return bare, nil
				}
				if c.DryRun {
					return nil, nil // the dry-run preview, not a page
				}
			}
			return nil, fmt.Errorf("%s: page %d is not a Graph collection (no value array)", path, rd.pageNumber(page))
		}
		all = append(all, *env.Value...)
		next := ""
		if env.Next != nil {
			next = *env.Next
		}
		if next == "" {
			if env.Next != nil && rd != nil {
				return nil, fmt.Errorf("%s: page %d carries an empty @odata.nextLink; the collection was not read to its end", path, rd.pageNumber(page))
			}
			return all, nil
		}
		u, err := url.Parse(next)
		if err != nil {
			return nil, fmt.Errorf("%s: page %d carries a nextLink that does not parse: %w", path, rd.pageNumber(page), err)
		}
		if err := rd.follow(path, next, u); err != nil {
			return nil, err
		}
		if rd.capReached() {
			return all, nil
		}
		curPath = graphRelPath(u.Path)
		curParams = flattenQuery(u.Query())
	}
	return nil, fmt.Errorf("%s: more than %d pages; the collection was not read to its end", path, maxPages)
}

// graphRelPath strips the Graph version prefix from a nextLink path so it can be
// re-joined against the client's base URL.
func graphRelPath(p string) string {
	for _, pre := range []string{"/v1.0", "/beta"} {
		if strings.HasPrefix(p, pre) {
			return strings.TrimPrefix(p, pre)
		}
	}
	return p
}

// flattenQuery collapses url.Values to the single-value map the client expects.
func flattenQuery(v url.Values) map[string]string {
	m := make(map[string]string, len(v))
	for k := range v {
		m[k] = v.Get(k)
	}
	return m
}

// embedGroupAssociations fetches /groups/{id}/members and /groups/{id}/owners
// and embeds them under "members" and "owners" keys in the group object, so
// 'groups risk' can audit ownerless/empty/guest-heavy groups entirely from the
// local store. Members are narrowed via $select to the fields the audit reads.
func embedGroupAssociations(ctx context.Context, c *client.Client, groupRaw json.RawMessage) (json.RawMessage, error) {
	var group map[string]json.RawMessage
	if err := json.Unmarshal(groupRaw, &group); err != nil {
		return groupRaw, err
	}
	var id string
	if raw, ok := group["id"]; ok {
		_ = json.Unmarshal(raw, &id)
	}
	if id == "" {
		return groupRaw, nil
	}
	members, err := graphGetAllPages(ctx, c, "/groups/"+url.PathEscape(id)+"/members", map[string]string{
		"$top":    "999",
		"$select": "id,displayName,userPrincipalName,userType,accountEnabled",
	})
	if err != nil {
		return groupRaw, err
	}
	owners, err := graphGetAllPages(ctx, c, "/groups/"+url.PathEscape(id)+"/owners", map[string]string{
		"$top":    "999",
		"$select": "id",
	})
	if err != nil {
		return groupRaw, err
	}
	mb, err := marshalJSONArray(members)
	if err != nil {
		return groupRaw, err
	}
	ob, err := marshalJSONArray(owners)
	if err != nil {
		return groupRaw, err
	}
	group["members"] = mb
	group["owners"] = ob
	out, err := json.Marshal(group)
	if err != nil {
		return groupRaw, err
	}
	return out, nil
}

// marshalJSONArray marshals a possibly-nil slice as a JSON array, never null.
// json.Marshal of a nil slice emits `null`, which downstream pointer-slice
// unmarshal targets (insights.groupLite) read as "associations never synced" —
// collapsing a genuinely ownerless/empty group into the missing-data bucket
// and silently dropping the exact findings 'groups risk' exists to surface.
// Always emitting [] keeps the zero-vs-missing distinction intact.
func marshalJSONArray(items []json.RawMessage) (json.RawMessage, error) {
	if items == nil {
		return json.RawMessage("[]"), nil
	}
	return json.Marshal(items)
}

// embedRoleMembers fetches /directoryRoles/{id}/members and embeds them under a
// "members" key in the role object, so 'admins audit' can resolve role
// membership entirely from the local store.
func embedRoleMembers(ctx context.Context, c *client.Client, roleRaw json.RawMessage) (json.RawMessage, error) {
	var role map[string]json.RawMessage
	if err := json.Unmarshal(roleRaw, &role); err != nil {
		return roleRaw, err
	}
	var id string
	if raw, ok := role["id"]; ok {
		_ = json.Unmarshal(raw, &id)
	}
	if id == "" {
		return roleRaw, nil
	}
	// "List members of a directory role" returns a user's default projection
	// (no accountEnabled or userType) and supports $select only, not $top.
	members, err := graphGetAllPages(ctx, c, "/directoryRoles/"+url.PathEscape(id)+"/members", map[string]string{
		"$select": "id,displayName,userPrincipalName,userType,accountEnabled",
	})
	if err != nil {
		return roleRaw, err
	}
	mb, err := marshalJSONArray(members)
	if err != nil {
		return roleRaw, err
	}
	role["members"] = mb
	out, err := json.Marshal(role)
	if err != nil {
		return roleRaw, err
	}
	return out, nil
}
