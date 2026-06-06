// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type dupeContact struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email,omitempty"`
	Organization   string `json:"organization,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
}

type dupeGroup struct {
	MatchType string        `json:"match_type"` // "email", "name", or "email+name"
	Key       string        `json:"key"`
	Count     int           `json:"count"`
	Contacts  []dupeContact `json:"contacts"`
}

// pp:data-source local
func newNovelContactsDupesCmd(flags *rootFlags) *cobra.Command {
	var flagOrg string

	cmd := &cobra.Command{
		Use:   "dupes",
		Short: "Find contacts that share a normalized name or email (local dedup)",
		Long: `Find duplicate contacts — entries that share a normalized email or name —
within an organization (or across all of them).

Overlapping PSA/RMM syncs routinely create the same person twice; no IT Glue
endpoint detects this. This is a local self-join over the synced contacts table,
grouping by lowercased, whitespace-collapsed email and name.`,
		Example: `  # Duplicate contacts across every client
  itglue-cli contacts dupes --agent

  # Duplicates within one organization
  itglue-cli contacts dupes --org 12345`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "contacts") {
				hintIfStale(cmd, db, "contacts", flags.maxAge)
			}

			recs, err := listITGRecords(db, "contacts")
			if err != nil {
				return apiErr(fmt.Errorf("reading contacts: %w", err))
			}

			contacts := make([]dupeContact, 0, len(recs))
			emailKey := make([]string, 0, len(recs))
			nameKey := make([]string, 0, len(recs))
			for _, rec := range recs {
				if flagOrg != "" && rec.orgID() != flagOrg {
					continue
				}
				email := rec.contactEmail()
				name := rec.displayName()
				contacts = append(contacts, dupeContact{
					ID:             rec.ID,
					Name:           name,
					Email:          email,
					Organization:   rec.orgName(),
					OrganizationID: rec.orgID(),
				})
				emailKey = append(emailKey, normalizeDupeKey(email))
				nameKey = append(nameKey, normalizeDupeKey(name))
			}

			// Group indices by normalized email and by normalized name.
			byEmail := map[string][]int{}
			byName := map[string][]int{}
			for i := range contacts {
				if emailKey[i] != "" {
					byEmail[emailKey[i]] = append(byEmail[emailKey[i]], i)
				}
				if nameKey[i] != "" {
					byName[nameKey[i]] = append(byName[nameKey[i]], i)
				}
			}

			// Merge candidate groups (size > 1) by their member-id signature so a
			// pair sharing BOTH name and email is reported once as "email+name".
			type pending struct {
				idxs    []int
				key     string
				matched map[string]bool
			}
			merged := map[string]*pending{}
			add := func(idxs []int, key, matchType string) {
				if len(idxs) < 2 {
					return
				}
				ids := make([]string, len(idxs))
				for n, i := range idxs {
					ids[n] = contacts[i].ID
				}
				sort.Strings(ids)
				sig := strings.Join(ids, "|")
				p, ok := merged[sig]
				if !ok {
					p = &pending{idxs: idxs, key: key, matched: map[string]bool{}}
					merged[sig] = p
				}
				p.matched[matchType] = true
				if matchType == "email" {
					p.key = key // prefer the email as the group key
				}
			}
			for k, idxs := range byEmail {
				add(idxs, k, "email")
			}
			for k, idxs := range byName {
				add(idxs, k, "name")
			}

			groups := make([]dupeGroup, 0, len(merged))
			for _, p := range merged {
				var parts []string
				for _, mt := range []string{"email", "name"} {
					if p.matched[mt] {
						parts = append(parts, mt)
					}
				}
				members := make([]dupeContact, 0, len(p.idxs))
				for _, i := range p.idxs {
					members = append(members, contacts[i])
				}
				sort.Slice(members, func(a, b int) bool { return members[a].ID < members[b].ID })
				groups = append(groups, dupeGroup{
					MatchType: strings.Join(parts, "+"),
					Key:       p.key,
					Count:     len(members),
					Contacts:  members,
				})
			}

			sort.Slice(groups, func(i, j int) bool {
				if groups[i].Count != groups[j].Count {
					return groups[i].Count > groups[j].Count
				}
				return groups[i].Key < groups[j].Key
			})

			return printJSONFiltered(cmd.OutOrStdout(), groups, flags)
		},
	}
	cmd.Flags().StringVar(&flagOrg, "org", "", "Limit to a single organization id (default: across all organizations)")
	return cmd
}
