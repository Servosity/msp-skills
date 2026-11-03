// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/store"
)

// containTarget is one resolved endpoint.
type containTarget struct {
	Target          string `json:"target"`
	TenantID        string `json:"tenant_id"`
	TenantName      string `json:"tenant_name,omitempty"`
	EndpointID      string `json:"endpoint_id"`
	Hostname        string `json:"hostname"`
	IsolationBefore string `json:"isolation_before,omitempty"`
	Health          string `json:"health,omitempty"`
}

// containAction is one planned or executed API call.
type containAction struct {
	TenantID    string          `json:"tenant_id"`
	Action      string          `json:"action"` // isolate | release | scan | forensic_log | ack_alert
	EndpointIDs []string        `json:"endpoint_ids,omitempty"`
	AlertID     string          `json:"alert_id,omitempty"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Body        any             `json:"body"`
	Status      string          `json:"status"` // planned | ok | error
	Receipt     json.RawMessage `json:"receipt,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type containPlan struct {
	Mode    string          `json:"mode"` // preview | executed
	Targets []containTarget `json:"targets"`
	Actions []containAction `json:"actions"`
	Note    string          `json:"note,omitempty"`
}

func newNovelContainCmd(flags *rootFlags) *cobra.Command {
	var flagScan, flagForensic, flagAck, flagRelease bool
	var flagComment, dbPath string

	cmd := &cobra.Command{
		Use:   "contain [hostname|ip|endpoint-id]...",
		Short: "Contain endpoints by hostname, IP or ID without knowing the tenant: preview isolation plus optional scan, forensic log and alert acknowledgement, run only with --yes.",
		Long: `Use this command to contain one or more endpoints by hostname, IP or endpoint ID when you do not know (or do not want to look up) the tenant: it resolves the tenant, previews isolation plus optional scan, forensic log and alert acknowledgement, and runs them only with --yes. Do NOT use this command to toggle isolation on a known endpoint ID in the selected tenant; use 'endpoints set-isolation' instead.

Targets resolve from the endpoint index built by 'fleet refresh' (exact hostname, IPv4 address or endpoint ID). With no local match, --tenant <id> makes it search that tenant live. Ambiguous matches stop with the candidates listed. Every target is re-read live before the plan is shown. Without --yes nothing is changed. --release lifts isolation instead. Alerts are acknowledged only where the alert's allowedActions include acknowledge.`,
		Example: strings.Trim(`
  sophos-central-cli contain WS-FIN-07 --scan --forensic-log --ack-alerts
  sophos-central-cli contain 10.20.4.17 WS-FIN-08 --comment "IR-2291 ransomware triage" --yes
  sophos-central-cli contain WS-FIN-07 --release --yes`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":              "false",
			"mcp:read-only-unless-flags": "yes",
			"pp:data-source":             "live",
			"pp:preview-happy-path":      "true",
			"pp:happy-args":              "target=WS-FIN-07",
			"pp:typed-exit-codes":        "0,2,3",
			"pp:no-error-path-probe":     "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "contain")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("name at least one hostname, IP address or endpoint ID to contain"))
			}
			if flags.yes && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "isolate endpoints")
			}
			if len(flagComment) > 400 {
				return usageErr(fmt.Errorf("--comment is limited to 400 characters by Sophos"))
			}
			// No boundCtx: these runs span many tenants under the rate limit;
			// --timeout bounds each request inside the generated client.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			targets, err := resolveContainTargets(ctx, c, fleetDBPath(dbPath), args, filter)
			if err != nil {
				return err
			}
			plan, err := planContain(ctx, c, targets, containOptions{scan: flagScan, forensic: flagForensic, ack: flagAck, release: flagRelease, comment: flagComment})
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if flags.yes {
				executeContain(ctx, c, &plan)
			} else {
				plan.Note = "preview only: nothing was changed; rerun with --yes to execute"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), plan, flags); err != nil {
					return err
				}
			} else {
				rows := make([]map[string]any, 0, len(plan.Actions))
				for _, a := range plan.Actions {
					subject := strings.Join(a.EndpointIDs, ",")
					if a.AlertID != "" {
						subject = "alert " + a.AlertID
					}
					rows = append(rows, map[string]any{"tenant": a.TenantID, "action": a.Action, "subject": subject, "status": a.Status, "error": a.Error})
				}
				if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
					return err
				}
				if plan.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), plan.Note)
				}
			}
			for _, a := range plan.Actions {
				if a.Status == "error" {
					return apiErr(fmt.Errorf("one or more containment actions failed; see actions[].error"))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagScan, "scan", false, "Also start a scan on each endpoint")
	cmd.Flags().BoolVar(&flagForensic, "forensic-log", false, "Also request a forensic log from each endpoint")
	cmd.Flags().BoolVar(&flagAck, "ack-alerts", false, "Also acknowledge each endpoint's open alerts that allow it")
	cmd.Flags().BoolVar(&flagRelease, "release", false, "Lift isolation instead of isolating")
	cmd.Flags().StringVar(&flagComment, "comment", "Contained by sophos-central-cli", "Isolation comment and alert-acknowledgement message (max 400 chars)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database with the fleet endpoint index (default: resolved data directory data.db)")
	return cmd
}

type containOptions struct {
	scan, forensic, ack, release bool
	comment                      string
}

// indexedEndpoint is the slice of a stored endpoint the resolver matches on.
type indexedEndpoint struct {
	ID       string   `json:"id"`
	Hostname string   `json:"hostname"`
	IPv4     []string `json:"ipv4Addresses"`
	Tenant   struct {
		ID string `json:"id"`
	} `json:"tenant"`
}

// matchEndpoint reports whether an endpoint is the named target.
func matchEndpoint(e indexedEndpoint, target string) bool {
	if strings.EqualFold(e.ID, target) || strings.EqualFold(e.Hostname, target) {
		return true
	}
	for _, ip := range e.IPv4 {
		if ip == target {
			return true
		}
	}
	return false
}

func resolveContainTargets(ctx context.Context, c *client.Client, dbPath string, args []string, filter map[string]bool) ([]containTarget, error) {
	matches := map[string][]indexedEndpoint{}
	names := map[string]string{}
	db, exists, err := openFleetStore(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if exists {
		ledger, _ := loadFleetLedger(db)
		names = tenantNames(db, ledger)
		// A row we cannot read might be a second match, so an unreadable
		// index fails closed instead of making a target look unique.
		unreadable := 0
		scanErr := db.ListScan("endpoints", func(_ string, raw json.RawMessage) bool {
			var e indexedEndpoint
			if json.Unmarshal(raw, &e) != nil || e.ID == "" || e.Tenant.ID == "" {
				unreadable++
				return true
			}
			if filter != nil && !filter[e.Tenant.ID] {
				return true
			}
			for _, t := range args {
				if matchEndpoint(e, t) {
					matches[t] = append(matches[t], e)
				}
			}
			return true
		})
		_ = db.Close() // read-only store: nothing to flush
		if scanErr != nil {
			return nil, fmt.Errorf("reading the local endpoint index: %w", scanErr)
		}
		if unreadable > 0 {
			return nil, fmt.Errorf("the local endpoint index has %d unreadable endpoint row(s), so no match can be proven unique; run 'sophos-central-cli fleet refresh --resources tenants,endpoints' and retry", unreadable)
		}
	}
	var single string
	if len(filter) == 1 {
		for id := range filter {
			single = id
		}
	}
	var problems []string
	notFound := false
	var out []containTarget
	for _, t := range args {
		found := matches[t]
		if len(found) == 0 && single != "" {
			live, err := liveFindEndpoint(ctx, c, single, t)
			if err != nil {
				return nil, classifyAPIErrorOnly(err)
			}
			found = live
		}
		switch len(found) {
		case 0:
			notFound = true
			hint := "run 'sophos-central-cli fleet refresh --resources tenants,endpoints' or pass --tenant <tenant-id>"
			problems = append(problems, fmt.Sprintf("%s: no endpoint matched (%s)", t, hint))
		case 1:
			e := found[0]
			out = append(out, containTarget{Target: t, TenantID: e.Tenant.ID, TenantName: names[e.Tenant.ID], EndpointID: e.ID, Hostname: e.Hostname})
		default:
			var cands []string
			for _, e := range found {
				cands = append(cands, fmt.Sprintf("%s (endpoint %s, tenant %s)", e.Hostname, e.ID, e.Tenant.ID))
			}
			problems = append(problems, fmt.Sprintf("%s is ambiguous: %s; target the endpoint ID or add --tenant", t, strings.Join(cands, "; ")))
		}
	}
	if len(problems) > 0 {
		err := fmt.Errorf("cannot resolve containment targets:\n  %s", strings.Join(problems, "\n  "))
		if notFound && len(problems) == countPrefix(problems, "no endpoint matched") {
			return nil, notFoundErr(err)
		}
		return nil, usageErr(err)
	}
	return out, nil
}

func countPrefix(list []string, sub string) int {
	n := 0
	for _, s := range list {
		if strings.Contains(s, sub) {
			n++
		}
	}
	return n
}

// liveFindEndpoint searches one tenant for an exact hostname, IP or ID match.
func liveFindEndpoint(ctx context.Context, c *client.Client, tenantID, target string) ([]indexedEndpoint, error) {
	params := map[string]string{"pageSize": "500", "view": "full"} // basic omits ipv4Addresses
	switch {
	case store.IsUUID(target):
		params["ids"] = target
	case net.ParseIP(target) != nil:
		params["ipAddresses"] = target
	default:
		params["hostnameContains"] = target
	}
	// Exhaust the search: a capped page could hide a second exact match and
	// make an ambiguous hostname look unique.
	raws, err := fetchKeyPaged(ctx, c, "/endpoint/v1/endpoints", params, tenantID, 0)
	if err != nil {
		return nil, err
	}
	var out []indexedEndpoint
	for _, r := range raws {
		var e indexedEndpoint
		if json.Unmarshal(r, &e) != nil || e.ID == "" {
			return nil, fmt.Errorf("searching tenant %s for %s: an endpoint row could not be read, so no match can be proven unique", tenantID, target)
		}
		if matchEndpoint(e, target) {
			if e.Tenant.ID == "" {
				e.Tenant.ID = tenantID
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// planContain re-reads every target live and lays out the API calls.
func planContain(ctx context.Context, c *client.Client, targets []containTarget, opt containOptions) (containPlan, error) {
	plan := containPlan{Mode: "preview", Targets: targets, Actions: make([]containAction, 0)}
	byTenant := map[string][]string{}
	var tenantOrder []string
	for i := range plan.Targets {
		t := &plan.Targets[i]
		raw, err := c.GetWithHeadersNoCache(ctx, "/endpoint/v1/endpoints/"+url.PathEscape(t.EndpointID), nil, tenantHeaders(t.TenantID))
		if err != nil {
			return plan, fmt.Errorf("re-reading endpoint %s (%s): %w", t.EndpointID, t.Target, err)
		}
		var live map[string]any
		if err := json.Unmarshal(raw, &live); err != nil {
			return plan, fmt.Errorf("re-reading endpoint %s (%s): %w", t.EndpointID, t.Target, err)
		}
		if err := revalidateTarget(*t, live); err != nil {
			return plan, err
		}
		if h := jsonString(live, "hostname"); h != "" {
			t.Hostname = h
		}
		t.IsolationBefore = jsonString(live, "isolation", "status")
		t.Health = jsonString(live, "health", "overall")
		if _, ok := byTenant[t.TenantID]; !ok {
			tenantOrder = append(tenantOrder, t.TenantID)
		}
		byTenant[t.TenantID] = append(byTenant[t.TenantID], t.EndpointID)
	}
	for _, tid := range tenantOrder {
		ids := byTenant[tid]
		action := "isolate"
		if opt.release {
			action = "release"
		}
		// Sophos takes at most 500 IDs per bulk isolation request.
		for start := 0; start < len(ids); start += 500 {
			end := min(start+500, len(ids))
			plan.Actions = append(plan.Actions, containAction{TenantID: tid, Action: action, EndpointIDs: ids[start:end], Method: "POST", Path: "/endpoint/v1/endpoints/isolation",
				Body: map[string]any{"enabled": !opt.release, "ids": ids[start:end], "comment": opt.comment}, Status: "planned"})
		}
		for _, id := range ids {
			if opt.scan {
				plan.Actions = append(plan.Actions, containAction{TenantID: tid, Action: "scan", EndpointIDs: []string{id}, Method: "POST", Path: "/endpoint/v1/endpoints/" + id + "/scans", Body: map[string]any{}, Status: "planned"})
			}
			if opt.forensic {
				plan.Actions = append(plan.Actions, containAction{TenantID: tid, Action: "forensic_log", EndpointIDs: []string{id}, Method: "POST", Path: "/endpoint/v1/endpoints/" + id + "/forensic-logs", Body: map[string]any{}, Status: "planned"})
			}
		}
		if opt.ack {
			alerts, err := fetchKeyPaged(ctx, c, "/common/v1/alerts", map[string]string{"pageSize": "100"}, tid, 0)
			if err != nil {
				return plan, fmt.Errorf("listing alerts for tenant %s: %w", tid, err)
			}
			want := map[string]bool{}
			for _, id := range ids {
				want[id] = true
			}
			acks, err := ackableAlerts(alerts, want)
			if err != nil {
				return plan, fmt.Errorf("listing alerts for tenant %s: %w", tid, err)
			}
			for _, a := range acks {
				plan.Actions = append(plan.Actions, containAction{TenantID: tid, Action: "ack_alert", AlertID: a, Method: "POST", Path: "/common/v1/alerts/" + a + "/actions",
					Body: map[string]any{"action": "acknowledge", "message": opt.comment}, Status: "planned"})
			}
		}
	}
	return plan, nil
}

// revalidateTarget refuses to act when the live endpoint no longer matches
// what the operator named: a stale index can map a hostname or IP to a
// device that has since been renamed or re-addressed.
func revalidateTarget(t containTarget, live map[string]any) error {
	if id := jsonString(live, "id"); id != "" && !strings.EqualFold(id, t.EndpointID) {
		return fmt.Errorf("endpoint %s re-read returned a different endpoint (%s); refusing to act", t.EndpointID, id)
	}
	if tid := jsonString(live, "tenant", "id"); tid != "" && tid != t.TenantID {
		return fmt.Errorf("endpoint %s now belongs to tenant %s, not %s; run 'sophos-central-cli fleet refresh' and retry", t.EndpointID, tid, t.TenantID)
	}
	if strings.EqualFold(t.Target, t.EndpointID) {
		return nil
	}
	var ips []string
	if raw, ok := jsonPath(live, "ipv4Addresses").([]any); ok {
		for _, ip := range raw {
			if s, ok := ip.(string); ok {
				ips = append(ips, s)
			}
		}
	}
	e := indexedEndpoint{ID: t.EndpointID, Hostname: jsonString(live, "hostname"), IPv4: ips}
	if !matchEndpoint(e, t.Target) {
		return fmt.Errorf("%s no longer matches endpoint %s (now hostname %q, IPv4 %v); run 'sophos-central-cli fleet refresh' and retry", t.Target, t.EndpointID, e.Hostname, ips)
	}
	return nil
}

// ackableAlerts returns alert IDs raised on the given endpoints whose
// allowedActions include acknowledge.
// An unreadable alert might be on a target, so it fails the plan.
func ackableAlerts(alerts []json.RawMessage, endpoints map[string]bool) ([]string, error) {
	var out []string
	for _, raw := range alerts {
		var a struct {
			ID             string   `json:"id"`
			AllowedActions []string `json:"allowedActions"`
			ManagedAgent   struct {
				ID string `json:"id"`
			} `json:"managedAgent"`
		}
		if json.Unmarshal(raw, &a) != nil {
			return nil, errors.New("an alert row could not be read")
		}
		if !endpoints[a.ManagedAgent.ID] {
			continue
		}
		for _, act := range a.AllowedActions {
			if act == "acknowledge" {
				out = append(out, a.ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// bulkIsolationErrors reads a 202 bulk-isolation receipt. Sophos accepts the
// request but lists per-endpoint rejections under "errors"; any rejection
// means containment did not fully land.
func bulkIsolationErrors(raw json.RawMessage, requested []string) string {
	var r struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Errors []struct {
			ID           string `json:"id"`
			ErrorCode    string `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "isolation receipt could not be read; containment not confirmed: " + err.Error()
	}
	parts := make([]string, 0, len(r.Errors))
	answered := map[string]bool{}
	for _, it := range r.Items {
		answered[it.ID] = true
	}
	for _, e := range r.Errors {
		answered[e.ID] = true
		parts = append(parts, fmt.Sprintf("%s: %s %s", e.ID, e.ErrorCode, e.ErrorMessage))
	}
	// Every requested endpoint must be accounted for, accepted or rejected.
	for _, id := range requested {
		if !answered[id] {
			parts = append(parts, id+": not confirmed in the receipt")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "isolation not confirmed: " + strings.Join(parts, "; ")
}

// executeContain runs each planned action, recording receipts and errors. A
// failed action does not stop the rest: partial containment beats none.
func executeContain(ctx context.Context, c *client.Client, plan *containPlan) {
	plan.Mode = "executed"
	for i := range plan.Actions {
		a := &plan.Actions[i]
		raw, _, err := c.PostWithHeaders(ctx, a.Path, a.Body, tenantHeaders(a.TenantID))
		if err != nil {
			a.Status, a.Error = "error", truncateErr(err)
			continue
		}
		a.Status = "ok"
		if len(raw) > 0 && json.Valid(raw) {
			a.Receipt = raw
		}
		if a.Action == "isolate" || a.Action == "release" {
			if msg := bulkIsolationErrors(raw, a.EndpointIDs); msg != "" {
				a.Status, a.Error = "error", msg
			}
		}
	}
}
