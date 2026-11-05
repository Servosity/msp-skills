// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command: external callers whose calls were missed or abandoned and
// who have not been called back yet.
// pp:data-source live
// Fetches account call history (GET /phone/call_history) for the window on
// every run; nothing is read from the local store.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
)

const (
	callbacksPageSize = 300 // spec maximum for /phone/call_history
	// The call history API returns at most one month per from/to range and
	// only covers the last six months.
	callbacksChunk    = 30 * 24 * time.Hour
	callbacksMaxSince = 180 * 24 * time.Hour
	callbacksTimeFmt  = "2006-01-02T15:04:05Z"
)

// call_result values (spec enum) that mean the caller did not reach anyone.
var callbacksMissResults = map[string]bool{
	"abandoned":    true,
	"no_answer":    true,
	"ring_timeout": true,
	"voicemail":    true,
}

// call_result values (spec enum) that mean a person picked up.
var callbacksAnsweredResults = map[string]bool{
	"answered":  true,
	"accepted":  true,
	"picked_up": true,
	"connected": true,
	"succeeded": true,
}

// One GET /phone/call_history item (call_history[], or the deprecated
// call_logs[] on older responses).
type callbacksCall struct {
	CallHistoryUUID string         `json:"call_history_uuid"`
	ID              string         `json:"id"`
	CallID          string         `json:"call_id"`
	Direction       string         `json:"direction"`
	ConnectType     string         `json:"connect_type"`
	CallResult      string         `json:"call_result"`
	HideCallerID    bool           `json:"hide_caller_id"`
	StartTime       string         `json:"start_time"`
	CallerDIDNumber string         `json:"caller_did_number"`
	CallerName      string         `json:"caller_name"`
	CalleeDIDNumber string         `json:"callee_did_number"`
	CalleeName      string         `json:"callee_name"`
	CalleeExtNumber zoomFlexString `json:"callee_ext_number"`
	CalleeExtType   string         `json:"callee_ext_type"`
}

type callbacksTarget struct {
	Name            string `json:"name"`
	ExtensionNumber string `json:"extension_number"`
	ExtensionType   string `json:"extension_type"`
	DIDNumber       string `json:"did_number,omitempty"`
}

type callbacksCaller struct {
	CallerNumber  string          `json:"caller_number"`
	CallerName    string          `json:"caller_name"`
	Attempts      int             `json:"attempts"`
	FirstMissedAt string          `json:"first_missed_at"`
	LastMissedAt  string          `json:"last_missed_at"`
	LastResult    string          `json:"last_result"`
	LastTarget    callbacksTarget `json:"last_target"`
	HoursWaiting  float64         `json:"hours_waiting"`
}

type callbacksResult struct {
	Since        string `json:"since"`
	From         string `json:"from"`
	To           string `json:"to"`
	Ext          string `json:"ext,omitempty"`
	ScannedCalls int    `json:"scanned_calls"`
	MissedCalls  int    `json:"missed_calls"`
	MaxPages     int    `json:"max_pages"`
	PagesFetched int    `json:"pages_fetched"`
	// PagesByRecordingStatus shows both partitions were read: the API's
	// recording_status filter defaults to "recorded", so each is requested.
	PagesByRecordingStatus map[string]int          `json:"pages_by_recording_status"`
	Truncated              bool                    `json:"truncated"`
	FetchFailures          []callbacksFetchFailure `json:"fetch_failures"`
	Note                   string                  `json:"note,omitempty"`
	Callers                []callbacksCaller       `json:"callers"`
}

// callbacksFetchFailure is one (window, recording_status) walk that failed.
type callbacksFetchFailure struct {
	RecordingStatus string `json:"recording_status"`
	From            string `json:"from"`
	To              string `json:"to"`
	PagesFetched    int    `json:"pages_fetched"`
	Error           string `json:"error"`
}

// GET /phone/call_history defaults recording_status to "recorded", which
// would silently drop every unrecorded call (most missed calls, and most
// callbacks). Both partitions are requested explicitly.
var callbacksRecordingStatuses = []string{"recorded", "non_recorded"}

type callbacksFetchResult struct {
	calls         []callbacksCall
	pages         int
	pagesByStatus map[string]int
	truncated     bool
	failures      []callbacksFetchFailure
	firstErr      error
}

func newNovelCallbacksCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagExt string
	var flagMaxPages int

	cmd := &cobra.Command{
		Use:   "callbacks",
		Short: "List external callers whose missed or abandoned calls have not been called back yet (live call history; --since, --ext).",
		Long: strings.TrimSpace(`
External callers whose calls were missed or abandoned and who have not been
called back yet, from live account call history (GET /phone/call_history).

A miss is an inbound external call with call_result abandoned, no_answer,
ring_timeout or voicemail. Calls with a hidden caller ID are ignored. A caller
is dropped once a later outbound call went to their number or a later inbound
call from them was answered (numbers compared by digits). Results are grouped
by caller and sorted by the most recent miss; hours_waiting counts from the
first unreturned miss. --ext keeps only misses whose callee extension matches.

The API's recording_status filter defaults to recorded calls only, so both the
recorded and non_recorded partitions are fetched for every window and merged
(deduplicated by call_history_uuid). Every run reads live call history (the
response cache is bypassed). If some fetches fail while others succeed, the
partial result is printed with fetch_failures and the command exits 6
(--allow-partial-failure exits 0).`),
		Example: strings.Trim(`
  zoom-phone-cli callbacks
  zoom-phone-cli callbacks --since 7d --agent
  zoom-phone-cli callbacks --since 2w --ext 1001 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "callbacks")
			}
			since, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil || since <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since must be a positive duration such as 7d, 2w or 36h"))
			}
			if since > callbacksMaxSince {
				return usageErr(fmt.Errorf("--since %s is beyond the six months of call history the API keeps; use 180d or less", flagSince))
			}
			if flagMaxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			ext := strings.TrimSpace(flagExt)

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// A callback made minutes ago must show up: bypass the response cache.
			c.NoCache = true

			now := time.Now().UTC()
			from := now.Add(-since)
			maxPages := flagMaxPages
			if cliutil.IsDogfoodEnv() {
				// One page of each recording_status partition.
				maxPages = len(callbacksRecordingStatuses)
			}
			// cmd.Context(), not boundCtx: the client applies --timeout per request.
			fr := callbacksFetch(cmd.Context(), c, from, now, maxPages)
			if fr.pages == 0 && fr.firstErr != nil {
				return classifyAPIErrorOnly(fr.firstErr)
			}

			callers, missed := callbacksCompute(fr.calls, ext, now)
			result := callbacksResult{
				Since:                  flagSince,
				From:                   from.Format(callbacksTimeFmt),
				To:                     now.Format(callbacksTimeFmt),
				Ext:                    ext,
				ScannedCalls:           len(fr.calls),
				MissedCalls:            missed,
				MaxPages:               maxPages,
				PagesFetched:           fr.pages,
				PagesByRecordingStatus: fr.pagesByStatus,
				Truncated:              fr.truncated,
				FetchFailures:          fr.failures,
				Callers:                callers,
			}
			if n := len(fr.failures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d call history fetch(es) failed (first: recording_status=%s: %s); results are incomplete (see fetch_failures)\n",
					n, fr.failures[0].RecordingStatus, fr.failures[0].Error)
			}
			incomplete := result.Truncated || len(fr.failures) > 0
			switch {
			case incomplete && len(callers) == 0:
				result.Note = "no unreturned callers in the scanned calls, but the page cap or a failed fetch left calls unscanned; raise --max-pages or retry"
			case incomplete:
				result.Note = "the scan is incomplete (page cap or failed fetch); a callback in the unscanned calls would not be seen"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), result, flags); err != nil {
					return err
				}
			} else if err := callbacksPrint(cmd, result); err != nil {
				return err
			}
			// Some fetches failed while others succeeded: the partial result is
			// printed, then exit 6 unless --allow-partial-failure downgrades it.
			if n := len(fr.failures); n > 0 && !flags.allowPartialFailure {
				return partialFailureErr(fmt.Errorf("callbacks: %d call history fetch(es) failed; results are incomplete (see fetch_failures)", n))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "How far back to look (e.g. 24h, 7d, 2w; at most 180d)")
	cmd.Flags().StringVar(&flagExt, "ext", "", "Only misses whose callee extension number matches (e.g. 1001)")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", routingDefaultMaxPages, "Maximum call history pages (300 calls each) to scan, shared by the recorded and non_recorded partitions")
	return cmd
}

// callbacksFetch walks the call history for [from, to] newest window first,
// in windows of at most 30 days (the API's one-month range limit). Each window
// is read twice, once per recording_status partition, alternating pages so a
// page cap never starves one partition. One page budget is shared by every
// walk. Calls are deduplicated by their unique id: adjacent windows share their
// boundary second. A failed walk is recorded and the others continue; an
// authentication failure stops everything (every other request would fail too).
func callbacksFetch(ctx context.Context, c *client.Client, from, to time.Time, maxPages int) callbacksFetchResult {
	res := callbacksFetchResult{
		calls:         make([]callbacksCall, 0),
		pagesByStatus: map[string]int{},
		failures:      make([]callbacksFetchFailure, 0),
	}
	for _, status := range callbacksRecordingStatuses {
		res.pagesByStatus[status] = 0
	}
	seen := map[string]bool{}
	type stream struct {
		status, token string
		pages         int
		done          bool
		seen          map[string]bool // every cursor used by this walk
	}
	for end := to; end.After(from); end = end.Add(-callbacksChunk) {
		start := end.Add(-callbacksChunk)
		if start.Before(from) {
			start = from
		}
		streams := make([]*stream, 0, len(callbacksRecordingStatuses))
		for _, status := range callbacksRecordingStatuses {
			streams = append(streams, &stream{status: status, seen: map[string]bool{}})
		}
		for {
			active := false
			for _, st := range streams {
				if st.done {
					continue
				}
				active = true
				if res.pages >= maxPages {
					res.truncated = true
					return res
				}
				params := map[string]string{
					"from":             start.Format(callbacksTimeFmt),
					"to":               end.Format(callbacksTimeFmt),
					"page_size":        strconv.Itoa(callbacksPageSize),
					"connect_types":    "external",
					"recording_status": st.status,
				}
				if st.token != "" {
					params["next_page_token"] = st.token
				}
				raw, err := c.Get(ctx, "/phone/call_history", params)
				var page struct {
					CallHistory   []callbacksCall `json:"call_history"`
					CallLogs      []callbacksCall `json:"call_logs"`
					NextPageToken string          `json:"next_page_token"`
				}
				if err == nil {
					if derr := json.Unmarshal(raw, &page); derr != nil {
						err = fmt.Errorf("decode /phone/call_history: %w", derr)
					}
				}
				loop := err == nil && page.NextPageToken != "" && st.seen[page.NextPageToken]
				if loop {
					// The page is kept (below); the walk itself is reported as
					// failed because the rest of the partition is unreachable.
					err = routingCursorLoopError("/phone/call_history?recording_status="+st.status, st.pages+1)
				}
				if err != nil && !loop {
					st.done = true
					res.failures = append(res.failures, callbacksFetchFailure{
						RecordingStatus: st.status,
						From:            start.Format(callbacksTimeFmt),
						To:              end.Format(callbacksTimeFmt),
						PagesFetched:    st.pages,
						Error:           routingErrText(err),
					})
					if res.firstErr == nil {
						res.firstErr = err
					}
					var ce *cliError
					if errors.As(classifyAPIErrorOnly(err), &ce) && ce.code == 4 {
						return res
					}
					continue
				}
				res.pages++
				st.pages++
				res.pagesByStatus[st.status]++
				items := page.CallHistory
				if len(items) == 0 {
					items = page.CallLogs
				}
				for _, item := range items {
					key := callbacksCallKey(item)
					if seen[key] {
						continue
					}
					seen[key] = true
					res.calls = append(res.calls, item)
				}
				switch {
				case loop:
					st.done = true
					res.failures = append(res.failures, callbacksFetchFailure{
						RecordingStatus: st.status,
						From:            start.Format(callbacksTimeFmt),
						To:              end.Format(callbacksTimeFmt),
						PagesFetched:    st.pages,
						Error:           err.Error(),
					})
					if res.firstErr == nil {
						res.firstErr = err
					}
				case page.NextPageToken == "":
					st.done = true
				default:
					st.seen[page.NextPageToken] = true
					st.token = page.NextPageToken
				}
			}
			if !active {
				break
			}
		}
	}
	return res
}

// callbacksCallKey is a call's unique id: call_history_uuid on call_history[]
// items, id on the deprecated call_logs[] items. call_id is not unique (one
// call id can span several records), so it is only part of a last-resort key.
func callbacksCallKey(c callbacksCall) string {
	switch {
	case c.CallHistoryUUID != "":
		return "uuid:" + c.CallHistoryUUID
	case c.ID != "":
		return "id:" + c.ID
	}
	return "call:" + c.CallID + "|" + c.StartTime + "|" + c.Direction + "|" + c.CallerDIDNumber + "|" + c.CalleeDIDNumber
}

type callbacksMiss struct {
	call callbacksCall
	at   time.Time
}

// callbacksCompute groups unreturned misses by caller. It returns the callers
// still waiting and the number of qualifying misses seen.
func callbacksCompute(calls []callbacksCall, ext string, now time.Time) ([]callbacksCaller, int) {
	resolvedAt := map[string]time.Time{}
	misses := map[string][]callbacksMiss{}
	missed := 0
	for _, call := range calls {
		at, err := time.Parse(time.RFC3339, call.StartTime)
		if err != nil {
			continue
		}
		switch call.Direction {
		case "outbound":
			if key := zoomPhoneKey(call.CalleeDIDNumber); key != "" && at.After(resolvedAt[key]) {
				resolvedAt[key] = at
			}
		case "inbound":
			if call.HideCallerID {
				continue
			}
			key := zoomPhoneKey(call.CallerDIDNumber)
			if key == "" {
				continue
			}
			if callbacksAnsweredResults[call.CallResult] {
				if at.After(resolvedAt[key]) {
					resolvedAt[key] = at
				}
				continue
			}
			if call.ConnectType != "external" || !callbacksMissResults[call.CallResult] {
				continue
			}
			if ext != "" && call.CalleeExtNumber.String() != ext {
				continue
			}
			missed++
			misses[key] = append(misses[key], callbacksMiss{call: call, at: at})
		}
	}

	callers := make([]callbacksCaller, 0)
	for key, list := range misses {
		open := make([]callbacksMiss, 0, len(list))
		for _, m := range list {
			// A later outbound call to the number, or a later answered call
			// from it, closes every miss before it.
			if r, ok := resolvedAt[key]; ok && r.After(m.at) {
				continue
			}
			open = append(open, m)
		}
		if len(open) == 0 {
			continue
		}
		sort.Slice(open, func(a, b int) bool { return open[a].at.Before(open[b].at) })
		first, last := open[0], open[len(open)-1]
		name := ""
		for i := len(open) - 1; i >= 0 && name == ""; i-- {
			name = open[i].call.CallerName
		}
		callers = append(callers, callbacksCaller{
			CallerNumber:  last.call.CallerDIDNumber,
			CallerName:    name,
			Attempts:      len(open),
			FirstMissedAt: first.at.UTC().Format(time.RFC3339),
			LastMissedAt:  last.at.UTC().Format(time.RFC3339),
			LastResult:    last.call.CallResult,
			LastTarget: callbacksTarget{
				Name:            last.call.CalleeName,
				ExtensionNumber: last.call.CalleeExtNumber.String(),
				ExtensionType:   last.call.CalleeExtType,
				DIDNumber:       last.call.CalleeDIDNumber,
			},
			HoursWaiting: math.Round(now.Sub(first.at).Hours()*10) / 10,
		})
	}
	sort.Slice(callers, func(a, b int) bool {
		if callers[a].LastMissedAt != callers[b].LastMissedAt {
			return callers[a].LastMissedAt > callers[b].LastMissedAt
		}
		return callers[a].CallerNumber < callers[b].CallerNumber
	})
	return callers, missed
}

func callbacksPrint(cmd *cobra.Command, r callbacksResult) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Window %s to %s: scanned %d calls (%d page(s)), %d missed, %d caller(s) still waiting\n",
		r.From, r.To, r.ScannedCalls, r.PagesFetched, r.MissedCalls, len(r.Callers))
	if r.Note != "" {
		fmt.Fprintln(w, "Note: "+r.Note)
	}
	if len(r.Callers) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(r.Callers))
	for _, c := range r.Callers {
		target := c.LastTarget.Name
		if c.LastTarget.ExtensionNumber != "" {
			target = strings.TrimSpace(target + " (ext " + c.LastTarget.ExtensionNumber + ")")
		}
		rows = append(rows, map[string]any{
			"caller":        c.CallerNumber,
			"name":          c.CallerName,
			"attempts":      c.Attempts,
			"last_missed":   c.LastMissedAt,
			"last_target":   target,
			"hours_waiting": c.HoursWaiting,
		})
	}
	fmt.Fprintln(w)
	return printAutoTable(w, rows)
}
