// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output). Pure logic behind 'respond scan',
// 'respond isolate' and 'respond release': target validation and resolution,
// the POST /v1/device_tasks request body, task-run completion and polling.
//
// Request bodies follow the ESET Connect user guide "Automation" examples:
// a manual trigger with an expireTime, targets.devicesUuids for one device or
// targets.deviceGroupsUuids for a customer's COMPANY group, and action.name
// from the guide's task table. OnDemandScan and InitiateVulnerabilityScan
// carry protobuf Any params identified by their "@type" URL.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/client"
	"github.com/spf13/cobra"
)

const (
	respondActionScan     = "OnDemandScan"
	respondActionVulnScan = "InitiateVulnerabilityScan"
	respondActionIsolate  = "StartNetworkIsolation"
	respondActionRelease  = "EndNetworkIsolation"

	respondScanParamsType = "type.googleapis.com/Era.Common.DataDefinition.Task.ESS.OnDemandScan"
	respondVulnParamsType = "type.googleapis.com/eset.dotnod.vulnerability_management.v1.ScanRequest"
	respondAllTargets     = "eset://AllTargets"

	// respondTriggerTTL is how long the manual trigger stays valid: a device
	// that comes online after this does not run a stale task.
	respondTriggerTTL   = 24 * time.Hour
	respondRunsMaxPages = 5
	respondRunStatusPfx = "TASK_RUN_STATUS_"
)

// respondPollInterval is how often --wait re-reads the task runs. A var so
// tests can shorten it.
var respondPollInterval = 10 * time.Second

var respondUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// respondLooksLikeUUID reports whether s has the RFC 4122 text shape ESET
// uses for device and group identifiers.
func respondLooksLikeUUID(s string) bool {
	return respondUUIDPattern.MatchString(strings.TrimSpace(s))
}

// respondRef names one task target. Unresolved is set only for a harness
// placeholder (a name that could not be resolved under verify or dogfood);
// such a target never reaches the API because the harness refuses --yes.
type respondRef struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	Unresolved bool   `json:"unresolved,omitempty"`
}

type respondTargets struct {
	Devices []respondRef `json:"devices"`
	Groups  []respondRef `json:"groups"`
}

func respondNewTargets() respondTargets {
	return respondTargets{Devices: make([]respondRef, 0), Groups: make([]respondRef, 0)}
}

// label is the human name used in the task displayName and messages.
func (t respondTargets) label() string {
	names := make([]string, 0, len(t.Devices)+len(t.Groups))
	for _, r := range append(append([]respondRef{}, t.Devices...), t.Groups...) {
		if r.Name != "" {
			names = append(names, r.Name)
		} else {
			names = append(names, r.UUID)
		}
	}
	return strings.Join(names, ", ")
}

// respondScanOptions are the OnDemandScan settings exposed as flags.
type respondScanOptions struct {
	Profile string // InDepth or Smart
	Clean   bool
}

// Wire shapes for POST /v1/device_tasks. Field order mirrors the guide.
type respondTaskBody struct {
	Task respondTask `json:"task"`
}

type respondTask struct {
	DisplayName string             `json:"displayName"`
	Description string             `json:"description"`
	Targets     respondTaskTargets `json:"targets"`
	Triggers    []respondTrigger   `json:"triggers"`
	Action      respondTaskAction  `json:"action"`
}

type respondTaskTargets struct {
	DevicesUuids      []string `json:"devicesUuids,omitempty"`
	DeviceGroupsUuids []string `json:"deviceGroupsUuids,omitempty"`
}

type respondTrigger struct {
	Manual respondManualTrigger `json:"manual"`
}

type respondManualTrigger struct {
	ExpireTime string `json:"expireTime"`
}

type respondTaskAction struct {
	Name   string `json:"name"`
	Params any    `json:"params,omitempty"`
}

type respondScanParams struct {
	Type              string   `json:"@type"`
	ScanProfile       string   `json:"scanProfile"`
	CustomProfileName string   `json:"customProfileName"`
	ScanTargets       []string `json:"scanTargets"`
	CleaningEnabled   bool     `json:"cleaningEnabled"`
}

type respondVulnParams struct {
	Type string `json:"@type"`
}

// respondPlaceholderUUID is what a harness placeholder target sends in the
// previewed body: visibly not a UUID, so nobody mistakes it for a real one.
func respondPlaceholderUUID(r respondRef) string {
	if r.Unresolved {
		return "<unresolved:" + r.Name + ">"
	}
	return r.UUID
}

// respondBuildBody builds the exact create-task request for one action.
// verb is the CLI subcommand (scan, isolate, release) for the description.
func respondBuildBody(action, verb string, scan respondScanOptions, targets respondTargets, now time.Time) respondTaskBody {
	tt := respondTaskTargets{}
	for _, d := range targets.Devices {
		tt.DevicesUuids = append(tt.DevicesUuids, respondPlaceholderUUID(d))
	}
	for _, g := range targets.Groups {
		tt.DeviceGroupsUuids = append(tt.DeviceGroupsUuids, respondPlaceholderUUID(g))
	}
	act := respondTaskAction{Name: action}
	switch action {
	case respondActionScan:
		profile := scan.Profile
		if profile == "" {
			profile = "InDepth"
		}
		act.Params = respondScanParams{
			Type:              respondScanParamsType,
			ScanProfile:       profile,
			CustomProfileName: "",
			ScanTargets:       []string{respondAllTargets},
			CleaningEnabled:   scan.Clean,
		}
	case respondActionVulnScan:
		act.Params = respondVulnParams{Type: respondVulnParamsType}
	}
	label := targets.label()
	return respondTaskBody{Task: respondTask{
		DisplayName: fmt.Sprintf("%s: %s %s", esetCLIName, action, label),
		Description: fmt.Sprintf("Created by %s (respond %s): %s on %s", esetCLIName, verb, action, label),
		Targets:     tt,
		Triggers:    []respondTrigger{{Manual: respondManualTrigger{ExpireTime: now.Add(respondTriggerTTL).UTC().Format(time.RFC3339)}}},
		Action:      act,
	}}
}

// respondNormalizeProfile accepts InDepth or Smart in any case.
func respondNormalizeProfile(p string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "indepth", "in-depth":
		return "InDepth", nil
	case "smart":
		return "Smart", nil
	}
	return "", fmt.Errorf("invalid --scan-profile %q: use InDepth or Smart", p)
}

// respondValidateTargets enforces the target rules for each verb and returns
// the device argument (empty when scanning a customer).
func respondValidateTargets(verb string, args []string, customer string) (string, error) {
	customer = strings.TrimSpace(customer)
	if len(args) > 1 {
		return "", fmt.Errorf("respond %s takes one device; got %d (%s)", verb, len(args), strings.Join(args, ", "))
	}
	device := ""
	if len(args) == 1 {
		device = strings.TrimSpace(args[0])
	}
	if verb != "scan" {
		if customer != "" {
			return "", fmt.Errorf("respond %s works on one device at a time and does not accept --customer: network isolation of a whole customer would cut every machine off at once; pass a single device name or UUID", verb)
		}
		if device == "" {
			return "", fmt.Errorf("respond %s needs a device name or UUID", verb)
		}
		return device, nil
	}
	switch {
	case device != "" && customer != "":
		return "", fmt.Errorf("pass either a device or --customer, not both")
	case device == "" && customer == "":
		return "", fmt.Errorf("pass a device name or UUID, or --customer <name|uuid> to scan a whole customer")
	}
	return device, nil
}

// respondMirror is the slice of the local store respond needs. present is
// false when the mirror file does not exist.
type respondMirror struct {
	present bool
	path    string
	devices []esetDevice
	groups  []esetGroup
}

// resolveDevice turns a name or UUID into a target. A UUID is used as is
// (the mirror only adds its name). A name needs the mirror; when it cannot
// be resolved and waive is true (verify or dogfood harness), a marked
// placeholder is returned so the preview still renders.
func (m respondMirror) resolveDevice(q string, waive bool) (respondRef, error) {
	q = strings.TrimSpace(q)
	if respondLooksLikeUUID(q) {
		ref := respondRef{UUID: q}
		for _, d := range m.devices {
			if strings.EqualFold(d.UUID, q) {
				ref.Name = d.name()
				break
			}
		}
		return ref, nil
	}
	if !m.present || len(m.devices) == 0 {
		// The learn hook can create an empty store file, so an empty devices
		// table is treated exactly like a missing mirror.
		if waive {
			return respondRef{Name: q, Unresolved: true}, nil
		}
		return respondRef{}, fmt.Errorf("cannot resolve device name %q: no synced devices in the local mirror at %s; pass the device UUID, or run '%s sync --resources devices' first", q, m.path, esetCLIName)
	}
	d, err := resolveEsetDevice(m.devices, q)
	if err != nil {
		if waive {
			return respondRef{Name: q, Unresolved: true}, nil
		}
		return respondRef{}, err
	}
	return respondRef{UUID: d.UUID, Name: d.name()}, nil
}

// resolveCustomer turns a customer name or COMPANY group UUID into a group
// target. A UUID missing from the synced tree (stale mirror) is used as is.
func (m respondMirror) resolveCustomer(q string, waive bool) (respondRef, error) {
	q = strings.TrimSpace(q)
	tree := newEsetTree(m.groups)
	if respondLooksLikeUUID(q) {
		if g, ok := tree.groups[q]; ok {
			if g.EntityType != esetCompanyEntityType {
				return respondRef{}, fmt.Errorf("group %s (%s) is not a customer (COMPANY) group", q, esetFlatten(g.Name))
			}
			return respondRef{UUID: g.UUID, Name: g.Name}, nil
		}
		return respondRef{UUID: q}, nil
	}
	if !m.present || len(m.groups) == 0 {
		if waive {
			return respondRef{Name: q, Unresolved: true}, nil
		}
		return respondRef{}, fmt.Errorf("cannot resolve customer %q: no synced device-group tree at %s; pass the customer's COMPANY group UUID, or run '%s sync --resources device-groups' first", q, m.path, esetCLIName)
	}
	g, err := tree.resolveCustomer(q)
	if err != nil {
		if waive {
			return respondRef{Name: q, Unresolved: true}, nil
		}
		return respondRef{}, err
	}
	return respondRef{UUID: g.UUID, Name: g.Name}, nil
}

// customerDeviceUUIDs lists mirrored devices owned by a COMPANY group: the
// runs a customer-wide scan must produce before it can be called complete.
func (m respondMirror) customerDeviceUUIDs(groupUUID string) []string {
	if groupUUID == "" || len(m.groups) == 0 {
		return nil
	}
	tree := newEsetTree(m.groups)
	var out []string
	for _, d := range m.devices {
		if tree.customerOf(d.ParentGroupUUID).UUID == groupUUID {
			out = append(out, d.UUID)
		}
	}
	return out
}

// deviceName returns the mirrored name of a device, or "".
func (m respondMirror) deviceName(uuid string) string {
	for _, d := range m.devices {
		if d.UUID == uuid {
			return d.name()
		}
	}
	return ""
}

// respondParseCreated extracts the task uuid from a create-task response.
// An empty body or a body without a uuid is an error, never a success.
func respondParseCreated(resp json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(resp))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return "", fmt.Errorf("ESET Connect returned an empty response to the task create, so there is no task uuid to follow; check '%s device-tasks list' before retrying", esetCLIName)
	}
	var wire struct {
		Task struct {
			UUID string `json:"uuid"`
		} `json:"task"`
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(resp, &wire); err != nil {
		return "", fmt.Errorf("decoding the ESET Connect task create response: %w", err)
	}
	if wire.Task.UUID != "" {
		return wire.Task.UUID, nil
	}
	if wire.UUID != "" {
		return wire.UUID, nil
	}
	return "", fmt.Errorf("ESET Connect answered the task create without a task uuid; check '%s device-tasks list' before retrying", esetCLIName)
}

// respondCreateError rewrites a failed task create. ESET Connect is known to
// answer some creates with HTTP 500 and an empty body (documented by the
// Fenrindale eset-protect-mcp README); that must read as an ESET failure
// with an unknown outcome, never as success and never as a blind retry.
func respondCreateError(err error) error {
	var ae *client.APIError
	if !errors.As(err, &ae) || ae.StatusCode < 500 {
		return err
	}
	if strings.TrimSpace(ae.Body) == "" {
		return fmt.Errorf("%w\nESET Connect failed the task create with HTTP %d and an empty body (a known ESET-side fault on POST /v1/device_tasks); the task may or may not exist, so check '%s device-tasks list' before retrying", err, ae.StatusCode, esetCLIName)
	}
	return fmt.Errorf("%w\nESET Connect failed the task create with HTTP %d; check '%s device-tasks list' before retrying, the task may exist", err, ae.StatusCode, esetCLIName)
}

// respondRun is one task run as respond reports it.
type respondRun struct {
	DeviceUUID string          `json:"device_uuid"`
	DeviceName string          `json:"device_name,omitempty"`
	Status     string          `json:"status"`
	StartTime  string          `json:"start_time"`
	EndTime    string          `json:"end_time"`
	Result     json.RawMessage `json:"result"`
}

type respondRunWire struct {
	DeviceUUID string          `json:"deviceUuid"`
	Status     string          `json:"status"`
	StartTime  string          `json:"startTime"`
	EndTime    string          `json:"endTime"`
	Result     json.RawMessage `json:"result"`
}

// respondDecodeRuns converts raw taskRuns items; status loses its
// TASK_RUN_STATUS_ prefix (FINISHED, FAILED, ...). A missing status is
// UNSPECIFIED, which is not terminal. It also returns how many items could
// not be decoded, since a dropped run must block a completion claim.
func respondDecodeRuns(items []json.RawMessage) ([]respondRun, int) {
	out := make([]respondRun, 0, len(items))
	rejected := 0
	for _, raw := range items {
		var w respondRunWire
		if !esetIsObject(raw) || json.Unmarshal(raw, &w) != nil {
			rejected++
			continue
		}
		status := shortEnum(w.Status, respondRunStatusPfx)
		if status == "" {
			status = "UNSPECIFIED"
		}
		res := w.Result
		if len(res) == 0 || string(res) == "null" {
			res = nil
		}
		out = append(out, respondRun{DeviceUUID: w.DeviceUUID, Status: status, StartTime: w.StartTime, EndTime: w.EndTime, Result: res})
	}
	return out, rejected
}

// respondRunTerminal reports whether a run status is final.
func respondRunTerminal(status string) bool {
	switch shortEnum(strings.ToUpper(strings.TrimSpace(status)), respondRunStatusPfx) {
	case "FINISHED", "FAILED", "SUPPRESSED":
		return true
	}
	return false
}

// respondExpectation says which runs must exist before a task is complete:
// every listed device, and at least MinRuns distinct devices. Unknown marks a
// target whose membership could not be established (a customer scan with no
// mirrored devices); such a task is never reported complete.
type respondExpectation struct {
	DeviceUUIDs []string
	MinRuns     int
	Unknown     bool
}

// respondRunsComplete is true when at least one run exists, every run is
// terminal, and the expectation is met. Zero runs is never complete: runs
// appear only when a device picks the task up.
func respondRunsComplete(runs []respondRun, want respondExpectation) bool {
	if len(runs) == 0 || want.Unknown {
		return false
	}
	seen := make(map[string]bool, len(runs))
	for _, r := range runs {
		if !respondRunTerminal(r.Status) {
			return false
		}
		seen[strings.ToLower(r.DeviceUUID)] = true
	}
	for _, d := range want.DeviceUUIDs {
		if !seen[strings.ToLower(d)] {
			return false
		}
	}
	return len(seen) >= want.MinRuns
}

// respondFetchRuns lists the latest run per device of one task. Tests swap
// it for a fake; production reads GET /v1/device_tasks/{taskUuid}/runs.
var respondFetchRuns = func(ctx context.Context, c *client.Client, taskUUID string) ([]respondRun, error) {
	u := esetHost("automation", "/v1/device_tasks/"+url.PathEscape(taskUUID)+"/runs")
	res, err := esetFetchAll(ctx, c, u, map[string]string{"listOnlyLastRuns": "true"}, "taskRuns", respondRunsMaxPages)
	if err != nil {
		return nil, err
	}
	runs, rejected := respondDecodeRuns(res.Items)
	switch {
	case res.Truncated:
		return runs, fmt.Errorf("the task has more runs than %d pages; completion cannot be confirmed (check 'device-tasks runs %s')", respondRunsMaxPages, taskUUID)
	case rejected > 0:
		return runs, fmt.Errorf("%d task run(s) could not be decoded; completion cannot be confirmed", rejected)
	}
	return runs, nil
}

// respondPollResult is the outcome of watching a task's runs.
type respondPollResult struct {
	Runs      []respondRun
	Complete  bool
	Waited    time.Duration
	LastError error // set when the final fetch failed
}

// respondPollRuns fetches runs until they are complete, wait elapses or ctx
// is cancelled. Each fetch gets its own reqCtx bound (the root --timeout is
// per request, while --wait can be much longer). A failed fetch does not end
// the watch: the task already exists, so the loop keeps trying until the
// deadline and reports the last error.
func respondPollRuns(ctx context.Context, c *client.Client, taskUUID string, want respondExpectation, wait, interval time.Duration, reqCtx func(context.Context) (context.Context, context.CancelFunc)) respondPollResult {
	start := time.Now()
	deadline := start.Add(wait)
	out := respondPollResult{Runs: make([]respondRun, 0)}
	if reqCtx == nil {
		reqCtx = func(p context.Context) (context.Context, context.CancelFunc) { return p, func() {} }
	}
	// Every fetch ends by the --wait deadline as well as its own --timeout.
	waitCtx, waitCancel := context.WithDeadline(ctx, deadline)
	defer waitCancel()
	for first := true; ; first = false {
		if !first && !time.Now().Before(deadline) {
			out.Waited = time.Since(start)
			return out
		}
		rctx, cancel := reqCtx(waitCtx)
		runs, err := respondFetchRuns(rctx, c, taskUUID)
		cancel()
		if runs != nil {
			out.Runs = runs
		}
		if err != nil {
			out.LastError = err
		} else {
			out.LastError = nil
			if respondRunsComplete(out.Runs, want) {
				out.Complete = true
				out.Waited = time.Since(start)
				return out
			}
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || ctx.Err() != nil {
			out.Waited = time.Since(start)
			if out.LastError == nil && ctx.Err() != nil {
				out.LastError = ctx.Err()
			}
			return out
		}
		sleep := interval
		if remaining < sleep {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			out.Waited = time.Since(start)
			out.LastError = ctx.Err()
			return out
		case <-timer.C:
		}
	}
}

// respondMembersMaxPages bounds the live membership walk (pageSize 1000).
const respondMembersMaxPages = 100

// respondLiveMembers lists every device ESET puts under groupUUID, subgroups
// included, as the expectation for a customer task's runs (handfixes.json:
// respond-wait-needs-complete-membership). A failed or truncated listing
// leaves completion unknown: --wait then never reports the task complete.
func respondLiveMembers(cmd *cobra.Command, c *client.Client, groupUUID string, flags *rootFlags) respondExpectation {
	unknown := respondExpectation{Unknown: true}
	if groupUUID == "" {
		return unknown
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	u := esetHost("device-management", "/v1/device_groups/"+url.PathEscape(groupUUID)+"/devices")
	res, err := esetFetchAll(ctx, c, u, map[string]string{"recurseSubgroups": "true"}, "devices", respondMembersMaxPages)
	if err == nil && res.Truncated {
		err = fmt.Errorf("more than %d pages of devices", respondMembersMaxPages)
	}
	if err != nil {
		esetFprintf(cmd.ErrOrStderr(), "warning: could not list the customer's current devices (%v); completion stays unknown\n", err)
		return unknown
	}
	want := respondExpectation{}
	for _, raw := range res.Items {
		var d struct {
			UUID string `json:"uuid"`
		}
		if json.Unmarshal(raw, &d) == nil && d.UUID != "" {
			want.DeviceUUIDs = append(want.DeviceUUIDs, d.UUID)
		}
	}
	want.MinRuns = len(want.DeviceUUIDs)
	want.Unknown = len(want.DeviceUUIDs) == 0
	return want
}
