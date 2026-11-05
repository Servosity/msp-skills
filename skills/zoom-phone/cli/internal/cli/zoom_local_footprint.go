// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Footprint join shared by `footprint` and `offboard` (slice prefix "local").

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/cliutil"
)

// localFootprintSyncResources are the sync resources footprint and offboard
// read. A user's desk phones are in the "assigned" device partition
// (/phone/devices?type=assigned); "unassigned" devices belong to nobody.
var localFootprintSyncResources = []string{
	"users", "call_queues", "shared_line_groups", "group_call_pickup", "monitoring_groups",
	"assigned", "numbers", "emergency_addresses",
}

type localFootprintData struct {
	users     []localUser
	groups    map[string][]localGroup
	devices   []localDevice
	numbers   []localNumber
	addresses []localEmergencyAddress
	synced    map[string]bool
	missing   []string
	reasons   map[string]string
}

func localLoadFootprintData(q localQuerier) (*localFootprintData, error) {
	d := &localFootprintData{groups: map[string][]localGroup{}}
	var err error
	if d.users, err = localLoadUsers(q); err != nil {
		return nil, err
	}
	for _, fam := range localMembershipFamilies {
		gs, err := localLoad[localGroup](q, fam)
		if err != nil {
			return nil, err
		}
		kept := make([]localGroup, 0, len(gs))
		for _, g := range gs {
			if g.ID.String() != "" {
				kept = append(kept, g)
			}
		}
		d.groups[fam] = kept
	}
	if d.devices, err = localLoadDevices(q); err != nil {
		return nil, err
	}
	if d.numbers, err = localLoad[localNumber](q, "numbers"); err != nil {
		return nil, err
	}
	if d.addresses, err = localLoad[localEmergencyAddress](q, "emergency_addresses"); err != nil {
		return nil, err
	}
	cov := localCoverageFor(q, nil, localFootprintSyncResources...)
	d.synced = cov.synced
	d.missing = localMissing(d.synced, localFootprintSyncResources...)
	d.reasons = localMissingReasons(cov, d.missing)
	return d, nil
}

type localFootprintSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type localFootprintUser struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Email            string              `json:"email"`
	ExtensionID      string              `json:"extension_id"`
	ExtensionNumber  string              `json:"extension_number"`
	Status           string              `json:"status"`
	ActivationStatus string              `json:"activation_status"`
	Department       string              `json:"department,omitempty"`
	CostCenter       string              `json:"cost_center,omitempty"`
	Site             *localFootprintSite `json:"site"`
}

type localFootprintNumber struct {
	ID         string `json:"id"`
	Number     string `json:"number"`
	Source     string `json:"source,omitempty"`
	Status     string `json:"status,omitempty"`
	NumberType string `json:"number_type,omitempty"`
	Site       string `json:"site,omitempty"`
}

type localFootprintPlan struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	BillingAccountID string `json:"billing_account_id,omitempty"`
}

type localFootprintDevice struct {
	ID                  string `json:"id"`
	DisplayName         string `json:"display_name"`
	DeviceType          string `json:"device_type"`
	MacAddress          string `json:"mac_address"`
	Status              string `json:"status"`
	FirmwareVersion     string `json:"firmware_version,omitempty"`
	Site                string `json:"site,omitempty"`
	AssigneeExtensionID string `json:"assignee_extension_id,omitempty"`
}

// localDeviceConflict is a desk phone whose assigned/unassigned rows tie, so
// it is neither listed as the user's desk phone nor removed by offboard.
type localDeviceConflict struct {
	DeviceID    string   `json:"device_id"`
	DisplayName string   `json:"display_name"`
	MacAddress  string   `json:"mac_address,omitempty"`
	Sources     []string `json:"sources"`
	Reason      string   `json:"reason"`
}

type localFootprint struct {
	Query string `json:"query"`
	// Assessed is true only when every resource the command reads has a
	// completed sync (missing_local is empty); no store, an empty store or
	// a partial sync is assessed: false.
	Assessed         bool                   `json:"assessed"`
	MatchedBy        string                 `json:"matched_by,omitempty"`
	User             *localFootprintUser    `json:"user"`
	PhoneNumbers     []localFootprintNumber `json:"phone_numbers"`
	CallingPlans     []localFootprintPlan   `json:"calling_plans"`
	CallQueues       []localMembership      `json:"call_queues"`
	SharedLineGroups []localMembership      `json:"shared_line_groups"`
	GroupCallPickup  []localMembership      `json:"group_call_pickup"`
	MonitoringGroups []localMembership      `json:"monitoring_groups"`
	DeskPhones       []localFootprintDevice `json:"desk_phones"`
	DeviceConflicts  []localDeviceConflict  `json:"device_conflicts"`
	EmergencyAddress *localEffectiveAddress `json:"emergency_address"`
	MembershipSource string                 `json:"membership_source"`
	MembershipScan   []localFamilyScan      `json:"membership_scan"`
	SkippedLive      []string               `json:"skipped_live"`
	FetchFailures    []localFetchFailure    `json:"fetch_failures"`
	Truncated        []localTruncation      `json:"truncated"`
	MissingLocal     []string               `json:"missing_local"`
	MissingReasons   map[string]string      `json:"missing_reasons"`
	Note             string                 `json:"note,omitempty"`

	// liveRan is true only once the live membership scan actually ran; it
	// drives the truthful meta.source.
	liveRan bool
}

// localUnassessedFootprint is the result when there is no local store at
// all: nothing was assessed and every resource footprint reads is missing.
func localUnassessedFootprint(query string) *localFootprint {
	fp := localEmptyFootprint(query)
	fp.MissingLocal = append(fp.MissingLocal, localFootprintSyncResources...)
	sort.Strings(fp.MissingLocal)
	for _, m := range fp.MissingLocal {
		fp.MissingReasons[m] = "no local store"
	}
	fp.Note = "no local store; run: " + localSyncCommand(localFootprintSyncResources)
	return fp
}

func localEmptyFootprint(query string) *localFootprint {
	return &localFootprint{
		Query:            query,
		PhoneNumbers:     make([]localFootprintNumber, 0),
		CallingPlans:     make([]localFootprintPlan, 0),
		CallQueues:       make([]localMembership, 0),
		SharedLineGroups: make([]localMembership, 0),
		GroupCallPickup:  make([]localMembership, 0),
		MonitoringGroups: make([]localMembership, 0),
		DeskPhones:       make([]localFootprintDevice, 0),
		DeviceConflicts:  make([]localDeviceConflict, 0),
		MembershipSource: "none",
		MembershipScan:   make([]localFamilyScan, 0),
		SkippedLive:      make([]string, 0),
		FetchFailures:    make([]localFetchFailure, 0),
		Truncated:        make([]localTruncation, 0),
		MissingLocal:     make([]string, 0),
		MissingReasons:   map[string]string{},
	}
}

// localResolved is the outcome of opening the store and resolving the query.
// data is nil when there is no local store at the default path; user is nil
// when the store has no phone users.
type localResolved struct {
	data      *localFootprintData
	user      *localUser
	matchedBy string
	// note explains a nil user when the store has users rows.
	note string
}

// localResolveFootprintUser opens the store and resolves the query to one
// user. exactOnly (offboard --apply) accepts only an exact user id, email or
// extension number; a phone-number, name or substring match is refused with
// exit 2 so a destructive run can never land on a fuzzy match.
func localResolveFootprintUser(cmd *cobra.Command, dbPath, query string, exactOnly bool) (localResolved, error) {
	var res localResolved
	db, err := localOpenStore(cmd, dbPath, localFootprintSyncResources)
	if err != nil || db == nil {
		return res, err
	}
	var data *localFootprintData
	err = localReadSnapshot(cmd.Context(), db, func(q localQuerier) error {
		var err error
		data, err = localLoadFootprintData(q)
		return err
	})
	_ = db.Close()
	if err != nil {
		return res, err
	}
	res.data = data
	localHintMissing(cmd, data.missing, data.reasons)
	if len(data.users) == 0 {
		return res, nil
	}
	matches, kind := localMatchUsers(data.users, query)
	switch {
	case len(matches) == 0 && !data.synced["users"]:
		// The users rows do not cover the account (filtered or unfinished
		// sync), so their absence proves nothing: report unassessed, not
		// "not found".
		res.note = fmt.Sprintf("no match for %q in the local users rows, but users do not cover the account (%s)", query, data.reasons["users"])
		return res, nil
	case len(matches) == 0:
		return res, notFoundErr(fmt.Errorf("no phone user matches %q in the local store (%d users synced); try an email, extension number or name", query, len(data.users)))
	case len(matches) > 1:
		return res, localAmbiguousErr(query, matches)
	case exactOnly && !localMatchIsExact(kind):
		u := matches[0]
		return res, usageErr(fmt.Errorf("--apply needs an exact user id, email or extension number; %q matched %s <%s> ext %s only by %s. Re-run with --apply %s",
			query, u.Name.String(), u.Email.String(), u.ExtensionNumber.String(), strings.ReplaceAll(kind, "_", " "), u.Email.String()))
	}
	res.user = &matches[0]
	res.matchedBy = kind
	return res, nil
}

// localBuildFootprint joins the user's local rows and, unless noLive, the
// live membership scan.
//
// fresh (offboard --apply) lists every group family live and never skips a
// group on a cached zero member count.
func localBuildFootprint(ctx context.Context, cmd *cobra.Command, flags *rootFlags, data *localFootprintData, u localUser, query string, noLive, fresh bool) *localFootprint {
	fp := localEmptyFootprint(query)
	fp.MissingLocal = append(fp.MissingLocal, data.missing...)
	fp.MissingReasons = data.reasons
	// assessed means every resource footprint reads finished syncing.
	fp.Assessed = len(fp.MissingLocal) == 0
	fu := &localFootprintUser{
		ID:               u.ID.String(),
		Name:             u.Name.String(),
		Email:            u.Email.String(),
		ExtensionID:      u.ExtensionID.String(),
		ExtensionNumber:  u.ExtensionNumber.String(),
		Status:           u.Status.String(),
		ActivationStatus: u.ActivationStatus.String(),
		Department:       u.Department.String(),
		CostCenter:       u.CostCenter.String(),
	}
	if u.Site.ID.String() != "" || u.Site.Name.String() != "" {
		fu.Site = &localFootprintSite{ID: u.Site.ID.String(), Name: u.Site.Name.String()}
	}
	fp.User = fu

	// Numbers: the user row's phone_numbers[] plus any /phone/numbers row
	// whose assignee is this user, enriched from the numbers row.
	numByID := map[string]localNumber{}
	for _, n := range data.numbers {
		numByID[n.ID.String()] = n
	}
	seen := map[string]bool{}
	addNumber := func(id, number string) {
		key := id
		if key == "" {
			key = "#" + number
		}
		if seen[key] {
			return
		}
		seen[key] = true
		row := localFootprintNumber{ID: id, Number: number}
		if n, ok := numByID[id]; ok && id != "" {
			if row.Number == "" {
				row.Number = n.Number.String()
			}
			row.Source = localNumberSourceClass(n)
			row.Status = n.Status.String()
			row.NumberType = n.NumberType.String()
			row.Site = localSiteLabel(n.Site)
		}
		fp.PhoneNumbers = append(fp.PhoneNumbers, row)
	}
	for _, pn := range u.PhoneNumbers {
		addNumber(pn.ID.String(), pn.Number.String())
	}
	for _, n := range data.numbers {
		if n.Assignee != nil && strings.EqualFold(n.Assignee.Type.String(), "user") && localSameNonEmpty(n.Assignee.ID.String(), u.ID.String()) {
			addNumber(n.ID.String(), n.Number.String())
		}
	}

	for _, p := range u.CallingPlans {
		fp.CallingPlans = append(fp.CallingPlans, localFootprintPlan{
			Type:             p.Type.String(),
			Name:             p.Name.String(),
			BillingAccountID: p.BillingAccountID.String(),
		})
	}

	for _, d := range data.devices {
		for _, a := range localDeviceAssigneeList(d) {
			if strings.EqualFold(a.ExtensionType.String(), "commonArea") {
				continue
			}
			if !localMemberMatches(a.ID.String(), a.ExtensionID.String(), a.ExtensionNumber.String(), a.ExtensionType.String(), u) {
				continue
			}
			if d.ambiguous() {
				fp.DeviceConflicts = append(fp.DeviceConflicts, localDeviceConflict{
					DeviceID:    d.ID.String(),
					DisplayName: d.DisplayName.String(),
					MacAddress:  d.MacAddress.String(),
					Sources:     d.ambiguousSources,
					Reason:      "the device is in both the assigned and unassigned lists synced at the same time; re-sync: " + localSyncCommand([]string{"assigned", "unassigned"}),
				})
				break
			}
			ext := a.ExtensionID.String()
			if ext == "" {
				ext = u.ExtensionID.String()
			}
			fp.DeskPhones = append(fp.DeskPhones, localFootprintDevice{
				ID:                  d.ID.String(),
				DisplayName:         d.DisplayName.String(),
				DeviceType:          d.DeviceType.String(),
				MacAddress:          d.MacAddress.String(),
				Status:              d.Status.String(),
				FirmwareVersion:     d.FirmwareVersion.String(),
				Site:                localSiteLabel(d.Site),
				AssigneeExtensionID: ext,
			})
			break
		}
	}

	addr := localResolveAddress(data.addresses, data.synced["emergency_addresses"], u.ID.String(), u.ExtensionNumber.String(), u.Site.ID.String())
	fp.EmergencyAddress = &addr

	notes := []string{}
	if noLive {
		fp.MembershipSource = "skipped"
		fp.SkippedLive = append(fp.SkippedLive, localMembershipFamilies...)
		notes = append(notes, "live membership fetch skipped: call queue, shared line group, pickup and monitoring group memberships are not in the local store")
	} else {
		fp.MembershipSource = "live"
		c, err := flags.newClient()
		if err != nil {
			fp.MembershipSource = "failed"
			for _, fam := range localMembershipFamilies {
				fp.FetchFailures = append(fp.FetchFailures, localFetchFailure{Family: fam, Error: err.Error()})
			}
		} else {
			c.NoCache = true
			scan := localScanMemberships(ctx, c, data.groups, data.synced, u, localScanOptions{dogfood: cliutil.IsDogfoodEnv(), fresh: fresh})
			fp.liveRan = true
			fp.MembershipScan = scan.Scans
			fp.Truncated = append(fp.Truncated, scan.Truncated...)
			fp.FetchFailures = append(fp.FetchFailures, scan.Failures...)
			notes = append(notes, scan.Notes...)
			for _, m := range scan.Memberships {
				switch m.Family {
				case localFamilyQueues:
					fp.CallQueues = append(fp.CallQueues, m)
				case localFamilySLG:
					fp.SharedLineGroups = append(fp.SharedLineGroups, m)
				case localFamilyPickup:
					fp.GroupCallPickup = append(fp.GroupCallPickup, m)
				case localFamilyMonitoring:
					fp.MonitoringGroups = append(fp.MonitoringGroups, m)
				}
			}
		}
		if len(fp.FetchFailures) > 0 {
			localWarnFailures(cmd.ErrOrStderr(), fp.FetchFailures)
			notes = append(notes, fmt.Sprintf("%d membership fetch(es) failed; memberships in those groups may be missing", len(fp.FetchFailures)))
		}
	}
	fp.Note = strings.Join(notes, "; ")
	if !fp.Assessed {
		fp.Note = localNotAssessedNote(fp.MissingLocal, fp.Note)
	}
	return fp
}
