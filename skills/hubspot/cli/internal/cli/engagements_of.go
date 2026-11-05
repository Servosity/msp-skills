// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: unified engagement timeline for a contact/deal/company.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/client"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelEngagementsOfCmd(flags *rootFlags) *cobra.Command {
	var since string
	var typesCSV string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "of [target]",
		Short:       "Timeline for target like contact:123, deal:456, or company:789",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     `  hubspot-cli engagements of deal:456 --since 30d`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			targetType, targetID, err := parseTargetSpec(args[0])
			if err != nil {
				return err
			}
			cutoff := ""
			if since != "" {
				cutoff, err = parseDurationOrTimestamp(since)
				if err != nil {
					return err
				}
			}
			engTypes := []string{"calls", "emails", "meetings", "notes", "tasks"}
			if typesCSV != "" {
				engTypes, err = parseEngagementTypes(typesCSV)
				if err != nil {
					return usageErr(err)
				}
			}

			if dbPath == "" {
				dbPath = defaultDBPath("hubspot-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			items, source, err := loadEngagementsFor(cmd, db, flags, targetType, targetID, engTypes, cutoff)
			if err != nil {
				return err
			}

			sort.SliceStable(items, func(i, j int) bool { return hsNewerFirst(items[i].Timestamp, items[j].Timestamp) })

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"target":      args[0],
					"data_source": source,
					"results":     items,
				})
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "data_source: %s\n", source)
			headers := []string{"type", "id", "timestamp", "title_or_snippet", "outcome"}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				rows = append(rows, []string{it.Type, it.ID, it.Timestamp, snippet(it.Title, 80), it.Outcome})
			}
			return flags.printTabular(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only include engagements newer than this (e.g. 30d, 4h, RFC3339)")
	cmd.Flags().StringVar(&typesCSV, "type", "", "Limit engagement types (calls,emails,meetings,notes,tasks)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func parseTargetSpec(arg string) (string, string, error) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", fmt.Errorf("target must be type:id (e.g. deal:123); got %q", arg)
	}
	t := strings.TrimSpace(parts[0])
	switch t {
	case "contact":
		return "contacts", parts[1], nil
	case "deal":
		return "deals", parts[1], nil
	case "company":
		return "companies", parts[1], nil
	default:
		return "", "", fmt.Errorf("unknown target type %q (expected contact, deal, company)", t)
	}
}

// parseEngagementTypes reads --type. An unknown or empty type is refused: a
// skipped type would make an empty timeline look complete.
func parseEngagementTypes(csv string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range splitCSV(csv) {
		if _, ok := engagementTables[t]; !ok {
			return nil, fmt.Errorf("unknown engagement type %q in --type (expected calls, emails, meetings, notes, tasks)", t)
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--type %q names no engagement type (expected calls, emails, meetings, notes, tasks)", csv)
	}
	return out, nil
}

type engagementItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Title     string `json:"title_or_snippet"`
	Outcome   string `json:"outcome,omitempty"`
}

func loadEngagementsFor(cmd *cobra.Command, db *store.Store, flags *rootFlags, targetType, targetID string, engTypes []string, cutoff string) ([]engagementItem, string, error) {
	// First try local hubspot_associations join.
	assocCount, err := hsAssociationCount(db)
	if err != nil {
		return nil, "", err
	}
	hasAssoc := assocCount > 0

	if hasAssoc {
		items, err := loadEngagementsLocal(cmd, db, targetType, targetID, engTypes, cutoff)
		if err != nil {
			return nil, "", fmt.Errorf("reading local engagements: %w", err)
		}
		if len(items) > 0 || flags.dataSource == "local" {
			return items, "local", nil
		}
	}
	// --data-source local never reaches the API, even when the mirror is empty.
	if flags.dataSource == "local" {
		if !hasAssoc {
			// No association rows means the store cannot say which
			// engagements belong to the target; an empty timeline would
			// claim it has none.
			return nil, "", fmt.Errorf("engagements of --data-source local needs engagement associations to find the target's engagements, and %s, so it cannot list a local timeline; rerun without --data-source local to read it live", hsAssociationsNotCollected)
		}
		return []engagementItem{}, "local", nil
	}

	c, err := flags.newClient()
	if err != nil {
		return nil, "", err
	}
	items, err := loadEngagementsLive(cmd.Context(), c, targetType, targetID, engTypes, cutoff)
	if err != nil {
		return nil, "", err
	}
	return items, "live", nil
}

func loadEngagementsLocal(cmd *cobra.Command, db *store.Store, targetType, targetID string, engTypes []string, cutoff string) ([]engagementItem, error) {
	items := []engagementItem{}
	for _, et := range engTypes {
		info, ok := engagementTables[et]
		if !ok {
			return nil, fmt.Errorf("unknown engagement type %q", et)
		}
		if err := hsRequireCollected(cmd.Context(), db, "engagements of", strings.ReplaceAll(info.Table, "_", "-")); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
			return nil, err
		}
		q := fmt.Sprintf(`
SELECT e.id,
  COALESCE(e.updated_at, e.created_at, '') AS ts,
  COALESCE(
    json_extract(e.data, '$.properties.hs_engagement_title'),
    json_extract(e.data, '$.properties.hs_task_subject'),
    json_extract(e.data, '$.properties.hs_meeting_title'),
    json_extract(e.data, '$.properties.hs_email_subject'),
    json_extract(e.data, '$.properties.hs_call_title'),
    json_extract(e.data, '$.properties.hs_note_body'),
    ''
  ) AS title,
  COALESCE(
    json_extract(e.data, '$.properties.hs_call_disposition'),
    json_extract(e.data, '$.properties.hs_meeting_outcome'),
    json_extract(e.data, '$.properties.hs_task_status'),
    ''
  ) AS outcome
FROM %s e
JOIN hubspot_associations a
  ON a.from_type = ? AND a.from_id = e.id
 AND a.to_type = ? AND a.to_id = ?
WHERE (? = '' OR julianday(ts) > julianday(?))
ORDER BY julianday(ts) DESC`, info.Table)
		rows, err := db.DB().QueryContext(cmd.Context(), q, et, targetType, targetID, cutoff, cutoff)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, ts, title, outcome sql.NullString
			if err := rows.Scan(&id, &ts, &title, &outcome); err != nil {
				_ = rows.Close()
				return nil, err
			}
			items = append(items, engagementItem{
				Type: et, ID: nullStr(id), Timestamp: nullStr(ts),
				Title: stripHTML(nullStr(title)), Outcome: nullStr(outcome),
			})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterating %s rows: %w", et, err)
		}
		_ = rows.Close()
	}
	return items, nil
}

// hsAssocPageLimit is the documented maximum page size of the v4
// association list (GET /crm/v4/objects/{objectType}/{objectId}/associations/
// {toObjectType}); hsAssocMaxPages caps one target's walk per type. Hitting
// the cap FAILS the command: a truncated list is not a timeline.
const (
	hsAssocPageLimit = 500
	hsAssocMaxPages  = 200
)

// listAssociatedIDs walks every page of a record's associations to toType,
// following paging.next.after until HubSpot returns no next page. Any HTTP
// or decode failure, a cursor that does not advance, or the page cap fails
// the walk; it never returns a partial list.
func listAssociatedIDs(ctx context.Context, c *client.Client, fromType, fromID, toType string) ([]string, error) {
	listPath := fmt.Sprintf("/crm/v4/objects/%s/%s/associations/%s", fromType, fromID, toType)
	what := fmt.Sprintf("%s associated with %s %s", toType, fromType, fromID)
	ids := []string{}
	seen := map[string]bool{}
	after := ""
	for page := 1; ; page++ {
		if page > hsAssocMaxPages {
			return nil, fmt.Errorf("listing %s: more than %d pages of %d associations; refusing to report a partial timeline", what, hsAssocMaxPages, hsAssocPageLimit)
		}
		params := map[string]string{"limit": fmt.Sprintf("%d", hsAssocPageLimit)}
		if after != "" {
			params["after"] = after
		}
		body, err := c.Get(ctx, listPath, params)
		if err != nil {
			return nil, fmt.Errorf("listing %s (page %d): %w", what, page, err)
		}
		var resp struct {
			Results []struct {
				ToObjectID json.RawMessage `json:"toObjectId"`
			} `json:"results"`
			Paging *struct {
				Next *struct {
					After string `json:"after"`
				} `json:"next"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decoding %s (page %d): %w", what, page, err)
		}
		for _, r := range resp.Results {
			// toObjectId is an integer in the v4 schema. Read it verbatim:
			// decoding into float64 renders an 11-digit id as 1.2e+10.
			id := strings.Trim(strings.TrimSpace(string(r.ToObjectID)), `"`)
			if id == "" || id == "null" {
				return nil, fmt.Errorf("decoding %s (page %d): an association has no toObjectId", what, page)
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if resp.Paging == nil || resp.Paging.Next == nil || resp.Paging.Next.After == "" {
			return ids, nil
		}
		if resp.Paging.Next.After == after {
			return nil, fmt.Errorf("listing %s (page %d): paging cursor %q did not advance; refusing to report a partial timeline", what, page, after)
		}
		after = resp.Paging.Next.After
	}
}

// loadEngagementsLive reads the target's full timeline live. Every
// association page and every engagement record must be read: any failure
// fails the command, never a successful partial timeline.
func loadEngagementsLive(ctx context.Context, c *client.Client, targetType, targetID string, engTypes []string, cutoff string) ([]engagementItem, error) {
	items := []engagementItem{}
	for _, et := range engTypes {
		info, ok := engagementTables[et]
		if !ok {
			return nil, fmt.Errorf("unknown engagement type %q", et)
		}
		ids, err := listAssociatedIDs(ctx, c, targetType, targetID, info.Path)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			detail, err := c.Get(ctx, fmt.Sprintf("/crm/v3/objects/%s/%s", info.Path, id), map[string]string{"properties": hsCollectedProperties(strings.ReplaceAll(info.Table, "_", "-"), "")}) // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
			if err != nil {
				return nil, fmt.Errorf("reading %s %s associated with %s %s: %w", et, id, targetType, targetID, err)
			}
			var obj struct {
				ID         string            `json:"id"`
				UpdatedAt  string            `json:"updatedAt"`
				CreatedAt  string            `json:"createdAt"`
				Properties map[string]string `json:"properties"`
			}
			if err := json.Unmarshal(detail, &obj); err != nil {
				return nil, fmt.Errorf("decoding %s %s: %w", et, id, err)
			}
			if obj.ID == "" {
				obj.ID = id
			}
			ts := obj.UpdatedAt
			if ts == "" {
				ts = obj.CreatedAt
			}
			// Same rule as the local path's julianday(ts) > julianday(cutoff):
			// strictly newer, and an empty or unparseable timestamp is never
			// counted as newer than the cutoff.
			if cutoff != "" && !hsTimeAfter(ts, cutoff) {
				continue
			}
			title := firstNonEmptyString(
				obj.Properties["hs_engagement_title"],
				obj.Properties["hs_task_subject"],
				obj.Properties["hs_meeting_title"],
				obj.Properties["hs_email_subject"],
				obj.Properties["hs_call_title"],
				obj.Properties["hs_note_body"],
			)
			outcome := firstNonEmptyString(
				obj.Properties["hs_call_disposition"],
				obj.Properties["hs_meeting_outcome"],
				obj.Properties["hs_task_status"],
			)
			items = append(items, engagementItem{
				Type:      et,
				ID:        obj.ID,
				Timestamp: ts,
				Title:     stripHTML(title),
				Outcome:   outcome,
			})
		}
	}
	return items, nil
}
