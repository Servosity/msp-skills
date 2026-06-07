// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: offboard.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"afi-pp-cli/internal/client"
	"afi-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type offboardStep struct {
	Step   string `json:"step"`
	Status string `json:"status"` // ok | skipped | failed
	Detail string `json:"detail,omitempty"`
}

type offboardView struct {
	ResourceID  string         `json:"resource_id"`
	TenantID    string         `json:"tenant_id"`
	PolicyID    string         `json:"policy_id"`
	JobID       string         `json:"job_id,omitempty"`
	TaskID      string         `json:"task_id,omitempty"`
	ArchiveID   string         `json:"archive_id,omitempty"`
	Steps       []offboardStep `json:"steps"`
	Unprotected bool           `json:"unprotected"`
	Reason      string         `json:"reason,omitempty"`
}

func newNovelOffboardCmd(flags *rootFlags) *cobra.Command {
	var flagTenant string
	var flagPolicy string
	var flagReason string
	var flagNoWait bool
	var flagSkipBackup bool
	var flagWaitTimeout time.Duration
	var flagPollInterval time.Duration

	cmd := &cobra.Command{
		Use:   "offboard <resource-id>",
		Short: "Safely back up a departing user's resource, verify the archive landed, then release the protection",
		Long: strings.TrimSpace(`
Run the vendor's archive-and-offboard sequence with a verification gate the
portal lacks: trigger a final out-of-schedule backup, wait for the task to
finish, confirm a fresh archive exists for the resource, and only then remove
the protection so the Microsoft 365 / Google Workspace seat can be released.
The command REFUSES the irreversible unprotect step until the backup is
verified (override the wait with --no-wait at your own risk).

Use this command to safely back up then release a departing user's resource (the vendor's archive-and-offboard sequence).
Do NOT use this command to merely list a resource's backups; use 'tenants archives list' instead.
Do NOT use this command to unprotect without a final backup; use 'tenants resources protections-unprotect' instead.`),
		Example: strings.Trim(`
  # Preview the full sequence without touching the API
  afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --dry-run

  # Run it for real, with an audit reason
  afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --reason "employee departure"
`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args": "resource-id=example-resource-id;--tenant=example-tenant-id;--policy=example-policy-id",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := errNoLocalSource(flags, "offboard"); err != nil {
				return err
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would run: find protection -> trigger final backup job -> wait for task -> verify fresh archive -> unprotect resource")
				return nil
			}
			if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a resource ID is required"))
			}
			resourceID := strings.TrimSpace(args[0])

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			tenantID := flagTenant
			if tenantID == "" {
				tenantID = lookupTenantForResource(ctx, resourceID)
				if tenantID == "" {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--tenant is required (the resource was not found in the local store; run 'afi-cli resolve %s' or pass --tenant explicitly)", resourceID))
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "resolved tenant %s for resource %s from the local store\n", tenantID, resourceID)
			}

			view := offboardView{ResourceID: resourceID, TenantID: tenantID, Reason: flagReason, Steps: make([]offboardStep, 0)}
			step := func(name, status, detail string) {
				view.Steps = append(view.Steps, offboardStep{Step: name, Status: status, Detail: detail})
			}
			emit := func() error { return printJSONFiltered(cmd.OutOrStdout(), view, flags) }

			// Step 1: locate the protection (job + policy).
			base := "/api/v1/tenants/" + url.PathEscape(tenantID)
			protData, err := c.Get(ctx, base+"/protections", map[string]string{"resource_id": resourceID})
			if err != nil {
				step("find-protection", "failed", err.Error())
				_ = emit()
				return classifyAPIError(err, flags)
			}
			var protPage struct {
				Items []struct {
					ID         string `json:"id"`
					ResourceID string `json:"resource_id"`
					PolicyID   string `json:"policy_id"`
					JobID      string `json:"job_id"`
				} `json:"items"`
			}
			if err := json.Unmarshal(protData, &protPage); err != nil {
				step("find-protection", "failed", "decoding protections: "+err.Error())
				_ = emit()
				return apiErr(fmt.Errorf("decoding protections: %w", err))
			}
			var jobID, policyID string
			matches := 0
			for _, p := range protPage.Items {
				if p.ResourceID != resourceID {
					continue
				}
				if flagPolicy != "" && p.PolicyID != flagPolicy {
					continue
				}
				matches++
				jobID, policyID = p.JobID, p.PolicyID
			}
			switch {
			case matches == 0 && flagPolicy != "":
				step("find-protection", "failed", "no protection with policy "+flagPolicy)
				_ = emit()
				return notFoundErr(fmt.Errorf("resource %s has no protection with policy %s in tenant %s", resourceID, flagPolicy, tenantID))
			case matches == 0:
				step("find-protection", "failed", "resource has no protection")
				_ = emit()
				return notFoundErr(fmt.Errorf("resource %s has no protection in tenant %s — nothing to offboard (is it already unprotected?)", resourceID, tenantID))
			case matches > 1:
				step("find-protection", "failed", "multiple protections; pass --policy")
				_ = emit()
				return usageErr(fmt.Errorf("resource %s has %d protections; disambiguate with --policy <policy-id>", resourceID, matches))
			}
			view.PolicyID, view.JobID = policyID, jobID
			step("find-protection", "ok", fmt.Sprintf("policy=%s job=%s", policyID, jobID))

			backupStarted := time.Now().UTC().Add(-5 * time.Second)

			// Step 2: trigger the final backup.
			if flagSkipBackup {
				step("trigger-backup", "skipped", "--skip-backup")
			} else {
				trigData, _, err := c.Put(ctx, base+"/jobs/"+url.PathEscape(jobID)+"/trigger", nil)
				if err != nil {
					step("trigger-backup", "failed", err.Error())
					_ = emit()
					return classifyAPIError(err, flags)
				}
				var trig struct {
					TaskID string `json:"task_id"`
				}
				_ = json.Unmarshal(trigData, &trig)
				view.TaskID = trig.TaskID
				step("trigger-backup", "ok", "task="+trig.TaskID)

				// Step 3: wait for the task.
				if flagNoWait {
					step("wait-task", "skipped", "--no-wait; archive verification will accept any existing archive")
				} else if trig.TaskID == "" {
					// No task handle (job may already be running): poll the
					// archives endpoint until a fresh archive lands or the
					// wait budget runs out, so verification below can pass.
					if err := waitForFreshArchive(ctx, c, base, resourceID, backupStarted, flagWaitTimeout, flagPollInterval); err != nil {
						step("wait-archive", "failed", err.Error())
						_ = emit()
						return err
					}
					step("wait-archive", "ok", "fresh archive observed (no task handle was returned)")
				} else {
					status, err := waitForTask(ctx, c, base, trig.TaskID, flagWaitTimeout, flagPollInterval)
					if err != nil {
						step("wait-task", "failed", err.Error())
						_ = emit()
						return err
					}
					step("wait-task", "ok", "status="+status)
				}
			}

			// Step 4: verify an archive exists (fresh when we waited).
			needFresh := !flagNoWait && !flagSkipBackup
			archID, archAt, err := newestArchive(ctx, c, base, resourceID)
			if err != nil {
				step("verify-archive", "failed", err.Error())
				_ = emit()
				return classifyAPIError(err, flags)
			}
			if archID == "" {
				step("verify-archive", "failed", "no archive with a parseable created_at exists for the resource")
				_ = emit()
				return apiErr(fmt.Errorf("REFUSING to unprotect: no archive with a usable timestamp exists for resource %s — the departing user's data is not verifiably backed up (inspect with 'afi-cli tenants archives list')", resourceID))
			}
			if needFresh {
				t, perr := time.Parse(time.RFC3339, archAt)
				if perr != nil || t.Before(backupStarted) {
					step("verify-archive", "failed", fmt.Sprintf("newest archive %s (created %s) predates the final backup", archID, archAt))
					_ = emit()
					return apiErr(fmt.Errorf("REFUSING to unprotect: newest archive for %s was created %s, before the final backup started — wait for the backup task to finish and re-run, or use --no-wait to accept the existing archive", resourceID, archAt))
				}
			}
			view.ArchiveID = archID
			step("verify-archive", "ok", fmt.Sprintf("archive=%s created=%s", archID, archAt))

			// Step 5: the irreversible bit.
			if _, _, err := c.DeleteWithParams(ctx, base+"/resources/"+url.PathEscape(resourceID)+"/protect", map[string]string{"policy_id": policyID}); err != nil {
				step("unprotect", "failed", err.Error())
				_ = emit()
				return classifyAPIError(err, flags)
			}
			view.Unprotected = true
			step("unprotect", "ok", "protection removed; the external seat can be released")
			return emit()
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Tenant ID owning the resource (resolved from the local store when omitted)")
	cmd.Flags().StringVar(&flagPolicy, "policy", "", "Policy ID of the protection to release (required when the resource has multiple protections)")
	cmd.Flags().StringVar(&flagReason, "reason", "", "Audit note recorded in the command output (e.g. \"employee departure\")")
	cmd.Flags().BoolVar(&flagNoWait, "no-wait", false, "Skip waiting for the final backup task; accept any existing archive as verification")
	cmd.Flags().BoolVar(&flagSkipBackup, "skip-backup", false, "Do not trigger a final backup; verify against existing archives only")
	cmd.Flags().DurationVar(&flagWaitTimeout, "timeout-wait", 30*time.Minute, "Maximum time to wait for the final backup task")
	cmd.Flags().DurationVar(&flagPollInterval, "poll-interval", 15*time.Second, "How often to poll the backup task status")
	return cmd
}

// lookupTenantForResource tries the local store for the resource's tenant.
// Best-effort: any failure returns "" and the caller demands --tenant.
func lookupTenantForResource(ctx context.Context, resourceID string) string {
	db, err := openAfiStore("")
	if err != nil {
		return ""
	}
	defer db.Close()
	rows, err := db.DB().QueryContext(ctx, `
		SELECT DISTINCT COALESCE(json_extract(data,'$.tenant_id'), '')
		FROM resources WHERE resource_type = 'resources' AND id = ?`, resourceID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	tids := []string{}
	for rows.Next() {
		var tid sql.NullString
		if err := rows.Scan(&tid); err != nil {
			return ""
		}
		if tid.String != "" {
			tids = append(tids, tid.String)
		}
	}
	if len(tids) != 1 {
		// Zero matches OR ambiguous (Multi-Geo style duplicate IDs): make the
		// operator pass --tenant explicitly rather than guess a region.
		return ""
	}
	return tids[0]
}

// waitForTask polls the task until a terminal status, timeout, or context end.
// Afi statuses are free-form strings; done/success* are success, fail*/error*
// are failure, anything else keeps polling. Polling is deliberately slow —
// Afi rate-limits aggressively.
func waitForTask(ctx context.Context, c *client.Client, base, taskID string, timeout, interval time.Duration) (string, error) {
	if cliutil.IsDogfoodEnv() && timeout > time.Minute {
		timeout = time.Minute
	}
	if interval <= 0 {
		interval = time.Millisecond // floor: never a zero-delay HTTP loop
	}
	deadline := time.Now().Add(timeout)
	for {
		data, err := c.Get(ctx, base+"/tasks/"+url.PathEscape(taskID), nil)
		if err != nil {
			return "", fmt.Errorf("polling task %s: %w", taskID, err)
		}
		var task struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(data, &task)
		s := strings.ToLower(task.Status)
		switch {
		case s == "done" || strings.HasPrefix(s, "succ") || s == "ok" || s == "completed":
			return task.Status, nil
		case strings.HasPrefix(s, "fail") || strings.HasPrefix(s, "error") || s == "cancelled" || s == "canceled":
			return "", apiErr(fmt.Errorf("final backup task %s ended with status %q — NOT unprotecting; inspect with 'afi-cli tenants tasks get'", taskID, task.Status))
		}
		if time.Now().After(deadline) {
			return "", apiErr(fmt.Errorf("timed out after %s waiting for task %s (last status %q) — NOT unprotecting; re-run later or raise --timeout-wait", timeout, taskID, task.Status))
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

// newestArchive returns the newest archive id + created_at for a resource.
func newestArchive(ctx context.Context, c *client.Client, base, resourceID string) (string, string, error) {
	data, err := c.Get(ctx, base+"/archives", map[string]string{"resource_id": resourceID})
	if err != nil {
		return "", "", err
	}
	var page struct {
		Items []struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return "", "", fmt.Errorf("decoding archives: %w", err)
	}
	bestID, bestAt := "", ""
	var bestT time.Time
	for _, a := range page.Items {
		t, err := time.Parse(time.RFC3339, a.CreatedAt)
		if err != nil {
			// Unparseable timestamp cannot win "newest"; skip it rather than
			// let a lexical comparison pick a chronologically wrong archive.
			continue
		}
		if bestID == "" || t.After(bestT) {
			bestID, bestAt, bestT = a.ID, a.CreatedAt, t
		}
	}
	return bestID, bestAt, nil
}

// waitForFreshArchive polls the archives endpoint until an archive created
// after `since` appears, the timeout elapses, or the context ends. Used when
// the trigger response carried no task handle to poll.
func waitForFreshArchive(ctx context.Context, c *client.Client, base, resourceID string, since time.Time, timeout, interval time.Duration) error {
	if cliutil.IsDogfoodEnv() && timeout > time.Minute {
		timeout = time.Minute
	}
	if interval <= 0 {
		interval = time.Millisecond // floor: never a zero-delay HTTP loop
	}
	deadline := time.Now().Add(timeout)
	for {
		_, archAt, err := newestArchive(ctx, c, base, resourceID)
		if err != nil {
			return fmt.Errorf("polling archives for %s: %w", resourceID, err)
		}
		if archAt != "" {
			if t, perr := time.Parse(time.RFC3339, archAt); perr == nil && !t.Before(since) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return apiErr(fmt.Errorf("timed out after %s waiting for a fresh archive for %s — NOT unprotecting; re-run later or raise --timeout-wait", timeout, resourceID))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
