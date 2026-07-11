// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command support helpers for the Zammad-specific hand-code layer.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/store"
)

type zammadStateKind string

const (
	zammadStateOpen     zammadStateKind = "open"
	zammadStatePending  zammadStateKind = "pending"
	zammadStateResolved zammadStateKind = "resolved"
	zammadStateClosed   zammadStateKind = "closed"
	zammadStateMerged   zammadStateKind = "merged"
)

type zammadStateInfo struct {
	ID   string
	Name string
	Kind zammadStateKind
}

type zammadPriorityInfo struct {
	ID     string
	Name   string
	Weight float64
}

var customFieldNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var htmlTagRE = regexp.MustCompile(`<[^>]+>`)
var whitespaceRE = regexp.MustCompile(`\s+`)
var lexiconPatternCache sync.Map

func defaultNovelDBPath(dbPath string) string {
	if strings.TrimSpace(dbPath) != "" {
		return dbPath
	}
	return defaultDBPath("zammad-cli")
}

func openNovelStore(ctx context.Context, dbPath string) (*store.Store, error) {
	db, err := store.OpenReadOnlyContext(ctx, defaultNovelDBPath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'zammad-cli sync' first to populate the local database.", err)
	}
	return db, nil
}

func loadZammadStateCatalog(db *store.Store) (map[string]zammadStateInfo, error) {
	rows, err := db.Query(`SELECT id, name, state_type_id FROM states`)
	if err != nil {
		return nil, fmt.Errorf("querying states: %w", err)
	}
	defer rows.Close()

	catalog := make(map[string]zammadStateInfo)
	for rows.Next() {
		var id, name sql.NullString
		var stateTypeID sql.NullInt64
		if err := rows.Scan(&id, &name, &stateTypeID); err != nil {
			return nil, fmt.Errorf("scanning states: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		stateName := strings.TrimSpace(name.String)
		catalog[id.String] = zammadStateInfo{
			ID:   id.String,
			Name: stateName,
			Kind: classifyZammadState(stateTypeID, stateName),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading states: %w", err)
	}
	if len(catalog) == 0 {
		return fallbackZammadStateCatalog(), nil
	}
	return catalog, nil
}

func classifyZammadState(stateTypeID sql.NullInt64, name string) zammadStateKind {
	if stateTypeID.Valid {
		switch stateTypeID.Int64 {
		case 1, 2:
			return zammadStateOpen
		case 3:
			return zammadStatePending
		case 4:
			return zammadStateResolved
		case 5, 7:
			return zammadStateClosed
		case 6:
			return zammadStateMerged
		}
	}
	return classifyZammadStateName(name)
}

func fallbackZammadStateCatalog() map[string]zammadStateInfo {
	return map[string]zammadStateInfo{
		"1": {ID: "1", Name: "new", Kind: zammadStateOpen},
		"2": {ID: "2", Name: "open", Kind: zammadStateOpen},
		"3": {ID: "3", Name: "pending reminder", Kind: zammadStatePending},
		"4": {ID: "4", Name: "closed", Kind: zammadStateClosed},
		"5": {ID: "5", Name: "merged", Kind: zammadStateMerged},
		"6": {ID: "6", Name: "pending close", Kind: zammadStateResolved},
	}
}

func classifyZammadStateName(name string) zammadStateKind {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case lower == "closed":
		return zammadStateClosed
	case lower == "merged":
		return zammadStateMerged
	case strings.Contains(lower, "pending close"):
		return zammadStateResolved
	case strings.Contains(lower, "pending"):
		return zammadStatePending
	default:
		return zammadStateOpen
	}
}

func zammadStateForID(catalog map[string]zammadStateInfo, stateID sql.NullString) zammadStateInfo {
	if stateID.Valid {
		id := strings.TrimSpace(stateID.String)
		if info, ok := catalog[id]; ok {
			return info
		}
		if id != "" {
			return zammadStateInfo{ID: id, Name: "#" + id, Kind: zammadStateOpen}
		}
	}
	return zammadStateInfo{ID: "", Name: "(unknown)", Kind: zammadStateOpen}
}

func zammadStateActive(kind zammadStateKind) bool {
	return kind != zammadStateClosed && kind != zammadStateMerged
}

func zammadStatePendingish(kind zammadStateKind) bool {
	return kind == zammadStatePending || kind == zammadStateResolved
}

func loadZammadPriorityCatalog(db *store.Store) (map[string]zammadPriorityInfo, error) {
	rows, err := db.Query(`SELECT id, name FROM priorities`)
	if err != nil {
		return nil, fmt.Errorf("querying priorities: %w", err)
	}
	defer rows.Close()

	catalog := make(map[string]zammadPriorityInfo)
	for rows.Next() {
		var id, name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning priorities: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		priorityName := strings.TrimSpace(name.String)
		catalog[id.String] = zammadPriorityInfo{
			ID:     id.String,
			Name:   priorityName,
			Weight: zammadPriorityWeight(id.String, priorityName),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading priorities: %w", err)
	}
	if len(catalog) == 0 {
		return fallbackZammadPriorityCatalog(), nil
	}
	return catalog, nil
}

func fallbackZammadPriorityCatalog() map[string]zammadPriorityInfo {
	return map[string]zammadPriorityInfo{
		"1": {ID: "1", Name: "low", Weight: 0.5},
		"2": {ID: "2", Name: "normal", Weight: 1},
		"3": {ID: "3", Name: "high", Weight: 3},
	}
}

func zammadPriorityForID(catalog map[string]zammadPriorityInfo, priorityID sql.NullString) zammadPriorityInfo {
	if priorityID.Valid {
		id := strings.TrimSpace(priorityID.String)
		if info, ok := catalog[id]; ok {
			return info
		}
		if id != "" {
			return zammadPriorityInfo{ID: id, Name: "#" + id, Weight: zammadPriorityWeight(id, "")}
		}
	}
	return zammadPriorityInfo{ID: "", Name: "(none)", Weight: 1}
}

func zammadPriorityWeight(id, name string) float64 {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(lower, "high"), strings.Contains(lower, "critical"), strings.Contains(lower, "urgent"), strings.Contains(lower, "emergency"):
		return 3
	case strings.Contains(lower, "normal"):
		return 1
	case strings.Contains(lower, "low"):
		return 0.5
	}
	switch strings.TrimSpace(id) {
	case "3":
		return 3
	case "2":
		return 1
	case "1":
		return 0.5
	default:
		return 1
	}
}

func loadZammadAgentNames(db *store.Store) (map[string]string, error) {
	rows, err := db.Query(`SELECT id, login, firstname, lastname, email FROM users`)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	names := make(map[string]string)
	for rows.Next() {
		var id, login, first, last, email sql.NullString
		if err := rows.Scan(&id, &login, &first, &last, &email); err != nil {
			return nil, fmt.Errorf("scanning users: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		display := strings.TrimSpace(strings.TrimSpace(first.String) + " " + strings.TrimSpace(last.String))
		if display == "" {
			display = strings.TrimSpace(login.String)
		}
		if display == "" {
			display = strings.TrimSpace(email.String)
		}
		if display == "" {
			display = "#" + id.String
		}
		names[id.String] = display
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading users: %w", err)
	}
	return names, nil
}

func zammadAgentName(ownerID sql.NullString, names map[string]string) string {
	id := "1"
	if ownerID.Valid && strings.TrimSpace(ownerID.String) != "" {
		id = strings.TrimSpace(ownerID.String)
	}
	if id == "1" {
		return "(unassigned)"
	}
	if name, ok := names[id]; ok && name != "" {
		return name
	}
	return "#" + id
}

func zammadOwnerID(ownerID sql.NullString) string {
	if !ownerID.Valid || strings.TrimSpace(ownerID.String) == "" {
		return "1"
	}
	return strings.TrimSpace(ownerID.String)
}

func loadZammadOrganizationNames(db *store.Store) (map[string]string, map[string]string, error) {
	rows, err := db.Query(`SELECT id, name, data FROM organizations`)
	if err != nil {
		return nil, nil, fmt.Errorf("querying organizations: %w", err)
	}
	defer rows.Close()

	names := make(map[string]string)
	rawData := make(map[string]string)
	for rows.Next() {
		var id, name, data sql.NullString
		if err := rows.Scan(&id, &name, &data); err != nil {
			return nil, nil, fmt.Errorf("scanning organizations: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		orgID := strings.TrimSpace(id.String)
		display := strings.TrimSpace(name.String)
		if display == "" {
			display = "#" + orgID
		}
		names[orgID] = display
		if data.Valid {
			rawData[orgID] = data.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading organizations: %w", err)
	}
	return names, rawData, nil
}

func zammadOrganizationName(orgID sql.NullString, names map[string]string) string {
	if !orgID.Valid || strings.TrimSpace(orgID.String) == "" {
		return "(no organization)"
	}
	id := strings.TrimSpace(orgID.String)
	if name, ok := names[id]; ok && name != "" {
		return name
	}
	return "#" + id
}

func zammadOrganizationID(orgID sql.NullString) string {
	if !orgID.Valid {
		return ""
	}
	return strings.TrimSpace(orgID.String)
}

func parseZammadTime(value string) (time.Time, bool) {
	t := cliutil.ParseStoredTime(value)
	if !t.IsZero() {
		return t, true
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func zammadAgeDays(now time.Time, created time.Time) int {
	if created.IsZero() || created.After(now) {
		return 0
	}
	return int(now.Sub(created).Hours() / 24)
}

func zammadAgeHours(now time.Time, created time.Time) int {
	if created.IsZero() || created.After(now) {
		return 0
	}
	return int(now.Sub(created).Hours())
}

func zammadCloseAtFromData(data string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return ""
	}
	return stringFromAny(obj["close_at"])
}

func zammadTierFromData(data, field string) string {
	if !customFieldNameRE.MatchString(field) {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(obj[field]))
}

func stringFromAny(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(typed)
	}
}

func cleanZammadArticleText(text string) string {
	text = htmlTagRE.ReplaceAllString(text, " ")
	text = cliutil.CleanText(text)
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(text, " "))
}

type lexiconMatch struct {
	Term    string
	Snippet string
	Index   int
}

func scanLexicon(text string, terms []string) []lexiconMatch {
	cleaned := cleanZammadArticleText(text)
	type matchSpan struct {
		term       string
		start, end int
	}
	spans := make([]matchSpan, 0)
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		pattern := cachedLexiconPattern(term)
		indices := pattern.FindStringSubmatchIndex(cleaned)
		if len(indices) < 4 {
			continue
		}
		spans = append(spans, matchSpan{term: term, start: indices[2], end: indices[3]})
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		if spans[i].end != spans[j].end {
			return spans[i].end > spans[j].end
		}
		return spans[i].term < spans[j].term
	})
	matches := make([]lexiconMatch, 0, len(spans))
	lastEnd := -1
	for _, span := range spans {
		if span.start < lastEnd {
			continue
		}
		matches = append(matches, lexiconMatch{
			Term:    span.term,
			Snippet: snippetAround(cleaned, span.start, span.end-span.start, 120),
			Index:   span.start,
		})
		lastEnd = span.end
	}
	return matches
}

func cachedLexiconPattern(term string) *regexp.Regexp {
	if cached, ok := lexiconPatternCache.Load(term); ok {
		return cached.(*regexp.Regexp)
	}
	pattern := regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}\p{M}_])(` + regexp.QuoteMeta(term) + `)(?:$|[^\p{L}\p{N}\p{M}_])`)
	actual, _ := lexiconPatternCache.LoadOrStore(term, pattern)
	return actual.(*regexp.Regexp)
}

func snippetAround(text string, idx, termLen, maxLen int) string {
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	if idx < 0 {
		idx = 0
	}
	center := idx + termLen/2
	start := center - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}
	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet += "..."
	}
	return snippet
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func articlesTableEmpty(db *store.Store) (bool, error) {
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&count); err != nil {
		return false, fmt.Errorf("counting articles: %w", err)
	}
	return count == 0, nil
}
