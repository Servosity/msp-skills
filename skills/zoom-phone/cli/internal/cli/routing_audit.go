// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: account-wide sweep for dead IVR targets, empty call queues,
// orphaned auto receptionists and auto receptionists with no upcoming holiday hours.
// pp:data-source live
// Lists and per-entity fan-out are live GETs; nothing is read from the local store.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

const (
	routingAuditConcurrency = 4
	// Under the live-dogfood matrix the per-entity fan-out is sampled so the
	// command finishes inside the harness's per-command timeout.
	routingAuditDogfoodFanout = 10

	routingCodeDeadTarget        = "DEAD_TARGET"
	routingCodeEmptyQueue        = "EMPTY_QUEUE"
	routingCodeOrphanAR          = "ORPHAN_AR"
	routingCodeNoUpcomingHoliday = "NO_UPCOMING_HOLIDAY"
)

type routingAuditFinding struct {
	Code                  string `json:"code"`
	EntityType            string `json:"entity_type"`
	EntityID              string `json:"entity_id"`
	EntityName            string `json:"entity_name"`
	ExtensionNumber       string `json:"extension_number,omitempty"`
	HoursType             string `json:"hours_type,omitempty"`
	Key                   string `json:"key,omitempty"`
	TargetType            string `json:"target_type,omitempty"`
	TargetID              string `json:"target_id,omitempty"`
	TargetName            string `json:"target_name,omitempty"`
	TargetExtensionNumber string `json:"target_extension_number,omitempty"`
	Detail                string `json:"detail"`
}

type routingFetchFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type routingSkippedCheck struct {
	Check    string `json:"check"`
	EntityID string `json:"entity_id,omitempty"`
	Reason   string `json:"reason"`
}

type routingAuditResult struct {
	Findings                 []routingAuditFinding `json:"findings"`
	Summary                  map[string]int        `json:"summary"`
	ScannedAutoReceptionists int                   `json:"scanned_auto_receptionists"`
	ScannedCallQueues        int                   `json:"scanned_call_queues"`
	ScannedUsers             int                   `json:"scanned_users"`
	ScannedCommonAreas       int                   `json:"scanned_common_areas"`
	ScannedSharedLineGroups  int                   `json:"scanned_shared_line_groups"`
	ScannedSites             int                   `json:"scanned_sites"`
	ScannedIVRs              int                   `json:"scanned_ivrs"`
	ScannedQueueMemberLists  int                   `json:"scanned_queue_member_lists"`
	MaxPages                 int                   `json:"max_pages"`
	HolidayWindowDays        int                   `json:"holiday_window_days"`
	TruncatedLists           []string              `json:"truncated_lists"`
	SkippedChecks            []routingSkippedCheck `json:"skipped_checks"`
	FetchFailures            []routingFetchFailure `json:"fetch_failures"`
	Note                     string                `json:"note,omitempty"`
}

// routingEntityInfo is one entity in the audit's cross-entity index.
type routingEntityInfo struct {
	typ              string
	id               string
	name             string
	ext              string
	status           string
	activationStatus string
	extensionID      string
}

type routingEntityIndex map[string]routingEntityInfo // keys: "id:", "extid:", "ext:"

func (ix routingEntityIndex) add(e routingEntityInfo, extensionID string) {
	e.extensionID = extensionID
	if e.id != "" {
		ix["id:"+e.id] = e
	}
	if extensionID != "" {
		ix["extid:"+extensionID] = e
	}
	if e.ext != "" {
		ix["ext:"+e.ext] = e
	}
}

// lookup resolves a routing target against the index. Stable identifiers
// are authoritative: when the target carries an id or extension_id it is
// resolved only by those, so a deleted user's extension number that now
// belongs to someone else cannot make a dead target look alive. The
// extension number is used only when the target has neither stable id.
// conflict is non-empty when the id and extension_id name different entities.
func (ix routingEntityIndex) lookup(t routingIVRTarget) (e routingEntityInfo, found bool, conflict string) {
	id, extID := strings.TrimSpace(t.ID), strings.TrimSpace(t.ExtensionID)
	if id == "" && extID == "" {
		if num := t.ExtensionNumber.String(); num != "" {
			e, found = ix["ext:"+num]
		}
		return e, found, ""
	}
	var byID, byExt routingEntityInfo
	okID, okExt := false, false
	if id != "" {
		byID, okID = ix["id:"+id]
	}
	if extID != "" {
		byExt, okExt = ix["extid:"+extID]
	}
	switch {
	case okID && extID != "" && byID.extensionID != "" && byID.extensionID != extID:
		// The entity behind the id carries a different extension id. (Common
		// area list rows have no extension_id, so a blank one is not a conflict.)
		return routingEntityInfo{}, false, fmt.Sprintf("target id %s is %s with extension_id %s, but the target names extension_id %s", id, routingFirstNonEmpty(byID.name, byID.id), byID.extensionID, extID)
	case okID:
		return byID, true, ""
	case id != "" && okExt:
		return routingEntityInfo{}, false, fmt.Sprintf("target id %s does not exist, but its extension_id %s belongs to %s (id %s)", id, extID, routingFirstNonEmpty(byExt.name, byExt.id), byExt.id)
	case okExt:
		return byExt, true, ""
	}
	return routingEntityInfo{}, false, ""
}

type routingListOutcome struct {
	items     []json.RawMessage
	truncated bool
	err       error
}

func (o routingListOutcome) complete() bool { return o.err == nil && !o.truncated }

type routingIVRFetch struct {
	ar    int
	hours string
	ivr   *routingIVR
}

func newNovelRoutingAuditCmd(flags *rootFlags) *cobra.Command {
	var flagMaxPages int
	var flagHolidayWindow string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Sweep the account for dead IVR targets, empty call queues, orphaned auto receptionists and auto receptionists with no upcoming holiday hours (live).",
		Long: strings.TrimSpace(`
Use this command for an account-wide sweep of dead IVR targets, empty call queues, orphaned auto receptionists and missing holiday hours. Do NOT use this command to follow a single phone number; use 'routing trace' instead.

Findings:
  DEAD_TARGET          an IVR key or no-input target (business or closed hours)
                       is not among the listed entities, or is a user who is not active
  EMPTY_QUEUE          a call queue with no members, or no member with receive_call=true
  ORPHAN_AR            an auto receptionist with no phone numbers that no IVR
                       targets and no site uses as its main auto receptionist
  NO_UPCOMING_HOLIDAY  an auto receptionist that answers a phone number (or is a
                       site main auto receptionist) with no holiday hours in the window

A check whose inputs were truncated by --max-pages or failed to fetch is listed
under skipped_checks instead of guessing. When some fetches fail the report is
still printed, failures are listed in fetch_failures and on stderr, and the
command exits 6 (--allow-partial-failure exits 0). Every run reads live data;
the response cache is bypassed.`),
		Example: strings.Trim(`
  zoom-phone-cli routing audit
  zoom-phone-cli routing audit --agent
  zoom-phone-cli routing audit --holiday-window 60d --max-pages 50 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "routing audit")
			}
			if flagMaxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			window, err := cliutil.ParseDurationLoose(flagHolidayWindow)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--holiday-window must be a positive duration such as 90d"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Audits must see a fix made a minute ago, never a cached response.
			c.NoCache = true

			fanoutCap := 0
			if cliutil.IsDogfoodEnv() {
				fanoutCap = routingAuditDogfoodFanout
			}
			// cmd.Context(), not boundCtx: the client already applies --timeout
			// per request, and a whole-command deadline would cap the fan-out.
			result, err := routingRunAudit(cmd.Context(), c, routingEffectiveMaxPages(flagMaxPages), window, fanoutCap, time.Now().UTC())
			if err != nil {
				return err
			}
			if n := len(result.FetchFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d fetch(es) failed; findings may be incomplete (see fetch_failures)\n", n)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), result, flags); err != nil {
					return err
				}
			} else if err := routingPrintAudit(cmd, result); err != nil {
				return err
			}
			// Some fetches failed and others succeeded: the report is printed,
			// then exit 6 unless --allow-partial-failure downgrades it.
			if n := len(result.FetchFailures); n > 0 && !flags.allowPartialFailure {
				return partialFailureErr(fmt.Errorf("routing audit: %d fetch(es) failed; findings are incomplete (see fetch_failures)", n))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", routingDefaultMaxPages, "Maximum pages to fetch per entity list")
	cmd.Flags().StringVar(&flagHolidayWindow, "holiday-window", "90d", "How far ahead NO_UPCOMING_HOLIDAY looks for holiday hours (e.g. 60d, 12w)")
	return cmd
}

func routingRunAudit(ctx context.Context, c *client.Client, maxPages int, window time.Duration, fanoutCap int, now time.Time) (routingAuditResult, error) {
	res := routingAuditResult{
		Findings:          make([]routingAuditFinding, 0),
		Summary:           map[string]int{routingCodeDeadTarget: 0, routingCodeEmptyQueue: 0, routingCodeOrphanAR: 0, routingCodeNoUpcomingHoliday: 0},
		MaxPages:          maxPages,
		HolidayWindowDays: int(window.Hours() / 24),
		TruncatedLists:    make([]string, 0),
		SkippedChecks:     make([]routingSkippedCheck, 0),
		FetchFailures:     make([]routingFetchFailure, 0),
	}
	var mu sync.Mutex
	fail := func(path string, err error) {
		mu.Lock()
		res.FetchFailures = append(res.FetchFailures, routingFetchFailure{Path: path, Error: routingErrText(err)})
		mu.Unlock()
	}

	// 1. Entity lists, bounded concurrency.
	lists := []struct {
		name, path, key string
		pageSize        int
	}{
		{"auto_receptionists", "/phone/auto_receptionists", "auto_receptionists", 100},
		{"call_queues", "/phone/call_queues", "call_queues", 100},
		{"users", "/phone/users", "users", 100},
		{"common_areas", "/phone/common_areas", "common_areas", 100},
		{"shared_line_groups", "/phone/shared_line_groups", "shared_line_groups", 300},
		{"sites", "/phone/sites", "sites", 300},
	}
	outcomes := make([]routingListOutcome, len(lists))
	// The first list walk runs alone so the client mints its access token
	// once, before any concurrent request could race the first mint.
	first, firstListErr := routingListAll(ctx, c, lists[0].path, lists[0].key, nil, lists[0].pageSize, maxPages, nil)
	outcomes[0] = routingListOutcome{items: first.Items, truncated: first.Truncated, err: firstListErr}
	sem := make(chan struct{}, routingAuditConcurrency)
	var wg sync.WaitGroup
	for i, l := range lists {
		if i == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, path, key string, pageSize int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pr, err := routingListAll(ctx, c, path, key, nil, pageSize, maxPages, nil)
			outcomes[i] = routingListOutcome{items: pr.Items, truncated: pr.Truncated, err: err}
		}(i, l.path, l.key, l.pageSize)
	}
	wg.Wait()
	allFailed := true
	var firstErr error
	for i, o := range outcomes {
		if o.err != nil {
			fail(lists[i].path, o.err)
			if firstErr == nil {
				firstErr = o.err
			}
		} else {
			allFailed = false
		}
		if o.truncated {
			res.TruncatedLists = append(res.TruncatedLists, lists[i].name)
		}
	}
	if allFailed && firstErr != nil {
		return res, classifyAPIErrorOnly(firstErr)
	}
	arOut, cqOut, userOut, caOut, slgOut, siteOut := outcomes[0], outcomes[1], outcomes[2], outcomes[3], outcomes[4], outcomes[5]

	ars := routingDecodeAll[routingAutoReceptionist](arOut.items)
	cqs := routingDecodeAll[routingCallQueue](cqOut.items)
	users := routingDecodeAll[routingUser](userOut.items)
	cas := routingDecodeAll[routingCommonArea](caOut.items)
	slgs := routingDecodeAll[routingSharedLineGroup](slgOut.items)
	sites := routingDecodeAll[routingSite](siteOut.items)
	res.ScannedAutoReceptionists, res.ScannedCallQueues, res.ScannedUsers = len(ars), len(cqs), len(users)
	res.ScannedCommonAreas, res.ScannedSharedLineGroups, res.ScannedSites = len(cas), len(slgs), len(sites)

	index := map[string]routingEntityIndex{
		routingTypeAutoReceptionist: {}, routingTypeCallQueue: {}, routingTypeUser: {},
		routingTypeCommonArea: {}, routingTypeSharedLineGroup: {},
	}
	for _, a := range ars {
		index[routingTypeAutoReceptionist].add(routingEntityInfo{typ: routingTypeAutoReceptionist, id: a.ID, name: a.Name, ext: a.ExtensionNumber.String()}, a.ExtensionID)
	}
	for _, q := range cqs {
		index[routingTypeCallQueue].add(routingEntityInfo{typ: routingTypeCallQueue, id: q.ID, name: q.Name, ext: q.ExtensionNumber.String(), status: q.Status}, q.ExtensionID)
	}
	for _, u := range users {
		index[routingTypeUser].add(routingEntityInfo{typ: routingTypeUser, id: u.ID, name: u.Name, ext: u.ExtensionNumber.String(), status: u.Status, activationStatus: u.ActivationStatus}, u.ExtensionID)
	}
	for _, ca := range cas {
		index[routingTypeCommonArea].add(routingEntityInfo{typ: routingTypeCommonArea, id: ca.ID, name: ca.DisplayName, ext: ca.ExtensionNumber.String()}, "")
	}
	for _, s := range slgs {
		index[routingTypeSharedLineGroup].add(routingEntityInfo{typ: routingTypeSharedLineGroup, id: s.ID, name: s.DisplayName, ext: s.ExtensionNumber.String(), status: s.Status}, s.ExtensionID)
	}
	listComplete := map[string]bool{
		routingTypeAutoReceptionist: arOut.complete(), routingTypeCallQueue: cqOut.complete(), routingTypeUser: userOut.complete(),
		routingTypeCommonArea: caOut.complete(), routingTypeSharedLineGroup: slgOut.complete(),
	}

	// 2. Per-entity fan-out: IVR (business + closed) and missing holiday data
	// per auto receptionist; members per call queue.
	arFan, cqFan := len(ars), len(cqs)
	if fanoutCap > 0 {
		if arFan > fanoutCap {
			arFan = fanoutCap
			res.TruncatedLists = append(res.TruncatedLists, "auto_receptionist_ivr_fanout")
		}
		if cqFan > fanoutCap {
			cqFan = fanoutCap
			res.TruncatedLists = append(res.TruncatedLists, "call_queue_member_fanout")
		}
	}
	ivrs := make([]routingIVRFetch, 0, arFan*2)
	ivrFailed := false
	memberLists := make([][]routingQueueMember, cqFan)
	memberOK := make([]bool, cqFan)
	// memberTruncated: --max-pages stopped the member walk with pages left.
	memberTruncated := make([]bool, cqFan)
	memberErr := make([]string, cqFan)
	holidayKnown := make([]bool, len(ars))
	for i := range ars {
		holidayKnown[i] = ars[i].HolidayHours != nil
	}

	var jobs []func()
	for i := 0; i < arFan; i++ {
		i := i
		for _, hours := range []string{routingHoursBusiness, routingHoursClosed} {
			hours := hours
			jobs = append(jobs, func() {
				path := "/phone/auto_receptionists/" + url.PathEscape(ars[i].ID) + "/ivr"
				raw, err := c.Get(ctx, path, map[string]string{"hours_type": hours})
				var ivr routingIVR
				if err == nil {
					if derr := json.Unmarshal(raw, &ivr); derr != nil {
						err = fmt.Errorf("decode IVR: %w", derr)
					}
				}
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					ivrFailed = true
					res.FetchFailures = append(res.FetchFailures, routingFetchFailure{Path: path + "?hours_type=" + hours, Error: routingErrText(err)})
					return
				}
				ivrs = append(ivrs, routingIVRFetch{ar: i, hours: hours, ivr: &ivr})
			})
		}
		if ars[i].HolidayHours == nil {
			jobs = append(jobs, func() {
				path := "/phone/auto_receptionists/" + url.PathEscape(ars[i].ID)
				raw, err := c.Get(ctx, path, nil)
				var detail struct {
					HolidayHours *[]routingHoliday `json:"holiday_hours"`
				}
				if err == nil {
					if derr := json.Unmarshal(raw, &detail); derr != nil {
						err = fmt.Errorf("decode auto receptionist: %w", derr)
					}
				}
				if err != nil {
					fail(path, err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if detail.HolidayHours == nil {
					empty := []routingHoliday{}
					detail.HolidayHours = &empty
				}
				ars[i].HolidayHours = detail.HolidayHours
				holidayKnown[i] = true
			})
		}
	}
	for i := 0; i < cqFan; i++ {
		i := i
		jobs = append(jobs, func() {
			path := "/phone/call_queues/" + url.PathEscape(cqs[i].ID) + "/members"
			pr, err := routingListAll(ctx, c, path, "call_queue_members", nil, 0, maxPages, nil)
			if err != nil {
				fail(path, err)
				mu.Lock()
				memberErr[i] = routingErrText(err)
				mu.Unlock()
				return
			}
			members := routingDecodeAll[routingQueueMember](pr.Items)
			mu.Lock()
			defer mu.Unlock()
			memberLists[i] = members
			memberOK[i] = true
			memberTruncated[i] = pr.Truncated
		})
	}
	for _, job := range jobs {
		wg.Add(1)
		go func(job func()) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			job()
		}(job)
	}
	wg.Wait()
	res.ScannedIVRs = len(ivrs)
	for _, ok := range memberOK {
		if ok {
			res.ScannedQueueMemberLists++
		}
	}
	sort.Slice(ivrs, func(a, b int) bool {
		if ivrs[a].ar != ivrs[b].ar {
			return ivrs[a].ar < ivrs[b].ar
		}
		return ivrs[a].hours < ivrs[b].hours
	})

	// 3. DEAD_TARGET + IVR references for ORPHAN_AR.
	referenced := map[string]bool{}
	skippedDead := map[string]bool{}
	for _, f := range ivrs {
		ar := ars[f.ar]
		type ivrTarget struct {
			key    string
			code   int
			target routingIVRTarget
			noIn   bool
		}
		targets := make([]ivrTarget, 0, len(f.ivr.KeyActions)+1)
		for _, ka := range f.ivr.KeyActions {
			targets = append(targets, ivrTarget{key: ka.Key, code: int(ka.Action), target: ka.Target})
		}
		if f.ivr.NoAction != nil {
			targets = append(targets, ivrTarget{key: "no_input", code: int(f.ivr.NoAction.Action), target: f.ivr.NoAction.ForwardTo, noIn: true})
		}
		for _, tg := range targets {
			info := routingIVRActionInfo(tg.code, tg.noIn)
			if info.skip || info.targetType == "" {
				continue
			}
			finding := routingAuditFinding{
				Code: routingCodeDeadTarget, EntityType: routingTypeAutoReceptionist, EntityID: ar.ID, EntityName: ar.Name,
				ExtensionNumber: ar.ExtensionNumber.String(), HoursType: f.hours, Key: tg.key, TargetType: info.targetType,
				TargetID: tg.target.ID, TargetName: tg.target.DisplayName, TargetExtensionNumber: tg.target.ExtensionNumber.String(),
			}
			if tg.target.ID == "" && tg.target.ExtensionID == "" && tg.target.ExtensionNumber.String() == "" {
				finding.Detail = fmt.Sprintf("IVR %s action %s has no target configured", routingKeyLabel(tg.key), info.label)
				res.Findings = append(res.Findings, finding)
				continue
			}
			if info.targetType == routingTypeAutoReceptionist {
				for _, k := range []string{tg.target.ID, tg.target.ExtensionID} {
					if k != "" && k != ar.ID && k != ar.ExtensionID {
						referenced[k] = true
					}
				}
			}
			ent, found, conflict := index[info.targetType].lookup(tg.target)
			inactive := ""
			if found && ent.typ == routingTypeUser {
				inactive = zoomUserInactiveReason(ent.status, ent.activationStatus)
			}
			switch {
			case conflict != "":
				// Sound even on truncated lists: both identifiers were found
				// (or one was found on the wrong entity), so they disagree.
				finding.Detail = fmt.Sprintf("IVR %s target identifiers disagree: %s", routingKeyLabel(tg.key), conflict)
				res.Findings = append(res.Findings, finding)
			case inactive != "":
				finding.TargetID, finding.TargetName = ent.id, routingFirstNonEmpty(ent.name, finding.TargetName)
				finding.Detail = fmt.Sprintf("IVR %s routes to user %s who is not active (%s)", routingKeyLabel(tg.key), routingFirstNonEmpty(ent.name, ent.id), inactive)
				res.Findings = append(res.Findings, finding)
			case found:
				if info.targetType == routingTypeAutoReceptionist && ent.id != ar.ID {
					referenced[ent.id] = true
				}
			case listComplete[info.targetType]:
				finding.Detail = fmt.Sprintf("IVR %s routes to a %s that does not exist in the account", routingKeyLabel(tg.key), info.targetType)
				res.Findings = append(res.Findings, finding)
			default:
				skippedDead[info.targetType] = true
			}
		}
	}
	for _, typ := range routingSortedKeys(skippedDead) {
		res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{
			Check:  routingCodeDeadTarget + " (" + typ + " targets not found)",
			Reason: "the " + typ + " list was truncated by --max-pages or failed to fetch, so a missing target cannot be told apart from an unscanned one",
		})
	}

	// 4. EMPTY_QUEUE.
	for i := 0; i < cqFan; i++ {
		if !memberOK[i] {
			if memberErr[i] != "" {
				res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{
					Check:    routingCodeEmptyQueue,
					EntityID: cqs[i].ID,
					Reason:   "the member list of call queue " + routingFirstNonEmpty(cqs[i].Name, cqs[i].ID) + " failed to fetch: " + memberErr[i],
				})
			}
			continue
		}
		q := cqs[i]
		members := memberLists[i]
		receiving := 0
		for _, m := range members {
			if m.Level == routingTypeCommonArea || m.ReceiveCall == nil || *m.ReceiveCall {
				receiving++
			}
		}
		if memberTruncated[i] {
			res.TruncatedLists = append(res.TruncatedLists, "call_queue_members:"+q.ID)
			if receiving == 0 {
				// A receiving member may sit on a page --max-pages did not reach:
				// report the gap instead of a possibly false EMPTY_QUEUE.
				res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{
					Check:    routingCodeEmptyQueue,
					EntityID: q.ID,
					Reason:   fmt.Sprintf("the member list of call queue %s stopped at --max-pages %d before a receiving member was found", routingFirstNonEmpty(q.Name, q.ID), maxPages),
				})
			}
			continue
		}
		detail := ""
		switch {
		case len(members) == 0:
			detail = "call queue has no members"
		case receiving == 0:
			detail = fmt.Sprintf("none of the %d members has receive_call=true", len(members))
		}
		if detail != "" {
			res.Findings = append(res.Findings, routingAuditFinding{
				Code: routingCodeEmptyQueue, EntityType: routingTypeCallQueue, EntityID: q.ID, EntityName: q.Name,
				ExtensionNumber: q.ExtensionNumber.String(), Detail: detail,
			})
		}
	}

	// 5. ORPHAN_AR.
	siteMain := map[string]bool{}
	for _, s := range sites {
		for _, k := range []string{s.MainAutoReceptionist.ID, s.MainAutoReceptionist.ExtensionID} {
			if k != "" {
				siteMain[k] = true
			}
		}
	}
	switch {
	case !arOut.complete():
		res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{Check: routingCodeOrphanAR, Reason: "the auto receptionist list was truncated by --max-pages or failed to fetch"})
	case !siteOut.complete():
		res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{Check: routingCodeOrphanAR, Reason: "the site list was truncated by --max-pages or failed to fetch, so main auto receptionists are unknown"})
	case ivrFailed || arFan < len(ars):
		res.SkippedChecks = append(res.SkippedChecks, routingSkippedCheck{Check: routingCodeOrphanAR, Reason: "not every auto receptionist IVR was fetched, so IVR references are incomplete"})
	default:
		for _, a := range ars {
			if len(a.PhoneNumbers) > 0 || referenced[a.ID] || (a.ExtensionID != "" && referenced[a.ExtensionID]) || siteMain[a.ID] || (a.ExtensionID != "" && siteMain[a.ExtensionID]) {
				continue
			}
			res.Findings = append(res.Findings, routingAuditFinding{
				Code: routingCodeOrphanAR, EntityType: routingTypeAutoReceptionist, EntityID: a.ID, EntityName: a.Name,
				ExtensionNumber: a.ExtensionNumber.String(),
				Detail:          "no phone numbers, not targeted by any auto receptionist IVR, and not a site main auto receptionist (call handling forwards from users or queues are not checked)",
			})
		}
	}

	// 6. NO_UPCOMING_HOLIDAY: only auto receptionists that answer inbound
	// numbers or serve as a site main auto receptionist.
	horizon := now.Add(window)
	for i, a := range ars {
		if !holidayKnown[i] || a.HolidayHours == nil {
			continue
		}
		if len(a.PhoneNumbers) == 0 && !siteMain[a.ID] && (a.ExtensionID == "" || !siteMain[a.ExtensionID]) {
			continue
		}
		upcoming := false
		for _, h := range *a.HolidayHours {
			from, ferr := time.Parse(time.RFC3339, h.From)
			to, terr := time.Parse(time.RFC3339, h.To)
			if ferr != nil || terr != nil {
				continue
			}
			if !to.Before(now) && !from.After(horizon) {
				upcoming = true
				break
			}
		}
		if !upcoming {
			res.Findings = append(res.Findings, routingAuditFinding{
				Code: routingCodeNoUpcomingHoliday, EntityType: routingTypeAutoReceptionist, EntityID: a.ID, EntityName: a.Name,
				ExtensionNumber: a.ExtensionNumber.String(),
				Detail:          fmt.Sprintf("no holiday hours between %s and %s (%d holiday entries total)", now.Format("2006-01-02"), horizon.Format("2006-01-02"), len(*a.HolidayHours)),
			})
		}
	}

	sort.SliceStable(res.Findings, func(a, b int) bool {
		fa, fb := res.Findings[a], res.Findings[b]
		if fa.Code != fb.Code {
			return fa.Code < fb.Code
		}
		if fa.EntityName != fb.EntityName {
			return fa.EntityName < fb.EntityName
		}
		if fa.HoursType != fb.HoursType {
			return fa.HoursType < fb.HoursType
		}
		return fa.Key < fb.Key
	})
	for _, f := range res.Findings {
		res.Summary[f.Code]++
	}
	sort.Slice(res.FetchFailures, func(a, b int) bool { return res.FetchFailures[a].Path < res.FetchFailures[b].Path })
	if len(res.Findings) == 0 && len(res.TruncatedLists) > 0 {
		res.Note = "no findings, but some lists hit --max-pages (" + strings.Join(res.TruncatedLists, ", ") + "); raise --max-pages for a full sweep"
	}
	return res, nil
}

func routingDecodeAll[T any](items []json.RawMessage) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		var v T
		if json.Unmarshal(it, &v) == nil {
			out = append(out, v)
		}
	}
	return out
}

func routingKeyLabel(key string) string {
	if key == "no_input" {
		return "no-input action"
	}
	return "key " + key
}

func routingFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func routingSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func routingPrintAudit(cmd *cobra.Command, r routingAuditResult) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Scanned: %d auto receptionists (%d IVRs), %d call queues, %d users, %d common areas, %d shared line groups, %d sites\n",
		r.ScannedAutoReceptionists, r.ScannedIVRs, r.ScannedCallQueues, r.ScannedUsers, r.ScannedCommonAreas, r.ScannedSharedLineGroups, r.ScannedSites)
	fmt.Fprintf(w, "Findings: %d DEAD_TARGET, %d EMPTY_QUEUE, %d ORPHAN_AR, %d NO_UPCOMING_HOLIDAY\n",
		r.Summary[routingCodeDeadTarget], r.Summary[routingCodeEmptyQueue], r.Summary[routingCodeOrphanAR], r.Summary[routingCodeNoUpcomingHoliday])
	for _, s := range r.SkippedChecks {
		fmt.Fprintf(w, "Skipped %s: %s\n", s.Check, s.Reason)
	}
	if r.Note != "" {
		fmt.Fprintln(w, "Note: "+r.Note)
	}
	if len(r.Findings) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	rows := make([]map[string]any, 0, len(r.Findings))
	for _, f := range r.Findings {
		rows = append(rows, map[string]any{
			"code":   f.Code,
			"entity": strings.TrimSpace(f.EntityName + " " + routingExtSuffix(f.ExtensionNumber)),
			"hours":  f.HoursType,
			"key":    f.Key,
			"detail": f.Detail,
		})
	}
	return printAutoTable(w, rows)
}

func routingExtSuffix(ext string) string {
	if ext == "" {
		return ""
	}
	return "(ext " + ext + ")"
}
