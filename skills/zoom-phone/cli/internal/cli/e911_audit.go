// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: account-wide E911 compliance sweep.
// pp:data-source local
// Joins users, common areas, sites, emergency addresses, emergency service
// locations and phone numbers from the local store. No API calls.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var localE911SyncResources = []string{"users", "common_areas", "sites", "emergency_addresses", "locations", "numbers"}

// localE911Checks is every finding kind the audit can produce.
var localE911Checks = []string{"NO_ADDRESS", "ADDRESS_NOT_VERIFIED", "SITE_NO_DEFAULT_ADDRESS", "SITE_NO_ERL", "BYOC_CARRIER_UPDATE_REQUIRED"}

// localUnassessedE911 is the result when there is no local store at all.
func localUnassessedE911() *localE911Result {
	res := localNewE911Result()
	res.MissingLocal = append(res.MissingLocal, localE911SyncResources...)
	sort.Strings(res.MissingLocal)
	for _, m := range res.MissingLocal {
		res.MissingReasons[m] = "no local store"
	}
	for _, c := range localE911Checks {
		res.SkippedChecks = append(res.SkippedChecks, c+": no local store")
	}
	res.Note = "no local store; nothing was assessed; run: " + localSyncCommand(localE911SyncResources)
	return res
}

type localE911Finding struct {
	Kind          string `json:"kind"`
	Severity      string `json:"severity"`
	Status        string `json:"status,omitempty"`
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	Name          string `json:"name"`
	Site          string `json:"site,omitempty"`
	AddressSource string `json:"address_source,omitempty"`
	AddressID     string `json:"address_id,omitempty"`
	Detail        string `json:"detail"`
}

type localE911Counts struct {
	Total      int            `json:"total"`
	ByKind     map[string]int `json:"by_kind"`
	BySeverity map[string]int `json:"by_severity"`
}

type localE911Checked struct {
	Users                int `json:"users"`
	CommonAreas          int `json:"common_areas"`
	Sites                int `json:"sites"`
	Numbers              int `json:"numbers"`
	SkippedInactiveUsers int `json:"skipped_inactive_users"`
}

type localE911Result struct {
	Assessed       bool               `json:"assessed"`
	Findings       []localE911Finding `json:"findings"`
	Counts         localE911Counts    `json:"counts"`
	Checked        localE911Checked   `json:"checked"`
	SkippedChecks  []string           `json:"skipped_checks"`
	MissingLocal   []string           `json:"missing_local"`
	MissingReasons map[string]string  `json:"missing_reasons"`
	Note           string             `json:"note,omitempty"`
}

func localNewE911Result() *localE911Result {
	return &localE911Result{
		Findings:       make([]localE911Finding, 0),
		Counts:         localE911Counts{ByKind: map[string]int{}, BySeverity: map[string]int{}},
		SkippedChecks:  make([]string, 0),
		MissingLocal:   make([]string, 0),
		MissingReasons: map[string]string{},
	}
}

// localAddressSeverity grades a not-OK verification status: rejected and
// failed addresses will not route correctly (high), unverified ones are an
// open task (medium), a requested verification is in progress (low).
func localAddressSeverity(code string) string {
	switch strings.TrimSpace(code) {
	case "5", "6":
		return "high"
	case "2":
		return "medium"
	default:
		return "low"
	}
}

var localSeverityRank = map[string]int{"high": 0, "medium": 1, "low": 2}

type localE911Inputs struct {
	users  []localUser
	areas  []localCommonArea
	sites  []localSite
	addrs  []localEmergencyAddress
	locs   []localLocation
	nums   []localNumber
	synced map[string]bool
	cov    localCoverage
	locCov localLocationCoverage
	byoc   localByocWalk
}

// localByocWalk is the BYOC numbers walk the BYOC check may use.
type localByocWalk struct {
	// current: a completed type=byoc walk not superseded by a later
	// unfiltered numbers sync (which rewrites the rows without BYOC fields).
	current bool
	// superseded: a walk exists but a later numbers sync replaced its rows.
	superseded bool
	// inProgress: a newer BYOC walk attempt started and did not complete,
	// so the previous walk is not used over the rows it partly rewrote.
	inProgress bool
	// rows are the number row ids that walk wrote.
	rows map[string]bool
}

// localLoadE911Inputs drains every table the audit needs before the caller
// closes the store.
func localLoadE911Inputs(q localQuerier) (*localE911Inputs, error) {
	in := &localE911Inputs{}
	var err error
	if in.users, err = localLoadUsers(q); err != nil {
		return nil, err
	}
	if in.areas, err = localLoad[localCommonArea](q, "common_areas"); err != nil {
		return nil, err
	}
	if in.sites, err = localLoad[localSite](q, "sites"); err != nil {
		return nil, err
	}
	if in.addrs, err = localLoad[localEmergencyAddress](q, "emergency_addresses"); err != nil {
		return nil, err
	}
	if in.locs, err = localLoadLocations(q); err != nil {
		return nil, err
	}
	if in.nums, err = localLoadNumbers(q); err != nil {
		return nil, err
	}
	in.cov = localCoverageFor(q, nil, localE911SyncResources...)
	in.synced = in.cov.synced
	// numbers: the audit reads numbers only for the BYOC carrier check, and
	// only the type=byoc walk carries the BYOC fields. A completed, current
	// BYOC walk covers what the audit needs; an unfiltered full numbers sync
	// also counts as covered (the BYOC check is then skipped with a hint).
	// Supersession comes from row evidence (the recorder marks the walk
	// when another numbers sync rewrites its rows), never from timestamps.
	// Strict read: an I/O error fails the audit instead of quietly reporting
	// the BYOC check as not covered; a legacy schema degrades to no walks.
	numberWalks, err := localReadWalksErr(q, "numbers")
	if err != nil {
		if !localLegacyWalkSchema(err) {
			return nil, err
		}
		// A store written before the walk schema was complete: no walk is
		// usable, so the BYOC check reports not covered.
		numberWalks = map[string]localWalk{}
	}
	if w, ok := numberWalks["type=byoc"]; ok {
		switch {
		case w.pending:
			in.byoc = localByocWalk{inProgress: true}
		case w.walkComplete && w.superseded:
			in.byoc = localByocWalk{superseded: true}
		case w.usable():
			in.byoc = localByocWalk{current: true, rows: w.rows}
		}
	}
	if in.byoc.inProgress && !in.synced["numbers"] {
		in.cov.reasons["numbers"] = "the last BYOC numbers walk did not complete (failed or interrupted), so the previous walk is not used; re-run: " + localByocSyncHint()
	}
	if in.byoc.current && !in.synced["numbers"] {
		in.synced["numbers"] = true
		delete(in.cov.reasons, "numbers")
	}
	// locations may also be covered site by site (multi-site accounts).
	in.locCov = localLocationCoverageFor(q, in.cov, in.sites)
	if !in.synced["locations"] {
		if in.locCov.complete(in.sites) {
			in.synced["locations"] = true
			delete(in.cov.reasons, "locations")
		} else if len(in.locCov.uncovered) > 0 {
			reason := fmt.Sprintf("no completed per-site locations sync (as new as the sites sync) for site(s) %s, and no unfiltered full locations sync; run: %s",
				strings.Join(in.locCov.uncovered, ", "), localPerSiteLocationsHint(in.locCov.uncovered))
			if len(in.locCov.pending) > 0 {
				reason += fmt.Sprintf("; the last walk for site(s) %s did not complete (failed or interrupted), so its previous walk is not used; re-run it", strings.Join(in.locCov.pending, ", "))
			}
			in.cov.reasons["locations"] = reason
		}
	}
	return in, nil
}

// localE911Audit is the pure join behind `e911 audit`.
//
// locCov decides, per site, whether SITE_NO_ERL can be judged: a site whose
// locations were never walked completely is skipped, not flagged.
//
// byoc is the BYOC numbers walk: the carrier check reads only the rows that
// walk wrote, so a number that has left BYOC is never flagged.
func localE911Audit(users []localUser, areas []localCommonArea, sites []localSite, addrs []localEmergencyAddress, locations []localLocation, numbers []localNumber, synced map[string]bool, locCov localLocationCoverage, byoc localByocWalk) *localE911Result {
	res := localNewE911Result()
	addrSynced := synced["emergency_addresses"]
	add := func(f localE911Finding) { res.Findings = append(res.Findings, f) }

	addressFinding := func(entityType, id, name, site string, eff localEffectiveAddress) {
		switch {
		case eff.Source == "none":
			add(localE911Finding{Kind: "NO_ADDRESS", Severity: "high", EntityType: entityType, EntityID: id, Name: name, Site: site, AddressSource: eff.Source,
				Detail: "no personal emergency address, no site default address and no account default address"})
		case !eff.OK:
			add(localE911Finding{Kind: "ADDRESS_NOT_VERIFIED", Severity: localAddressSeverity(eff.StatusCode), Status: eff.Status, EntityType: entityType, EntityID: id, Name: name, Site: site,
				AddressSource: eff.Source, AddressID: eff.ID,
				Detail: fmt.Sprintf("effective %s address %q is %s", strings.ReplaceAll(eff.Source, "_", " "), eff.Address, eff.Status)})
		}
	}

	if !addrSynced {
		res.SkippedChecks = append(res.SkippedChecks,
			"NO_ADDRESS, ADDRESS_NOT_VERIFIED, SITE_NO_DEFAULT_ADDRESS: emergency_addresses not synced")
	}
	// An entity collection that does not cover the account (never synced,
	// unfinished, or filtered) still yields true findings for the entities
	// that are stored; it only means the sweep is not account-wide.
	for _, rt := range []struct {
		resource, what string
		stored         int
	}{
		{"users", "user address checks (NO_ADDRESS, ADDRESS_NOT_VERIFIED)", len(users)},
		{"common_areas", "common area address checks (NO_ADDRESS, ADDRESS_NOT_VERIFIED)", len(areas)},
		{"sites", "site checks (SITE_NO_DEFAULT_ADDRESS, SITE_NO_ERL)", len(sites)},
	} {
		if synced[rt.resource] {
			continue
		}
		msg := rt.what + ": " + rt.resource + " not synced"
		if rt.stored > 0 {
			msg = fmt.Sprintf("%s: %s not synced for the whole account; only the %d stored locally were checked", rt.what, rt.resource, rt.stored)
		}
		res.SkippedChecks = append(res.SkippedChecks, msg)
	}
	for _, u := range users {
		// Users whose license status is not "activate" cannot place calls
		// (shared inactive rule, status part only).
		if zoomUserInactiveReason(u.Status.String(), "") != "" {
			res.Checked.SkippedInactiveUsers++
			continue
		}
		res.Checked.Users++
		if !addrSynced {
			continue
		}
		eff := localResolveAddress(addrs, true, u.ID.String(), u.ExtensionNumber.String(), u.Site.ID.String())
		name := u.Name.String()
		if e := u.Email.String(); e != "" {
			name = fmt.Sprintf("%s <%s>", name, e)
		}
		addressFinding("user", u.ID.String(), name, localSiteLabel(u.Site), eff)
	}
	for _, a := range areas {
		res.Checked.CommonAreas++
		if !addrSynced {
			continue
		}
		eff := localResolveAddress(addrs, true, a.ID.String(), a.ExtensionNumber.String(), a.Site.ID.String())
		addressFinding("common_area", a.ID.String(), a.DisplayName.String(), localSiteLabel(a.Site), eff)
	}

	// siteERLKnown reports whether a site's locations were walked completely.
	siteERLKnown := func(sid string) bool {
		if locCov.all {
			return true
		}
		_, ok := locCov.sites[sid]
		return ok
	}
	switch {
	case locCov.all:
	case len(locCov.sites) == 0:
		res.SkippedChecks = append(res.SkippedChecks, "SITE_NO_ERL: locations not synced")
	case len(locCov.uncovered) > 0:
		res.SkippedChecks = append(res.SkippedChecks, fmt.Sprintf("SITE_NO_ERL: locations not synced for site(s) %s; run: %s",
			strings.Join(locCov.uncovered, ", "), localPerSiteLocationsHint(locCov.uncovered)))
	}
	locBySite := map[string]int{}
	for _, l := range locations {
		sid := l.Site.ID.String()
		if !locCov.all {
			// Only rows the covering per-site walk returned count; other rows
			// of that site were not returned by it (stale).
			walkRows, ok := locCov.sites[sid]
			if !ok || !walkRows[l.storageID] {
				continue
			}
		}
		locBySite[sid]++
	}
	for _, s := range sites {
		res.Checked.Sites++
		sid := s.ID.String()
		ref := localRef{ID: s.ID, Name: s.Name}
		if addrSynced {
			var def *localEmergencyAddress
			for i := range addrs {
				a := addrs[i]
				if a.Level.String() == "0" && bool(a.IsDefault) && a.Site.ID.String() == sid {
					def = &addrs[i]
					break
				}
			}
			if def == nil {
				add(localE911Finding{Kind: "SITE_NO_DEFAULT_ADDRESS", Severity: "high", EntityType: "site", EntityID: sid, Name: s.Name.String(), Site: localSiteLabel(ref),
					Detail: "site has no default company emergency address; users without a personal address fall through"})
			} else if !localAddressStatusOK(def.Status.String()) {
				add(localE911Finding{Kind: "ADDRESS_NOT_VERIFIED", Severity: localAddressSeverity(def.Status.String()), Status: localAddressStatusLabel(def.Status.String()),
					EntityType: "site", EntityID: sid, Name: s.Name.String(), Site: localSiteLabel(ref), AddressSource: "site_default", AddressID: def.ID.String(),
					Detail: fmt.Sprintf("site default address %q is %s", def.oneLine(), localAddressStatusLabel(def.Status.String()))})
			}
		}
		if siteERLKnown(sid) && locBySite[sid] == 0 {
			add(localE911Finding{Kind: "SITE_NO_ERL", Severity: "low", EntityType: "site", EntityID: sid, Name: s.Name.String(), Site: localSiteLabel(ref),
				Detail: "site has no emergency service locations (ERLs), so nomadic E911 cannot detect a caller's location"})
		}
	}

	for _, n := range numbers {
		res.Checked.Numbers++
		// Only rows the current BYOC walk wrote carry trustworthy BYOC
		// fields; a row it did not return (a number no longer BYOC, or
		// removed) keeps stale fields and is ignored.
		if !byoc.current || !byoc.rows[n.storageID] || n.EmergencyAddressStatus == nil {
			continue
		}
		if n.EmergencyAddressStatus.String() == "1" {
			add(localE911Finding{Kind: "BYOC_CARRIER_UPDATE_REQUIRED", Severity: "medium", Status: "carrier_update_required", EntityType: "phone_number",
				EntityID: n.ID.String(), Name: n.Number.String(), Site: localSiteLabel(n.Site),
				Detail: "BYOC number's emergency address needs a carrier update (emergency_address_status=1)"})
		}
	}
	switch {
	case byoc.current:
	case byoc.inProgress:
		res.SkippedChecks = append(res.SkippedChecks,
			"BYOC_CARRIER_UPDATE_REQUIRED: the last BYOC numbers walk did not complete (failed or interrupted), so the previous walk is not used; re-run: "+localByocSyncHint())
	case byoc.superseded:
		res.SkippedChecks = append(res.SkippedChecks,
			"BYOC_CARRIER_UPDATE_REQUIRED: a later numbers sync replaced the BYOC walk's rows; run: "+localByocSyncHint())
	default:
		res.SkippedChecks = append(res.SkippedChecks,
			"BYOC_CARRIER_UPDATE_REQUIRED: no completed BYOC numbers walk (only type=byoc rows carry the carrier status); run: "+localByocSyncHint())
	}

	sort.SliceStable(res.Findings, func(i, j int) bool {
		a, b := res.Findings[i], res.Findings[j]
		if localSeverityRank[a.Severity] != localSeverityRank[b.Severity] {
			return localSeverityRank[a.Severity] < localSeverityRank[b.Severity]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.EntityType != b.EntityType {
			return a.EntityType < b.EntityType
		}
		return a.Name < b.Name
	})
	for _, f := range res.Findings {
		res.Counts.Total++
		res.Counts.ByKind[f.Kind]++
		res.Counts.BySeverity[f.Severity]++
	}
	return res
}

func newNovelE911AuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "See every user, common area and site whose emergency address is missing or unverified",
		Long: strings.TrimSpace(`
Use this command for an account-wide sweep of users, common areas and sites whose effective emergency address is missing or not verified, sites with no emergency service locations, and BYOC numbers needing a carrier update. Do NOT use this command for one user's emergency address; use 'footprint' instead.

Reads the local store only. The effective address of a user or common area
is its personal (level 1) address, else its site's default company address,
else an account-wide default. Verification status follows the spec: 1 not
required and 4 verified are OK; 2 unverified, 3 verification requested,
5 rejected and 6 verification failed are flagged. Users whose license
status is not "activate" are skipped. Findings kinds: NO_ADDRESS,
ADDRESS_NOT_VERIFIED, SITE_NO_DEFAULT_ADDRESS, SITE_NO_ERL,
BYOC_CARRIER_UPDATE_REQUIRED.

Checks whose data never finished syncing are listed under skipped_checks
instead of being reported as findings. "assessed" is true only when every
resource the audit reads has a completed, full, unfiltered sync
(zoom-phone-cli sync --full; a plain sync does not delete rows the API no
longer returns), i.e. missing_local is empty; missing_reasons says why each
missing resource does not count. The BYOC check needs the additive BYOC
walk, zoom-phone-cli sync --resources numbers --resource-param
numbers:type=byoc (additive: a filtered sync never prunes, with or without
--full). It reads only the rows that walk wrote, so a number that
has left BYOC is never flagged; for this audit that walk also counts as
numbers coverage. numbers inventory keeps its own unfiltered --full
coverage, which the BYOC walk does not disturb.

Multi-site accounts: GET /phone/locations requires site_id there, so sync
locations once per site with zoom-phone-cli sync --resources locations
--resource-param locations:site_id=<site id> (a filtered sync never prunes,
so the per-site syncs add up). Locations then count as
covered when every synced site has such a sync at least as new as the sites
sync; SITE_NO_ERL is judged only for covered sites, counting only the rows
that site's walk returned, and the uncovered site ids are listed.

With no store, an empty
store or a partial sync it is false; zero findings then
does not mean the account is clean. With no store every check is listed
under skipped_checks. A --db path that does not exist is a usage error
(exit 2).`),
		Example: strings.Trim(`
  zoom-phone-cli e911 audit --agent
  zoom-phone-cli e911 audit --json --select findings.kind,findings.name,findings.detail`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "e911 audit")
			}
			res := localNewE911Result()
			db, err := localOpenStore(cmd, dbPath, localE911SyncResources)
			if err != nil {
				return err
			}
			if db != nil {
				var in *localE911Inputs
				err := localReadSnapshot(cmd.Context(), db, func(q localQuerier) error {
					var err error
					in, err = localLoadE911Inputs(q)
					return err
				})
				_ = db.Close()
				if err != nil {
					return err
				}
				res = localE911Audit(in.users, in.areas, in.sites, in.addrs, in.locs, in.nums, in.synced, in.locCov, in.byoc)
				res.MissingLocal = localMissing(in.synced, localE911SyncResources...)
				res.MissingReasons = localMissingReasons(in.cov, res.MissingLocal)
				// assessed: every resource the audit reads finished syncing.
				res.Assessed = len(res.MissingLocal) == 0
				localHintMissing(cmd, res.MissingLocal, res.MissingReasons)
			} else {
				res = localUnassessedE911()
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), res, flags)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SEVERITY\tKIND\tENTITY\tNAME\tSITE\tDETAIL")
			for _, f := range res.Findings {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", f.Severity, f.Kind, f.EntityType, f.Name, f.Site, f.Detail)
			}
			fmt.Fprintf(w, "\n%d finding(s); checked %d users, %d common areas, %d sites, %d numbers\n",
				res.Counts.Total, res.Checked.Users, res.Checked.CommonAreas, res.Checked.Sites, res.Checked.Numbers)
			for _, s := range res.SkippedChecks {
				fmt.Fprintf(w, "skipped: %s\n", s)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: the synced store for the active account)")
	return cmd
}
