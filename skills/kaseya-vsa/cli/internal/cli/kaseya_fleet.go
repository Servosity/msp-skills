package cli

// Shared helpers for the hand-written Kaseya VSA 10 fleet commands (health,
// patch-exposure, software, audit-logs digest, devices brief, scripts/tasks
// run-wait). Kept in one file so regen preserves it as a whole.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/client"
	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/store"
)

const fleetCLIName = "kaseya-vsa-cli"

// fleetRow is one synced resource row: its stored JSON plus when sync stored it.
type fleetRow struct {
	ID       string
	Data     json.RawMessage
	SyncedAt time.Time
}

// openFleetStore resolves the same per-credential database path sync writes,
// returns (nil, nil) when no mirror exists yet, and prints sync hints for the
// resources the command reads.
func openFleetStore(cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath(fleetCLIName)
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: %s sync --resources %s --db %s\n",
			dbPath, fleetCLIName, strings.Join(resources, ","), dbPath)
		return nil, nil
	}
	db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database %s: %w", dbPath, err)
	}
	for _, r := range resources {
		if !hintIfUnsynced(cmd, db, r) {
			hintIfStale(cmd, db, r, flags.maxAge)
		}
	}
	return db, nil
}

// loadFleetRows reads every synced row of one resource type. Drain-first: the
// rows are fully scanned and closed before the caller runs any other query.
func loadFleetRows(ctx context.Context, db *store.Store, resourceType string) ([]fleetRow, error) {
	rows, err := db.DB().QueryContext(ctx,
		`SELECT id, data, COALESCE(synced_at, '') FROM resources WHERE resource_type = ? ORDER BY id`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", resourceType, err)
	}
	out := make([]fleetRow, 0)
	for rows.Next() {
		var id, data string
		var synced sql.NullString
		if err := rows.Scan(&id, &data, &synced); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s row: %w", resourceType, err)
		}
		t, _ := parseVSATime(synced.String)
		out = append(out, fleetRow{ID: id, Data: json.RawMessage(data), SyncedAt: t})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate %s rows: %w", resourceType, err)
	}
	return out, rows.Close()
}

// loadNameIndex maps a synced resource's numeric Id to its Name
// (organizations, sites). Names always come from IDs: "Org - Site - Group"
// strings are never split, because client names can contain " - ".
func loadNameIndex(ctx context.Context, db *store.Store, resourceType string) (map[int64]string, error) {
	rows, err := loadFleetRows(ctx, db, resourceType)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		var v struct {
			ID   int64  `json:"Id"`
			Name string `json:"Name"`
		}
		if json.Unmarshal(r.Data, &v) == nil && v.ID != 0 {
			out[v.ID] = v.Name
		}
	}
	return out, nil
}

// fleetAsset is the subset of a synced asset record the fleet commands read.
// Pointer and RawMessage fields distinguish "absent / section not synced" from
// a real zero value or an empty list.
type fleetAsset struct {
	Identifier        string          `json:"Identifier"`
	Name              string          `json:"Name"`
	GroupName         string          `json:"GroupName"`
	OrganizationID    int64           `json:"OrganizationId"`
	SiteID            int64           `json:"SiteId"`
	LastSeenOnline    string          `json:"LastSeenOnline"`
	CPUUsage          *float64        `json:"CpuUsage"`
	MemoryUsage       *float64        `json:"MemoryUsage"`
	FirewallEnabled   *bool           `json:"FirewallEnabled"`
	AntivirusEnabled  *string         `json:"AntivirusEnabled"`
	AntivirusUpToDate *string         `json:"AntivirusUpToDate"`
	UacEnabled        *bool           `json:"UacEnabled"`
	Disks             []fleetDisk     `json:"Disks"`
	AvailableUpdates  json.RawMessage `json:"AvailableUpdates"`
	InstalledSoftware json.RawMessage `json:"InstalledSoftware"`
	updates           []fleetUpdate   // decoded AvailableUpdates
	software          []fleetSoftware // decoded InstalledSoftware
	syncedAt          time.Time
}

type fleetDisk struct {
	Name           string   `json:"Name"`
	System         bool     `json:"System"`
	FreePercentage *float64 `json:"FreePercentage"`
}

type fleetUpdate struct {
	KbArticle   string   `json:"KbArticle"`
	Title       string   `json:"Title"`
	Severity    string   `json:"Severity"`
	CvssScore   *float64 `json:"CvssScore"`
	CveCodes    []string `json:"CveCodes"`
	Category    string   `json:"Category"`
	ReleaseDate string   `json:"ReleaseDate"`
}

type fleetSoftware struct {
	Name      string `json:"Name"`
	Publisher string `json:"Publisher"`
	Version   string `json:"Version"`
}

// hasUpdateData / hasSoftwareData report whether the section was synced at all
// (an absent key is "no data", an empty array is a real empty answer).
func (a *fleetAsset) hasUpdateData() bool   { return sectionPresent(a.AvailableUpdates) }
func (a *fleetAsset) hasSoftwareData() bool { return sectionPresent(a.InstalledSoftware) }

func sectionPresent(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null"
}

func loadFleetAssets(ctx context.Context, db *store.Store) ([]*fleetAsset, error) {
	rows, err := loadFleetRows(ctx, db, "assets")
	if err != nil {
		return nil, err
	}
	out := make([]*fleetAsset, 0, len(rows))
	for _, r := range rows {
		a := &fleetAsset{}
		if err := json.Unmarshal(r.Data, a); err != nil {
			return nil, fmt.Errorf("decode asset %s: %w", r.ID, err)
		}
		if a.Identifier == "" {
			a.Identifier = r.ID
		}
		if a.hasUpdateData() {
			if err := json.Unmarshal(a.AvailableUpdates, &a.updates); err != nil {
				return nil, fmt.Errorf("decode AvailableUpdates for %s: %w", a.Name, err)
			}
		}
		if a.hasSoftwareData() {
			if err := json.Unmarshal(a.InstalledSoftware, &a.software); err != nil {
				return nil, fmt.Errorf("decode InstalledSoftware for %s: %w", a.Name, err)
			}
		}
		a.syncedAt = r.SyncedAt
		out = append(out, a)
	}
	return out, nil
}

// orgFilter matches an organization by numeric ID or case-insensitive name substring.
func orgFilter(filter string, orgID int64, orgName string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	if id, err := strconv.ParseInt(filter, 10, 64); err == nil {
		return id == orgID
	}
	return strings.Contains(strings.ToLower(orgName), strings.ToLower(filter))
}

// vsaData unwraps the VSA 10 `{"Data": ..., "Meta": {...}}` envelope.
func vsaData(raw json.RawMessage) json.RawMessage {
	var env struct {
		Data json.RawMessage `json:"Data"`
		Meta json.RawMessage `json:"Meta"`
	}
	if json.Unmarshal(raw, &env) == nil && (env.Meta != nil || env.Data != nil) {
		return env.Data
	}
	return raw
}

func vsaGet(ctx context.Context, c *client.Client, path string, params map[string]string) (json.RawMessage, error) {
	raw, err := c.Get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	return vsaData(raw), nil
}

// requireTenantBaseURL fails fast with a config error naming the env vars when
// no VSA 10 server is configured, instead of surfacing a raw DNS error for the
// spec's placeholder default. The generated config takes {server} only from
// KASEYA_VSA_SERVER; a concrete base URL (KASEYA_VSA_BASE_URL or the config
// file's base_url) also counts as configured.
func requireTenantBaseURL(c *client.Client) error {
	base := c.BaseURL
	configured := !strings.Contains(base, "{server}") || strings.TrimSpace(cliutil.EnvOverride("KASEYA_VSA_SERVER")) != ""
	if c.Config != nil {
		base = strings.ReplaceAll(base, "{server}", c.Config.TemplateVars["server"])
	}
	u, err := url.Parse(base)
	if !configured || err != nil || u.Host == "" || strings.Contains(u.Host, "{") {
		return configErr(fmt.Errorf("no VSA 10 server configured: set KASEYA_VSA_SERVER to your server host, e.g. acme.vsax.net (or KASEYA_VSA_BASE_URL to the full https://<server>/api/v3), plus KASEYA_VSA_TOKEN_ID and KASEYA_VSA_TOKEN_SECRET"))
	}
	return nil
}

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// odataQuote escapes a string literal for an OData $filter expression.
func odataQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// resolveDeviceID turns a device name or Identifier into an Identifier. GUIDs
// pass through; names always resolve live (case-insensitive exact match), never
// from the local mirror: a stale mirror could map a renamed or reused hostname
// to the wrong device, and run-wait executes code on whatever this returns.
func resolveDeviceID(ctx context.Context, flags *rootFlags, c *client.Client, nameOrID string) (string, string, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	if guidPattern.MatchString(nameOrID) {
		return nameOrID, "", nil
	}
	if nameOrID == "" {
		return "", "", usageErr(fmt.Errorf("device name or Identifier is required"))
	}
	data, err := vsaGet(ctx, c, "/devices", map[string]string{"$filter": "tolower(Name) eq " + odataQuote(strings.ToLower(nameOrID)), "$top": "2"})
	if err != nil {
		return "", "", classifyAPIErrorOnly(err)
	}
	var found []struct{ Identifier, Name string }
	if err := json.Unmarshal(data, &found); err != nil {
		return "", "", fmt.Errorf("decode device lookup: %w", err)
	}
	switch len(found) {
	case 0:
		return "", "", notFoundErr(fmt.Errorf("no device named %q (run '%s devices list --filter \"contains(Name, '...')\"' to find it)", nameOrID, fleetCLIName))
	case 1:
		return found[0].Identifier, found[0].Name, nil
	default:
		return "", "", usageErr(fmt.Errorf("more than one device is named %q; pass the device Identifier instead", nameOrID))
	}
}

// parseVSATime accepts the timestamp shapes the API and the store use. VSA
// audit timestamps carry no zone and are treated as UTC.
func parseVSATime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// compareVersions compares dotted numeric versions ("141.0.7390.66"). Missing
// or non-numeric segments compare as 0, so "1.2" == "1.2.0".
func compareVersions(a, b string) int {
	as, bs := strings.Split(strings.TrimSpace(a), "."), strings.Split(strings.TrimSpace(b), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(leadingDigits(as[i]))
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(leadingDigits(bs[i]))
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func leadingDigits(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}

// splitCSV lower-cases and trims a comma-separated flag value.
func splitCSV(s string) []string {
	out := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
