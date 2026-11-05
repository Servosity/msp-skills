// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: devices assign.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"apple-business-pp-cli/internal/client"
	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type serialVerdict struct {
	Serial           string `json:"serial"`
	OK               bool   `json:"ok"`
	AssignedServerID string `json:"assigned_server_id"`
	Error            string `json:"error,omitempty"`
}

type activityReport struct {
	ActivityType string          `json:"activity_type"`
	ServerID     string          `json:"server_id,omitempty"`
	ServerName   string          `json:"server_name,omitempty"`
	Serials      []string        `json:"serials"`
	Activity     activityState   `json:"activity"`
	Verified     bool            `json:"verified"`
	Results      []serialVerdict `json:"results,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type assignOutput struct {
	Activities []activityReport `json:"activities"`
	Skipped    []serialVerdict  `json:"skipped,omitempty"`
	Mismatches int              `json:"mismatches"`
}

// activityOpts are the --wait knobs shared by assign and unassign; the
// durations arrive as strings so 1d / 2h style values work.
type activityOpts struct {
	wait        bool
	waitTimeout string
	poll        string
	timeout     time.Duration
	every       time.Duration
}

func (o *activityOpts) parse() error {
	var err error
	if o.timeout, err = cliutil.ParseDurationLoose(o.waitTimeout); err != nil || o.timeout <= 0 {
		return usageErr(fmt.Errorf("--wait-timeout must be a positive duration such as 10m or 1h"))
	}
	if o.every, err = cliutil.ParseDurationLoose(o.poll); err != nil {
		return usageErr(fmt.Errorf("--poll must be a duration such as 5s"))
	}
	if o.every < time.Second {
		o.every = time.Second
	}
	return nil
}

// runActivity posts one orgDeviceActivities request and, with --wait, polls
// it (bounded only by --wait-timeout) and verifies every serial with want().
func runActivity(ctx context.Context, c *client.Client, ids deviceIDs, rep *activityReport, deadline string, o activityOpts, want func(string) bool) error {
	raw, _, err := c.Post(ctx, "/orgDeviceActivities", activityBody(rep.ActivityType, rep.ServerID, ids.all(rep.Serials), deadline))
	if err != nil {
		return fmt.Errorf("creating %s activity: %w", rep.ActivityType, err)
	}
	if rep.Activity, err = parseActivity(raw); err != nil {
		return err
	}
	if activityDone(rep.Activity) && !activitySucceeded(rep.Activity) && !o.wait {
		return fmt.Errorf("activity %s ended %s/%s", rep.Activity.ID, rep.Activity.Status, rep.Activity.SubStatus)
	}
	if !o.wait || rep.Activity.ID == "" {
		return nil
	}
	limit := o.timeout
	if cliutil.IsDogfoodEnv() && limit > 20*time.Second {
		limit = 20 * time.Second
	}
	wctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	if rep.Activity, err = waitActivity(wctx, c, rep.Activity.ID, o.every); err != nil {
		return err
	}
	rep.Verified = true
	for _, s := range rep.Serials {
		v := serialVerdict{Serial: s}
		id, err := assignedServerID(ctx, c, ids.of(s))
		if err != nil {
			v.Error = err.Error()
		} else {
			v.AssignedServerID, v.OK = id, want(id)
		}
		rep.Results = append(rep.Results, v)
	}
	// The serials above were verified either way; a failed or stopped
	// activity is still an error.
	if a := rep.Activity; !activitySucceeded(a) {
		return fmt.Errorf("activity %s ended %s/%s", a.ID, a.Status, a.SubStatus)
	}
	return nil
}

// countMismatches: serials verified on the wrong server, serials that could
// not be checked or submitted, and activities that failed outright.
func countMismatches(out *assignOutput) int {
	n := 0
	for _, a := range out.Activities {
		if a.Error != "" && len(a.Results) == 0 {
			n += len(a.Serials)
			continue
		}
		for _, r := range a.Results {
			if !r.OK {
				n++
			}
		}
	}
	for _, s := range out.Skipped {
		if !s.OK {
			n++
		}
	}
	return n
}

// emitActivityOutput always prints what was submitted, then reports the
// first hard error, or a non-zero exit when any serial missed its target.
func emitActivityOutput(cmd *cobra.Command, flags *rootFlags, out assignOutput, firstErr error) error {
	out.Mismatches = countMismatches(&out)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		w := cmd.OutOrStdout()
		for _, a := range out.Activities {
			fmt.Fprintf(w, "%s %d device(s) %s: activity %s %s/%s %s\n", a.ActivityType, len(a.Serials), firstNonEmpty(a.ServerName, a.ServerID), firstNonEmpty(a.Activity.ID, "-"), a.Activity.Status, a.Activity.SubStatus, a.Error)
			for _, r := range a.Results {
				fmt.Fprintf(w, "  %s ok=%v server=%s %s\n", r.Serial, r.OK, firstNonEmpty(r.AssignedServerID, "-"), r.Error)
			}
		}
		for _, s := range out.Skipped {
			fmt.Fprintf(w, "  skipped %s: %s\n", s.Serial, s.Error)
		}
	} else if err := printJSONFiltered(cmd.OutOrStdout(), out, flags); err != nil {
		return err
	}
	if firstErr != nil {
		return classifyAPIErrorOnly(firstErr)
	}
	if out.Mismatches > 0 {
		return apiErr(fmt.Errorf("%d device(s) did not reach the requested state; see results and skipped", out.Mismatches))
	}
	return nil
}

func newNovelDevicesAssignCmd(flags *rootFlags) *cobra.Command {
	var server, file, family, addedWithin, deadline, dbPath string
	var fromUnassigned bool
	var o activityOpts
	cmd := &cobra.Command{
		Use:   "assign [serial...]",
		Short: "Assign or move serials to an MDM server by name, optionally waiting and verifying each serial",
		Long: `Use this command to assign or move serials to an MDM server by name, optionally waiting for the activity and verifying each serial. Do NOT use this command to remove serials from their server; use 'devices unassign' instead.

--server takes the device management service name (case-insensitive exact match)
or its id. Serials come from arguments, --file (one per line, first CSV column, or
this CLI's JSON output; - reads stdin), or --from-unassigned, which selects
devices the local store shows as UNASSIGNED (narrow with --family and
--added-within; run 'sync --resources devices,mdm_servers' first). Store-selected
serials are re-checked live and skipped if they were assigned since the last
sync. --migration-deadline (RFC 3339, at most 90 days out) sends
ASSIGN_DEVICES_WITH_MDM_MIGRATION_DEADLINE for a no-erase MDM migration.
--dry-run prints the request with the resolved serial list without sending it.
Exits non-zero when --wait finds any serial not on the requested server.`,
		Example: strings.Trim(`
  apple-business-cli devices assign --server "Jamf Prod" C02XL0ABJGH5 --wait --dry-run
  apple-business-cli devices assign --server "Jamf Prod" --from-unassigned --family Mac --added-within 7d --wait
  apple-business-cli devices unassigned --json --select serial | apple-business-cli devices assign --server Intune --file -`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:happy-args": "serial=XABC123X0ABC123X0;--server=1F97349736CF4614A94F624E705841AD"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := o.parse(); err != nil {
				return err
			}
			activityType := "ASSIGN_DEVICES"
			if deadline != "" {
				activityType = "ASSIGN_DEVICES_WITH_MDM_MIGRATION_DEADLINE"
				d, ok := parseTime(deadline)
				if !ok {
					return usageErr(fmt.Errorf("--migration-deadline must be RFC 3339, e.g. 2026-03-15T17:00:00Z"))
				}
				if time.Until(d) > 90*24*time.Hour || time.Until(d) <= 0 {
					return usageErr(fmt.Errorf("--migration-deadline must be in the future and at most 90 days out"))
				}
			}
			if server == "" && !requiredInputWaived() {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--server is required (device management service name or id)"))
			}
			if dryRunOK(flags) {
				// Local-only resolution: positional serials, an existing --file,
				// and --from-unassigned from the store. No network.
				serials, err := readSerials(args, existingFile(file), cmd.InOrStdin())
				if err != nil && !requiredInputWaived() {
					return err
				}
				if fromUnassigned {
					more, err := selectUnassigned(cmd, flags, dbPath, currentClientID(flags), family, addedWithin)
					if err != nil && !requiredInputWaived() {
						return err
					}
					serials, _ = readSerials(append(serials, more...), "", nil)
				}
				body := activityBody(activityType, "<id of "+firstNonEmpty(server, "--server")+">", serials, deadline)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"dry_run": true, "action": "devices assign", "would": "POST /orgDeviceActivities", "serials": len(serials), "body": body}, flags)
				}
				b, _ := json.MarshalIndent(body, "", "  ")
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would POST /orgDeviceActivities for %d serial(s)\n%s\n", len(serials), b)
				return nil
			}
			serials, err := readSerials(args, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			// Requests are bounded per call by --timeout in the generated
			// client; --wait is bounded by --wait-timeout. One config picks
			// both the store and the account the activity runs as.
			ctx := cmd.Context()
			c, cur, err := frozenClient(flags)
			if err != nil {
				return err
			}
			c.NoCache = true // a write resolves servers and assignments from live reads only
			var fromStore []string
			if fromUnassigned {
				if fromStore, err = selectUnassigned(cmd, flags, dbPath, cur, family, addedWithin); err != nil {
					return err
				}
			}
			if len(serials) == 0 && len(fromStore) == 0 {
				if fromUnassigned {
					return printJSONFiltered(cmd.OutOrStdout(), assignOutput{Activities: []activityReport{}}, flags)
				}
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("no serials: pass them as arguments, --file, or --from-unassigned"))
			}
			ids := loadDeviceIDs(ctx, storeDBPath(""))
			if fromUnassigned && dbPath != "" {
				// The selection store passed its ownership check in selectUnassigned.
				maps.Copy(ids, loadDeviceIDs(ctx, storeDBPath(dbPath)))
			}
			servers, err := liveServers(ctx, c)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			srv, err := resolveServer(servers, server)
			if err != nil {
				return usageErr(err)
			}
			out := assignOutput{Activities: []activityReport{}}
			// A store row can be stale: ASSIGN_DEVICES would move a device that
			// was assigned since the last sync, so re-check those live.
			explicit := map[string]bool{}
			for _, s := range serials {
				explicit[s] = true
			}
			for _, s := range fromStore {
				if explicit[s] {
					continue
				}
				id, err := assignedServerID(ctx, c, ids.of(s))
				switch {
				case err != nil:
					out.Skipped = append(out.Skipped, serialVerdict{Serial: s, Error: "could not confirm it is still unassigned: " + err.Error()})
				case id != "":
					out.Skipped = append(out.Skipped, serialVerdict{Serial: s, OK: true, AssignedServerID: id, Error: "assigned since the last sync; not moved"})
				default:
					serials = append(serials, s)
					explicit[s] = true
				}
			}
			if len(serials) == 0 {
				return emitActivityOutput(cmd, flags, out, nil)
			}
			rep := activityReport{ActivityType: activityType, ServerID: srv.ID, ServerName: srv.Attributes.ServerName, Serials: serials}
			runErr := runActivity(ctx, c, ids, &rep, deadline, o, func(id string) bool { return id == srv.ID })
			if runErr != nil {
				rep.Error = runErr.Error()
			}
			out.Activities = append(out.Activities, rep)
			return emitActivityOutput(cmd, flags, out, runErr)
		},
	}
	cmd.Flags().StringVar(&server, "server", "", "Device management service name (case-insensitive exact) or id")
	cmd.Flags().StringVar(&file, "file", "", "File of serials (one per line, first CSV column, or this CLI's JSON); - reads stdin")
	cmd.Flags().BoolVar(&fromUnassigned, "from-unassigned", false, "Add every locally synced UNASSIGNED device (see 'devices unassigned'); re-checked live before sending")
	cmd.Flags().StringVar(&family, "family", "", "With --from-unassigned: only this product family (Mac, iPhone, iPad, AppleTV, Watch, Vision)")
	cmd.Flags().StringVar(&addedWithin, "added-within", "", "With --from-unassigned: only devices added to the org within this window (e.g. 7d)")
	cmd.Flags().StringVar(&deadline, "migration-deadline", "", "RFC 3339 deadline, at most 90 days out, for a no-erase MDM migration")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path for --from-unassigned")
	cmd.Flags().BoolVar(&o.wait, "wait", false, "Poll the activity until it finishes, then re-read each serial's assignment and report ok or mismatch")
	cmd.Flags().StringVar(&o.waitTimeout, "wait-timeout", "10m", "Give up waiting after this long (e.g. 10m, 1h)")
	cmd.Flags().StringVar(&o.poll, "poll", "5s", "Interval between activity status checks (minimum 1s)")
	return cmd
}

// existingFile lets only the verification harness dry-run with a placeholder
// --file; a real dry run reports a missing or unreadable file like a live run.
func existingFile(path string) string {
	if path == "" || path == "-" || !requiredInputWaived() {
		return path
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// selectUnassigned reads serials of UNASSIGNED, unreleased devices from the
// local store, refusing a store that was never synced.
func selectUnassigned(cmd *cobra.Command, flags *rootFlags, dbPath, cur, family, addedWithin string) ([]string, error) {
	path := storeDBPath(dbPath)
	synced := false
	if _, err := os.Stat(path); err == nil {
		db, err := store.OpenReadOnlyContext(cmd.Context(), path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		ownerErr := verifyStoreOwner(cmd.Context(), db, path, dbPath, cur)
		synced = db.GetLastSyncedAt("devices") != ""
		_ = db.Close()
		if ownerErr != nil {
			return nil, ownerErr
		}
	}
	if !synced {
		return nil, usageErr(fmt.Errorf("--from-unassigned needs a synced store; run 'apple-business-cli sync --resources devices,mdm_servers' first"))
	}
	rows, err := unassignedRows(cmd, flags, []string{path}, false, path, family, addedWithin)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Serial)
	}
	return out, nil
}
