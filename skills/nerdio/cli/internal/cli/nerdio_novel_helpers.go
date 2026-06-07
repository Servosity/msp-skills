// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature helpers for the Nerdio Manager for MSP CLI.
// Lives in its own file (not a generated one) so regen-merge preserves it.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nerdio-pp-cli/internal/client"
	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// nmmJobPath is the Partner API job endpoint all async mutations report into.
const nmmJobPath = "/rest-api/v1/job/"

// fleetAccount is one NMM customer account targeted by a fleet fan-out.
type fleetAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// decodeObjects decodes an API payload into a list of generic objects.
// NMM list endpoints usually return bare arrays; some wrap in {items:[...]}
// or {data:[...]}. Single objects come back as a one-element list. Numbers
// stay json.RawMessage so cliutil.ExtractInt/ExtractNumber handle both
// JSON numbers and string-encoded numbers without float64 precision loss.
func decodeObjects(data json.RawMessage) []map[string]json.RawMessage {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	for _, key := range []string{"items", "data", "results", "value"} {
		if raw, ok := obj[key]; ok {
			var inner []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &inner); err == nil {
				return inner
			}
		}
	}
	return []map[string]json.RawMessage{obj}
}

// extractStringAny returns the first non-empty string value among keys.
// Key matching is case-insensitive so accountId/AccountID/accountid all hit.
func extractStringAny(obj map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, key := range keys {
		for objKey, raw := range obj {
			if !strings.EqualFold(objKey, key) {
				continue
			}
			var s string
			if err := json.Unmarshal(raw, &s); err == nil && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

// extractIntAny returns the first extractable integer among keys
// (case-insensitive), accepting JSON numbers and string-encoded numbers.
func extractIntAny(obj map[string]json.RawMessage, keys ...string) (int64, bool) {
	for _, key := range keys {
		for objKey, raw := range obj {
			if !strings.EqualFold(objKey, key) {
				continue
			}
			if v, ok := cliutil.ExtractInt(map[string]json.RawMessage{key: raw}, key); ok {
				return v, true
			}
		}
	}
	return 0, false
}

// extractNumberAny returns the first extractable float among keys
// (case-insensitive), accepting JSON numbers and string-encoded numbers.
func extractNumberAny(obj map[string]json.RawMessage, keys ...string) (float64, bool) {
	for _, key := range keys {
		for objKey, raw := range obj {
			if !strings.EqualFold(objKey, key) {
				continue
			}
			if v, ok := cliutil.ExtractNumber(map[string]json.RawMessage{key: raw}, key); ok {
				return v, true
			}
		}
	}
	return 0, false
}

// extractBoolAny returns the first boolean value among keys (case-insensitive).
func extractBoolAny(obj map[string]json.RawMessage, keys ...string) (bool, bool) {
	for _, key := range keys {
		for objKey, raw := range obj {
			if !strings.EqualFold(objKey, key) {
				continue
			}
			var b bool
			if err := json.Unmarshal(raw, &b); err == nil {
				return b, true
			}
		}
	}
	return false, false
}

// parsePeriod splits a "YYYY-MM-DD:YYYY-MM-DD" period flag into start/end.
func parsePeriod(s string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("period must be <start>:<end> (e.g. 2026-05-01:2026-05-31), got %q", s)
	}
	for _, p := range parts {
		if _, err := time.Parse("2006-01-02", p); err != nil {
			return "", "", fmt.Errorf("period date %q must be YYYY-MM-DD: %w", p, err)
		}
	}
	return parts[0], parts[1], nil
}

// parseAccountsCSV parses a --accounts CSV of NMM account IDs.
func parseAccountsCSV(s string) ([]fleetAccount, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []fleetAccount
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		id, err := strconv.ParseInt(tok, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--accounts entries must be numeric NMM account IDs, got %q", tok)
		}
		out = append(out, fleetAccount{ID: id, Name: fmt.Sprintf("account-%d", id)})
	}
	return out, nil
}

// resolveFleetAccounts resolves the account set for a fleet fan-out.
// Explicit --accounts wins; otherwise the synced local store is consulted
// (with the standard sync hints); otherwise the live /accounts endpoint.
func resolveFleetAccounts(ctx context.Context, cmd *cobra.Command, flags *rootFlags, c *client.Client, accountsCSV string) ([]fleetAccount, error) {
	if explicit, err := parseAccountsCSV(accountsCSV); err != nil {
		return nil, usageErr(err)
	} else if len(explicit) > 0 {
		return explicit, nil
	}
	if local := fleetAccountsFromStore(ctx, cmd, flags); len(local) > 0 {
		return local, nil
	}
	data, err := c.Get(ctx, "/rest-api/v1/accounts", nil)
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	accounts := fleetAccountsFromObjects(decodeObjects(data))
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts found: pass --accounts <csv> or run 'nerdio-cli sync --resources accounts' first")
	}
	return accounts, nil
}

// fleetAccountsFromStore loads synced accounts from the local SQLite store.
// Returns nil (never errors) when the store is missing or empty so callers
// can fall through to the live API.
func fleetAccountsFromStore(ctx context.Context, cmd *cobra.Command, flags *rootFlags) []fleetAccount {
	dbPath := defaultDBPath("nerdio-cli")
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return nil
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, "accounts") {
		hintIfStale(cmd, db, "accounts", 0)
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = 'accounts'`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var objs []map[string]json.RawMessage
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(blob), &obj); err == nil {
			objs = append(objs, obj)
		}
	}
	return fleetAccountsFromObjects(objs)
}

// fleetAccountsFromObjects extracts account IDs and names from generic
// account objects, tolerating id/accountId/nmmId key variants.
func fleetAccountsFromObjects(objs []map[string]json.RawMessage) []fleetAccount {
	var out []fleetAccount
	seen := map[int64]bool{}
	for _, obj := range objs {
		id, ok := extractIntAny(obj, "id", "accountId", "nmmId")
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		name, _ := extractStringAny(obj, "name", "accountName", "companyName")
		if name == "" {
			name = fmt.Sprintf("account-%d", id)
		}
		out = append(out, fleetAccount{ID: id, Name: name})
	}
	return out
}

// jobTerminal reports whether an NMM job status string is terminal and
// whether it represents success. Statuses observed in the wild: Pending,
// Running, InProgress, Completed, CompletedWithErrors, Failed, Cancelled.
func jobTerminal(status string) (terminal bool, succeeded bool) {
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case s == "completed" || s == "succeeded" || s == "success":
		return true, true
	case strings.HasPrefix(s, "completedwith"):
		return true, false
	case s == "failed" || s == "error" || s == "cancelled" || s == "canceled":
		return true, false
	default:
		return false, false
	}
}

// jobWaitOutcome is the JSON envelope returned by job wait and the --wait
// branch of scripted-actions fan-run.
type jobWaitOutcome struct {
	JobID     int64  `json:"job_id"`
	Status    string `json:"status"`
	Terminal  bool   `json:"terminal"`
	Succeeded bool   `json:"succeeded"`
	Polls     int    `json:"polls"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

// waitForJob polls GET /job/{id} on a throttled interval until the job is
// terminal or the timeout elapses. Transient fetch errors are tolerated up
// to 3 consecutive misses (429s/blips) before giving up.
func waitForJob(ctx context.Context, c *client.Client, jobID int64, interval, timeout time.Duration) (jobWaitOutcome, error) {
	out := jobWaitOutcome{JobID: jobID, Status: "unknown"}
	start := time.Now()
	deadline := start.Add(timeout)
	consecutiveErrs := 0
	maxPolls := -1
	if cliutil.IsDogfoodEnv() {
		maxPolls = 1 // live-dogfood: one status read, no long poll loops
	}
	for {
		data, err := c.Get(ctx, nmmJobPath+strconv.FormatInt(jobID, 10), nil)
		out.Polls++
		if err != nil {
			consecutiveErrs++
			if consecutiveErrs >= 3 {
				out.ElapsedMS = time.Since(start).Milliseconds()
				return out, fmt.Errorf("polling job %d failed %d times in a row: %w", jobID, consecutiveErrs, err)
			}
		} else {
			consecutiveErrs = 0
			objs := decodeObjects(data)
			if len(objs) > 0 {
				if status, ok := extractStringAny(objs[0], "status", "jobStatus", "state"); ok {
					out.Status = status
				}
			}
			terminal, succeeded := jobTerminal(out.Status)
			if terminal {
				out.Terminal = true
				out.Succeeded = succeeded
				out.ElapsedMS = time.Since(start).Milliseconds()
				return out, nil
			}
		}
		if maxPolls > 0 && out.Polls >= maxPolls {
			out.ElapsedMS = time.Since(start).Milliseconds()
			return out, nil
		}
		if time.Now().After(deadline) {
			out.ElapsedMS = time.Since(start).Milliseconds()
			return out, fmt.Errorf("job %d still %q after %s timeout", jobID, out.Status, timeout)
		}
		select {
		case <-ctx.Done():
			out.ElapsedMS = time.Since(start).Milliseconds()
			return out, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// fleetFetchFailure is one failed account fetch inside a fleet fan-out.
// Failed accounts are reported separately - never folded into aggregates.
type fleetFetchFailure struct {
	Account string `json:"account"`
	Error   string `json:"error"`
}

// fanoutFailures converts cliutil fan-out errors into the JSON envelope shape.
func fanoutFailures(errs []cliutil.FanoutError) []fleetFetchFailure {
	out := make([]fleetFetchFailure, 0, len(errs))
	for _, e := range errs {
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		out = append(out, fleetFetchFailure{Account: e.Source, Error: msg})
	}
	return out
}

// poolIdentity extracts the {subscription, resourceGroup, name} triple that
// addresses a host pool in NMM detail paths, tolerating key variants.
func poolIdentity(obj map[string]json.RawMessage) (subscription, resourceGroup, name string, ok bool) {
	subscription, _ = extractStringAny(obj, "subscriptionId", "subscription", "azureSubscriptionId")
	resourceGroup, _ = extractStringAny(obj, "resourceGroup", "resourceGroupName", "azureResourceGroup")
	name, _ = extractStringAny(obj, "name", "hostPoolName", "poolName")
	if subscription == "" || resourceGroup == "" || name == "" {
		return subscription, resourceGroup, name, false
	}
	return subscription, resourceGroup, name, true
}

// capFleetAccounts applies the scan cap (and the live-dogfood curtail) to a
// fleet fan-out's account list, reporting whether the cap truncated it.
func capFleetAccounts(accounts []fleetAccount, maxAccounts int) ([]fleetAccount, bool) {
	if cliutil.IsDogfoodEnv() && len(accounts) > 1 {
		return accounts[:1], true
	}
	if maxAccounts > 0 && len(accounts) > maxAccounts {
		return accounts[:maxAccounts], true
	}
	return accounts, false
}
