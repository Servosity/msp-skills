// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ninjaone-pp-cli/internal/store"
)

// Class report-row-identity: the user-supplied parameters a /v2/queries/* report walk may send
// and still be the whole report (hand-fix; its two call sites in the generated sync.go, the
// first line of syncPruneSkipReason and the refusal near the top of syncResource, are pinned in
// handfixes.json report-row-identity).
//
// One classifier, ninjaReportWalkParams, decides it for every report walk, from the same
// --param / --global-param / --resource-param values applyTo puts on the request:
//
//   - row-preserving: a parameter spec.yaml documents as paging or projection for the
//     endpoint, so the report returns the same rows in the same order with it as without it;
//   - row-narrowing: every other parameter, and any parameter the spec does not document
//     there. A filter returns part of the report; a cursor pins the walk's position (applyTo
//     overrides the walk's own cursor on every page, so the walk neither starts at page one
//     nor advances); an unknown key may be either.
//
// A walk whose user parameters are all row-preserving is complete-and-unfiltered for the
// snapshot replace (ninjaReportPruneParams). A row-narrowing walk of a report keyed by row
// ordinal is refused before any request (ninjaRefuseNarrowedOrdinalWalk): the narrowed rows
// would take the ordinals of other rows of the whole report and be stored over them.
//
// A refusal fails that resource only: the other resources of the run still sync, as with any
// per-resource error. It is reported as a sync_error and counted as a critical failure, so the
// run exits non-zero without --strict: the CLI declined the request rather than failing to
// reach NinjaOne, and a filter that was not applied must not exit 0.

// ninjaReportParam is one user-supplied parameter a report walk sends, with the flag that
// supplied the value on the request.
type ninjaReportParam struct {
	Flag     string
	Resource string
	Key      string
	Value    string
}

func (p ninjaReportParam) String() string {
	if p.Flag == "--resource-param" {
		return fmt.Sprintf("%s %s:%s=%s", p.Flag, p.Resource, p.Key, p.Value)
	}
	return fmt.Sprintf("%s %s=%s", p.Flag, p.Key, p.Value)
}

// ninjaReportProjectionParams are the projection parameters spec.yaml documents per report
// endpoint: each adds fields to every row and never selects rows.
//
//   - /v2/queries/volumes `include`: "Additional information to include (bl - BitLocker status)".
var ninjaReportProjectionParams = map[string]map[string]bool{
	"/v2/queries/volumes": {"include": true},
}

// ninjaReportRowPreserving is the allowlist: whether key=value, sent to the /v2/queries/*
// endpoint at path, leaves the report's rows and their order unchanged.
//
//   - pageSize, on every /v2/queries/* endpoint ("Limit number of records per page"): paging.
//     Only a positive integer: syncRequestedPageLimit reads the value to decide where the
//     report ends, and a value it cannot read leaves it comparing pages against a size the
//     request did not ask for.
//   - a projection parameter the endpoint documents (ninjaReportProjectionParams).
//
// cursor is documented as paging but is not here: a user value pins the walk's position.
func ninjaReportRowPreserving(path, key, value string) bool {
	if key == "pageSize" {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		return err == nil && n > 0
	}
	return ninjaReportProjectionParams[path][key]
}

// ninjaReportWalkParams classifies the user-supplied parameters one walk of resource sends,
// each key once with the value applyTo leaves on the request (--param on a flat walk, then
// --global-param, then --resource-param), sorted by key. A resource that is not a
// /v2/queries/* report has no row-preserving parameter. A declared global-scope selector
// always narrows.
func ninjaReportWalkParams(resource string, dependent bool, p *syncUserParams) (preserving, narrowing []ninjaReportParam) {
	if p == nil {
		return nil, nil
	}
	sent := map[string]ninjaReportParam{}
	if !dependent {
		for k, v := range p.flatGlobal {
			sent[k] = ninjaReportParam{Flag: "--param", Key: k, Value: v}
		}
	}
	for k, v := range p.trueGlobal {
		sent[k] = ninjaReportParam{Flag: "--global-param", Key: k, Value: v}
	}
	for k, v := range p.perResource[resource] {
		sent[k] = ninjaReportParam{Flag: "--resource-param", Resource: resource, Key: k, Value: v}
	}
	keys := make([]string, 0, len(sent))
	for k := range sent {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	path, err := syncResourcePath(resource)
	report := err == nil && strings.HasPrefix(path, "/v2/queries/")
	for _, k := range keys {
		prm := sent[k]
		if report && !syncParamIsGlobalScope(k) && ninjaReportRowPreserving(path, k, prm.Value) {
			preserving = append(preserving, prm)
		} else {
			narrowing = append(narrowing, prm)
		}
	}
	return preserving, narrowing
}

// ninjaReportPruneParams is the userParams syncPruneSkipReason judges a walk by: for a report
// snapshot partition, only the row-narrowing parameters, each kept under every flag that
// supplied it so the skip reason is the one it always was; for every other partition, and for
// a dependent walk, userParams unchanged.
func ninjaReportPruneParams(resource string, dependent bool, p *syncUserParams) *syncUserParams {
	if p == nil || dependent || !ninjaSnapshotPartition(resource) {
		return p
	}
	_, narrowing := ninjaReportWalkParams(resource, dependent, p)
	narrows := make(map[string]bool, len(narrowing))
	for _, prm := range narrowing {
		narrows[prm.Key] = true
	}
	keep := func(in map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range in {
			if narrows[k] {
				out[k] = v
			}
		}
		return out
	}
	return &syncUserParams{
		flatGlobal:  keep(p.flatGlobal),
		trueGlobal:  keep(p.trueGlobal),
		perResource: map[string]map[string]string{resource: keep(p.perResource[resource])},
	}
}

// ninjaOrdinalReportLiveCommands are the live reads that answer a filtered question about a
// report keyed by row ordinal.
var ninjaOrdinalReportLiveCommands = map[string]string{
	"queries":                  "queries get-disk-drives",
	"queries-antivirus-status": "queries get-antivirus-status-report",
	"queries-logged-on-users":  "queries get-last-logged-on-users-report",
	"queries-processors":       "queries get-processors",
	"queries-software":         "queries get-software",
	"queries-volumes":          "queries get-volumes",
	"queries-windows-services": "queries get-windows-services-report",
}

// ninjaNarrowedOrdinalWalkError is the refusal of a walk of an ordinal report that would not be
// the whole report, nil when the walk may run. sinceParam/effectiveSince is the incremental
// window the walk would send (none of these reports documents one today).
func ninjaNarrowedOrdinalWalkError(resource, sinceParam, effectiveSince string, p *syncUserParams) error {
	if !store.ReportRowsAreOrdinal(resource) {
		return nil
	}
	_, narrowing := ninjaReportWalkParams(resource, false, p)
	named := make([]string, 0, len(narrowing)+1)
	for _, prm := range narrowing {
		named = append(named, prm.String())
	}
	if strings.TrimSpace(effectiveSince) != "" {
		named = append(named, fmt.Sprintf("the since window %s=%s", sinceParam, effectiveSince))
	}
	if len(named) == 0 {
		return nil
	}
	path, _ := syncResourcePath(resource)
	allowed := []string{"pageSize"}
	for k := range ninjaReportProjectionParams[path] {
		allowed = append(allowed, k)
	}
	sort.Strings(allowed)
	return fmt.Errorf("refusing to sync %s with %s: its rows have no id of their own and are keyed by their position in the whole report, so %s syncs only whole (a narrowed or repositioned walk would store its rows over other rows' positions); nothing was requested or stored. Only %s may be set for this report; for a filtered question read it live with `ninjaone-cli %s`",
		resource, strings.Join(named, ", "), resource, strings.Join(allowed, ", "), ninjaOrdinalReportLiveCommands[resource])
}

// ninjaRefuseNarrowedOrdinalWalk is the refusal at the top of syncResource, before the walk
// binds its sync state or sends a request: ok reports that the walk was refused and res is its
// result (a critical failure; see the class comment above).
func ninjaRefuseNarrowedOrdinalWalk(resource, effectiveSince string, p *syncUserParams, syncEvents io.Writer, started time.Time) (syncResult, bool) {
	err := ninjaNarrowedOrdinalWalkError(resource, syncResourceSinceParam(resource), effectiveSince, p)
	if err == nil {
		return syncResult{}, false
	}
	if !humanFriendly {
		fmt.Fprintln(syncEvents, syncErrorJSON(resource, "", err))
	}
	return syncResult{Resource: resource, Err: err, IntegrityFailure: true, Duration: time.Since(started)}, true
}
