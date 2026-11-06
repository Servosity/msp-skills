// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence command for rewst-cli. NOT generated.
//
// orgs answers the first question every other rollup needs: which
// organization ids can this token see? health, failures, roi, dormant and
// drift take --org, coverage takes --parent, and the typed `organizations`
// mirror cannot reach the GraphQL gateway (it is generated REST-shaped), so
// without this command an operator had to copy ids out of the web console.
// One live GraphQL read: the token's own organization and its managed and
// sub-organizations.

package cli

// pp:data-source live

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const orgsQuery = `query Orgs {
  userOrganization {
    id name isMsp managingOrgId
    managedAndSubOrgs { id name managingOrgId isEnabled isDeleted }
  }
}`

type orgRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ManagingOrgID string `json:"managing_org_id"`
	Enabled       *bool  `json:"enabled"`
	Deleted       bool   `json:"deleted"`
}

type orgsView struct {
	Org           orgRow   `json:"org"`
	IsMSP         *bool    `json:"is_msp"`
	ManagedCount  int      `json:"managed_count"`
	DisabledCount int      `json:"disabled_count"`
	ManagedOrgs   []orgRow `json:"managed_orgs"`
}

// orgsFromAnswer reads the Orgs answer. A missing userOrganization is an
// error (the token resolved no organization), never an empty directory, and
// a body that does not decode is an error too: an MSP picking --org values
// from this output must not read "no orgs" where the API said nothing.
func orgsFromAnswer(data json.RawMessage, includeDeleted bool) (orgsView, error) {
	var payload struct {
		UserOrganization *struct {
			ID                any    `json:"id"`
			Name              string `json:"name"`
			IsMSP             *bool  `json:"isMsp"`
			ManagingOrgID     *any   `json:"managingOrgId"`
			ManagedAndSubOrgs *[]struct {
				ID            any    `json:"id"`
				Name          string `json:"name"`
				ManagingOrgID *any   `json:"managingOrgId"`
				IsEnabled     *bool  `json:"isEnabled"`
				IsDeleted     bool   `json:"isDeleted"`
			} `json:"managedAndSubOrgs"`
		} `json:"userOrganization"`
	}
	if len(data) == 0 {
		return orgsView{}, fmt.Errorf("the Orgs query answered no data")
	}
	// decodeRow refuses a null wherever the answer is read as a value (a null
	// org id, name or isDeleted, a null managed-org element): rewst handfix
	// novel-missing-input-is-an-error.
	if err := decodeRow(data, &payload); err != nil {
		return orgsView{}, fmt.Errorf("decoding the Orgs answer: %w", err)
	}
	u := payload.UserOrganization
	if u == nil || idText(u.ID) == "" {
		return orgsView{}, fmt.Errorf("the token resolved no organization (userOrganization is empty); check REWST_API_TOKEN and REWST_BASE_URL (region)")
	}
	if u.ManagedAndSubOrgs == nil {
		return orgsView{}, fmt.Errorf("the Orgs answer carries no managedAndSubOrgs list for %s", idText(u.ID))
	}
	view := orgsView{
		Org:         orgRow{ID: idText(u.ID), Name: u.Name, ManagingOrgID: idPtrText(u.ManagingOrgID)},
		IsMSP:       u.IsMSP,
		ManagedOrgs: make([]orgRow, 0, len(*u.ManagedAndSubOrgs)),
	}
	for _, o := range *u.ManagedAndSubOrgs {
		row := orgRow{ID: idText(o.ID), Name: o.Name, ManagingOrgID: idPtrText(o.ManagingOrgID), Enabled: o.IsEnabled}
		if o.IsDeleted {
			row.Deleted = true
			if !includeDeleted {
				continue
			}
		}
		if row.ID == "" || row.ID == view.Org.ID {
			continue
		}
		if row.Enabled != nil && !*row.Enabled {
			view.DisabledCount++
		}
		view.ManagedOrgs = append(view.ManagedOrgs, row)
	}
	sort.SliceStable(view.ManagedOrgs, func(i, j int) bool {
		a, b := strings.ToLower(view.ManagedOrgs[i].Name), strings.ToLower(view.ManagedOrgs[j].Name)
		if a != b {
			return a < b
		}
		return view.ManagedOrgs[i].ID < view.ManagedOrgs[j].ID
	})
	view.ManagedCount = len(view.ManagedOrgs)
	return view, nil
}

// idPtrText is idText for an optional relationship (managingOrgId: null on
// the root org means "manages itself", not a missing value).
func idPtrText(v *any) string {
	if v == nil {
		return ""
	}
	return idText(*v)
}

// idText renders a GraphQL ID (a string, or a number some resolvers emit)
// as text; nil is "".
func idText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func newOrgsCmd(flags *rootFlags) *cobra.Command {
	var includeDeleted bool
	cmd := &cobra.Command{
		Use:         "orgs",
		Short:       "The token's organization and every managed sub-org, with the ids the other commands take",
		Long:        "List the organization this token belongs to and every organization it manages (id, name, managing org, enabled), sorted by name. Use the ids as --org for health, failures, roi, dormant and drift, and the MSP org's id as --parent for coverage.",
		Example:     "  rewst-cli orgs --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch userOrganization and its managedAndSubOrgs")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			data, err := rewstGraphQL(cmd.Context(), c, orgsQuery, nil)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view, err := orgsFromAnswer(data, includeDeleted)
			if err != nil {
				return err
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%s  %s", view.Org.ID, view.Org.Name)
				if view.IsMSP != nil && *view.IsMSP {
					fmt.Fprint(w, "  (MSP)")
				}
				fmt.Fprintf(w, "\n%d managed org(s), %d disabled\n", view.ManagedCount, view.DisabledCount)
				for _, o := range view.ManagedOrgs {
					state := ""
					if o.Enabled != nil && !*o.Enabled {
						state = "  disabled"
					}
					if o.Deleted {
						state += "  deleted"
					}
					fmt.Fprintf(w, "  %s  %s%s\n", o.ID, o.Name, state)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&includeDeleted, "include-deleted", false, "Also list organizations Rewst marks deleted")
	return cmd
}
