// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature logic for skykick-cli (not generator-emitted).
//
// Package fleet holds the pure logic behind the fleet-* transcendence
// commands: tolerant parsing of SkyKick API responses whose exact schemas are
// not publicly documented (the developer portal reference is login-gated), and
// the analyzers that join per-subscription facets into fleet-wide answers.
//
// Parsing philosophy: the API is an ASP.NET service behind Azure APIM. Field
// names observed in community wrapper source are PascalCase
// (CustomerInformation.CompanyName, IndividualMailboxes, the intentionally
// misspelled ExchangeRentionPeriodInDays). Every extractor here matches keys
// case-insensitively across a candidate list, returns nil/zero on absence
// instead of guessing, and the raw JSON is always preserved alongside the
// extraction so `sql` users can reach fields we did not model.
package fleet

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"skykick-pp-cli/internal/cliutil"
)

// ---------- models ----------

// Subscription is one placed backup subscription order (one customer tenant).
type Subscription struct {
	ID          string          `json:"id"`
	CompanyName string          `json:"company_name,omitempty"`
	PartnerID   string          `json:"partner_id,omitempty"`
	Raw         json.RawMessage `json:"-"`
}

// Settings is the per-subscription backup settings facet.
type Settings struct {
	SubscriptionID    string          `json:"subscription_id"`
	CompanyName       string          `json:"company_name,omitempty"`
	ExchangeEnabled   *bool           `json:"exchange_enabled,omitempty"`
	SharePointEnabled *bool           `json:"sharepoint_enabled,omitempty"`
	Raw               json.RawMessage `json:"-"`
}

// Retention is the per-subscription retention facet.
type Retention struct {
	SubscriptionID string          `json:"subscription_id"`
	ExchangeDays   *int            `json:"exchange_days,omitempty"`
	SharePointDays *int            `json:"sharepoint_days,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// Autodiscover is the per-subscription auto-discover state facet.
type Autodiscover struct {
	SubscriptionID string          `json:"subscription_id"`
	ExchangeOn     *bool           `json:"exchange_on,omitempty"`
	SharePointOn   *bool           `json:"sharepoint_on,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// MailboxStat is one mailbox's last-snapshot statistics row.
type MailboxStat struct {
	SubscriptionID string          `json:"subscription_id"`
	Mailbox        string          `json:"mailbox"`
	LastSnapshot   *time.Time      `json:"last_snapshot,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// Mailbox is one Exchange mailbox with its backup enablement state.
type Mailbox struct {
	SubscriptionID string          `json:"subscription_id"`
	ID             string          `json:"id,omitempty"`
	Email          string          `json:"email,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// Site is one SharePoint site with its backup enablement state.
type Site struct {
	SubscriptionID string          `json:"subscription_id"`
	URL            string          `json:"url,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// Alert is one alert attached to a backup service (or migration order).
type Alert struct {
	SubscriptionID string          `json:"subscription_id"`
	ID             string          `json:"id"`
	Severity       string          `json:"severity,omitempty"`
	Status         string          `json:"status,omitempty"`
	Description    string          `json:"description,omitempty"`
	Created        *time.Time      `json:"created,omitempty"`
	Raw            json.RawMessage `json:"-"`
}

// ---------- tolerant JSON helpers ----------

// objectOf unmarshals raw into a key->RawMessage map; nil when not an object.
func objectOf(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// arrayOf returns raw's elements when raw is a JSON array, or unwraps the
// first matching wrapper key (case-insensitive) whose value is an array.
// Returns nil when nothing array-shaped is found.
func arrayOf(raw json.RawMessage, wrapperKeys ...string) []json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err == nil {
			return items
		}
		return nil
	}
	obj := objectOf(raw)
	if obj == nil {
		return nil
	}
	for _, want := range wrapperKeys {
		for k, v := range obj {
			if strings.EqualFold(k, want) {
				var items []json.RawMessage
				if err := json.Unmarshal(v, &items); err == nil {
					return items
				}
			}
		}
	}
	return nil
}

// lookup returns the value for the first candidate key present (case-insensitive).
func lookup(obj map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, want := range keys {
		for k, v := range obj {
			if strings.EqualFold(k, want) {
				return v, true
			}
		}
	}
	return nil, false
}

// findString extracts the first candidate key as a string (numbers are
// stringified). Dotted candidates ("CustomerInformation.CompanyName") descend
// one nested object level per dot.
func findString(obj map[string]json.RawMessage, keys ...string) string {
	for _, want := range keys {
		if strings.Contains(want, ".") {
			parts := strings.SplitN(want, ".", 2)
			if inner, ok := lookup(obj, parts[0]); ok {
				if innerObj := objectOf(inner); innerObj != nil {
					if s := findString(innerObj, parts[1]); s != "" {
						return s
					}
				}
			}
			continue
		}
		v, ok := lookup(obj, want)
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil && s != "" {
			return s
		}
		var n json.Number
		if err := json.Unmarshal(v, &n); err == nil && n.String() != "" {
			return n.String()
		}
	}
	return ""
}

// findBool extracts the first candidate key as a tri-state bool. JSON booleans
// pass through; strings matching enabled/active/on/true (resp. disabled/
// inactive/off/false) map to true (resp. false). Absent or unrecognized
// values return nil — unknown is distinct from false everywhere in fleet.
func findBool(obj map[string]json.RawMessage, keys ...string) *bool {
	t, f := true, false
	for _, want := range keys {
		v, ok := lookup(obj, want)
		if !ok {
			continue
		}
		var b bool
		if err := json.Unmarshal(v, &b); err == nil {
			if b {
				return &t
			}
			return &f
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "enabled", "active", "on", "true", "yes", "subscribed":
				return &t
			case "disabled", "inactive", "off", "false", "no", "unsubscribed":
				return &f
			}
		}
	}
	return nil
}

// numberOf converts a raw JSON value to float64, accepting JSON numbers and
// JSON-encoded numeric strings (the streaming-feed dual encoding).
func numberOf(v json.RawMessage) (float64, bool) {
	probe := map[string]json.RawMessage{"v": v}
	return cliutil.ExtractNumber(probe, "v")
}

// findInt extracts the first candidate key as an int (accepts JSON numbers and
// numeric strings). Returns nil when absent or unparseable.
func findInt(obj map[string]json.RawMessage, keys ...string) *int {
	for _, want := range keys {
		v, ok := lookup(obj, want)
		if !ok {
			continue
		}
		probe := map[string]json.RawMessage{"v": v}
		if n, ok := cliutil.ExtractInt(probe, "v"); ok {
			i := int(n)
			return &i
		}
	}
	return nil
}

// timeLayouts are the wire formats observed across .NET/APIM APIs.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999", // .NET DateTime without zone
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"1/2/2006 3:04:05 PM", // US-locale .NET ToString
}

// parseTimeLoose parses a JSON value as a timestamp: RFC3339 and common .NET
// layouts, OData /Date(ms)/ literals, and epoch seconds/milliseconds.
func parseTimeLoose(v json.RawMessage) *time.Time {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if ts, ok := cliutil.ParseODataDate(s); ok {
			return &ts
		}
		for _, layout := range timeLayouts {
			if ts, err := time.Parse(layout, s); err == nil {
				utc := ts.UTC()
				return &utc
			}
		}
		return nil
	}
	// Bare epoch number: seconds or milliseconds.
	if n, ok := numberOf(v); ok && n > 0 {
		var ts time.Time
		if n > 1e12 { // milliseconds
			ts = time.UnixMilli(int64(n)).UTC()
		} else {
			ts = time.Unix(int64(n), 0).UTC()
		}
		return &ts
	}
	return nil
}

// findTime extracts the first candidate key as a timestamp.
func findTime(obj map[string]json.RawMessage, keys ...string) *time.Time {
	for _, want := range keys {
		if v, ok := lookup(obj, want); ok {
			if ts := parseTimeLoose(v); ts != nil {
				return ts
			}
		}
	}
	return nil
}

// findAnyTime scans ALL keys whose lowercase name contains every fragment in
// one of the fragment sets, returning the first parseable timestamp. Used for
// snapshot stats where the exact field name is unverified upstream.
func findAnyTime(obj map[string]json.RawMessage, fragmentSets ...[]string) *time.Time {
	// Deterministic order: sort keys so repeated runs extract the same field.
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, set := range fragmentSets {
		for _, k := range keys {
			lk := strings.ToLower(k)
			all := true
			for _, frag := range set {
				if !strings.Contains(lk, frag) {
					all = false
					break
				}
			}
			if !all {
				continue
			}
			if ts := parseTimeLoose(obj[k]); ts != nil {
				return ts
			}
		}
	}
	return nil
}

// ---------- parsers (one per API facet) ----------

// ParseSubscriptions parses GET /Backup (or /Backup/{partnerId}) responses.
func ParseSubscriptions(raw json.RawMessage) []Subscription {
	items := arrayOf(raw, "value", "items", "subscriptions", "orders", "results", "data")
	var subs []Subscription
	for _, item := range items {
		obj := objectOf(item)
		if obj == nil {
			continue
		}
		id := findString(obj, "id", "subscriptionId", "backupServiceId", "serviceId", "orderId")
		if id == "" {
			continue
		}
		subs = append(subs, Subscription{
			ID:          id,
			CompanyName: findString(obj, "companyName", "company", "CustomerInformation.CompanyName", "customerName", "name"),
			PartnerID:   findString(obj, "partnerId", "resellerId", "partner", "CustomerInformation.PartnerId"),
			Raw:         item,
		})
	}
	return subs
}

// ParseSettings parses GET /Backup/{id}/subscriptionsettings.
func ParseSettings(subID string, raw json.RawMessage) Settings {
	s := Settings{SubscriptionID: subID, Raw: raw}
	obj := objectOf(raw)
	if obj == nil {
		return s
	}
	s.CompanyName = findString(obj, "CustomerInformation.CompanyName", "companyName", "customerName")
	s.ExchangeEnabled = findBool(obj,
		"exchangeBackupEnabled", "isExchangeEnabled", "exchangeEnabled", "exchangeState", "exchangeBackupState", "exchangeSubscriptionState")
	s.SharePointEnabled = findBool(obj,
		"sharePointBackupEnabled", "isSharePointEnabled", "sharePointEnabled", "sharepointEnabled", "sharePointState", "sharePointBackupState", "sharePointSubscriptionState")
	return s
}

// ParseRetention parses GET /Backup/{id}/retentionperiod. The Exchange field
// is intentionally misspelled upstream (ExchangeRentionPeriodInDays); both
// spellings are matched.
func ParseRetention(subID string, raw json.RawMessage) Retention {
	r := Retention{SubscriptionID: subID, Raw: raw}
	obj := objectOf(raw)
	if obj == nil {
		return r
	}
	r.ExchangeDays = findInt(obj,
		"ExchangeRentionPeriodInDays", "ExchangeRetentionPeriodInDays", "exchangeRetentionDays", "exchangeRetention")
	r.SharePointDays = findInt(obj,
		"SharePointRetentionPeriodInDays", "SharePointRentionPeriodInDays", "sharePointRetentionDays", "sharepointRetentionDays", "sharePointRetention")
	return r
}

// ParseAutodiscover parses GET /Backup/{id}/autodiscover.
func ParseAutodiscover(subID string, raw json.RawMessage) Autodiscover {
	a := Autodiscover{SubscriptionID: subID, Raw: raw}
	obj := objectOf(raw)
	if obj == nil {
		return a
	}
	a.ExchangeOn = findBool(obj,
		"exchangeAutoDiscoverEnabled", "isExchangeAutoDiscoverEnabled", "exchangeAutoDiscover", "exchangeAutodiscover", "exchange")
	a.SharePointOn = findBool(obj,
		"sharePointAutoDiscoverEnabled", "isSharePointAutoDiscoverEnabled", "sharePointAutoDiscover", "sharepointAutodiscover", "sharePoint", "sharepoint")
	return a
}

// ParseMailboxStats parses GET /Backup/{id}/lastsnapshotstats into per-mailbox
// rows. The exact stat field names are unverified upstream, so timestamps are
// scanned by key fragments (snapshot+date, snapshot+time, backup+date, ...).
func ParseMailboxStats(subID string, raw json.RawMessage) []MailboxStat {
	items := arrayOf(raw, "value", "items", "mailboxes", "stats", "snapshotStats", "mailboxStats", "results", "data", "IndividualMailboxes")
	var stats []MailboxStat
	for _, item := range items {
		obj := objectOf(item)
		if obj == nil {
			continue
		}
		stat := MailboxStat{
			SubscriptionID: subID,
			Mailbox: findString(obj,
				"smtpAddress", "emailAddress", "email", "mailboxName", "mailbox", "userPrincipalName", "displayName", "name", "id", "mailboxId"),
			Raw: item,
		}
		stat.LastSnapshot = findTime(obj,
			"lastSnapshotDate", "lastSnapshotTime", "lastSnapshot", "lastSuccessfulSnapshot", "lastBackupDate", "lastBackupTime", "snapshotDate")
		if stat.LastSnapshot == nil {
			stat.LastSnapshot = findAnyTime(obj,
				[]string{"snapshot", "date"}, []string{"snapshot", "time"},
				[]string{"backup", "date"}, []string{"backup", "time"},
				[]string{"snapshot"})
		}
		if stat.Mailbox == "" {
			// A stat row with no mailbox identity is unactionable and would
			// collide on the (run, subscription, mailbox) store key, silently
			// replacing siblings. Skip it entirely.
			continue
		}
		stats = append(stats, stat)
	}
	return stats
}

// ParseMailboxes parses GET /Backup/{id}/mailboxes (IndividualMailboxes array
// per community wrapper evidence).
func ParseMailboxes(subID string, raw json.RawMessage) []Mailbox {
	items := arrayOf(raw, "IndividualMailboxes", "value", "items", "mailboxes", "results", "data")
	var boxes []Mailbox
	for _, item := range items {
		obj := objectOf(item)
		if obj == nil {
			continue
		}
		mb := Mailbox{
			SubscriptionID: subID,
			ID:             findString(obj, "id", "mailboxId", "MailboxId"),
			Email:          findString(obj, "smtpAddress", "emailAddress", "email", "userPrincipalName", "mailboxName", "displayName", "name"),
			Enabled: findBool(obj,
				"isEnabled", "backupEnabled", "isSubscribed", "enabled", "isProtected", "backupState", "state", "status"),
			Raw: item,
		}
		if mb.ID == "" && mb.Email == "" {
			continue
		}
		boxes = append(boxes, mb)
	}
	return boxes
}

// ParseSites parses GET /Backup/{id}/sites.
func ParseSites(subID string, raw json.RawMessage) []Site {
	items := arrayOf(raw, "value", "items", "sites", "siteCollections", "results", "data")
	var sites []Site
	for _, item := range items {
		obj := objectOf(item)
		if obj == nil {
			continue
		}
		st := Site{
			SubscriptionID: subID,
			URL:            findString(obj, "url", "siteUrl", "siteCollectionUrl", "webUrl", "absoluteUrl", "name", "id"),
			Enabled: findBool(obj,
				"isEnabled", "backupEnabled", "isSubscribed", "enabled", "isProtected", "backupState", "state", "status"),
			Raw: item,
		}
		if st.URL == "" {
			continue
		}
		sites = append(sites, st)
	}
	return sites
}

// ParseAlerts parses GET /Alerts/{id}.
func ParseAlerts(subID string, raw json.RawMessage) []Alert {
	items := arrayOf(raw, "value", "items", "alerts", "results", "data")
	var alerts []Alert
	for _, item := range items {
		obj := objectOf(item)
		if obj == nil {
			continue
		}
		al := Alert{
			SubscriptionID: subID,
			ID:             findString(obj, "id", "alertId", "AlertId"),
			Severity:       findString(obj, "severity", "level", "priority", "alertType", "type"),
			Status:         findString(obj, "status", "state", "alertStatus"),
			Description:    findString(obj, "description", "message", "text", "title", "subject", "details"),
			Created:        findTime(obj, "createdDate", "createdAt", "created", "date", "timestamp", "alertDate"),
			Raw:            item,
		}
		if al.ID == "" {
			continue
		}
		alerts = append(alerts, al)
	}
	return alerts
}

// ---------- analyzers (pure) ----------

// TenantPosture is one row of the fleet-health table.
type TenantPosture struct {
	SubscriptionID      string     `json:"subscription_id"`
	Company             string     `json:"company,omitempty"`
	PartnerID           string     `json:"partner_id,omitempty"`
	ExchangeEnabled     *bool      `json:"exchange_enabled,omitempty"`
	SharePointEnabled   *bool      `json:"sharepoint_enabled,omitempty"`
	ExchangeRetention   *int       `json:"exchange_retention_days,omitempty"`
	SharePointRetention *int       `json:"sharepoint_retention_days,omitempty"`
	AutodiscoverOn      *bool      `json:"autodiscover_on,omitempty"`
	MailboxesEnabled    int        `json:"mailboxes_enabled"`
	MailboxesTotal      int        `json:"mailboxes_total"`
	SitesEnabled        int        `json:"sites_enabled"`
	SitesTotal          int        `json:"sites_total"`
	NewestSnapshot      *time.Time `json:"newest_snapshot,omitempty"`
	OldestSnapshot      *time.Time `json:"oldest_snapshot,omitempty"`
	Gaps                []string   `json:"gaps"`
}

// FleetState bundles all facet rows for one sync run.
type FleetState struct {
	Subscriptions []Subscription
	Settings      []Settings
	Retention     []Retention
	Autodiscover  []Autodiscover
	Stats         []MailboxStat
	Mailboxes     []Mailbox
	Sites         []Site
	Alerts        []Alert
}

// staleGapThreshold is the snapshot age beyond which fleet-health flags a
// tenant's backup as stale. stale-snapshots takes a --hours flag for the
// per-mailbox view; this constant only drives the posture gap flag.
const staleGapThreshold = 48 * time.Hour

// BuildPostures joins all facets into one posture row per subscription.
func BuildPostures(state FleetState, now time.Time) []TenantPosture {
	settingsBy := make(map[string]Settings, len(state.Settings))
	for _, s := range state.Settings {
		settingsBy[s.SubscriptionID] = s
	}
	retentionBy := make(map[string]Retention, len(state.Retention))
	for _, r := range state.Retention {
		retentionBy[r.SubscriptionID] = r
	}
	autoBy := make(map[string]Autodiscover, len(state.Autodiscover))
	for _, a := range state.Autodiscover {
		autoBy[a.SubscriptionID] = a
	}
	type mbCount struct{ enabled, total int }
	mailboxesBy := map[string]*mbCount{}
	for _, m := range state.Mailboxes {
		c := mailboxesBy[m.SubscriptionID]
		if c == nil {
			c = &mbCount{}
			mailboxesBy[m.SubscriptionID] = c
		}
		c.total++
		if m.Enabled != nil && *m.Enabled {
			c.enabled++
		}
	}
	sitesBy := map[string]*mbCount{}
	for _, s := range state.Sites {
		c := sitesBy[s.SubscriptionID]
		if c == nil {
			c = &mbCount{}
			sitesBy[s.SubscriptionID] = c
		}
		c.total++
		if s.Enabled != nil && *s.Enabled {
			c.enabled++
		}
	}
	type snapAgg struct{ newest, oldest *time.Time }
	snapBy := map[string]*snapAgg{}
	for _, st := range state.Stats {
		if st.LastSnapshot == nil {
			continue
		}
		agg := snapBy[st.SubscriptionID]
		if agg == nil {
			agg = &snapAgg{}
			snapBy[st.SubscriptionID] = agg
		}
		if agg.newest == nil || st.LastSnapshot.After(*agg.newest) {
			agg.newest = st.LastSnapshot
		}
		if agg.oldest == nil || st.LastSnapshot.Before(*agg.oldest) {
			agg.oldest = st.LastSnapshot
		}
	}

	postures := make([]TenantPosture, 0, len(state.Subscriptions))
	for _, sub := range state.Subscriptions {
		p := TenantPosture{
			SubscriptionID: sub.ID,
			Company:        sub.CompanyName,
			PartnerID:      sub.PartnerID,
			Gaps:           []string{},
		}
		if s, ok := settingsBy[sub.ID]; ok {
			if p.Company == "" {
				p.Company = s.CompanyName
			}
			p.ExchangeEnabled = s.ExchangeEnabled
			p.SharePointEnabled = s.SharePointEnabled
		}
		if r, ok := retentionBy[sub.ID]; ok {
			p.ExchangeRetention = r.ExchangeDays
			p.SharePointRetention = r.SharePointDays
		}
		if a, ok := autoBy[sub.ID]; ok {
			// Collapsed flag: on when either workload's autodiscover is on.
			switch {
			case a.ExchangeOn != nil && a.SharePointOn != nil:
				v := *a.ExchangeOn || *a.SharePointOn
				p.AutodiscoverOn = &v
			case a.ExchangeOn != nil:
				p.AutodiscoverOn = a.ExchangeOn
			case a.SharePointOn != nil:
				p.AutodiscoverOn = a.SharePointOn
			}
		}
		if c, ok := mailboxesBy[sub.ID]; ok {
			p.MailboxesEnabled, p.MailboxesTotal = c.enabled, c.total
		}
		if c, ok := sitesBy[sub.ID]; ok {
			p.SitesEnabled, p.SitesTotal = c.enabled, c.total
		}
		if agg, ok := snapBy[sub.ID]; ok {
			p.NewestSnapshot, p.OldestSnapshot = agg.newest, agg.oldest
		}

		if p.ExchangeEnabled != nil && !*p.ExchangeEnabled {
			p.Gaps = append(p.Gaps, "exchange_backup_off")
		}
		if p.SharePointEnabled != nil && !*p.SharePointEnabled {
			p.Gaps = append(p.Gaps, "sharepoint_backup_off")
		}
		if p.AutodiscoverOn != nil && !*p.AutodiscoverOn {
			p.Gaps = append(p.Gaps, "autodiscover_off")
		}
		if p.MailboxesTotal > 0 && p.MailboxesEnabled < p.MailboxesTotal {
			p.Gaps = append(p.Gaps, "unprotected_mailboxes")
		}
		if p.SitesTotal > 0 && p.SitesEnabled < p.SitesTotal {
			p.Gaps = append(p.Gaps, "unprotected_sites")
		}
		if p.NewestSnapshot != nil && now.Sub(*p.NewestSnapshot) > staleGapThreshold {
			p.Gaps = append(p.Gaps, "stale_backup")
		}
		postures = append(postures, p)
	}
	sort.Slice(postures, func(i, j int) bool {
		if len(postures[i].Gaps) != len(postures[j].Gaps) {
			return len(postures[i].Gaps) > len(postures[j].Gaps)
		}
		return postures[i].Company < postures[j].Company
	})
	return postures
}

// StaleRow is one stale-mailbox finding.
type StaleRow struct {
	SubscriptionID string     `json:"subscription_id"`
	Company        string     `json:"company,omitempty"`
	Mailbox        string     `json:"mailbox"`
	LastSnapshot   *time.Time `json:"last_snapshot,omitempty"`
	AgeHours       *float64   `json:"age_hours,omitempty"`
	NeverSeen      bool       `json:"never_snapshotted,omitempty"`
}

// FindStale returns mailboxes whose last snapshot is older than threshold,
// plus mailboxes with no parseable snapshot at all (NeverSeen).
func FindStale(stats []MailboxStat, companies map[string]string, threshold time.Duration, now time.Time) []StaleRow {
	var rows []StaleRow
	for _, st := range stats {
		if st.LastSnapshot == nil {
			rows = append(rows, StaleRow{
				SubscriptionID: st.SubscriptionID,
				Company:        companies[st.SubscriptionID],
				Mailbox:        st.Mailbox,
				NeverSeen:      true,
			})
			continue
		}
		age := now.Sub(*st.LastSnapshot)
		if age > threshold {
			hours := age.Hours()
			rows = append(rows, StaleRow{
				SubscriptionID: st.SubscriptionID,
				Company:        companies[st.SubscriptionID],
				Mailbox:        st.Mailbox,
				LastSnapshot:   st.LastSnapshot,
				AgeHours:       &hours,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		// Oldest (largest age) first; never-seen rows lead.
		ai, aj := -1.0, -1.0
		if rows[i].AgeHours != nil {
			ai = *rows[i].AgeHours
		}
		if rows[j].AgeHours != nil {
			aj = *rows[j].AgeHours
		}
		if rows[i].NeverSeen != rows[j].NeverSeen {
			return rows[i].NeverSeen
		}
		return ai > aj
	})
	return rows
}

// GapRow is one coverage-gap finding (a mailbox or site explicitly NOT
// enabled for backup).
type GapRow struct {
	SubscriptionID string `json:"subscription_id"`
	Company        string `json:"company,omitempty"`
	Kind           string `json:"kind"` // "mailbox" | "site"
	Name           string `json:"name"`
}

// CoverageGaps returns mailboxes/sites whose Enabled flag is explicitly false.
// Unknown enablement (nil) is NOT a gap — it is counted separately by callers
// so absence of schema knowledge never fabricates findings.
func CoverageGaps(mailboxes []Mailbox, sites []Site, companies map[string]string, kind string) (gaps []GapRow, unknown int) {
	if kind == "mailboxes" || kind == "all" || kind == "" {
		for _, m := range mailboxes {
			name := m.Email
			if name == "" {
				name = m.ID
			}
			switch {
			case m.Enabled == nil:
				unknown++
			case !*m.Enabled:
				gaps = append(gaps, GapRow{SubscriptionID: m.SubscriptionID, Company: companies[m.SubscriptionID], Kind: "mailbox", Name: name})
			}
		}
	}
	if kind == "sites" || kind == "all" || kind == "" {
		for _, s := range sites {
			switch {
			case s.Enabled == nil:
				unknown++
			case !*s.Enabled:
				gaps = append(gaps, GapRow{SubscriptionID: s.SubscriptionID, Company: companies[s.SubscriptionID], Kind: "site", Name: s.URL})
			}
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Company != gaps[j].Company {
			return gaps[i].Company < gaps[j].Company
		}
		return gaps[i].Name < gaps[j].Name
	})
	return gaps, unknown
}

// RetentionRow is one retention-audit finding.
type RetentionRow struct {
	SubscriptionID string `json:"subscription_id"`
	Company        string `json:"company,omitempty"`
	ExchangeDays   *int   `json:"exchange_retention_days,omitempty"`
	SharePointDays *int   `json:"sharepoint_retention_days,omitempty"`
	Status         string `json:"status"` // "pass" | "under_floor" | "unknown"
}

// RetentionAudit grades each tenant's retention against floorDays. A tenant
// passes when every known workload retention >= floor; it is under_floor when
// any known value < floor; unknown when no value was extractable.
func RetentionAudit(retentions []Retention, companies map[string]string, floorDays int) []RetentionRow {
	rows := make([]RetentionRow, 0, len(retentions))
	for _, r := range retentions {
		row := RetentionRow{
			SubscriptionID: r.SubscriptionID,
			Company:        companies[r.SubscriptionID],
			ExchangeDays:   r.ExchangeDays,
			SharePointDays: r.SharePointDays,
		}
		switch {
		case r.ExchangeDays == nil && r.SharePointDays == nil:
			row.Status = "unknown"
		case (r.ExchangeDays != nil && *r.ExchangeDays < floorDays) || (r.SharePointDays != nil && *r.SharePointDays < floorDays):
			row.Status = "under_floor"
		default:
			row.Status = "pass"
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		rank := map[string]int{"under_floor": 0, "unknown": 1, "pass": 2}
		if rank[rows[i].Status] != rank[rows[j].Status] {
			return rank[rows[i].Status] < rank[rows[j].Status]
		}
		return rows[i].Company < rows[j].Company
	})
	return rows
}

// AutodiscoverRow is one autodiscover-audit finding.
type AutodiscoverRow struct {
	SubscriptionID string `json:"subscription_id"`
	Company        string `json:"company,omitempty"`
	ExchangeOn     *bool  `json:"exchange_on,omitempty"`
	SharePointOn   *bool  `json:"sharepoint_on,omitempty"`
	Status         string `json:"status"` // "on" | "off" | "partial" | "unknown"
}

// AutodiscoverAudit aggregates per-tenant autodiscover state.
func AutodiscoverAudit(states []Autodiscover, companies map[string]string, onlyOff bool) []AutodiscoverRow {
	var rows []AutodiscoverRow
	for _, a := range states {
		row := AutodiscoverRow{
			SubscriptionID: a.SubscriptionID,
			Company:        companies[a.SubscriptionID],
			ExchangeOn:     a.ExchangeOn,
			SharePointOn:   a.SharePointOn,
		}
		known, on := 0, 0
		for _, b := range []*bool{a.ExchangeOn, a.SharePointOn} {
			if b != nil {
				known++
				if *b {
					on++
				}
			}
		}
		switch {
		case known == 0:
			row.Status = "unknown"
		case on == known:
			row.Status = "on"
		case on == 0:
			row.Status = "off"
		default:
			row.Status = "partial"
		}
		if onlyOff && row.Status != "off" && row.Status != "partial" {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		rank := map[string]int{"off": 0, "partial": 1, "unknown": 2, "on": 3}
		if rank[rows[i].Status] != rank[rows[j].Status] {
			return rank[rows[i].Status] < rank[rows[j].Status]
		}
		return rows[i].Company < rows[j].Company
	})
	return rows
}

// DriftReport is the diff between two fleet sync runs.
type DriftReport struct {
	AddedSubscriptions   []string    `json:"added_subscriptions"`
	RemovedSubscriptions []string    `json:"removed_subscriptions"`
	EnablementFlips      []FlipRow   `json:"enablement_flips"`
	NewlyStaleMailboxes  []StaleRow  `json:"newly_stale_mailboxes"`
	MailboxFlips         []FlipRow   `json:"mailbox_flips"`
	RetentionChanges     []ChangeRow `json:"retention_changes"`
}

// FlipRow records a boolean state change between runs.
type FlipRow struct {
	SubscriptionID string `json:"subscription_id"`
	Company        string `json:"company,omitempty"`
	What           string `json:"what"`
	Name           string `json:"name,omitempty"`
	From           string `json:"from"`
	To             string `json:"to"`
}

// ChangeRow records a numeric change between runs.
type ChangeRow struct {
	SubscriptionID string `json:"subscription_id"`
	Company        string `json:"company,omitempty"`
	What           string `json:"what"`
	From           *int   `json:"from,omitempty"`
	To             *int   `json:"to,omitempty"`
}

func boolWord(b *bool) string {
	switch {
	case b == nil:
		return "unknown"
	case *b:
		return "on"
	default:
		return "off"
	}
}

// Drift compares a previous and current FleetState. Only transitions between
// KNOWN states count as flips — unknown->known and known->unknown are noise
// from schema-extraction coverage, not real-world changes.
func Drift(prev, cur FleetState, staleThreshold time.Duration, now time.Time) DriftReport {
	rep := DriftReport{
		AddedSubscriptions:   []string{},
		RemovedSubscriptions: []string{},
		EnablementFlips:      []FlipRow{},
		NewlyStaleMailboxes:  []StaleRow{},
		MailboxFlips:         []FlipRow{},
		RetentionChanges:     []ChangeRow{},
	}
	companies := map[string]string{}
	for _, s := range cur.Subscriptions {
		companies[s.ID] = s.CompanyName
	}
	for _, s := range prev.Subscriptions {
		if _, ok := companies[s.ID]; !ok {
			companies[s.ID] = s.CompanyName
		}
	}

	prevSubs := map[string]bool{}
	for _, s := range prev.Subscriptions {
		prevSubs[s.ID] = true
	}
	curSubs := map[string]bool{}
	for _, s := range cur.Subscriptions {
		curSubs[s.ID] = true
		if !prevSubs[s.ID] {
			rep.AddedSubscriptions = append(rep.AddedSubscriptions, s.ID)
		}
	}
	for id := range prevSubs {
		if !curSubs[id] {
			rep.RemovedSubscriptions = append(rep.RemovedSubscriptions, id)
		}
	}
	sort.Strings(rep.AddedSubscriptions)
	sort.Strings(rep.RemovedSubscriptions)

	// Enablement + autodiscover flips (settings facet).
	prevSet := map[string]Settings{}
	for _, s := range prev.Settings {
		prevSet[s.SubscriptionID] = s
	}
	for _, curS := range cur.Settings {
		prevS, ok := prevSet[curS.SubscriptionID]
		if !ok {
			continue
		}
		for _, f := range []struct {
			what     string
			from, to *bool
		}{
			{"exchange_backup", prevS.ExchangeEnabled, curS.ExchangeEnabled},
			{"sharepoint_backup", prevS.SharePointEnabled, curS.SharePointEnabled},
		} {
			if f.from != nil && f.to != nil && *f.from != *f.to {
				rep.EnablementFlips = append(rep.EnablementFlips, FlipRow{
					SubscriptionID: curS.SubscriptionID,
					Company:        companies[curS.SubscriptionID],
					What:           f.what,
					From:           boolWord(f.from),
					To:             boolWord(f.to),
				})
			}
		}
	}
	prevAuto := map[string]Autodiscover{}
	for _, a := range prev.Autodiscover {
		prevAuto[a.SubscriptionID] = a
	}
	for _, curA := range cur.Autodiscover {
		prevA, ok := prevAuto[curA.SubscriptionID]
		if !ok {
			continue
		}
		for _, f := range []struct {
			what     string
			from, to *bool
		}{
			{"exchange_autodiscover", prevA.ExchangeOn, curA.ExchangeOn},
			{"sharepoint_autodiscover", prevA.SharePointOn, curA.SharePointOn},
		} {
			if f.from != nil && f.to != nil && *f.from != *f.to {
				rep.EnablementFlips = append(rep.EnablementFlips, FlipRow{
					SubscriptionID: curA.SubscriptionID,
					Company:        companies[curA.SubscriptionID],
					What:           f.what,
					From:           boolWord(f.from),
					To:             boolWord(f.to),
				})
			}
		}
	}

	// Newly stale mailboxes: stale now, not stale before.
	prevStale := map[string]bool{}
	for _, row := range FindStale(prev.Stats, companies, staleThreshold, now) {
		prevStale[row.SubscriptionID+"\x00"+row.Mailbox] = true
	}
	for _, row := range FindStale(cur.Stats, companies, staleThreshold, now) {
		if !prevStale[row.SubscriptionID+"\x00"+row.Mailbox] {
			rep.NewlyStaleMailboxes = append(rep.NewlyStaleMailboxes, row)
		}
	}

	// Mailbox enablement flips.
	prevMB := map[string]*bool{}
	for _, m := range prev.Mailboxes {
		key := m.SubscriptionID + "\x00" + m.Email + "\x00" + m.ID
		prevMB[key] = m.Enabled
	}
	for _, m := range cur.Mailboxes {
		key := m.SubscriptionID + "\x00" + m.Email + "\x00" + m.ID
		if from, ok := prevMB[key]; ok && from != nil && m.Enabled != nil && *from != *m.Enabled {
			name := m.Email
			if name == "" {
				name = m.ID
			}
			rep.MailboxFlips = append(rep.MailboxFlips, FlipRow{
				SubscriptionID: m.SubscriptionID,
				Company:        companies[m.SubscriptionID],
				What:           "mailbox_backup",
				Name:           name,
				From:           boolWord(from),
				To:             boolWord(m.Enabled),
			})
		}
	}

	// Retention changes.
	prevRet := map[string]Retention{}
	for _, r := range prev.Retention {
		prevRet[r.SubscriptionID] = r
	}
	for _, curR := range cur.Retention {
		prevR, ok := prevRet[curR.SubscriptionID]
		if !ok {
			continue
		}
		for _, ch := range []struct {
			what     string
			from, to *int
		}{
			{"exchange_retention_days", prevR.ExchangeDays, curR.ExchangeDays},
			{"sharepoint_retention_days", prevR.SharePointDays, curR.SharePointDays},
		} {
			if ch.from != nil && ch.to != nil && *ch.from != *ch.to {
				rep.RetentionChanges = append(rep.RetentionChanges, ChangeRow{
					SubscriptionID: curR.SubscriptionID,
					Company:        companies[curR.SubscriptionID],
					What:           ch.what,
					From:           ch.from,
					To:             ch.to,
				})
			}
		}
	}
	return rep
}

// Empty reports whether the drift report contains no findings.
func (d DriftReport) Empty() bool {
	return len(d.AddedSubscriptions) == 0 && len(d.RemovedSubscriptions) == 0 &&
		len(d.EnablementFlips) == 0 && len(d.NewlyStaleMailboxes) == 0 &&
		len(d.MailboxFlips) == 0 && len(d.RetentionChanges) == 0
}

// PartnerSummary is one row of the partner-rollup table.
type PartnerSummary struct {
	PartnerID        string `json:"partner_id"`
	Tenants          int    `json:"tenants"`
	TenantsWithGaps  int    `json:"tenants_with_gaps"`
	UnprotectedBoxes int    `json:"unprotected_mailboxes"`
	UnprotectedSites int    `json:"unprotected_sites"`
	StaleTenants     int    `json:"stale_tenants"`
}

// PartnerRollup groups postures by partner. Postures without an extractable
// partner id land under "(unknown)" rather than being dropped.
func PartnerRollup(postures []TenantPosture) []PartnerSummary {
	byPartner := map[string]*PartnerSummary{}
	for _, p := range postures {
		pid := p.PartnerID
		if pid == "" {
			pid = "(unknown)"
		}
		s := byPartner[pid]
		if s == nil {
			s = &PartnerSummary{PartnerID: pid}
			byPartner[pid] = s
		}
		s.Tenants++
		if len(p.Gaps) > 0 {
			s.TenantsWithGaps++
		}
		s.UnprotectedBoxes += p.MailboxesTotal - p.MailboxesEnabled
		s.UnprotectedSites += p.SitesTotal - p.SitesEnabled
		for _, g := range p.Gaps {
			if g == "stale_backup" {
				s.StaleTenants++
			}
		}
	}
	out := make([]PartnerSummary, 0, len(byPartner))
	for _, s := range byPartner {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantsWithGaps != out[j].TenantsWithGaps {
			return out[i].TenantsWithGaps > out[j].TenantsWithGaps
		}
		return out[i].PartnerID < out[j].PartnerID
	})
	return out
}

// severityRank orders alert severities for RankAlerts; unknown severities sort
// after known ones but before empty.
func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "fatal":
		return 0
	case "error", "high", "severe":
		return 1
	case "warning", "warn", "medium":
		return 2
	case "info", "information", "informational", "low":
		return 4
	case "":
		return 5
	default:
		return 3
	}
}

// RankAlerts sorts alerts by severity (critical first), then created desc,
// then id for determinism.
func RankAlerts(alerts []Alert) []Alert {
	out := make([]Alert, len(alerts))
	copy(out, alerts)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri < rj
		}
		ti, tj := time.Time{}, time.Time{}
		if out[i].Created != nil {
			ti = *out[i].Created
		}
		if out[j].Created != nil {
			tj = *out[j].Created
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// CompanyIndex builds the subscription-id -> company-name lookup used by the
// row builders, preferring settings-derived names over list-derived ones.
func CompanyIndex(subs []Subscription, settings []Settings) map[string]string {
	idx := make(map[string]string, len(subs))
	for _, s := range subs {
		if s.CompanyName != "" {
			idx[s.ID] = s.CompanyName
		}
	}
	for _, s := range settings {
		if s.CompanyName != "" {
			idx[s.SubscriptionID] = s.CompanyName
		}
	}
	return idx
}

// OperationTerminal reports whether an /operation/{id} response status string
// represents a terminal state, and whether it succeeded. Matching is tolerant:
// completed/succeeded/success/finished/done are success; failed/error/
// cancelled/canceled/timedout are failure; anything else is non-terminal.
func OperationTerminal(status string) (terminal bool, succeeded bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "succeeded", "success", "finished", "done":
		return true, true
	case "failed", "failure", "error", "errored", "cancelled", "canceled", "timedout", "timed_out", "aborted":
		return true, false
	default:
		return false, false
	}
}

// ExtractOperationStatus pulls the status string and operation id out of an
// /operation/{id} response.
func ExtractOperationStatus(raw json.RawMessage) (status, opID string) {
	obj := objectOf(raw)
	if obj == nil {
		return "", ""
	}
	return findString(obj, "status", "state", "operationStatus", "result"),
		findString(obj, "id", "operationId", "OperationId")
}

// ExtractOperationID pulls an operation id out of a discovery POST response so
// callers can chain into watch-operation.
func ExtractOperationID(raw json.RawMessage) string {
	obj := objectOf(raw)
	if obj == nil {
		// Some async endpoints return a bare GUID string.
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return strings.TrimSpace(s)
		}
		return ""
	}
	return findString(obj, "operationId", "id", "OperationId")
}
