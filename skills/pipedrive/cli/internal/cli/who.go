// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: contact 360. Hand-authored against the local store.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type whoPerson struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	OrgID   string `json:"org_id,omitempty"`
	OrgName string `json:"org_name,omitempty"`
}

type whoDeal struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
	StageID  int64   `json:"stage_id"`
}

type whoActivity struct {
	ID      string `json:"id"`
	Subject string `json:"subject,omitempty"`
	Type    string `json:"type,omitempty"`
	DueDate string `json:"due_date,omitempty"`
	Done    bool   `json:"done"`
}

type whoNote struct {
	AddTime string `json:"add_time"`
	Excerpt string `json:"excerpt"`
}

type whoMatch struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type whoResult struct {
	Person        whoPerson    `json:"person"`
	OpenDeals     []whoDeal    `json:"open_deals"`
	OpenDealCount int          `json:"open_deal_count"`
	OpenDealValue float64      `json:"open_deal_value"`
	LastActivity  *whoActivity `json:"last_activity"`
	NextActivity  *whoActivity `json:"next_activity"`
	Notes         []whoNote    `json:"notes"`
	OtherMatches  []whoMatch   `json:"other_matches,omitempty"`
}

// pp:data-source local
func newNovelWhoCmd(flags *rootFlags) *cobra.Command {
	var notesN int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "who [name]",
		Short: "One-shot card for a person: org, open deals, last/next activity, recent notes.",
		Long: `Use this command for a full joined profile of one contact (their org, open deals, last/next activity, recent notes) assembled from the local store.
Do NOT use this command for a fuzzy text search across many records to find an entity; use 'search' instead.`,
		Example: strings.Trim(`
  pipedrive-cli who "Jane Doe"
  pipedrive-cli who Acme --notes 5 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a person name is required"))
			}
			name := strings.TrimSpace(strings.Join(args, " "))
			if name == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a person name is required"))
			}
			if notesN < 0 {
				return usageErr(fmt.Errorf("--notes must be >= 0"))
			}

			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "persons") {
				hintIfStale(cmd, db, "persons", flags.maxAge)
			}

			res, found, err := queryWho(cmd.Context(), db.DB(), name, notesN)
			if err != nil {
				return err
			}
			if !found {
				return notFoundErr(fmt.Errorf("no person matching %q in the local store (run 'sync' if it is empty)", name))
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) { renderWhoCard(w, res) })
		},
	}
	cmd.Flags().IntVar(&notesN, "notes", 3, "Number of recent notes to include")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}

// queryWho assembles a contact-360 card for the person best matching name.
// It tries the persons FTS index first, falling back to a name LIKE scan, then
// joins org, open deals, last/next activity, and recent notes. found is false
// when no person matches.
func queryWho(ctx context.Context, db *sql.DB, name string, notesN int) (whoResult, bool, error) {
	matches, err := lookupPersons(ctx, db, name)
	if err != nil {
		return whoResult{}, false, err
	}
	if len(matches) == 0 {
		return whoResult{}, false, nil
	}

	top := matches[0]
	res := whoResult{Person: top, OpenDeals: make([]whoDeal, 0), Notes: make([]whoNote, 0)}

	// Disambiguation: surface up to 5 of the remaining matches.
	for _, m := range matches[1:] {
		if len(res.OtherMatches) >= 5 {
			break
		}
		res.OtherMatches = append(res.OtherMatches, whoMatch{ID: m.ID, Name: m.Name})
	}

	// Organization name (deals carry org_name, but resolve it canonically
	// from the organizations table via the person's org_id).
	if top.OrgID != "" {
		var orgName sql.NullString
		if err := db.QueryRowContext(ctx,
			`SELECT COALESCE(name,'') FROM organizations WHERE id = ?`, top.OrgID).Scan(&orgName); err == nil {
			res.Person.OrgName = nullStr(orgName)
		} else if err != sql.ErrNoRows {
			return whoResult{}, false, fmt.Errorf("looking up organization: %w", err)
		}
	}

	// Open deals for this person.
	dealRows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(value,0), COALESCE(currency,''), stage_id
		FROM deals
		WHERE status='open' AND person_id = ?
		ORDER BY value DESC`, top.ID)
	if err != nil {
		return whoResult{}, false, fmt.Errorf("querying open deals: %w", err)
	}
	for dealRows.Next() {
		var d whoDeal
		var stage sql.NullInt64
		if err := dealRows.Scan(&d.ID, &d.Title, &d.Value, &d.Currency, &stage); err != nil {
			_ = dealRows.Close()
			return whoResult{}, false, fmt.Errorf("scanning open deal: %w", err)
		}
		d.StageID = nullI64(stage)
		res.OpenDeals = append(res.OpenDeals, d)
		res.OpenDealValue += d.Value
	}
	_ = dealRows.Close()
	if err := dealRows.Err(); err != nil {
		return whoResult{}, false, err
	}
	res.OpenDealCount = len(res.OpenDeals)

	// activities.person_id is an INTEGER column; persons.id is TEXT. Compare
	// as text so a numeric or string id both match.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Last activity: most recent done activity (not future-dated), or the
	// latest past-due one. The COALESCE keeps done activities with a NULL or
	// empty due_date eligible ('' sorts before any timestamp).
	if a, ok, err := scanActivity(ctx, db, `
		SELECT id, COALESCE(subject,''), COALESCE(type,''), COALESCE(due_date,''), COALESCE(done,0)
		FROM activities
		WHERE CAST(person_id AS TEXT) = ?
		  AND ((done=1 AND COALESCE(due_date,'') <= ?) OR (due_date IS NOT NULL AND due_date <> '' AND due_date < ?))
		ORDER BY due_date DESC LIMIT 1`, top.ID, now, now); err != nil {
		return whoResult{}, false, err
	} else if ok {
		res.LastActivity = &a
	}

	// Next activity: earliest not-done activity due now or later.
	if a, ok, err := scanActivity(ctx, db, `
		SELECT id, COALESCE(subject,''), COALESCE(type,''), COALESCE(due_date,''), COALESCE(done,0)
		FROM activities
		WHERE CAST(person_id AS TEXT) = ?
		  AND (done IS NULL OR done=0)
		  AND due_date IS NOT NULL AND due_date <> '' AND due_date >= ?
		ORDER BY due_date ASC LIMIT 1`, top.ID, now); err != nil {
		return whoResult{}, false, err
	} else if ok {
		res.NextActivity = &a
	}

	// Recent notes.
	if notesN > 0 {
		noteRows, err := db.QueryContext(ctx, `
			SELECT COALESCE(add_time,''), COALESCE(content,'')
			FROM notes
			WHERE CAST(person_id AS TEXT) = ?
			ORDER BY add_time DESC LIMIT ?`, top.ID, notesN)
		if err != nil {
			return whoResult{}, false, fmt.Errorf("querying notes: %w", err)
		}
		for noteRows.Next() {
			var addTime, content string
			if err := noteRows.Scan(&addTime, &content); err != nil {
				_ = noteRows.Close()
				return whoResult{}, false, fmt.Errorf("scanning note: %w", err)
			}
			res.Notes = append(res.Notes, whoNote{AddTime: addTime, Excerpt: noteExcerpt(content)})
		}
		_ = noteRows.Close()
		if err := noteRows.Err(); err != nil {
			return whoResult{}, false, err
		}
	}

	return res, true, nil
}

// lookupPersons resolves name to candidate persons, FTS first then LIKE. The
// top result is the best match; the rest feed the other_matches disambiguation.
func lookupPersons(ctx context.Context, db *sql.DB, name string) ([]whoPerson, error) {
	// FTS5 MATCH on a quoted phrase (so spaces/punctuation are safe). Errors
	// or no rows fall through to the LIKE scan.
	ftsQuery := `
		SELECT p.id, COALESCE(p.name,''), COALESCE(p.org_id,''), COALESCE(p.data,'')
		FROM persons p
		JOIN persons_fts f ON p.rowid = f.rowid
		WHERE persons_fts MATCH ?
		LIMIT 10`
	if rows, err := db.QueryContext(ctx, ftsQuery, `"`+strings.ReplaceAll(name, `"`, `""`)+`"`); err == nil {
		people, scanErr := scanPersons(rows)
		_ = rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if len(people) > 0 {
			return people, nil
		}
	}

	likeRows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(name,''), COALESCE(org_id,''), COALESCE(data,'')
		FROM persons
		WHERE name LIKE '%'||?||'%'
		ORDER BY length(name) ASC
		LIMIT 10`, name)
	if err != nil {
		return nil, fmt.Errorf("looking up person: %w", err)
	}
	defer likeRows.Close()
	return scanPersons(likeRows)
}

// scanPersons reads (id, name, org_id, data) rows into whoPerson, extracting the
// primary email and phone from the data JSON via extractPrimary (see dupes.go).
func scanPersons(rows *sql.Rows) ([]whoPerson, error) {
	var out []whoPerson
	for rows.Next() {
		var p whoPerson
		var data string
		if err := rows.Scan(&p.ID, &p.Name, &p.OrgID, &data); err != nil {
			return nil, fmt.Errorf("scanning person: %w", err)
		}
		p.Email = extractPrimary(data, "email")
		p.Phone = extractPrimary(data, "phone")
		out = append(out, p)
	}
	return out, rows.Err()
}

// scanActivity runs a single-row activity query and returns ok=false on no rows.
func scanActivity(ctx context.Context, db *sql.DB, query string, args ...any) (whoActivity, bool, error) {
	var a whoActivity
	var done int64
	err := db.QueryRowContext(ctx, query, args...).Scan(&a.ID, &a.Subject, &a.Type, &a.DueDate, &done)
	if err == sql.ErrNoRows {
		return whoActivity{}, false, nil
	}
	if err != nil {
		return whoActivity{}, false, fmt.Errorf("querying activity: %w", err)
	}
	a.Done = done != 0
	return a, true, nil
}

// noteExcerpt strips newlines and truncates a note body to ~140 runes.
func noteExcerpt(content string) string {
	s := strings.Join(strings.Fields(content), " ")
	return truncateRunes(s, 140)
}

func renderWhoCard(w io.Writer, res whoResult) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "Name:\t%s\n", res.Person.Name)
	if res.Person.OrgName != "" {
		fmt.Fprintf(tw, "Org:\t%s\n", res.Person.OrgName)
	}
	if res.Person.Email != "" {
		fmt.Fprintf(tw, "Email:\t%s\n", res.Person.Email)
	}
	if res.Person.Phone != "" {
		fmt.Fprintf(tw, "Phone:\t%s\n", res.Person.Phone)
	}
	fmt.Fprintf(tw, "Open deals:\t%d (%.2f)\n", res.OpenDealCount, res.OpenDealValue)
	if res.LastActivity != nil {
		fmt.Fprintf(tw, "Last activity:\t%s — %s\n", res.LastActivity.DueDate, res.LastActivity.Subject)
	}
	if res.NextActivity != nil {
		fmt.Fprintf(tw, "Next activity:\t%s — %s\n", res.NextActivity.DueDate, res.NextActivity.Subject)
	}
	_ = tw.Flush()

	if len(res.OpenDeals) > 0 {
		fmt.Fprintln(w, "\nOpen deals:")
		dt := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		for _, d := range res.OpenDeals {
			fmt.Fprintf(dt, "  %.0f\t%s\t%s\n", d.Value, d.Currency, truncateRunes(d.Title, 48))
		}
		_ = dt.Flush()
	}
	if len(res.Notes) > 0 {
		fmt.Fprintln(w, "\nRecent notes:")
		for _, n := range res.Notes {
			fmt.Fprintf(w, "  [%s] %s\n", n.AddTime, n.Excerpt)
		}
	}
	if len(res.OtherMatches) > 0 {
		fmt.Fprintln(w, "\nOther matches (disambiguate by id):")
		for _, m := range res.OtherMatches {
			fmt.Fprintf(w, "  id=%s\t%s\n", m.ID, m.Name)
		}
	}
}
