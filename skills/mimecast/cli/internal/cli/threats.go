// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel commands: threats mailbox and threats sender share one
// fan-out over the five Mimecast logs that say what a message did: URL
// Protect clicks, Attachment Protect verdicts, Impersonation Protect hits,
// gateway rejections and the hold queue.
// pp:data-source live
// pp:client-call

package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
)

func newNovelThreatsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "threats",
		Short:       "Trace threat events across URL, attachment and impersonation logs, rejections and holds",
		Example:     "  mimecast-cli threats mailbox alice@example.com --since 7d --agent\n  mimecast-cli threats sender @phish.example --since 3d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelThreatsMailboxCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelThreatsSenderCmd(flags))
	return cmd
}

// Threat event sources, in fetch order.
const (
	threatSrcURL           = "url-protect"
	threatSrcAttachment    = "attachment-protect"
	threatSrcImpersonation = "impersonation-protect"
	threatSrcRejection     = "gateway-rejections"
	threatSrcHeld          = "hold-queue"
)

type threatEvent struct {
	When      string `json:"when"`
	Source    string `json:"source"`
	Kind      string `json:"kind"`                // url-click, url-scan, attachment, impersonation, rejected, held
	Direction string `json:"direction,omitempty"` // threats mailbox only: inbound (to the mailbox) or outbound (from it)
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Action    string `json:"action,omitempty"`
	Verdict   string `json:"verdict,omitempty"`
	Detail    string `json:"detail,omitempty"` // the URL, file name, impersonation definition, or rejection/hold reason
	MessageID string `json:"message_id,omitempty"`
	HeldID    string `json:"held_id,omitempty"`
	// ClickedThroughWarning is a URL click where the user continued past a warning page.
	ClickedThroughWarning bool `json:"clicked_through_warning,omitempty"`
	at                    time.Time
}

type threatSource struct {
	Name      string `json:"name"`
	Scanned   int    `json:"scanned"`
	Matched   int    `json:"matched"`
	Pages     int    `json:"pages"`
	Complete  bool   `json:"complete"`
	Note      string `json:"note,omitempty"`
	Error     string `json:"error,omitempty"`
	Retention string `json:"window,omitempty"`
}

// threatScan is one fan-out over the five sources for one address or domain.
type threatScan struct {
	ctx        context.Context
	c          *client.Client
	acct       string
	start, now time.Time
	maxPages   int
	// match reports whether the event involves the target and, for mailbox
	// mode, in which direction ("" = not a match).
	match func(sender string, recipients []string) string
	// searchField/searchValue narrow the hold-queue and rejection searches
	// server-side (both accept fieldName all/from/to); the client-side match
	// still decides.
	searchField, searchValue string

	events   []threatEvent
	sources  []threatSource
	failures int
	firstErr error
}

func (s *threatScan) run() {
	s.urlClicks()
	s.attachments()
	s.impersonation()
	s.rejections()
	s.held()
	sort.SliceStable(s.events, func(i, j int) bool {
		a, b := s.events[i], s.events[j]
		if !a.at.Equal(b.at) {
			return a.at.After(b.at) // newest first; unparseable times sort last
		}
		return a.When > b.When
	})
}

func (s *threatScan) record(src threatSource, page mcPage, err error) {
	src.Pages = page.Pages
	src.Complete = err == nil && !page.Truncated
	if err != nil {
		s.failures++
		if s.firstErr == nil {
			s.firstErr = err
		}
		src.Error = err.Error()
	} else {
		src.Note = page.note("raise --max-scan-pages to scan further")
	}
	s.sources = append(s.sources, src)
}

func (s *threatScan) add(src *threatSource, e threatEvent, sender string, recipients []string) {
	dir := s.match(sender, recipients)
	if dir == "" {
		return
	}
	if dir != "*" {
		e.Direction = dir
	}
	e.at, _ = parseMCTime(e.When)
	src.Matched++
	s.events = append(s.events, e)
}

func (s *threatScan) ttpWindow() map[string]any {
	return map[string]any{"from": mcTime(s.start), "to": mcTime(s.now), "oldestFirst": false}
}

func (s *threatScan) urlClicks() {
	src := threatSource{Name: threatSrcURL}
	item := s.ttpWindow()
	item["route"], item["scanResult"] = "all", "all"
	page, err := mcReadAll(s.ctx, s.c, "/api/ttp/url/get-logs", []any{item}, s.acct, mcMaxPageSize, s.maxPages)
	if err == nil {
		rows := unwrapList(page.Data, "clickLogs")
		src.Scanned = len(rows)
		for _, raw := range rows {
			var l struct {
				Date, UserEmailAddress, FromUserEmailAddress, URL, Action, ScanResult, CreationMethod string
				UserOverride, MessageID, Subject, TTPDefinition                                       string
			}
			if json.Unmarshal(raw, &l) != nil {
				continue
			}
			kind := "url-click"
			// "Entry Scan" rows are delivery-time scans, not a person clicking.
			if l.CreationMethod != "" && !strings.EqualFold(l.CreationMethod, "User Click") {
				kind = "url-scan"
			}
			s.add(&src, threatEvent{When: l.Date, Source: threatSrcURL, Kind: kind, Sender: l.FromUserEmailAddress, Recipient: l.UserEmailAddress,
				Subject: l.Subject, Action: l.Action, Verdict: l.ScanResult, Detail: l.URL, MessageID: l.MessageID,
				ClickedThroughWarning: kind == "url-click" && strings.EqualFold(l.UserOverride, "continue")}, l.FromUserEmailAddress, []string{l.UserEmailAddress})
		}
	}
	s.record(src, page, err)
}

func (s *threatScan) attachments() {
	src := threatSource{Name: threatSrcAttachment}
	item := s.ttpWindow()
	item["route"], item["result"] = "all", "all"
	page, err := mcReadAll(s.ctx, s.c, "/api/ttp/attachment/get-logs", []any{item}, s.acct, mcMaxPageSize, s.maxPages)
	if err == nil {
		rows := unwrapList(page.Data, "attachmentLogs")
		src.Scanned = len(rows)
		for _, raw := range rows {
			var l struct {
				Date, RecipientAddress, SenderAddress, FileName, Result, ActionTriggered, MessageID, Subject, Definition string
			}
			if json.Unmarshal(raw, &l) != nil {
				continue
			}
			s.add(&src, threatEvent{When: l.Date, Source: threatSrcAttachment, Kind: "attachment", Sender: l.SenderAddress, Recipient: l.RecipientAddress,
				Subject: l.Subject, Action: l.ActionTriggered, Verdict: l.Result, Detail: cmp.Or(l.FileName, l.Definition), MessageID: l.MessageID},
				l.SenderAddress, []string{l.RecipientAddress})
		}
	}
	s.record(src, page, err)
}

func (s *threatScan) impersonation() {
	src := threatSource{Name: threatSrcImpersonation}
	page, err := mcReadAll(s.ctx, s.c, "/api/ttp/impersonation/get-logs", []any{s.ttpWindow()}, s.acct, mcMaxPageSize, s.maxPages)
	if err == nil {
		rows := unwrapList(page.Data, "impersonationLogs")
		src.Scanned = len(rows)
		for _, raw := range rows {
			var l struct {
				EventTime, RecipientAddress, SenderAddress, Action, Definition, MessageID, Subject string
				TaggedMalicious                                                                    bool
			}
			if json.Unmarshal(raw, &l) != nil {
				continue
			}
			verdict := "not tagged malicious"
			if l.TaggedMalicious {
				verdict = "tagged malicious"
			}
			s.add(&src, threatEvent{When: l.EventTime, Source: threatSrcImpersonation, Kind: "impersonation", Sender: l.SenderAddress, Recipient: l.RecipientAddress,
				Subject: l.Subject, Action: l.Action, Verdict: verdict, Detail: l.Definition, MessageID: l.MessageID}, l.SenderAddress, []string{l.RecipientAddress})
		}
	}
	s.record(src, page, err)
}

func (s *threatScan) rejections() {
	src := threatSource{Name: threatSrcRejection}
	start := s.start
	if limit := s.now.AddDate(0, 0, -7); start.Before(limit) {
		start = limit // the rejection log covers 7 days
		src.Retention = "last 7 days (the rejection log keeps 7)"
	}
	item := map[string]any{"admin": true, "start": mcTime(start), "end": mcTime(s.now), "searchBy": map[string]any{"fieldName": s.searchField, "value": s.searchValue}}
	page, err := mcReadAll(s.ctx, s.c, "/api/gateway/get-rejections", []any{item}, s.acct, mcMaxPageSize, s.maxPages)
	if err == nil {
		rows := unwrapList(page.Data, "rejections")
		src.Scanned = len(rows)
		for _, raw := range rows {
			var r struct {
				Created, Description, Info, FromAddress, ToAddress, ID string
				SpamScore                                              *int
			}
			if json.Unmarshal(raw, &r) != nil {
				continue
			}
			verdict := ""
			if r.SpamScore != nil {
				verdict = fmt.Sprintf("spam score %d", *r.SpamScore)
			}
			s.add(&src, threatEvent{When: r.Created, Source: threatSrcRejection, Kind: "rejected", Sender: r.FromAddress, Recipient: r.ToAddress,
				Verdict: verdict, Detail: strings.TrimSpace(r.Description + " " + r.Info)}, r.FromAddress, []string{r.ToAddress})
		}
	}
	s.record(src, page, err)
}

func (s *threatScan) held() {
	src := threatSource{Name: threatSrcHeld}
	held, page, err := fetchHeld(s.ctx, s.c, s.start, s.now, s.acct, s.maxPages, []map[string]any{{"fieldName": s.searchField, "value": s.searchValue}})
	if err == nil {
		src.Scanned = len(held)
		for _, h := range held {
			// One event per matching recipient, so a held message to three
			// users counts once for each of them.
			rcpts := h.recipients()
			if len(rcpts) == 0 {
				rcpts = []string{""}
			}
			for _, rcpt := range rcpts {
				class, _ := h.risk()
				s.add(&src, threatEvent{When: cmp.Or(h.HeldSince, h.Received), Source: threatSrcHeld, Kind: "held", Sender: h.sender(), Recipient: rcpt,
					Subject: h.Subject, Action: "held", Verdict: "release risk " + class, Detail: strings.TrimSpace(h.HeldReason + " " + h.PolicyInfo), HeldID: h.ID},
					h.sender(), []string{rcpt})
			}
		}
	}
	s.record(src, page, err)
}

// threatsFlags are the options both commands share.
type threatsFlags struct {
	since, accountCode string
	maxScanPages       int
	limit              int
}

// newThreatScan validates the shared flags and builds a scan, or returns a
// usage error.
func (o *threatsFlags) newThreatScan(cmd *cobra.Command, flags *rootFlags) (*threatScan, func(), error) {
	now := time.Now()
	start, err := parseSince(o.since, now)
	if err != nil {
		return nil, nil, usageErr(err)
	}
	if o.maxScanPages < 1 {
		return nil, nil, usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
	}
	if cliutil.IsDogfoodEnv() {
		o.maxScanPages = 1
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	c, err := flags.newClient()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return &threatScan{ctx: ctx, c: c, acct: mcAccountCode(o.accountCode), start: start, now: now, maxPages: o.maxScanPages}, cancel, nil
}

// threatsReport is the fields both views carry about the scan itself.
type threatsReport struct {
	Target   string `json:"target"`
	Since    string `json:"since"`
	Until    string `json:"until"`
	Complete bool   `json:"complete"`
	// FetchFailures counts sources that could not be read: their events are
	// missing, so a short answer is not a clean bill of health.
	FetchFailures     int            `json:"fetch_failures"`
	FirstError        string         `json:"first_error,omitempty"`
	IncompleteSources []string       `json:"incomplete_sources"`
	Sources           []threatSource `json:"sources"`
}

func (s *threatScan) report(target string) threatsReport {
	r := threatsReport{Target: target, Since: mcTime(s.start), Until: mcTime(s.now), FetchFailures: s.failures, IncompleteSources: []string{}, Sources: s.sources}
	if s.firstErr != nil {
		r.FirstError = s.firstErr.Error()
	}
	for _, src := range s.sources {
		if !src.Complete {
			r.IncompleteSources = append(r.IncompleteSources, src.Name)
		}
	}
	r.Complete = len(r.IncompleteSources) == 0
	return r
}

// finish turns an all-sources-failed scan into its first error (exit code
// kept), and warns on stderr about any source that failed or stopped early.
func (s *threatScan) finish(cmd *cobra.Command, flags *rootFlags) error {
	if s.failures == len(s.sources) && s.firstErr != nil {
		return classifyAPIError(cmd.ErrOrStderr(), fmt.Errorf("every threat source failed: %w", s.firstErr), flags)
	}
	for _, src := range s.sources {
		switch {
		case src.Error != "":
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s unavailable, its events are missing: %s\n", src.Name, src.Error)
		case src.Note != "":
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s incomplete: %s\n", src.Name, src.Note)
		}
	}
	return nil
}

// threatTarget normalizes an address or domain argument ("@x.example" and
// "x.example" both mean the domain).
func threatTarget(args []string) (string, error) {
	if len(args) != 1 {
		return "", usageErr(fmt.Errorf("exactly one address or domain is required"))
	}
	t := strings.ToLower(strings.TrimSpace(args[0]))
	if strings.HasPrefix(t, "@") {
		t = strings.TrimPrefix(t, "@")
	}
	if t == "" || strings.ContainsAny(t, " \t\r\n,;") || strings.Count(t, "@") > 1 || strings.HasSuffix(t, "@") || strings.HasPrefix(t, "@") {
		return "", usageErr(fmt.Errorf("%q is not an email address or domain", args[0]))
	}
	return t, nil
}
