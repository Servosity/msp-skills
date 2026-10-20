// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source local
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"saas-alerts-pp-cli/internal/store"
)

const (
	saasCoverageSilent              = "SILENT"
	saasCoverageNeverSeen           = "NEVER_SEEN"
	saasCoverageRespondDisconnected = "RESPOND_DISCONNECTED"
	saasCoverageRespondNotConnected = "RESPOND_NOT_CONNECTED"
	saasCoverageNoRules             = "NO_RULES"
	saasCoverageRulesUnknown        = "RULES_UNKNOWN"
	saasCoverageAmbiguousOrgMatch   = "AMBIGUOUS_ORG_MATCH"

	saasCoverageEventWindow = 7 * 24 * time.Hour
)

type saasCoverageCustomer struct {
	CustomerID        string  `json:"customer_id"`
	CustomerName      string  `json:"customer_name"`
	Status            string  `json:"status"`
	LastEventTime     *string `json:"last_event_time"`
	EventAge          *string `json:"event_age"`
	EventAgeSeconds   *int64  `json:"event_age_seconds"`
	Events7d          int     `json:"events_7d"`
	MaxPullLagSeconds *int64  `json:"max_pull_lag_seconds"`
	// Match is "id", "name" (exactly one org has the customer's name; inferred),
	// "ambiguous" (several orgs share the name; protection state unknown) or
	// "none".
	Match                  string   `json:"match"`
	MatchInferred          bool     `json:"match_inferred"`
	RespondOrgCandidates   []string `json:"respond_org_candidates,omitempty"`
	RespondOrgID           string   `json:"respond_org_id"`
	RespondOrgName         string   `json:"respond_org_name"`
	RespondStatus          string   `json:"respond_status"`
	RespondLastError       string   `json:"respond_last_error"`
	RespondLastConnectedAt *string  `json:"respond_last_connected_at"`
	// RuleCount counts confirmed ACTIVE synced rules that apply to the org;
	// UnknownRuleRefs counts rules whose state is not known locally (ruleIds
	// references that were never synced, synced rules with no status). Both
	// are null when the org match is ambiguous.
	RuleCount       *int     `json:"rule_count"`
	UnknownRuleRefs *int     `json:"unknown_rule_refs"`
	Flags           []string `json:"flags"`
}

type saasCoverageSummary struct {
	Total               int `json:"total"`
	Silent              int `json:"silent"`
	NeverSeen           int `json:"never_seen"`
	RespondDisconnected int `json:"respond_disconnected"`
	RespondNotConnected int `json:"respond_not_connected"`
	NoRules             int `json:"no_rules"`
	RulesUnknown        int `json:"rules_unknown"`
	AmbiguousOrgMatch   int `json:"ambiguous_org_match"`
}

type saasCoverageResult struct {
	// Synced is false when sync_state has never recorded customers or events;
	// Note then says how to sync so an empty or flag-free list is not read as
	// all-clear.
	Synced bool   `json:"synced"`
	Note   string `json:"note,omitempty"`
	// UnsyncedResources lists which of customers, events, organizations and
	// rules sync_state has never recorded.
	UnsyncedResources []string               `json:"unsynced_resources"`
	Customers         []saasCoverageCustomer `json:"customers"`
	Summary           saasCoverageSummary    `json:"summary"`
}

const saasCoverageSyncResources = "customers,events,organizations,organizations-disconnected,rules"

// saasCoverageCheckedResources must all be synced for coverage flags to be
// trusted: the monitoring side (customers, events) and the Respond side
// (organizations, rules).
var saasCoverageCheckedResources = []string{"customers", "events", "organizations", "rules"}

// saasCoverageFlagWeight ranks flags by severity. Distinct powers of two make
// the summed weight order customers by their worst flag first: a monitoring
// gap (SILENT, NEVER_SEEN) outranks any combination of Respond gaps.
var saasCoverageFlagWeight = map[string]int{
	saasCoverageSilent:              16,
	saasCoverageNeverSeen:           8,
	saasCoverageRespondDisconnected: 4,
	saasCoverageRespondNotConnected: 2,
	saasCoverageAmbiguousOrgMatch:   2,
	saasCoverageNoRules:             1,
	saasCoverageRulesUnknown:        1,
}

func saasCoverageWeight(flags []string) int {
	w := 0
	for _, f := range flags {
		w += saasCoverageFlagWeight[f]
	}
	return w
}

// saasSortCoverageCustomers orders by flag severity weight, then by event age
// (never seen first, then the oldest last event), then by name and id.
func saasSortCoverageCustomers(rows []saasCoverageCustomer) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if wa, wb := saasCoverageWeight(a.Flags), saasCoverageWeight(b.Flags); wa != wb {
			return wa > wb
		}
		if (a.EventAgeSeconds == nil) != (b.EventAgeSeconds == nil) {
			return a.EventAgeSeconds == nil
		}
		if a.EventAgeSeconds != nil && *a.EventAgeSeconds != *b.EventAgeSeconds {
			return *a.EventAgeSeconds > *b.EventAgeSeconds
		}
		if !strings.EqualFold(a.CustomerName, b.CustomerName) {
			return strings.ToLower(a.CustomerName) < strings.ToLower(b.CustomerName)
		}
		return a.CustomerID < b.CustomerID
	})
}

var saasCoverageKeep = []string{"customer_id", "customer_name", "status", "last_event_time", "event_age", "event_age_seconds", "events_7d", "max_pull_lag_seconds", "match", "match_inferred", "respond_org_candidates", "respond_org_id", "respond_org_name", "respond_status", "respond_last_error", "respond_last_connected_at", "rule_count", "unknown_rule_refs", "flags"}

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagSilent string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Flag customers that went silent, lost their Respond connection, or have no Respond rules.",
		Long: "Use this command for per-customer monitoring and Respond protection gaps. Do NOT use this command for webhook delivery health; use 'subscriptions health' instead.\n\n" +
			"Reads the local store only: run 'saas-alerts-cli sync --resources customers,events,organizations,organizations-disconnected,rules' first. " +
			"Customers are ordered by their most severe flag (SILENT and NEVER_SEEN, then RESPOND_DISCONNECTED, RESPOND_NOT_CONNECTED and AMBIGUOUS_ORG_MATCH, NO_RULES and RULES_UNKNOWN), then oldest last event first. " +
			"rule_count counts only confirmed ACTIVE synced rules; unknown_rule_refs counts rule references whose state is not known locally (never synced, or no status). NO_RULES needs both to be zero; with only unknown references the flag is RULES_UNKNOWN. " +
			"A customer is matched to a Respond organization by id, else by an exact case-insensitive name only when exactly one organization has that name; several same-name organizations give match ambiguous with the candidate ids and an unknown protection state. " +
			"synced is false (with a note) when customers or events were never synced, so an empty list is not an all-clear.",
		Example: strings.Trim(`
  saas-alerts-cli coverage --silent 72h --agent
  saas-alerts-cli coverage --silent 24h --select customers.customer_name,customers.flags --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--silent=72h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "coverage")
			}
			silent, err := saasParseWindow("silent", flagSilent)
			if err != nil {
				return err
			}
			result := saasCoverageResult{
				Customers:         make([]saasCoverageCustomer, 0),
				UnsyncedResources: append([]string(nil), saasCoverageCheckedResources...),
				Note:              saasUnsyncedNote(saasCoverageCheckedResources, saasCoverageSyncResources),
			}

			dbPath = saasResolveDBPath(dbPath)
			if handled, err := saasLocalMirrorMissing(cmd, flags, dbPath, saasCoverageSyncResources, result, saasCoverageKeep); handled {
				return err
			}
			db, err := saasOpenLocalStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if err := saasBuildCoverage(cmd.Context(), db, &result, saasLocalNow().UTC(), silent); err != nil {
				return err
			}
			saasLocalSyncHints(cmd, db, flags, "customers")
			unsynced := saasUnsyncedResources(db, saasCoverageCheckedResources...)
			result.Synced = len(unsynced) == 0
			result.UnsyncedResources = append(make([]string, 0, len(unsynced)), unsynced...)
			result.Note = saasUnsyncedNote(unsynced, saasCoverageSyncResources)

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				return saasRenderCoverageHuman(cmd, result)
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), result, flags, saasCoverageKeep...)
		},
	}
	cmd.Flags().StringVar(&flagSilent, "silent", "72h", "Flag a customer SILENT when its newest event is older than this (24h, 3d, 1w)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// saasRespondOrg is a Respond organization (OrganizationConnection) or a
// stand-in built from an organizations-disconnected entry.
type saasRespondOrg struct {
	ID            string
	Name          string
	Connections   []map[string]any // applicationConnections[]
	RuleIDs       []string
	connectionIDs map[string]bool
	disconnected  []map[string]any
}

type saasCustomerEventStats struct {
	last   time.Time
	count  int
	maxLag *float64
}

func saasBuildCoverage(ctx context.Context, db *store.Store, result *saasCoverageResult, now time.Time, silent time.Duration) error {
	_, customers, err := saasLoadLocalCustomers(ctx, db)
	if err != nil {
		return err
	}
	events, err := saasLoadLocalEvents(ctx, db, saasEventQuery{})
	if err != nil {
		return err
	}
	orgDocs, err := saasLoadResourceDocs(ctx, db, "organizations")
	if err != nil {
		return err
	}
	disconnectedDocs, err := saasLoadResourceDocs(ctx, db, "organizations-disconnected")
	if err != nil {
		return err
	}
	ruleDocs, err := saasLoadResourceDocs(ctx, db, "rules")
	if err != nil {
		return err
	}

	windowStart := now.Add(-saasCoverageEventWindow)
	stats := map[string]*saasCustomerEventStats{}
	for _, ev := range events {
		if ev.CustomerID == "" {
			continue
		}
		st, ok := stats[ev.CustomerID]
		if !ok {
			st = &saasCustomerEventStats{}
			stats[ev.CustomerID] = st
		}
		// A future-dated event is not evidence the tenant is reporting now.
		if ev.Time.After(st.last) && !ev.Time.After(now) {
			st.last = ev.Time
		}
		if !ev.Time.Before(windowStart) && !ev.Time.After(now) {
			st.count++
			if ev.PullLag != nil && (st.maxLag == nil || *ev.PullLag > *st.maxLag) {
				lag := *ev.PullLag
				st.maxLag = &lag
			}
		}
	}

	_, byID, byName := saasIndexRespondOrgs(orgDocs, disconnectedDocs)

	for _, c := range customers {
		row := saasCoverageCustomer{
			CustomerID:   c.ID,
			CustomerName: c.Name,
			Status:       c.Status,
			Match:        "none",
			Flags:        make([]string, 0),
		}
		if st, ok := stats[c.ID]; ok && !st.last.IsZero() {
			lastStr := saasFormatTime(st.last)
			age := now.Sub(st.last)
			ageStr := saasFormatAge(age)
			ageSecs := int64(age / time.Second)
			row.LastEventTime, row.EventAge, row.EventAgeSeconds = &lastStr, &ageStr, &ageSecs
			row.Events7d = st.count
			row.MaxPullLagSeconds = saasLagSeconds(st.maxLag)
			if age > silent {
				row.Flags = append(row.Flags, saasCoverageSilent)
			}
		} else {
			// No event at all is also "no event newer than --silent".
			row.Flags = append(row.Flags, saasCoverageSilent, saasCoverageNeverSeen)
		}

		org := byID[c.ID]
		var nameCands []*saasRespondOrg
		if org == nil && strings.TrimSpace(c.Name) != "" {
			nameCands = byName[strings.ToLower(strings.TrimSpace(c.Name))]
		}
		switch {
		case org != nil:
			row.Match = "id"
		case len(nameCands) == 1:
			org = nameCands[0]
			row.Match = "name"
			row.MatchInferred = true
		case len(nameCands) > 1:
			// Several orgs share the name: report the candidates and leave the
			// protection state unknown instead of trusting an arbitrary pick.
			row.Match = "ambiguous"
			row.MatchInferred = true
			for _, cand := range nameCands {
				row.RespondOrgCandidates = append(row.RespondOrgCandidates, cand.ID)
			}
			sort.Strings(row.RespondOrgCandidates)
		}
		switch {
		case row.Match == "ambiguous":
			row.Flags = append(row.Flags, saasCoverageAmbiguousOrgMatch)
		case org == nil:
			zero, none := 0, 0
			row.RuleCount, row.UnknownRuleRefs = &zero, &none
			row.Flags = append(row.Flags, saasCoverageRespondNotConnected)
		default:
			row.RespondOrgID, row.RespondOrgName = org.ID, org.Name
			status, lastErr, lastConnected, broken := saasRespondConnectionState(org)
			row.RespondStatus, row.RespondLastError, row.RespondLastConnectedAt = status, lastErr, lastConnected
			if broken || len(org.disconnected) > 0 {
				row.Flags = append(row.Flags, saasCoverageRespondDisconnected)
			}
			confirmed, unknown := saasCountOrgRules(org, ruleDocs)
			row.RuleCount, row.UnknownRuleRefs = &confirmed, &unknown
			// NO_RULES only when nothing could protect the org; with unknown
			// references the state is RULES_UNKNOWN. Neither is raised for an
			// unmatched customer, which already carries RESPOND_NOT_CONNECTED.
			switch {
			case confirmed == 0 && unknown == 0:
				row.Flags = append(row.Flags, saasCoverageNoRules)
			case confirmed == 0:
				row.Flags = append(row.Flags, saasCoverageRulesUnknown)
			}
		}

		result.Summary.Total++
		for _, f := range row.Flags {
			switch f {
			case saasCoverageSilent:
				result.Summary.Silent++
			case saasCoverageNeverSeen:
				result.Summary.NeverSeen++
			case saasCoverageRespondDisconnected:
				result.Summary.RespondDisconnected++
			case saasCoverageRespondNotConnected:
				result.Summary.RespondNotConnected++
			case saasCoverageNoRules:
				result.Summary.NoRules++
			case saasCoverageRulesUnknown:
				result.Summary.RulesUnknown++
			case saasCoverageAmbiguousOrgMatch:
				result.Summary.AmbiguousOrgMatch++
			}
		}
		result.Customers = append(result.Customers, row)
	}
	saasSortCoverageCustomers(result.Customers)
	return nil
}

// saasIndexRespondOrgs indexes organizations by id and lowercased name and
// attaches organizations-disconnected entries (ApplicationOrganizationConnection:
// id is the connection id, organization.id the org). An org seen only in the
// disconnected list still gets an entry so its customer matches.
func saasIndexRespondOrgs(orgDocs, disconnectedDocs []map[string]any) ([]*saasRespondOrg, map[string]*saasRespondOrg, map[string][]*saasRespondOrg) {
	var orgs []*saasRespondOrg
	byID := map[string]*saasRespondOrg{}
	byConn := map[string]*saasRespondOrg{}
	for _, doc := range orgDocs {
		id := saasStr(doc, "id")
		if id == "" || byID[id] != nil {
			continue
		}
		org := &saasRespondOrg{ID: id, Name: saasStr(doc, "name"), connectionIDs: map[string]bool{}}
		for _, raw := range saasArr(doc, "applicationConnections") {
			if conn, ok := raw.(map[string]any); ok {
				org.Connections = append(org.Connections, conn)
				if cid := saasStr(conn, "id"); cid != "" {
					org.connectionIDs[cid] = true
					byConn[cid] = org
				}
			}
		}
		for _, raw := range saasArr(doc, "ruleIds") {
			if s, ok := raw.(string); ok && s != "" {
				org.RuleIDs = append(org.RuleIDs, s)
			}
		}
		orgs = append(orgs, org)
		byID[id] = org
	}
	for _, doc := range disconnectedDocs {
		orgID, orgName := saasDisconnectedOrgRef(doc)
		org := byID[orgID]
		if org == nil {
			org = byConn[saasStr(doc, "id")]
		}
		if org == nil && orgID != "" {
			org = &saasRespondOrg{ID: orgID, Name: orgName, connectionIDs: map[string]bool{}}
			orgs = append(orgs, org)
			byID[orgID] = org
		}
		if org != nil {
			org.disconnected = append(org.disconnected, doc)
		}
	}
	byName := map[string][]*saasRespondOrg{}
	for _, org := range orgs {
		if n := strings.ToLower(strings.TrimSpace(org.Name)); n != "" {
			byName[n] = append(byName[n], org)
		}
	}
	return orgs, byID, byName
}

// saasDisconnectedOrgRef reads the org reference of a disconnected entry.
// The schema names it organization{id,name}; the published example spells
// the key `organization"` (stray quote), and ApplicationConnection uses
// organizationId. All three are accepted.
func saasDisconnectedOrgRef(doc map[string]any) (string, string) {
	for _, key := range []string{"organization", `organization"`} {
		if id := saasStr(doc, key, "id"); id != "" {
			return id, saasStr(doc, key, "name")
		}
	}
	return saasStr(doc, "organizationId"), ""
}

// saasRespondConnectionState summarizes an org's application connections.
// status is the worst connection status (Broken > New > Upgradable >
// Active); broken is true for Broken or any status mentioning disconnect.
func saasRespondConnectionState(org *saasRespondOrg) (status, lastErr string, lastConnected *string, broken bool) {
	worst := -1
	var newest time.Time
	conns := append([]map[string]any{}, org.Connections...)
	conns = append(conns, org.disconnected...)
	for _, conn := range conns {
		st := strings.TrimSpace(saasStr(conn, "status"))
		lower := strings.ToLower(st)
		rank := 0
		switch {
		case lower == "broken" || strings.Contains(lower, "disconnect"):
			rank = 4
			broken = true
		case lower == "new":
			rank = 3
		case lower == "upgradable":
			rank = 2
		case lower == "active":
			rank = 1
		}
		if st != "" && rank > worst {
			worst = rank
			status = st
		}
		if msg := strings.TrimSpace(saasStr(conn, "lastError", "message")); msg != "" && lastErr == "" {
			lastErr = msg
		}
		// lastConnectedAt is documented on ApplicationConnection (epoch ms);
		// OrganizationConnection.applicationConnections may omit it.
		if t, ok := saasParseTime(saasGet(conn, "lastConnectedAt")); ok && t.After(newest) {
			newest = t
		}
	}
	if !newest.IsZero() {
		s := saasFormatTime(newest)
		lastConnected = &s
	}
	return status, lastErr, lastConnected, broken
}

// saasCountOrgRules splits the rules that apply to an org into confirmed and
// unknown. A synced rule applies when organizations.type is ALL (and the org
// is not in excludedOrganizations), when it lists the org, or when the org's
// ruleIds references it. Applying rules with status ACTIVE are confirmed;
// applying rules with no status are unknown; DRAFT and INACTIVE rules protect
// nothing. ruleIds references to rules that were never synced are unknown.
func saasCountOrgRules(org *saasRespondOrg, ruleDocs []map[string]any) (confirmed, unknown int) {
	matchesOrg := func(item any) bool {
		// The published example swaps id and name inside organizations[];
		// accept the org id in either slot.
		return saasStr(item, "id") == org.ID || saasStr(item, "name") == org.ID
	}
	referenced := map[string]bool{}
	for _, rid := range org.RuleIDs {
		referenced[rid] = true
	}
	confirmedIDs := map[string]bool{}
	unknownIDs := map[string]bool{}
	known := map[string]bool{}
	for i, rule := range ruleDocs {
		id := saasStr(rule, "id")
		if id != "" {
			known[id] = true
		}
		applies := id != "" && referenced[id]
		switch strings.ToUpper(saasStr(rule, "organizations", "type")) {
		case "ALL":
			excluded := false
			for _, ex := range saasArr(rule, "organizations", "excludedOrganizations") {
				if matchesOrg(ex) {
					excluded = true
					break
				}
			}
			applies = applies || !excluded
		default:
			for _, item := range saasArr(rule, "organizations", "organizations") {
				if matchesOrg(item) {
					applies = true
					break
				}
			}
		}
		if !applies {
			continue
		}
		key := id
		if key == "" {
			key = fmt.Sprintf("anon-%d", i)
		}
		switch strings.ToUpper(strings.TrimSpace(saasStr(rule, "status"))) {
		case "ACTIVE":
			confirmedIDs[key] = true
		case "":
			unknownIDs[key] = true
		}
	}
	for rid := range referenced {
		if !known[rid] {
			unknownIDs[rid] = true
		}
	}
	return len(confirmedIDs), len(unknownIDs)
}

func saasRenderCoverageHuman(cmd *cobra.Command, r saasCoverageResult) error {
	out := cmd.OutOrStdout()
	if r.Note != "" {
		fmt.Fprintf(out, "note: %s\n", r.Note)
	}
	s := r.Summary
	fmt.Fprintf(out, "%d customers: %d silent (%d never seen), %d Respond disconnected, %d not connected, %d ambiguous org match, %d without rules, %d with unknown rules\n",
		s.Total, s.Silent, s.NeverSeen, s.RespondDisconnected, s.RespondNotConnected, s.AmbiguousOrgMatch, s.NoRules, s.RulesUnknown)
	if len(r.Customers) == 0 {
		fmt.Fprintln(out, "No synced customers.")
		return nil
	}
	tw := newTabWriter(out)
	fmt.Fprintln(tw, "CUSTOMER\tLAST EVENT\tAGE\tEVENTS 7D\tRESPOND\tRULES\tFLAGS")
	for _, c := range r.Customers {
		last, age := "never", "-"
		if c.LastEventTime != nil {
			last = *c.LastEventTime
		}
		if c.EventAge != nil {
			age = *c.EventAge
		}
		respond := c.RespondStatus
		switch {
		case c.Match == "none":
			respond = "not connected"
		case c.Match == "ambiguous":
			respond = "ambiguous: " + strings.Join(c.RespondOrgCandidates, ",")
		case respond == "":
			respond = "matched"
		}
		if c.Match == "name" {
			respond += " (name match)"
		}
		rules := "?"
		if c.RuleCount != nil {
			rules = fmt.Sprintf("%d", *c.RuleCount)
			if c.UnknownRuleRefs != nil && *c.UnknownRuleRefs > 0 {
				rules += fmt.Sprintf(" (+%d unknown)", *c.UnknownRuleRefs)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", truncate(c.CustomerName, 32), last, age, c.Events7d, respond, rules, strings.Join(c.Flags, ","))
	}
	return tw.Flush()
}
