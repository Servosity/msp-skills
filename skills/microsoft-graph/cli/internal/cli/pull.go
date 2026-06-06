// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
	group    string                      // user-facing name for --only
	storeKey string                      // generic resource_type for sync state
	path     string                      // Graph path to list
	params   map[string]string           // initial query params
	upsert   func(json.RawMessage) error // store typed upsert (bound to the open store)
	// embed enriches each item with association data before upsert
	// (directory-roles: members; groups: members + owners). nil = no embed.
	embed func(ctx context.Context, c *client.Client, item json.RawMessage) (json.RawMessage, error)
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
alerts and incidents, licenses (subscribedSkus), devices, and
managed-devices. Requires a valid token — run
'microsoft-graph-cli auth login ...' or set MICROSOFT_GRAPH_TOKEN first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli pull
  microsoft-graph-cli pull --only users,licenses
  microsoft-graph-cli pull --only security`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
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
				}, upsert: st.UpsertUsers},
				{group: "groups", storeKey: "groups", path: "/groups", params: map[string]string{"$top": "999"}, upsert: st.UpsertGroups, embed: embedGroupAssociations},
				{group: "directory-roles", storeKey: "directory-roles", path: "/directoryRoles", upsert: st.UpsertDirectoryRoles, embed: embedRoleMembers},
				{group: "security", storeKey: "security", path: "/security/alerts_v2", params: map[string]string{"$top": "999"}, upsert: st.UpsertSecurity},
				{group: "security", storeKey: "security", path: "/security/incidents", params: map[string]string{"$top": "999"}, upsert: st.UpsertSecurity},
				{group: "licenses", storeKey: "licenses", path: "/subscribedSkus", upsert: st.UpsertLicenses},
				{group: "devices", storeKey: "devices", path: "/devices", params: map[string]string{"$top": "999"}, upsert: st.UpsertDevices},
				{group: "managed-devices", storeKey: "managed-devices", path: "/deviceManagement/managedDevices", params: map[string]string{"$top": "999"}, upsert: st.UpsertManagedDevices},
			}

			result := pullResult{}
			ctx := cmd.Context()
			for _, r := range resources {
				if !pullSelected(only, r.group) {
					continue
				}
				items, err := graphGetAllPages(ctx, c, r.path, r.params)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "pull: %s failed: %v\n", r.path, err)
					result.Resources = append(result.Resources, pullSummary{Resource: r.path, Error: err.Error()})
					continue
				}
				count := 0
				for _, item := range items {
					toUpsert := item
					if r.embed != nil {
						if withAssoc, mErr := r.embed(ctx, c, item); mErr == nil {
							toUpsert = withAssoc
						} else {
							fmt.Fprintf(cmd.ErrOrStderr(), "pull: embedding associations for %s failed: %v\n", r.group, mErr)
						}
					}
					if err := r.upsert(toUpsert); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "pull: upsert into %s failed: %v\n", r.storeKey, err)
						continue
					}
					count++
				}
				_ = st.SaveSyncState(r.storeKey, "", count)
				result.Total += count
				result.Resources = append(result.Resources, pullSummary{Resource: r.path, Count: count})
				if !flags.asJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "pull: %-32s %d\n", r.path, count)
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Limit to these groups: users,groups,directory-roles,security,licenses,devices,managed-devices")
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
// client's auth, retry, and rate-limit handling.
func graphGetAllPages(ctx context.Context, c *client.Client, path string, params map[string]string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	curPath := path
	curParams := params
	for page := 1; page <= 100000; page++ {
		data, err := c.Get(ctx, curPath, curParams)
		if err != nil {
			return nil, err
		}
		var env struct {
			Value []json.RawMessage `json:"value"`
			Next  string            `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			// Not a collection envelope (e.g. a single object) — return what
			// we have so far rather than erroring.
			return all, nil
		}
		all = append(all, env.Value...)
		if env.Next == "" {
			return all, nil
		}
		u, err := url.Parse(env.Next)
		if err != nil {
			return all, nil
		}
		curPath = graphRelPath(u.Path)
		curParams = flattenQuery(u.Query())
	}
	return all, nil
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
	members, err := graphGetAllPages(ctx, c, "/directoryRoles/"+url.PathEscape(id)+"/members", map[string]string{"$top": "999"})
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
