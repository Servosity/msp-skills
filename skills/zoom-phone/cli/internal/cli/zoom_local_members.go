// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Live membership discovery for footprint and offboard (slice prefix
// "local"). Sync stores call queues, shared line groups, group call pickup
// groups and monitoring groups as list rows only; none of their member lists
// is a sync resource, so membership is read live from the member endpoints:
//
//	GET /phone/call_queues/{callQueueId}/members            -> .call_queue_members[]
//	GET /phone/shared_line_groups/{sharedLineGroupId}       -> .members.users[] (detail only)
//	GET /phone/group_call_pickup/{groupId}/members          -> .group_call_pickup_member[]
//	GET /phone/monitoring_groups/{monitoringGroupId}/monitor_members?member_type=monitor|monitored -> .members[]

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const (
	localFamilyQueues     = "call_queues"
	localFamilySLG        = "shared_line_groups"
	localFamilyPickup     = "group_call_pickup"
	localFamilyMonitoring = "monitoring_groups"

	localMemberConcurrency = 4
	localDogfoodGroupCap   = 10
	localPageSize          = "100"
)

var localMembershipFamilies = []string{localFamilyQueues, localFamilySLG, localFamilyPickup, localFamilyMonitoring}

// localGroupListSpec maps each family to its account-wide list endpoint and
// the response array key (from the spec 200 schemas).
var localGroupListSpec = map[string]struct{ path, key string }{
	localFamilyQueues:     {"/phone/call_queues", "call_queues"},
	localFamilySLG:        {"/phone/shared_line_groups", "shared_line_groups"},
	localFamilyPickup:     {"/phone/group_call_pickup", "group_call_pickup"},
	localFamilyMonitoring: {"/phone/monitoring_groups", "monitoring_groups"},
}

type localGetter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// localMembership is one group the user belongs to, with the identifiers the
// matching DELETE endpoint needs.
type localMembership struct {
	Family               string `json:"family"`
	GroupID              string `json:"id"`
	GroupName            string `json:"name"`
	GroupExtensionNumber string `json:"extension_number,omitempty"`
	Site                 string `json:"site,omitempty"`
	MemberID             string `json:"member_id,omitempty"`
	MemberExtensionID    string `json:"member_extension_id,omitempty"`
	MemberType           string `json:"member_type,omitempty"`
	ReceiveCall          *bool  `json:"receive_call,omitempty"`
}

type localFetchFailure struct {
	Family    string `json:"family"`
	GroupID   string `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	Error     string `json:"error"`
}

type localFamilyScan struct {
	Family       string `json:"family"`
	ListSource   string `json:"list_source"`
	Groups       int    `json:"groups"`
	Scanned      int    `json:"scanned"`
	SkippedEmpty int    `json:"skipped_empty"`
	Capped       bool   `json:"capped,omitempty"`
}

// localTruncation records a list that stopped at a page or group cap, so
// some memberships may be missing. offboard --apply refuses while any exist.
type localTruncation struct {
	Family    string `json:"family"`
	GroupID   string `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	Reason    string `json:"reason"`
}

type localMembershipResult struct {
	Memberships []localMembership
	Failures    []localFetchFailure
	Truncated   []localTruncation
	Scans       []localFamilyScan
	Notes       []string
}

// localScanOptions controls one membership scan.
type localScanOptions struct {
	// dogfood caps list pages and groups for the live dogfood matrix.
	dogfood bool
	// fresh (offboard --apply) lists every family live instead of trusting
	// the synced group lists, and never skips a group because a member
	// count says it is empty.
	fresh bool
}

// localFetchPaged walks a Zoom list endpoint with next_page_token.
func localFetchPaged(ctx context.Context, c localGetter, path string, base map[string]string, itemsKey string, maxPages int) ([]json.RawMessage, bool, error) {
	out := make([]json.RawMessage, 0)
	token := ""
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		params := map[string]string{}
		for k, v := range base {
			params[k] = v
		}
		if token != "" {
			params["next_page_token"] = token
		}
		raw, err := c.Get(ctx, path, params)
		if err != nil {
			return out, false, err
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(raw, &env); err != nil {
			return out, false, fmt.Errorf("decoding %s response: %w", path, err)
		}
		if v, ok := env[itemsKey]; ok {
			var items []json.RawMessage
			if err := json.Unmarshal(v, &items); err == nil {
				out = append(out, items...)
			}
		}
		var next string
		if v, ok := env["next_page_token"]; ok {
			_ = json.Unmarshal(v, &next)
		}
		if strings.TrimSpace(next) == "" {
			return out, false, nil
		}
		// A cursor the walk has already followed (the same token again, or
		// any cycle) means the list can never be walked to its end: that is
		// a failure, not a natural end, so callers record it and offboard
		// --apply refuses.
		if seen[next] {
			return out, false, fmt.Errorf("%s returned next_page_token %q again after %d page(s); the list cannot be walked to its end", path, next, page+1)
		}
		seen[next] = true
		token = next
	}
	return out, true, nil
}

type localMemberJob struct {
	family     string
	group      localGroup
	memberType string
}

func localMemberJobLabel(j localMemberJob) string {
	if j.memberType != "" {
		return j.family + "/" + j.memberType
	}
	return j.family
}

// localCountIsZero reports a list row that declares an empty member count.
func localCountIsZero(c *zoomFlexString) bool {
	return c != nil && c.String() == "0"
}

// localMaxPages caps every list walk; a var so tests can force truncation.
var localMaxPages = 100

// localScanMemberships finds every queue, shared line group, pickup group and
// monitoring group the user belongs to. localGroups holds the synced list
// rows per family; a family absent from synced, or every family when
// opts.fresh, is listed live first.
func localScanMemberships(ctx context.Context, c localGetter, localGroups map[string][]localGroup, synced map[string]bool, user localUser, opts localScanOptions) localMembershipResult {
	res := localMembershipResult{
		Memberships: make([]localMembership, 0),
		Failures:    make([]localFetchFailure, 0),
		Truncated:   make([]localTruncation, 0),
		Scans:       make([]localFamilyScan, 0, len(localMembershipFamilies)),
	}
	listPages := localMaxPages
	if opts.dogfood {
		listPages = 1
	}
	jobs := make([]localMemberJob, 0)
	listedLive := false
	for _, fam := range localMembershipFamilies {
		scan := localFamilyScan{Family: fam, ListSource: "local"}
		groups := localGroups[fam]
		if !synced[fam] || opts.fresh {
			listedLive = true
			scan.ListSource = "live"
			spec := localGroupListSpec[fam]
			raws, capped, err := localFetchPaged(ctx, c, spec.path, map[string]string{"page_size": localPageSize}, spec.key, listPages)
			if err != nil {
				scan.ListSource = "failed"
				res.Failures = append(res.Failures, localFetchFailure{Family: fam, Error: "listing groups: " + err.Error()})
			}
			if capped {
				scan.Capped = true
				reason := fmt.Sprintf("group list stopped after %d page(s); later groups were not scanned", listPages)
				res.Truncated = append(res.Truncated, localTruncation{Family: fam, Reason: reason})
				res.Notes = append(res.Notes, fam+" "+reason)
			}
			groups = make([]localGroup, 0, len(raws))
			for _, raw := range raws {
				var g localGroup
				if json.Unmarshal(raw, &g) == nil && g.ID.String() != "" {
					groups = append(groups, g)
				}
			}
		}
		sort.SliceStable(groups, func(i, j int) bool {
			if groups[i].label() != groups[j].label() {
				return groups[i].label() < groups[j].label()
			}
			return groups[i].ID < groups[j].ID
		})
		scan.Groups = len(groups)
		if opts.dogfood && len(groups) > localDogfoodGroupCap {
			groups = groups[:localDogfoodGroupCap]
			scan.Capped = true
			reason := fmt.Sprintf("dogfood run: membership scan capped at the first %d of %d groups", localDogfoodGroupCap, scan.Groups)
			res.Truncated = append(res.Truncated, localTruncation{Family: fam, Reason: reason})
			res.Notes = append(res.Notes, fam+" "+reason)
		}
		// A cached member count can be stale, so a fresh scan never skips.
		skipZero := func(c *zoomFlexString) bool { return !opts.fresh && localCountIsZero(c) }
		for _, g := range groups {
			switch fam {
			case localFamilyPickup:
				if skipZero(g.MemberCount) {
					scan.SkippedEmpty++
					continue
				}
				jobs = append(jobs, localMemberJob{family: fam, group: g})
				scan.Scanned++
			case localFamilyMonitoring:
				scanned := false
				if skipZero(g.MonitorMembersCount) {
					scan.SkippedEmpty++
				} else {
					jobs = append(jobs, localMemberJob{family: fam, group: g, memberType: "monitor"})
					scanned = true
				}
				if skipZero(g.MonitoredMembersCount) {
					scan.SkippedEmpty++
				} else {
					jobs = append(jobs, localMemberJob{family: fam, group: g, memberType: "monitored"})
					scanned = true
				}
				if scanned {
					scan.Scanned++
				}
			default:
				jobs = append(jobs, localMemberJob{family: fam, group: g})
				scan.Scanned++
			}
		}
		res.Scans = append(res.Scans, scan)
	}

	type jobResult struct {
		matches []localMembership
		capped  bool
		err     error
	}
	results := make([]jobResult, len(jobs))
	// The first token mint races when requests start concurrently, so the
	// first API call is always sequential: either a live group list above
	// or, when every list came from the store, the first member fetch here.
	first := 0
	if !listedLive && len(jobs) > 0 {
		m, capped, err := localFetchGroupMembers(ctx, c, jobs[0], user)
		results[0] = jobResult{matches: m, capped: capped, err: err}
		first = 1
	}
	sem := make(chan struct{}, localMemberConcurrency)
	var wg sync.WaitGroup
	for i := first; i < len(jobs); i++ {
		job := jobs[i]
		wg.Add(1)
		go func(i int, job localMemberJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m, capped, err := localFetchGroupMembers(ctx, c, job, user)
			results[i] = jobResult{matches: m, capped: capped, err: err}
		}(i, job)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			res.Failures = append(res.Failures, localFetchFailure{
				Family:    localMemberJobLabel(jobs[i]),
				GroupID:   jobs[i].group.ID.String(),
				GroupName: jobs[i].group.label(),
				Error:     r.err.Error(),
			})
			continue
		}
		if r.capped {
			res.Truncated = append(res.Truncated, localTruncation{
				Family:    localMemberJobLabel(jobs[i]),
				GroupID:   jobs[i].group.ID.String(),
				GroupName: jobs[i].group.label(),
				Reason:    fmt.Sprintf("member list stopped after %d page(s); the user may be on a later page", localMaxPages),
			})
		}
		res.Memberships = append(res.Memberships, r.matches...)
	}
	return res
}

// localMemberMatches decides whether a live member row is this user. Stable
// ids decide: every id the row and the user both carry must agree, and a
// conflicting id rejects the row outright. The extension number is used
// only for a row that carries no stable id at all, because numbers are
// reassigned: Jane's former extension may now belong to Bob.
func localMemberMatches(memberID, memberExtID, memberExtNumber, memberExtType string, u localUser) bool {
	uid, uext, unum := u.ID.String(), u.ExtensionID.String(), u.ExtensionNumber.String()
	if memberID != "" || memberExtID != "" {
		compared := false
		if memberID != "" && uid != "" {
			if memberID != uid {
				return false
			}
			compared = true
		}
		if memberExtID != "" && uext != "" {
			if memberExtID != uext {
				return false
			}
			compared = true
		}
		return compared
	}
	return strings.EqualFold(memberExtType, "user") && memberExtNumber != "" && memberExtNumber == unum
}

func localFetchGroupMembers(ctx context.Context, c localGetter, job localMemberJob, user localUser) ([]localMembership, bool, error) {
	g := job.group
	gid := g.ID.String()
	base := localMembership{
		Family:               job.family,
		GroupID:              gid,
		GroupName:            g.label(),
		GroupExtensionNumber: g.ExtensionNumber.String(),
		Site:                 localSiteLabel(g.Site),
	}
	uid, uext := user.ID.String(), user.ExtensionID.String()
	out := make([]localMembership, 0)
	switch job.family {
	case localFamilyQueues:
		raws, capped, err := localFetchPaged(ctx, c, "/phone/call_queues/"+url.PathEscape(gid)+"/members", nil, "call_queue_members", localMaxPages)
		if err != nil {
			return nil, false, err
		}
		for _, raw := range raws {
			var m struct {
				ID          zoomFlexString `json:"id"`
				Level       zoomFlexString `json:"level"`
				ReceiveCall *localFlexBool `json:"receive_call"`
				ExtensionID zoomFlexString `json:"extension_id"`
			}
			if json.Unmarshal(raw, &m) != nil || !localMemberMatches(m.ID.String(), m.ExtensionID.String(), "", "", user) {
				continue
			}
			row := base
			row.MemberID = m.ID.String()
			if row.MemberID == "" {
				row.MemberID = uid
			}
			row.MemberExtensionID = m.ExtensionID.String()
			if m.ReceiveCall != nil {
				v := bool(*m.ReceiveCall)
				row.ReceiveCall = &v
			}
			out = append(out, row)
		}
		return out, capped, nil
	case localFamilySLG:
		raw, err := c.Get(ctx, "/phone/shared_line_groups/"+url.PathEscape(gid), nil)
		if err != nil {
			return nil, false, err
		}
		var detail struct {
			Members struct {
				Users []struct {
					ID          zoomFlexString `json:"id"`
					ExtensionID zoomFlexString `json:"extension_id"`
				} `json:"users"`
			} `json:"members"`
		}
		if err := json.Unmarshal(raw, &detail); err != nil {
			return nil, false, fmt.Errorf("decoding shared line group %s: %w", gid, err)
		}
		for _, m := range detail.Members.Users {
			if !localMemberMatches(m.ID.String(), m.ExtensionID.String(), "", "", user) {
				continue
			}
			row := base
			row.MemberID = m.ID.String()
			if row.MemberID == "" {
				row.MemberID = uid
			}
			row.MemberExtensionID = m.ExtensionID.String()
			out = append(out, row)
		}
		return out, false, nil
	case localFamilyPickup:
		raws, capped, err := localFetchPaged(ctx, c, "/phone/group_call_pickup/"+url.PathEscape(gid)+"/members", map[string]string{"page_size": localPageSize}, "group_call_pickup_member", localMaxPages)
		if err != nil {
			return nil, false, err
		}
		for _, raw := range raws {
			var m struct {
				ID              zoomFlexString `json:"id"`
				ExtensionID     zoomFlexString `json:"extension_id"`
				ExtensionType   zoomFlexString `json:"extension_type"`
				ExtensionNumber zoomFlexString `json:"extension_number"`
			}
			if json.Unmarshal(raw, &m) != nil || !localMemberMatches(m.ID.String(), m.ExtensionID.String(), m.ExtensionNumber.String(), m.ExtensionType.String(), user) {
				continue
			}
			row := base
			row.MemberID = m.ID.String()
			row.MemberExtensionID = m.ExtensionID.String()
			if row.MemberExtensionID == "" {
				row.MemberExtensionID = uext
			}
			out = append(out, row)
		}
		return out, capped, nil
	case localFamilyMonitoring:
		raws, capped, err := localFetchPaged(ctx, c, "/phone/monitoring_groups/"+url.PathEscape(gid)+"/monitor_members", map[string]string{"member_type": job.memberType, "page_size": localPageSize}, "members", localMaxPages)
		if err != nil {
			return nil, false, err
		}
		for _, raw := range raws {
			var m struct {
				ExtensionID     zoomFlexString `json:"extension_id"`
				ExtensionNumber zoomFlexString `json:"extension_number"`
				ExtensionType   zoomFlexString `json:"extension_type"`
			}
			if json.Unmarshal(raw, &m) != nil || !localMemberMatches("", m.ExtensionID.String(), m.ExtensionNumber.String(), m.ExtensionType.String(), user) {
				continue
			}
			row := base
			row.MemberType = job.memberType
			row.MemberExtensionID = m.ExtensionID.String()
			if row.MemberExtensionID == "" {
				row.MemberExtensionID = uext
			}
			out = append(out, row)
		}
		return out, capped, nil
	}
	return out, false, nil
}

// localWarnFailures prints one stderr warning per failed fetch.
func localWarnFailures(w io.Writer, failures []localFetchFailure) {
	for _, f := range failures {
		target := f.Family
		if f.GroupName != "" || f.GroupID != "" {
			target = fmt.Sprintf("%s %q (%s)", f.Family, f.GroupName, f.GroupID)
		}
		fmt.Fprintf(w, "warning: could not read %s: %s\n", target, f.Error)
	}
}
