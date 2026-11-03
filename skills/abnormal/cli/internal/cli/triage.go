// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	// Remediated: every SYNCED message of the threat is remediated. The
	// detail may not hold every message (see the triage ordering note), so a
	// remediated threat is listed after the open ones, never hidden.
	Remediated bool `json:"remediated"`
	// InWindow: the newest synced message falls inside --since (or has no
	// parseable time). An out-of-window threat is listed last, never hidden.
	InWindow bool `json:"in_window"`
	Score    int  `json:"score"`
}

type triageView struct {
	Items          []triageItem `json:"items"`
	ScannedThreats int          `json:"scanned_threats"`
	Since          string       `json:"since"`
	// RemediatedListedLast and OutsideWindow count threats ordered after
	// the in-window unremediated ones (triage hides nothing).
	RemediatedListedLast int    `json:"remediated_listed_last"`
	OutsideWindow        int    `json:"outside_window"`
	Note                 string `json:"note,omitempty"`
}

// threatDoc mirrors the synced ThreatDetails rows in the local store.
type threatDoc struct {
	ThreatID       string             `json:"threatId"`
	RecipientCount int                `json:"recipientCount"`
	Messages       []threatDocMessage `json:"messages"`
}

type threatDocMessage struct {
	ThreatID             string `json:"threatId"`
	ParentID             string `json:"parent_id"`
	AbxMessageIDStr      string `json:"abxMessageIdStr"`
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

// Triage ordering note (Codex r1 and r2 class
// incomplete-threat-detail-read-as-complete): GET /threats/{threat_id} returns
// at most ten messages (spec.json: pageNumber "Currently ignored, only 10
// results for messages will show") and accepts pageSize, so sync can store as
// few as one message of a campaign, and recipientCount counts recipients, not
// messages. No count of stored messages proves a campaign complete, so triage
// never hides a threat on stored-message evidence: remediated-looking and
// out-of-window threats are ordered after the open in-window ones instead.

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
signals; nothing is hidden: threats whose synced messages are all remediated, and
threats older than --since, are listed after the open in-window ones (a
synced threat detail may not hold every message of the campaign).
Run 'sync --resources threats' first.`, "\n"),
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
			// One doc per threat. The threats list stores bare {threatId}
			// rows; sync's detail walker (threats_get, 4.34.x) stores each
			// message of GET /threats/{threat_id} as its own row (a message
			// names its threatId); 4.24 stored whole detail docs
			// (threats_threats). Messages merge per threat, once each.
			rows, err := db.DB().QueryContext(cmd.Context(), `
				SELECT resource_type, id, data FROM resources
				WHERE resource_type IN ('threats', 'threats_threats', 'threats_get')`)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()
			now := time.Now().UTC()
			cutoff := now.Add(-since)
			view := triageView{Items: make([]triageItem, 0), Since: flagSince}
			docs := map[string]*threatDoc{}
			seenMsg := map[string]map[string]bool{}
			var order []string
			docFor := func(threatID string) *threatDoc {
				if d, ok := docs[threatID]; ok {
					return d
				}
				d := &threatDoc{ThreatID: threatID}
				docs[threatID] = d
				seenMsg[threatID] = map[string]bool{}
				order = append(order, threatID)
				return d
			}
			addMessage := func(d *threatDoc, m threatDocMessage) {
				key := m.AbxMessageIDStr
				if key == "" {
					b, _ := json.Marshal(m)
					key = string(b)
				}
				if !seenMsg[d.ThreatID][key] {
					seenMsg[d.ThreatID][key] = true
					d.Messages = append(d.Messages, m)
				}
			}
			for rows.Next() {
				var rtype, id, data string
				if err := rows.Scan(&rtype, &id, &data); err != nil {
					continue
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(data), &fields); err != nil {
					continue
				}
				if _, whole := fields["messages"]; whole || rtype != "threats_get" {
					var doc threatDoc
					if err := json.Unmarshal([]byte(data), &doc); err != nil {
						continue
					}
					if doc.ThreatID == "" {
						doc.ThreatID = id
					}
					d := docFor(doc.ThreatID)
					if doc.RecipientCount > d.RecipientCount {
						d.RecipientCount = doc.RecipientCount
					}
					for _, m := range doc.Messages {
						addMessage(d, m)
					}
					continue
				}
				var m threatDocMessage
				if err := json.Unmarshal([]byte(data), &m); err != nil {
					continue
				}
				threatID := m.ThreatID
				if threatID == "" {
					threatID = m.ParentID
				}
				if threatID == "" {
					continue
				}
				addMessage(docFor(threatID), m)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning rows: %w", err)
			}
			// tier orders, never filters: in-window open threats first, then
			// in-window remediated ones (unless --include-remediated ranks
			// them by score), then out-of-window threats the same way.
			tier := func(it triageItem) int {
				n := 0
				if !it.InWindow {
					n += 2
				}
				if it.Remediated && !includeRemediated {
					n++
				}
				return n
			}
			for _, threatID := range order {
				doc := *docs[threatID]
				view.ScannedThreats++
				item := scoreThreat(doc, now)
				// A missing or unparseable receivedTime counts as in window
				// (at the lowest recency score).
				item.InWindow = true
				if t, err := time.Parse(time.RFC3339, item.ReceivedTime); err == nil && t.Before(cutoff) {
					item.InWindow = false
					view.OutsideWindow++
				}
				if item.Remediated && !includeRemediated {
					view.RemediatedListedLast++
				}
				view.Items = append(view.Items, item)
			}
			sort.SliceStable(view.Items, func(i, j int) bool {
				if ti, tj := tier(view.Items[i]), tier(view.Items[j]); ti != tj {
					return ti < tj
				}
				if view.Items[i].Score != view.Items[j].Score {
					return view.Items[i].Score > view.Items[j].Score
				}
				return view.Items[i].ReceivedTime > view.Items[j].ReceivedTime
			})
			if len(view.Items) > flagTop {
				view.Items = view.Items[:flagTop]
			}
			if len(view.Items) == 0 {
				view.Note = "no local threats; run 'sync --resources threats'"
			} else if tier(view.Items[0]) != 0 {
				view.Note = fmt.Sprintf("no unremediated threat received in the last %s; the list shows remediated or older threats (synced messages may not be the whole campaign)", flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Rank threats received within this window first; older ones are listed after them (e.g. 6h, 24h, 7d)")
	cmd.Flags().IntVar(&flagTop, "top", 20, "Maximum ranked threats to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (defaults to the CLI's synced store)")
	cmd.Flags().BoolVar(&includeRemediated, "include-remediated", false, "Rank threats whose synced messages are all remediated by score, not after the open ones")
	return cmd
}
