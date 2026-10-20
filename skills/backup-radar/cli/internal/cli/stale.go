// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/radar"
)

type staleClient struct {
	Client    string `json:"client"`
	ClientID  string `json:"client_id,omitempty"`
	BRCompany string `json:"br_company,omitempty"`
	Count     int    `json:"count"`
}

// groupStaleByClient counts rows per client. Two ScalePad clients can share a
// display name, so groups key on the client id too; rows without one
// (unmapped, v2-only) group by Backup Radar company and name.
func groupStaleByClient(rows []radar.StaleRow) []staleClient {
	out := make([]staleClient, 0)
	index := map[[2]string]int{}
	for _, r := range rows {
		k := [2]string{r.ClientID, r.Client}
		brCompany := ""
		if r.ClientID == "" { // HAND-FIX(radar-same-named-clients-never-merge): no ScalePad id, so the Backup Radar company tells them apart
			brCompany = r.BRCompany
			k = [2]string{"br:" + brCompany, r.Client}
		}
		if i, ok := index[k]; ok {
			out[i].Count++
			continue
		}
		index[k] = len(out)
		out = append(out, staleClient{Client: r.Client, ClientID: r.ClientID, BRCompany: brCompany, Count: 1})
	}
	return out
}

type staleView struct {
	Days     int              `json:"days"`
	Rows     []radar.StaleRow `json:"rows"`
	ByClient []staleClient    `json:"by_client"`
	Sources  radarSources     `json:"sources"`
	Next     string           `json:"next,omitempty"`
}

func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var days, limit int
	var client, dbPath string
	var bodies, closePSA bool

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List backups with no result in --days days (or never), grouped by client, with dry-run retire request bodies",
		Long: `List backups that have received no result in at least --days days (or never),
grouped by client with last success and open ticket count. Each row carries a
retire_request body with dry_run=true; --bodies prints just those bodies as
JSON lines so one can be piped into 'backups retire --stdin' to preview it.
A mapped device is pinned by its ScalePad device id; an unmapped one by its
Backup Radar company, device and job names. A backup with no client and no
device id, or one seen only in v2 data, gets no body (retire_note explains):
name-only filters could match the wrong backup.

Use this command to list silent backups that are candidates for retirement.
Do NOT use this command to retire anything; use 'backups retire' with dry_run instead.
Do NOT use it for failing backups that are still in service; use 'check' instead.`,
		Example: `  backup-radar-cli stale --days 30 --agent
  backup-radar-cli stale --days 45 --client "Acme Dental"
  backup-radar-cli stale --days 60 --bodies | head -n 1 | backup-radar-cli backups retire --stdin
  backup-radar-cli stale --days 60 --bodies | Select-Object -First 1 | backup-radar-cli backups retire --stdin   # PowerShell`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "csv,plain,quiet"}, // HAND-FIX(radar-machine-modes): refused modes leave the MCP schema
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "stale"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "stale")
			}
			if days < 1 || days > 3650 {
				return usageErr(fmt.Errorf("--days must be between 1 and 3650 (the retire endpoint's no_result_in limit)"))
			}
			snap, ok, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			view := staleView{Days: days, Rows: make([]radar.StaleRow, 0), ByClient: make([]staleClient, 0), Sources: sourcesOf(snap, "devices", "backups")}
			if ok {
				view.Rows = radar.Stale(snap, days, client, closePSA, time.Now().UTC())
			}
			view.ByClient = groupStaleByClient(view.Rows)
			if limit > 0 && len(view.Rows) > limit {
				view.Rows = view.Rows[:limit]
			}
			for _, r := range view.Rows {
				if r.RetireRequest != nil { // only suggest the pipe when a body exists
					scope := ""
					if client != "" {
						scope += " --client " + shellQuote(client)
					}
					if dbPath != "" {
						scope += " --db " + shellQuote(dbPath)
					}
					view.Next = fmt.Sprintf("preview one retire (dry_run is true in every body): backup-radar-cli stale --days %d%s --bodies | %s | backup-radar-cli backups retire --stdin", days, scope, firstLineCmd())
					break
				}
			}
			if bodies {
				// HAND-FIX(stale-bodies-ascii-dedup): each body once, ASCII-only
				// JSON so a PowerShell pipe's code page cannot mangle a name.
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				enc.SetEscapeHTML(false)
				unscoped, shared := 0, false
				seen := map[radar.RetireRequest]bool{}
				for _, r := range view.Rows {
					if r.RetireRequest == nil {
						unscoped++
						continue
					}
					buf.Reset()
					if err := enc.Encode(r.RetireRequest); err != nil {
						return err
					}
					shared = shared || r.RetireNote != "" // the note counts twins --limit cut off too
					k := radar.RetireBodyKey(*r.RetireRequest)
					if seen[k] {
						continue
					}
					seen[k] = true
					if _, err := cmd.OutOrStdout().Write(asciiJSON(buf.Bytes())); err != nil {
						return err
					}
				}
				if view.Sources.Note != "" { // HAND-FIX(radar-human-notes): warning: so the MCP shell-out forwards it
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+scrubRadarText(view.Sources.Note))
				}
				if shared {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: some retire bodies match more than one stale backup (same company, device and job names); each body is printed once, and retire acts on every backup it matches")
				}
				if unscoped > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped %d backup(s) with no client or ScalePad device id; see retire_note in the JSON output\n", unscoped)
				}
				return nil
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, radarKeep(radar.StaleRow{})...)
			}
			if len(view.Rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No backups silent for %d+ days in the local store.\n", days)
				printSourcesNote(cmd.OutOrStdout(), view.Sources.Note)
				return nil
			}
			table := make([][]string, 0, len(view.Rows))
			for _, r := range view.Rows {
				silent := "never"
				if r.DaysSilent != nil {
					silent = strconv.Itoa(*r.DaysSilent)
				}
				table = append(table, []string{r.Client, r.Device, r.Job, silent, r.LastResult, r.Source})
			}
			if err := printRadarTable(cmd.OutOrStdout(), []string{"client", "device", "job", "days_silent", "last_result", "source"}, table); err != nil {
				return err
			}
			printSourcesNote(cmd.ErrOrStderr(), view.Sources.Note)
			for _, r := range view.Rows {
				if r.RetireRequest != nil && r.RetireNote != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: some retire bodies match more than one stale backup (same company, device and job names); see retire_note in --json")
					break
				}
			}
			if view.Next != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), scrubRadarText(view.Next))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Minimum days without any backup result (1-3650)")
	cmd.Flags().StringVar(&client, "client", "", "Only clients whose name contains this text (case-insensitive)")
	cmd.Flags().BoolVar(&bodies, "bodies", false, "Print only the retire request bodies, one JSON object per line")
	cmd.Flags().BoolVar(&closePSA, "close-psa-tickets", false, "Set close_psa_tickets=true in the generated retire bodies")
	cmd.Flags().IntVar(&limit, "limit", 500, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}

// shellQuote single-quotes s for POSIX shells so a copied hint can never
// expand $(...), backticks or variables from a client name or path.
// shellQuote and firstLineCmd shape the stale "next" hint for the shell the
// user is in: PowerShell on Windows (most MSPs), POSIX elsewhere.
// HAND-FIX(stale-hint-windows-twin)
func shellQuote(s string) string {
	if runtime.GOOS == "windows" {
		return psQuote(s)
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// psQuote single-quotes s for PowerShell, which also closes a single-quoted
// string on U+2018-U+201B; each is doubled like the ASCII quote.
func psQuote(s string) string {
	return "'" + strings.NewReplacer("'", "''", "\u2018", "\u2018\u2018", "\u2019", "\u2019\u2019", "\u201a", "\u201a\u201a", "\u201b", "\u201b\u201b").Replace(s) + "'"
}

// asciiJSON escapes every non-ASCII rune in encoder output as \uXXXX (a
// surrogate pair above U+FFFF); JSON decoding restores the original text.
func asciiJSON(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] < utf8.RuneSelf {
			out = append(out, b[i])
			i++
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if r > 0xffff {
			r1, r2 := utf16.EncodeRune(r)
			out = fmt.Appendf(out, `\u%04x\u%04x`, r1, r2)
		} else {
			out = fmt.Appendf(out, `\u%04x`, r)
		}
		i += size
	}
	return out
}

func firstLineCmd() string {
	if runtime.GOOS == "windows" {
		return "Select-Object -First 1"
	}
	return "head -n 1"
}

// printSourcesNote prints a report's not-synced note on a terminal, as check
// does, so an empty table never reads as "checked". HAND-FIX(radar-human-notes)
func printSourcesNote(w io.Writer, note string) {
	if note != "" {
		fmt.Fprintln(w, "note: "+scrubRadarText(note))
	}
}

// refuseTabularModes keeps the analysis reports to JSON or the human table:
// each has several lists plus a not-synced note, which --csv, --plain and
// --quiet cannot carry, so those modes printed a shape that changed with the
// data and dropped the note. HAND-FIX(radar-machine-modes)
func refuseTabularModes(flags *rootFlags, name string) error {
	if flags.csv || flags.plain || flags.quiet {
		return usageErr(fmt.Errorf("%s does not support --csv, --plain or --quiet (its report has several lists and a not-synced note); use --json or --agent, or no flag for the table", name))
	}
	return nil
}
