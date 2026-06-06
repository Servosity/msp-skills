// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: resolve-by-URL / by-name. Parses a Hudu
// object URL or an exact name and assembles the object plus its company, layout,
// and relations from local SQLite joins — the Get-HuduObjectByUrl pattern with
// offline relation assembly no MCP server or PowerShell module ships.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

type resolveRelation struct {
	Description string `json:"description,omitempty"`
	FromType    string `json:"from_type"`
	FromID      int    `json:"from_id"`
	ToType      string `json:"to_type"`
	ToID        int    `json:"to_id"`
}

type resolveMatch struct {
	Kind        string            `json:"kind"`
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Slug        string            `json:"slug,omitempty"`
	URL         string            `json:"url,omitempty"`
	CompanyID   int               `json:"company_id,omitempty"`
	CompanyName string            `json:"company_name,omitempty"`
	LayoutID    int               `json:"layout_id,omitempty"`
	LayoutName  string            `json:"layout_name,omitempty"`
	Relations   []resolveRelation `json:"relations,omitempty"`
}

// resolveView is the machine envelope: it echoes the query so agents (and
// empty results) always carry the input that produced them.
type resolveView struct {
	Query   string         `json:"query"`
	Matches []resolveMatch `json:"matches"`
	Note    string         `json:"note,omitempty"`
}

// resolveKinds maps the user-facing kind to its typed local table and the
// polymorphic type name Hudu uses in relations rows.
var resolveKinds = []struct {
	kind     string
	table    string
	huduType string
	hasURL   bool // table has a promoted url column
	hasSlug  bool // table has a promoted slug column
}{
	{"company", "companies", "Company", false, true},
	{"asset", "assets", "Asset", true, true},
	{"article", "articles", "Article", true, true},
	{"password", "asset_passwords", "AssetPassword", true, false},
	{"website", "websites", "Website", false, false},
}

// parseResolveInput classifies the positional argument. For URLs it returns
// the path (query/fragment stripped) and the last non-empty path segment as a
// slug candidate.
func parseResolveInput(input string) (path, slug string, isURL bool) {
	if !strings.Contains(input, "://") {
		return "", "", false
	}
	u, err := url.Parse(input)
	if err != nil || strings.TrimRight(u.Path, "/") == "" {
		// Unparseable or path-less URL-shaped input: fall back to the
		// name/slug search path instead of dead-ending with zero matches.
		return "", "", false
	}
	p := strings.TrimRight(u.Path, "/")
	segs := strings.Split(p, "/")
	last := ""
	if len(segs) > 0 {
		last = segs[len(segs)-1]
	}
	return p, last, true
}

func newNovelResolveCmd(flags *rootFlags) *cobra.Command {
	var typeFilter string
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "resolve [url-or-name]",
		Short:       "Paste a Hudu object URL or exact name and get the object plus its company, layout, and relations.",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "url-or-name=example-server"},
		Long: `Resolve a single Hudu object from its portal URL or its exact name using the
local mirror (run 'sync' first), and assemble its company, asset layout, and
relations in one shot.

Use this command to resolve a single object from its Hudu URL or exact name and
see its company/layout/relations. Do NOT use it for fuzzy or full-text discovery
across resources; use 'search' for keyword search.`,
		Example: `  # Resolve a pasted portal link
  hudu-cli resolve https://docs.example.huducloud.com/a/dc01-abc123

  # Resolve by exact asset name, as JSON
  hudu-cli resolve "DC01" --agent

  # Only consider knowledge-base articles
  hudu-cli resolve "Onboarding Runbook" --type article`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				if len(args) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "would resolve %q against the local mirror\n", args[0])
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "would resolve a Hudu URL or name against the local mirror")
				}
				return nil
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a Hudu object URL or name is required"))
			}
			input := strings.TrimSpace(args[0])
			if input == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a Hudu object URL or name is required"))
			}
			typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))
			if typeFilter != "" {
				ok := false
				for _, k := range resolveKinds {
					if k.kind == typeFilter {
						ok = true
						break
					}
				}
				if !ok {
					return usageErr(fmt.Errorf("--type must be one of: company, asset, article, password, website"))
				}
			}

			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}
			companyNames := loadCompanyNames(cmd.Context(), db)

			urlPath, slugCand, isURL := parseResolveInput(input)
			matches := []resolveMatch{}
			seen := map[string]bool{}

			collect := func(kind, table string, where string, qargs ...any) {
				if typeFilter != "" && typeFilter != kind {
					return
				}
				if flagLimit > 0 && len(matches) >= flagLimit {
					return
				}
				rows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM "`+table+`" WHERE `+where, qargs...)
				if err != nil {
					return
				}
				for _, raw := range rows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					id := intField(m, "id")
					key := kind + ":" + fmt.Sprint(id)
					if id == 0 || seen[key] {
						continue
					}
					seen[key] = true
					cid := intField(m, "company_id")
					match := resolveMatch{
						Kind: kind, ID: id,
						Name: asString(m["name"]), Slug: asString(m["slug"]),
						URL: asString(m["url"]), CompanyID: cid, CompanyName: companyNames[cid],
					}
					if kind == "company" {
						match.CompanyID = id
						match.CompanyName = match.Name
					}
					if kind == "asset" {
						match.LayoutID = intField(m, "asset_layout_id")
					}
					matches = append(matches, match)
					if flagLimit > 0 && len(matches) >= flagLimit {
						return
					}
				}
			}

			if isURL {
				for _, k := range resolveKinds {
					if k.hasURL && urlPath != "" {
						collect(k.kind, k.table, `url LIKE ?`, "%"+urlPath)
					}
				}
				if len(matches) == 0 && slugCand != "" {
					for _, k := range resolveKinds {
						if k.hasSlug {
							collect(k.kind, k.table, `slug = ?`, slugCand)
						}
					}
				}
			} else {
				for _, k := range resolveKinds {
					collect(k.kind, k.table, `name = ? COLLATE NOCASE`, input)
				}
				if len(matches) == 0 {
					for _, k := range resolveKinds {
						if k.hasSlug {
							collect(k.kind, k.table, `slug = ?`, input)
						}
					}
				}
				if len(matches) == 0 {
					for _, k := range resolveKinds {
						collect(k.kind, k.table, `name LIKE ? COLLATE NOCASE`, "%"+input+"%")
					}
				}
			}

			// Assemble layout names and relations for the resolved objects.
			layoutNames := map[int]string{}
			if rows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM asset_layouts`); err == nil {
				for _, raw := range rows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					layoutNames[intField(m, "id")] = asString(m["name"])
				}
			}
			huduType := map[string]string{}
			for _, k := range resolveKinds {
				huduType[k.kind] = k.huduType
			}
			for i := range matches {
				m := &matches[i]
				if m.LayoutID > 0 {
					m.LayoutName = layoutNames[m.LayoutID]
				}
				rows, err := queryDataRows(cmd.Context(), db,
					`SELECT data FROM relations WHERE (fromable_type = ? AND fromable_id = ?) OR (toable_type = ? AND toable_id = ?)`,
					huduType[m.Kind], m.ID, huduType[m.Kind], m.ID)
				if err != nil {
					continue
				}
				for _, raw := range rows {
					var r map[string]any
					if json.Unmarshal(raw, &r) != nil {
						continue
					}
					m.Relations = append(m.Relations, resolveRelation{
						Description: asString(r["description"]),
						FromType:    asString(r["fromable_type"]),
						FromID:      intField(r, "fromable_id"),
						ToType:      asString(r["toable_type"]),
						ToID:        intField(r, "toable_id"),
					})
				}
			}

			view := resolveView{Query: input, Matches: matches}
			if len(matches) == 0 {
				view.Note = "no local object matches; run 'hudu-cli sync' first, or use 'hudu-cli search' for fuzzy discovery"
			}
			return emitAudit(cmd, flags, view, func(w io.Writer) {
				if len(matches) == 0 {
					fmt.Fprintf(w, "No local object matches %q. Run 'hudu-cli sync' first, or use 'hudu-cli search' for fuzzy discovery.\n", input)
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "KIND\tID\tNAME\tCOMPANY\tLAYOUT\tRELATIONS")
				for _, m := range matches {
					fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%d\n", m.Kind, m.ID, m.Name, m.CompanyName, m.LayoutName, len(m.Relations))
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Restrict matches to one kind: company, asset, article, password, website")
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Maximum matches to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
