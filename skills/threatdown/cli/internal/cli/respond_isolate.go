// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/client"
	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
)

// respondVerb describes one guarded job command.
type respondVerb struct {
	name    string // CLI verb
	command string // OneView job command
	short   string
	// skip reports why an endpoint should not receive this job (no-op guard).
	skip func(fleet.Endpoint) string
}

var respondVerbs = []respondVerb{
	{"isolate", fleet.CmdIsolate, "Isolate endpoints (network, process, desktop) by host name, with a target preview and blast-radius guard",
		func(e fleet.Endpoint) string {
			if e.Isolated {
				return "already isolated"
			}
			return ""
		}},
	{"restore", fleet.CmdRestore, "Release isolated endpoints back onto the network, with a target preview",
		func(e fleet.Endpoint) string {
			if !e.Isolated {
				return "not isolated"
			}
			return ""
		}},
	{"scan", fleet.CmdScan, "Run a threat scan on endpoints by host name, optionally quarantining what it finds", nil},
	{"reboot", fleet.CmdReboot, "Reboot endpoints by host name, with a target preview and blast-radius guard", nil},
	{"update", fleet.CmdUpdate, "Update protection definitions on endpoints by host name", nil},
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		parent, _, err := root.Find([]string{"respond"})
		if err != nil || parent == root {
			return
		}
		addNovelCommandIfAbsent(parent, newRespondRestoreCmd(flags))
		addNovelCommandIfAbsent(parent, newRespondScanCmd(flags))
		addNovelCommandIfAbsent(parent, newRespondRebootCmd(flags))
		addNovelCommandIfAbsent(parent, newRespondUpdateCmd(flags))
	})
}

// One constructor per verb with a literal Use, so static doc checkers can see
// each command.
func newNovelRespondIsolateCmd(flags *rootFlags) *cobra.Command {
	return configureRespondJobCmd(&cobra.Command{Use: "isolate"}, flags, respondVerbs[0])
}

func newRespondRestoreCmd(flags *rootFlags) *cobra.Command {
	return configureRespondJobCmd(&cobra.Command{Use: "restore"}, flags, respondVerbs[1])
}

func newRespondScanCmd(flags *rootFlags) *cobra.Command {
	return configureRespondJobCmd(&cobra.Command{Use: "scan"}, flags, respondVerbs[2])
}

func newRespondRebootCmd(flags *rootFlags) *cobra.Command {
	return configureRespondJobCmd(&cobra.Command{Use: "reboot"}, flags, respondVerbs[3])
}

func newRespondUpdateCmd(flags *rootFlags) *cobra.Command {
	return configureRespondJobCmd(&cobra.Command{Use: "update"}, flags, respondVerbs[4])
}

type respondTarget struct {
	Site       string `json:"site"`
	AccountID  string `json:"account_id"`
	EndpointID string `json:"endpoint_id"`
	HostName   string `json:"host_name"`
	Online     bool   `json:"online"`
	Isolated   bool   `json:"isolated"`
	LastSeen   string `json:"last_seen,omitempty"`
	Action     string `json:"action"` // issue | skip | rejected | unconfirmed | not-sent
	Reason     string `json:"reason,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	JobState   string `json:"job_state,omitempty"`
}

type respondView struct {
	Verb           string          `json:"verb"`
	Command        string          `json:"command"`
	Data           map[string]any  `json:"data,omitempty"`
	Issued         bool            `json:"issued"`
	IssuedCount    int             `json:"issued_count"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	CorrelationIDs []string        `json:"correlation_ids,omitempty"` // one per batch of 99 endpoints
	WouldIssue     int             `json:"would_issue"`
	Skipped        int             `json:"skipped"`
	Targets        []respondTarget `json:"targets"`
	Unresolved     []string        `json:"unresolved,omitempty"`
	Errors         []fetchFailure  `json:"errors,omitempty"`
	Note           string          `json:"note,omitempty"`
}

// configureRespondJobCmd fills in a respond verb command; the caller sets Use.
func configureRespondJobCmd(cmd *cobra.Command, flags *rootFlags, v respondVerb) *cobra.Command {
	var hosts []string
	var siteFilter, isoMode, scanType string
	var useStdin, confirm, wait, remove, allMatches bool
	var maxTargets, maxPages int
	var waitTimeout time.Duration

	sibling := "respond pending"
	cmd.Short = v.short
	cmd.Long = fmt.Sprintf(`Use this command to issue a %s job (%s) against named endpoints with a target preview. Do NOT use this command to check jobs already issued; use '%s' instead.

Targets come from --host (exact host name or fully qualified host name,
case-insensitive, matched across every site in scope) or --stdin (JSON with
endpoint_id/id and account_id, e.g. piped from 'fleet stale --json' or
'threats queue --json'). A host name that matches endpoints in more than one
place is refused unless --site narrows it or --all-matches is given. Without
--confirm (or the global --yes) nothing is sent: the command prints which
endpoints would get the job and which are skipped. More than --max-targets
endpoints is refused. The global --dry-run only prints a placeholder; run
without --confirm to see the real preview.`, v.name, v.command, sibling)
	cmd.Example = strings.Trim(fmt.Sprintf(`
  threatdown-cli respond %[1]s --host LAPTOP-042
  threatdown-cli respond %[1]s --host LAPTOP-042 --site "Acme" --confirm --wait
  threatdown-cli fleet stale --hours 72 --json | threatdown-cli respond %[1]s --stdin`, v.name), "\n")
	cmd.Annotations = map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "--host=LAPTOP-042"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if dryRunOK(flags) {
			return writeDryRun(cmd.OutOrStdout(), flags, "respond "+v.name)
		}
		if len(hosts) == 0 && !useStdin {
			_ = cmd.Usage()
			return usageErr(fmt.Errorf("give --host <name> or --stdin"))
		}
		if maxTargets < 1 {
			return usageErr(fmt.Errorf("--max-targets must be at least 1"))
		}
		data, err := respondJobData(v.name, isoMode, scanType, remove)
		if err != nil {
			return usageErr(err)
		}
		if scope := strings.Fields(cliutil.EnvOverride("THREATDOWN_OAUTH_SCOPE")); len(scope) > 0 && !containsFold(scope, "execute") {
			return authErr(fmt.Errorf("THREATDOWN_OAUTH_SCOPE=%q lacks the execute scope that job commands need", strings.Join(scope, " ")))
		}
		var stdinRefs []endpointRef
		if useStdin {
			if stdinRefs, err = readEndpointRefs(os.Stdin); err != nil {
				return usageErr(err)
			}
		}
		ctx := cmd.Context()
		c, err := flags.newClient()
		if err != nil {
			return err
		}
		view := respondView{Verb: v.name, Command: v.command, Data: data, Targets: make([]respondTarget, 0)}
		res, err := resolveRespondTargets(ctx, c, hosts, stdinRefs, siteFilter, maxPages)
		if err != nil {
			return classifyAPIErrorOnly(err)
		}
		if len(res.ambiguous) > 0 && !allMatches {
			return usageErr(fmt.Errorf("%s; add --site to narrow it or --all-matches to target every match", strings.Join(res.ambiguous, "; ")))
		}
		endpoints, sites := res.endpoints, res.sites
		view.Unresolved = res.unresolved
		idx := fleet.SiteIndex(sites)
		now := time.Now().UTC()
		issue := make([]map[string]string, 0)
		for _, e := range endpoints {
			t := respondTarget{Site: idx[e.AccountID].Name, AccountID: e.AccountID, EndpointID: e.ID, HostName: e.Label(), Online: e.Online, Isolated: e.Isolated, Action: "issue"}
			if ls, ok := e.LastSeen(); ok {
				t.LastSeen = ls.Format(time.RFC3339)
			}
			if v.skip != nil {
				if reason := v.skip(e); reason != "" {
					t.Action, t.Reason = "skip", reason
				}
			}
			if t.Action == "issue" {
				issue = append(issue, map[string]string{"id": e.ID, "account_id": e.AccountID})
				if !e.Online {
					t.Reason = fmt.Sprintf("offline; runs when it next checks in (%dh since seen)", e.HoursSinceSeen(now))
				}
			} else {
				view.Skipped++
			}
			view.Targets = append(view.Targets, t)
		}
		view.WouldIssue = len(issue)
		if len(issue) > maxTargets {
			_ = printJSONFiltered(cmd.ErrOrStderr(), view, flags)
			return usageErr(fmt.Errorf("%d endpoints selected, more than --max-targets %d; narrow --host/--site or raise --max-targets", len(issue), maxTargets))
		}
		if !(confirm || flags.yes) || len(issue) == 0 {
			switch {
			case len(issue) == 0:
				view.Note = "nothing to issue"
			default:
				view.Note = "preview only; add --confirm to issue"
			}
			return printRespond(cmd, flags, view)
		}
		if cliutil.IsAnyHarness() {
			return writeHarnessRefusal(cmd.OutOrStdout(), flags, "respond "+v.name)
		}
		// The vendor documents that a jobs request with 100+ endpoints returns no
		// job ids, so issue in batches small enough to keep every id.
		const bulkBatch = 99
		jobByMachine := map[string]string{}
		rejected := map[string]bool{}
		notSent := map[string]string{}   // never dispatched
		uncertain := map[string]string{} // dispatched, outcome unknown
		var batchErr error
		for start := 0; start < len(issue); start += bulkBatch {
			batch := issue[start:min(start+bulkBatch, len(issue))]
			if batchErr != nil {
				for _, b := range batch {
					notSent[b["id"]] = "not sent: an earlier batch failed"
				}
				continue
			}
			body := map[string]any{"command": v.command, "endpoints": batch, "groups": []any{}}
			if len(data) > 0 {
				body["data"] = data
			}
			resp, _, err := c.Post(ctx, "/v1/jobs/bulk", body)
			if err != nil {
				batchErr = err
				var apiErr *client.APIError
				refused := errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500
				for _, b := range batch {
					if refused {
						rejected[b["id"]] = true
						view.Errors = append(view.Errors, fetchFailure{Item: b["id"], Error: fmt.Sprintf("OneView refused the request (HTTP %d)", apiErr.StatusCode)})
					} else {
						uncertain[b["id"]] = "request failed (" + err.Error() + "); the job may still have been created, check '" + tdBinary + " respond pending' before retrying"
					}
				}
				continue
			}
			var out struct {
				Jobs []struct {
					MachineID string `json:"machine_id"`
					JobID     string `json:"job_id"`
				} `json:"jobs"`
				Errors []struct {
					ID        string `json:"id"`
					AccountID string `json:"account_id"`
				} `json:"errors"`
				CorrelationID string `json:"correlation_id"`
			}
			if err := json.Unmarshal(resp, &out); err != nil {
				batchErr = fmt.Errorf("parsing jobs/bulk response: %w", err)
				for _, b := range batch {
					uncertain[b["id"]] = "sent, but the response could not be read; check '" + tdBinary + " respond pending' before retrying"
				}
				continue
			}
			if out.CorrelationID != "" {
				view.CorrelationIDs = append(view.CorrelationIDs, out.CorrelationID)
			}
			for _, j := range out.Jobs {
				jobByMachine[j.MachineID] = j.JobID
			}
			for _, e := range out.Errors {
				rejected[e.ID] = true
				view.Errors = append(view.Errors, fetchFailure{Item: e.ID, Error: "rejected by OneView for account " + e.AccountID})
			}
		}
		if len(view.CorrelationIDs) > 0 {
			view.CorrelationID = view.CorrelationIDs[0]
		}
		notCreated := 0
		for i := range view.Targets {
			t := &view.Targets[i]
			if t.Action != "issue" {
				continue
			}
			if id := jobByMachine[t.EndpointID]; id != "" {
				t.JobID = id
				view.IssuedCount++
				continue
			}
			notCreated++
			if reason, ok := notSent[t.EndpointID]; ok {
				t.Action, t.Reason = "not-sent", reason
			} else if reason, ok := uncertain[t.EndpointID]; ok {
				t.Action, t.Reason = "unconfirmed", reason
			} else if rejected[t.EndpointID] {
				t.Action, t.Reason = "rejected", "OneView rejected the job for this endpoint"
			} else {
				t.Action, t.Reason = "unconfirmed", "OneView returned no job id for this endpoint"
			}
		}
		view.Issued = view.IssuedCount > 0
		var pollErr error
		if wait && view.IssuedCount > 0 {
			pollErr = waitForJobs(ctx, c, view.Targets, waitTimeout, cmd.ErrOrStderr())
		}
		if err := printRespond(cmd, flags, view); err != nil {
			return err
		}
		if notCreated > 0 {
			if view.IssuedCount == 0 && batchErr != nil {
				return classifyAPIErrorOnly(fmt.Errorf("issuing %s: %w", v.command, batchErr))
			}
			return apiErr(fmt.Errorf("%d of %d jobs were not confirmed as created; see targets[].reason", notCreated, view.WouldIssue))
		}
		return pollErr
	}
	cmd.Flags().StringSliceVar(&hosts, "host", nil, "Endpoint host name (exact, case-insensitive); repeat or comma-separate for several")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only look for hosts in sites whose name contains this text")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "Read targets as JSON (endpoint_id/id + account_id) from stdin")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Issue the job (without it, only a preview is printed)")
	cmd.Flags().IntVar(&maxTargets, "max-targets", 25, "Refuse when more than this many endpoints would get the job")
	cmd.Flags().BoolVar(&allMatches, "all-matches", false, "Target every endpoint a host name matches, even across several sites")
	cmd.Flags().IntVar(&maxPages, "max-scan-pages", tdDefaultMaxPages, "Maximum 2000-row search pages to scan while resolving targets (refuses when exceeded)")
	cmd.Flags().BoolVar(&wait, "wait", false, "After issuing, poll each job until it completes, fails or --wait-timeout passes")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 2*time.Minute, "How long --wait polls")
	switch v.name {
	case "isolate":
		cmd.Flags().StringVar(&isoMode, "mode", "network,process,desktop", "Isolation extent: any of network, process, desktop")
	case "scan":
		cmd.Flags().StringVar(&scanType, "scan-type", "ThreatScan", "Scan type: ThreatScan or HyperScan")
		cmd.Flags().BoolVar(&remove, "remove", false, "Quarantine what the scan finds (default: report only)")
	}
	return cmd
}

// respondJobData builds the command-specific `data` block documented on
// POST /v1/jobs/bulk.
func respondJobData(verb, isoMode, scanType string, remove bool) (map[string]any, error) {
	switch verb {
	case "isolate":
		data := map[string]any{"network": false, "process": false, "desktop": false}
		n := 0
		for _, m := range strings.Split(isoMode, ",") {
			m = strings.ToLower(strings.TrimSpace(m))
			if m == "" {
				continue
			}
			if _, ok := data[m]; !ok {
				return nil, fmt.Errorf("--mode %q: use network, process and/or desktop", m)
			}
			data[m] = true
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("--mode needs at least one of network, process, desktop")
		}
		return data, nil
	case "scan":
		if scanType != "ThreatScan" && scanType != "HyperScan" {
			return nil, fmt.Errorf("--scan-type must be ThreatScan or HyperScan, got %q", scanType)
		}
		return map[string]any{"scan_settings": map[string]any{"type": scanType, "remove": remove}}, nil
	}
	return nil, nil
}

type endpointRef struct {
	ID        string
	AccountID string
}

// readEndpointRefs accepts a JSON array, or an object wrapping one under
// endpoints/queue/targets/results, of records carrying endpoint_id (or id)
// and account_id.
func readEndpointRefs(r io.Reader) ([]endpointRef, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		var env map[string]json.RawMessage
		if json.Unmarshal(raw, &env) != nil {
			return nil, fmt.Errorf("stdin is not JSON")
		}
		for _, k := range []string{"endpoints", "queue", "targets", "results"} {
			if v, ok := env[k]; ok && json.Unmarshal(v, &items) == nil {
				break
			}
		}
	}
	refs := make([]endpointRef, 0, len(items))
	for _, it := range items {
		id, _ := it["endpoint_id"].(string)
		if id == "" {
			id, _ = it["id"].(string)
		}
		acct, _ := it["account_id"].(string)
		if id != "" && acct != "" {
			refs = append(refs, endpointRef{ID: id, AccountID: acct})
		}
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("stdin had no records with endpoint_id/id and account_id")
	}
	return refs, nil
}

type respondResolution struct {
	endpoints  []fleet.Endpoint
	sites      []fleet.Site
	unresolved []string
	ambiguous  []string
}

// resolveRespondTargets turns host names and stdin refs into current endpoint
// records. Host names must equal an endpoint's host name or fully qualified
// host name (case-insensitive); a name with several matches is reported as
// ambiguous. Stdin refs must belong to a site in scope. A lookup that hits
// maxPages is an error, never a silently partial target list.
func resolveRespondTargets(ctx context.Context, c *client.Client, hosts []string, refs []endpointRef, siteFilter string, maxPages int) (respondResolution, error) {
	var res respondResolution
	sites, err := fetchSites(ctx, c)
	if err != nil {
		return res, err
	}
	res.sites = sites
	scoped, err := scopeSites(sites, siteFilter)
	if err != nil {
		return res, err
	}
	accounts := fleet.AccountIDs(scoped, "")
	inScope := map[string]bool{}
	for _, a := range accounts {
		inScope[a] = true
	}
	idx := fleet.SiteIndex(sites)
	seen := map[string]bool{}
	add := func(e fleet.Endpoint) {
		if !seen[e.ID] {
			seen[e.ID] = true
			res.endpoints = append(res.endpoints, e)
		}
	}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if len(accounts) == 0 {
			res.unresolved = append(res.unresolved, h+" (no sites in scope)")
			continue
		}
		raws, info, err := searchPages(ctx, c, "/v1/endpoints", map[string]any{"account_ids": accounts, "name_multi_search": h}, "endpoints", maxPages)
		if err != nil {
			return res, err
		}
		if info.Truncated {
			return res, fmt.Errorf("searching for host %q stopped at --max-scan-pages %d; narrow --site or raise --max-scan-pages", h, maxPages)
		}
		eps, bad := parseEndpoints(raws)
		if !info.Complete || bad > 0 {
			// Without the full list a second endpoint with this name could go
			// unseen, so the single-match and ambiguity checks would not hold.
			return res, fmt.Errorf("could not confirm the complete search result for host %q (%d unreadable records); retry or use --stdin with endpoint ids", h, bad)
		}
		var matches []fleet.Endpoint
		for _, e := range eps {
			if !e.IsDeleted && (strings.EqualFold(e.HostName, h) || strings.EqualFold(e.FQDN, h)) {
				matches = append(matches, e)
			}
		}
		switch {
		case len(matches) == 0:
			res.unresolved = append(res.unresolved, h+" (no exact host match)")
		case len(matches) > 1:
			where := make([]string, 0, len(matches))
			for _, m := range matches {
				where = append(where, idx[m.AccountID].Name+"/"+m.ID)
			}
			res.ambiguous = append(res.ambiguous, fmt.Sprintf("host %q matches %d endpoints (%s)", h, len(matches), strings.Join(where, ", ")))
			for _, m := range matches {
				add(m)
			}
		default:
			add(matches[0])
		}
	}
	byAccount := map[string][]endpointRef{}
	order := []string{}
	for _, r := range refs {
		if !inScope[r.AccountID] {
			res.unresolved = append(res.unresolved, r.AccountID+"/"+r.ID+" (account not in --site scope)")
			continue
		}
		if _, ok := byAccount[r.AccountID]; !ok {
			order = append(order, r.AccountID)
		}
		byAccount[r.AccountID] = append(byAccount[r.AccountID], r)
	}
	for _, acct := range order {
		raws, info, err := searchPages(ctx, c, "/v1/endpoints", map[string]any{"account_ids": []string{acct}}, "endpoints", maxPages)
		if err != nil {
			return res, err
		}
		eps, bad := parseEndpoints(raws)
		if info.Truncated {
			return res, fmt.Errorf("listing endpoints of account %s stopped at --max-scan-pages %d; raise --max-scan-pages", acct, maxPages)
		}
		if !info.Complete || bad > 0 {
			return res, fmt.Errorf("listing endpoints of account %s returned an incomplete or unreadable result (%d unreadable records); refusing to pick targets from it", acct, bad)
		}
		byID := map[string]fleet.Endpoint{}
		for _, e := range eps {
			byID[e.ID] = e
		}
		for _, r := range byAccount[acct] {
			e, ok := byID[r.ID]
			switch {
			case ok && !e.IsDeleted:
				add(e)
			default:
				res.unresolved = append(res.unresolved, acct+"/"+r.ID+" (not found in account)")
			}
		}
	}
	return res, nil
}

// waitForJobs polls each issued job until it reaches a final state or the
// timeout passes. A failed poll keeps the job pending and is retried; jobs
// whose last poll failed at the deadline are returned as an error.
func waitForJobs(ctx context.Context, c *client.Client, targets []respondTarget, timeout time.Duration, logw io.Writer) error {
	deadline := time.Now().Add(timeout)
	for {
		pending, failing := 0, 0
		for i := range targets {
			t := &targets[i]
			if t.JobID == "" || fleet.JobTerminal(t.JobState) {
				continue
			}
			data, err := c.GetNoCache(ctx, "/v1/accounts/"+t.AccountID+"/jobs/"+t.JobID, nil)
			if err != nil {
				t.JobState = "POLL_ERROR: " + err.Error()
				pending++
				failing++
				continue
			}
			var job struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(data, &job); err != nil || !fleet.KnownJobState(job.State) {
				t.JobState = "POLL_ERROR: unrecognized job state " + strconv.Quote(job.State)
				pending++
				failing++
				continue
			}
			t.JobState = job.State
			if !fleet.JobTerminal(t.JobState) {
				pending++
			}
		}
		if pending == 0 {
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			fmt.Fprintf(logw, "warning: %d job(s) not finished after %s; check later with: %s respond pending\n", pending, timeout, tdBinary)
			if failing > 0 {
				return apiErr(fmt.Errorf("could not poll %d job(s); see targets[].job_state", failing))
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func printRespond(cmd *cobra.Command, flags *rootFlags, view respondView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "reason", "job_id", "job_state", "unresolved", "errors", "note")
	}
	w := cmd.OutOrStdout()
	state := "PREVIEW"
	if view.Issued {
		state = "ISSUED"
	}
	fmt.Fprintf(w, "%s %s (%s): %d to issue, %d issued, %d skipped\n", state, view.Verb, view.Command, view.WouldIssue, view.IssuedCount, view.Skipped)
	if len(view.Targets) > 0 {
		if err := printColumns(w, view.Targets, "action", "site", "host_name", "online", "isolated", "last_seen", "job_id", "job_state", "reason"); err != nil {
			return err
		}
	}
	if len(view.Unresolved) > 0 {
		fmt.Fprintf(w, "not found: %s\n", strings.Join(view.Unresolved, ", "))
	}
	for _, e := range view.Errors {
		fmt.Fprintf(w, "rejected: %s: %s\n", e.Item, e.Error)
	}
	if view.Note != "" {
		fmt.Fprintln(w, view.Note)
	}
	return nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
