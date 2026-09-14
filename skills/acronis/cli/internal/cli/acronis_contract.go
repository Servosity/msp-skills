// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"acronis-pp-cli/internal/client"
	"acronis-pp-cli/internal/store"
)

// acronisRootTenant asks the authenticated API which tenant owns this client.
// Never choose the first tenant in a partner's accessible tenant collection.
func acronisRootTenant(ctx context.Context, c *client.Client) (string, error) {
	path := "/api/2/users/me"
	clientID := os.Getenv("ACRONIS_CLIENT_ID")
	if clientID == "" && c.Config != nil {
		clientID = c.Config.ClientID
	}
	if clientID != "" {
		path = "/api/2/clients/" + url.PathEscape(clientID)
	}
	data, err := c.Get(ctx, path, nil)
	if err != nil {
		return "", fmt.Errorf("resolve root tenant: %w; supply an explicit tenant scope", err)
	}
	var obj struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil || obj.TenantID == "" {
		return "", fmt.Errorf("root tenant missing from authenticated identity; supply an explicit tenant scope")
	}
	return obj.TenantID, nil
}

// Acronis documents paging.cursors in its guide and cursors in its OpenAPI.
// An existing empty cursor object is an explicit terminal page, even when full.
func acronisPageCursor(obj map[string]json.RawMessage) (string, bool) {
	for _, key := range []string{"paging", ""} {
		container := obj
		if key != "" {
			if json.Unmarshal(obj[key], &container) != nil {
				continue
			}
		}
		var cursors map[string]json.RawMessage
		if raw, ok := container["cursors"]; ok && json.Unmarshal(raw, &cursors) == nil && cursors != nil {
			var after string
			_ = json.Unmarshal(cursors["after"], &after)
			return after, true
		}
	}
	return "", false
}

func acronisTaskOrder(order string) string {
	fields := strings.Fields(order)
	if len(fields) == 2 && (strings.EqualFold(fields[1], "asc") || strings.EqualFold(fields[1], "desc")) {
		return strings.ToLower(fields[1]) + "(" + fields[0] + ")"
	}
	return order
}

func resolveAcronisTaskRead(ctx context.Context, c *client.Client, flags *rootFlags, resource, path string, params map[string]string, all bool, hints io.Writer) (json.RawMessage, DataProvenance, error) {
	if flags.dataSource == "local" {
		if params["tenant_id"] != "" {
			return nil, DataProvenance{}, usageErr(fmt.Errorf("--tenant-id requires live enumeration; use --data-source live"))
		}
		data, prov, err := resolveLocal(ctx, flags, hints, resource, true, path, params, "user_requested")
		return data, attachFreshness(prov, flags), err
	}
	tenant := params["tenant_id"]
	clean := map[string]string{}
	names := map[string]string{"result_code": "resultCode", "policy_id": "policyId", "resource_id": "resourceId", "task_id": "taskId"}
	for k, v := range params {
		if k == "tenant_id" || v == "" || v == "0" {
			continue
		}
		if name, ok := names[k]; ok {
			k = name
		}
		if k == "order" {
			v = acronisTaskOrder(v)
		}
		clean[k] = v
	}
	if tenant != "" && clean["after"] != "" {
		return nil, DataProvenance{}, usageErr(fmt.Errorf("--tenant-id scans all pages; omit --after"))
	}
	if n, err := strconv.Atoi(clean["limit"]); err != nil || n < 1 {
		return nil, DataProvenance{}, usageErr(fmt.Errorf("--limit must be positive"))
	}
	data, err := acronisTaskPages(ctx, c, path, clean, all || tenant != "", tenant)
	if err != nil {
		return nil, DataProvenance{}, err
	}
	return data, attachFreshness(DataProvenance{Source: "live"}, flags), nil
}

type acronisGetter interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
}

func acronisTaskPages(ctx context.Context, c acronisGetter, path string, params map[string]string, all bool, tenant string) (json.RawMessage, error) {
	items := make([]json.RawMessage, 0)
	seen := map[string]bool{}
	if params["after"] != "" {
		seen[params["after"]] = true
	}
	limit, _ := strconv.Atoi(params["limit"])
	for page := 0; page < paginatedGetMaxPages; page++ {
		data, err := c.Get(ctx, path, params)
		if err != nil {
			return nil, err
		}
		if isDryRunResponse(data) {
			return data, nil
		}
		if !all {
			return data, nil
		}
		if err := acronisValidatePage(data); err != nil {
			return nil, err
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(data, &envelope) != nil {
			return nil, fmt.Errorf("invalid task response")
		}
		var batch []json.RawMessage
		if json.Unmarshal(envelope["items"], &batch) != nil || batch == nil {
			return nil, fmt.Errorf("task response missing items array")
		}
		for _, item := range batch {
			if tenant != "" {
				obj, err := store.DecodeJSONObject(item)
				if err != nil {
					return nil, err
				}
				id := store.LookupFieldValue(obj, "tenant_id")
				if id == nil {
					return nil, fmt.Errorf("cannot filter task with missing tenant identity")
				}
				if fmt.Sprint(id) != tenant {
					continue
				}
			}
			items = append(items, item)
		}
		next, known := acronisPageCursor(envelope)
		if next == "" {
			if !known && len(batch) >= limit {
				return nil, fmt.Errorf("task results incomplete: full page without pagination cursor")
			}
			return json.Marshal(items)
		}
		if seen[next] {
			return nil, fmt.Errorf("task results incomplete: repeated pagination cursor")
		}
		seen[next] = true
		// The cursor carries filters/order. Continuations send only limit and after.
		params = map[string]string{"limit": params["limit"], "after": next}
	}
	return nil, fmt.Errorf("task results incomplete: pagination page limit reached")
}

func acronisSetMirrorState(db *store.Store, complete bool) error {
	if _, err := db.DB().Exec(`CREATE TABLE IF NOT EXISTS acronis_mirror_state (id INTEGER PRIMARY KEY CHECK(id=1), complete INTEGER NOT NULL)`); err != nil {
		return err
	}
	_, err := db.DB().Exec(`INSERT INTO acronis_mirror_state(id,complete) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET complete=excluded.complete`, complete)
	return err
}

func acronisRequireCompleteMirror(db *store.Store) error {
	var complete int
	if err := db.DB().QueryRow(`SELECT complete FROM acronis_mirror_state WHERE id=1`).Scan(&complete); err != nil || complete != 1 {
		return fmt.Errorf("local mirror is incomplete or predates completeness tracking; run 'acronis-cli sync --full' successfully before using rollups")
	}
	// Agent API versions can return legacy numeric tenant IDs. Refuse a misleading
	// zero-agent rollup until those identities can be mapped to account UUIDs.
	var unmatched int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM agent_manager a LEFT JOIN tenants t ON a.tenant_id=t.id WHERE t.id IS NULL`).Scan(&unmatched); err != nil {
		return err
	}
	if unmatched > 0 {
		return fmt.Errorf("%d agents have tenant identities absent from the tenant mirror; tenant rollups are incomplete", unmatched)
	}
	return nil
}

// AcronisTaskRead shares task parameter translation with the direct MCP tools.
func AcronisTaskRead(ctx context.Context, c *client.Client, path string, params map[string]string) (json.RawMessage, error) {
	if path != "/api/task_manager/v2/tasks" && path != "/api/task_manager/v2/activities" {
		return nil, fmt.Errorf("unsupported task list path")
	}
	if params["limit"] == "" {
		params["limit"] = "100"
	}
	data, _, err := resolveAcronisTaskRead(ctx, c, &rootFlags{dataSource: "live"}, "task-manager", path, params, params["after"] == "", io.Discard)
	return data, err
}

// AcronisSearchRead preserves public MCP names while using the account API contract.
func AcronisSearchRead(ctx context.Context, c *client.Client, params map[string]string) (json.RawMessage, error) {
	text := params["query"]
	tenant := params["tenant_id"]
	limit := params["limit"]
	if limit == "" {
		limit = "10"
	}
	n, err := strconv.Atoi(limit)
	if len([]rune(text)) < 3 || err != nil || n < 1 || n > 300 {
		return nil, fmt.Errorf("search requires at least 3 characters and limit between 1 and 300")
	}
	if tenant == "" {
		tenant, err = acronisRootTenant(ctx, c)
		if err != nil {
			return nil, err
		}
	}
	return c.Get(ctx, "/api/2/search", map[string]string{"text": text, "tenant": tenant, "limit": limit})
}

func acronisValidatePage(data json.RawMessage) error {
	var items []json.RawMessage
	if json.Unmarshal(data, &items) == nil && items != nil {
		return nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) == nil && json.Unmarshal(envelope["items"], &items) == nil && items != nil {
		for _, key := range []string{"paging", ""} {
			container := envelope
			if key != "" {
				raw, exists := envelope[key]
				if !exists {
					continue
				}
				if json.Unmarshal(raw, &container) != nil || container == nil {
					return fmt.Errorf("malformed paging object")
				}
			}
			if raw, exists := container["cursors"]; exists {
				var cursors map[string]json.RawMessage
				if json.Unmarshal(raw, &cursors) != nil || cursors == nil {
					return fmt.Errorf("malformed cursors object")
				}
				if after, exists := cursors["after"]; exists {
					var cursor string
					if len(after) == 0 || after[0] != '"' || json.Unmarshal(after, &cursor) != nil {
						return fmt.Errorf("malformed after cursor")
					}
				}
			}
		}
		if more, known := pageExplicitHasMore(data); known && more {
			next, _ := extractPaginationFromEnvelope(envelope, "after")
			if next == "" {
				return fmt.Errorf("incomplete response: more data declared without a cursor")
			}
		}
		return nil
	}
	return fmt.Errorf("incomplete sync: expected an items array in the API response")
}

// acronisLockMirror serializes whole runs, including independent MCP processes.
// O_EXCL is supported on every release platform. A crash leaves a conservative
// stale lock; operators must verify no sync is active before removing it.
func acronisLockMirror(dbPath string) (func(), error) {
	canonical, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		return nil, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, err
	}
	lockPath := canonical + ".sync.lock"
	directory, err := os.OpenRoot(filepath.Dir(canonical))
	if err != nil {
		return nil, err
	}
	name := filepath.Base(canonical) + ".sync.lock"
	file, err := directory.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("mirror is locked or lock cannot be created: %w; after an interrupted sync, verify no sync is running before removing %s", err, lockPath)
	}
	if err := file.Close(); err != nil {
		_ = directory.Remove(name)
		_ = directory.Close()
		return nil, err
	}
	return func() { _ = directory.Remove(name); _ = directory.Close() }, nil
}
