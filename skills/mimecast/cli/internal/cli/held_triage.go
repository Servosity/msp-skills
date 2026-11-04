// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: hold-queue triage sheet.
// pp:data-source live
// pp:client-call

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/cliutil"
	"mimecast-pp-cli/internal/store"
)

type triageGroup struct {
	Key           string     `json:"key"`
	Count         int        `json:"count"`
	NewCount      int        `json:"new_count"`
	RiskClass     string     `json:"risk_class"`
	RiskBasis     string     `json:"risk_basis"`
	HeldGroups    []string   `json:"held_groups,omitempty"`
	SampleSubject []string   `json:"sample_subjects,omitempty"`
	IDBatches     [][]string `json:"id_batches"`
	NextCommand   string     `json:"next_command"`
}

type triageView struct {
	Since        string        `json:"since"`
	GroupBy      string        `json:"group_by"`
	NewOnly      bool          `json:"new_only"`
	ScannedHeld  int           `json:"scanned_held"`
	MaxScanPages int           `json:"max_scan_pages"`
	Truncated    bool          `json:"truncated"`
	Groups       []triageGroup `json:"groups"`
	Note         string        `json:"note,omitempty"`
}

func newNovelHeldTriageCmd(flags *rootFlags) *cobra.Command {
	var since, groupBy, dbPath, accountCode string
	var newOnly bool
	var maxScanPages, limit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "See the whole hold queue grouped by hold reason, policy or sender domain",
		Long: "Use this command to review the whole hold queue grouped by hold reason, policy, or sender domain with a release-risk class per group. " +
			"Do NOT use it to explain one user's missing message; use 'why' instead. Do NOT use it to clear a single sender's flood; use 'held purge-sender' instead.\n\n" +
			"Risk classes come from a fixed rule table over the hold reason and policy: never-release (impersonation, malware, sandbox, phishing, URL/attachment protect), " +
			"likely-junk (spam/graymail/bulk with a high spam score), policy-review (content, DLP, attachment, size, recipient policies) and review (everything else). " +
			"--new keeps a local snapshot of seen IDs and shows only messages not seen on the previous run.",
		Example:     "  mimecast-cli held triage --since 24h --group-by reason --agent\n  mimecast-cli held triage --since 24h --new --agent\n  mimecast-cli held triage --group-by sender-domain --limit 10",
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:read-only-unless-flags": "new", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "held triage")
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			switch groupBy {
			case "reason", "policy", "sender-domain":
			default:
				return usageErr(fmt.Errorf("--group-by must be reason, policy or sender-domain (got %q)", groupBy))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			held, page, err := fetchHeld(ctx, c, start, now, mcAccountCode(accountCode), maxScanPages, nil)
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}

			seen := map[string]bool{}
			var db *store.Store
			if newOnly {
				db, err = openAccountStore(ctx, flags, dbPath, mcAccountCode(accountCode))
				if err != nil {
					return err
				}
				defer db.Close()
				if seen, err = loadSeen(ctx, db, held); err != nil {
					return err
				}
			}

			view := buildTriage(held, groupBy, seen, newOnly)
			view.Since = mcTime(start)
			view.ScannedHeld = len(held)
			view.MaxScanPages = maxScanPages
			view.Truncated = page.Truncated
			if limit > 0 && len(view.Groups) > limit {
				view.Groups = view.Groups[:limit]
			}
			if newOnly {
				// Mark seen only what this run actually shows; messages in
				// groups cut by --limit stay new for the next run.
				shown := map[string]bool{}
				for _, g := range view.Groups {
					for _, b := range g.IDBatches {
						for _, id := range b {
							shown[id] = true
						}
					}
				}
				emitted := make([]heldMessage, 0, len(shown))
				for _, h := range held {
					if shown[h.ID] {
						emitted = append(emitted, h)
					}
				}
				if err := recordSeen(ctx, db, emitted, now); err != nil {
					return err
				}
			}
			switch {
			case len(held) == 0:
				view.Note = "no held messages in the window"
			case len(view.Groups) == 0 && newOnly:
				view.Note = "no held messages that were not already seen on a previous --new run"
			}
			if n := page.note("raise --max-scan-pages to see the whole queue"); n != "" {
				view.Note = strings.TrimSpace(view.Note + " " + n)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			return printTriageHuman(cmd, view)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Lookback window (24h, 7d) or an RFC3339 start time")
	cmd.Flags().StringVar(&groupBy, "group-by", "reason", "Group by: reason, policy or sender-domain")
	cmd.Flags().BoolVar(&newOnly, "new", false, "Only show held messages not seen on the previous --new run (keeps a local snapshot)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum hold-queue pages (100 messages each) to scan")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum groups to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite path for the --new snapshot (default: the CLI data dir)")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// recordSeen upserts the current queue IDs into the snapshot table.
func recordSeen(ctx context.Context, db *store.Store, held []heldMessage, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, h := range held {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mimecast_held_seen (id, first_seen, last_seen, reason) VALUES (?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen`, h.ID, ts, ts, h.HeldReason); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording triage snapshot: %w", err)
		}
	}
	return tx.Commit()
}

// buildTriage groups held messages and sorts groups most-dangerous first,
// then by size.
func buildTriage(held []heldMessage, groupBy string, seen map[string]bool, newOnly bool) triageView {
	type acc struct {
		g        triageGroup
		ids      []string
		groups   map[string]bool
		bestRank int
	}
	byKey := map[string]*acc{}
	var order []string
	for _, h := range held {
		isNew := !seen[h.ID]
		if newOnly && !isNew {
			continue
		}
		key := triageKey(h, groupBy)
		a := byKey[key]
		if a == nil {
			a = &acc{g: triageGroup{Key: key}, groups: map[string]bool{}, bestRank: 99}
			byKey[key] = a
			order = append(order, key)
		}
		a.g.Count++
		if isNew {
			a.g.NewCount++
		}
		a.ids = append(a.ids, h.ID)
		if h.HeldGroup != "" {
			a.groups[h.HeldGroup] = true
		}
		if len(a.g.SampleSubject) < 3 && h.Subject != "" {
			a.g.SampleSubject = append(a.g.SampleSubject, h.Subject)
		}
		class, basis := h.risk()
		if r := riskRank(class); r < a.bestRank {
			a.bestRank, a.g.RiskClass, a.g.RiskBasis = r, class, basis
		}
	}
	view := triageView{GroupBy: groupBy, NewOnly: newOnly, Groups: make([]triageGroup, 0, len(order))}
	for _, k := range order {
		a := byKey[k]
		a.g.HeldGroups = slices.Sorted(maps.Keys(a.groups))
		a.g.IDBatches = batches(a.ids, 25)
		a.g.NextCommand = triageNextCommand(a.g.RiskClass, a.g.IDBatches)
		view.Groups = append(view.Groups, a.g)
	}
	sort.SliceStable(view.Groups, func(i, j int) bool {
		ri, rj := riskRank(view.Groups[i].RiskClass), riskRank(view.Groups[j].RiskClass)
		if ri != rj {
			return ri < rj
		}
		return view.Groups[i].Count > view.Groups[j].Count
	})
	return view
}

func triageKey(h heldMessage, groupBy string) string {
	var k string
	switch groupBy {
	case "policy":
		k = h.PolicyInfo
	case "sender-domain":
		k = addrDomain(h.sender())
	default:
		k = h.HeldReason
	}
	if strings.TrimSpace(k) == "" {
		return "(unknown)"
	}
	return k
}

func triageNextCommand(class string, idBatches [][]string) string {
	if len(idBatches) == 0 {
		return ""
	}
	ids := make([]string, 0, len(idBatches[0]))
	for _, id := range idBatches[0] {
		ids = append(ids, shellQuote(id))
	}
	first := strings.Join(ids, " ")
	switch class {
	case riskLikelyJunk:
		payload, _ := json.Marshal([]map[string]any{{"ids": idBatches[0], "reasonType": "MESSAGE CONTAINS UNDESIRABLE CONTENT"}})
		return "mimecast-cli held reject --data " + shellQuote(string(payload))
	case riskNeverRelease:
		return "mimecast-cli why " + ids[0] + "   # investigate; release-safe refuses this class without --override"
	case riskPolicyReview:
		return "mimecast-cli held release-safe " + first + " --ticket <ticket> --override \"<what you reviewed>\""
	default:
		return "mimecast-cli held release-safe " + first + " --ticket <ticket>"
	}
}

func printTriageHuman(cmd *cobra.Command, view triageView) error {
	w := cmd.OutOrStdout()
	if len(view.Groups) == 0 {
		fmt.Fprintf(w, "Hold queue since %s: %s\n", view.Since, view.Note)
		return nil
	}
	fmt.Fprintf(w, "Hold queue since %s: %d held, %d groups by %s\n\n", view.Since, view.ScannedHeld, len(view.Groups), view.GroupBy)
	for _, g := range view.Groups {
		fmt.Fprintf(w, "%-14s %4d (%d new)  %s\n", g.RiskClass, g.Count, g.NewCount, g.Key)
		for _, s := range g.SampleSubject {
			fmt.Fprintf(w, "    - %s\n", s)
		}
		fmt.Fprintf(w, "    next: %s\n", g.NextCommand)
	}
	if view.Note != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nnote: %s\n", view.Note)
	}
	return nil
}

// loadSeen returns which of the current held IDs earlier --new runs showed,
// querying only those IDs rather than the whole (ever-growing) table.
func loadSeen(ctx context.Context, db *store.Store, held []heldMessage) (map[string]bool, error) {
	seen := map[string]bool{}
	ids := make([]any, 0, len(held))
	for _, h := range held {
		ids = append(ids, h.ID)
	}
	for chunk := range slices.Chunk(ids, 500) {
		q := "SELECT id FROM mimecast_held_seen WHERE id IN (?" + strings.Repeat(",?", len(chunk)-1) + ")"
		rows, err := db.DB().QueryContext(ctx, q, chunk...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			seen[id] = true
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return seen, nil
}
