// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// Hand-authored. 'respond scan', 'respond isolate' and 'respond release':
// resolve one device (or, for scan, a customer) from the local mirror, build
// the exact ESET Connect device-task request, preview it by default, create
// it only with the root --yes, then optionally watch the task runs.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

const respondUseText = "Use this command to scan, isolate or release one named device (or scan a whole customer) and watch the task run. Do NOT use it for scheduled or arbitrary task actions; use 'device-tasks create' instead."

const respondSafetyText = "Nothing is sent without the root --yes: the default run prints the exact\n" +
	"POST /v1/device_tasks request and exits 0. Over MCP the root --yes is never\n" +
	"forwarded, so the MCP tool only previews."

const respondIsolationWarning = "WARNING: isolation cuts the device off the network except for ESET management traffic until it is released with 'respond release'."

func newNovelRespondCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "respond",
		Short: "Scan, isolate or release a device by name (or scan a whole customer), preview the exact task first",
		Long: respondUseText + "\n\n" +
			"Subcommands build an ESET Connect device task with a manual trigger (valid for\n" +
			"24 hours), resolve device and customer names from the local mirror, and can\n" +
			"poll the task runs until they finish with --wait.\n\n" + respondSafetyText,
		Example: strings.Trim(`
  eset-protect-cli respond scan FRONTDESK-01
  eset-protect-cli respond scan FRONTDESK-01 --yes --wait 10m
  eset-protect-cli respond isolate FRONTDESK-01`, "\n"),
		// A bare 'respond --json' is a structured usage error (exit 2), the same
		// contract the generated parents declare via pp:typed-exit-codes.
		// Not pp:parent-group: "respond" is the novel-feature command, so the
		// MCP context tool lists the capability only while this tool exists.
		Annotations: map[string]string{"pp:data-source": "live", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(respondNewScanCmd(flags), respondNewIsolateCmd(flags), respondNewReleaseCmd(flags))
	return cmd
}

// respondCommonFlags are shared by every respond subcommand.
type respondCommonFlags struct {
	wait string
	db   string
}

func respondAddCommonFlags(cmd *cobra.Command, f *respondCommonFlags) {
	cmd.Flags().StringVar(&f.wait, "wait", "0", "After creating the task, poll its runs every 10s for up to this long (e.g. 10m); 0 does not wait")
	cmd.Flags().StringVar(&f.db, "db", "", "Path to the local mirror used to resolve device and customer names ("+esetDBDefault+")")
}

func respondAnnotations() map[string]string {
	return map[string]string{
		"mcp:read-only":         "false",
		"pp:data-source":        "live",
		"pp:preview-happy-path": "true",
		"pp:happy-args":         "device=example-value",
	}
}

func respondNewScanCmd(flags *rootFlags) *cobra.Command {
	var common respondCommonFlags
	var customer, profile string
	var vuln, clean bool
	cmd := &cobra.Command{
		Use:   "scan [device]",
		Short: "Scan one device or a whole customer (on-demand malware scan, or --vuln for a vulnerability scan)",
		Long: respondUseText + "\n\n" +
			"Creates an OnDemandScan task (scan profile InDepth or Smart, all targets,\n" +
			"cleaning off unless --clean) or, with --vuln, an InitiateVulnerabilityScan task.\n" +
			"Target one device by exact display name, hostname or UUID, or a whole customer\n" +
			"with --customer (name or COMPANY group UUID); exactly one of the two.\n\n" +
			respondSafetyText,
		Example: strings.Trim(`
  eset-protect-cli respond scan FRONTDESK-01
  eset-protect-cli respond scan FRONTDESK-01 --yes --wait 10m
  eset-protect-cli respond scan --customer "Acme Dental" --vuln --yes
  eset-protect-cli respond scan 3f2b7c1e-8a4d-4f6b-9c2e-1d5a7b9e0f12 --scan-profile Smart --clean --yes`, "\n"),
		Annotations: respondAnnotations(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "respond scan")
			}
			device, err := respondValidateTargets("scan", args, customer)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			wait, err := respondParseWait(common.wait)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			action := respondActionScan
			opts := respondScanOptions{Clean: clean}
			if vuln {
				if cmd.Flags().Changed("scan-profile") || cmd.Flags().Changed("clean") {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--scan-profile and --clean configure the malware scan; drop them when using --vuln"))
				}
				action = respondActionVulnScan
			} else {
				opts.Profile, err = respondNormalizeProfile(profile)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(err)
				}
			}
			return respondExecute(cmd, flags, respondJob{
				verb: "scan", action: action, device: device, customer: strings.TrimSpace(customer),
				scan: opts, wait: wait, db: common.db,
			})
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Scan every device of this customer (name or COMPANY group UUID) instead of one device")
	cmd.Flags().BoolVar(&vuln, "vuln", false, "Run a vulnerability scan (InitiateVulnerabilityScan) instead of a malware scan")
	cmd.Flags().StringVar(&profile, "scan-profile", "InDepth", "Malware scan profile: InDepth or Smart")
	cmd.Flags().BoolVar(&clean, "clean", false, "Let the malware scan clean infected files (cleaningEnabled)")
	respondAddCommonFlags(cmd, &common)
	return cmd
}

func respondNewIsolateCmd(flags *rootFlags) *cobra.Command {
	return respondSingleDevice(flags, &cobra.Command{
		Use:   "isolate <device>",
		Short: "Isolate one device from the network (StartNetworkIsolation)",
		Example: strings.Trim(`
  eset-protect-cli respond isolate FRONTDESK-01
  eset-protect-cli respond isolate FRONTDESK-01 --yes --wait 5m`, "\n"),
	}, "isolate", respondActionIsolate,
		"Creates a StartNetworkIsolation task for one device, named by exact display\n"+
			"name, hostname or UUID. HIGH RISK: the isolated device loses all network access\n"+
			"except to ESET management until you run 'respond release' on it. Isolation is\n"+
			"one device at a time; --customer is refused.")
}

func respondNewReleaseCmd(flags *rootFlags) *cobra.Command {
	return respondSingleDevice(flags, &cobra.Command{
		Use:   "release <device>",
		Short: "Release one device from network isolation (EndNetworkIsolation)",
		Example: strings.Trim(`
  eset-protect-cli respond release FRONTDESK-01
  eset-protect-cli respond release FRONTDESK-01 --yes --wait 5m`, "\n"),
	}, "release", respondActionRelease,
		"Creates an EndNetworkIsolation task for one device, named by exact display\n"+
			"name, hostname or UUID, restoring its normal network access. One device at a\n"+
			"time; --customer is refused.")
}

// respondSingleDevice completes a one-device respond subcommand. Callers
// declare Use, Short and Example in their own cobra.Command literal so tools
// that read the source statically (SKILL/README checks) see each subcommand.
func respondSingleDevice(flags *rootFlags, cmd *cobra.Command, verb, action, detail string) *cobra.Command {
	var common respondCommonFlags
	var customer string
	cmd.Long = respondUseText + "\n\n" + detail + "\n\n" + respondSafetyText
	cmd.Annotations = respondAnnotations()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && cmd.Flags().NFlag() == 0 {
			return cmd.Help()
		}
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "respond "+verb)
		}
		device, err := respondValidateTargets(verb, args, customer)
		if err != nil {
			_ = cmd.Usage()
			return usageErr(err)
		}
		wait, err := respondParseWait(common.wait)
		if err != nil {
			_ = cmd.Usage()
			return usageErr(err)
		}
		return respondExecute(cmd, flags, respondJob{verb: verb, action: action, device: device, wait: wait, db: common.db})
	}
	// --customer exists only to refuse it with an explanation instead of an
	// "unknown flag" error; hidden so MCP and --help do not advertise it.
	cmd.Flags().StringVar(&customer, "customer", "", "Not supported: "+verb+" works on one device at a time")
	_ = cmd.Flags().MarkHidden("customer")
	respondAddCommonFlags(cmd, &common)
	return cmd
}

func respondParseWait(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := cliutil.ParseDurationLoose(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid --wait %q: use a duration such as 5m or 1h, or 0 to not wait", s)
	}
	return d, nil
}

// respondJob is one validated respond invocation.
type respondJob struct {
	verb     string
	action   string
	device   string
	customer string
	scan     respondScanOptions
	wait     time.Duration
	db       string
}

type respondRequestView struct {
	Method string          `json:"method"`
	URL    string          `json:"url"`
	Body   respondTaskBody `json:"body"`
}

type respondPreviewView struct {
	Preview  bool               `json:"preview"`
	Action   string             `json:"action"`
	Targets  respondTargets     `json:"targets"`
	Request  respondRequestView `json:"request"`
	Note     string             `json:"note"`
	Warnings []string           `json:"warnings,omitempty"`
}

type respondRefusalView struct {
	Refused bool           `json:"refused"`
	Harness string         `json:"harness"`
	Action  string         `json:"action"`
	Targets respondTargets `json:"targets"`
	Reason  string         `json:"reason"`
}

type respondCreatedView struct {
	TaskUUID string         `json:"task_uuid"`
	Action   string         `json:"action"`
	Targets  respondTargets `json:"targets"`
}

type respondResultView struct {
	Created       respondCreatedView `json:"created"`
	Runs          []respondRun       `json:"runs"`
	Complete      bool               `json:"complete"`
	WaitedSeconds int64              `json:"waited_seconds"`
	ExpectedRuns  int                `json:"expected_runs,omitempty"`
	PollError     string             `json:"poll_error,omitempty"`
	Note          string             `json:"note,omitempty"`
}

// respondLoadMirror opens the local mirror only when it exists or a name
// must be resolved (a UUID target needs no mirror; the mirror only adds names).
func respondLoadMirror(cmd *cobra.Command, flags *rootFlags, job respondJob) (respondMirror, error) {
	path := job.db
	if path == "" {
		path = defaultDBPath(esetCLIName)
	}
	m := respondMirror{path: path}
	resources := []string{"devices"}
	if job.customer != "" {
		resources = []string{"device-groups", "devices"}
	}
	needNames := (job.device != "" && !respondLooksLikeUUID(job.device)) || (job.customer != "" && !respondLooksLikeUUID(job.customer))
	if _, err := os.Stat(path); err != nil && !needNames {
		return m, nil
	}
	db, err := openEsetStore(cmd, path, resources...)
	if err != nil || db == nil {
		return m, err
	}
	defer db.Close()
	m.present = true
	// A --customer run also takes its expected device list from the mirror,
	// so an unfinished sync is worth a hint even for a group UUID.
	if needNames || job.customer != "" {
		esetSyncHints(cmd, db, flags.maxAge, resources...)
	}
	if m.devices, err = loadEsetDevices(db); err != nil {
		return m, fmt.Errorf("reading devices from %s: %w", path, err)
	}
	if job.customer != "" {
		if m.groups, err = loadEsetGroups(db); err != nil {
			return m, fmt.Errorf("reading device groups from %s: %w", path, err)
		}
	}
	return m, nil
}

func respondExecute(cmd *cobra.Command, flags *rootFlags, job respondJob) error {
	w := cmd.OutOrStdout()
	m, err := respondLoadMirror(cmd, flags, job)
	if err != nil {
		return err
	}
	waive := requiredInputWaived()
	targets := respondNewTargets()
	want := respondExpectation{}
	if job.device != "" {
		ref, err := m.resolveDevice(job.device, waive)
		if err != nil {
			_ = cmd.Usage()
			return usageErr(err)
		}
		targets.Devices = append(targets.Devices, ref)
		if ref.UUID != "" {
			want.DeviceUUIDs = []string{ref.UUID}
		}
	} else {
		ref, err := m.resolveCustomer(job.customer, waive)
		if err != nil {
			_ = cmd.Usage()
			return usageErr(err)
		}
		targets.Groups = append(targets.Groups, ref)
		want.DeviceUUIDs = m.customerDeviceUUIDs(ref.UUID)
		want.MinRuns = len(want.DeviceUUIDs)
		want.Unknown = len(want.DeviceUUIDs) == 0
	}

	body := respondBuildBody(job.action, job.verb, job.scan, targets, time.Now())
	createURL := esetHost("automation", "/v1/device_tasks")

	if !flags.yes {
		sendURL, err := esetPreviewURL(flags, createURL)
		if err != nil {
			return err
		}
		view := respondPreviewView{
			Preview: true,
			Action:  job.action,
			Targets: targets,
			Request: respondRequestView{Method: "POST", URL: sendURL, Body: body},
			Note:    "nothing was sent; rerun with --yes to create the task",
		}
		if job.action == respondActionIsolate {
			view.Note += ". " + respondIsolationWarning
		}
		for _, r := range append(append([]respondRef{}, targets.Devices...), targets.Groups...) {
			if r.Unresolved {
				view.Warnings = append(view.Warnings, fmt.Sprintf("%q was not resolved (test harness run); a real run needs a synced name or a UUID", r.Name))
			}
		}
		if !wantsHumanTable(w, flags) {
			return printJSONFiltered(w, view, flags)
		}
		return respondPrintPreviewHuman(w, view)
	}

	if cliutil.IsAnyHarness() {
		harness := cliutil.HarnessName()
		view := respondRefusalView{
			Refused: true,
			Harness: harness,
			Action:  job.action,
			Targets: targets,
			Reason:  fmt.Sprintf("running under an automated test harness (%s): respond never creates ESET device tasks (scan, isolate or release a real device) from a test run", harness),
		}
		if !wantsHumanTable(w, flags) {
			return printJSONFiltered(w, view, flags)
		}
		_, err := esetFprintf(w, "refused: %s\n", view.Reason)
		return err
	}

	c, err := flags.newClient()
	if err != nil {
		return err
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	resp, _, err := c.Post(ctx, createURL, body)
	cancel()
	if err != nil {
		return classifyAPIError(w, respondCreateError(err), flags)
	}
	taskUUID, err := respondParseCreated(resp)
	if err != nil {
		return classifyAPIError(w, apiErr(err), flags)
	}

	result := respondResultView{
		Created:      respondCreatedView{TaskUUID: taskUUID, Action: job.action, Targets: targets},
		Runs:         make([]respondRun, 0),
		ExpectedRuns: want.MinRuns,
	}
	if job.wait > 0 {
		poll := respondPollRuns(cmd.Context(), c, taskUUID, want, job.wait, respondPollInterval,
			func(p context.Context) (context.Context, context.CancelFunc) { return boundCtx(p, flags) })
		result.Runs = poll.Runs
		result.Complete = poll.Complete
		result.WaitedSeconds = int64(math.Round(poll.Waited.Seconds()))
		if poll.LastError != nil {
			result.PollError = poll.LastError.Error()
			esetFprintf(cmd.ErrOrStderr(), "warning: task %s was created, but watching its runs failed: %v\n", taskUUID, poll.LastError)
		}
	}
	for i := range result.Runs {
		if result.Runs[i].DeviceName == "" {
			result.Runs[i].DeviceName = m.deviceName(result.Runs[i].DeviceUUID)
		}
	}
	result.Note = respondResultNote(result, job.wait, taskUUID)

	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, result, flags)
	}
	return respondPrintResultHuman(w, result)
}

// respondResultNote explains an incomplete result so it does not read as a failure.
func respondResultNote(r respondResultView, wait time.Duration, taskUUID string) string {
	follow := fmt.Sprintf("follow it with '%s device-tasks runs device-tasks-list-task %s'", esetCLIName, taskUUID)
	switch {
	case r.Complete:
		return ""
	case wait <= 0:
		return "task created; not waiting (--wait 0). " + follow
	case len(r.Runs) == 0:
		return "no runs yet: a run appears only when the device picks the task up (it may be offline); " + follow
	default:
		return "some runs are still queued or running, or not every device has reported; " + follow
	}
}

func respondPrintPreviewHuman(w io.Writer, v respondPreviewView) error {
	esetFprintf(w, "PREVIEW (nothing sent): %s on %s\n", v.Action, v.Targets.label())
	for _, d := range v.Targets.Devices {
		esetFprintf(w, "  device  %s  %s\n", d.Name, d.UUID)
	}
	for _, g := range v.Targets.Groups {
		esetFprintf(w, "  group   %s  %s\n", g.Name, g.UUID)
	}
	esetFprintf(w, "\n%s %s\n", v.Request.Method, v.Request.URL)
	b, err := json.MarshalIndent(v.Request.Body, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\n\n", b)
	for _, warn := range v.Warnings {
		esetFprintf(w, "warning: %s\n", warn)
	}
	_, err = esetFprintf(w, "%s\n", v.Note)
	return err
}

func respondPrintResultHuman(w io.Writer, r respondResultView) error {
	esetFprintf(w, "Created task %s: %s on %s\n", r.Created.TaskUUID, r.Created.Action, r.Created.Targets.label())
	if len(r.Runs) > 0 {
		rows := make([]map[string]any, 0, len(r.Runs))
		for _, run := range r.Runs {
			rows = append(rows, map[string]any{
				"device": run.DeviceName, "device_uuid": run.DeviceUUID, "status": run.Status,
				"start_time": run.StartTime, "end_time": run.EndTime,
			})
		}
		if err := printAutoTable(w, rows); err != nil {
			return err
		}
	}
	esetFprintf(w, "complete: %t (waited %ds)\n", r.Complete, r.WaitedSeconds)
	if r.PollError != "" {
		esetFprintf(w, "poll error: %s\n", r.PollError)
	}
	if r.Note != "" {
		_, err := esetFprintln(w, r.Note)
		return err
	}
	return nil
}
