// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"saas-alerts-pp-cli/internal/store"
)

type saasInvestigateEvent struct {
	Time         string   `json:"time"`
	EventID      string   `json:"event_id"`
	CustomerID   string   `json:"customer_id"`
	CustomerName string   `json:"customer_name"`
	Product      string   `json:"product"`
	Severity     string   `json:"severity"`
	JointType    string   `json:"joint_type"`
	JointDesc    string   `json:"joint_desc"`
	IP           string   `json:"ip"`
	Country      string   `json:"country"`
	City         string   `json:"city"`
	Tickets      []string `json:"tickets"`
}

type saasInvestigateEvents struct {
	Total      int                    `json:"total"`
	ByProduct  map[string]int         `json:"by_product"`
	BySeverity map[string]int         `json:"by_severity"`
	Recent     []saasInvestigateEvent `json:"recent"`
}

type saasInvestigateIP struct {
	IP        string   `json:"ip"`
	Count     int      `json:"count"`
	FirstSeen string   `json:"first_seen"`
	LastSeen  string   `json:"last_seen"`
	Countries []string `json:"countries"`
	ASN       string   `json:"asn"`
	ASNName   string   `json:"asn_name"`
	Threat    any      `json:"threat"`

	first, last time.Time
	countries   map[string]bool
	threatTime  time.Time
}

type saasCountryHop struct {
	From       string `json:"from"`
	To         string `json:"to"`
	FromTime   string `json:"from_time"`
	ToTime     string `json:"to_time"`
	Gap        string `json:"gap"`
	GapSeconds int64  `json:"gap_seconds"`
	FromIP     string `json:"from_ip"`
	ToIP       string `json:"to_ip"`
}

type saasInvestigateLastEvent struct {
	Time      string `json:"time"`
	IP        string `json:"ip"`
	JointDesc string `json:"joint_desc"`
	Country   string `json:"country"`
}

type saasInvestigateUser struct {
	ID               string                    `json:"id"`
	Name             string                    `json:"name"`
	Email            string                    `json:"email"`
	Username         string                    `json:"username"`
	CustomerID       string                    `json:"customer_id"`
	CustomerName     string                    `json:"customer_name"`
	Product          string                    `json:"product"`
	AccountEnabled   *bool                     `json:"account_enabled"`
	IsLicensed       *bool                     `json:"is_licensed"`
	IsBillable       *bool                     `json:"is_billable"`
	IsServiceAccount *bool                     `json:"is_service_account"`
	Role             string                    `json:"role"`
	Licenses         []string                  `json:"licenses"`
	LastEvent        *saasInvestigateLastEvent `json:"last_event"`
	Records          int                       `json:"records"`
	Products         []string                  `json:"products"`
}

type saasInvestigateTrigger struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	RuleID           string `json:"rule_id"`
	RuleName         string `json:"rule_name"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	TriggeredAt      string `json:"triggered_at"`

	at time.Time
}

type saasInvestigateSuppression struct {
	ID           string `json:"id"`
	Scope        string `json:"scope"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	Filters      any    `json:"filters"`
	// Match is how the account was found: "users" (users[] list), "filter"
	// (the user-name filter key) or "inferred" (email text anywhere in the
	// suppression document; INFERRED, the criteria shape was not recognized).
	Match string `json:"match"`
}

type saasInvestigateDevice struct {
	ID             string   `json:"id"`
	DeviceName     string   `json:"device_name"`
	DeviceType     string   `json:"device_type"`
	DeviceOS       string   `json:"device_os"`
	CustomerID     string   `json:"customer_id"`
	CustomerName   string   `json:"customer_name"`
	Product        string   `json:"product"`
	LastActiveTime string   `json:"last_active_time"`
	MappedBy       string   `json:"mapped_by"`
	Confidence     *float64 `json:"confidence"`
}

type saasInvestigateResult struct {
	Email       string `json:"email"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
	// Synced is false when sync_state has never recorded any resource the
	// dossier joins (saasInvestigateCheckedResources); Note then says how to
	// sync, so an empty section is not read as all-clear.
	Synced bool   `json:"synced"`
	Note   string `json:"note,omitempty"`
	// UnsyncedResources lists which of events, users, rule-triggers,
	// suppressions and devices sync_state has never recorded.
	UnsyncedResources []string `json:"unsynced_resources"`
	// AmbiguousIDMatches counts events that carry the account's id in its
	// customer but whose product cannot be compared with the synced user's,
	// so they are not attributed; AmbiguousIDNote explains when nonzero.
	AmbiguousIDMatches int                          `json:"ambiguous_id_matches"`
	AmbiguousIDNote    string                       `json:"ambiguous_id_note,omitempty"`
	Events             saasInvestigateEvents        `json:"events"`
	IPs                []saasInvestigateIP          `json:"ips"`
	CountryHops        []saasCountryHop             `json:"country_hops"`
	User               *saasInvestigateUser         `json:"user"`
	RuleTriggers       []saasInvestigateTrigger     `json:"rule_triggers"`
	Suppressions       []saasInvestigateSuppression `json:"suppressions"`
	Devices            []saasInvestigateDevice      `json:"devices"`
}

const saasInvestigateRecentMax = 20

// saasInvestigateSyncResources is the sync list investigate needs; naming
// customers also syncs each customer's users (a parent-keyed dependent).
const saasInvestigateSyncResources = "customers,events,rule-triggers,suppressions,devices"

// saasInvestigateCheckedResources are the resources the dossier joins; each
// must be synced before its section can be trusted (users is recorded by the
// customers sync as a parent-keyed dependent).
var saasInvestigateCheckedResources = []string{"events", "users", "rule-triggers", "suppressions", "devices"}

var saasInvestigateKeep = []string{
	"time", "event_id", "customer_id", "customer_name", "product", "severity", "joint_type", "joint_desc", "ip", "country", "city", "tickets",
	"count", "first_seen", "last_seen", "countries", "asn", "asn_name", "threat",
	"from", "to", "from_time", "to_time", "gap", "gap_seconds", "from_ip", "to_ip",
	"id", "status", "rule_id", "rule_name", "organization_id", "organization_name", "triggered_at",
	"scope", "reason", "start_date", "end_date", "filters", "match",
	"device_name", "device_type", "device_os", "last_active_time", "mapped_by", "confidence",
}

func newSaasInvestigateResult(email string, start, end time.Time) saasInvestigateResult {
	return saasInvestigateResult{
		Email:       email,
		WindowStart: saasFormatTime(start),
		WindowEnd:   saasFormatTime(end),
		Note:        saasUnsyncedNote(saasInvestigateCheckedResources, saasInvestigateSyncResources),
		// Until a store is read, every joined resource counts as unsynced.
		UnsyncedResources: append([]string(nil), saasInvestigateCheckedResources...),
		Events: saasInvestigateEvents{
			ByProduct:  map[string]int{},
			BySeverity: map[string]int{"critical": 0, "medium": 0, "low": 0},
			Recent:     make([]saasInvestigateEvent, 0),
		},
		IPs:          make([]saasInvestigateIP, 0),
		CountryHops:  make([]saasCountryHop, 0),
		RuleTriggers: make([]saasInvestigateTrigger, 0),
		Suppressions: make([]saasInvestigateSuppression, 0),
		Devices:      make([]saasInvestigateDevice, 0),
	}
}

func newNovelInvestigateCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagHopWindow string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "investigate [email]",
		Short: "Everything known about one account: events, country hops, IPs and ASNs, user record, Respond triggers, suppressions",
		Long: "Use this command for everything known about one account. Do NOT use this command for a fleet-wide digest; use 'triage' instead. Do NOT use it to act on the account; use 'contain' instead.\n\n" +
			"Reads the local store only: run 'saas-alerts-cli sync --resources customers,events,rule-triggers,suppressions,devices' first (customers also syncs each customer's users). " +
			"Events match the email in their userEmail field, the deprecated email field, or an email-shaped user name (case-insensitive), or carry the user id or UserId of a synced user with that email. " +
			"synced is false (with a note, and unsynced_resources naming them) when events, users, rule-triggers, suppressions or devices were never synced or never finished a sync, when the last events sync did not finish, or when --since starts before the oldest synced events, so empty sections are not an all-clear.",
		Example: strings.Trim(`
  saas-alerts-cli investigate jane.doe@example.com --since 7d --hop-window 2h --agent
  saas-alerts-cli investigate jane.doe@example.com --since 30d --select country_hops,user --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "email=jane.doe@example.com;--since=7d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "investigate")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an account email is required, e.g. saas-alerts-cli investigate jane.doe@example.com"))
			}
			if len(args) > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("investigate takes exactly one email, got %d arguments", len(args)))
			}
			email := strings.TrimSpace(args[0])
			since, err := saasParseWindow("since", flagSince)
			if err != nil {
				return err
			}
			hopWindow, err := saasParseWindow("hop-window", flagHopWindow)
			if err != nil {
				return err
			}
			now := saasLocalNow().UTC()
			start := now.Add(-since)
			result := newSaasInvestigateResult(email, start, now)

			dbPath = saasResolveDBPath(dbPath)
			if handled, err := saasLocalMirrorMissing(cmd, flags, dbPath, saasInvestigateSyncResources, result, saasInvestigateKeep); handled {
				return err
			}
			db, err := saasOpenLocalStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := saasBuildInvestigation(cmd.Context(), db, &result, start, now, hopWindow); err != nil {
				return err
			}
			saasLocalSyncHints(cmd, db, flags, "events")
			unsynced := saasInvestigateUnsynced(cmd.Context(), db)
			gaps, incomplete := saasSyncGaps(db, unsynced, saasInvestigateCheckedResources...) // saas-alerts hand-fix: an unfinished sync is not synced
			result.Synced = len(gaps) == 0
			result.UnsyncedResources = gaps
			result.Note = saasSyncGapNote(db, unsynced, incomplete, saasInvestigateSyncResources)
			saasApplyWindowGap(db, start, flagSince, &result.Synced, &result.Note, &result.UnsyncedResources) // saas-alerts hand-fix: window older than the synced range

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return saasRenderInvestigateHuman(cmd, result)
			}
			saasSelectKeepsSyncState(flags, "synced", "note", "unsynced_resources") // saas-alerts hand-fix
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, saasInvestigateKeep...)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Look-back window for events (24h, 7d, 1w); user, Respond, suppression and device records are not windowed")
	cmd.Flags().StringVar(&flagHopWindow, "hop-window", "2h", "Report consecutive events from different countries at most this far apart")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// saasInvestigateUnsynced lists the joined resources sync_state has never
// recorded. users is a dependent of customers: a completed customers sync
// with no stored customer rows has no parents to fetch users for and never
// stamps users, so that case counts as synced-and-empty rather than a gap no
// sync can close. Any stored customer row, or any error, keeps users listed.
func saasInvestigateUnsynced(ctx context.Context, db *store.Store) []string {
	missing := saasUnsyncedResources(db, saasInvestigateCheckedResources...)
	for i, r := range missing {
		if r != "users" {
			continue
		}
		if db != nil && len(saasUnsyncedResources(db, "customers")) == 0 && !saasStoreHasCustomerRows(ctx, db) {
			missing = append(missing[:i:i], missing[i+1:]...)
		}
		break
	}
	return missing
}

// saasStoreHasCustomerRows reports whether any customer row is stored, in the
// generic resources table or the typed customers table the dependent users
// sync enumerates parents from. It errs toward true: only a clean zero count
// (or a never-created table) returns false.
func saasStoreHasCustomerRows(ctx context.Context, db *store.Store) bool {
	for _, q := range []string{
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'customers'`,
		`SELECT COUNT(*) FROM "customers"`,
	} {
		var n int
		if err := db.DB().QueryRowContext(ctx, q).Scan(&n); err != nil {
			if syncHintMissingTable(err) {
				continue
			}
			return true
		}
		if n > 0 {
			return true
		}
	}
	return false
}

func saasBuildInvestigation(ctx context.Context, db *store.Store, result *saasInvestigateResult, start, end time.Time, hopWindow time.Duration) error {
	email := result.Email
	_, customers, err := saasLoadLocalCustomers(ctx, db)
	if err != nil {
		return err
	}
	customerNames := make(map[string]string, len(customers))
	for _, c := range customers {
		customerNames[c.ID] = c.Name
	}

	// Resolve the account's ids from synced users first: Reports events can
	// carry only user {id, display name} with no email field, so an event is
	// attributed by an email field OR by one of these ids.
	userDocs, err := saasLoadResourceDocs(ctx, db, "users")
	if err != nil {
		return err
	}
	result.User = saasInvestigateMatchUser(userDocs, email, customerNames)
	scopes := saasInvestigateAccountScopes(userDocs, email)

	ambiguous := 0
	events, err := saasLoadLocalEvents(ctx, db, saasEventQuery{Start: start, End: end, UserEmail: email, AccountScopes: scopes, AmbiguousIDMatches: &ambiguous})
	if err != nil {
		return err
	}
	result.AmbiguousIDMatches = ambiguous
	if ambiguous > 0 {
		result.AmbiguousIDNote = fmt.Sprintf("%d event(s) carry this account's id in its customer but no product comparable with the synced user's; not attributed", ambiguous)
	}
	saasInvestigateSummarizeEvents(result, events, hopWindow)

	triggerDocs, err := saasLoadResourceDocs(ctx, db, "rule-triggers")
	if err != nil {
		return err
	}
	result.RuleTriggers = saasInvestigateMatchTriggers(triggerDocs, email)

	suppressionDocs, err := saasLoadResourceDocs(ctx, db, "suppressions")
	if err != nil {
		return err
	}
	result.Suppressions = saasInvestigateMatchSuppressions(suppressionDocs, email)

	deviceDocs, err := saasLoadResourceDocs(ctx, db, "devices", "devices-mapped")
	if err != nil {
		return err
	}
	result.Devices = saasInvestigateMatchDevices(deviceDocs, email)
	return nil
}

func saasInvestigateSummarizeEvents(result *saasInvestigateResult, events []saasLocalEvent, hopWindow time.Duration) {
	result.Events.Total = len(events)
	ips := map[string]*saasInvestigateIP{}
	for _, ev := range events {
		product := ev.ProductName
		if product == "" {
			product = ev.ProductID
		}
		if product == "" {
			product = "unknown"
		}
		result.Events.ByProduct[product]++
		sev := ev.Severity
		if sev == "" {
			sev = "unknown"
		}
		result.Events.BySeverity[sev]++
		if ev.IP != "" {
			row, ok := ips[ev.IP]
			if !ok {
				row = &saasInvestigateIP{IP: ev.IP, first: ev.Time, last: ev.Time, countries: map[string]bool{}}
				ips[ev.IP] = row
			}
			row.Count++
			if ev.Time.Before(row.first) {
				row.first = ev.Time
			}
			if ev.Time.After(row.last) {
				row.last = ev.Time
			}
			if ev.Country != "" {
				row.countries[ev.Country] = true
			}
			if ev.ASN != "" {
				row.ASN = ev.ASN
			}
			if ev.ASNName != "" {
				row.ASNName = ev.ASNName
			}
			if ev.Threat != nil && (row.Threat == nil || !ev.Time.Before(row.threatTime)) {
				row.Threat = ev.Threat
				row.threatTime = ev.Time
			}
		}
	}

	// recent: newest first, capped. events arrive oldest first.
	for i := len(events) - 1; i >= 0 && len(result.Events.Recent) < saasInvestigateRecentMax; i-- {
		ev := events[i]
		product := ev.ProductName
		if product == "" {
			product = ev.ProductID
		}
		tickets := ev.Tickets
		if tickets == nil {
			tickets = []string{}
		}
		result.Events.Recent = append(result.Events.Recent, saasInvestigateEvent{
			Time: saasFormatTime(ev.Time), EventID: ev.EventID, CustomerID: ev.CustomerID, CustomerName: ev.CustomerName,
			Product: product, Severity: ev.Severity, JointType: ev.JointType, JointDesc: ev.JointDesc,
			IP: ev.IP, Country: ev.Country, City: ev.City, Tickets: tickets,
		})
	}

	for _, row := range ips {
		row.FirstSeen = saasFormatTime(row.first)
		row.LastSeen = saasFormatTime(row.last)
		row.Countries = saasSortedKeys(row.countries)
		result.IPs = append(result.IPs, *row)
	}
	sort.SliceStable(result.IPs, func(i, j int) bool {
		a, b := result.IPs[i], result.IPs[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if !a.last.Equal(b.last) {
			return a.last.After(b.last)
		}
		return a.IP < b.IP
	})

	// country hops: consecutive (time-ordered) events with a known country.
	var prev *saasLocalEvent
	for i := range events {
		ev := &events[i]
		if ev.Country == "" {
			continue
		}
		if prev != nil && !strings.EqualFold(prev.Country, ev.Country) {
			gap := ev.Time.Sub(prev.Time)
			if gap <= hopWindow {
				result.CountryHops = append(result.CountryHops, saasCountryHop{
					From: prev.Country, To: ev.Country,
					FromTime: saasFormatTime(prev.Time), ToTime: saasFormatTime(ev.Time),
					Gap: saasFormatAge(gap), GapSeconds: int64(gap / time.Second),
					FromIP: prev.IP, ToIP: ev.IP,
				})
			}
		}
		prev = ev
	}
}

// saasInvestigateAccountScopes returns the scoped identities of synced users
// whose email (or username) equals email, case-insensitively: the user's
// parent customer (sync stores it as parent_id), the user's product when the
// row carries one, and the account id. Rows without a customer or id are
// skipped, because a bare id cannot be attributed safely.
func saasInvestigateAccountScopes(docs []map[string]any, email string) []saasAccountScope {
	seen := map[saasAccountScope]bool{}
	var scopes []saasAccountScope
	for _, doc := range docs {
		if !strings.EqualFold(strings.TrimSpace(saasStr(doc, "email")), email) &&
			!strings.EqualFold(strings.TrimSpace(saasStr(doc, "username")), email) {
			continue
		}
		sc := saasAccountScope{
			CustomerID:  saasUserCustomerID(doc),
			ProductID:   strings.TrimSpace(saasStr(doc, "product", "id")),
			ProductName: strings.TrimSpace(saasStr(doc, "product", "name")),
			AccountID:   strings.TrimSpace(saasStr(doc, "id")),
		}
		if sc.CustomerID == "" || sc.AccountID == "" || seen[sc] {
			continue
		}
		seen[sc] = true
		scopes = append(scopes, sc)
	}
	sort.Slice(scopes, func(i, j int) bool {
		a, b := scopes[i], scopes[j]
		if a.CustomerID != b.CustomerID {
			return a.CustomerID < b.CustomerID
		}
		if a.AccountID != b.AccountID {
			return a.AccountID < b.AccountID
		}
		return a.ProductID+a.ProductName < b.ProductID+b.ProductName
	})
	return scopes
}

// saasUserCustomerID is the customer a synced user row belongs to: sync
// stores the parent customer id as parent_id (and customers_id).
func saasUserCustomerID(doc map[string]any) string {
	for _, key := range []string{"parent_id", "customers_id", "customerId"} {
		if v := strings.TrimSpace(saasStr(doc, key)); v != "" {
			return v
		}
	}
	return ""
}

// saasInvestigateMatchUser matches Reports users (ISImportedUser) by email,
// falling back to username. The same account can appear once per product or
// customer; the most recently active record is reported, with the record
// count and every product listed.
func saasInvestigateMatchUser(docs []map[string]any, email string, customerNames map[string]string) *saasInvestigateUser {
	type match struct {
		doc  map[string]any
		last time.Time
	}
	var matches []match
	for _, doc := range docs {
		if !strings.EqualFold(strings.TrimSpace(saasStr(doc, "email")), email) &&
			!strings.EqualFold(strings.TrimSpace(saasStr(doc, "username")), email) {
			continue
		}
		last, _ := saasParseTime(saasGet(doc, "lastEvent", "time"))
		matches = append(matches, match{doc: doc, last: last})
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if !matches[i].last.Equal(matches[j].last) {
			return matches[i].last.After(matches[j].last)
		}
		return saasStr(matches[i].doc, "id") < saasStr(matches[j].doc, "id")
	})
	doc := matches[0].doc
	// sync stores the parent customer id as parent_id (and customers_id).
	custID := saasUserCustomerID(doc)
	product := saasStr(doc, "product", "name")
	if product == "" {
		product = saasStr(doc, "product", "id")
	}
	u := &saasInvestigateUser{
		ID: saasStr(doc, "id"), Name: saasStr(doc, "name"), Email: saasStr(doc, "email"), Username: saasStr(doc, "username"),
		CustomerID: custID, CustomerName: customerNames[custID], Product: product,
		AccountEnabled: saasBoolPtr(doc, "accountEnabled"), IsLicensed: saasBoolPtr(doc, "isLicensed"),
		IsBillable: saasBoolPtr(doc, "isBillable"), IsServiceAccount: saasBoolPtr(doc, "isServiceAccount"),
		Role: saasStr(doc, "role"), Licenses: []string{}, Records: len(matches), Products: []string{},
	}
	for _, lic := range saasArr(doc, "assignedLicenses") {
		if sku := saasStr(lic, "skuId"); sku != "" {
			u.Licenses = saasAppendUnique(u.Licenses, sku)
		}
	}
	if le, ok := saasGet(doc, "lastEvent").(map[string]any); ok {
		t, _ := saasParseTime(le["time"])
		u.LastEvent = &saasInvestigateLastEvent{
			Time: saasFormatTime(t), IP: saasStr(le, "ip"), JointDesc: saasStr(le, "jointDesc"), Country: saasStr(le, "location", "country"),
		}
	}
	products := map[string]bool{}
	for _, m := range matches {
		if p := saasStr(m.doc, "product", "name"); p != "" {
			products[p] = true
		}
	}
	u.Products = saasSortedKeys(products)
	return u
}

// saasInvestigateMatchTriggers returns Respond rule triggers whose
// accounts[].email matches, newest first. Triggers are not windowed.
func saasInvestigateMatchTriggers(docs []map[string]any, email string) []saasInvestigateTrigger {
	out := make([]saasInvestigateTrigger, 0)
	for _, doc := range docs {
		matched := false
		for _, acct := range saasArr(doc, "accounts") {
			if strings.EqualFold(strings.TrimSpace(saasStr(acct, "email")), email) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		at, _ := saasParseTime(saasGet(doc, "triggeredAt"))
		out = append(out, saasInvestigateTrigger{
			ID: saasStr(doc, "id"), Status: saasStr(doc, "status"),
			RuleID: saasStr(doc, "rule", "id"), RuleName: saasStr(doc, "rule", "name"),
			OrganizationID: saasStr(doc, "organization", "id"), OrganizationName: saasStr(doc, "organization", "name"),
			TriggeredAt: saasFormatTime(at), at: at,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.After(out[j].at)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// saasSuppressionUserField is the event field path a suppression filter keys the
// account on. Built from parts because dogfood's URL scan reads the dotted literal
// as a hostname.
var saasSuppressionUserField = strings.Join([]string{"user", "name"}, ".")

// saasInvestigateMatchSuppressions finds Manage suppressions that name the
// account: scope "users" with the email in users[], or a filters["user.name"]
// criterion (user.name is a documented suppression filter key). Any other
// shape that still contains the email text is returned with match
// "inferred".
func saasInvestigateMatchSuppressions(docs []map[string]any, email string) []saasInvestigateSuppression {
	lower := strings.ToLower(email)
	out := make([]saasInvestigateSuppression, 0)
	for _, doc := range docs {
		match := ""
		for _, u := range saasArr(doc, "users") {
			if s, ok := u.(string); ok && strings.EqualFold(strings.TrimSpace(s), email) {
				match = "users"
				break
			}
			if strings.EqualFold(strings.TrimSpace(saasStr(u, "email")), email) {
				match = "users"
				break
			}
		}
		if match == "" {
			if filters, ok := doc["filters"].(map[string]any); ok {
				switch v := filters[saasSuppressionUserField].(type) {
				case string:
					if strings.EqualFold(strings.TrimSpace(v), email) {
						match = "filter"
					}
				case []any:
					for _, item := range v {
						if s, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(s), email) {
							match = "filter"
							break
						}
					}
				}
			}
		}
		if match == "" {
			// INFERRED: the criteria shape is not documented beyond users[] and
			// filters; fall back to the email appearing anywhere in the document.
			// Audit fields name the MSP technician who edited the rule, not the
			// suppressed account, so they are left out of the scan.
			scan := make(map[string]any, len(doc))
			for k, v := range doc {
				switch k {
				case "lastUpdatedBy", "createdBy", "updatedBy":
					continue
				}
				scan[k] = v
			}
			raw, err := json.Marshal(scan)
			if err == nil && strings.Contains(strings.ToLower(string(raw)), lower) {
				match = "inferred"
			}
		}
		if match == "" {
			continue
		}
		out = append(out, saasInvestigateSuppression{
			ID: saasStr(doc, "id"), Scope: saasStr(doc, "scope"), Status: saasStr(doc, "status"), Reason: saasStr(doc, "reason"),
			CustomerID: saasStr(doc, "customerId"), CustomerName: saasStr(doc, "customerName"),
			StartDate: saasStr(doc, "startDate"), EndDate: saasStr(doc, "endDate"),
			Filters: doc["filters"], Match: match,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// saasInvestigateMatchDevices returns Unify devices whose mappedAccounts[]
// include the email. devices and devices-mapped overlap; rows dedupe on _id.
func saasInvestigateMatchDevices(docs []map[string]any, email string) []saasInvestigateDevice {
	out := make([]saasInvestigateDevice, 0)
	seen := map[string]bool{}
	for _, doc := range docs {
		var acct map[string]any
		for _, raw := range saasArr(doc, "mappedAccounts") {
			if m, ok := raw.(map[string]any); ok && strings.EqualFold(strings.TrimSpace(saasStr(m, "email")), email) {
				acct = m
				break
			}
		}
		if acct == nil {
			continue
		}
		id := saasStr(doc, "_id")
		if id == "" {
			id = saasStr(doc, "id")
		}
		dedupe := id
		if dedupe == "" {
			dedupe = saasStr(doc, "deviceName") + "|" + saasStr(doc, "customer", "id")
		}
		if seen[dedupe] {
			continue
		}
		seen[dedupe] = true
		var conf *float64
		if c, ok := acct["confidence"].(float64); ok {
			conf = &c
		}
		lastActive := saasStr(doc, "lastActiveTime")
		if t, ok := saasParseTime(saasGet(doc, "lastActiveTime")); ok {
			lastActive = saasFormatTime(t)
		}
		out = append(out, saasInvestigateDevice{
			ID: id, DeviceName: saasStr(doc, "deviceName"), DeviceType: saasStr(doc, "deviceType"), DeviceOS: saasStr(doc, "deviceOS"),
			CustomerID: saasStr(doc, "customer", "id"), CustomerName: saasStr(doc, "customer", "name"),
			Product: saasStr(doc, "product", "name"), LastActiveTime: lastActive,
			MappedBy: saasStr(acct, "mappedBy"), Confidence: conf,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DeviceName != out[j].DeviceName {
			return out[i].DeviceName < out[j].DeviceName
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func saasRenderInvestigateHuman(cmd *cobra.Command, r saasInvestigateResult) error {
	out := cmd.OutOrStdout()
	if r.Note != "" {
		fmt.Fprintf(out, "note: %s\n", r.Note)
	}
	if r.AmbiguousIDNote != "" {
		fmt.Fprintf(out, "note: %s\n", r.AmbiguousIDNote)
	}
	fmt.Fprintf(out, "%s  (%s .. %s)\n", r.Email, r.WindowStart, r.WindowEnd)
	if r.User != nil {
		enabled := "unknown"
		if r.User.AccountEnabled != nil {
			enabled = fmt.Sprintf("%t", *r.User.AccountEnabled)
		}
		fmt.Fprintf(out, "User: %s, customer %s, product %s, enabled %s, %d record(s)\n", r.User.Name, r.User.CustomerName, r.User.Product, enabled, r.User.Records)
	} else {
		fmt.Fprintln(out, "User: no synced user record")
	}
	fmt.Fprintf(out, "Events: %d (critical %d, medium %d, low %d)\n", r.Events.Total, r.Events.BySeverity["critical"], r.Events.BySeverity["medium"], r.Events.BySeverity["low"])
	if len(r.Events.Recent) > 0 {
		tw := newTabWriter(out)
		fmt.Fprintln(tw, "  TIME\tSEVERITY\tEVENT\tIP\tCOUNTRY")
		for _, ev := range r.Events.Recent {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", ev.Time, ev.Severity, truncate(ev.JointDesc, 48), ev.IP, ev.Country)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "IPs: %d distinct\n", len(r.IPs))
	for _, ip := range r.IPs {
		fmt.Fprintf(out, "  %s  x%d  %s  %s\n", ip.IP, ip.Count, strings.Join(ip.Countries, ","), ip.ASNName)
	}
	fmt.Fprintf(out, "Country hops: %d\n", len(r.CountryHops))
	for _, h := range r.CountryHops {
		fmt.Fprintf(out, "  %s -> %s in %s (%s .. %s)\n", h.From, h.To, h.Gap, h.FromTime, h.ToTime)
	}
	fmt.Fprintf(out, "Rule triggers: %d\n", len(r.RuleTriggers))
	for _, t := range r.RuleTriggers {
		fmt.Fprintf(out, "  %s  %s  %s  %s\n", t.TriggeredAt, t.Status, t.RuleName, t.ID)
	}
	fmt.Fprintf(out, "Suppressions: %d\n", len(r.Suppressions))
	for _, s := range r.Suppressions {
		fmt.Fprintf(out, "  %s  %s  %s  (%s match)\n", s.ID, s.Status, truncate(s.Reason, 48), s.Match)
	}
	fmt.Fprintf(out, "Devices: %d\n", len(r.Devices))
	for _, d := range r.Devices {
		fmt.Fprintf(out, "  %s  %s  %s\n", d.DeviceName, d.DeviceOS, d.CustomerName)
	}
	return nil
}
