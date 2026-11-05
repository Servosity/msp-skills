// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: permit/block suggestions from hold history.
// pp:data-source auto
// pp:client-call

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/cliutil"
)

type senderSuggestion struct {
	Sender   string `json:"sender"`
	To       string `json:"to"`
	Released int    `json:"released"`
	Rejected int    `json:"rejected"`
	LastSeen string `json:"last_seen"`
	risky    bool
}

type sendersSuggestView struct {
	Since            string             `json:"since"`
	DataSource       string             `json:"data_source"`
	CachedRows       int                `json:"cached_rows"`
	FetchedRows      int                `json:"fetched_rows"`
	MinHolds         int                `json:"min_holds"`
	PermitCandidates []senderSuggestion `json:"permit_candidates"`
	BlockCandidates  []senderSuggestion `json:"block_candidates"`
	PermitOrBlock    []map[string]any   `json:"permit_or_block_payload"`
	WindowComplete   bool               `json:"window_complete"`
	Note             string             `json:"note,omitempty"`
}

func newNovelSendersSuggestCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath, accountCode string
	var minHolds, maxScanPages int

	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "Find senders that keep getting held then released (permit candidates) or keep getting rejected (block candidates)",
		Long: "Use this command to find senders that keep getting held and released (permit candidates) or keep getting rejected (block candidates) over weeks of history. " +
			"Do NOT use it to act on a sender flooding the queue right now; use 'held purge-sender' instead.\n\n" +
			"Release-log rows are cached in the local store every time this command or 'held release-audit' runs from the CLI, so history accumulates beyond any single API " +
			"window; under the MCP server or with MIMECAST_NO_LOCAL_DATA=1 nothing is written and the answer comes from the live window plus any existing cache. A (sender, recipient) pair that was held at least --min-holds times and always released is a narrow permit candidate, unless any of " +
			"those holds was a never-release type (impersonation, malware...), had a high spam-verdict risk, or failed SPF/DKIM/DMARC; one that was always rejected is a block candidate. The output includes a ready payload for 'senders permit-or-block --data'. Review it before applying. " +
			"--data-source local reads only the cache; the default pulls the window first and falls back to the cache if the API is unreachable.",
		Example:     "  mimecast-cli senders suggest --since 30d --min-holds 3 --agent\n  mimecast-cli senders suggest --data-source local --since 90d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "senders suggest")
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if minHolds < 1 {
				return usageErr(fmt.Errorf("--min-holds must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			// Hand-fix (handfixes.json mimecast-read-only-workflows-write-nothing): a read-only tool writes nothing under MCP.
			db, writable, err := openReadOnlyWorkflowStore(ctx, flags, dbPath, mcAccountCode(accountCode))
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
			}
			var live []releaseLog

			view := sendersSuggestView{Since: mcTime(start), MinHolds: minHolds, DataSource: "local"}
			if flags.dataSource != "local" {
				c, fetchErr := flags.newClient()
				var logs []releaseLog
				var page mcPage
				if fetchErr == nil {
					logs, page, fetchErr = fetchReleaseLogs(ctx, c, start, now, mcAccountCode(accountCode), maxScanPages, nil)
				}
				switch {
				case fetchErr == nil:
					if writable {
						if err := cacheReleaseLogs(ctx, db, logs); err != nil {
							return fmt.Errorf("caching release logs: %w", err)
						}
					} else {
						live = logs
					}
					view.FetchedRows, view.DataSource = len(logs), "live+cache"
					view.WindowComplete = !page.Truncated
				case flags.dataSource == "live" || !isNetworkError(fetchErr):
					// Only an unreachable API falls back; auth, permission and
					// quota errors must surface instead of reading as "no candidates".
					return classifyAPIError(cmd.ErrOrStderr(), fetchErr, flags)
				default:
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: Mimecast API unreachable (%v); using the local cache\n", fetchErr)
					view.Note = "API unreachable; results come from the local cache only"
				}
			}
			cached, err := readCachedReleases(ctx, db, start)
			if err != nil {
				return fmt.Errorf("reading release-log cache: %w", err)
			}
			cached = mergeLiveReleases(cached, live)
			view.CachedRows = len(cached)
			view.PermitCandidates, view.BlockCandidates = suggestSenders(cached, minHolds)
			view.PermitOrBlock = make([]map[string]any, 0, len(view.PermitCandidates)+len(view.BlockCandidates))
			for _, s := range view.PermitCandidates {
				view.PermitOrBlock = append(view.PermitOrBlock, map[string]any{"sender": s.Sender, "to": s.To, "action": "permit"})
			}
			for _, s := range view.BlockCandidates {
				view.PermitOrBlock = append(view.PermitOrBlock, map[string]any{"sender": s.Sender, "to": s.To, "action": "block"})
			}
			if !view.WindowComplete && len(view.PermitOrBlock) > 0 {
				// Without the whole window, "always released" may hide an older
				// rejection; list candidates but withhold the ready payload.
				view.PermitOrBlock = view.PermitOrBlock[:0]
				view.Note = "the release-log window was not fully read (scan cap or cache-only); candidates are listed but no payload is emitted; raise --max-scan-pages"
			} else if len(view.PermitOrBlock) > 100 {
				view.PermitOrBlock = view.PermitOrBlock[:100]
				view.Note = "payload capped at 100 pairs (the permit-or-block per-call limit)"
			}
			if len(cached) == 0 {
				switch {
				case flags.dataSource == "local":
					view.Note = "the local cache has no release-log history in the window; drop --data-source local to fetch it"
				case view.FetchedRows == 0 && view.DataSource == "live+cache":
					view.Note = "Mimecast returned no held-message release or reject logs in the window"
				default:
					view.Note = "no release-log history available; check connectivity with 'mimecast-cli doctor'"
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				raw, err := json.Marshal(view)
				if err != nil {
					return err
				}
				// The envelope reports where the rows actually came from.
				return printOutputWithFlagsMetaAndKeep(cmd.OutOrStdout(), raw, flags, map[string]any{"source": view.DataSource}, novelKeys(raw))
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Hold history since %s (%d rows, %s)\n", view.Since, view.CachedRows, view.DataSource)
			fmt.Fprintf(w, "\nPermit candidates (held >= %d times, always released):\n", minHolds)
			for _, s := range view.PermitCandidates {
				fmt.Fprintf(w, "  %s -> %s  (%d releases)\n", s.Sender, s.To, s.Released)
			}
			fmt.Fprintf(w, "\nBlock candidates (held >= %d times, always rejected):\n", minHolds)
			for _, s := range view.BlockCandidates {
				fmt.Fprintf(w, "  %s -> %s  (%d rejections)\n", s.Sender, s.To, s.Rejected)
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "\nnote: %s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "History window (30d, 12w) or an RFC3339 start time")
	cmd.Flags().IntVar(&minHolds, "min-holds", 3, "Minimum holds for a (sender, recipient) pair to be suggested")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum release-log pages (100 entries each) to fetch")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite path for the release-log cache (default: the CLI data dir)")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// suggestSenders aggregates cached release-log rows per (sender, recipient).
func suggestSenders(rows []cachedRelease, minHolds int) (permit, block []senderSuggestion) {
	byPair := map[[2]string]*senderSuggestion{}
	for _, r := range rows {
		for _, to := range r.Recipients {
			key := [2]string{r.Sender, to}
			s := byPair[key]
			if s == nil {
				s = &senderSuggestion{Sender: r.Sender, To: to}
				byPair[key] = s
			}
			s.risky = s.risky || r.Risky
			switch {
			case r.Status == "released":
				s.Released++
			case r.Operator == "":
				// housekeeping expiry ("rejected by housekeeping"): nobody decided; not a block signal
			default:
				s.Rejected++
			}
			if r.Released > s.LastSeen {
				s.LastSeen = r.Released
			}
		}
	}
	permit, block = make([]senderSuggestion, 0), make([]senderSuggestion, 0)
	for _, s := range byPair {
		switch {
		case s.Sender == "" || s.To == "":
		case s.Released >= minHolds && s.Rejected == 0 && !s.risky:
			permit = append(permit, *s)
		case s.Rejected >= minHolds && s.Released == 0:
			block = append(block, *s)
		}
	}
	sort.Slice(permit, func(i, j int) bool {
		return permit[i].Released > permit[j].Released || (permit[i].Released == permit[j].Released && permit[i].Sender < permit[j].Sender)
	})
	sort.Slice(block, func(i, j int) bool {
		return block[i].Rejected > block[j].Rejected || (block[i].Rejected == block[j].Rejected && block[i].Sender < block[j].Sender)
	})
	return permit, block
}
