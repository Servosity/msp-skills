// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"saas-alerts-pp-cli/internal/client"
	"saas-alerts-pp-cli/internal/cliutil"
)

const (
	shWebhookBase    = "/outgoingWebhookApi/api/v1"
	shSubsPath       = shWebhookBase + "/subscriptions"
	shDisabledPath   = shWebhookBase + "/subscriptions/disabled"
	shQueuePath      = shWebhookBase + "/queue"
	shDomainsPath    = shWebhookBase + "/domains"
	shTestEventPath  = shWebhookBase + "/misc/sendTestEvent"
	shCustomerIDsMax = 50

	shFlagFiltersEmpty    = "FILTERS_EMPTY"
	shFlagFiltersBoth     = "FILTERS_BOTH"
	shFlagCustomerIDsOver = "CUSTOMER_IDS_OVER_50"
	shFlagDisabled        = "DISABLED"
	shFlagUnverified      = "UNVERIFIED_DOMAIN"
	shFlagBacklog         = "QUEUE_BACKLOG"
	// The queue depth or domain verification could not be read, so the
	// corresponding check was not evaluated for this subscription.
	shFlagQueueUnknown  = "QUEUE_UNKNOWN"
	shFlagDomainUnknown = "DOMAIN_UNKNOWN"
)

// shSubscription is the Webhooks SSubscription subset this command reads.
// eventTypes can carry nulls (the spec example is [null]), so list fields
// decode raw and keep only non-empty strings.
type shSubscription struct {
	ChannelID      string            `json:"channelId"`
	URL            string            `json:"url"`
	PartnerID      string            `json:"partnerId"`
	Enabled        *bool             `json:"enabled"`
	DisabledTime   any               `json:"disabledTime"`
	AlertStatuses  []json.RawMessage `json:"alertStatuses"`
	EventTypes     []json.RawMessage `json:"eventTypes"`
	CustomerIDs    []json.RawMessage `json:"customerIds"`
	SkipSuppressed *bool             `json:"skipSuppressed"`
	FailedAttempts []struct {
		Message string `json:"message"`
	} `json:"failedAttempts"`
}

func shStrings(raws []json.RawMessage) []string {
	out := make([]string, 0, len(raws))
	for _, r := range raws {
		var s string
		if json.Unmarshal(r, &s) == nil && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

type shFilterCounts struct {
	AlertStatuses int `json:"alert_statuses"`
	EventTypes    int `json:"event_types"`
	CustomerIDs   int `json:"customer_ids"`
}

type shRow struct {
	ChannelID        string         `json:"channel_id"`
	URL              string         `json:"url"`
	Enabled          bool           `json:"enabled"`
	DisabledTime     string         `json:"disabled_time"`
	UnsentQueueDepth *int           `json:"unsent_queue_depth"`
	Domain           string         `json:"domain"`
	DomainVerified   *bool          `json:"domain_verified"`
	Filters          shFilterCounts `json:"filters"`
	AlertStatuses    []string       `json:"alert_statuses"`
	EventTypes       []string       `json:"event_types"`
	FailedAttempts   int            `json:"failed_attempts"`
	// The failure message itself is not copied (it can echo request data);
	// only the length of the latest one is reported, 0 when there is none.
	LastFailureLength int      `json:"last_failure_message_length"`
	Flags             []string `json:"flags"`

	partnerID      string
	customerIDs    []string
	skipSuppressed *bool
}

func (r shRow) hasFlag(f string) bool {
	for _, x := range r.Flags {
		if x == f {
			return true
		}
	}
	return false
}

type shRequest struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Body    any    `json:"body"`
	Purpose string `json:"purpose"`
}

type shPlanItem struct {
	ChannelID string      `json:"channel_id"`
	URL       string      `json:"url"`
	Planned   bool        `json:"planned"`
	Reason    string      `json:"reason"`
	Requests  []shRequest `json:"requests"`

	// patch and testBody are what --confirm executes. testBody is kept for
	// every planned channel, even when its preview shares another channel's
	// test event, so a failed PATCH elsewhere cannot drop the event.
	patch    *shRequest
	testBody map[string]any
}

type shExecuted struct {
	ChannelID string   `json:"channel_id"`
	Method    string   `json:"method"`
	Path      string   `json:"path"`
	Purpose   string   `json:"purpose"`
	Channels  []string `json:"channels"`
	OK        bool     `json:"ok"`
	// Outcome is reenabled / reenable_unverified / error for a PATCH, and
	// delivered / failed / error for a test event.
	Outcome string `json:"outcome"`
	// Verification says what established a re-enable: the PATCH answer, a
	// re-read of the subscription lists, or nothing.
	Verification string `json:"verification"`
	HTTPStatus   int    `json:"http_status"`
	Error        string `json:"error"`
	// DeliveryErrors carries the IWhError entries a test event returned.
	DeliveryErrors []shDeliveryError `json:"delivery_errors"`
}

// shDeliveryError is one IWhError from a test event. The message text is
// not copied (response bodies can echo credentials); only its length is.
type shDeliveryError struct {
	ChannelID     string `json:"channel_id"`
	MessageLength int    `json:"message_length"`
}

type shSummary struct {
	Total       int `json:"total"`
	Enabled     int `json:"enabled"`
	Disabled    int `json:"disabled"`
	Backlog     int `json:"backlog"`
	UnsentPacks int `json:"unsent_packs"`
	Flagged     int `json:"flagged"`
	// QueueUnknown and DomainUnknown count subscriptions whose queue depth
	// or domain verification could not be read.
	QueueUnknown  int `json:"queue_unknown"`
	DomainUnknown int `json:"domain_unknown"`
}

type shResult struct {
	Subscriptions []shRow   `json:"subscriptions"`
	Summary       shSummary `json:"summary"`
	QueueSource   string    `json:"queue_source"`
	Warnings      []string  `json:"warnings"`
	// CollectionIncomplete lists responses with members that were not valid
	// objects; their checks are partial and the run exits non-zero.
	CollectionIncomplete []shIncomplete `json:"collection_incomplete"`
	Plan                 *[]shPlanItem  `json:"plan,omitempty"`
	Executed             *[]shExecuted  `json:"executed,omitempty"`
	Note                 string         `json:"note,omitempty"`
}

type shIncomplete struct {
	Collection string `json:"collection"`
	Valid      int    `json:"valid"`
	Invalid    int    `json:"invalid"`
}

func newNovelSubscriptionsHealthCmd(flags *rootFlags) *cobra.Command {
	var flagReenable bool
	var flagConfirm bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check every webhook subscription's state, unsent queue, and domain, flag the update trap",
		Long: `Use this command for outbound webhook feed health. Do NOT use this command for per-customer monitoring or Respond connection gaps; use 'coverage' instead.

Read-only by default. --reenable adds a plan to re-enable disabled subscriptions and only previews it; re-enabling needs --reenable --confirm. Only disabled subscriptions on a verified domain are re-enabled (each with its existing filter fields, then a matching test event); ones with no filter, both filter kinds, or an unverified domain are listed with the reason they were skipped.`,
		Example: strings.Trim(`
  saas-alerts-cli subscriptions health --agent
  saas-alerts-cli subscriptions health --reenable --json
  saas-alerts-cli subscriptions health --reenable --confirm`, "\n"),
		Annotations: map[string]string{
			"pp:data-source":             "live",
			"pp:preview-happy-path":      "true",
			"pp:happy-args":              "--reenable",
			"mcp:read-only-unless-flags": "confirm",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "subscriptions health")
			}
			if flagConfirm && !flagReenable {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--confirm only applies with --reenable"))
			}
			if flagConfirm && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "subscriptions health --reenable --confirm")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			result, err := shCollect(ctx, c)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if flagReenable {
				plan := shPlan(result.Subscriptions)
				result.Plan = &plan
				if !flagConfirm {
					result.Note = "preview only: nothing was changed; rerun with --reenable --confirm to apply the plan"
				}
			}
			failed := 0
			var firstErr error
			if flagReenable && flagConfirm {
				var executed []shExecuted
				known := map[string]bool{}
				for _, r := range result.Subscriptions {
					known[r.ChannelID] = true
				}
				executed, failed, firstErr = shExecutePlan(ctx, c, *result.Plan, known)
				result.Executed = &executed
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), result, flags); err != nil {
					return err
				}
			} else if err := shPrintHuman(cmd, result); err != nil {
				return err
			}
			if failed > 0 {
				if failed == len(*result.Executed) && firstErr != nil && len(result.CollectionIncomplete) == 0 {
					return classifyAPIErrorOnly(firstErr)
				}
				return partialFailureErr(fmt.Errorf("%d of %d re-enable requests failed", failed, len(*result.Executed)))
			}
			if n := len(result.CollectionIncomplete); n > 0 {
				return apiErr(fmt.Errorf("%d response collection(s) had members that are not valid objects (see collection_incomplete); the health report is partial", n))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagReenable, "reenable", false, "Plan re-enabling each disabled subscription on a verified domain (PATCH enabled:true with its filters, then a test event)")
	cmd.Flags().BoolVar(&flagConfirm, "confirm", false, "With --reenable, send the planned requests; without it the plan is only previewed")
	return cmd
}

// shDecodeSubs decodes a subscription list. Every member must be an object
// that decodes as a subscription with a non-empty channelId; the others are
// counted as invalid (never silently dropped).
func shDecodeSubs(data json.RawMessage, path string) ([]shSubscription, int, error) {
	var raws []json.RawMessage
	if !rqIsJSONArray(data) || json.Unmarshal(data, &raws) != nil {
		return nil, 0, fmt.Errorf("unexpected GET %s response: expected an array, got %s", path, rqDescribeJSON(data))
	}
	subs := make([]shSubscription, 0, len(raws))
	invalid := 0
	for _, raw := range raws {
		var sub shSubscription
		if !rqIsJSONObject(raw) || json.Unmarshal(raw, &sub) != nil || strings.TrimSpace(sub.ChannelID) == "" {
			invalid++
			continue
		}
		// A row carrying a failure indicator proves nothing about its
		// channel: it counts as invalid, so the list is incomplete.
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || rqFailureIndicator(row, false) != "" {
			invalid++
			continue
		}
		subs = append(subs, sub)
	}
	return subs, invalid, nil
}

// shCollect reads subscriptions (enabled and disabled), verified domains and
// the unsent queue, then builds one row per channel with lint flags.
// Subscriptions are required; domains and queue degrade to warnings.
func shCollect(ctx context.Context, c *client.Client) (shResult, error) {
	result := shResult{Subscriptions: make([]shRow, 0), Warnings: make([]string, 0)}
	data, err := c.GetNoCache(ctx, shSubsPath, nil)
	if err != nil {
		return result, err
	}
	result.CollectionIncomplete = make([]shIncomplete, 0)
	enabled, invalid, err := shDecodeSubs(data, shSubsPath)
	if err != nil {
		return result, err
	}
	if invalid > 0 {
		result.CollectionIncomplete = append(result.CollectionIncomplete, shIncomplete{Collection: "subscriptions", Valid: len(enabled), Invalid: invalid})
	}
	data, err = c.GetNoCache(ctx, shDisabledPath, nil)
	if err != nil {
		return result, err
	}
	disabled, invalid, err := shDecodeSubs(data, shDisabledPath)
	if err != nil {
		return result, err
	}
	if invalid > 0 {
		result.CollectionIncomplete = append(result.CollectionIncomplete, shIncomplete{Collection: "subscriptions/disabled", Valid: len(disabled), Invalid: invalid})
	}

	rows := make([]shRow, 0, len(enabled)+len(disabled))
	index := map[string]int{}
	add := func(s shSubscription, fromDisabledList bool) {
		if s.ChannelID == "" {
			return
		}
		isEnabled := !fromDisabledList
		if s.Enabled != nil {
			isEnabled = *s.Enabled && !fromDisabledList
		}
		alert, events := shStrings(s.AlertStatuses), shStrings(s.EventTypes)
		customers := shStrings(s.CustomerIDs)
		row := shRow{
			ChannelID:      s.ChannelID,
			URL:            s.URL,
			Enabled:        isEnabled,
			Filters:        shFilterCounts{AlertStatuses: len(alert), EventTypes: len(events), CustomerIDs: len(customers)},
			AlertStatuses:  alert,
			EventTypes:     events,
			FailedAttempts: len(s.FailedAttempts),
			Flags:          make([]string, 0),
			partnerID:      s.PartnerID,
			customerIDs:    customers,
			skipSuppressed: s.SkipSuppressed,
		}
		if s.DisabledTime != nil {
			row.DisabledTime = containScalar(s.DisabledTime)
		}
		if n := len(s.FailedAttempts); n > 0 {
			row.LastFailureLength = len(s.FailedAttempts[n-1].Message)
		}
		if i, ok := index[s.ChannelID]; ok {
			// Listed in both: the disabled list wins.
			if fromDisabledList {
				rows[i] = row
			}
			return
		}
		index[s.ChannelID] = len(rows)
		rows = append(rows, row)
	}
	for _, s := range enabled {
		add(s, false)
	}
	for _, s := range disabled {
		add(s, true)
	}

	domains, domInvalid, domErr := shFetchDomains(ctx, c)
	if domErr == nil && domInvalid > 0 {
		// A partial domain list could mark a verified URL unverified, so
		// verification is left unknown instead.
		result.CollectionIncomplete = append(result.CollectionIncomplete, shIncomplete{Collection: "domains", Valid: len(domains), Invalid: domInvalid})
		domErr = fmt.Errorf("%d of %d domain entries are not valid objects with a domain", domInvalid, domInvalid+len(domains))
	}
	if domErr != nil {
		result.Warnings = append(result.Warnings, "domains unavailable, UNVERIFIED_DOMAIN not evaluated: "+domErr.Error())
	}
	depths, queueSource, qWarn, qIncomplete := shQueueDepths(ctx, c, rows)
	result.QueueSource = queueSource
	result.Warnings = append(result.Warnings, qWarn...)
	result.CollectionIncomplete = append(result.CollectionIncomplete, qIncomplete...)

	for i := range rows {
		r := &rows[i]
		if domErr == nil {
			d, ok := shMatchDomain(r.URL, domains)
			verified := ok
			r.DomainVerified = &verified
			r.Domain = d
		}
		if d, ok := depths[r.ChannelID]; ok {
			depth := d
			r.UnsentQueueDepth = &depth
		}
		// FILTERS_EMPTY: PATCH without alertStatuses or eventTypes returns 500
		// (community report). FILTERS_BOTH: the spec says they cannot be combined.
		if r.Filters.AlertStatuses == 0 && r.Filters.EventTypes == 0 {
			r.Flags = append(r.Flags, shFlagFiltersEmpty)
		}
		if r.Filters.AlertStatuses > 0 && r.Filters.EventTypes > 0 {
			r.Flags = append(r.Flags, shFlagFiltersBoth)
		}
		if r.Filters.CustomerIDs > shCustomerIDsMax {
			r.Flags = append(r.Flags, shFlagCustomerIDsOver)
		}
		if !r.Enabled {
			r.Flags = append(r.Flags, shFlagDisabled)
		}
		switch {
		case r.DomainVerified == nil:
			r.Flags = append(r.Flags, shFlagDomainUnknown)
		case !*r.DomainVerified:
			r.Flags = append(r.Flags, shFlagUnverified)
		}
		switch {
		case r.UnsentQueueDepth == nil:
			r.Flags = append(r.Flags, shFlagQueueUnknown)
		case *r.UnsentQueueDepth > 0:
			r.Flags = append(r.Flags, shFlagBacklog)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Enabled != rows[j].Enabled {
			return !rows[i].Enabled
		}
		if len(rows[i].Flags) != len(rows[j].Flags) {
			return len(rows[i].Flags) > len(rows[j].Flags)
		}
		return rows[i].ChannelID < rows[j].ChannelID
	})
	result.Subscriptions = rows
	for _, r := range rows {
		result.Summary.Total++
		if r.Enabled {
			result.Summary.Enabled++
		} else {
			result.Summary.Disabled++
		}
		if r.UnsentQueueDepth != nil && *r.UnsentQueueDepth > 0 {
			result.Summary.Backlog++
			result.Summary.UnsentPacks += *r.UnsentQueueDepth
		}
		if len(r.Flags) > 0 {
			result.Summary.Flagged++
		}
		if r.UnsentQueueDepth == nil {
			result.Summary.QueueUnknown++
		}
		if r.DomainVerified == nil {
			result.Summary.DomainUnknown++
		}
	}
	return result, nil
}

// shFetchDomains reads the verified domains. null must not read as "no
// verified domains", so a real array is required; members that are not
// objects with a non-empty domain, or that carry a failure indicator, are
// counted as invalid.
func shFetchDomains(ctx context.Context, c *client.Client) ([]string, int, error) {
	data, err := c.GetNoCache(ctx, shDomainsPath, nil)
	if err != nil {
		return nil, 0, err
	}
	var raws []json.RawMessage
	if !rqIsJSONArray(data) || json.Unmarshal(data, &raws) != nil {
		return nil, 0, fmt.Errorf("unexpected GET %s response: expected an array, got %s", shDomainsPath, rqDescribeJSON(data))
	}
	out := make([]string, 0, len(raws))
	invalid := 0
	for _, raw := range raws {
		var item struct {
			Domain string `json:"domain"`
		}
		if !rqIsJSONObject(raw) || json.Unmarshal(raw, &item) != nil || strings.TrimSpace(item.Domain) == "" {
			invalid++
			continue
		}
		// A domain row carrying a failure indicator is not a verified
		// domain; the Domain schema has no status of its own, so the whole
		// rule applies. Counting it invalid leaves verification unknown.
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || rqFailureIndicator(row, false) != "" {
			invalid++
			continue
		}
		out = append(out, item.Domain)
	}
	return out, invalid, nil
}

// shMatchDomain reports whether a subscription URL sits under a verified
// domain. Domain values look like "example.com/webhook" (spec example), with
// or without a scheme: the host must match and, when the domain carries a
// path, the URL path must start with it (INFERRED matching rule).
func shMatchDomain(rawURL string, domains []string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range domains {
		norm := strings.TrimSpace(d)
		if i := strings.Index(norm, "://"); i >= 0 {
			norm = norm[i+3:]
		}
		dHost, dPath, _ := strings.Cut(norm, "/")
		if h, _, err := net.SplitHostPort(dHost); err == nil {
			dHost = h
		}
		if !strings.EqualFold(dHost, host) {
			continue
		}
		dPath = strings.Trim(dPath, "/")
		if dPath == "" || strings.HasPrefix(strings.Trim(u.Path, "/"), dPath) {
			return d, true
		}
	}
	return "", false
}

// shQueueDepths counts unsent queue packs per channel. GET /queue items carry
// unsentChannelIds; when any item lacks it, the per-channel endpoint
// GET /queue/channels/{channelId} is asked instead.
func shQueueDepths(ctx context.Context, c *client.Client, rows []shRow) (map[string]int, string, []string, []shIncomplete) {
	depths := map[string]int{}
	warnings := make([]string, 0)
	incomplete := make([]shIncomplete, 0)
	data, err := c.GetNoCache(ctx, shQueuePath, nil)
	if err != nil {
		return depths, "unavailable", append(warnings, "queue unavailable, unsent_queue_depth not evaluated: "+err.Error()), incomplete
	}
	// null must not read as an empty (healthy) queue: require a real array
	// whose members are all objects.
	var raws []json.RawMessage
	if !rqIsJSONArray(data) || json.Unmarshal(data, &raws) != nil {
		return depths, "unavailable", append(warnings, fmt.Sprintf("queue unavailable, GET %s returned %s instead of an array", shQueuePath, rqDescribeJSON(data))), incomplete
	}
	type queueItem struct {
		UnsentChannelIDs *[]string `json:"unsentChannelIds"`
	}
	items := make([]queueItem, 0, len(raws))
	invalid := 0
	for _, raw := range raws {
		var it queueItem
		if !rqIsJSONObject(raw) || json.Unmarshal(raw, &it) != nil || shQueueMemberFailure(raw) {
			invalid++
			continue
		}
		items = append(items, it)
	}
	if invalid > 0 {
		incomplete = append(incomplete, shIncomplete{Collection: "queue", Valid: len(items), Invalid: invalid})
		return depths, "unavailable", append(warnings, fmt.Sprintf("queue unavailable, %d of %d GET %s entries are not objects", invalid, len(raws), shQueuePath)), incomplete
	}
	allTagged := true
	for _, it := range items {
		if it.UnsentChannelIDs == nil {
			allTagged = false
			break
		}
	}
	if allTagged {
		for _, r := range rows {
			depths[r.ChannelID] = 0
		}
		for _, it := range items {
			seen := map[string]bool{}
			for _, ch := range *it.UnsentChannelIDs {
				if _, tracked := depths[ch]; tracked && !seen[ch] {
					depths[ch]++
					seen[ch] = true
				}
			}
		}
		return depths, "queue", warnings, incomplete
	}
	for _, r := range rows {
		path := shWebhookBase + "/queue/channels/" + url.PathEscape(r.ChannelID)
		data, err := c.GetNoCache(ctx, path, nil)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("queue for channel %s unavailable: %v", r.ChannelID, err))
			continue
		}
		var packs []json.RawMessage
		if !rqIsJSONArray(data) || json.Unmarshal(data, &packs) != nil {
			warnings = append(warnings, fmt.Sprintf("queue for channel %s is not an array (got %s)", r.ChannelID, rqDescribeJSON(data)))
			continue
		}
		bad := 0
		for _, p := range packs {
			if !rqIsJSONObject(p) || shQueueMemberFailure(p) {
				bad++
			}
		}
		if bad > 0 {
			incomplete = append(incomplete, shIncomplete{Collection: "queue/channels/" + r.ChannelID, Valid: len(packs) - bad, Invalid: bad})
			warnings = append(warnings, fmt.Sprintf("queue for channel %s has %d entries that are not objects", r.ChannelID, bad))
			continue
		}
		depths[r.ChannelID] = len(packs)
	}
	return depths, "queue/channels", warnings, incomplete
}

// shQueueMemberFailure applies the shared failure rule to one queue member
// (SSubscriptionQueueItem has no status of its own, so the full rule
// applies). A flagged member is not an unsent pack and does not count toward
// a depth; it makes the queue collection incomplete instead.
func shQueueMemberFailure(raw json.RawMessage) bool {
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) != nil || rqFailureIndicator(obj, false) != ""
}

// shPlan builds the --reenable plan: for each disabled subscription on a
// verified domain, PATCH enabled:true carrying its existing filter fields
// (a PATCH without them hits the 500 trap), then a test event shaped to pass
// its filter. The test event is not channel-scoped (WebhookTestParams has no
// channelId), so identical test events are planned once.
func shPlan(rows []shRow) []shPlanItem {
	plan := make([]shPlanItem, 0)
	plannedTests := map[string]string{}
	for _, r := range rows {
		if r.Enabled {
			continue
		}
		item := shPlanItem{ChannelID: r.ChannelID, URL: r.URL, Requests: make([]shRequest, 0)}
		switch {
		case r.hasFlag(shFlagFiltersEmpty):
			item.Reason = "FILTERS_EMPTY: a PATCH without alertStatuses or eventTypes returns HTTP 500; set one filter with 'subscriptions update' first"
		case r.hasFlag(shFlagFiltersBoth):
			item.Reason = "FILTERS_BOTH: alertStatuses and eventTypes cannot be combined; keep one with 'subscriptions update' first"
		case r.DomainVerified == nil:
			item.Reason = "DOMAIN_UNKNOWN: domain verification could not be read, so it is not re-enabled; re-run when GET /domains answers"
		case !*r.DomainVerified:
			item.Reason = "UNVERIFIED_DOMAIN: verify the destination domain with 'webhook-domains' before re-enabling"
		default:
			body := map[string]any{"enabled": true}
			test := map[string]any{}
			if len(r.AlertStatuses) > 0 {
				body["alertStatuses"] = r.AlertStatuses
				test["alertStatus"] = r.AlertStatuses[0]
			} else {
				body["eventTypes"] = r.EventTypes
				test["jointType"] = r.EventTypes[0]
			}
			if len(r.customerIDs) > 0 {
				body["customerIds"] = r.customerIDs
			}
			if r.skipSuppressed != nil {
				body["skipSuppressed"] = *r.skipSuppressed
			}
			if r.partnerID != "" {
				test["partnerId"] = r.partnerID
			}
			item.Planned = true
			item.Reason = "disabled on a verified domain"
			patch := shRequest{
				Method: "PATCH", Path: shSubsPath + "/" + url.PathEscape(r.ChannelID), Body: body,
				Purpose: "re-enable with the existing filter fields",
			}
			item.patch = &patch
			item.testBody = test
			item.Requests = append(item.Requests, patch)
			key, _ := json.Marshal(test)
			if first, dup := plannedTests[string(key)]; dup {
				item.Reason += "; shares the test event planned for channel " + first + " (sent once for whichever of them re-enables)"
			} else {
				plannedTests[string(key)] = r.ChannelID
				item.Requests = append(item.Requests, shRequest{
					Method: "POST", Path: shTestEventPath, Body: test,
					Purpose: "send a test event matching the subscription filter (delivered to every matching subscription)",
				})
			}
		}
		plan = append(plan, item)
	}
	return plan
}

// shExecutePlan runs every planned PATCH first, then sends each distinct test
// event once, only for channels whose PATCH succeeded. It returns the
// executed requests, the number that failed, and the first error.
func shExecutePlan(ctx context.Context, c *client.Client, plan []shPlanItem, knownChannels map[string]bool) ([]shExecuted, int, error) {
	executed := make([]shExecuted, 0)
	failed := 0
	var firstErr error
	fail := func(ex *shExecuted, err error) {
		failed++
		if firstErr == nil {
			firstErr = err
		}
		ex.Error = err.Error()
	}

	reenabled := make([]shPlanItem, 0)
	for _, item := range plan {
		if !item.Planned || item.patch == nil {
			continue
		}
		ex := shExecuted{ChannelID: item.ChannelID, Method: item.patch.Method, Path: item.patch.Path, Purpose: item.patch.Purpose,
			Channels: []string{item.ChannelID}, DeliveryErrors: make([]shDeliveryError, 0)}
		data, code, err := c.Patch(ctx, item.patch.Path, item.patch.Body)
		ex.HTTPStatus = code
		if err != nil {
			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				ex.HTTPStatus = apiErr.StatusCode
			}
			ex.Outcome = "error"
			fail(&ex, err)
			executed = append(executed, ex)
			continue
		}
		why := shPatchAnswerShowsEnabled(data, item.ChannelID)
		if why == "" {
			ex.OK, ex.Outcome, ex.Verification = true, "reenabled", "answer"
			reenabled = append(reenabled, item)
			executed = append(executed, ex)
			continue
		}
		// The answer does not show the subscription enabled: read the lists.
		enabled, rerr := shReadEnabled(ctx, c, item.ChannelID)
		switch {
		case rerr == nil && enabled:
			ex.OK, ex.Outcome, ex.Verification = true, "reenabled", "read (the PATCH answer was "+why+")"
			reenabled = append(reenabled, item)
		case rerr != nil:
			// A failed or incomplete re-read establishes nothing.
			ex.Outcome, ex.Verification = "reenable_unverified", "read_failed"
			fail(&ex, fmt.Errorf("the PATCH answered %s and re-reading the subscriptions failed (%v); not retried, no test event sent", why, rerr))
		default:
			ex.Outcome, ex.Verification = "reenable_unverified", "read"
			fail(&ex, fmt.Errorf("the PATCH answered %s and a re-read does not show the subscription enabled; not retried, no test event sent", why))
		}
		executed = append(executed, ex)
	}

	// Group the re-enabled channels by identical test event, in plan order.
	type group struct {
		body     map[string]any
		channels []string
	}
	groups := make([]*group, 0)
	byKey := map[string]*group{}
	for _, item := range reenabled {
		if item.testBody == nil {
			continue
		}
		key, _ := json.Marshal(item.testBody)
		g, ok := byKey[string(key)]
		if !ok {
			g = &group{body: item.testBody}
			byKey[string(key)] = g
			groups = append(groups, g)
		}
		g.channels = append(g.channels, item.ChannelID)
	}
	for _, g := range groups {
		ex := shExecuted{ChannelID: g.channels[0], Method: "POST", Path: shTestEventPath,
			Purpose: "send a test event matching the re-enabled subscription filter", Channels: g.channels, DeliveryErrors: make([]shDeliveryError, 0)}
		data, code, err := c.Post(ctx, shTestEventPath, g.body)
		ex.HTTPStatus = code
		if err != nil {
			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				ex.HTTPStatus = apiErr.StatusCode
			}
			fail(&ex, err)
			ex.Outcome = "error"
		} else if ok, detail, derrs := shTestEventOutcome(data, knownChannels); ok {
			ex.OK, ex.Outcome = true, "delivered"
		} else {
			ex.Outcome = "failed"
			ex.DeliveryErrors = append(ex.DeliveryErrors, derrs...)
			fail(&ex, errors.New(detail))
		}
		executed = append(executed, ex)
	}
	return executed, failed, firstErr
}

// shPatchAnswerShowsEnabled checks a PATCH /subscriptions/{channelId} answer
// (the spec example is the updated subscription): an object that passes the
// shared failure rule, a channelId equal to the one patched when present, and a literal
// enabled:true. It returns a value-free description of what was missing, or
// "" when the answer shows the subscription enabled.
func shPatchAnswerShowsEnabled(data json.RawMessage, channelID string) string {
	var obj map[string]json.RawMessage
	if !rqIsJSONObject(data) || json.Unmarshal(data, &obj) != nil {
		return "not a subscription object (" + rqDescribeJSON(data) + ")"
	}
	// The full shared failure rule: a subscription has no status field of
	// its own, so a status here is a failure signal like the rest.
	if why := rqFailureIndicator(obj, false); why != "" {
		return "a subscription carrying " + why
	}
	if raw, ok := obj["channelId"]; ok {
		var got string
		if json.Unmarshal(raw, &got) != nil || got != channelID {
			return "a different subscription (channelId mismatch)"
		}
	}
	if string(bytes.TrimSpace(obj["enabled"])) != "true" {
		return "a subscription without enabled:true (" + rqDescribeJSON(data) + ")"
	}
	return ""
}

// shReadEnabled re-reads the subscription lists to establish whether a
// channel is now enabled: listed by GET /subscriptions (not enabled:false)
// and not listed by GET /subscriptions/disabled. If either list has members
// that are not valid subscriptions, the target could be one of them (an
// unreadable disabled entry would otherwise vanish and let the active list
// "prove" enabled), so the state is not established and an error says why.
func shReadEnabled(ctx context.Context, c *client.Client, channelID string) (bool, error) {
	find := func(path string) (*shSubscription, error) {
		data, err := c.GetNoCache(ctx, path, nil)
		if err != nil {
			return nil, err
		}
		subs, invalid, err := shDecodeSubs(data, path)
		if err != nil {
			return nil, err
		}
		if invalid > 0 {
			return nil, fmt.Errorf("GET %s returned %d member(s) that are not valid subscriptions", path, invalid)
		}
		for i := range subs {
			if subs[i].ChannelID == channelID {
				return &subs[i], nil
			}
		}
		return nil, nil
	}
	disabled, err := find(shDisabledPath)
	if err != nil {
		return false, err
	}
	if disabled != nil {
		return false, nil
	}
	sub, err := find(shSubsPath)
	if err != nil {
		return false, err
	}
	return sub != nil && (sub.Enabled == nil || *sub.Enabled), nil
}

// shTestEventOutcome reads a POST /misc/sendTestEvent 200 body, which the
// spec types as anyOf string | boolean | IWhError[] ({channelId, message}).
// true, an empty error list, or a non-error string mean delivered; false, an
// error-looking string, or any IWhError mean the delivery failed.
func shTestEventOutcome(data json.RawMessage, knownChannels map[string]bool) (bool, string, []shDeliveryError) {
	var b bool
	if json.Unmarshal(data, &b) == nil {
		if b {
			return true, "", nil
		}
		return false, "test event was not delivered (the API answered false)", nil
	}
	var str string
	if json.Unmarshal(data, &str) == nil {
		switch shClassifyTestEventString(str) {
		case "ok":
			return true, "", nil
		case "failed":
			return false, fmt.Sprintf("test event was not delivered (the API answered with an error string of %d characters)", len(str)), nil
		default:
			return false, fmt.Sprintf("test event outcome unknown (the API answered with an unrecognized string of %d characters)", len(str)), nil
		}
	}
	var raw []struct {
		ChannelID string `json:"channelId"`
		Message   string `json:"message"`
	}
	if rqIsJSONArray(data) && json.Unmarshal(data, &raw) == nil {
		whErrors := make([]shDeliveryError, 0, len(raw))
		for _, e := range raw {
			whErrors = append(whErrors, shDeliveryError{ChannelID: shSafeChannelID(e.ChannelID, knownChannels), MessageLength: len(e.Message)})
		}
		if len(whErrors) == 0 {
			return true, "", nil
		}
		channels := make([]string, 0, len(whErrors))
		for _, e := range whErrors {
			channels = append(channels, e.ChannelID)
		}
		return false, "test event delivery failed for channel(s) " + strings.Join(channels, ", "), whErrors
	}
	return false, "unexpected test event response: " + rqDescribeJSON(data), nil
}

// shTestEventFailureWords mark a string answer from sendTestEvent as a
// failure; shTestEventSuccessAnswers is the whole, bounded set of answers
// accepted as success. Everything else is unknown (a failure for exit).
var (
	shTestEventFailureWords = []string{"error", "fail", "not authorized", "unauthorized", "access denied", "denied",
		"forbidden", "invalid", "rejected", "not sent", "not delivered", "unsuccessful"}
	shTestEventSuccessAnswers = map[string]bool{
		"ok": true, "sent": true, "success": true, "delivered": true, "true": true,
		"test event sent": true, "test event delivered": true,
	}
)

// shClassifyTestEventString returns ok, failed or unknown. Success needs the
// whole trimmed answer (case-insensitive, ignoring trailing . or !) to be one
// of shTestEventSuccessAnswers, so "Test event wasn't sent", "0 events sent"
// or "Success: false" are never read as success.
func shClassifyTestEventString(str string) string {
	low := strings.ToLower(strings.TrimSpace(str))
	if low == "" || low == "false" {
		return "failed"
	}
	for _, w := range shTestEventFailureWords {
		if strings.Contains(low, w) {
			return "failed"
		}
	}
	if shTestEventSuccessAnswers[strings.TrimRight(low, ".! ")] {
		return "ok"
	}
	return "unknown"
}

// shSafeChannelID echoes an IWhError channelId only when it is one of this
// partner's subscription channels (read from GET /subscriptions); any other
// value is masked, since the response body is not trusted to be an id.
func shSafeChannelID(id string, knownChannels map[string]bool) string {
	if knownChannels[id] {
		return id
	}
	return "(unrecognized channel id)"
}

func shPrintHuman(cmd *cobra.Command, r shResult) error {
	w := saasHumanWriter(cmd.OutOrStdout()) // saas-alerts hand-fix: tenant strings never drive the terminal
	fmt.Fprintf(w, "%d subscription(s): %d enabled, %d disabled, %d with backlog (%d unsent packs), %d flagged\n",
		r.Summary.Total, r.Summary.Enabled, r.Summary.Disabled, r.Summary.Backlog, r.Summary.UnsentPacks, r.Summary.Flagged)
	if len(r.Subscriptions) > 0 {
		tw := newTabWriter(w)
		fmt.Fprintln(tw, "CHANNEL\tENABLED\tQUEUE\tDOMAIN OK\tFILTERS\tURL\tFLAGS")
		for _, s := range r.Subscriptions {
			depth := "?"
			if s.UnsentQueueDepth != nil {
				depth = fmt.Sprintf("%d", *s.UnsentQueueDepth)
			}
			verified := "?"
			if s.DomainVerified != nil {
				verified = fmt.Sprintf("%t", *s.DomainVerified)
			}
			filters := fmt.Sprintf("status:%d type:%d cust:%d", s.Filters.AlertStatuses, s.Filters.EventTypes, s.Filters.CustomerIDs)
			fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%s\t%s\t%s\n", s.ChannelID, s.Enabled, depth, verified, filters, truncate(s.URL, 48), strings.Join(s.Flags, ","))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
	for _, inc := range r.CollectionIncomplete {
		fmt.Fprintf(w, "warning: %s is incomplete: %d valid, %d invalid member(s)\n", inc.Collection, inc.Valid, inc.Invalid)
	}
	if r.Plan != nil {
		for _, p := range *r.Plan {
			verb := "skip"
			if p.Planned {
				verb = "re-enable"
			}
			fmt.Fprintf(w, "%s %s: %s\n", verb, p.ChannelID, p.Reason)
		}
	}
	if r.Executed != nil {
		for _, e := range *r.Executed {
			status := "ok"
			if !e.OK {
				status = "FAILED " + e.Error
			}
			fmt.Fprintf(w, "%s %s (%s): %s\n", e.Method, e.Path, e.ChannelID, status)
		}
	}
	if r.Note != "" {
		fmt.Fprintln(w, r.Note)
	}
	return nil
}
