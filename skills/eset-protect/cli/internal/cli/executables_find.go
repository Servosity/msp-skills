// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
//
// Hand-authored. `executables find`: turn a file hash or name into the
// executable UUID that 'executables block' needs. Absorbs Fenrindale
// search_executables. ESET Connect has no hash or name filter on
// GET /v1/executables, so this searches the local mirror when it holds
// executables, else scans live pages up to --max-scan-pages.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"eset-protect-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if executables, _, err := root.Find([]string{"executables"}); err == nil && executables != root {
			addNovelCommandIfAbsent(executables, newNovelExecutablesFindCmd(flags))
		}
	})
}

const (
	executablesFindSourceLocal = "local"
	executablesFindSourceLive  = "live"
)

// executablesFindFloat decodes a protobuf-JSON double that may arrive as a
// number or a quoted string. Present is false when the field is absent.
type executablesFindFloat struct {
	Value   float64
	Present bool
}

func (f *executablesFindFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // tolerate odd values rather than dropping the record
	}
	f.Value, f.Present = v, true
	return nil
}

type executablesFindExecutable struct {
	UUID                   string  `json:"uuid"`
	DisplayName            string  `json:"displayName"`
	ApplicationDisplayName string  `json:"applicationDisplayName"`
	ApplicationVersionName string  `json:"applicationVersionName"`
	VersionName            string  `json:"versionName"`
	HashSha1               string  `json:"hashSha1"`
	HashMd5                string  `json:"hashMd5"`
	HashSha2256            string  `json:"hashSha2256"`
	IsBlocked              bool    `json:"isBlocked"`
	DevicesCount           flexInt `json:"devicesCount"`
	FirstOccurTime         string  `json:"firstOccurTime"`
	Signatures             []struct {
		SignerName string `json:"signerName"`
	} `json:"signatures"`
	LiveGridFindings struct {
		Reputation executablesFindFloat `json:"reputation"`
	} `json:"liveGridFindings"`
}

type executablesFindRow struct {
	UUID         string   `json:"uuid"`
	DisplayName  string   `json:"display_name"`
	Application  string   `json:"application"`
	Version      string   `json:"version"`
	SHA1         string   `json:"sha1"`
	MD5          string   `json:"md5"`
	SHA256       string   `json:"sha256"`
	IsBlocked    bool     `json:"is_blocked"`
	DevicesCount int64    `json:"devices_count"`
	Signer       string   `json:"signer"`
	Reputation   *float64 `json:"reputation"`
	FirstSeen    string   `json:"first_seen"`
}

type executablesFindView struct {
	Items              []executablesFindRow `json:"items"`
	Matched            int                  `json:"matched"`
	Source             string               `json:"source"`
	ScannedExecutables int                  `json:"scanned_executables"`
	Truncated          bool                 `json:"truncated"`
	Note               string               `json:"note"`
	RejectedRows       int                  `json:"rejected_rows"`
	// SyncedHint is set when the answer came from a local mirror whose
	// executables sync is missing or did not complete.
	SyncedHint string `json:"synced_hint,omitempty"`
}

// executablesFindQuery is either a hash lookup on one field or a name search.
type executablesFindQuery struct {
	HashField string // hashMd5 | hashSha1 | hashSha2256
	Hash      string // lowercase hex
	Name      string // lowercase substring
}

// executablesFindClassifyHash picks the executable hash field from the hash
// length: 32 md5, 40 sha1, 64 sha256. Non-hex input is refused.
func executablesFindClassifyHash(h string) (field, norm string, err error) {
	norm = strings.ToLower(strings.TrimSpace(h))
	for _, r := range norm {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", "", fmt.Errorf("--hash %q is not hexadecimal", h)
		}
	}
	switch len(norm) {
	case 32:
		return "hashMd5", norm, nil
	case 40:
		return "hashSha1", norm, nil
	case 64:
		return "hashSha2256", norm, nil
	}
	return "", "", fmt.Errorf("--hash has %d hex characters; expected 32 (MD5), 40 (SHA-1) or 64 (SHA-256)", len(norm))
}

func (q executablesFindQuery) matches(e executablesFindExecutable) bool {
	if q.HashField != "" {
		var v string
		switch q.HashField {
		case "hashMd5":
			v = e.HashMd5
		case "hashSha1":
			v = e.HashSha1
		case "hashSha2256":
			v = e.HashSha2256
		}
		return v != "" && strings.EqualFold(strings.TrimSpace(v), q.Hash)
	}
	if q.Name == "" {
		return false
	}
	return strings.Contains(strings.ToLower(e.DisplayName), q.Name) ||
		strings.Contains(strings.ToLower(e.ApplicationDisplayName), q.Name)
}

func executablesFindToRow(e executablesFindExecutable) executablesFindRow {
	row := executablesFindRow{
		UUID:         e.UUID,
		DisplayName:  e.DisplayName,
		Application:  e.ApplicationDisplayName,
		Version:      e.VersionName,
		SHA1:         strings.ToLower(e.HashSha1),
		MD5:          strings.ToLower(e.HashMd5),
		SHA256:       strings.ToLower(e.HashSha2256),
		IsBlocked:    e.IsBlocked,
		DevicesCount: int64(e.DevicesCount),
		FirstSeen:    e.FirstOccurTime,
	}
	if row.Version == "" {
		row.Version = e.ApplicationVersionName
	}
	for _, s := range e.Signatures {
		if s.SignerName != "" {
			row.Signer = s.SignerName
			break
		}
	}
	if e.LiveGridFindings.Reputation.Present {
		v := e.LiveGridFindings.Reputation.Value
		row.Reputation = &v
	}
	return row
}

// executablesFindSearch returns every matching executable, most widespread
// first. A row that does not decode (devicesCount is a strict integer) or has
// no uuid could hide the very executable searched for, so it is counted in
// rej rather than skipped silently.
func executablesFindSearch(raw []json.RawMessage, q executablesFindQuery, rej *esetRejects) []executablesFindRow {
	if rej == nil {
		rej = &esetRejects{}
	}
	out := make([]executablesFindRow, 0)
	for i, r := range raw {
		if !esetIsObject(r) {
			rej.add(fmt.Sprintf("executable row %d", i), errors.New("not a JSON object"))
			continue
		}
		var e executablesFindExecutable
		if err := json.Unmarshal(r, &e); err != nil {
			rej.add(fmt.Sprintf("executable row %d", i), err)
			continue
		}
		if e.UUID == "" {
			rej.add(fmt.Sprintf("executable row %d", i), errors.New("no uuid"))
			continue
		}
		if q.matches(e) {
			out = append(out, executablesFindToRow(e))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DevicesCount != out[j].DevicesCount {
			return out[i].DevicesCount > out[j].DevicesCount
		}
		if !strings.EqualFold(out[i].DisplayName, out[j].DisplayName) {
			return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName)
		}
		return out[i].UUID < out[j].UUID
	})
	return out
}

// executablesFindFinish applies --limit and writes the explanatory note,
// which also names any rows that could not be searched (view.RejectedRows).
func executablesFindFinish(view *executablesFindView, matches []executablesFindRow, limit, pages int) {
	view.Matched = len(matches)
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	view.Items = append(view.Items, matches...)
	switch {
	case view.Matched == 0 && view.Truncated:
		view.Note = fmt.Sprintf("no match in the first %d executables (%d page(s), the --max-scan-pages cap); raise --max-scan-pages, or run '%s sync --resources executables' and search the local mirror", view.ScannedExecutables, pages, esetCLIName)
	case view.Matched == 0 && view.Source == executablesFindSourceLocal:
		view.Note = fmt.Sprintf("no match among %d executables in the local mirror; it may be stale, rerun with --live to scan ESET directly", view.ScannedExecutables)
	case view.Matched > len(view.Items):
		view.Note = fmt.Sprintf("showing %d of %d matches; raise --limit or use --hash for an exact match", len(view.Items), view.Matched)
	}
	if view.RejectedRows > 0 {
		unread := fmt.Sprintf("%d executable row(s) could not be decoded and were not searched", view.RejectedRows)
		if view.Note == "" {
			view.Note = unread
		} else {
			view.Note += "; " + unread
		}
	}
}

func newNovelExecutablesFindCmd(flags *rootFlags) *cobra.Command {
	var hash, name, dbPath string
	var live bool
	var maxScanPages, limit int
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find an executable's UUID, blocked state and signer by file hash or name",
		Long: "Use this command to turn a file hash or name into the executable UUID that 'executables block'\n" +
			"needs, with its blocked state and signer. Do NOT use it to see how often a hash was detected;\n" +
			"use 'detections spread' instead.\n\n" +
			"Pass exactly one of --hash (MD5, SHA-1 or SHA-256, picked by length, case-insensitive) or\n" +
			"--name (case-insensitive substring of the file or application name). ESET Connect cannot\n" +
			"filter executables server-side, so the command searches the local mirror when it holds\n" +
			"executables (source local) and otherwise scans live pages of 1000 up to --max-scan-pages\n" +
			"(source live). --live, or --data-source live, always scans ESET directly. An executable\n" +
			"row that cannot be decoded is not searched; it is counted in rejected_rows.",
		Example: strings.Trim(`
  eset-protect-cli executables find --hash 3395856ce81f2b7382dee72602f798b642f14140
  eset-protect-cli executables find --name anydesk --agent
  eset-protect-cli executables find --hash E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855 --live --max-scan-pages 20 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:data-source":      "auto",
			"pp:happy-args":       "--name=chrome",
			"pp:typed-exit-codes": "0,2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "executables find")
			}
			hash, name = strings.TrimSpace(hash), strings.TrimSpace(name)
			if (hash == "") == (name == "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one of --hash or --name"))
			}
			var q executablesFindQuery
			if hash != "" {
				field, norm, err := executablesFindClassifyHash(hash)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(err)
				}
				q.HashField, q.Hash = field, norm
			} else {
				q.Name = strings.ToLower(name)
			}
			if limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1"))
			}
			if maxScanPages < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxScanPages = 1
			}
			forceLive := live || flags.dataSource == "live"
			localOnly := flags.dataSource == "local"
			if forceLive && localOnly {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--live conflicts with --data-source local"))
			}

			view := executablesFindView{Items: make([]executablesFindRow, 0)}
			w := cmd.OutOrStdout()

			if !forceLive {
				mirror := dbPath
				if mirror == "" {
					mirror = defaultDBPath(esetCLIName)
				}
				if _, statErr := os.Stat(mirror); statErr == nil || localOnly {
					db, err := openEsetStore(cmd, mirror, "executables")
					if err != nil {
						return err
					}
					var raw []json.RawMessage
					if db != nil {
						defer db.Close()
						if raw, err = loadEsetRaw(db, "executables"); err != nil {
							return fmt.Errorf("reading executables: %w", err)
						}
					}
					// A mirror with executables answers locally; an empty one
					// falls through to the live scan unless --data-source local.
					if len(raw) > 0 || localOnly {
						if db != nil {
							esetSyncHints(cmd, db, flags.maxAge, "executables")
						}
						view.Source = executablesFindSourceLocal
						view.SyncedHint = esetMirrorHint(db, "executables")
						view.ScannedExecutables = len(raw)
						var rej esetRejects
						matches := executablesFindSearch(raw, q, &rej)
						view.RejectedRows = rej.N
						rej.warn(cmd)
						executablesFindFinish(&view, matches, limit, 0)
						switch {
						case db == nil:
							view.Note = fmt.Sprintf("no local mirror; run '%s sync --resources executables' or rerun with --live", esetCLIName)
						case len(raw) == 0:
							view.Note = fmt.Sprintf("the local mirror holds no executables; run '%s sync --resources executables' or rerun with --live", esetCLIName)
						}
						return executablesFindPrint(w, flags, view)
					}
				}
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			res, err := esetFetchAll(ctx, c, esetHost("application-management", "/v1/executables"), nil, "executables", maxScanPages)
			if err != nil {
				return classifyAPIError(w, err, flags)
			}
			view.Source = executablesFindSourceLive
			view.ScannedExecutables = len(res.Items)
			view.Truncated = res.Truncated
			var rej esetRejects
			matches := executablesFindSearch(res.Items, q, &rej)
			view.RejectedRows = rej.N
			rej.warn(cmd)
			executablesFindFinish(&view, matches, limit, res.Pages)
			return executablesFindPrint(w, flags, view)
		},
	}
	cmd.Flags().StringVar(&hash, "hash", "", "File hash: 32 hex chars MD5, 40 SHA-1 or 64 SHA-256 (case-insensitive)")
	cmd.Flags().StringVar(&name, "name", "", "Case-insensitive substring of the executable or application name")
	cmd.Flags().BoolVar(&live, "live", false, "Scan ESET directly even when the local mirror holds executables")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum live pages of 1000 executables to scan")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum matches to return")
	cmd.Flags().StringVar(&dbPath, "db", "", esetDBFlagUsage)
	return cmd
}

func executablesFindPrint(w io.Writer, flags *rootFlags, view executablesFindView) error {
	// The --agent envelope's meta.source must name where the rows came from
	// (the mirror or a live scan), not the command's "auto" default
	// (handfixes.json: agent-meta-source-matches-results).
	if flags != nil && !flags.dryRun && view.Source != "" {
		flags.agentSource = view.Source
	}
	if !wantsHumanTable(w, flags) {
		return printJSONFiltered(w, view, flags)
	}
	if len(view.Items) == 0 {
		esetFprintf(w, "No executable matches (%s, %d scanned).\n", view.Source, view.ScannedExecutables)
		if view.Note != "" {
			esetFprintln(w, view.Note)
		}
		if view.SyncedHint != "" {
			esetFprintln(w, view.SyncedHint)
		}
		return nil
	}
	tw := newTabWriter(w)
	fmt.Fprintln(tw, "UUID\tNAME\tAPPLICATION\tBLOCKED\tDEVICES\tSIGNER\tSHA1")
	for _, r := range view.Items {
		esetFprintf(tw, "%s\t%s\t%s\t%t\t%d\t%s\t%s\n", r.UUID, triageClip(r.DisplayName, 36), triageClip(r.Application, 30), r.IsBlocked, r.DevicesCount, triageClip(r.Signer, 30), r.SHA1)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	esetFprintf(w, "\n%d of %d matches (%s, %d executables scanned)\n", len(view.Items), view.Matched, view.Source, view.ScannedExecutables)
	if view.Note != "" {
		esetFprintln(w, view.Note)
	}
	if view.SyncedHint != "" {
		esetFprintln(w, view.SyncedHint)
	}
	return nil
}
