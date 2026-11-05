// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): what every warranty command
// shares. The transport guard is installed here, before any command runs.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/cliutil"
	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"
)

const warrantyCLIName = "warranty-cli"

func init() {
	registerClientHook(func(c *client.Client) error {
		client.InstallGuard(c)
		return nil
	})
	// These commands copy vendor data into the local mirror, like sync.
	for _, path := range []string{"import", "recheck", "apple-pull"} {
		localDataWriterCommands[path] = true
	}
}

// vendorState says whether one vendor's credentials are configured.
type vendorState struct {
	Vendor     string   `json:"vendor"`
	Configured bool     `json:"configured"`
	Needs      []string `json:"needs,omitempty"`
	Enrollment string   `json:"enrollment"`
}

func dellConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if cfg.CredentialConfigured() {
		return true
	}
	id, secret := cfg.ClientID, cfg.ClientSecret
	if id == "" {
		id = cliutil.EnvOverride("WARRANTY_DELL_CLIENT_ID")
	}
	if secret == "" {
		secret = cliutil.EnvOverride("WARRANTY_DELL_CLIENT_SECRET")
	}
	return strings.TrimSpace(id) != "" && strings.TrimSpace(secret) != ""
}

func vendorStates(cfg *config.Config) map[string]vendorState {
	out := map[string]vendorState{}
	dell := vendorState{Vendor: warranty.Dell, Configured: dellConfigured(cfg),
		Enrollment: "Dell TechDirect: request Warranty API access (APIs, API Key Management); approval can take several days"}
	if !dell.Configured {
		dell.Needs = []string{"WARRANTY_DELL_CLIENT_ID", "WARRANTY_DELL_CLIENT_SECRET"}
	}
	out[warranty.Dell] = dell
	lenovo := vendorState{Vendor: warranty.Lenovo, Configured: client.LenovoKeyFromEnv() != "",
		Enrollment: "Lenovo eSupport WebAPI: a ClientID token from your Lenovo account manager or partner representative"}
	if !lenovo.Configured {
		lenovo.Needs = []string{client.EnvLenovoClientKey}
	}
	out[warranty.Lenovo] = lenovo
	_, unset := client.AppleCredentialsFromEnv()
	apple := vendorState{Vendor: warranty.Apple, Configured: len(unset) == 0, Needs: unset,
		Enrollment: "Apple Business: an API account (Settings, API) in each customer's organization"}
	out[warranty.Apple] = apple
	return out
}

// refuseUnderMCP keeps host-file reads and other operator-only actions off
// the MCP surface: an MCP tool argument never names a file on the host.
func refuseUnderMCP(action string) error {
	if cliutil.IsMCPChildEnv() {
		return usageErr(fmt.Errorf("%s reads a file on this machine and is available from the CLI only, not over MCP", action))
	}
	return nil
}

// warrantyDBPath is the mirror's own database, data-warranty.db. It is not
// the generated store (which is scoped by credential): one mirror holds every
// vendor and customer, so it must not move to another file when a token is
// minted or a credential changes. The name matches the store-file patterns
// local-data purge and doctor use, and can never equal a 12-hex scope hash.
func warrantyDBPath() string {
	dir, err := cliutil.DataDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "data-warranty.db"
		}
		dir = filepath.Join(home, ".local", "share", warrantyCLIName)
	}
	return filepath.Join(dir, "data-warranty.db")
}

// openWarrantyRead opens the mirror read-only, or returns nil, nil when no
// mirror exists yet.
func openWarrantyRead(ctx context.Context) (*store.Store, error) {
	p := warrantyDBPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, nil
	}
	return store.OpenReadOnlyContext(ctx, p)
}

func openWarrantyWrite(ctx context.Context) (*store.Store, error) {
	p := warrantyDBPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	return store.OpenWithContext(ctx, p)
}

// judgeCoverageOn re-reads each device's coverage from its lines on asOf: the saved
// columns were judged on the date of the lookup that saved them.
func judgeCoverageOn(ctx context.Context, db *store.Store, devices []store.Device, asOf string) error {
	for i := range devices {
		lines, err := db.Lines(ctx, devices[i].Serial)
		if err != nil {
			return err
		}
		if len(lines) > 0 {
			sum := warranty.Summarize(lines, asOf)
			devices[i].CoverageEnd, devices[i].CoverageOpen = sum.End, sum.Open
		}
	}
	return nil
}

// today returns the --as-of date or the current UTC date.
func today(asOf string) (string, error) {
	if asOf == "" {
		return time.Now().UTC().Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", asOf); err != nil {
		return "", usageErr(fmt.Errorf("--as-of must be a date like 2026-10-28"))
	}
	return asOf, nil
}

// parseDays reads a day-scale duration flag: "90d", "12w", "4y" (365 days a
// year) or a Go duration such as "2160h".
func parseDays(flag, v string) (int, error) {
	v = strings.TrimSpace(v)
	if n := len(v); n >= 2 && v[n-1] == 'y' {
		years, err := strconv.Atoi(v[:n-1])
		if err != nil || years < 0 || years > 50 {
			return 0, usageErr(fmt.Errorf("%s must be a duration like 90d, 12w or 4y", flag))
		}
		return years * 365, nil
	}
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("%s must be a duration like 90d, 12w or 4y", flag))
	}
	return int(d.Hours() / 24), nil
}

// coverageTrusted reports whether a device's stored coverage may be used:
// only when the last vendor answer was a success.
func coverageTrusted(d store.Device) bool {
	return d.LastResult == warranty.ResultOK || d.LastResult == warranty.ResultDuplicated
}

func normalizeCustomer(s string) string { return strings.Join(strings.Fields(s), " ") }

// ---- lookup engine ------------------------------------------------------

// Batch sizes and pacing. Dell documents 1 to 100 service tags per call. Lenovo
// takes several serials per call without a published cap and throttles
// without a published rate, so batches stay small and calls are spaced the
// way breeze spaces them.
const (
	dellBatch   = 100
	lenovoBatch = 25
	lenovoPace  = 250 * time.Millisecond
)

type lookupTarget struct {
	Serial       string
	Vendor       string
	VendorSource string
	Customer     string
}

type lookupOutcome struct {
	Serial       string          `json:"serial"`
	Vendor       string          `json:"vendor"`
	VendorSource string          `json:"vendor_source,omitempty"`
	Customer     string          `json:"customer,omitempty"`
	Result       string          `json:"result"`
	Message      string          `json:"message,omitempty"`
	Model        string          `json:"model,omitempty"`
	ShipDate     string          `json:"ship_date,omitempty"`
	PurchaseDate string          `json:"purchase_date,omitempty"`
	CoverageEnd  string          `json:"coverage_end"`
	CoverageOpen bool            `json:"coverage_open"`
	Status       string          `json:"status"`
	DaysLeft     *int            `json:"days_left"`
	Lines        []warranty.Line `json:"lines"`
	checked      bool
}

type fetchFailure struct {
	Vendor  string   `json:"vendor"`
	Serials []string `json:"serials"`
	Error   string   `json:"error"`
}

type lookupRun struct {
	Results         []lookupOutcome `json:"results"`
	Calls           map[string]int  `json:"calls"`
	FetchFailures   []fetchFailure  `json:"fetch_failures"`
	FirstError      string          `json:"first_error,omitempty"`
	EventsRaised    int             `json:"events_raised"`
	Saved           bool            `json:"saved"`
	BudgetExhausted bool            `json:"budget_exhausted,omitempty"`
	Note            string          `json:"note,omitempty"`
}

type lookupOpts struct {
	MaxCalls   int // 0 = no limit
	WindowDays int
	AsOf       string
	Save       bool
}

// vendorCalls is the number of API calls a lookup of these serials costs.
func vendorCalls(vendor string, n int) int {
	switch vendor {
	case warranty.Dell:
		return (n + dellBatch - 1) / dellBatch
	case warranty.Lenovo:
		return (n + lenovoBatch - 1) / lenovoBatch
	case warranty.Apple:
		return 2 * n
	}
	return 0
}

// resolveTargets fills each target's vendor: declared, else the mirror's,
// else detected from the serial.
func resolveTargets(ctx context.Context, targets []lookupTarget) []lookupTarget {
	var db *store.Store
	if s, err := openWarrantyRead(ctx); err == nil && s != nil {
		db = s
		defer db.Close()
	}
	for i := range targets {
		t := &targets[i]
		t.Serial = warranty.NormalizeSerial(t.Serial)
		if t.Vendor != "" {
			continue
		}
		if db != nil {
			if d, ok, _ := db.GetDevice(ctx, t.Serial); ok && d.Vendor != "" {
				t.Vendor, t.VendorSource = d.Vendor, d.VendorSource
				continue
			}
		}
		if v, ok := warranty.DetectVendor(t.Serial); ok {
			t.Vendor, t.VendorSource = v, warranty.SourceDetected
		}
	}
	return targets
}

func runLookups(ctx context.Context, flags *rootFlags, targets []lookupTarget, opts lookupOpts) (lookupRun, error) {
	run := lookupRun{Results: []lookupOutcome{}, Calls: map[string]int{}, FetchFailures: []fetchFailure{}}
	c, err := flags.newClient()
	if err != nil {
		return run, err
	}
	states := vendorStates(c.Config)
	byVendor := map[string][]lookupTarget{}
	outcomes := map[string]*lookupOutcome{}
	order := []string{}
	for _, t := range targets {
		if _, dup := outcomes[t.Serial]; dup {
			continue
		}
		o := &lookupOutcome{Serial: t.Serial, Vendor: t.Vendor, VendorSource: t.VendorSource, Customer: t.Customer, Lines: []warranty.Line{}}
		outcomes[t.Serial] = o
		order = append(order, t.Serial)
		switch {
		case t.Vendor == "":
			o.Result = warranty.ResultVendorUnknown
			o.Message = "vendor not detected from the serial: pass --vendor dell|lenovo|apple or add a manufacturer column"
		case !states[t.Vendor].Configured:
			o.Result = warranty.ResultVendorUnconfigured
			o.Message = "set " + strings.Join(states[t.Vendor].Needs, ", ")
		default:
			byVendor[t.Vendor] = append(byVendor[t.Vendor], t)
		}
	}
	budget := opts.MaxCalls
	spend := func(n int) bool {
		if opts.MaxCalls <= 0 {
			return true
		}
		if n > budget {
			return false
		}
		budget -= n
		return true
	}
	fail := func(vendor string, serials []string, err error) {
		msg := lookupErrMessage(err)
		result := warranty.ResultError
		if errors.Is(err, client.ErrVendorNotConfigured) {
			result = warranty.ResultVendorUnconfigured
		}
		for _, s := range serials {
			outcomes[s].Result, outcomes[s].Message = result, msg
		}
		if result == warranty.ResultError {
			run.FetchFailures = append(run.FetchFailures, fetchFailure{Vendor: vendor, Serials: serials, Error: msg})
			if run.FirstError == "" {
				run.FirstError = msg
			}
		}
	}
	apply := func(recs []warranty.Record) {
		for _, r := range recs {
			o, ok := outcomes[r.Serial]
			if !ok {
				continue
			}
			o.Result, o.Message, o.Model, o.ShipDate, o.PurchaseDate, o.Lines = r.Result, r.Message, r.Model, r.ShipDate, r.PurchaseDate, r.Lines
			if o.Lines == nil {
				o.Lines = []warranty.Line{}
			}
			o.checked = true
		}
	}
	skipBudget := func(serials []string) {
		run.BudgetExhausted = true
		for _, s := range serials {
			outcomes[s].Result, outcomes[s].Message = "skipped", "--max-calls budget reached"
		}
	}

	if ts := byVendor[warranty.Dell]; len(ts) > 0 {
		for _, chunk := range chunkTargets(ts, dellBatch) {
			serials := targetSerials(chunk)
			if !spend(1) {
				skipBudget(serials)
				continue
			}
			run.Calls[warranty.Dell]++
			body, err := c.GetNoCache(ctx, "/asset-entitlements", map[string]string{"servicetags": strings.Join(serials, ",")})
			if err != nil {
				fail(warranty.Dell, serials, err)
				continue
			}
			recs, err := warranty.ParseDell(body, serials)
			if err != nil {
				fail(warranty.Dell, serials, err)
				continue
			}
			apply(recs)
		}
	}
	if ts := byVendor[warranty.Lenovo]; len(ts) > 0 {
		lc := c.WithTier("lenovo")
		for i, chunk := range chunkTargets(ts, lenovoBatch) {
			serials := targetSerials(chunk)
			if !spend(1) {
				skipBudget(serials)
				continue
			}
			if i > 0 {
				if err := sleepCtx(ctx, lenovoPace); err != nil {
					return run, err
				}
			}
			run.Calls[warranty.Lenovo]++
			body, err := lc.GetWithHeadersNoCacheValues(ctx, "/warranty", url.Values{"Serial": serials}, nil)
			if err != nil {
				fail(warranty.Lenovo, serials, err)
				continue
			}
			recs, err := warranty.ParseLenovo(body, serials)
			if err != nil {
				fail(warranty.Lenovo, serials, err)
				continue
			}
			apply(recs)
		}
	}
	if ts := byVendor[warranty.Apple]; len(ts) > 0 {
		ac := c.WithTier("apple")
		for _, t := range ts {
			if !spend(2) {
				skipBudget([]string{t.Serial})
				continue
			}
			path := "/orgDevices/" + url.PathEscape(t.Serial)
			rec, notInOrg, err := appleCoverage(ctx, ac, t.Serial, path, spend, run.Calls)
			switch {
			case notInOrg:
				apply([]warranty.Record{{Serial: t.Serial, Vendor: warranty.Apple, Result: warranty.ResultNotInOrg,
					Message: "not in the Apple Business organization whose API account is configured"}})
				continue
			case err != nil:
				fail(warranty.Apple, []string{t.Serial}, err)
				continue
			}
			run.Calls[warranty.Apple]++
			if dev, err := ac.GetNoCache(ctx, path, nil); err == nil {
				_ = warranty.ParseAppleDevice(dev, &rec)
			}
			apply([]warranty.Record{rec})
		}
	}

	attempted, answered := 0, 0
	now := time.Now().UTC().Format(time.RFC3339Nano) // part of change-event identity: never two lookups at one stamp
	for _, s := range order {
		o := outcomes[s]
		sum := warranty.Summarize(o.Lines, opts.AsOf)
		o.CoverageEnd, o.CoverageOpen = sum.End, sum.Open
		if o.Result == warranty.ResultOK || o.Result == warranty.ResultDuplicated {
			o.Status, o.DaysLeft = warranty.Status(sum, opts.AsOf, opts.WindowDays)
		}
		if o.checked || o.Result == warranty.ResultError {
			attempted++
		}
		if o.checked {
			answered++
		}
		run.Results = append(run.Results, *o)
	}
	if attempted == 0 && len(order) > 0 && !run.BudgetExhausted {
		run.Note = "no vendor was called: see each result's message (vendor not configured or not detected)"
	}
	if opts.Save && !localDataDisabled() && len(order) > 0 {
		db, err := openWarrantyWrite(ctx)
		if err != nil {
			return run, fmt.Errorf("opening the warranty mirror: %w", err)
		}
		defer db.Close()
		for _, o := range run.Results {
			if o.Result == "skipped" {
				continue
			}
			checkedAt := ""
			if o.checked {
				checkedAt = now
			}
			evs, err := db.ApplyLookup(ctx, store.LookupWrite{
				Record: warranty.Record{Serial: o.Serial, Vendor: o.Vendor, Result: o.Result, Message: o.Message,
					Model: o.Model, ShipDate: o.ShipDate, PurchaseDate: o.PurchaseDate, Lines: o.Lines},
				VendorSource: o.VendorSource, Customer: o.Customer, CheckedAt: checkedAt, AsOf: opts.AsOf, WindowDays: opts.WindowDays,
			})
			if err != nil {
				return run, fmt.Errorf("saving %s to the warranty mirror: %w", o.Serial, err)
			}
			run.EventsRaised += len(evs)
		}
		run.Saved = true
	}
	if attempted > 0 && answered == 0 {
		return run, apiErr(fmt.Errorf("every vendor call failed: %s", run.FirstError))
	}
	return run, nil
}

// appleCoveragePages caps how many coverage pages one device may take.
const appleCoveragePages = 10

// appleCoverage reads every page of a device's AppleCare coverage. Coverage
// is never recorded from a partial set: a page past the cap or past the call
// budget is an error. notInOrg reports a 404 on the first page.
func appleCoverage(ctx context.Context, ac *client.Client, serial, path string, spend func(int) bool, calls map[string]int) (warranty.Record, bool, error) {
	var rec warranty.Record
	cursor := ""
	for page := 0; page < appleCoveragePages; page++ {
		if page > 0 && !spend(1) {
			return rec, false, fmt.Errorf("AppleCare coverage for %s has more pages than the --max-calls budget allows; nothing recorded", serial)
		}
		calls[warranty.Apple]++
		params := map[string]string{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		body, err := ac.GetNoCache(ctx, path+"/appleCareCoverage", params)
		if err != nil {
			var apiErr *client.APIError
			if page == 0 && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
				return rec, true, nil
			}
			return rec, false, err
		}
		pageRec, err := warranty.ParseAppleCoverage(serial, body)
		if err != nil {
			return rec, false, err
		}
		if page == 0 {
			rec = pageRec
		} else {
			rec.Lines = warranty.AppendLines(rec.Lines, pageRec.Lines)
		}
		if cursor = warranty.AppleNextCursor(body); cursor == "" {
			return rec, false, nil
		}
	}
	return rec, false, fmt.Errorf("AppleCare coverage for %s has more than %d pages; nothing recorded", serial, appleCoveragePages)
}

// lookupErrMessage turns a request error into a short message.
func lookupErrMessage(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		msg := fmt.Sprintf("HTTP %d", apiErr.StatusCode)
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			msg += ": the vendor rejected the credentials (run doctor)"
		case http.StatusTooManyRequests:
			msg += ": rate limited; try again later or lower --max-calls"
		}
		return msg
	}
	return err.Error()
}

func chunkTargets(ts []lookupTarget, n int) [][]lookupTarget {
	var out [][]lookupTarget
	for len(ts) > 0 {
		k := n
		if len(ts) < k {
			k = len(ts)
		}
		out = append(out, ts[:k])
		ts = ts[k:]
	}
	return out
}

func targetSerials(ts []lookupTarget) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Serial
	}
	return out
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// mirrorWriteAllowed applies the MCP write rule: over MCP, a write previews and
// is sent only with the boolean confirm: true. Text such as confirm: "true" sets
// the flag and not the marker. The CLI writes its own mirror by default.
func mirrorWriteAllowed() bool { return !cliutil.MCPChildUnconfirmed() }

const previewNote = "preview: nothing was saved to the mirror; call again with confirm: true to save"

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// markSourceByCalls keeps the --agent envelope honest: meta.source is live
// only when a vendor was actually called.
func markSourceByCalls(flags *rootFlags, calls map[string]int) {
	total := 0
	for _, n := range calls {
		total += n
	}
	if total == 0 && flags != nil && !flags.dryRun {
		flags.agentSource = "local"
	}
}

// planText describes what a lookup of these targets would call, for --dry-run.
// Vendors without credentials are named as skipped, as the real run skips them.
func planText(action string, targets []lookupTarget, states map[string]vendorState) string {
	count := map[string]int{}
	for _, t := range targets {
		v := t.Vendor
		if v == "" {
			v = "unknown"
		}
		count[v]++
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		if k == "unknown" {
			parts = append(parts, fmt.Sprintf("%d with no detected vendor", count[k]))
			continue
		}
		if st, ok := states[k]; ok && !st.Configured {
			parts = append(parts, fmt.Sprintf("%s %d serial(s) skipped (not configured)", k, count[k]))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d serial(s) in %d call(s)", k, count[k], vendorCalls(k, count[k])))
	}
	if len(parts) == 0 {
		return action + " (nothing to look up)"
	}
	return action + ": " + strings.Join(parts, "; ")
}
