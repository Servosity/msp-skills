// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type corkWarranty struct {
	UUID       string `json:"uuid"`
	Active     bool   `json:"active"`
	ClientUUID string `json:"client_uuid"`
	ClientName string `json:"client_name"`
	Package    string `json:"package"`
	StartDate  string `json:"start_date"`
}

type warrantyExposureRow struct {
	ClientUUID     string `json:"client_uuid"`
	Client         string `json:"client"`
	WarrantyStatus string `json:"warranty_status"`
	Covered        bool   `json:"covered"`
	Package        string `json:"package,omitempty"`
	Score          int    `json:"score"`
	Trend          int    `json:"trend"`
	Reason         string `json:"reason"`
}

type warrantyExposureView struct {
	Items          []warrantyExposureRow `json:"items"`
	ClientsScanned int                   `json:"clients_scanned"`
	Uncovered      int                   `json:"uncovered"`
	WarrantiesSeen int                   `json:"warranties_seen"`
	ScanCapHit     bool                  `json:"scan_cap_hit"`
	Note           string                `json:"note,omitempty"`
}

func newNovelWarrantiesExposureCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var flagIncludeCovered bool
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Rank unwarranted or lapsed clients by current risk so coverage conversations start with the ones that need it most.",
		Long: "Rank unwarranted or lapsed clients by current risk so coverage conversations\n" +
			"start with the ones that need it most.\n\n" +
			"Warranty state, client warranty status, and score trend live on three separate\n" +
			"paths, so this ranking exists in no single Cork response and is normally a\n" +
			"manual spreadsheet merge that goes stale as soon as a score moves.\n\n" +
			"Use this command to rank clients for coverage conversations. Do NOT use this\n" +
			"command to inspect warranty records or billing detail; use 'warranties' and\n" +
			"'invoices' instead.",
		Example: "  cork-cli warranties exposure --limit 20 --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "warranties exposure")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Active warranties, keyed by client.
			rawW, _, warrantyCapHit, err := corkFetchPages(ctx, c, "/warranties", nil, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			active := map[string]corkWarranty{}
			badWarranties := 0
			for _, r := range rawW {
				var w corkWarranty
				if json.Unmarshal(r, &w) != nil {
					badWarranties++
					continue
				}
				if w.ClientUUID == "" {
					continue
				}
				// Keep an active warranty over an inactive one for the same client.
				if prev, ok := active[w.ClientUUID]; ok && prev.Active && !w.Active {
					continue
				}
				active[w.ClientUUID] = w
			}

			// Client roster with embedded score history, preferring the local
			// mirror and falling back to a live fetch.
			clients := make([]corkClient, 0)
			if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients,warranties"); openErr == nil && ok {
				defer db.Close()
				if !hintIfUnsynced(cmd, db, "clients") {
					hintIfStale(cmd, db, "clients", flags.maxAge)
				}
				if cl, loadErr := corkLoadClients(ctx, db); loadErr == nil {
					clients = cl
				}
			}
			clientCapHit := false
			if len(clients) == 0 {
				rawC, _, cCap, cErr := corkFetchPages(ctx, c, "/clients", nil, flagMaxScanPages)
				if cErr != nil {
					return classifyAPIError(cErr, flags)
				}
				clientCapHit = cCap
				roster, undecodable := corkDecodeClients(rawC)
				if len(roster) == 0 && undecodable > 0 {
					return corkDecodeFailure("client record(s)", undecodable)
				}
				clients = roster
			}
			if badWarranties > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d warranty record(s) could not be decoded; some clients may read as uncovered\n", badWarranties)
			}

			rows := make([]warrantyExposureRow, 0, len(clients))
			scanned, uncovered := 0, 0
			for _, cl := range clients {
				if cl.Hidden {
					continue
				}
				scanned++
				w, hasW := active[cl.UUID]
				status := strings.TrimSpace(cl.WarrantyStatus)
				// The client's own warranty_status is the platform's view and must
				// be able to BOTH demote and promote. Letting it only demote meant
				// a client whose warranty record fell past the /warranties page cap
				// was reported "no active warranty" while its own status said
				// active — a self-contradictory row that inflated the uncovered
				// count.
				var covered bool
				switch {
				case status != "" && !strings.EqualFold(status, "active"):
					covered = false
				case hasW:
					covered = w.Active
				case strings.EqualFold(status, "active"):
					covered = true
				default:
					covered = false
				}
				if covered && !flagIncludeCovered {
					continue
				}
				if !covered {
					uncovered++
				}

				score, trend := 0, 0
				var newest, oldest *corkScorePoint
				var newestT, oldestT time.Time
				for i := range cl.ScoreHistory {
					t, ok := corkParseTime(cl.ScoreHistory[i].CreatedAt)
					if !ok {
						continue
					}
					if newest == nil || t.After(newestT) {
						newest = &cl.ScoreHistory[i]
						newestT = t
					}
					if oldest == nil || t.Before(oldestT) {
						oldest = &cl.ScoreHistory[i]
						oldestT = t
					}
				}
				if newest != nil {
					score = newest.Score
					if oldest != nil {
						trend = newest.Score - oldest.Score
					}
				}

				reason := "no active warranty"
				switch {
				case covered:
					reason = "covered"
				case status != "" && !strings.EqualFold(status, "active"):
					reason = "warranty status: " + status
				case hasW && !w.Active:
					reason = "warranty lapsed"
				}

				pkg := ""
				if hasW {
					pkg = w.Package
				}
				rows = append(rows, warrantyExposureRow{
					ClientUUID:     cl.UUID,
					Client:         cl.Name,
					WarrantyStatus: status,
					Covered:        covered,
					Package:        pkg,
					Score:          score,
					Trend:          trend,
					Reason:         reason,
				})
			}

			// Worst risk first: lowest score, then the steepest decline.
			corkSortStable(rows, func(a, b warrantyExposureRow) bool {
				if a.Covered != b.Covered {
					return !a.Covered
				}
				if a.Score != b.Score {
					return a.Score < b.Score
				}
				return a.Trend < b.Trend
			})
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			capHit := warrantyCapHit || clientCapHit || badWarranties > 0
			view := warrantyExposureView{
				Items:          rows,
				ClientsScanned: scanned,
				Uncovered:      uncovered,
				WarrantiesSeen: len(active),
				ScanCapHit:     capHit,
			}
			switch {
			case scanned == 0:
				view.Note = "no clients available; run: cork-cli sync --resources clients"
			case len(rows) == 0:
				view.Note = fmt.Sprintf("all %d client(s) carry an active warranty", scanned)
			case capHit:
				// A truncated /warranties read makes covered clients look
				// uncovered, so the caveat has to travel with the result.
				view.Note = "the warranty or client scan was truncated; some clients may be listed uncovered only because their warranty record was not read (raise --max-scan-pages)"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CLIENT\tSCORE\tTREND\tCOVERED\tREASON")
			for _, r := range rows {
				cov := "no"
				if r.Covered {
					cov = "yes"
				}
				fmt.Fprintf(tw, "%s\t%d\t%+d\t%s\t%s\n", truncate(r.Client, 34), r.Score, r.Trend, cov, truncate(r.Reason, 30))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d of %d client(s) without active coverage\n", view.Uncovered, view.ClientsScanned)
			return nil
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "Maximum clients to return (0 for all)")
	cmd.Flags().BoolVar(&flagIncludeCovered, "include-covered", false, "Include clients that already have active coverage")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum warranty and client pages to read")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used for the client roster")
	return cmd
}
