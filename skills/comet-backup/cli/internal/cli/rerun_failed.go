// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/comet"
)

type rerunRow struct {
	Tenant       string `json:"tenant"`
	Device       string `json:"device,omitempty"`
	Item         string `json:"item,omitempty"`
	ItemID       string `json:"item_id"`
	Vault        string `json:"vault,omitempty"`
	VaultID      string `json:"vault_id"`
	FailedJobID  string `json:"failed_job_id"`
	StatusName   string `json:"status_name"`
	RuleID       string `json:"rule_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	Action       string `json:"action"`
	Reason       string `json:"reason,omitempty"`
	NewJobID     string `json:"new_job_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type rerunView struct {
	WindowHours float64    `json:"window_hours"`
	Dispatched  bool       `json:"dispatched"`
	Planned     int        `json:"planned"`
	Skipped     int        `json:"skipped"`
	Failed      int        `json:"failed"` // dispatch errors plus unconfirmed replies
	Plan        []rerunRow `json:"plan"`
	Note        string     `json:"note,omitempty"`
}

// Failures a re-run cannot fix, or that a person stopped on purpose.
var rerunSkipStatus = map[int64]string{
	7003: "quota exceeded; a retry fails the same way until the quota is raised",
	7005: "cancelled by someone; not re-run automatically",
	7006: "skipped because a backup was already running",
}

// rerunIntentHold is how long a dispatch left in doubt is not sent again: time for
// the device to take it and the server to show the job, after which the plan can see
// for itself (comet-backup handfix: rerun-dispatch-intent).
var rerunIntentHold = 30 * time.Minute

// rerunIntentKey names one re-run of one failed job, not a connection (a device
// that reconnects gets a new connection id). A later failure of the same rule is a
// new job and a new key: its retry is not held back by an older dispatch.
func rerunIntentKey(tenant, device, rule, failedJob string) string {
	return tenant + "\x00" + device + "\x00" + rule + "\x00" + failedJob
}

func newNovelRerunFailedCmd(flags *rootFlags) *cobra.Command {
	var flagWindow string

	cmd := &cobra.Command{
		Use:   "rerun-failed",
		Short: "Plan and, with --yes, re-run failed scheduled backups on devices that are online now",
		Long: `Plan and, with --yes, re-run failed scheduled backups on devices that are online now.

Use this command to re-run the scheduled backups that failed in a time window on devices that are online now. Do NOT use this command to read why jobs failed; use 'triage' instead. Do NOT use it for a single device and rule you already know; use 'devices run-backup' instead.

Without --yes it only prints the plan. Each failed backup is matched to its backup rule (BackupRuleGUID, or the rule with the same protected item and Storage Vault on servers before 24.6.6) and to the device's live connection. Offline devices, quota failures, cancelled jobs and already-running skips are listed as skipped, never dispatched.`,
		Example: strings.Trim(`
  comet-backup-cli rerun-failed --window 24h --agent
  comet-backup-cli rerun-failed --window 24h --yes --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live", "comet:sends-with-yes": "true"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rerun-failed")
			}
			window, err := durationFlag("window", flagWindow)
			if err != nil {
				return err
			}
			flagYes := flags.yes // root --yes: explicit confirmation to dispatch
			if flagYes && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "dispatch backups to customer devices")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			// One run: read the plan and, with --yes, dispatch it.
			run := func() error {
				now := time.Now()
				version, err := serverVersion(ctx, c)
				if err != nil {
					return fleetError(cmd, flags, c, err)
				}
				// Comet 24.6.6+ records BackupRuleGUID on scheduled jobs; a job without
				// it there is a manual or custom run and must not be mapped to a rule.
				ruleIDRecorded := comet.VersionAtLeast(version, 24, 6, 6)
				jobs, err := fetchJobs(ctx, c, now, window)
				if err != nil {
					return fleetError(cmd, flags, c, err)
				}
				f, err := loadFleet(ctx, c)
				if err != nil {
					return fleetError(cmd, flags, c, err)
				}
				online := comet.OnlineIndex(f.live)
				view := rerunView{WindowHours: window.Hours(), Plan: make([]rerunRow, 0)}
				deviceIDs := make([]string, 0) // beside view.Plan: the row prints a device's name
				for _, j := range comet.ProblemJobs(jobs, comet.TriageOptions{Now: now}) {
					if comet.Family(j.Status) != "failed" {
						continue
					}
					p := f.profiles[j.Username]
					row := rerunRow{Tenant: j.Username, ItemID: j.SourceGUID, VaultID: j.DestinationGUID, FailedJobID: j.GUID, StatusName: comet.StatusName(j.Status)}
					if d, ok := p.Devices[j.DeviceID]; ok {
						row.Device = d.FriendlyName
					}
					if s, ok := p.Sources[j.SourceGUID]; ok {
						row.Item = s.Description
					}
					if v, ok := p.Destinations[j.DestinationGUID]; ok {
						row.Vault = v.Description
					}
					row.ConnectionID = online[j.Username+"\x00"+j.DeviceID]
					ruleID, why := ruleFor(p, j, ruleIDRecorded)
					row.RuleID = ruleID
					switch reason, skip := rerunSkipStatus[j.Status]; {
					case skip:
						row.Action, row.Reason = "skip", reason
					case why != "":
						row.Action, row.Reason = "skip", why
					case row.ConnectionID == "":
						row.Action, row.Reason = "skip", "device is offline"
					default:
						row.Action = "dispatch"
					}
					view.Plan = append(view.Plan, row)
					deviceIDs = append(deviceIDs, j.DeviceID)
				}
				for i := range view.Plan {
					r := &view.Plan[i]
					if r.Action != "dispatch" {
						view.Skipped++
						continue
					}
					view.Planned++
					if !flagYes {
						continue
					}
					key := rerunIntentKey(r.Tenant, deviceIDs[i], r.RuleID, r.FailedJobID)
					age, inDoubt, ierr := cliutil.DispatchIntentAge(key)
					if ierr == nil && inDoubt && rerunIntentHolds(age) {
						r.Action = "unconfirmed"
						r.Error = fmt.Sprintf("an earlier run sent this dispatch and never heard back, so the backup may already have started; check backup-jobs recent, or re-run after %s", rerunIntentHold)
						view.Failed++
						continue
					}
					// Past the deadline nothing is sent: no record, or the next run calls it in doubt.
					if ierr == nil {
						ierr = ctx.Err()
					}
					if ierr == nil {
						ierr = cliutil.RecordDispatchIntent(key)
					}
					if ierr != nil {
						r.Action, r.Error = "failed", safeText(c, ierr.Error())
						view.Failed++
						continue
					}
					fields := url.Values{}
					fields.Set("TargetID", r.ConnectionID)
					fields.Set("BackupRule", r.RuleID)
					data, status, derr := c.PostFormWithParams(ctx, pathRunBackup, nil, fields)
					if derr != nil {
						// Only a 4xx is the server saying it did not act, and a 408 says it
						// stopped waiting. A lost or late reply, or a 5xx from anything in
						// front of it, may follow a started backup: it stays in doubt.
						if status >= 400 && status < 500 && status != 408 {
							cliutil.ClearDispatchIntent(key)
							r.Action, r.Error = "failed", safeText(c, derr.Error())
						} else {
							r.Action, r.Error = "unconfirmed", safeText(c, derr.Error())
						}
						view.Failed++
						continue
					}
					// Only a well-formed DispatchWithJobIDResponse with a 2xx Status and a
					// JobID confirms the dispatch. Anything else may or may not have
					// started a backup, so it is reported (never retried automatically).
					var resp struct {
						Status  int
						Message string
						JobID   string
					}
					if jerr := json.Unmarshal(data, &resp); jerr != nil || resp.Status < 200 || resp.Status > 299 || resp.JobID == "" {
						r.Action = "unconfirmed"
						// The server message is not echoed: it is unmasked server text.
						r.Error = fmt.Sprintf("dispatch reply did not confirm a job (status %d); check devices online / backup-jobs recent before retrying", resp.Status)
						view.Failed++
						continue
					}
					cliutil.ClearDispatchIntent(key)
					r.Action, r.NewJobID = "dispatched", resp.JobID
				}
				view.Dispatched = flagYes
				if !flagYes && view.Planned > 0 {
					view.Note = "plan only; re-run with --yes to dispatch"
				}
				if view.Failed > 0 {
					fmt.Fprintf(os.Stderr, "warning: %d of %d dispatches failed or were not confirmed\n", view.Failed, view.Planned)
				}
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					if len(view.Plan) == 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "No failed scheduled backups in the last %s.\n", flagWindow)
					} else {
						rows2 := make([][]string, 0, len(view.Plan))
						for _, r := range view.Plan {
							rows2 = append(rows2, []string{r.Tenant, r.Device, r.Item, r.StatusName, r.Action, r.Reason + r.Error})
						}
						if err := printCleanTable(cmd, flags, []string{"TENANT", "DEVICE", "ITEM", "STATUS", "ACTION", "REASON"}, rows2); err != nil {
							return err
						}
						if view.Note != "" {
							fmt.Fprintln(cmd.OutOrStdout(), view.Note)
						}
					}
				} else if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, rerunAgentKeep...); err != nil {
					return err
				}
				if view.Failed > 0 && !flags.allowPartialFailure {
					return &cliError{code: 6, err: fmt.Errorf("%d of %d backup dispatches failed or were not confirmed; pass --allow-partial-failure to exit 0", view.Failed, view.Planned)}
				}
				return nil
			}
			if !flagYes {
				return run()
			}
			// comet-backup hand-fix (rerun-dispatch-lock): with --yes the plan is
			// read and dispatched under this host's dispatch lock for the Comet
			// Server. A second rerun-failed --yes (another terminal, an agent, a
			// scheduled job) plans only after the first has finished, so it sees the
			// backups the first started as running and never dispatches them again.
			return withRerunDispatchLock(ctx, c, run)
		}),
	}
	cmd.Flags().StringVar(&flagWindow, "window", "24h", "How far back to look for failed backups (e.g. 24h, 48h)")
	return cmd
}

// ruleFor returns the backup rule to re-run for job j, re-checked against the
// tenant's current profile, or a skip reason. The rule must still exist, still
// target the same protected item and Storage Vault, and the protected item
// must still belong to the device the job ran on; otherwise dispatching it
// would back up something other than what failed. On servers that record
// BackupRuleGUID (24.6.6+), a job without one is a manual or custom run.
func ruleFor(p comet.Profile, j comet.Job, ruleIDRecorded bool) (string, string) {
	ruleID := j.BackupRuleGUID
	if ruleID == "" {
		if ruleIDRecorded {
			return "", "job has no backup rule id (a manual or custom run); use devices run-backup-custom"
		}
		ids := make([]string, 0, len(p.BackupRules))
		for id, r := range p.BackupRules {
			if r.Source == j.SourceGUID && r.Destination == j.DestinationGUID {
				ids = append(ids, id)
			}
		}
		switch len(ids) {
		case 0:
			return "", "no backup rule matches this protected item and Storage Vault (a custom job); use devices run-backup-custom"
		case 1:
			ruleID = ids[0]
		default:
			return "", fmt.Sprintf("%d backup rules match this protected item and Storage Vault; pick one with devices run-backup", len(ids))
		}
	}
	r, ok := p.BackupRules[ruleID]
	if !ok {
		return ruleID, "backup rule no longer exists"
	}
	if r.Source != j.SourceGUID || r.Destination != j.DestinationGUID {
		return ruleID, "backup rule now targets a different protected item or Storage Vault"
	}
	if src, ok := p.Sources[j.SourceGUID]; !ok {
		return ruleID, "protected item no longer exists"
	} else if src.OwnerDevice != "" && src.OwnerDevice != j.DeviceID {
		return ruleID, "protected item now belongs to a different device"
	}
	return ruleID, ""
}

// rerunLockPoll is how often a waiting rerun-failed --yes retries the lock.
const rerunLockPoll = 10 * time.Millisecond

// rerunDispatchLockName names the one rerun-failed dispatch lock of an OS
// account. It is a constant: nothing an invocation supplies renames it.
const rerunDispatchLockName = "rerun-failed-dispatch"

// withRerunDispatchLock runs fn while holding this OS account's one
// rerun-failed dispatch lock, waiting for a run that holds it until ctx ends.
// cliutil.TryWithDispatchLock pairs a process mutex with an OS lock, so runs
// in one process (the MCP server) and in separate processes both serialize;
// the OS releases the lock if a holder dies. When the wait runs out, or the
// lock is refused as not private to this account, nothing has been read or
// dispatched and the error says so.
//
// The lock is found from the OS identity alone (comet-backup hand-fix
// rerun-dispatch-lock, cliutil/comet_dispatch_lock.go): not from HOME,
// USERPROFILE, XDG_*, TMPDIR/TMP/TEMP, COMET_BACKUP_HOME, --home or a config
// path, which differ between two runs by one account (Codex: HOME=/tmp/comet-a
// and HOME=/tmp/comet-b dispatched the same failed job twice), and not from
// the server URL or admin user: one server reached as backup.example.com and
// backup.example.com:443, by IP or alias, or by two admins is still one
// server, and any per-name key splits it (Codex r1). ponytail: runs against
// different Comet Servers wait for each other too; key by the server's own
// identity if that wait ever matters.
func withRerunDispatchLock(ctx context.Context, c *client.Client, fn func() error) error {
	_ = c // the lock is deliberately not keyed by the server; see above
	for {
		ran, err := cliutil.TryWithDispatchLock(rerunDispatchLockName, fn)
		if ran || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return &cliError{code: 7, err: fmt.Errorf("another rerun-failed --yes is dispatching to this Comet Server and did not finish in time; nothing was dispatched. Run again after it finishes")}
		case <-time.After(rerunLockPoll):
		}
	}
}

// rerunIntentHolds reports whether an unresolved dispatch intent of this age
// still refuses a new dispatch. A record from the future (the clock moved back)
// keeps refusing: a negative age does not show the hold has passed.
func rerunIntentHolds(age time.Duration) bool { return age < rerunIntentHold }
