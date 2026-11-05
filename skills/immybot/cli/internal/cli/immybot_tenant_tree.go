// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored shared plumbing for the ImmyBot novel commands: the mirrored
// tenant hierarchy. Kept in its own file so `generate --force` preserves it.
//
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"
)

// immyTenantTree is the parent link of every mirrored tenant
// (GetTenantResponse.parentTenantId; null for a top-level tenant). A
// deployment bound to a tenant with propagateToChildTenants true
// (LocalTargetAssignmentResource / GlobalTargetAssignmentResource) also
// reaches every descendant of that tenant, which only this hierarchy can name.
type immyTenantTree struct {
	parent map[string]string // tenant id -> parent id, "" for a top-level tenant
}

// immyLoadTenantTree reads the hierarchy from the mirrored tenants. A mirror
// without tenants yields an empty tree, under which no ancestry is provable.
func immyLoadTenantTree(ctx context.Context, db *store.Store) (immyTenantTree, error) {
	tree := immyTenantTree{parent: map[string]string{}}
	rows, err := db.DB().QueryContext(ctx,
		`SELECT id, json_extract(data,'$.parentTenantId') FROM resources WHERE resource_type = 'tenants'`)
	if err != nil {
		if immyNoSuchTableErr(err) {
			return tree, nil
		}
		return tree, fmt.Errorf("querying tenants: %w", err)
	}
	for rows.Next() {
		var id, parent sql.NullString
		if err := rows.Scan(&id, &parent); err != nil {
			_ = rows.Close()
			return tree, fmt.Errorf("scanning tenant: %w", err)
		}
		if key := strings.TrimSpace(nullStr(id)); key != "" {
			tree.parent[key] = strings.TrimSpace(nullStr(parent))
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return tree, fmt.Errorf("iterating tenants: %w", err)
	}
	return tree, rows.Close()
}

type immyAncestry int

const (
	immyAncestryNo immyAncestry = iota
	immyAncestryYes
	// immyAncestryUnknown: the mirror lacks a tenant row on the parent chain
	// (or the chain loops), so membership can be neither proven nor ruled out.
	immyAncestryUnknown
)

// within reports whether tenant is root or one of root's descendants, walking
// tenant's parent chain to the top. It is No only when the whole chain is
// mirrored and ends without passing root; a missing row or a loop is Unknown,
// and gap names the tenant the walk could not get past.
func (t immyTenantTree) within(tenant, root string) (verdict immyAncestry, gap string) {
	tenant, root = strings.TrimSpace(tenant), strings.TrimSpace(root)
	if tenant == "" || root == "" {
		return immyAncestryNo, ""
	}
	seen := map[string]bool{}
	for cur := tenant; ; {
		if cur == root {
			return immyAncestryYes, ""
		}
		if seen[cur] {
			return immyAncestryUnknown, cur
		}
		seen[cur] = true
		p, ok := t.parent[cur]
		if !ok {
			return immyAncestryUnknown, cur
		}
		if p == "" {
			return immyAncestryNo, ""
		}
		cur = p
	}
}

// ids lists every mirrored tenant id.
func (t immyTenantTree) ids() []string {
	out := make([]string, 0, len(t.parent))
	for id := range t.parent {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (t immyTenantTree) empty() bool { return len(t.parent) == 0 }

// immyTenantsSyncHint is the remedy every propagation gap names.
const immyTenantsSyncHint = "run 'immybot-cli sync --resources tenants'"

// gapReason explains why a rule propagating from root cannot be placed on
// tenant (gap is what within returned).
func (t immyTenantTree) gapReason(root, tenant, gap string) string {
	if t.empty() {
		return fmt.Sprintf("propagates to the child tenants of tenant %s, and the local mirror holds no tenants, "+
			"so it cannot tell whether tenant %s is one; %s", root, tenant, immyTenantsSyncHint)
	}
	return fmt.Sprintf("propagates to the child tenants of tenant %s, and the local mirror cannot follow tenant %s's "+
		"parent chain past tenant %s, so it cannot tell whether tenant %s is one; %s", root, tenant, gap, tenant, immyTenantsSyncHint)
}

// immyListSome joins ids for a note, naming at most limit of them.
func immyListSome(ids []string, limit int) string {
	if len(ids) <= limit {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ids[:limit], ", "), len(ids)-limit)
}
