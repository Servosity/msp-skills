// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Shared plumbing for the fleet/abb novel commands.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
	"synology-pp-cli/internal/store"
)

// exitFindings is the typed exit code for "the command ran and found
// something that needs attention" (failed backups, stale devices, storage
// problems, a NAS that stopped reporting). RMM checks alert on it.
const exitFindings = 6

func findingsErr(n int) error {
	return &cliError{code: exitFindings, err: fmt.Errorf("%d row(s) need attention (exit 6; --no-fail exits 0)", n)}
}

// redactErr masks every credential and session value c has used in err's
// text (response-derived values can echo them), keeping its exit code.
func redactErr(c *dsm.Client, err error) error {
	if err == nil || c == nil {
		return err
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return &cliError{code: ce.code, err: errors.New(c.Redact(ce.err.Error()))}
	}
	return errors.New(c.Redact(err.Error()))
}

// dsmDryRunRequest is the exact request a DSM write command would send.
type dsmDryRunRequest struct {
	HTTPMethod string            `json:"http_method"`
	NAS        string            `json:"nas"`
	Path       string            `json:"path"`
	API        string            `json:"api"`
	APIMethod  string            `json:"api_method"`
	Version    string            `json:"version"`
	Params     map[string]string `json:"params"`
	Session    string            `json:"session"`
	Note       string            `json:"note,omitempty"`
}

// dsmDryRunResult keeps the standard dry-run keys (dry_run, action, would)
// and adds the request, so the person approving a backup start, a cancel or
// a logout sees which NAS, which DSM call and which task ids it would hit.
type dsmDryRunResult struct {
	DryRun  bool             `json:"dry_run"`
	Action  string           `json:"action"`
	Would   string           `json:"would"`
	Request dsmDryRunRequest `json:"request"`
}

// dsmDryRunSession says what is added to the request when it is sent; the
// values themselves (password, sid, SynoToken) are never printed.
const dsmDryRunSession = "the CLI's DSM session is added when sent (_sid query parameter and X-SYNO-TOKEN header); not shown"

// dsmDryRun answers --dry-run for a generated DSM write command
// (HAND-FIX(dsm-dry-run)), sending nothing. The preview names the exact
// request: the HTTP method and spec path from the command's own pp:method and
// pp:path annotations, the NAS origin from SYNOLOGY_BASE_URL and every
// parameter the command would add (params, for example task_ids). An address
// carrying a user or password is refused even in a preview.
func dsmDryRun(cmd *cobra.Command, flags *rootFlags, action string, params map[string]string) error {
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	if err := dsm.CheckOrigin(c.BaseURL); err != nil {
		return configErr(err)
	}
	method, path := cmd.Annotations["pp:method"], cmd.Annotations["pp:path"]
	if method == "" || path == "" {
		return fmt.Errorf("%s: no declared request to preview (pp:method/pp:path missing); nothing was sent", action)
	}
	if err := dsm.CheckOperationParams(path, params); err != nil {
		return usageErr(err)
	}
	if _, err := url.Parse(strings.TrimSpace(c.BaseURL)); err != nil {
		return configErr(errors.New("SYNOLOGY_BASE_URL is not a URL"))
	}
	// The address is printed through the one display rule (no user info,
	// query or fragment), and the preview's request is built from it.
	origin := strings.TrimRight(dsm.DisplayAddress(c.BaseURL), "/")
	target, err := url.Parse(origin + path)
	if err != nil {
		return fmt.Errorf("%s: the declared path is not a URL", action)
	}
	q := target.Query()
	req := dsmDryRunRequest{HTTPMethod: method, NAS: origin, Path: target.Path,
		API: q.Get("api"), APIMethod: q.Get("method"), Version: q.Get("version"),
		Params: map[string]string{}, Session: dsmDryRunSession}
	for k := range q {
		if k != "api" && k != "method" && k != "version" {
			req.Params[k] = q.Get(k)
		}
	}
	for k, v := range params {
		if v != "" { // the live GET drops an empty value too
			q.Set(k, v)
			req.Params[k] = v
		}
	}
	target.RawQuery = cliutil.EncodeQuery(q)
	if dsm.PlaceholderOrigin(origin) {
		req.Note = "SYNOLOGY_BASE_URL is the example placeholder; set it to your NAS before running this for real"
	}
	res := dsmDryRunResult{DryRun: true, Action: action, Would: method + " " + target.String(), Request: req}
	w := cmd.OutOrStdout()
	if flags != nil && flags.asJSON {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(res)
	}
	keys := make([]string, 0, len(req.Params))
	for k := range req.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var p strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&p, " %s=%s", k, req.Params[k])
	}
	if p.Len() == 0 {
		p.WriteString(" (none)")
	}
	fmt.Fprintf(w, "dry-run: would %s (%s; nothing was sent)\n", res.Would, action)
	fmt.Fprintf(w, "  NAS:        %s\n  DSM call:   %s method=%s version=%s\n  parameters:%s\n  session:    %s\n", req.NAS, req.API, req.APIMethod, req.Version, p.String(), dsmDryRunSession)
	if req.Note != "" {
		fmt.Fprintf(w, "  note:       %s\n", req.Note)
	}
	return nil
}

// fleetNAS returns the registered NAS units. With an empty registry it falls
// back to the single NAS configured through SYNOLOGY_BASE_URL and
// SYNOLOGY_ACCOUNT/SYNOLOGY_PASSWORD, named "default", so a one-NAS user needs
// no registration.
func fleetNAS(flags *rootFlags) ([]fleet.NAS, *fleet.Registry, string, error) {
	path, err := fleet.RegistryPath()
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	reg, err := fleet.LoadRegistry(path)
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	if len(reg.NAS) > 0 {
		return reg.NAS, reg, path, nil
	}
	d, ok, err := defaultNAS(flags)
	if err != nil {
		return nil, nil, "", configErr(err)
	}
	if ok {
		return []fleet.NAS{d}, reg, path, nil
	}
	return nil, reg, path, nil
}

// defaultNAS builds the implicit "default" fleet entry from the CLI config.
// A SYNOLOGY_BASE_URL carrying a user or password, or a path, is an error: it
// would be stored in the mirror and printed.
func defaultNAS(flags *rootFlags) (fleet.NAS, bool, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return fleet.NAS{}, false, fmt.Errorf("reading the CLI configuration: %w", err)
	}
	if cfg.BaseURL == "" || doctorBaseURLIsPlaceholder(cfg.BaseURL) {
		return fleet.NAS{}, false, nil
	}
	origin, err := fleet.OriginURL(cfg.BaseURL)
	if err != nil {
		return fleet.NAS{}, false, fmt.Errorf("SYNOLOGY_BASE_URL: %w", err)
	}
	pin, skip := dsm.EnvTLS()
	n := fleet.NAS{Name: "default", URL: origin, EnvPrefix: "SYNOLOGY", TLSFingerprint: pin, Insecure: skip}
	n.Override = &dsm.Credentials{Account: cfg.SynologyAccount, Password: cfg.SynologyPassword, OTPCode: cfg.SynologyOtpCode, DeviceID: cfg.SynologyDeviceId}
	return n, true, nil
}

// resolveNAS picks one NAS by name, or the only one when name is empty.
func resolveNAS(flags *rootFlags, name string) (fleet.NAS, error) {
	list, _, _, err := fleetNAS(flags)
	if err != nil {
		return fleet.NAS{}, err
	}
	if len(list) == 0 {
		return fleet.NAS{}, configErr(errors.New("no NAS configured: run 'synology-cli fleet add <name> --url https://<nas>:5001' or set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD"))
	}
	if name == "" {
		if len(list) == 1 {
			return list[0], nil
		}
		return fleet.NAS{}, usageErr(fmt.Errorf("%d NAS units are registered; pick one with --nas", len(list)))
	}
	for _, n := range list {
		if n.Name == name {
			return n, nil
		}
	}
	return fleet.NAS{}, notFoundErr(fmt.Errorf("no registered NAS named %q (see 'synology-cli fleet list')", name))
}

// dsmClient builds the per-NAS RPC client.
func dsmClient(flags *rootFlags, n fleet.NAS) (*dsm.Client, error) {
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	hc, err := n.HTTPClient(timeout)
	if err != nil {
		return nil, configErr(err)
	}
	baseURL := strings.TrimRight(n.URL, "/")
	creds := n.Credentials()
	return &dsm.Client{HTTP: hc, BaseURL: baseURL, Creds: creds, OnDeviceID: func(did string) error {
		// Cache the trusted-device token so the next login skips the one-time code.
		dir, err := cliutil.DataDir()
		if err != nil {
			return err
		}
		return dsm.SaveDeviceID(dir, baseURL, creds.Account, did)
	}}, nil
}

func fleetDBPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	p, err := fleet.DBPath()
	if err != nil {
		return "", configErr(err)
	}
	return p, nil
}

// openFleetWrite opens (creating if needed) the fleet mirror for writing.
func openFleetWrite(ctx context.Context, path string) (*store.Store, error) {
	s, err := store.OpenWithContext(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := fleet.EnsureSchema(ctx, s.DB()); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// openFleetRead opens the mirror read-only so reads never contend with a
// running fleet sync. A missing mirror is created first (schema only) when NAS
// units are registered, so their rows read as "never synced", never as empty.
func openFleetRead(ctx context.Context, path string, haveNAS bool) (*sql.DB, func(), error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if !haveNAS {
			return nil, func() {}, nil
		}
		s, err := openFleetWrite(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		_ = s.Close()
	}
	s, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	// A mirror from an older version is migrated once, through a short write
	// open; reads stay read-only otherwise.
	if stale, err := fleet.NeedsMigration(ctx, s.DB()); err != nil || stale {
		_ = s.Close()
		if err != nil {
			return nil, nil, err
		}
		w, err := openFleetWrite(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		_ = w.Close()
		if s, err = store.OpenReadOnlyContext(ctx, path); err != nil {
			return nil, nil, err
		}
	}
	return s.DB(), func() { _ = s.Close() }, nil
}

// printFleetRows writes machine output through the generated helpers and,
// for a human, one card per row (writeFleetCards); rows must be a slice.
func printFleetRows(cmd *cobra.Command, flags *rootFlags, rows any, empty string) error {
	w := cmd.OutOrStdout()
	if !wantsHumanTable(w, flags) {
		return printReportJSON(w, rows, flags) // agent-compaction-keeps-report-fields
	}
	return writeFleetCards(w, rows, empty)
}

// fleetFieldTier ranks a report key for the human view: identity first (the
// NAS, its client, then the host, task or item the row is about), then what
// the row is (kind, state, ok), then why (error, sync_error, note, ...), then
// every other field in the order the row type declares it.
func fleetFieldTier(key string, hasNAS bool) (tier, pos int) {
	order := [][]string{
		{"nas"},
		{"client"},
		{"host_name", "task_name", "name", "key", "device_name"},
		{"kind", "state", "severity", "ok", "status"},
		{"error", "sync_error", "last_sync_error", "problem", "note", "first_error", "warnings"},
	}
	if !hasNAS && key == "name" {
		return 0, 0 // fleet list: the row's name is the NAS
	}
	for t, keys := range order {
		for i, k := range keys {
			if k == key {
				return t, i
			}
		}
	}
	return len(order), 0
}

// fleetTimeKeys are unix-second fields shown as local times.
func fleetTimeKey(key string) bool {
	return strings.HasSuffix(key, "_at") || key == "last_success" || key == "time_start" || key == "time_end" || key == "history_from"
}

// writeFleetCards is the human view of a fleet/abb report: one card per row.
// Every key that any row carries is shown on every card, in one fixed order
// per row type (fleetFieldTier, then the type's declaration order), and no
// value is truncated, so the NAS, client, host or task a row is about and its
// kind, error, sync_error or note are never dropped. A key a row does not
// carry, or a null (not measured), shows "-".
func writeFleetCards(w io.Writer, rows any, empty string) error {
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, empty)
		return err
	}
	present := map[string]bool{}
	for _, it := range items {
		for k := range it {
			present[k] = true
		}
	}
	declared := map[string]int{}
	for i, k := range declaredJSONFields(reflect.TypeOf(rows)) {
		if _, dup := declared[k]; !dup {
			declared[k] = i
		}
	}
	keys := make([]string, 0, len(present))
	for k := range present {
		keys = append(keys, k)
	}
	hasNAS := present["nas"]
	sort.Slice(keys, func(i, j int) bool {
		ti, pi := fleetFieldTier(keys[i], hasNAS)
		tj, pj := fleetFieldTier(keys[j], hasNAS)
		if ti != tj {
			return ti < tj
		}
		if pi != pj {
			return pi < pj
		}
		di, iok := declared[keys[i]]
		dj, jok := declared[keys[j]]
		switch {
		case iok && jok && di != dj:
			return di < dj
		case iok != jok:
			return iok // declared fields before undeclared ones
		}
		return keys[i] < keys[j]
	})
	width := 0
	for _, k := range keys {
		width = max(width, len(k))
	}
	var b strings.Builder
	for n, it := range items {
		if n > 0 {
			b.WriteString("\n")
		}
		var title []string
		for _, k := range keys {
			if t, _ := fleetFieldTier(k, hasNAS); t > 2 {
				break
			}
			if v := fleetCardValue(k, it[k]); v != "-" {
				title = append(title, v)
			}
		}
		if len(title) == 0 {
			title = append(title, fmt.Sprintf("row %d", n+1))
		}
		b.WriteString(bold(strings.Join(title, " / ")) + "\n")
		for _, k := range keys {
			v, ok := it[k]
			text := "-"
			if ok {
				text = fleetCardValue(k, v)
			}
			fmt.Fprintf(&b, "  %-*s  %s\n", width+1, k+":", strings.ReplaceAll(text, "\n", "\n  "+strings.Repeat(" ", width+3)))
		}
	}
	_, err = io.WriteString(w, b.String())
	return err
}

// fleetCardValue renders one report value for a card, in full: strings are
// scrubbed of terminal controls (never truncated), unix times are local
// times, null is "-", an empty list is "(none)" and a list puts one item per
// line.
func fleetCardValue(key string, v any) string {
	switch val := v.(type) {
	case nil:
		return "-"
	case string:
		return cliutil.ScrubTerminal(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val > 0 && fleetTimeKey(key) {
			return time.Unix(int64(val), 0).Local().Format("2006-01-02 15:04")
		}
		if val == math.Trunc(val) && math.Abs(val) < 1e15 {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case []any:
		if len(val) == 0 {
			return "(none)"
		}
		lines := make([]string, 0, len(val))
		for _, e := range val {
			lines = append(lines, "- "+fleetCardValue("", e))
		}
		return strings.Join(lines, "\n")
	default:
		raw, _ := json.Marshal(val) // an object: compact JSON, keys sorted
		return cliutil.ScrubTerminal(string(raw))
	}
}

// declaredJSONFields lists the JSON names of t's struct fields (through
// pointers and slices) in declaration order, embedded structs inline.
func declaredJSONFields(t reflect.Type) []string {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if f.Anonymous && name == "" {
			out = append(out, declaredJSONFields(f.Type)...)
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// checkLimits rejects threshold flags that would silently disable a check: a
// non-finite or out-of-range percentage or multiple, or a negative
// temperature. 0 keeps its documented meaning (check off).
func checkLimits(fullPct, overrun float64, diskTemp int64) error {
	switch {
	case math.IsNaN(fullPct) || math.IsInf(fullPct, 0) || fullPct < 0 || fullPct > 100:
		return usageErr(fmt.Errorf("--full-pct must be between 0 and 100, got %v", fullPct))
	case math.IsNaN(overrun) || math.IsInf(overrun, 0) || overrun < 0:
		return usageErr(fmt.Errorf("--overrun must be a finite number of 0 or more, got %v", overrun))
	case diskTemp < 0:
		return usageErr(fmt.Errorf("--disk-temp must be 0 or more, got %d", diskTemp))
	}
	return nil
}

func parseWindow(name, v string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a positive duration (use 24h, 36h, 7d, 2w)", name, v))
	}
	return d, nil
}

// noFleetHint tells a human how to populate the mirror.
func noFleetHint(w io.Writer) {
	fmt.Fprintln(w, "hint: no NAS configured; run 'synology-cli fleet add <name> --url https://<nas>:5001' then 'synology-cli fleet sync'")
}

// apiInfo asks SYNO.API.Info which of the given spec paths' APIs this NAS
// exposes, keyed by path.
func apiInfo(ctx context.Context, c *dsm.Client, paths ...string) (map[string]bool, error) {
	raw, err := c.Get(ctx, fleet.PathAPIInfo, nil)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing API list: %w", err)
	}
	out := map[string]bool{}
	for _, p := range paths {
		_, out[p] = m[fleet.APIOf(p)]
	}
	return out, nil
}

// fleetReport is the shared body of the local fleet/abb report commands: open
// the mirror read-only, run the query over the configured NAS units, print,
// and exit 6 when findings exist (unless --no-fail).
type fleetReport struct {
	dbPath     string
	maxSyncAge string
	noFail     bool
}

func (r *fleetReport) bind(cmd *cobra.Command, findings bool) {
	cmd.Flags().StringVar(&r.dbPath, "db", "", "Fleet mirror path (default <data dir>/fleet.db)")
	cmd.Flags().StringVar(&r.maxSyncAge, "max-sync-age", "26h", "A NAS whose last good sync is older than this reads as not synced, never as OK")
	if findings {
		cmd.Flags().BoolVar(&r.noFail, "no-fail", false, "Exit 0 even when findings exist (default: exit 6)")
	}
}

// run's empty is printed for humans when nothing needs attention; %d is the
// number of NAS units checked.
func (r *fleetReport) run(cmd *cobra.Command, flags *rootFlags, empty string,
	query func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (rows any, findings int, err error)) error {
	maxSyncAge, err := parseWindow("max-sync-age", r.maxSyncAge)
	if err != nil {
		return err
	}
	nas, _, _, err := fleetNAS(flags)
	if err != nil {
		return err
	}
	path, err := fleetDBPath(r.dbPath)
	if err != nil {
		return err
	}
	if len(nas) == 0 {
		// An empty answer from an empty fleet would read as "all clear" to an
		// RMM check; nothing was checked, so it is a configuration error.
		detail := "a fleet mirror exists but no NAS is registered"
		if _, statErr := os.Stat(path); statErr != nil {
			detail = "fleet mirror: " + statErr.Error()
		}
		return configErr(fmt.Errorf("no NAS configured: run 'synology-cli fleet add <name> --url https://<nas>:5001' then 'synology-cli fleet sync', or set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD (%s)", detail))
	}
	db, closeDB, err := openFleetRead(cmd.Context(), path, true)
	if err != nil {
		return err
	}
	defer closeDB()
	// One read transaction: every query in the report sees the same snapshot,
	// even while a fleet sync commits.
	tx, err := db.BeginTx(cmd.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, findings, err := query(cmd.Context(), tx, nas, maxSyncAge)
	if err != nil {
		return err
	}

	if err := printFleetRows(cmd, flags, rows, fmt.Sprintf(empty, len(nas))); err != nil {
		return err
	}
	if findings == noMatch {
		return notFoundErr(errors.New("no match"))
	}
	if findings > 0 && !r.noFail {
		return findingsErr(findings)
	}
	return nil
}

// noMatch is a findings value a report returns to exit 3 after printing an
// empty result (abb device with no matching host on fresh mirrors).
const noMatch = -1
