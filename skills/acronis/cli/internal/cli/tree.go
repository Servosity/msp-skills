// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type treeNode struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"`
	AgentCount int         `json:"agent_count"`
	UserCount  int         `json:"user_count"`
	Children   []*treeNode `json:"children,omitempty"`
}

type tenantInfo struct {
	id, name, kind, parentID string
}

// buildTenantForest reads tenants + per-tenant agent/user counts and returns
// the root nodes of the hierarchy.
func buildTenantForest(db *store.Store) ([]*treeNode, error) {
	rows, err := db.Query(`SELECT id, name, kind, parent_id FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("querying tenants: %w", err)
	}
	defer rows.Close()

	var infos []tenantInfo
	idSet := map[string]bool{}
	for rows.Next() {
		var id string
		var name, kind, parentID *string
		if rows.Scan(&id, &name, &kind, &parentID) != nil {
			continue
		}
		infos = append(infos, tenantInfo{id: id, name: deref(name), kind: deref(kind), parentID: deref(parentID)})
		idSet[id] = true
	}

	agentCounts := countBy(db, `SELECT tenant_id, COUNT(*) FROM agent_manager GROUP BY tenant_id`)
	userCounts := countBy(db, `SELECT tenants_id, COUNT(*) FROM users GROUP BY tenants_id`)

	nodes := map[string]*treeNode{}
	for _, in := range infos {
		nodes[in.id] = &treeNode{
			ID:         in.id,
			Name:       in.name,
			Kind:       in.kind,
			AgentCount: agentCounts[in.id],
			UserCount:  userCounts[in.id],
		}
	}

	var roots []*treeNode
	for _, in := range infos {
		n := nodes[in.id]
		if in.parentID == "" || !idSet[in.parentID] {
			roots = append(roots, n)
			continue
		}
		p := nodes[in.parentID]
		p.Children = append(p.Children, n)
	}

	var sortChildren func(n *treeNode)
	sortChildren = func(n *treeNode) {
		sort.Slice(n.Children, func(i, j int) bool {
			if n.Children[i].Name != n.Children[j].Name {
				return n.Children[i].Name < n.Children[j].Name
			}
			return n.Children[i].ID < n.Children[j].ID
		})
		for _, c := range n.Children {
			sortChildren(c)
		}
	}
	for _, r := range roots {
		sortChildren(r)
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Name != roots[j].Name {
			return roots[i].Name < roots[j].Name
		}
		return roots[i].ID < roots[j].ID
	})
	return roots, nil
}

func countBy(db *store.Store, query string) map[string]int {
	out := map[string]int{}
	rows, err := db.Query(query)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var key *string
		var c int
		if rows.Scan(&key, &c) == nil {
			out[deref(key)] = c
		}
	}
	return out
}

func renderTree(w *strings.Builder, n *treeNode, level, maxDepth int) {
	if maxDepth > 0 && level >= maxDepth {
		return
	}
	indent := strings.Repeat("  ", level)
	name := n.Name
	if name == "" {
		name = n.ID
	}
	kind := n.Kind
	if kind == "" {
		kind = "?"
	}
	fmt.Fprintf(w, "%s%s (%s) — agents=%d users=%d\n", indent, name, kind, n.AgentCount, n.UserCount)
	for _, c := range n.Children {
		renderTree(w, c, level+1, maxDepth)
	}
}

// pp:data-source local
func newNovelTreeCmd(flags *rootFlags) *cobra.Command {
	var flagDepth string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "tree",
		Short:       "Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			maxDepth := 0
			if flagDepth != "" {
				d, err := strconv.Atoi(flagDepth)
				if err != nil || d < 0 {
					return fmt.Errorf("invalid --depth %q: want a non-negative integer", flagDepth)
				}
				maxDepth = d
			}

			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			roots, err := buildTenantForest(db)
			if err != nil {
				return err
			}

			if wantJSON(flags, cmd) {
				if roots == nil {
					roots = []*treeNode{}
				}
				return encodeJSON(cmd, flags, roots)
			}
			if len(roots) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tenants — run 'acronis-cli sync' first.")
				return nil
			}
			var sb strings.Builder
			for _, r := range roots {
				renderTree(&sb, r, 0, maxDepth)
			}
			fmt.Fprint(cmd.OutOrStdout(), sb.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDepth, "depth", "", "Maximum depth to render (default: unlimited)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	return cmd
}
