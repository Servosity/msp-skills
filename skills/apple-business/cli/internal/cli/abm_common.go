// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written helpers shared by the Apple Business novel commands.

package cli

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"apple-business-pp-cli/internal/client"
	"apple-business-pp-cli/internal/config"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// orgDevice is the subset of OrgDevice the novel commands read.
type orgDevice struct {
	ID         string `json:"id"`
	Attributes struct {
		SerialNumber                 string   `json:"serialNumber"`
		DeviceModel                  string   `json:"deviceModel"`
		ProductFamily                string   `json:"productFamily"`
		ProductType                  string   `json:"productType"`
		Status                       string   `json:"status"`
		OrderNumber                  string   `json:"orderNumber"`
		PurchaseSourceType           string   `json:"purchaseSourceType"`
		PurchaseSourceID             string   `json:"purchaseSourceId"`
		AddedToOrgDateTime           string   `json:"addedToOrgDateTime"`
		ReleasedFromOrgDateTime      *string  `json:"releasedFromOrgDateTime"`
		WifiMacAddress               string   `json:"wifiMacAddress"`
		BluetoothMacAddress          string   `json:"bluetoothMacAddress"`
		EthernetMacAddress           []string `json:"ethernetMacAddress"`
		IMEI                         []string `json:"imei"`
		MEID                         []string `json:"meid"`
		EID                          string   `json:"eid"`
		MdmMigrationStatus           string   `json:"mdmMigrationStatus"`
		MdmMigrationDeadlineDateTime string   `json:"mdmMigrationDeadlineDateTime"`
	} `json:"attributes"`
}

func (d orgDevice) serial() string {
	if d.Attributes.SerialNumber != "" {
		return d.Attributes.SerialNumber
	}
	return d.ID
}

func (d orgDevice) released() bool {
	return d.Attributes.ReleasedFromOrgDateTime != nil && *d.Attributes.ReleasedFromOrgDateTime != ""
}

type mdmServer struct {
	ID         string `json:"id"`
	Attributes struct {
		ServerName             string   `json:"serverName"`
		ServerType             string   `json:"serverType"`
		Status                 string   `json:"status"`
		DefaultProductFamilies []string `json:"defaultProductFamilies"`
		DeviceCount            *int     `json:"deviceCount"`
		LastConnectedDateTime  string   `json:"lastConnectedDateTime"`
	} `json:"attributes"`
}

type appleCareCoverage struct {
	ID         string `json:"id"`
	Attributes struct {
		Status          string  `json:"status"`
		Description     string  `json:"description"`
		StartDateTime   *string `json:"startDateTime"`
		EndDateTime     *string `json:"endDateTime"`
		AgreementNumber *string `json:"agreementNumber"`
		PaymentType     string  `json:"paymentType"`
		IsRenewable     bool    `json:"isRenewable"`
		IsCanceled      bool    `json:"isCanceled"`
	} `json:"attributes"`
}

// activationLock decodes Apple's attributes; lockView is what we print.
type activationLock struct {
	IsLocked bool   `json:"isLocked"`
	LockType string `json:"lockType"`
}

type lockView struct {
	IsLocked bool   `json:"is_locked"`
	LockType string `json:"lock_type"`
}

// normalizeHW folds MAC/IMEI spellings (aa:bb, AA-BB, aabb.ccdd) to one key.
func normalizeHW(s string) string {
	return strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(s)))
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// countByKey is a key -> count pair for grouped report output.
type countByKey struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// sortCounts orders by count descending, then key, for stable output.
func sortCounts(c []countByKey) {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Count > c[j].Count || (c[i].Count == c[j].Count && c[i].Key < c[j].Key)
	})
}

// fetchFailure records one failed API call inside a multi-call command.
type fetchFailure struct {
	Serial string `json:"serial,omitempty"`
	Kind   string `json:"kind"`
	Error  string `json:"error"`
}

// ---- local store ----------------------------------------------------------

// abm_enrichment holds the per-device endpoints (AppleCare, Activation Lock,
// assigned server) that sync cannot fetch; kind is applecare | lock | server.
const enrichmentDDL = `CREATE TABLE IF NOT EXISTS abm_enrichment (
	serial TEXT NOT NULL,
	kind TEXT NOT NULL,
	data TEXT NOT NULL,
	enriched_at TEXT NOT NULL,
	PRIMARY KEY (serial, kind)
)`

const metaDDL = `CREATE TABLE IF NOT EXISTS abm_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`

func storeDBPath(dbPath string) string {
	if dbPath == "" {
		dbPath = defaultDBPath("apple-business-cli")
	}
	if abs, err := filepath.Abs(dbPath); err == nil {
		return abs
	}
	return dbPath
}

// verifyStoreOwner guards an explicit --db against mixing customers: the
// default store is already scoped to the active API account, but any other
// store must carry the active account's Client ID (recorded by a default-path
// `devices enrich`).
func verifyStoreOwner(ctx context.Context, db *store.Store, path, dbPath, cur string) error {
	if dbPath == "" || path == storeDBPath("") {
		return nil
	}
	stored := readMeta(ctx, db, "client_id")
	if cur == "" || stored != cur {
		return usageErr(fmt.Errorf("store %s is not verified as API account %s's (recorded account: %q); use the default store, or run 'devices enrich' as that account first",
			path, firstNonEmpty(cur, "<unknown>"), stored))
	}
	return nil
}

// orgStorePaths lists the stores a report reads. Each API account (one per
// customer org) gets its own credential-scoped data-<hash>.db next to the
// active one, so --all-orgs is every store in that directory.
// ponytail: same-directory glob; add per --home roots if MSPs split homes.
func orgStorePaths(dbPath string, all bool) []string {
	current := storeDBPath(dbPath)
	if !all {
		return []string{current}
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(current), "data*.db"))
	out := []string{current}
	for _, m := range matches {
		if abs, err := filepath.Abs(m); err == nil && abs != current {
			out = append(out, abs)
		}
	}
	sort.Strings(out[1:])
	return out
}

// storeHasData reports whether a store was ever synced or enriched, so
// --all-orgs can skip empty leftovers from earlier store names.
func storeHasData(ctx context.Context, db *store.Store) bool {
	return db.GetLastSyncedAt("devices") != "" || readMeta(ctx, db, "client_id") != ""
}

// serverLinkID decodes the "server" enrichment row.
func serverLinkID(e enrichRow) string {
	var v struct {
		ServerID string `json:"server_id"`
	}
	_ = json.Unmarshal(e.Data, &v)
	return v.ServerID
}

// lockFrom decodes a "lock" enrichment row; ok is false when unusable.
func lockFrom(e enrichRow) (activationLock, bool) {
	var l activationLock
	if json.Unmarshal(e.Data, &l) != nil {
		return l, false
	}
	return l, true
}

func tableExists(ctx context.Context, db *store.Store, name string) bool {
	var n int
	err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return err == nil && n > 0
}

// loadResourceRows drains every stored row of one resource type.
func loadResourceRows(ctx context.Context, db *store.Store, resourceType string) ([]json.RawMessage, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", resourceType, err)
	}
	out := make([]json.RawMessage, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s: %w", resourceType, err)
		}
		out = append(out, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

func loadLocalDevices(ctx context.Context, db *store.Store) ([]orgDevice, error) {
	raw, err := loadResourceRows(ctx, db, "devices")
	if err != nil {
		return nil, err
	}
	out := make([]orgDevice, 0, len(raw))
	for _, r := range raw {
		var d orgDevice
		if json.Unmarshal(r, &d) == nil && d.serial() != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

func loadLocalServers(ctx context.Context, db *store.Store) ([]mdmServer, error) {
	raw, err := loadResourceRows(ctx, db, "mdm_servers")
	if err != nil {
		return nil, err
	}
	out := make([]mdmServer, 0, len(raw))
	for _, r := range raw {
		var s mdmServer
		if json.Unmarshal(r, &s) == nil && s.ID != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

type enrichRow struct {
	Data       json.RawMessage
	EnrichedAt time.Time
}

// loadEnrichment returns serial -> row for one kind; empty when never enriched.
func loadEnrichment(ctx context.Context, db *store.Store, kind string) (map[string]enrichRow, error) {
	out := map[string]enrichRow{}
	if !tableExists(ctx, db, "abm_enrichment") {
		return out, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT serial, data, enriched_at FROM abm_enrichment WHERE kind = ?`, kind)
	if err != nil {
		return nil, fmt.Errorf("query enrichment: %w", err)
	}
	for rows.Next() {
		var serial, data, at string
		if err := rows.Scan(&serial, &data, &at); err != nil {
			_ = rows.Close()
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, at)
		out[serial] = enrichRow{Data: json.RawMessage(data), EnrichedAt: t}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

func saveEnrichment(ctx context.Context, db *store.Store, serial, kind string, data json.RawMessage) error {
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO abm_enrichment (serial, kind, data, enriched_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(serial, kind) DO UPDATE SET data = excluded.data, enriched_at = excluded.enriched_at`,
		serial, kind, string(data), time.Now().UTC().Format(time.RFC3339))
	return err
}

func readMeta(ctx context.Context, db *store.Store, key string) string {
	if !tableExists(ctx, db, "abm_meta") {
		return ""
	}
	var v sql.NullString
	_ = db.DB().QueryRowContext(ctx, `SELECT value FROM abm_meta WHERE key = ?`, key).Scan(&v)
	return v.String
}

func writeMeta(ctx context.Context, db *store.Store, key, value string) error {
	_, err := db.DB().ExecContext(ctx, `INSERT INTO abm_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// hintStoreUnsynced warns, in multi-org mode, that an included store has
// never synced devices, so its empty rows mean unknown, not healthy (the
// single-org path gets the same signal from hintIfUnsynced).
func hintStoreUnsynced(cmd *cobra.Command, db *store.Store, label string) {
	if db.GetLastSyncedAt("devices") == "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: store %s has never synced devices; its rows are empty, not healthy. Run 'apple-business-cli sync' with that org's API account.\n", label)
	}
}

// orgLabel names a store in cross-org output: the API account's Client ID
// recorded by `devices enrich`, else the store file name.
func orgLabel(ctx context.Context, db *store.Store, path string) string {
	if id := readMeta(ctx, db, "client_id"); id != "" {
		return id
	}
	return filepath.Base(path)
}

// currentClientID is the API account the CLI is acting as, in the same order
// the store scope and token mint use (config.ReconcileAppleAccount).
// frozenClient loads the config once and pins the default store to that same
// account, so a concurrent `auth login` cannot split the store a command
// writes from the account its requests run as.
func frozenClient(flags *rootFlags) (*client.Client, string, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, "", err
	}
	setDefaultDBScopeIdentity(c.Config.StoreScopeCredential(), c.Config.StoreScopeLegacyCredential(), c.Config.BaseURL)
	return c, firstNonEmpty(c.Config.AppleBusinessClientId, c.Config.ClientID), nil
}

func currentClientID(flags *rootFlags) string {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return ""
	}
	return firstNonEmpty(cfg.AppleBusinessClientId, cfg.ClientID)
}

// ---- live API -------------------------------------------------------------

type pagedEnvelope struct {
	Data []json.RawMessage `json:"data"`
	Meta struct {
		Paging struct {
			NextCursor string `json:"nextCursor"`
		} `json:"paging"`
	} `json:"meta"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// listAll walks an Apple cursor-paged collection (opaque cursor from
// meta.paging.nextCursor or links.next). Pass {"limit": "1000"} where the
// endpoint documents a limit.
func listAll(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) ([]json.RawMessage, error) {
	out, _, _, err := listAllPages(ctx, c, path, params, maxPages)
	return out, err
}

// listAllPages also reports pages read and whether more pages remained when
// maxPages stopped the walk. A repeated cursor ends the walk.
func listAllPages(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) ([]json.RawMessage, int, bool, error) {
	out := make([]json.RawMessage, 0)
	cursor := ""
	seen := map[string]bool{}
	page := 0
	for ; maxPages <= 0 || page < maxPages; page++ {
		p := map[string]string{}
		for k, v := range params {
			p[k] = v
		}
		if cursor != "" {
			p["cursor"] = cursor
		}
		raw, err := c.Get(ctx, path, p)
		if err != nil {
			return out, page, false, err
		}
		var env pagedEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return out, page, false, fmt.Errorf("parsing %s page: %w", path, err)
		}
		out = append(out, env.Data...)
		next := env.Meta.Paging.NextCursor
		if next == "" && env.Links.Next != "" {
			if u, err := url.Parse(env.Links.Next); err == nil {
				next = u.Query().Get("cursor")
			}
		}
		if next == "" || len(env.Data) == 0 {
			return out, page + 1, false, nil
		}
		if seen[next] {
			return out, page + 1, false, fmt.Errorf("%s: pagination cursor repeated; listing is incomplete", path)
		}
		seen[next] = true
		cursor = next
	}
	return out, page, true, nil
}

func liveServers(ctx context.Context, c *client.Client) ([]mdmServer, error) {
	servers, _, err := liveServersRaw(ctx, c)
	return servers, err
}

// liveServersRaw also returns Apple's untouched records for storing.
func liveServersRaw(ctx context.Context, c *client.Client) ([]mdmServer, []json.RawMessage, error) {
	raw, err := listAll(ctx, c, "/mdmServers", map[string]string{"limit": "1000"}, 0)
	if err != nil {
		return nil, nil, err
	}
	out := make([]mdmServer, 0, len(raw))
	kept := make([]json.RawMessage, 0, len(raw))
	for _, r := range raw {
		var s mdmServer
		if json.Unmarshal(r, &s) == nil && s.ID != "" {
			out = append(out, s)
			kept = append(kept, r)
		}
	}
	return out, kept, nil
}

// resolveServer matches a device management service by exact id or by
// case-insensitive exact name; an ambiguous or unknown name is an error that
// lists the candidates.
func resolveServer(servers []mdmServer, nameOrID string) (mdmServer, error) {
	want := strings.TrimSpace(nameOrID)
	var matches []mdmServer
	for _, s := range servers {
		if s.ID == want {
			return s, nil
		}
		if strings.EqualFold(s.Attributes.ServerName, want) {
			matches = append(matches, s)
		}
	}
	names := make([]string, 0, len(servers))
	for _, s := range servers {
		names = append(names, fmt.Sprintf("%q (%s)", s.Attributes.ServerName, s.ID))
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return mdmServer{}, fmt.Errorf("no device management service named %q; available: %s", want, strings.Join(names, ", "))
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return mdmServer{}, fmt.Errorf("%d device management services are named %q (%s); pass the id instead", len(matches), want, strings.Join(ids, ", "))
	}
}

// deviceIDs maps a serial to the device's resource ID, from a store the
// active account owns. Apple documents the two as the same value; a serial
// the store does not know is used as its own ID.
type deviceIDs map[string]string

func (m deviceIDs) of(serial string) string {
	return firstNonEmpty(m[strings.ToUpper(serial)], serial)
}

func (m deviceIDs) all(serials []string) []string {
	out := make([]string, 0, len(serials))
	for _, s := range serials {
		out = append(out, m.of(s))
	}
	return out
}

func loadDeviceIDs(ctx context.Context, path string) deviceIDs {
	m := deviceIDs{}
	if _, err := os.Stat(path); err != nil {
		return m
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return m
	}
	defer db.Close()
	devices, _ := loadLocalDevices(ctx, db)
	for _, d := range devices {
		if d.ID != "" {
			m[strings.ToUpper(d.serial())] = d.ID
		}
	}
	return m
}

// assignedServerID returns the server id a device is assigned to, "" if none.
func assignedServerID(ctx context.Context, c *client.Client, serial string) (string, error) {
	// Assignment safety checks must never come from the response cache.
	raw, err := c.GetNoCache(ctx, "/orgDevices/"+url.PathEscape(serial)+"/relationships/assignedServer", nil)
	if err != nil {
		return "", err
	}
	var env struct {
		Data *struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("parsing assigned server for %s: %w", serial, err)
	}
	if env.Data == nil {
		return "", nil
	}
	return env.Data.ID, nil
}

// activityBody builds the orgDeviceActivities POST body per Apple's "Assign
// or Unassign Devices" reference.
func activityBody(activityType, serverID string, serials []string, deadline string) map[string]any {
	devices := make([]map[string]string, 0, len(serials))
	for _, s := range serials {
		devices = append(devices, map[string]string{"type": "orgDevices", "id": s})
	}
	attrs := map[string]any{"activityType": activityType}
	if deadline != "" {
		attrs["activityTypeMetadata"] = map[string]string{"mdmMigrationDeadlineDateTime": deadline}
	}
	rel := map[string]any{"devices": map[string]any{"data": devices}}
	if serverID != "" {
		rel["mdmServer"] = map[string]any{"data": map[string]string{"type": "mdmServers", "id": serverID}}
	}
	return map[string]any{"data": map[string]any{"type": "orgDeviceActivities", "attributes": attrs, "relationships": rel}}
}

type activityState struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	SubStatus         string `json:"sub_status"`
	CompletedDateTime string `json:"completed_date_time,omitempty"`
	DownloadURL       string `json:"download_url,omitempty"`
}

func parseActivity(raw json.RawMessage) (activityState, error) {
	var env struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Status            string `json:"status"`
				SubStatus         string `json:"subStatus"`
				CompletedDateTime string `json:"completedDateTime"`
				DownloadURL       string `json:"downloadUrl"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return activityState{}, fmt.Errorf("parsing activity: %w", err)
	}
	a := env.Data.Attributes
	return activityState{ID: env.Data.ID, Status: a.Status, SubStatus: a.SubStatus, CompletedDateTime: a.CompletedDateTime, DownloadURL: a.DownloadURL}, nil
}

// activityDone: Apple's top-level statuses are IN_PROGRESS, COMPLETED,
// STOPPED and FAILED; only IN_PROGRESS keeps a poll going.
func activityDone(s activityState) bool {
	return s.Status != "" && s.Status != "IN_PROGRESS"
}

// activitySucceeded: COMPLETED with COMPLETED_WITH_SUCCESS (or no substatus).
func activitySucceeded(s activityState) bool {
	return s.Status == "COMPLETED" && (s.SubStatus == "" || s.SubStatus == "COMPLETED_WITH_SUCCESS")
}

func waitActivity(ctx context.Context, c *client.Client, id string, every time.Duration) (activityState, error) {
	for {
		raw, err := c.GetNoCache(ctx, "/orgDeviceActivities/"+url.PathEscape(id), nil)
		if err != nil {
			return activityState{ID: id}, err
		}
		st, err := parseActivity(raw)
		if err != nil || activityDone(st) {
			return st, err
		}
		select {
		case <-ctx.Done():
			return st, fmt.Errorf("activity %s still %s/%s when the wait ended: %w", id, st.Status, st.SubStatus, ctx.Err())
		case <-time.After(every):
		}
	}
}

// readSerials merges positional serials with a --file of one serial per line
// (first CSV column; "-" reads stdin), de-duplicated in order.
func readSerials(args []string, file string, stdin io.Reader) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(args))
	add := func(s string) {
		s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"`))
		header := strings.ReplaceAll(strings.ToLower(s), " ", "")
		if s == "" || header == "serial" || header == "serialnumber" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, a := range args {
		for _, s := range strings.Split(a, ",") {
			add(s)
		}
	}
	if file != "" {
		var data []byte
		var err error
		if file == "-" {
			data, err = io.ReadAll(io.LimitReader(stdin, 16<<20))
		} else {
			data, err = os.ReadFile(filepath.Clean(file))
		}
		if err != nil {
			return nil, fmt.Errorf("reading --file: %w", err)
		}
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			got, err := serialsFromJSON(data)
			if err != nil {
				return nil, fmt.Errorf("reading --file: %w", err)
			}
			for _, s := range got {
				add(s)
			}
			return out, nil
		}
		got, err := serialsFromCSV(trimmed)
		if err != nil {
			return nil, fmt.Errorf("reading --file: %w", err)
		}
		for _, s := range got {
			add(s)
		}
	}
	return out, nil
}

// familyKey folds device productFamily ("AppleTV") and server
// defaultProductFamilies ("APPLE_TV") spellings to one key.
func familyKey(s string) string {
	return strings.ToUpper(strings.NewReplacer("_", "", " ", "").Replace(s))
}

// coverageSummary reduces a device's AppleCare records to what reports need.
// Active means status ACTIVE and not canceled. An active record with no end
// date (AppleCare+ for Business subscriptions) is open-ended: covered, never
// expiring. End is the latest end date among dated active records.
type coverageView struct {
	Active          bool       `json:"active"`
	OpenEnded       bool       `json:"open_ended"`
	End             *time.Time `json:"end,omitempty"`
	Description     string     `json:"description,omitempty"`
	AgreementNumber string     `json:"agreement_number,omitempty"`
	Renewable       bool       `json:"renewable"`
	Records         int        `json:"records"`
}

func coverageSummary(raw json.RawMessage) coverageView {
	var env struct {
		Data []appleCareCoverage `json:"data"`
	}
	var list []appleCareCoverage
	if json.Unmarshal(raw, &env) == nil && env.Data != nil {
		list = env.Data
	} else {
		_ = json.Unmarshal(raw, &list)
	}
	v := coverageView{Records: len(list)}
	for _, c := range list {
		a := c.Attributes
		if !strings.EqualFold(a.Status, "ACTIVE") || a.IsCanceled {
			continue
		}
		v.Active = true
		if a.EndDateTime == nil || *a.EndDateTime == "" {
			v.OpenEnded = true
			if v.End == nil {
				v.Description, v.Renewable = a.Description, a.IsRenewable
			}
			continue
		}
		end, ok := parseTime(*a.EndDateTime)
		if !ok || (v.End != nil && !end.After(*v.End)) {
			continue
		}
		e := end
		v.End = &e
		v.Description, v.Renewable = a.Description, a.IsRenewable
		v.AgreementNumber = ""
		if a.AgreementNumber != nil {
			v.AgreementNumber = *a.AgreementNumber
		}
	}
	return v
}

// expiringWithin: dated coverage ending in [now, now+window], no open-ended
// coverage keeping the device covered.
func (v coverageView) expiringWithin(now time.Time, window time.Duration) bool {
	return v.Active && !v.OpenEnded && v.End != nil && !v.End.Before(now) && !v.End.After(now.Add(window))
}

// uncovered: no active coverage, or only dated coverage that already ended.
func (v coverageView) uncovered(now time.Time) bool {
	return !v.Active || (!v.OpenEnded && v.End != nil && v.End.Before(now))
}

// serialsFromJSON accepts JSON piped back in: arrays of strings or objects,
// NDJSON, and envelopes with devices/results/data arrays. An object's serial
// is "serial", "serialNumber" or attributes.serialNumber; a bare "id" counts
// only on orgDevices records, so other resources' ids never become serials.
func serialsFromJSON(data []byte) ([]string, error) {
	out := make([]string, 0)
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, item := range t {
				walk(item)
			}
		case string:
			out = append(out, t)
		case map[string]any:
			for _, k := range []string{"devices", "results", "data"} {
				if inner, ok := t[k]; ok {
					switch inner.(type) {
					case []any, map[string]any:
						walk(inner)
						return
					}
				}
			}
			for _, k := range []string{"serial", "serialNumber"} {
				if s, ok := t[k].(string); ok && s != "" {
					out = append(out, s)
					return
				}
			}
			if attrs, ok := t["attributes"].(map[string]any); ok {
				if s, ok := attrs["serialNumber"].(string); ok && s != "" {
					out = append(out, s)
					return
				}
			}
			if t["type"] == "orgDevices" {
				if s, ok := t["id"].(string); ok && s != "" {
					out = append(out, s)
				}
			}
		}
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("invalid JSON input: %w", err)
		}
		walk(v)
	}
}

// serialsFromCSV takes one serial per line, or, when the first line is a
// header (this CLI's --csv output sorts columns), the serial/serialNumber
// column. A header with no serial column is refused rather than guessed.
func serialsFromCSV(text string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV input: %w", err)
	}
	col := 0
	if len(rows) > 0 && len(rows[0]) > 1 {
		col = -1
		for i, h := range rows[0] {
			h = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(h)), " ", "")
			if h == "serial" || h == "serialnumber" {
				col = i
			}
		}
		if col < 0 {
			if looksLikeHeader(rows[0]) {
				return nil, fmt.Errorf("CSV header has no serial or serialNumber column")
			}
			col = 0
		} else {
			rows = rows[1:]
		}
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if col < len(row) {
			out = append(out, row[col])
		}
	}
	return out, nil
}

// looksLikeHeader: a first CSV row of lowercase/snake identifiers.
func looksLikeHeader(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" || strings.ToLower(cell) != cell || strings.ContainsAny(cell, "0123456789") {
			return false
		}
	}
	return true
}

// fanOut runs fn for jobs 0..n-1 on up to `workers` goroutines. Each worker
// owns its own API client: the generated client rewrites its Config while
// minting a token, so sharing one client across goroutines races on token
// refresh. The token is refreshed serially first (and an auth failure stops
// here) so workers do not all mint at once.
func fanOut(ctx context.Context, flags *rootFlags, c *client.Client, workers, n int, fn func(wc *client.Client, i int)) error {
	if n == 0 {
		return nil
	}
	if err := ensureFreshToken(ctx, c); err != nil {
		return err
	}
	workers = max(1, min(workers, n, 16))
	clients := []*client.Client{c}
	for len(clients) < workers {
		// A frozen copy of this run's account: a worker never re-reads
		// credentials from disk, so a login elsewhere mid-run cannot switch
		// accounts under it, and it shares no mutable config with others.
		wc := client.New(c.Config.WorkerCopy(), flags.timeout, flags.rateLimit)
		if flags.timeoutExplicit {
			wc.SetTimeoutExplicit(true)
		}
		wc.DryRun, wc.NoCache = c.DryRun, c.NoCache
		wc.ShareLimiter(c) // one --rate-limit budget across workers
		if err := ApplyClientHooks(wc); err != nil {
			return err
		}
		clients = append(clients, wc)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for _, wc := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				fn(wc, i)
			}
		}()
	}
dispatch:
	for i := 0; i < n; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	return ctx.Err()
}

// ensureFreshToken mints serially when the token is missing, has unknown
// expiry, or expires within five minutes, by clearing it and making one
// cheap uncached call.
func ensureFreshToken(ctx context.Context, c *client.Client) error {
	cfg := c.Config
	if cfg == nil || c.IsDryRun() {
		return nil
	}
	// Only force a mint when client credentials can re-mint; a token from
	// `auth set-token` must be left alone.
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		if strings.TrimSpace(os.Getenv("APPLE_BUSINESS_CLIENT_ID")) == "" || strings.TrimSpace(os.Getenv("APPLE_BUSINESS_PRIVATE_KEY")) == "" {
			return nil
		}
	}
	if cfg.AccessToken != "" && !cfg.TokenExpiry.IsZero() && time.Until(cfg.TokenExpiry) > 5*time.Minute {
		return nil
	}
	cfg.AccessToken = ""
	_, err := c.GetNoCache(ctx, "/mdmServers", map[string]string{"limit": "1"})
	return err
}

// apiUnreachable reports whether err means the API could not be reached
// (network failure, 5xx, 429): the only case a live read may answer from the
// local store. Credential, token-exchange and other client errors never do.
func apiUnreachable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, client.ErrTokenExchange) {
		return false
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500 || apiErr.StatusCode == 429
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// flagDB is the --db value of the running command ("" when unset).
func flagDB(cmd *cobra.Command) string {
	if f := cmd.Flags().Lookup("db"); f != nil {
		return f.Value.String()
	}
	return ""
}
