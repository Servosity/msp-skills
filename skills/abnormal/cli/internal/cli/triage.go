// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: morning triage queue. Hand-authored; preserved across regenerations.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"abnormal-pp-cli/internal/cliutil"
	"abnormal-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// triageItem is one ranked row in the triage queue.
type triageItem struct {
	ThreatID          string `json:"threatId"`
	Subject           string `json:"subject,omitempty"`
	FromAddress       string `json:"fromAddress,omitempty"`
	AttackType        string `json:"attackType,omitempty"`
	AttackStrategy    string `json:"attackStrategy,omitempty"`
	AttackVector      string `json:"attackVector,omitempty"`
	ImpersonatedParty string `json:"impersonatedParty,omitempty"`
	ReceivedTime      string `json:"receivedTime,omitempty"`
	RecipientCount    int    `json:"recipientCount,omitempty"`
	MessageCount      int    `json:"messageCount"`
	Remediated        bool   `json:"remediated"`
	Score             int    `json:"score"`
}

type triageView struct {
	Items          []triageItem `json:"items"`
	ScannedThreats int          `json:"scanned_threats"`
	Since          string       `json:"since"`
	Suppressed     int          `json:"suppressed_remediated"`
	Note           string       `json:"note,omitempty"`
}

// threatDoc mirrors the synced ThreatDetails rows in the local store.
type threatDoc struct {
	ThreatID       string             `json:"threatId"`
	RecipientCount int                `json:"recipientCount"`
	Messages       []threatDocMessage `json:"messages"`
}

type threatDocMessage struct {
	Subject              string `json:"subject"`
	FromAddress          string `json:"fromAddress"`
	ReceivedTime         string `json:"receivedTime"`
	AttackType           string `json:"attackType"`
	AttackStrategy       string `json:"attackStrategy"`
	AttackVector         string `json:"attackVector"`
	ImpersonatedParty    string `json:"impersonatedParty"`
	RemediationStatus    string `json:"remediationStatus"`
	RemediationTimestamp string `json:"remediationTimestamp"`
	AutoRemediated       any    `json:"autoRemediated"`
	PostRemediated       any    `json:"postRemediated"`
}

// attackTypeWeight ranks Abnormal's attack-type taxonomy by triage urgency.
func attackTypeWeight(attackType string) int {
	t := strings.ToLower(attackType)
	switch {
	case strings.Contains(t, "account takeover"), strings.Contains(t, "internal-to-internal"):
		return 40
	case strings.Contains(t, "extortion"):
		return 36
	case strings.Contains(t, "malware"):
		return 34
	case strings.Contains(t, "invoice"), strings.Contains(t, "payment fraud"):
		return 32
	case strings.Contains(t, "credential"):
		return 30
	case strings.Contains(t, "social engineering"):
		return 28
	case strings.Contains(t, "sensitive data"):
		return 26
	case strings.Contains(t, "scam"):
		return 22
	case strings.Contains(t, "phishing"):
		return 24
	case strings.Contains(t, "reconnaissance"):
		return 14
	case strings.Contains(t, "spam"):
		return 6
	case strings.Contains(t, "graymail"):
		return 4
	case t == "":
		return 10
	default:
		return 18
	}
}

// recencyWeight scores how fresh the threat is.
func recencyWeight(received time.Time, now time.Time) int {
	if received.IsZero() {
		return 5
	}
	age := now.Sub(received)
	switch {
	case age <= time.Hour:
		return 40
	case age <= 6*time.Hour:
		return 35
	case age <= 24*time.Hour:
		return 30
	case age <= 72*time.Hour:
		return 20
	case age <= 7*24*time.Hour:
		return 10
	default:
		return 5
	}
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || strings.EqualFold(x, "yes")
	case float64:
		return x != 0
	default:
		return false
	}
}

// messageRemediated reports whether one message looks already-handled.
func messageRemediated(m threatDocMessage) bool {
	if truthy(m.AutoRemediated) || truthy(m.PostRemediated) {
		return true
	}
	if m.RemediationTimestamp != "" {
		return true
	}
	s := strings.ToLower(m.RemediationStatus)
	return s != "" && strings.Contains(s, "remediated") && !strings.Contains(s, "not ")
}

// scoreThreat converts a synced threat doc into a ranked triage item.
func scoreThreat(doc threatDoc, now time.Time) triageItem {
	item := triageItem{ThreatID: doc.ThreatID, RecipientCount: doc.RecipientCount, MessageCount: len(doc.Messages)}
	var newest time.Time
	allRemediated := len(doc.Messages) > 0
	for i, m := range doc.Messages {
		if i == 0 {
			item.Subject = m.Subject
			item.FromAddress = m.FromAddress
			item.AttackType = m.AttackType
			item.AttackStrategy = m.AttackStrategy
			item.AttackVector = m.AttackVector
			item.ImpersonatedParty = m.ImpersonatedParty
			item.ReceivedTime = m.ReceivedTime
		}
		if t, err := time.Parse(time.RFC3339, m.ReceivedTime); err == nil && t.After(newest) {
			newest = t
			item.Subject = m.Subject
			item.FromAddress = m.FromAddress
			item.AttackType = m.AttackType
			item.AttackStrategy = m.AttackStrategy
			item.AttackVector = m.AttackVector
			item.ImpersonatedParty = m.ImpersonatedParty
			item.ReceivedTime = m.ReceivedTime
		}
		if !messageRemediated(m) {
			allRemediated = false
		}
	}
	item.Remediated = allRemediated
	score := attackTypeWeight(item.AttackType) + recencyWeight(newest, now)
	if strings.TrimSpace(item.ImpersonatedParty) != "" && !strings.EqualFold(item.ImpersonatedParty, "none") {
		score += 10
	}
	item.Score = score
	return item
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagTop int
	var flagDB string
	var includeRemediated bool

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Ranked queue of the newest, highest-severity, still-unremediated threats",
		Long: strings.Trim(`
Use this command for the daily ranked "what to work on first" backlog from local SQLite.
Do NOT use it for raw filtered listing; use the generated 'threats' list.
Do NOT use it to confirm a remediation finished; use 'remediate-watch'.

Ranking combines recency, Abnormal attack-type severity, and impersonation
signals; threats whose messages are all remediated are suppressed unless
--include-remediated is set. Run 'sync --resources threats' first.`, "\n"),
		Example: strings.Trim(`
  abnormal-cli triage --since 24h --top 20
  abnormal-cli triage --since 7d --top 50 --agent --select items.threatId,items.attackType,items.score
  abnormal-cli triage --include-remediated --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("triage reads the local store only; no live equivalent — run 'sync --resources threats' then re-run triage"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would rank local synced threats into a triage queue")
				return nil
			}
			since, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since %q: %w", flagSince, err))
			}
			if flagTop <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--top must be a positive integer"))
			}
			if flagDB == "" {
				flagDB = defaultDBPath("abnormal-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), flagDB)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "threats") {
				hintIfStale(cmd, db, "threats", flags.maxAge)
			}
			rows, err := db.DB().QueryContext(cmd.Context(), `
				SELECT id, data FROM resources
				WHERE resource_type IN ('threats', 'threats_threats')`)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()
			now := time.Now().UTC()
			cutoff := now.Add(-since)
			view := triageView{Items: make([]triageItem, 0), Since: flagSince}
			for rows.Next() {
				var id, data string
				if err := rows.Scan(&id, &data); err != nil {
					continue
				}
				var doc threatDoc
				if err := json.Unmarshal([]byte(data), &doc); err != nil {
					continue
				}
				if doc.ThreatID == "" {
					doc.ThreatID = id
				}
				view.ScannedThreats++
				item := scoreThreat(doc, now)
				// Fail-open on purpose: a threat with a missing or unparseable
				// receivedTime stays in the queue (at the lowest recency score)
				// rather than being silently hidden by the --since window.
				if item.ReceivedTime != "" {
					if t, err := time.Parse(time.RFC3339, item.ReceivedTime); err == nil && t.Before(cutoff) {
						continue
					}
				}
				if item.Remediated && !includeRemediated {
					view.Suppressed++
					continue
				}
				view.Items = append(view.Items, item)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning rows: %w", err)
			}
			sort.Slice(view.Items, func(i, j int) bool {
				if view.Items[i].Score != view.Items[j].Score {
					return view.Items[i].Score > view.Items[j].Score
				}
				return view.Items[i].ReceivedTime > view.Items[j].ReceivedTime
			})
			if len(view.Items) > flagTop {
				view.Items = view.Items[:flagTop]
			}
			if len(view.Items) == 0 {
				view.Note = fmt.Sprintf("scanned %d local threats without finding an unremediated one in the last %s; widen --since, pass --include-remediated, or run 'sync --resources threats'", view.ScannedThreats, flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Only rank threats received within this window (e.g. 6h, 24h, 7d)")
	cmd.Flags().IntVar(&flagTop, "top", 20, "Maximum ranked threats to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (defaults to the CLI's synced store)")
	cmd.Flags().BoolVar(&includeRemediated, "include-remediated", false, "Include threats whose messages are all remediated")
	return cmd
}
