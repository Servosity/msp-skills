// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written helpers shared by the fleet commands (health, triage, usage,
// snapshot, changes, rerun-failed, devices stale, devices outdated).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/comet"
)

const (
	pathListUsersFull  = "/api/v1/admin/list-users-full"
	pathListActive     = "/api/v1/admin/dispatcher/list-active"
	pathJobsDateRange  = "/api/v1/admin/get-jobs-for-date-range"
	pathJobLogEntries  = "/api/v1/admin/get-job-log-entries"
	pathMetaVersion    = "/api/v1/admin/meta/version"
	pathRunBackup      = "/api/v1/admin/dispatcher/run-backup"
	cometDBName        = "comet-backup-cli"
	fleetDefaultGrace  = "6h"
	fleetDefaultMaxAge = "48h"
)

// Row keys --agent/--compact must keep. The fleet rows are already narrow
// projections; their omitempty keys are the sparse, decision-relevant ones (a
// suspended tenant, a device's connection id, an unresolved schedule) that the
// generic compaction would otherwise drop from every row when fewer than 80%
// of rows carry them.
var (
	healthAgentKeep = []string{"device", "device_id", "connection_id", "suspended", "schedule_unresolved",
		"last_status_name", "last_job_id", "last_job_end", "last_success_at", "expected_run_at"}
	usageAgentKeep  = []string{"vault", "vault_id", "shared_quota_id", "shared_quota_vaults", "engines", "last_deep_verify_at"}
	deviceAgentKeep = []string{"connection_id", "client_version", "server_version", "os", "registered_at", "last_job_end"}
	changeAgentKeep = []string{"device", "item", "vault", "before", "after", "detail"}
	triageAgentKeep = []string{"device", "device_id", "item", "vault", "started_at", "ended_at", "connection_id", "error_group"}
	rerunAgentKeep  = []string{"device", "item", "vault", "rule_id", "connection_id", "reason", "new_job_id", "error"}
)

// cometRead sends a read-only Comet admin call. The client adds credentials
// to the form body (see internal/client/comet_auth.go).
func cometRead(ctx context.Context, c *client.Client, path string, fields url.Values) (json.RawMessage, error) {
	if fields == nil {
		fields = url.Values{}
	}
	data, _, err := c.PostQueryFormWithParams(ctx, path, nil, fields)
	return data, err
}

type fleet struct {
	profiles map[string]comet.Profile
	live     map[string]comet.LiveConn
}

// loadFleet fetches every tenant profile (decoded into the credential-free
// projection) and the current live connections.
func loadFleet(ctx context.Context, c *client.Client) (fleet, error) {
	raw, err := cometRead(ctx, c, pathListUsersFull, nil)
	if err != nil {
		return fleet{}, err
	}
	profiles, err := comet.DecodeProfiles(raw)
	if err != nil {
		return fleet{}, err
	}
	raw, err = cometRead(ctx, c, pathListActive, nil)
	if err != nil {
		return fleet{}, err
	}
	live, err := comet.DecodeLive(raw)
	if err != nil {
		return fleet{}, err
	}
	return fleet{profiles: profiles, live: live}, nil
}

// serverVersion reads ServerMetaVersionInfo.Version.
func serverVersion(ctx context.Context, c *client.Client) (string, error) {
	raw, err := cometRead(ctx, c, pathMetaVersion, nil)
	if err != nil {
		return "", err
	}
	var v struct{ Version string }
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("decoding server version: %w", err)
	}
	// comet-backup hand-fix (version-fail-closed): a reply without a readable
	// MAJOR.MINOR version (a Comet failure envelope such as {"Status":403} on
	// HTTP 200, which carries no Message for the client to fail on) is an
	// error, never an old server. Read as old, it turned off rerun-failed's
	// custom-run guard and dispatched scheduled rules for custom-run failures.
	var major, minor int
	if n, _ := fmt.Sscanf(strings.TrimSpace(v.Version), "%d.%d", &major, &minor); n < 2 {
		return "", fmt.Errorf("decoding server version: the reply carried no MAJOR.MINOR Version")
	}
	return v.Version, nil
}

// jobsClockMargin widens the end of a date-range read past this machine's clock.
// The server filters on its own clock, so with that clock ahead a backup started
// a moment ago falls outside [now-window, now], and rerun-failed would plan its
// failed predecessor again (comet-backup handfix: jobs-range-tolerates-clock-skew).
// ponytail: a clock more than a day ahead still hides jobs; that server is misconfigured.
const jobsClockMargin = 24 * time.Hour

// fetchJobs returns BackupJobDetail records whose start falls after now-window.
func fetchJobs(ctx context.Context, c *client.Client, now time.Time, window time.Duration) ([]comet.Job, error) {
	f := url.Values{}
	f.Set("Start", fmt.Sprint(now.Add(-window).Unix()))
	f.Set("End", fmt.Sprint(now.Add(jobsClockMargin).Unix()))
	raw, err := cometRead(ctx, c, pathJobsDateRange, f)
	if err != nil {
		return nil, err
	}
	return comet.DecodeJobs(raw)
}

// durationFlag parses a 6h / 7d / 1w style flag value, naming the flag on error.
func durationFlag(name, value string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--%s %q: use a duration such as 6h, 24h or 7d", name, value))
	}
	return d, nil
}

// csvSet turns "failed, overdue" into a lookup set.
func csvSet(v string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out[p] = true
		}
	}
	return out
}

// validateChoices rejects values outside allowed so a typo is not a silent empty result.
func validateChoices(name string, got map[string]bool, allowed ...string) error {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for g := range got {
		if !ok[g] {
			return usageErr(fmt.Errorf("--%s: unknown value %q (allowed: %s)", name, g, strings.Join(allowed, ", ")))
		}
	}
	return nil
}

// maskSecrets hides the configured Comet secrets in server-provided free text
// (job log lines) before it is stored or printed; see client.MaskCometText.
func maskSecrets(c *client.Client, text string) string {
	if c == nil {
		return text
	}
	return c.MaskCometText(text)
}

// safeText is the single funnel for server-provided text this CLI keeps or
// prints: secrets masked (or the message withheld), terminal controls removed.
func safeText(c *client.Client, text string) string {
	// Clean first so masking sees exactly the characters that will be shown;
	// the masker's output adds no control characters.
	return maskSecrets(c, cleanCell(text))
}

// fleetError is the one exit path for errors from the fleet commands: the
// error is sanitized (safeErr), classified without the mutation-only
// --idempotent 409 no-op shortcut (these are reads), and written as the
// standard error envelope.
func fleetError(cmd *cobra.Command, flags *rootFlags, c *client.Client, err error) error {
	if err == nil {
		return nil
	}
	classified := classifyAPIErrorOnly(safeErr(c, err))
	writeAPIErrorEnvelope(cmd.OutOrStdout(), flags, classified, ExitCode(classified))
	return classified
}

// guardRunE is the last-line backstop for every fleet command: whatever error
// leaves RunE (flag, decode, store or renderer errors included) prints
// sanitized.
func guardRunE(flags *rootFlags, run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := run(cmd, args)
		if err == nil {
			return nil
		}
		// Typed CLI errors are already classified: fleetError sanitized API
		// errors before classifying, and usage/partial-failure errors carry only
		// this CLI's own text. Re-wrapping would lose their exit code.
		var typed *cliError
		if errors.As(err, &typed) {
			return err
		}
		var c *client.Client
		if nc, cerr := flags.newClient(); cerr == nil {
			c = nc
		}
		return safeErr(c, err)
	}
}

// safeErr sanitizes an error before it reaches JSON envelopes or Cobra's
// "Error:" line. An *client.APIError keeps its method, path and status (exit
// codes are derived from "returned HTTP <n>") with only the server body
// sanitized; any other error keeps its chain for classification but prints a
// sanitized message.
func safeErr(c *client.Client, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		clean := *apiErr
		clean.Body = safeText(c, apiErr.Body)
		clean.Path = cleanCell(apiErr.Path)
		return &clean
	}
	return sanitizedError{msg: safeText(c, err.Error()), err: err}
}

type sanitizedError struct {
	msg string
	err error
}

func (e sanitizedError) Error() string { return e.msg }
func (e sanitizedError) Unwrap() error { return e.err }

// cleanCell makes server-provided text safe for a terminal (table cells and
// diagnostic lines alike): control
// characters (ANSI escapes, C1 controls) are dropped and tabs/newlines become
// spaces so a value cannot clear the screen, recolor output or forge a row.
func cleanCell(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == utf8.RuneError:
			return -1
		}
		return r
	}, s)
}

// printCleanTable renders an ordered human table with every cell scrubbed.
func printCleanTable(cmd *cobra.Command, flags *rootFlags, headers []string, rows [][]string) error {
	clean := make([][]string, len(rows))
	for i, row := range rows {
		clean[i] = make([]string, len(row))
		for j, cell := range row {
			clean[i][j] = cleanCell(cell)
		}
	}
	return flags.printTable(cmd, headers, clean)
}

func fmtTime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}
