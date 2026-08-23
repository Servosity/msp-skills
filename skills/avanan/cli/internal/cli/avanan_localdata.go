// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared local-store readers for the Avanan offline commands.
//
// Every offline command needs the same three things: open the mirror, pull
// rows of one resource type, and decode the fields Avanan happens to name
// differently per payload. Centralizing that here keeps the command files
// about their own logic.
//
// SQLite runs on a single connection, so every reader below drains and closes
// its *sql.Rows before any follow-up query. Issuing a second query while a
// parent result set is open is the classic way to deadlock this store.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"avanan-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// avananRow is one decoded record from the local mirror.
type avananRow struct {
	ID   string
	Raw  json.RawMessage
	Obj  map[string]any
	Type string
}

// openMirror opens the local store, or reports the missing-mirror state in the
// shape each output mode expects. A nil *store.Store with a nil error means
// "no mirror yet, caller already emitted the right empty result".
func openMirror(cmd *cobra.Command, ctx context.Context, flags *rootFlags, dbPath string, empty any) (*store.Store, string, bool, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("avanan-cli")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: avanan-cli mirror --since 7d --db %s\n", dbPath, dbPath)
		if !wantsHumanTable(cmd.OutOrStdout(), flags) {
			return nil, dbPath, false, printJSONFiltered(cmd.OutOrStdout(), empty, flags)
		}
		return nil, dbPath, false, nil
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, dbPath, false, fmt.Errorf("opening local store: %w", err)
	}
	return db, dbPath, true, nil
}

// loadResources reads every row of a resource type, fully draining the result
// set before returning so callers may issue follow-up queries.
func loadResources(ctx context.Context, db *store.Store, resourceType string) ([]avananRow, error) {
	rows, err := db.DB().QueryContext(ctx,
		`SELECT id, data FROM resources WHERE resource_type = ? ORDER BY id`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", resourceType, err)
	}

	out := make([]avananRow, 0)
	for rows.Next() {
		var id sql.NullString
		var data sql.NullString
		if err := rows.Scan(&id, &data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning %s: %w", resourceType, err)
		}
		row := avananRow{ID: id.String, Type: resourceType, Raw: json.RawMessage(data.String)}
		if data.Valid {
			_ = json.Unmarshal([]byte(data.String), &row.Obj)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating %s: %w", resourceType, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing %s result set: %w", resourceType, err)
	}
	return out, nil
}

// str pulls the first present string field from a decoded record. Avanan names
// the same concept differently across payloads (senderEmail vs fromEmail vs
// from), so every read goes through a candidate list rather than one key.
func str(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			switch s := v.(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			case float64:
				return fmt.Sprintf("%.0f", s)
			case bool:
				return fmt.Sprintf("%t", s)
			}
		}
		// Support one level of nesting: entity detail is frequently wrapped
		// in entityInfo or entityPayload.
		for _, container := range []string{"entityInfo", "entityPayload", "emailInfo"} {
			if nested, ok := obj[container].(map[string]any); ok {
				if v, ok := nested[k]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
	}
	return ""
}

// Field name candidates, in the order Avanan's payloads tend to use them.
var (
	senderKeys    = []string{"senderEmail", "fromEmail", "from", "sender", "senderAddress"}
	subjectKeys   = []string{"subject", "title", "name"}
	recipientKeys = []string{"recipientEmail", "toEmail", "to", "recipient"}
	eventTypeKeys = []string{"type", "eventType", "detectionType"}
	stateKeys     = []string{"state", "eventState", "status"}
	severityKeys  = []string{"severity", "confidenceIndicator", "riskLevel"}
	saasKeys      = []string{"saas", "saasName", "platform"}
	scopeKeys     = []string{"scope", "customerId", "tenant", "farm"}
	entityIDKeys  = []string{"entityId", "entityID", "id"}
	eventIDKeys   = []string{"eventId", "eventID", "id"}
	timeKeys      = []string{"eventCreated", "createdAt", "created", "date", "eventDate", "receivedTime", "time"}
)

// avananTime parses the timestamp formats Avanan emits. Records whose time
// cannot be parsed return the zero time; callers must treat that as "unknown"
// rather than "epoch", or a single malformed record drags every window
// aggregate to the beginning of time.
func avananTime(obj map[string]any) time.Time {
	raw := str(obj, timeKeys...)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

// emailDomain extracts the domain from an address. Returns "" for anything
// that is not a parseable address, so callers can group the unknowns together
// instead of inventing a domain.
func emailDomain(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if parsed, err := mail.ParseAddress(addr); err == nil {
		addr = parsed.Address
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(strings.Trim(addr[at+1:], "<>() "))
}

// subjectPrefixes are the reply/forward markers stripped before campaign
// grouping. Localized variants are included because Avanan protects
// non-English tenants too.
var subjectPrefixRE = regexp.MustCompile(`(?i)^\s*((re|fw|fwd|aw|wg|tr|rv|sv|vs|antwort|encaminhar)\s*(\[\d+\])?\s*:\s*)+`)

// digitRunRE collapses long digit runs, which phishing campaigns vary
// per-recipient (invoice numbers, tracking IDs) to defeat exact matching.
var digitRunRE = regexp.MustCompile(`\d{2,}`)

// whitespaceRE normalizes runs of whitespace introduced by header folding.
var whitespaceRE = regexp.MustCompile(`\s+`)

// normalizeSubject reduces a subject line to a deterministic campaign key.
//
// Deliberately mechanical: no similarity scoring, no embeddings. Two subjects
// group together only when they are identical after stripping reply markers,
// collapsing digit runs, folding case, and normalizing whitespace. That makes
// every grouping decision explainable and reproducible, which matters when the
// output drives a quarantine.
func normalizeSubject(subject string) string {
	s := strings.TrimSpace(subject)
	if s == "" {
		return ""
	}
	s = subjectPrefixRE.ReplaceAllString(s, "")
	s = digitRunRE.ReplaceAllString(s, "#")
	s = whitespaceRE.ReplaceAllString(s, " ")
	return strings.ToLower(strings.TrimSpace(s))
}

// terminalStates are the event states that need no further analyst action.
var terminalStates = map[string]bool{
	"remediated": true,
	"dismissed":  true,
	"exception":  true,
}

func isUnresolved(state string) bool {
	return !terminalStates[strings.ToLower(strings.TrimSpace(state))]
}

// hintMirrorFreshness reports local-store staleness for the resources `mirror`
// owns.
//
// The generated hint helpers exist, but they tell the user to run `sync` —
// which does not populate events, entities, or exceptions at all. Sending
// someone to the wrong command when their result set is empty is worse than
// saying nothing, so these resources get their own hint naming `mirror`.
func hintMirrorFreshness(cmd *cobra.Command, db *store.Store, resourceType string, maxAge time.Duration) {
	if cmd == nil || db == nil {
		return
	}
	state, err := readSyncHintState(db, resourceType)
	if err != nil {
		return
	}
	if !state.hasState {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"hint: no %s mirrored yet. Run 'avanan-cli mirror --since 7d' before trusting local results.\n",
			mirrorResourceLabel(resourceType))
		return
	}
	if maxAge <= 0 {
		return
	}
	if age := time.Since(state.lastSynced); age > maxAge {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"hint: mirrored %s is %s old, older than --max-age=%s. Run 'avanan-cli mirror --since 7d' to refresh.\n",
			mirrorResourceLabel(resourceType), syncHintRoundAge(age), maxAge)
	}
}

// mirrorResourceLabel turns a namespaced store key back into the word a user
// typed, so the hint reads in the CLI's own vocabulary.
func mirrorResourceLabel(resourceType string) string {
	for name, mapped := range mirrorResourceNames {
		if mapped == resourceType {
			return name
		}
	}
	return resourceType
}
