// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: cross-tenant fan-out plumbing shared by the
// MSP lead queries (mfa-gaps, stale-licenses, sharing-drift, posture, ...).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/config"
)

// fanoutFlags is embedded by every cross-tenant command.
type fanoutFlags struct {
	all          bool
	tenants      string
	concurrency  int
	maxScanPages int
}

const allTenantsUsage = "Run against every registered client tenant"

// bind declares the shared fan-out flags. Each command declares
// --all-tenants itself (cmd.Flags().BoolVar(&ff.all, "all-tenants", ...)) so
// static flag checks can see it on the command that documents it.
func (ff *fanoutFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&ff.tenants, "tenants", "", "Comma-separated tenant names (default: --tenant or the only registered tenant)")
	cmd.Flags().IntVar(&ff.concurrency, "concurrency", 4, "Tenants queried in parallel")
	cmd.Flags().IntVar(&ff.maxScanPages, "max-scan-pages", 200, "Stop paging any one list after this many pages; a capped tenant is reported in fetch_failures")
}

// targets resolves which tenants a fan-out command runs against. With no
// tenant registered or selected, a configured bearer token (GOOGLE_WORKSPACE_TOKEN
// or 'auth set-token') runs as the implicit single-org tenant "default".
func (ff *fanoutFlags) targets(configPath string) ([]gwsTenant, error) {
	reg, err := loadTenantRegistry()
	if err != nil {
		return nil, configErr(err)
	}
	switch {
	case ff.all:
		c := reg.clients()
		if len(c) == 0 {
			if defaultTenantAvailable(configPath, reg) {
				return []gwsTenant{{Name: "default"}}, nil
			}
			return nil, configErr(fmt.Errorf("no client tenants registered (run 'tenants add')"))
		}
		return c, nil
	case ff.tenants != "":
		if len(splitTenantCSV(ff.tenants)) == 0 {
			return nil, usageErr(fmt.Errorf("--tenants %q names no tenant", ff.tenants))
		}
		var out []gwsTenant
		seen := map[string]bool{}
		for _, name := range splitTenantCSV(ff.tenants) {
			// --tenants acme,acme queries acme once (no doubled counts).
			if seen[name] {
				continue
			}
			seen[name] = true
			t, ok := reg.find(name)
			if !ok {
				return nil, usageErr(fmt.Errorf("unknown tenant %q", name))
			}
			out = append(out, t)
		}
		return out, nil
	}
	t, err := selectedTenant()
	if err != nil {
		return nil, err
	}
	if t == nil {
		if defaultTenantAvailable(configPath, reg) {
			return []gwsTenant{{Name: "default"}}, nil
		}
		return nil, usageErr(fmt.Errorf("no tenant selected: pass --tenant <name>, --tenants a,b or --all-tenants (see 'tenants list')"))
	}
	return []gwsTenant{*t}, nil
}

// defaultTenantAvailable reports the bearer-only single-org mode: no tenant
// registered, but a raw token is configured (GOOGLE_WORKSPACE_TOKEN or
// 'auth set-token'). That token then acts as the one tenant "default".
func defaultTenantAvailable(configPath string, reg *tenantRegistry) bool {
	if len(reg.Tenants) > 0 {
		return false
	}
	cfg, err := config.Load(configPath)
	return err == nil && cfg.CredentialConfigured()
}

type fetchFailure struct {
	Tenant string `json:"tenant"`
	Error  string `json:"error"`
}

// fanoutResult is the machine-mode envelope for every cross-tenant command,
// shaped {meta, results} so --agent keeps it as one envelope. Failed tenants
// are listed in meta.fetch_failures, never folded into results as empty rows,
// so no count or rollup silently drops a client.
type fanoutResult[T any] struct {
	Meta    fanoutMeta `json:"meta"`
	Results []T        `json:"results"`
}

type fanoutMeta struct {
	TenantsQueried int            `json:"tenants_queried"`
	FetchFailures  []fetchFailure `json:"fetch_failures"`
}

// fanout runs fn once per tenant with its own authenticated client.
func fanout[T any](cmd *cobra.Command, flags *rootFlags, ff *fanoutFlags,
	fn func(ctx context.Context, t gwsTenant, c *client.Client) ([]T, error)) (fanoutResult[T], error) {
	res := fanoutResult[T]{Results: []T{}, Meta: fanoutMeta{FetchFailures: []fetchFailure{}}}
	targets, err := ff.targets(flags.configPath)
	if err != nil {
		return res, err
	}
	res.Meta.TenantsQueried = len(targets)
	ctx := withScanCap(cmd.Context(), ff.maxScanPages)
	results, errs := cliutil.FanoutRun(ctx, targets, func(t gwsTenant) string { return t.Name },
		func(ctx context.Context, t gwsTenant) ([]T, error) {
			c, err := flags.newTenantClient(ctx, t)
			if err != nil {
				return nil, err
			}
			return fn(ctx, t, c)
		}, cliutil.WithConcurrency(ff.concurrency))
	for _, r := range results {
		res.Results = append(res.Results, r.Value...)
	}
	for _, e := range errs {
		res.Meta.FetchFailures = append(res.Meta.FetchFailures, fetchFailure{Tenant: e.Source, Error: e.Err.Error()})
	}
	return res, nil
}

// emitFanout prints the result (table, CSV rows, or the JSON envelope) and
// turns any tenant failure into exit code 5 unless --allow-partial-failure is
// set, so a broken client delegation never reads as a clean sweep.
func emitFanout[T any](cmd *cobra.Command, flags *rootFlags, res fanoutResult[T], table func([]T) ([]string, [][]string)) error {
	if n := len(res.Meta.FetchFailures); n > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d tenant(s) failed; results cover the remaining %d\n", n, res.Meta.TenantsQueried, res.Meta.TenantsQueried-n)
		for _, f := range res.Meta.FetchFailures {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %s\n", f.Tenant, f.Error)
		}
	}
	// --agent compacts rows to high-gravity fields; a sparse but decisive
	// field (admin_role, issues) must survive, so keep every row field.
	keep := append(rowFieldNames[T](), "meta", "results", "tenants_queried", "fetch_failures")
	var err error
	switch {
	case wantsHumanTable(cmd.OutOrStdout(), flags) && table != nil:
		headers, body := table(res.Results)
		if len(body) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No findings.")
		} else {
			err = flags.printTable(cmd, headers, body)
		}
	case flags.csv:
		err = printJSONFilteredKeep(cmd.OutOrStdout(), res.Results, flags, keep...)
	default:
		err = printJSONFilteredKeep(cmd.OutOrStdout(), res, flags, keep...)
	}
	if err != nil {
		return err
	}
	failed, total := len(res.Meta.FetchFailures), res.Meta.TenantsQueried
	switch {
	case failed == 0 || flags.allowPartialFailure:
		return nil
	case failed == total:
		return apiErr(fmt.Errorf("all %d tenant(s) failed", total))
	}
	return partialFailureErr(fmt.Errorf("%d of %d tenant(s) failed (results are partial; pass --allow-partial-failure to exit 0)", failed, total))
}

// rowFieldNames lists the JSON field names of a fan-out row type.
func rowFieldNames[T any]() []string {
	var names []string
	t := reflect.TypeOf((*T)(nil)).Elem()
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}

// helpOrDryRun is the verify-friendly RunE preamble: a bare invocation prints
// help, and --dry-run prints a parseable envelope before any file or network
// IO. handled reports whether the caller should return err immediately.
func helpOrDryRun(cmd *cobra.Command, args []string, flags *rootFlags, action string) (handled bool, err error) {
	if len(args) == 0 && cmd.Flags().NFlag() == 0 {
		return true, cmd.Help()
	}
	if dryRunOK(flags) {
		return true, writeDryRun(cmd.OutOrStdout(), flags, action)
	}
	return false, nil
}

// parseSince turns a --since value (7d, 24h, 2w) into a start time.
func parseSince(s string, maxDays int) (time.Time, error) {
	d, err := cliutil.ParseDurationLoose(s)
	if err != nil {
		return time.Time{}, usageErr(fmt.Errorf("--since %q: %w", s, err))
	}
	if d <= 0 || d > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, usageErr(fmt.Errorf("--since must be between 1m and %dd (audit retention)", maxDays))
	}
	return time.Now().UTC().Add(-d), nil
}

// --- paging -----------------------------------------------------------------

type scanCapKey struct{}

func withScanCap(ctx context.Context, pages int) context.Context {
	return context.WithValue(ctx, scanCapKey{}, pages)
}

func scanCap(ctx context.Context) int {
	if cliutil.IsDogfoodEnv() {
		return 1 // live dogfood runs each command under a 30s budget
	}
	if n, ok := ctx.Value(scanCapKey{}).(int); ok && n > 0 {
		return n
	}
	return 200
}

var errScanCapped = errors.New("scan capped")

// listAll follows Google's nextPageToken until exhausted (or max items when
// max > 0) and returns the raw items found under key. Hitting the page cap
// returns errScanCapped so a partial tenant is never mistaken for complete.
func listAll(ctx context.Context, c *client.Client, path string, params map[string]string, key string, max int) ([]json.RawMessage, error) {
	p := map[string]string{}
	for k, v := range params {
		p[k] = v
	}
	limit := scanCap(ctx)
	var items []json.RawMessage
	for page := 0; ; page++ {
		raw, err := getWithQuotaRetry(ctx, c, path, p)
		if err != nil {
			return items, err
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(raw, &env); err != nil {
			return items, fmt.Errorf("%s: decoding page: %w", path, err)
		}
		if arr, ok := env[key]; ok && len(arr) > 0 && string(arr) != "null" {
			var batch []json.RawMessage
			if err := json.Unmarshal(arr, &batch); err != nil {
				return items, fmt.Errorf("%s: decoding %s: %w", path, key, err)
			}
			items = append(items, batch...)
		}
		var next string
		if tok, ok := env["nextPageToken"]; ok {
			_ = json.Unmarshal(tok, &next)
		}
		if max > 0 && len(items) >= max {
			if len(items) == max && next == "" {
				return items, nil
			}
			if cliutil.IsDogfoodEnv() {
				return items[:max], nil
			}
			// Never present a truncated window as the whole answer.
			return items[:max], fmt.Errorf("%w: %s has more than %d item(s); raise --max-events or narrow --since", errScanCapped, path, max)
		}
		if next == "" {
			return items, nil
		}
		if page+1 >= limit {
			if cliutil.IsDogfoodEnv() {
				return items, nil
			}
			return items, fmt.Errorf("%w: %s stopped after %d page(s); raise --max-scan-pages", errScanCapped, path, limit)
		}
		p["pageToken"] = next
	}
}

// quotaBackoff is a var so tests can shrink it.
var quotaBackoff = time.Second

// getWithQuotaRetry retries Google's 403 userRateLimitExceeded/quotaExceeded,
// which the generated client treats as final (it retries only 429 and 5xx).
// Google documents 2^n seconds plus jitter for n = 0..4.
func getWithQuotaRetry(ctx context.Context, c *client.Client, path string, params map[string]string) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		raw, err := c.GetNoCache(ctx, path, params)
		if err == nil || attempt >= 4 || !isGoogleQuotaError(err) {
			return raw, err
		}
		// #nosec G404 -- backoff jitter, not security sensitive
		wait := quotaBackoff<<attempt + time.Duration(rand.Int63n(int64(quotaBackoff)))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func isGoogleQuotaError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "userRateLimitExceeded") || strings.Contains(msg, "rateLimitExceeded") || strings.Contains(msg, "quotaExceeded")
}

// --- directory user helpers -------------------------------------------------

type dirUser struct {
	ID           string `json:"id"`
	PrimaryEmail string `json:"primaryEmail"`
	Name         struct {
		FullName string `json:"fullName"`
	} `json:"name"`
	IsAdmin          bool   `json:"isAdmin"`
	IsDelegatedAdmin bool   `json:"isDelegatedAdmin"`
	IsEnrolledIn2Sv  bool   `json:"isEnrolledIn2Sv"`
	IsEnforcedIn2Sv  bool   `json:"isEnforcedIn2Sv"`
	Suspended        bool   `json:"suspended"`
	SuspensionReason string `json:"suspensionReason"`
	Archived         bool   `json:"archived"`
	AgreedToTerms    *bool  `json:"agreedToTerms"`
	LastLoginTime    string `json:"lastLoginTime"`
	CreationTime     string `json:"creationTime"`
	OrgUnitPath      string `json:"orgUnitPath"`
}

const dirUserFields = "nextPageToken,users(id,primaryEmail,name/fullName,isAdmin,isDelegatedAdmin,isEnrolledIn2Sv,isEnforcedIn2Sv,suspended,suspensionReason,archived,agreedToTerms,lastLoginTime,creationTime,orgUnitPath)"

// listUsers pages every user in a tenant. query uses Directory search syntax.
func listUsers(ctx context.Context, c *client.Client, t gwsTenant, query string) ([]dirUser, error) {
	params := map[string]string{"customer": t.customer(), "maxResults": "500", "fields": dirUserFields, "projection": "basic"}
	if query != "" {
		params["query"] = query
	}
	raw, err := listAll(ctx, c, "/admin/directory/v1/users", params, "users", 0)
	if err != nil {
		return nil, err
	}
	users := make([]dirUser, 0, len(raw))
	for _, r := range raw {
		var u dirUser
		if err := json.Unmarshal(r, &u); err != nil {
			return nil, fmt.Errorf("decoding user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

// neverSignedIn reports Google's 1970-01-01 lastLoginTime sentinel, or a user
// who never accepted the terms (agreedToTerms=false means no first sign-in).
func neverSignedIn(u dirUser) bool {
	return neverLoggedIn(u.LastLoginTime) || (u.AgreedToTerms != nil && !*u.AgreedToTerms)
}

func neverLoggedIn(ts string) bool {
	return ts == "" || strings.HasPrefix(ts, "1970-01-01")
}

// daysSince returns whole days since an RFC3339 timestamp, or -1 if unknown.
func daysSince(ts string, now time.Time) int {
	if neverLoggedIn(ts) {
		return -1
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return -1
	}
	return int(now.Sub(t).Hours() / 24)
}

func adminRole(u dirUser) string {
	switch {
	case u.IsAdmin:
		return "super_admin"
	case u.IsDelegatedAdmin:
		return "delegated_admin"
	}
	return ""
}

// laterTS reports whether RFC3339 timestamp a is after b, comparing instants
// ("...:00Z" vs "...:00.001Z" sort wrong as strings). Unparseable values
// fall back to string order.
func laterTS(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	if errA != nil || errB != nil {
		return a > b
	}
	return ta.After(tb)
}

func itoaOrDash(n int) string {
	if n < 0 {
		return "never"
	}
	return strconv.Itoa(n)
}
