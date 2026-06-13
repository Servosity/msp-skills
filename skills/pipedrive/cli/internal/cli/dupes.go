// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: duplicate detection. Hand-authored against the local store.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type dupeMember struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type dupeCluster struct {
	Key     string       `json:"key"`
	Size    int          `json:"size"`
	Members []dupeMember `json:"members"`
}

type dupesResult struct {
	Entity           string        `json:"entity"`
	ClusterCount     int           `json:"cluster_count"`
	DuplicateRecords int           `json:"duplicate_records"`
	Clusters         []dupeCluster `json:"clusters"`
}

// pp:data-source local
func newNovelDupesCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var dbPath string
	var entityFlag string

	cmd := &cobra.Command{
		Use:   "dupes [persons|organizations]",
		Short: "Find likely-duplicate persons or organizations by normalized name, email, and phone.",
		Long: `Scans the whole local contact set and clusters likely duplicates. Persons are
clustered by normalized name (punctuation and company suffixes stripped),
primary email, and primary phone; organizations by normalized name. Defaults to
persons.

Choose the entity with --entity (recommended) or as a positional argument.
Reads the local store, so run 'pipedrive-cli sync' first.`,
		Example: strings.Trim(`
  pipedrive-cli dupes --entity persons
  pipedrive-cli dupes --entity organizations --json
  pipedrive-cli dupes --entity persons --limit 20 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			// Entity from --entity (preferred), else the positional argument,
			// else default to persons.
			raw := entityFlag
			if raw == "" && len(args) > 0 {
				raw = args[0]
			}
			entity := "persons"
			if raw != "" {
				switch strings.ToLower(raw) {
				case "persons", "person", "people":
					entity = "persons"
				case "organizations", "organization", "orgs", "org":
					entity = "organizations"
				default:
					return fmt.Errorf("dupes target must be 'persons' or 'organizations' (got %q)", raw)
				}
			}
			return runDupes(cmd, flags, entity, limit, dbPath)
		},
	}
	cmd.Flags().StringVar(&entityFlag, "entity", "", "Entity to dedupe: persons (default) or organizations")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum clusters to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

// runDupes clusters likely-duplicate records of one entity using a union-find
// over normalized name and (for persons) primary email + phone.
func runDupes(cmd *cobra.Command, flags *rootFlags, entity string, limit int, dbPath string) error {
	db, err := pdOpenStore(cmd.Context(), dbPath)
	if err != nil {
		return fmt.Errorf("opening local store: %w", err)
	}
	defer db.Close()

	if !hintIfUnsynced(cmd, db, "persons") {
		hintIfStale(cmd, db, "persons", flags.maxAge)
	}

	rows, err := db.DB().QueryContext(cmd.Context(),
		fmt.Sprintf("SELECT id, COALESCE(name,''), COALESCE(data,'') FROM %q", entity))
	if err != nil {
		return fmt.Errorf("querying %s: %w", entity, err)
	}
	defer rows.Close()

	uf := newUnionFind()
	members := map[string]dupeMember{}
	firstByKey := map[string]string{} // clustering key -> first id seen
	var order []string

	for rows.Next() {
		var id, name, data string
		if err := rows.Scan(&id, &name, &data); err != nil {
			return fmt.Errorf("scanning %s: %w", entity, err)
		}
		m := dupeMember{ID: id, Name: name}
		if entity == "persons" {
			m.Email = extractPrimary(data, "email")
			m.Phone = extractPrimary(data, "phone")
		}
		members[id] = m
		uf.add(id)
		order = append(order, id)

		// Union by normalized name and (for persons) by email + phone.
		if nk := pipeintel.NormalizeName(name); nk != "" {
			linkKey(uf, firstByKey, "name:"+nk, id)
		}
		if m.Email != "" {
			linkKey(uf, firstByKey, "email:"+strings.ToLower(m.Email), id)
		}
		if pk := pipeintel.NormalizePhone(m.Phone); pk != "" {
			linkKey(uf, firstByKey, "phone:"+pk, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Group members by their union-find root, preserving first-seen order.
	groups := map[string][]dupeMember{}
	var rootOrder []string
	seenRoot := map[string]bool{}
	for _, id := range order {
		r := uf.find(id)
		if !seenRoot[r] {
			seenRoot[r] = true
			rootOrder = append(rootOrder, r)
		}
		groups[r] = append(groups[r], members[id])
	}

	res := dupesResult{Entity: entity, Clusters: []dupeCluster{}}
	for _, r := range rootOrder {
		g := groups[r]
		if len(g) < 2 {
			continue
		}
		key := pipeintel.NormalizeName(g[0].Name)
		if key == "" {
			key = g[0].Name
		}
		res.Clusters = append(res.Clusters, dupeCluster{Key: key, Size: len(g), Members: g})
		res.DuplicateRecords += len(g)
	}
	sort.SliceStable(res.Clusters, func(i, j int) bool { return res.Clusters[i].Size > res.Clusters[j].Size })
	res.ClusterCount = len(res.Clusters)
	if limit > 0 && len(res.Clusters) > limit {
		res.Clusters = res.Clusters[:limit]
	}

	return emitNovel(cmd, flags, res, func(w io.Writer) {
		if res.ClusterCount == 0 {
			fmt.Fprintf(w, "No likely-duplicate %s found. (Run 'sync' if the local store is empty.)\n", entity)
			return
		}
		fmt.Fprintf(w, "%d duplicate cluster(s) covering %d %s:\n\n", res.ClusterCount, res.DuplicateRecords, entity)
		for _, c := range res.Clusters {
			fmt.Fprintf(w, "• %s (%d)\n", c.Key, c.Size)
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			for _, m := range c.Members {
				fmt.Fprintf(tw, "    id=%s\t%s\t%s\t%s\n", m.ID, truncateRunes(m.Name, 32), m.Email, m.Phone)
			}
			_ = tw.Flush()
		}
	})
}

// extractPrimary pulls the primary (or first) value of a Pipedrive contact
// field (email/phone) from a person's JSON data blob. Pipedrive encodes these
// as an array of {value,primary} objects, an array of strings, or a bare
// string; this handles all three and returns "" if absent.
func extractPrimary(data, field string) string {
	if data == "" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return ""
	}
	raw, ok := obj[field]
	if !ok {
		return ""
	}
	// Array of objects.
	var arrObj []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(raw, &arrObj); err == nil && len(arrObj) > 0 {
		for _, e := range arrObj {
			if e.Primary && e.Value != "" {
				return e.Value
			}
		}
		return arrObj[0].Value
	}
	// Array of strings.
	var arrStr []string
	if err := json.Unmarshal(raw, &arrStr); err == nil && len(arrStr) > 0 {
		return arrStr[0]
	}
	// Bare string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// linkKey unions id with the first id previously seen under clustering key k.
func linkKey(uf *unionFind, firstByKey map[string]string, k, id string) {
	if prev, ok := firstByKey[k]; ok {
		uf.union(prev, id)
	} else {
		firstByKey[k] = id
	}
}

// unionFind is a tiny disjoint-set over string ids for clustering duplicates.
type unionFind struct{ parent map[string]string }

func newUnionFind() *unionFind { return &unionFind{parent: map[string]string{}} }

func (u *unionFind) add(x string) {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
}

func (u *unionFind) find(x string) string {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}
