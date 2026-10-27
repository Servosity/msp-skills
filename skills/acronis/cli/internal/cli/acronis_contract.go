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

// acronisTaskPartitions maps each task-manager list endpoint to the sync
// partition that mirrors it (sync.go syncResourcePath).
var acronisTaskPartitions = map[string]string{
	"/api/task_manager/v2/tasks":      "task-manager",
	"/api/task_manager/v2/activities": "task-manager-v2-activities",
}

// acronisPartitionEndpoints lists, for each partition sync mirrors, the
// endpoints whose answers are that partition's rows: the list path and the
// detail path ("*" is one path segment). It is the one table that picks the
// partition of every store read and write outside sync: the read resolvers,
// their local fallback, write-through and mutation answers (hand-fix
// handfixes.json read-partition-by-endpoint). The rollups trust each partition
// to hold one endpoint's rows, so an activity, a hardware node or a settings
// object never lands in the tasks or agents partition.
var acronisPartitionEndpoints = map[string][]string{
	"clients":                    {"api/2/clients", "api/2/clients/*"},
	"tenants":                    {"api/2/tenants", "api/2/tenants/*"},
	"users":                      {"api/2/tenants/*/users"},
	"usages":                     {"api/2/tenants/*/usages"},
	"offering_items":             {"api/2/tenants/*/offering_items"},
	"agent-manager":              {"api/agent_manager/v2/agents", "api/agent_manager/v2/agents/*"},
	"task-manager":               {"api/task_manager/v2/tasks", "api/task_manager/v2/tasks/*"},
	"task-manager-v2-activities": {"api/task_manager/v2/activities", "api/task_manager/v2/activities/*"},
}

// acronisPartition returns the local partition for a store read or write of
// path, which the command labelled label. The endpoint decides: a listed path
// maps to its partition. Any other path keeps a label of its own (reports,
// remote_search) but never a mirrored one: it gets "", no partition, so
// nothing is written and no local read answers it.
func acronisPartition(label, path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for partition, patterns := range acronisPartitionEndpoints {
		for _, pattern := range patterns {
			if acronisPathMatches(strings.Split(pattern, "/"), segs) {
				return partition
			}
		}
	}
	if _, mirrored := acronisPartitionEndpoints[strings.ReplaceAll(label, "-", "_")]; mirrored {
		return ""
	}
	if _, mirrored := acronisPartitionEndpoints[label]; mirrored {
		return ""
	}
	return label
}

// acronisPathMatches: "*" matches one id segment. An id never holds ':', so
// a custom method (agents/update:force) is not a detail path.
func acronisPathMatches(pattern, segs []string) bool {
	if len(pattern) != len(segs) {
		return false
	}
	for i, p := range pattern {
		if p == "*" {
			if segs[i] == "" || strings.Contains(segs[i], ":") {
				return false
			}
		} else if p != segs[i] {
			return false
		}
	}
	return true
}

// resolveAcronisTaskRead serves a task or activity list. The endpoint path,
// never the caller's resource label, selects the local partition, so an
// activities read cannot be answered from the tasks mirror.
func resolveAcronisTaskRead(ctx context.Context, c *client.Client, flags *rootFlags, _ string, path string, params map[string]string, all bool, hints io.Writer) (json.RawMessage, DataProvenance, error) {
	resource, ok := acronisTaskPartitions[path]
	if !ok {
		return nil, DataProvenance{}, fmt.Errorf("unsupported task list path %q", path)
	}
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
		if isDryRunResponseForClient(c, data) {
			return data, nil
		}
		// Every page, the single-page read included, must pass the shared
		// validator: an in-band failure is never returned as data.
		if err := acronisValidatePage(data); err != nil {
			return nil, err
		}
		if !all {
			return data, nil
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
	if _, ok := acronisTaskPartitions[path]; !ok {
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

// acronisPageFailureKeys name in-band failure or partial-data signals. Any of
// them carrying a non-empty value (a message, a list, an object, true, a
// non-zero number) at the top level or inside any top-level object other than
// "items" (paging, meta, pagination, ...) marks the page as not a complete,
// successful enumeration.
var acronisPageFailureKeys = []string{
	"error", "errors", "warning", "warnings", "failure", "failures",
	"partial", "incomplete", "truncated",
}

// acronisJSONValueEmpty reports whether raw is absent, null, false, 0, "",
// [] or {}.
func acronisJSONValueEmpty(raw json.RawMessage) bool {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return len(raw) == 0
	}
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case float64:
		return x == 0
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// acronisPageSignalsFailure reports any in-band failure signal: the generated
// success/ok/status detector (on the page and on every top-level object), or a
// non-empty acronisPageFailureKeys value at the top level or inside any
// top-level object other than "items". Any signal wins.
func acronisPageSignalsFailure(data json.RawMessage, envelope map[string]json.RawMessage) bool {
	if responseDeclaresFailure(data) {
		return true
	}
	containers := []map[string]json.RawMessage{envelope}
	for key, raw := range envelope {
		if key == "items" {
			continue
		}
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil && nested != nil {
			if envelopeReportsFailure(nested) {
				return true
			}
			containers = append(containers, nested)
		}
	}
	for _, container := range containers {
		for _, key := range acronisPageFailureKeys {
			if raw, ok := container[key]; ok && !acronisJSONValueEmpty(raw) {
				return true
			}
		}
	}
	return false
}

func acronisValidatePage(data json.RawMessage) error {
	var items []json.RawMessage
	if json.Unmarshal(data, &items) == nil && items != nil {
		return nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) == nil && json.Unmarshal(envelope["items"], &items) == nil && items != nil {
		if acronisPageSignalsFailure(data, envelope) {
			return fmt.Errorf("incomplete response: the API reported a failure or partial result in-band")
		}
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
			next, _ := extractPaginationFromEnvelope(envelope, "after", "")
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
