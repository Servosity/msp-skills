// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: why was this message blocked / where is it.
// pp:data-source live
// pp:client-call

package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
)

type whyTracked struct {
	ID             string   `json:"id"`
	Subject        string   `json:"subject,omitempty"`
	From           string   `json:"from,omitempty"`
	To             []string `json:"to,omitempty"`
	Received       string   `json:"received,omitempty"`
	Status         string   `json:"status,omitempty"`
	Info           string   `json:"info,omitempty"`
	Route          string   `json:"route,omitempty"`
	SpamScore      *int     `json:"spam_score,omitempty"`
	SpamVerdict    string   `json:"spam_verdict,omitempty"`
	DetectionLevel string   `json:"detection_level,omitempty"`
}

type whyDetail struct {
	ID             string          `json:"id"`
	Status         string          `json:"status,omitempty"`
	SpamScore      *int            `json:"spam_score,omitempty"`
	DetectionLevel string          `json:"detection_level,omitempty"`
	Verdict        string          `json:"spam_verdict,omitempty"`
	AuthChecks     *spamDetail     `json:"checks,omitempty"`
	FailedChecks   []string        `json:"failed_checks,omitempty"`
	Delivery       json.RawMessage `json:"delivery,omitempty"`
	recipientInfo  json.RawMessage
}

type whyHeld struct {
	ID         string `json:"id"`
	Subject    string `json:"subject,omitempty"`
	HeldSince  string `json:"held_since,omitempty"`
	HeldReason string `json:"held_reason,omitempty"`
	Policy     string `json:"policy,omitempty"`
	HeldGroup  string `json:"held_group,omitempty"`
	SpamScore  *int   `json:"spam_score,omitempty"`
	RiskClass  string `json:"risk_class"`
	RiskBasis  string `json:"risk_basis"`
}

type whyRelease struct {
	When         string   `json:"when"`
	Status       string   `json:"status"`
	Operator     string   `json:"operator,omitempty"`
	Subject      string   `json:"subject,omitempty"`
	HeldReason   string   `json:"held_reason,omitempty"`
	Policy       string   `json:"policy,omitempty"`
	RejectReason string   `json:"reject_reason,omitempty"`
	MessageInfo  string   `json:"message_info,omitempty"`
	FailedChecks []string `json:"failed_checks,omitempty"`
}

type whyRejection struct {
	When        string `json:"when"`
	From        string `json:"from"`
	To          string `json:"to"`
	Description string `json:"description,omitempty"`
	Info        string `json:"info,omitempty"`
	SpamScore   *int   `json:"spam_score,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	RemoteName  string `json:"remote_name,omitempty"`
	ManagedSndr bool   `json:"managed_sender_rule,omitempty"`
}

type whyView struct {
	Query struct {
		ID      string `json:"id,omitempty"`
		From    string `json:"from,omitempty"`
		To      string `json:"to,omitempty"`
		Subject string `json:"subject,omitempty"`
		Since   string `json:"since"`
	} `json:"query"`
	Stage          string            `json:"stage"` // held | rejected-at-gateway | released-from-hold | rejected-from-hold | <tracking status> | not-found
	Summary        string            `json:"summary"`
	Held           []whyHeld         `json:"held"`
	Rejections     []whyRejection    `json:"gateway_rejections"`
	ReleaseHistory []whyRelease      `json:"release_history"`
	Tracked        []whyTracked      `json:"tracked"`
	Details        []whyDetail       `json:"details"`
	NextSteps      []string          `json:"next_steps"`
	SectionErrors  map[string]string `json:"section_errors,omitempty"`
	// IncompleteSections lists searches that stopped at their page cap or on
	// a repeated page token; a match could lie beyond them.
	IncompleteSections []string `json:"incomplete_sections,omitempty"`
	// RelatedSenderHistory holds the same sender's other hold, release and
	// rejection records when the query names one message ID. It is context
	// only and never drives the stage.
	RelatedSenderHistory *whyRelated `json:"related_sender_history,omitempty"`
}

type whyRelated struct {
	Note           string         `json:"note"`
	Held           []whyHeld      `json:"held"`
	Rejections     []whyRejection `json:"gateway_rejections"`
	ReleaseHistory []whyRelease   `json:"release_history"`
}

func newNovelWhyCmd(flags *rootFlags) *cobra.Command {
	var from, to, subject, since, accountCode string
	var limit int

	cmd := &cobra.Command{
		Use:   "why [message-id]",
		Short: "Explain one message in a single verdict: where it is now and why it was held, rejected or delayed",
		Long: "Use this command to explain what happened to one specific message: where it is now and why it was held, rejected, or delayed. " +
			"Do NOT use it to review the whole hold queue; use 'held triage' instead. Do NOT use it for tenant-wide queue backlog; use 'mailflow' instead.\n\n" +
			"Give a Mimecast message ID, or --from and/or --to (address or domain) with optional --subject. The command joins message tracking, " +
			"message detail (spam score, SPF/DKIM/DMARC), the hold queue, the held-release log and the 7-day gateway rejection log, then reports one stage: " +
			"held, rejected-at-gateway, released-from-hold, rejected-from-hold, the tracking status (delivery, bounced, deferred...) or not-found. " +
			"A section the API app has no product access to is reported in section_errors instead of failing the whole answer.",
		Example:     "  mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent\n  mimecast-cli why eNo1jr0OgjAYRd-lExample\n  mimecast-cli why --from vendor.example --subject \"Invoice\" --since 7d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--from=billing@vendor.example;--to=alice@example.com;--since=3d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "why")
			}
			id := ""
			if len(args) > 0 {
				id = strings.TrimSpace(args[0])
			}
			if id == "" && from == "" && to == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("give a message ID, or --from and/or --to"))
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && limit > 1 {
				limit = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			w := whyRun{ctx: ctx, c: c, acct: mcAccountCode(accountCode), start: start, now: now, from: from, to: to, subject: subject}
			view := w.run(id, limit)
			if w.ran > 0 && w.failedCalls == w.ran {
				// Every section that ran failed: there is no answer, so surface
				// the first error with its exit code. A partial failure still
				// answers (possibly not-found) with section_errors.
				return classifyAPIError(cmd.ErrOrStderr(), w.firstErr, flags)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Stage: %s\n%s\n", view.Stage, view.Summary)
			for _, h := range view.Held {
				fmt.Fprintf(out, "  held since %s: %s / %s [%s]\n", h.HeldSince, h.HeldReason, h.Policy, h.RiskClass)
			}
			for _, r := range view.Rejections {
				fmt.Fprintf(out, "  rejected %s: %s %s (spam score %s)\n", r.When, r.Description, r.Info, intPtrString(r.SpamScore))
			}
			for _, r := range view.ReleaseHistory {
				fmt.Fprintf(out, "  %s %s by %s: %s %s\n", r.When, r.Status, r.Operator, r.HeldReason, r.RejectReason)
			}
			for _, d := range view.Details {
				if len(d.FailedChecks) > 0 {
					fmt.Fprintf(out, "  %s failed checks: %s\n", d.ID, strings.Join(d.FailedChecks, ", "))
				}
			}
			for _, s := range view.NextSteps {
				fmt.Fprintf(out, "  next: %s\n", s)
			}
			for _, s := range sortedKeys(view.SectionErrors) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unavailable: %s\n", s, view.SectionErrors[s])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Sender address or domain")
	cmd.Flags().StringVar(&to, "to", "", "Recipient address or domain")
	cmd.Flags().StringVar(&subject, "subject", "", "Subject text to narrow matches (case-insensitive substring)")
	cmd.Flags().StringVar(&since, "since", "7d", "Lookback window (3d, 7d; tracking covers 30 days, rejections 7)")
	cmd.Flags().IntVar(&limit, "limit", 3, "Maximum tracked messages to pull full detail for")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

type whyRun struct {
	ctx               context.Context
	c                 *client.Client
	acct              string
	start, now        time.Time
	from, to, subject string
	firstErr          error
	ran               int // section calls attempted
	failedCalls       int // section calls that failed
}

func (w *whyRun) noteTruncated(view *whyView, section string, page mcPage) {
	if page.Truncated {
		view.IncompleteSections = append(view.IncompleteSections, section)
	}
}

func (w *whyRun) fail(view *whyView, section string, err error) {
	w.failedCalls++
	if w.firstErr == nil {
		w.firstErr = err
	}
	if view.SectionErrors == nil {
		view.SectionErrors = map[string]string{}
	}
	view.SectionErrors[section] = err.Error()
}

// searchKey is the server-side filter: the sender when known, else the
// recipient (all three log endpoints accept fieldName from or to).
func (w *whyRun) searchKey() (string, string) {
	if w.from != "" {
		return "from", w.from
	}
	return "to", w.to
}

func anyAddrMatches(want string, addrs ...string) bool {
	return want == "" || slices.ContainsFunc(addrs, func(a string) bool { return addrMatches(a, want) })
}

func (w *whyRun) subjectOK(s string) bool {
	return w.subject == "" || strings.Contains(strings.ToLower(s), strings.ToLower(w.subject))
}

func (w *whyRun) run(id string, limit int) whyView {
	var view whyView
	view.Query.ID, view.Query.From, view.Query.To, view.Query.Subject, view.Query.Since = id, w.from, w.to, w.subject, mcTime(w.start)
	view.Held, view.Rejections, view.ReleaseHistory = []whyHeld{}, []whyRejection{}, []whyRelease{}
	view.Tracked, view.Details, view.NextSteps = []whyTracked{}, []whyDetail{}, []string{}

	ids := []string{}
	if id != "" {
		ids = append(ids, id)
	} else {
		w.track(&view)
		for i := 0; i < len(view.Tracked) && i < limit; i++ {
			ids = append(ids, view.Tracked[i].ID)
		}
	}
	for _, mid := range ids {
		w.detail(&view, mid)
	}
	if w.from == "" && w.to == "" && len(view.Details) > 0 {
		// ID-only query: correlate on the detail's sender (delivery detail
		// first, then recipientInfo.messageInfo for undelivered messages).
		w.from = senderFromDelivery(view.Details[0].Delivery)
		if w.from == "" {
			w.from = senderFromDelivery(view.Details[0].recipientInfo)
		}
	}
	if w.from != "" || w.to != "" {
		w.held(&view)
		w.releases(&view)
		w.rejections(&view)
	}
	if id == "" && w.subject != "" && len(view.Rejections) > 0 {
		// Rejection records carry no subject, so with --subject they cannot be
		// tied to this message; keep them as context, never as the verdict.
		view.RelatedSenderHistory = &whyRelated{Note: "gateway rejections for these addresses; the rejection log has no subject, so they are not tied to this message",
			Held: []whyHeld{}, Rejections: view.Rejections, ReleaseHistory: []whyRelease{}}
		view.Rejections = []whyRejection{}
	}
	if id != "" {
		// Release-log and rejection IDs are not the message ID, so only an
		// exact hold-ID match may decide this message's stage.
		related := &whyRelated{Note: "other records for the same sender; not used for this message's verdict",
			Held: []whyHeld{}, Rejections: view.Rejections, ReleaseHistory: view.ReleaseHistory}
		exact := []whyHeld{}
		for _, h := range view.Held {
			if h.ID == id {
				exact = append(exact, h)
			} else {
				related.Held = append(related.Held, h)
			}
		}
		view.Held, view.Rejections, view.ReleaseHistory = exact, []whyRejection{}, []whyRelease{}
		if len(related.Held)+len(related.Rejections)+len(related.ReleaseHistory) > 0 {
			view.RelatedSenderHistory = related
		}
	}
	w.verdict(&view)
	return view
}

func (w *whyRun) track(view *whyView) {
	w.ran++
	opts := map[string]any{}
	if w.from != "" {
		opts["from"] = w.from
	}
	if w.to != "" {
		opts["to"] = w.to
	}
	if w.subject != "" {
		opts["subject"] = w.subject
	}
	start := w.start
	if limit := w.now.AddDate(0, 0, -30); start.Before(limit) {
		start = limit // message tracking covers 30 days
	}
	item := map[string]any{"start": mcTime(start), "end": mcTime(w.now), "searchReason": "mimecast-cli why", "advancedTrackAndTraceOptions": opts}
	page, err := mcReadAll(w.ctx, w.c, "/api/message-finder/search", []any{item}, w.acct, mcMaxPageSize, 2)
	if err != nil {
		w.fail(view, "tracking", err)
		return
	}
	w.noteTruncated(view, "tracking", page)
	seen := map[string]bool{}
	for _, raw := range unwrapList(page.Data, "trackedEmails") {
		var t struct {
			ID             string   `json:"id"`
			Subject        string   `json:"subject"`
			FromEnv        mcAddr   `json:"fromEnv"`
			FromHdr        mcAddr   `json:"fromHdr"`
			To             []mcAddr `json:"to"`
			Received       string   `json:"received"`
			Status         string   `json:"status"`
			Info           string   `json:"info"`
			Route          string   `json:"route"`
			SpamScore      *int     `json:"spamScore"`
			SpamVerdict    string   `json:"spamVerdict"`
			DetectionLevel string   `json:"detectionLevel"`
		}
		if json.Unmarshal(raw, &t) != nil || t.ID == "" || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		from := cmp.Or(t.FromEnv.EmailAddress, t.FromHdr.EmailAddress)
		var tos []string
		for _, a := range t.To {
			tos = append(tos, a.EmailAddress)
		}
		view.Tracked = append(view.Tracked, whyTracked{ID: t.ID, Subject: t.Subject, From: from, To: tos, Received: t.Received, Status: t.Status,
			Info: t.Info, Route: t.Route, SpamScore: t.SpamScore, SpamVerdict: t.SpamVerdict, DetectionLevel: t.DetectionLevel})
	}
	sort.SliceStable(view.Tracked, func(i, j int) bool {
		a, _ := parseMCTime(view.Tracked[i].Received)
		b, _ := parseMCTime(view.Tracked[j].Received)
		return a.After(b)
	})
}

func (w *whyRun) detail(view *whyView, id string) {
	w.ran++
	resp, err := mcRead(w.ctx, w.c, "/api/message-finder/get-message-info", mcBody([]any{map[string]any{"id": id}}, 0, "", w.acct))
	if err != nil {
		w.fail(view, "detail", err)
		return
	}
	for _, raw := range resp.Data {
		var d struct {
			ID            string          `json:"id"`
			Status        string          `json:"status"`
			Delivery      json.RawMessage `json:"deliveredMessage"`
			RecipientInfo json.RawMessage `json:"recipientInfo"`
			Spam          struct {
				SpamScore      *int        `json:"spamScore"`
				DetectionLevel string      `json:"detectionLevel"`
				Detail         *spamDetail `json:"spamProcessingDetail"`
			} `json:"spamInfo"`
		}
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		if d.ID == "" {
			d.ID = id
		}
		out := whyDetail{ID: d.ID, Status: d.Status, SpamScore: d.Spam.SpamScore, DetectionLevel: d.Spam.DetectionLevel,
			AuthChecks: d.Spam.Detail, FailedChecks: d.Spam.Detail.failedChecks(), Delivery: d.Delivery, recipientInfo: d.RecipientInfo}
		if d.Spam.Detail != nil && d.Spam.Detail.Verdict != nil {
			out.Verdict = strings.TrimSpace(d.Spam.Detail.Verdict.Decision + " " + d.Spam.Detail.Verdict.Risk)
		}
		view.Details = append(view.Details, out)
	}
}

// senderFromDelivery pulls an envelope/header sender out of the
// deliveredMessage object, whose shape varies (it may be keyed by recipient).
func senderFromDelivery(raw json.RawMessage) string {
	var found string
	var walk func(v any)
	walk = func(v any) {
		if found != "" {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			for _, k := range []string{"fromEnvelope", "fromHeader"} {
				if s, ok := t[k].(string); ok && strings.Contains(s, "@") {
					found = s
					return
				}
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		walk(v)
	}
	return found
}

func (w *whyRun) held(view *whyView) {
	w.ran++
	field, value := w.searchKey()
	held, page, err := fetchHeld(w.ctx, w.c, w.start, w.now, w.acct, 2, []map[string]any{{"fieldName": field, "value": value}})
	if err != nil {
		w.fail(view, "hold_queue", err)
		return
	}
	w.noteTruncated(view, "hold_queue", page)
	for _, h := range held {
		if !anyAddrMatches(w.from, h.FromEnv.EmailAddress, h.FromHdr.EmailAddress) || !anyAddrMatches(w.to, h.recipients()...) || !w.subjectOK(h.Subject) {
			continue
		}
		class, basis := h.risk()
		view.Held = append(view.Held, whyHeld{ID: h.ID, Subject: h.Subject, HeldSince: h.HeldSince, HeldReason: h.HeldReason, Policy: h.PolicyInfo,
			HeldGroup: h.HeldGroup, SpamScore: h.SpamScore, RiskClass: class, RiskBasis: basis})
	}
}

func (w *whyRun) releases(view *whyView) {
	w.ran++
	field, value := w.searchKey()
	logs, page, err := fetchReleaseLogs(w.ctx, w.c, w.start, w.now, w.acct, 2, map[string]any{"fieldName": field, "value": value})
	if err != nil {
		w.fail(view, "release_log", err)
		return
	}
	w.noteTruncated(view, "release_log", page)
	for _, l := range logs {
		var tos []string
		for _, t := range l.To {
			tos = append(tos, t.EmailAddress)
		}
		if !anyAddrMatches(w.from, l.FromEnv.EmailAddress, l.FromHdr.EmailAddress) || !anyAddrMatches(w.to, tos...) || !w.subjectOK(l.Subject) {
			continue
		}
		view.ReleaseHistory = append(view.ReleaseHistory, whyRelease{When: l.Released, Status: l.Status, Operator: l.Operator.EmailAddress, Subject: l.Subject,
			HeldReason: l.HeldReason, Policy: l.Policy, RejectReason: l.RejectReason, MessageInfo: l.MessageInfo, FailedChecks: l.SpamProcessingDetail.failedChecks()})
	}
}

func (w *whyRun) rejections(view *whyView) {
	w.ran++
	start := w.start
	if limit := w.now.AddDate(0, 0, -7); start.Before(limit) {
		start = limit // the rejection log covers 7 days
	}
	field, value := w.searchKey()
	item := map[string]any{"admin": true, "start": mcTime(start), "end": mcTime(w.now), "searchBy": map[string]any{"fieldName": field, "value": value}}
	page, err := mcReadAll(w.ctx, w.c, "/api/gateway/get-rejections", []any{item}, w.acct, mcMaxPageSize, 2)
	if err != nil {
		w.fail(view, "rejections", err)
		return
	}
	w.noteTruncated(view, "rejections", page)
	for _, raw := range unwrapList(page.Data, "rejections") {
		var r struct {
			Created, Description, Info, FromAddress, ToAddress, IPAddress, RemoteName string
			SpamScore                                                                 *int
			ManageRecipient                                                           bool
		}
		if json.Unmarshal(raw, &r) != nil || !anyAddrMatches(w.from, r.FromAddress) || !anyAddrMatches(w.to, r.ToAddress) {
			continue
		}
		view.Rejections = append(view.Rejections, whyRejection{When: r.Created, From: r.FromAddress, To: r.ToAddress, Description: r.Description,
			Info: r.Info, SpamScore: r.SpamScore, IPAddress: r.IPAddress, RemoteName: r.RemoteName, ManagedSndr: r.ManageRecipient})
	}
}

// verdict picks the most specific stage the evidence supports.
func (w *whyRun) verdict(view *whyView) {
	switch {
	case len(view.Held) > 0:
		h := view.Held[0]
		view.Stage = "held"
		view.Summary = fmt.Sprintf("Held since %s by %q (policy %q); release risk %s (%s).", h.HeldSince, h.HeldReason, h.Policy, h.RiskClass, h.RiskBasis)
		if h.RiskClass == riskNeverRelease {
			view.NextSteps = append(view.NextSteps, "Do not release on request: treat as a threat and confirm with the sender out of band.")
		} else {
			view.NextSteps = append(view.NextSteps, "mimecast-cli held release-safe "+shellQuote(h.ID)+" --ticket <ticket>")
		}
	case len(view.Rejections) > 0:
		r := view.Rejections[0]
		view.Stage = "rejected-at-gateway"
		view.Summary = fmt.Sprintf("Rejected at the gateway on %s: %s %s (spam score %s, sending IP %s).", r.When, r.Description, r.Info, intPtrString(r.SpamScore), r.IPAddress)
		if r.ManagedSndr {
			view.Summary += " A managed-sender block caused it."
			payload, _ := json.Marshal([]map[string]string{{"sender": r.From, "to": r.To, "action": "permit"}})
			view.NextSteps = append(view.NextSteps, "Review the managed-sender entry: mimecast-cli senders permit-or-block --data "+shellQuote(string(payload))+" --dry-run")
		} else {
			view.NextSteps = append(view.NextSteps, "Ask the sender to resend after fixing the cause; rejected mail is not stored and cannot be released.")
		}
	case len(view.ReleaseHistory) > 0:
		r := view.ReleaseHistory[0]
		if strings.EqualFold(r.Status, "released") {
			view.Stage = "released-from-hold"
			view.Summary = fmt.Sprintf("Was held (%s, policy %q) and released by %s at %s.", r.HeldReason, r.Policy, r.Operator, r.When)
			view.NextSteps = append(view.NextSteps, "If the user still cannot see it, check their junk folder or the delivery detail below.")
		} else {
			view.Stage = "rejected-from-hold"
			view.Summary = fmt.Sprintf("Was held (%s, policy %q) and rejected by %s at %s: %s %s.", r.HeldReason, r.Policy, r.Operator, r.When, r.RejectReason, r.MessageInfo)
		}
	case len(view.Tracked) > 0 || len(view.Details) > 0:
		status, info := "", ""
		if len(view.Tracked) > 0 {
			status, info = view.Tracked[0].Status, view.Tracked[0].Info
		}
		if status == "" && len(view.Details) > 0 {
			status = view.Details[0].Status
		}
		view.Stage = strings.ToLower(strings.TrimSpace(status))
		if view.Stage == "" {
			view.Stage = "tracked"
		}
		view.Summary = fmt.Sprintf("Message tracking reports %q %s.", status, info)
		switch view.Stage {
		case "bounced", "rejected":
			view.NextSteps = append(view.NextSteps, "Read the detail checks below (SPF/DKIM/DMARC, managed sender) for the bounce cause.")
		case "deferred", "processing", "bulk_processing":
			view.NextSteps = append(view.NextSteps, "mimecast-cli mailflow")
		}
	default:
		view.Stage = "not-found"
		view.Summary = "No tracking, hold, release or rejection record matched. Tracking covers 30 days and rejections 7; check the addresses, widen --since, or confirm the API app has the Email Security Cloud Gateway and Audit Events products."
	}
	for _, d := range view.Details {
		if len(d.FailedChecks) > 0 {
			view.Summary += fmt.Sprintf(" Sender authentication failed: %s.", strings.Join(d.FailedChecks, ", "))
			break
		}
	}
	if len(view.SectionErrors) > 0 {
		view.Summary += " Some sections were unavailable (see section_errors)."
	}
	if len(view.IncompleteSections) > 0 {
		view.Summary += " Searches stopped early in: " + strings.Join(view.IncompleteSections, ", ") + "; narrow --since or add --subject to see the rest."
	}
}

func intPtrString(p *int) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprint(*p)
}
