// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. `incidents context`: one incident with its comments and every
// linked detection and device, in one call. The incident itself is required;
// the linked records are fetched in parallel and each failure is reported in
// fetch_failures instead of failing the whole command. A record or comment
// that comes back but cannot be decoded is counted in rejected_rows. A
// detection that the v1 endpoint does not know (HTTP 404) is retried on v2,
// which also serves ESET Inspect and Cloud Office detections; api_version
// records which answered.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if incidents, _, err := root.Find([]string{"incidents"}); err == nil && incidents != root {
			addNovelCommandIfAbsent(incidents, newNovelIncidentsContextCmd(flags))
		}
	})
}

const incidentsContextConcurrency = 4

// incidentsContextGetter issues one GET; tests substitute a fake.
type incidentsContextGetter func(ctx context.Context, url string) (json.RawMessage, error)

type incidentsContextWire struct {
	UUID             string   `json:"uuid"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description"`
	Status           string   `json:"status"`
	Severity         string   `json:"severity"`
	AssigneeUUID     string   `json:"assigneeUuid"`
	CreateTime       string   `json:"createTime"`
	UpdateTime       string   `json:"updateTime"`
	TriageDuration   string   `json:"triageDuration"`
	ResponseDuration string   `json:"responseDuration"`
	Tags             []string `json:"tags"`
	DeviceUUIDs      []string `json:"deviceUuids"`
	DetectionUUIDs   []string `json:"detectionUuids"`
}

type incidentsContextIncident struct {
	UUID             string   `json:"uuid"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Status           string   `json:"status"`
	Severity         string   `json:"severity"`
	AssigneeUUID     string   `json:"assignee_uuid"`
	Created          string   `json:"created"`
	Updated          string   `json:"updated"`
	TriageDuration   string   `json:"triage_duration"`
	ResponseDuration string   `json:"response_duration"`
	Tags             []string `json:"tags"`
	DetectionCount   int      `json:"detection_count"`
	DeviceCount      int      `json:"device_count"`
}

type incidentsContextComment struct {
	UUID       string `json:"uuid"`
	AuthorUUID string `json:"author_uuid"`
	Text       string `json:"text"`
	Created    string `json:"created"`
	Updated    string `json:"updated"`
	Final      bool   `json:"final"`
}

type incidentsContextDetection struct {
	UUID       string `json:"uuid"`
	APIVersion string `json:"api_version"`
	Severity   string `json:"severity"`
	Name       string `json:"name"`
	Object     string `json:"object"`
	SHA1       string `json:"sha1"`
	OccurredAt string `json:"occurred_at"`
	DeviceUUID string `json:"device_uuid"`
}

type incidentsContextDevice struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	Customer            string `json:"customer"`
	FunctionalityStatus string `json:"functionality_status"`
	LastSeen            string `json:"last_seen"`
}

type incidentsContextTruncated struct {
	Detections bool `json:"detections"`
	Devices    bool `json:"devices"`
}

type incidentsContextFailure struct {
	Part  string `json:"part"`
	Error string `json:"error"`
}

type incidentsContextView struct {
	Incident      incidentsContextIncident    `json:"incident"`
	Comments      []incidentsContextComment   `json:"comments"` // null when the comments fetch failed or its payload did not decode
	Detections    []incidentsContextDetection `json:"detections"`
	Devices       []incidentsContextDevice    `json:"devices"`
	Truncated     incidentsContextTruncated   `json:"truncated"`
	FetchFailures []incidentsContextFailure   `json:"fetch_failures"`
	RejectedRows  int                         `json:"rejected_rows"`
	// UnattributedDevices counts linked devices whose device group is not in
	// the synced tree, so their customer is unknown (shown as "(group not in
	// synced tree)", not "(unassigned)"); 0 when no tree is synced, where the
	// customer is blank (handfixes.json: customer-views-count-unattributed-rows).
	UnattributedDevices     int      `json:"unattributed_devices"`
	UnattributedDeviceUUIDs []string `json:"unattributed_device_uuids,omitempty"`
	// SyncedHint is set when the device-group mirror that names each device's
	// customer is missing or has no completed sync.
	SyncedHint string `json:"synced_hint,omitempty"`
	// parentNotVisible names the synced groups whose parent this API user
	// cannot see (esetTree.parentNotVisible), for the unattributed line.
	parentNotVisible []string
	// ParentNotVisible is the same set for agents: each synced group (uuid
	// and name) whose parent this API user cannot see. Rows under them stay
	// unattributed after any sync (handfixes.json:
	// unattributed-cause-names-hidden-parent).
	ParentNotVisible []esetGroupRef `json:"parent_not_visible,omitempty"`
}

type incidentsContextJob struct {
	Kind string // comments | detection | device
	ID   string
}

func (j incidentsContextJob) name() string {
	if j.Kind == "comments" {
		return "comments"
	}
	return j.Kind + ":" + j.ID
}

type incidentsContextFetched struct {
	Job        incidentsContextJob
	Raw        json.RawMessage
	APIVersion string
}

// incidentsContextIsNotFound reports an HTTP 404 from the generated client.
func incidentsContextIsNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// incidentsContextFetchDetection reads a detection from v1 and, only when v1
// answers 404, from v2. Any other v1 error is returned as-is.
func incidentsContextFetchDetection(ctx context.Context, get incidentsContextGetter, id string) (json.RawMessage, string, error) {
	raw, err := get(ctx, replacePathParam(esetHost("incident-management", "/v1/detections/{detectionUuid}"), "detectionUuid", id))
	if err == nil {
		return raw, "v1", nil
	}
	if !incidentsContextIsNotFound(err) {
		return nil, "", err
	}
	raw, err = get(ctx, replacePathParam(esetHost("incident-management", "/v2/detections/{detectionUuid}"), "detectionUuid", id))
	if err != nil {
		return nil, "", fmt.Errorf("not found on v1 detections; v2 lookup failed: %w", err)
	}
	return raw, "v2", nil
}

// incidentsContextParseIncident reads GET /v2/incidents/{uuid} ({"incident": {...}}).
func incidentsContextParseIncident(raw json.RawMessage) (incidentsContextWire, error) {
	var env struct {
		Incident json.RawMessage `json:"incident"`
	}
	body := raw
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Incident) > 0 {
		body = env.Incident
	}
	var inc incidentsContextWire
	if !esetIsObject(body) {
		return inc, errors.New("decoding incident: not a JSON object")
	}
	if err := json.Unmarshal(body, &inc); err != nil {
		return inc, fmt.Errorf("decoding incident: %w", err)
	}
	if inc.UUID == "" {
		return inc, fmt.Errorf("incident response carried no uuid")
	}
	return inc, nil
}

// incidentsContextDedupe drops blanks and repeats, keeping first-seen order.
func incidentsContextDedupe(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// incidentsContextPlan lists the linked fetches, capped per kind.
func incidentsContextPlan(inc incidentsContextWire, maxDetections, maxDevices int) ([]incidentsContextJob, incidentsContextTruncated) {
	jobs := []incidentsContextJob{{Kind: "comments"}}
	var tr incidentsContextTruncated
	dets := incidentsContextDedupe(inc.DetectionUUIDs)
	if len(dets) > maxDetections {
		dets, tr.Detections = dets[:maxDetections], true
	}
	for _, id := range dets {
		jobs = append(jobs, incidentsContextJob{Kind: "detection", ID: id})
	}
	devs := incidentsContextDedupe(inc.DeviceUUIDs)
	if len(devs) > maxDevices {
		devs, tr.Devices = devs[:maxDevices], true
	}
	for _, id := range devs {
		jobs = append(jobs, incidentsContextJob{Kind: "device", ID: id})
	}
	return jobs, tr
}

// incidentsContextGather runs every planned fetch with bounded concurrency.
func incidentsContextGather(ctx context.Context, get incidentsContextGetter, incidentUUID string, jobs []incidentsContextJob) ([]cliutil.FanoutResult[incidentsContextFetched], []cliutil.FanoutError) {
	return cliutil.FanoutRun(ctx, jobs, incidentsContextJob.name,
		func(ctx context.Context, j incidentsContextJob) (incidentsContextFetched, error) {
			out := incidentsContextFetched{Job: j}
			var err error
			switch j.Kind {
			case "comments":
				out.Raw, err = get(ctx, replacePathParam(esetHost("incident-management", "/v2/incidents/{incidentUuid}/comments"), "incidentUuid", incidentUUID))
			case "detection":
				out.Raw, out.APIVersion, err = incidentsContextFetchDetection(ctx, get, j.ID)
			case "device":
				out.Raw, err = get(ctx, replacePathParam(esetHost("device-management", "/v1/devices/{deviceUuid}"), "deviceUuid", j.ID))
			default:
				err = fmt.Errorf("unknown fetch kind %q", j.Kind)
			}
			return out, err
		}, cliutil.WithConcurrency(incidentsContextConcurrency))
}

// incidentsContextParseComments decodes the comments list row by row: a
// comment that does not decode is counted in rej and the rest are kept. It
// fails only when the payload itself is not a comments list.
func incidentsContextParseComments(raw json.RawMessage, rej *esetRejects) ([]incidentsContextComment, error) {
	if rej == nil {
		rej = &esetRejects{}
	}
	var env struct {
		Comments []json.RawMessage `json:"comments"`
	}
	if !esetIsObject(raw) {
		return nil, errors.New("decoding comments: not a JSON object")
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decoding comments: %w", err)
	}
	out := make([]incidentsContextComment, 0, len(env.Comments))
	for i, r := range env.Comments {
		var c struct {
			UUID           string `json:"uuid"`
			AuthorUUID     string `json:"authorUuid"`
			Text           string `json:"text"`
			CreateTime     string `json:"createTime"`
			UpdateTime     string `json:"updateTime"`
			IsFinalComment bool   `json:"isFinalComment"`
		}
		if !esetIsObject(r) {
			rej.add(fmt.Sprintf("comment row %d", i), errors.New("not a JSON object"))
			continue
		}
		if err := json.Unmarshal(r, &c); err != nil {
			rej.add(fmt.Sprintf("comment row %d", i), err)
			continue
		}
		out = append(out, incidentsContextComment{
			UUID: c.UUID, AuthorUUID: c.AuthorUUID, Text: c.Text,
			Created: c.CreateTime, Updated: c.UpdateTime, Final: c.IsFinalComment,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return devicesProfileTime(out[i].Created).Before(devicesProfileTime(out[j].Created))
	})
	return out, nil
}

// incidentsContextParseDetection reads a v1 ({"detection": {..., context.deviceUuid}})
// or v2 ({"detection": {..., device.uuid}}) detection.
func incidentsContextParseDetection(raw json.RawMessage, apiVersion, requestedID string) (incidentsContextDetection, error) {
	var env struct {
		Detection json.RawMessage `json:"detection"`
	}
	body := raw
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Detection) > 0 {
		body = env.Detection
	}
	if !esetIsObject(body) {
		return incidentsContextDetection{}, errors.New("decoding detection: not a JSON object")
	}
	var d struct {
		UUID           string `json:"uuid"`
		DisplayName    string `json:"displayName"`
		ObjectName     string `json:"objectName"`
		ObjectHashSha1 string `json:"objectHashSha1"`
		OccurTime      string `json:"occurTime"`
		SeverityLevel  string `json:"severityLevel"`
		Context        struct {
			DeviceUUID string `json:"deviceUuid"`
		} `json:"context"`
		Device struct {
			UUID string `json:"uuid"`
		} `json:"device"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return incidentsContextDetection{}, fmt.Errorf("decoding detection: %w", err)
	}
	out := incidentsContextDetection{
		UUID: d.UUID, APIVersion: apiVersion, Severity: reportVulnsSeverity(d.SeverityLevel),
		Name: d.DisplayName, Object: d.ObjectName, SHA1: d.ObjectHashSha1, OccurredAt: d.OccurTime,
		DeviceUUID: d.Context.DeviceUUID,
	}
	if out.UUID == "" {
		out.UUID = requestedID
	}
	if out.DeviceUUID == "" {
		out.DeviceUUID = d.Device.UUID
	}
	return out, nil
}

// incidentsContextBuild shapes the incident and the gathered linked records.
// Failed fetches are excluded from the lists and listed in fetch_failures; a
// fetched record (or comment) that does not decode is excluded and counted in
// rej and rejected_rows.
func incidentsContextBuild(inc incidentsContextWire, truncated incidentsContextTruncated, results []cliutil.FanoutResult[incidentsContextFetched], errs []cliutil.FanoutError, groups []esetGroup, rej *esetRejects) incidentsContextView {
	if rej == nil {
		rej = &esetRejects{}
	}
	start := rej.N
	view := incidentsContextView{
		Incident: incidentsContextIncident{
			UUID: inc.UUID, Name: inc.DisplayName, Description: inc.Description,
			Status:           shortEnum(inc.Status, "INCIDENT_STATUS_"),
			Severity:         shortEnum(inc.Severity, "INCIDENT_SEVERITY_LEVEL_"),
			AssigneeUUID:     inc.AssigneeUUID,
			Created:          inc.CreateTime,
			Updated:          inc.UpdateTime,
			TriageDuration:   inc.TriageDuration,
			ResponseDuration: inc.ResponseDuration,
			Tags:             make([]string, 0, len(inc.Tags)),
			DetectionCount:   len(incidentsContextDedupe(inc.DetectionUUIDs)),
			DeviceCount:      len(incidentsContextDedupe(inc.DeviceUUIDs)),
		},
		Detections:    make([]incidentsContextDetection, 0),
		Devices:       make([]incidentsContextDevice, 0),
		Truncated:     truncated,
		FetchFailures: make([]incidentsContextFailure, 0),
	}
	view.Incident.Tags = append(view.Incident.Tags, inc.Tags...)
	var tree *esetTree
	if len(groups) > 0 {
		tree = newEsetTree(groups)
		view.parentNotVisible = tree.parentNotVisible()
		view.ParentNotVisible = tree.parentNotVisibleGroups()
	}
	label := detectionsSpreadCustomerLabels(tree)
	var un esetUnattributed
	fail := func(part string, err error) {
		view.FetchFailures = append(view.FetchFailures, incidentsContextFailure{Part: part, Error: err.Error()})
	}
	for _, e := range errs {
		fail(e.Source, e.Err)
	}
	for _, r := range results {
		f := r.Value
		switch f.Job.Kind {
		case "comments":
			comments, err := incidentsContextParseComments(f.Raw, rej)
			if err != nil {
				rej.add("comments payload", err)
				continue
			}
			view.Comments = comments
		case "detection":
			d, err := incidentsContextParseDetection(f.Raw, f.APIVersion, f.Job.ID)
			if err != nil {
				rej.add("detection row "+f.Job.ID, err)
				continue
			}
			view.Detections = append(view.Detections, d)
		case "device":
			d, err := devicesProfileParseDevice(f.Raw)
			if err != nil {
				rej.add("device row "+f.Job.ID, err)
				continue
			}
			dv := incidentsContextDevice{
				UUID: d.UUID, Name: d.name(),
				FunctionalityStatus: shortEnum(d.FunctionalityStatus, "DEVICE_FUNCTIONALITY_STATUS_"),
				LastSeen:            d.LastSyncTime,
			}
			if tree != nil {
				owner, known := tree.ownerOf(d.ParentGroupUUID)
				dv.Customer = label(owner)
				if !known {
					un.add(d.UUID)
				}
			}
			view.Devices = append(view.Devices, dv)
		}
	}
	sort.SliceStable(view.Detections, func(i, j int) bool {
		ti, tj := devicesProfileTime(view.Detections[i].OccurredAt), devicesProfileTime(view.Detections[j].OccurredAt)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return view.Detections[i].UUID < view.Detections[j].UUID
	})
	sort.SliceStable(view.Devices, func(i, j int) bool {
		return strings.ToLower(view.Devices[i].Name) < strings.ToLower(view.Devices[j].Name)
	})
	sort.SliceStable(view.FetchFailures, func(i, j int) bool { return view.FetchFailures[i].Part < view.FetchFailures[j].Part })
	view.RejectedRows = rej.N - start
	view.UnattributedDevices, view.UnattributedDeviceUUIDs = un.N, un.UUIDs()
	return view
}

func newNovelIncidentsContextCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var maxDetections, maxDevices int
	cmd := &cobra.Command{
		Use:   "context <incident-uuid>",
		Short: "One incident with its comments and every linked detection and device in one call",
		Long: "Use this command to work one incident: details, comments, every linked detection and device in one call. " +
			"Do NOT use it for the queue of open incidents; use 'triage' instead.\n\n" +
			"The incident is read live (GET /v2/incidents/{uuid} fills the detection and device lists that the\n" +
			"incident list leaves empty). Comments, each linked detection and each linked device are then\n" +
			"fetched in parallel. A detection the v1 endpoint does not know is looked up on v2 (ESET Inspect\n" +
			"and Cloud Office); api_version says which answered. Detections are listed oldest first.\n" +
			"Failed lookups are left out of the lists and reported in fetch_failures; comments is null when\n" +
			"the comments fetch failed. A record or comment that comes back but cannot be decoded is left out\n" +
			"and counted in rejected_rows. Customer names come from the synced device-group tree when present.",
		Example: strings.Trim(`
  eset-protect-cli incidents context 8d4f2a17-3c6e-4b9a-a1d2-5e7f9c0b3a64 --agent
  eset-protect-cli incidents context 8d4f2a17-3c6e-4b9a-a1d2-5e7f9c0b3a64 --max-detections 50 --json
  eset-protect-cli incidents context 8d4f2a17-3c6e-4b9a-a1d2-5e7f9c0b3a64 --select incident,detections`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "incident-uuid=example-value",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "incidents context")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one incident UUID (list them with 'incidents list' or 'triage')"))
			}
			if maxDetections < 0 || maxDevices < 0 {
				return usageErr(fmt.Errorf("--max-detections and --max-devices must be 0 or more"))
			}
			incidentUUID := strings.TrimSpace(args[0])

			groups := make([]esetGroup, 0)
			db, err := openEsetStore(cmd, dbPath, "device-groups")
			if err != nil {
				return err
			}
			mirrorHint := esetMirrorHint(db, "device-groups")
			if db != nil {
				esetSyncHints(cmd, db, flags.maxAge, "device-groups")
				groups, err = loadEsetGroups(db)
				_ = db.Close()
				if err != nil {
					return fmt.Errorf("reading synced device groups: %w", err)
				}
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			get := func(ctx context.Context, url string) (json.RawMessage, error) {
				return c.GetNoCache(ctx, url, nil)
			}
			raw, err := get(ctx, replacePathParam(esetHost("incident-management", "/v2/incidents/{incidentUuid}"), "incidentUuid", incidentUUID))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			inc, err := incidentsContextParseIncident(raw)
			if err != nil {
				return apiErr(err)
			}
			jobs, truncated := incidentsContextPlan(inc, maxDetections, maxDevices)
			results, errs := incidentsContextGather(ctx, get, incidentUUID, jobs)
			var rej esetRejects
			view := incidentsContextBuild(inc, truncated, results, errs, groups, &rej)
			view.SyncedHint = mirrorHint
			if n := len(view.FetchFailures); n > 0 {
				esetFprintf(cmd.ErrOrStderr(), "warning: %d of %d linked lookups failed (%s); see fetch_failures\n", n, len(jobs), incidentsContextFailureSummary(view.FetchFailures))
			}
			rej.warn(cmd)

			w := cmd.OutOrStdout()
			if !wantsHumanTable(w, flags) {
				return printJSONFiltered(w, view, flags)
			}
			return incidentsContextHuman(w, view)
		},
	}
	cmd.Flags().IntVar(&maxDetections, "max-detections", 25, "Maximum linked detections to look up")
	cmd.Flags().IntVar(&maxDevices, "max-devices", 25, "Maximum linked devices to look up")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}

// incidentsContextFailureSummary renders "comments, 2 detection(s), 1 device(s)".
func incidentsContextFailureSummary(fs []incidentsContextFailure) string {
	counts := map[string]int{}
	for _, f := range fs {
		kind, _, _ := strings.Cut(f.Part, ":")
		counts[kind]++
	}
	parts := make([]string, 0, 3)
	if counts["comments"] > 0 {
		parts = append(parts, "comments")
	}
	if n := counts["detection"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d detection(s)", n))
	}
	if n := counts["device"]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d device(s)", n))
	}
	return strings.Join(parts, ", ")
}

func incidentsContextHuman(w io.Writer, v incidentsContextView) error {
	inc := v.Incident
	esetFprintf(w, "%s (%s)\n", inc.Name, inc.UUID)
	esetFprintf(w, "  status %s, severity %s, created %s, updated %s\n", inc.Status, inc.Severity, inc.Created, inc.Updated)
	if inc.AssigneeUUID != "" {
		esetFprintf(w, "  assignee %s\n", inc.AssigneeUUID)
	}
	if inc.Description != "" {
		esetFprintf(w, "  %s\n", inc.Description)
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	esetFprintf(w, "\nDetections: %d shown of %d linked\n", len(v.Detections), inc.DetectionCount)
	if len(v.Detections) > 0 {
		fmt.Fprintln(tw, "  OCCURRED\tSEVERITY\tAPI\tNAME\tOBJECT")
		for _, d := range v.Detections {
			esetFprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", d.OccurredAt, d.Severity, d.APIVersion, d.Name, d.Object)
		}
		_ = tw.Flush()
	}
	esetFprintf(w, "\nDevices: %d shown of %d linked\n", len(v.Devices), inc.DeviceCount)
	if len(v.Devices) > 0 {
		fmt.Fprintln(tw, "  NAME\tCUSTOMER\tSTATUS\tLAST SEEN")
		for _, d := range v.Devices {
			esetFprintf(tw, "  %s\t%s\t%s\t%s\n", d.Name, d.Customer, d.FunctionalityStatus, d.LastSeen)
		}
		_ = tw.Flush()
	}
	if un := esetUnattributedLine(v.UnattributedDevices, "linked device(s)", v.UnattributedDeviceUUIDs, "", "Run '"+esetCLIName+" sync --resources device-groups' to name their customer.", v.parentNotVisible); un != "" {
		esetFprintln(w, "  "+un)
	}
	switch {
	case v.Comments == nil:
		fmt.Fprintln(w, "\nComments: not available")
	case len(v.Comments) == 0:
		fmt.Fprintln(w, "\nComments: none")
	default:
		esetFprintf(w, "\nComments: %d\n", len(v.Comments))
		for _, c := range v.Comments {
			esetFprintf(w, "  [%s] %s\n", c.Created, c.Text)
		}
	}
	if v.Truncated.Detections || v.Truncated.Devices {
		fmt.Fprintln(w, "\nnote: some linked records were not looked up; raise --max-detections / --max-devices")
	}
	if v.RejectedRows > 0 {
		esetFprintf(w, "\nnote: %d returned record(s) or comment(s) could not be decoded and were left out\n", v.RejectedRows)
	}
	for _, f := range v.FetchFailures {
		esetFprintf(w, "\n%s: not available (%s)\n", f.Part, f.Error)
	}
	if v.SyncedHint != "" {
		esetFprintf(w, "\nsynced data: %s\n", v.SyncedHint)
	}
	return nil
}
