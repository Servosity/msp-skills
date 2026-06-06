// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature logic (Printing Press transcendence). Pure functions
// over in-memory fleet entities so the fleet rollups are unit-testable without a
// live Falcon tenant or a database.

package cli

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// fleetEntity is one CID-tagged record synced from a child tenant. The typed
// columns (Name/Severity/Status/LastSeen) are extracted from the raw Falcon
// payload at sync time so the rollups below are cheap, offline SQL/aggregation.
type fleetEntity struct {
	CID      string          `json:"cid"`
	Kind     string          `json:"kind"` // host | alert | vuln | policy
	ID       string          `json:"id"`
	Name     string          `json:"name,omitempty"`
	Severity string          `json:"severity,omitempty"`
	Status   string          `json:"status,omitempty"`
	LastSeen time.Time       `json:"last_seen,omitempty"`
	SyncedAt time.Time       `json:"synced_at,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

const (
	kindHost   = "host"
	kindAlert  = "alert"
	kindVuln   = "vuln"
	kindPolicy = "policy"

	// Flight Control fabric kinds, synced parent-scoped by `fleet sync`'s
	// fabric kind and joined offline by `fleet tenants`.
	kindChildCID        = "child_cid"
	kindCIDGroup        = "cid_group"
	kindCIDGroupMember  = "cid_group_member"
	kindUserGroup       = "user_group"
	kindUserGroupMember = "user_group_member"
	kindMSSPRole        = "mssp_role"
)

// severityRank orders Falcon severities high→low for sorting. Unknown/empty
// sorts last. Falcon uses both named severities (alerts, vulns) and 0-100
// numeric scores (alerts); sync normalizes both into these names.
func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default: // informational / unknown / none
		return 0
	}
}

// tenantScore is one row of `fleet scorecard`: a per-CID posture summary.
type tenantScore struct {
	CID            string  `json:"cid"`
	Hosts          int     `json:"hosts"`
	ActiveSensors  int     `json:"active_sensors"`
	CoveragePct    float64 `json:"coverage_pct"`
	OpenCritAlerts int     `json:"open_critical_alerts"`
	CriticalVulns  int     `json:"critical_vulns"`
	PreventionPols int     `json:"prevention_policies"`
}

// scorecardStaleDays is the window after which a host's sensor is treated as
// inactive for the coverage metric.
const scorecardStaleDays = 7

// scorecardRollup aggregates entities into one posture row per CID. `now` is
// injected so the active-sensor window is deterministic under test.
func scorecardRollup(entities []fleetEntity, now time.Time) []tenantScore {
	type acc struct {
		hosts, active, critAlerts, critVulns, pols int
	}
	byCID := map[string]*acc{}
	order := []string{}
	get := func(cid string) *acc {
		a, ok := byCID[cid]
		if !ok {
			a = &acc{}
			byCID[cid] = a
			order = append(order, cid)
		}
		return a
	}
	cutoff := now.Add(-scorecardStaleDays * 24 * time.Hour)
	for _, e := range entities {
		a := get(e.CID)
		switch e.Kind {
		case kindHost:
			a.hosts++
			if !e.LastSeen.IsZero() && e.LastSeen.After(cutoff) {
				a.active++
			}
		case kindAlert:
			if severityRank(e.Severity) == 4 && isOpenAlertStatus(e.Status) {
				a.critAlerts++
			}
		case kindVuln:
			if severityRank(e.Severity) == 4 && isOpenVulnStatus(e.Status) {
				a.critVulns++
			}
		case kindPolicy:
			a.pols++
		}
	}
	sort.Strings(order)
	out := make([]tenantScore, 0, len(order))
	for _, cid := range order {
		a := byCID[cid]
		cov := 0.0
		if a.hosts > 0 {
			cov = float64(a.active) / float64(a.hosts) * 100
			cov = float64(int(cov*10+0.5)) / 10 // round to 1 decimal
		}
		out = append(out, tenantScore{
			CID: cid, Hosts: a.hosts, ActiveSensors: a.active, CoveragePct: cov,
			OpenCritAlerts: a.critAlerts, CriticalVulns: a.critVulns, PreventionPols: a.pols,
		})
	}
	return out
}

// isOpenAlertStatus reports whether an alert status is still actionable (not
// closed/resolved). Empty status is treated as open.
func isOpenAlertStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "closed", "resolved", "true_positive_closed", "false_positive", "ignored", "reopened_closed":
		return false
	default:
		return true
	}
}

// isOpenVulnStatus reports whether a vulnerability is still open.
func isOpenVulnStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "closed", "expired", "resolved":
		return false
	default:
		return true
	}
}

// staleHost is one row of `fleet stale`.
type staleHost struct {
	CID      string `json:"cid"`
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	LastSeen string `json:"last_seen"`
	DaysAgo  int    `json:"days_ago"`
}

// staleHosts returns hosts whose sensor has not checked in within `days`,
// sorted oldest first. Hosts with no last_seen are treated as stale (DaysAgo -1
// sorts to the very top as "never seen").
func staleHosts(entities []fleetEntity, days int, now time.Time) []staleHost {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	out := []staleHost{}
	for _, e := range entities {
		if e.Kind != kindHost {
			continue
		}
		if e.LastSeen.IsZero() {
			out = append(out, staleHost{CID: e.CID, ID: e.ID, Hostname: e.Name, LastSeen: "", DaysAgo: -1})
			continue
		}
		if e.LastSeen.Before(cutoff) {
			daysAgo := int(now.Sub(e.LastSeen).Hours() / 24)
			out = append(out, staleHost{
				CID: e.CID, ID: e.ID, Hostname: e.Name,
				LastSeen: e.LastSeen.UTC().Format(time.RFC3339), DaysAgo: daysAgo,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DaysAgo != out[j].DaysAgo {
			// -1 (never) first, then largest DaysAgo first
			if out[i].DaysAgo < 0 {
				return true
			}
			if out[j].DaysAgo < 0 {
				return false
			}
			return out[i].DaysAgo > out[j].DaysAgo
		}
		return out[i].CID < out[j].CID
	})
	return out
}

// vulnRow is one row of `fleet vulns`.
type vulnRow struct {
	CID      string `json:"cid"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Status   string `json:"status,omitempty"`
}

// rankVulns filters vulnerabilities by minimum severity (when severityFilter is
// non-empty) and sorts by severity high→low, then CID. severityFilter matches
// the exact severity name (e.g. "critical"); empty returns all open vulns.
func rankVulns(entities []fleetEntity, severityFilter string) []vulnRow {
	want := strings.ToLower(strings.TrimSpace(severityFilter))
	out := []vulnRow{}
	for _, e := range entities {
		if e.Kind != kindVuln {
			continue
		}
		if want != "" && strings.ToLower(e.Severity) != want {
			continue
		}
		out = append(out, vulnRow{CID: e.CID, ID: e.ID, Name: e.Name, Severity: e.Severity, Status: e.Status})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if out[i].CID != out[j].CID {
			return out[i].CID < out[j].CID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// alertRow is one row of `fleet alerts`.
type alertRow struct {
	CID      string `json:"cid"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// alertQueue filters alerts by status (default keeps all open) and returns a
// single severity-sorted queue across every tenant. statusFilter matches the
// exact Falcon status (e.g. "new"); empty keeps all open alerts.
func alertQueue(entities []fleetEntity, statusFilter string) []alertRow {
	want := strings.ToLower(strings.TrimSpace(statusFilter))
	out := []alertRow{}
	for _, e := range entities {
		if e.Kind != kindAlert {
			continue
		}
		if want != "" {
			if strings.ToLower(e.Status) != want {
				continue
			}
		} else if !isOpenAlertStatus(e.Status) {
			continue
		}
		out = append(out, alertRow{CID: e.CID, ID: e.ID, Name: e.Name, Severity: e.Severity, Status: e.Status})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return out[i].CID < out[j].CID
	})
	return out
}

// driftRow is one row of `fleet policy-drift`.
type driftRow struct {
	CID       string   `json:"cid"`
	Signature string   `json:"signature"`
	Matches   bool     `json:"matches_baseline"`
	Missing   []string `json:"missing_vs_baseline,omitempty"`
	Extra     []string `json:"extra_vs_baseline,omitempty"`
}

// policySignature is the set of enabled prevention-policy identities for a CID,
// used to detect drift. Each member is "<platform>:<name>" for an enabled
// policy. Sorted + joined gives a comparable signature string.
func policySignature(entities []fleetEntity, cid string) []string {
	set := map[string]struct{}{}
	for _, e := range entities {
		if e.Kind != kindPolicy || e.CID != cid {
			continue
		}
		// Status carries the enabled flag ("true"/"false") set at sync time;
		// Name carries "<platform>:<name>".
		if strings.EqualFold(e.Status, "true") || strings.EqualFold(e.Status, "enabled") {
			if e.Name != "" {
				set[e.Name] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// policyDrift compares every tenant's enabled-prevention-policy signature to a
// baseline CID's signature and reports which tenants diverge. When baselineCID
// is empty, the most common signature across tenants is used as the baseline.
func policyDrift(entities []fleetEntity, baselineCID string) []driftRow {
	cids := distinctPolicyCIDs(entities)
	sigs := map[string][]string{}
	for _, cid := range cids {
		sigs[cid] = policySignature(entities, cid)
	}
	if baselineCID == "" {
		baselineCID = mostCommonSignatureCID(cids, sigs)
	}
	baseSet := map[string]struct{}{}
	for _, m := range sigs[baselineCID] {
		baseSet[m] = struct{}{}
	}
	out := []driftRow{}
	for _, cid := range cids {
		sig := sigs[cid]
		row := driftRow{CID: cid, Signature: strings.Join(sig, ","), Matches: true}
		sigSet := map[string]struct{}{}
		for _, m := range sig {
			sigSet[m] = struct{}{}
		}
		for m := range baseSet {
			if _, ok := sigSet[m]; !ok {
				row.Missing = append(row.Missing, m)
			}
		}
		for m := range sigSet {
			if _, ok := baseSet[m]; !ok {
				row.Extra = append(row.Extra, m)
			}
		}
		sort.Strings(row.Missing)
		sort.Strings(row.Extra)
		row.Matches = len(row.Missing) == 0 && len(row.Extra) == 0
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Matches != out[j].Matches {
			return !out[i].Matches // drifted tenants first
		}
		return out[i].CID < out[j].CID
	})
	return out
}

func distinctPolicyCIDs(entities []fleetEntity) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, e := range entities {
		if e.Kind != kindPolicy {
			continue
		}
		if _, ok := seen[e.CID]; !ok {
			seen[e.CID] = struct{}{}
			out = append(out, e.CID)
		}
	}
	sort.Strings(out)
	return out
}

func mostCommonSignatureCID(cids []string, sigs map[string][]string) string {
	counts := map[string]int{}
	rep := map[string]string{}
	for _, cid := range cids {
		key := strings.Join(sigs[cid], ",")
		counts[key]++
		if _, ok := rep[key]; !ok {
			rep[key] = cid
		}
	}
	bestKey, bestN := "", -1
	for key, n := range counts {
		if n > bestN || (n == bestN && key < bestKey) {
			bestKey, bestN = key, n
		}
	}
	return rep[bestKey]
}
