// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"riverside-fm-pp-cli/internal/credkit"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newBulkCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bulk",
		Short: "Bulk operations across a studio: export every take's transcript + asset metadata + manifest with a resume cursor.",
	}
	cmd.AddCommand(newBulkExportCmd(flags))
	return cmd
}

func newBulkExportCmd(flags *rootFlags) *cobra.Command {
	var studio string
	var since string
	var outDir string
	var resume bool

	cmd := &cobra.Command{
		Use:         "export",
		Short:       "Export every project / take / asset for a studio with resume.",
		Long:        "Walks projects-by-studio -> takes-by-project -> transcripts + take/assets endpoints. Writes each take's transcript + assets-manifest + per-participant HLS manifest URL to <out>/<studio>/<project>/<session>/. Resume state lives in <out>/.resume.json so interruption never restarts the walk. A take is checkpointed only once every fetch and write for it succeeded and Riverside has finished it; any failure is named on stderr, exits non-zero, and is retried on the next run.",
		Example:     `  riverside-fm-cli bulk export --studio damien-stevenss-studio --since 2026-04-01 --out ./archive`,
		Annotations: map[string]string{"mcp:hidden": "true"}, // writes media files; never an MCP tool
		RunE: func(cmd *cobra.Command, args []string) error {
			if studio == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = "./riverside-export"
			}
			if err := os.MkdirAll(outDir, 0o700); err != nil {
				return err
			}

			var sinceTime *time.Time
			if since != "" {
				t, perr := parseSince(since)
				if perr != nil {
					return usageErr(fmt.Errorf("--since %q: %w", since, perr))
				}
				sinceTime = &t
			}

			resumePath := filepath.Join(outDir, ".resume.json")
			done := map[string]bool{}
			if resume {
				// #nosec G304 -- resumePath is .resume.json inside the operator-chosen
				// --out directory; a local path the operator named themselves.
				if b, err := os.ReadFile(filepath.Clean(resumePath)); err == nil {
					_ = json.Unmarshal(b, &done)
				}
			}

			summary := bulkSummary{}

			// 1. List projects in studio
			projects, err := listStudioProjects(cmd.Context(), c, studio)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Found %d projects in studio %s\n", len(projects), studio)

			// A take is checkpointed in .resume.json only once every fetch and every
			// write for it succeeded and Riverside has finished it (transcript has
			// content, every track done). A failed fetch or write is counted, named
			// on stderr and leaves the take un-checkpointed, so the next run retries
			// it instead of skipping it forever; the command then exits non-zero.
			var firstErr error
			firstIsFetch := false
			failed := func(e error, fetch bool) {
				summary.Errors++
				fmt.Fprintf(cmd.ErrOrStderr(), "WARN: %v\n", e)
				if firstErr == nil {
					firstErr, firstIsFetch = e, fetch
				}
			}

			for _, proj := range projects {
				if sinceTime != nil && proj.CreatedAt.Before(*sinceTime) {
					summary.SkippedOlderThanSince++
					continue
				}
				projDir := safeJoin(outDir, studio, proj.Title+"-"+proj.ID)
				if projDir == "" {
					failed(fmt.Errorf("project %s: its title/id would escape --out; not exported", proj.ID), false)
					continue
				}
				if err := os.MkdirAll(projDir, 0o700); err != nil {
					return err
				}

				// 2. List takes per project
				takes, err := listProjectTakes(cmd.Context(), c, proj.ID)
				if err != nil {
					failed(fmt.Errorf("list takes for %s: %w", proj.ID, err), true)
					continue
				}

				for _, take := range takes {
					if done[take.SessionID] {
						summary.AlreadyDone++
						continue
					}
					takeDir := safeJoin(projDir, take.Title+"-"+take.SessionID)
					if takeDir == "" {
						failed(fmt.Errorf("take %s: its title/id would escape --out; not exported", take.SessionID), false)
						continue
					}
					if err := os.MkdirAll(takeDir, 0o700); err != nil {
						return err
					}
					summary.TakesProcessed++
					takeFailed := false
					takeFinished := true
					fetchFailed := func(what string, e error) {
						takeFailed = true
						failed(fmt.Errorf("take %s %s: %w", take.SessionID, what, e), true)
					}
					write := func(name string, body []byte) bool {
						if werr := writeUnderDir(takeDir, name, body); werr != nil {
							takeFailed = true
							failed(fmt.Errorf("take %s: writing %s: %w", take.SessionID, name, werr), false)
							return false
						}
						return true
					}

					// 3a. Transcript
					tt := "/api/v4/transcriptions/editableWithVoiceActivity/" + url.PathEscape(take.SessionID)
					tdata, terr := c.Get(cmd.Context(), tt, nil)
					switch {
					case terr != nil:
						fetchFailed("transcript", terr)
					case hasTranscriptContent(tdata):
						okJSON := write("transcript.json", responseFileBytes(tdata))
						okTxt := write("transcript.txt", []byte(transcriptToPlainText(tdata)))
						if okJSON && okTxt {
							summary.TranscriptsWritten++
						}
					default:
						takeFinished = false // Riverside has not transcribed it yet
					}

					// 3b. Asset metadata
					ap := "/api/v4/take/" + url.PathEscape(take.SessionID) + "/assets"
					adata, aerr := c.Get(cmd.Context(), ap, nil)
					if aerr != nil {
						fetchFailed("assets", aerr)
					} else {
						if write("assets.json", responseFileBytes(adata)) {
							summary.AssetManifestsWritten++
						}
						if !allTracksDone(adata) {
							takeFinished = false // a track is still uploading or processing
						}

						// 3c. VOD manifest per participant
						handles := extractParticipantHandles(adata)
						for _, h := range handles {
							vp := "/api/v4/vod/" + url.PathEscape(take.SessionID) + "/" + url.PathEscape(h)
							vdata, verr := c.Get(cmd.Context(), vp, nil)
							if verr != nil {
								fetchFailed("manifest "+h, verr)
								continue
							}
							if manifest := responseFileBytes(vdata); len(manifest) > 0 && write(h+".m3u8", manifest) {
								summary.HlsManifestsWritten++
							}
						}
					}

					if takeFailed {
						continue
					}
					if !takeFinished {
						summary.Pending++
						continue
					}
					// A take counts as checkpointed only once the cursor is on disk.
					done[take.SessionID] = true
					rb, merr := json.Marshal(done)
					if merr == nil {
						merr = credkit.WriteFile(resumePath, rb, 0o600)
					}
					if merr != nil {
						delete(done, take.SessionID)
						failed(fmt.Errorf("take %s: writing the resume cursor: %w", take.SessionID, merr), false)
						continue
					}
					summary.TakesCheckpointed++
				}
			}

			if flags.asJSON {
				sb, _ := json.MarshalIndent(summary, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(sb))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "\nDone. takes=%d checkpointed=%d pending=%d transcripts=%d assets=%d hls=%d errors=%d skipped_resume=%d skipped_since=%d\n",
					summary.TakesProcessed, summary.TakesCheckpointed, summary.Pending, summary.TranscriptsWritten, summary.AssetManifestsWritten, summary.HlsManifestsWritten,
					summary.Errors, summary.AlreadyDone, summary.SkippedOlderThanSince)
			}
			if firstErr != nil {
				incomplete := fmt.Errorf("bulk export incomplete: %d error(s); the affected takes were not checkpointed, so re-running retries them. First: %w", summary.Errors, firstErr)
				if firstIsFetch {
					return classifyAPIErrorOnly(incomplete)
				}
				return incomplete
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&studio, "studio", "", "Studio slug (required)")
	cmd.Flags().StringVar(&since, "since", "", "Only process projects created after this date (YYYY-MM-DD or duration like 30d / 7d)")
	cmd.Flags().StringVar(&outDir, "out", "./riverside-export", "Output directory root")
	cmd.Flags().BoolVar(&resume, "resume", true, "Skip takes already exported in this directory")
	return cmd
}

type bulkSummary struct {
	TakesProcessed        int `json:"takes_processed"`
	TakesCheckpointed     int `json:"takes_checkpointed"`
	Pending               int `json:"pending"`
	TranscriptsWritten    int `json:"transcripts_written"`
	AssetManifestsWritten int `json:"asset_manifests_written"`
	HlsManifestsWritten   int `json:"hls_manifests_written"`
	Errors                int `json:"errors"`
	AlreadyDone           int `json:"already_done"`
	SkippedOlderThanSince int `json:"skipped_older_than_since"`
}

type projectSummary struct {
	ID        string
	Title     string
	CreatedAt time.Time
}

type takeSummary struct {
	ID        string
	SessionID string
	Title     string
}

func extractProjects(data json.RawMessage) []projectSummary {
	// /api/v4/projects/studio/{slug} returns either {projects:[...]} or an array directly.
	var wrapped struct {
		Projects []struct {
			ID        string `json:"_id"`
			Title     string `json:"title"`
			CreatedAt string `json:"createdAt"`
		} `json:"projects"`
	}
	if json.Unmarshal(data, &wrapped) == nil && len(wrapped.Projects) > 0 {
		out := make([]projectSummary, 0, len(wrapped.Projects))
		for _, p := range wrapped.Projects {
			t, _ := time.Parse(time.RFC3339, p.CreatedAt)
			out = append(out, projectSummary{ID: p.ID, Title: p.Title, CreatedAt: t})
		}
		return out
	}
	var arr []struct {
		ID        string `json:"_id"`
		Title     string `json:"title"`
		CreatedAt string `json:"createdAt"`
	}
	if json.Unmarshal(data, &arr) == nil {
		out := make([]projectSummary, 0, len(arr))
		for _, p := range arr {
			t, _ := time.Parse(time.RFC3339, p.CreatedAt)
			out = append(out, projectSummary{ID: p.ID, Title: p.Title, CreatedAt: t})
		}
		return out
	}
	return nil
}

func extractTakes(data json.RawMessage) []takeSummary {
	var wrapped struct {
		Takes []struct {
			ID         string `json:"_id"`
			Title      string `json:"title"`
			Recordings []struct {
				SessionID string `json:"sessionId"`
			} `json:"recordings"`
		} `json:"takes"`
	}
	if json.Unmarshal(data, &wrapped) != nil {
		return nil
	}
	out := make([]takeSummary, 0, len(wrapped.Takes))
	for _, t := range wrapped.Takes {
		sid := ""
		if len(t.Recordings) > 0 {
			sid = t.Recordings[0].SessionID
		}
		if sid == "" {
			continue
		}
		out = append(out, takeSummary{ID: t.ID, SessionID: sid, Title: t.Title})
	}
	return out
}

func sanitize(s string) string {
	if s == "" {
		return "untitled"
	}
	bad := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, b := range bad {
		s = strings.ReplaceAll(s, b, "-")
	}
	// Control bytes (NUL especially) have no business in a filename and can
	// truncate a path at the syscall boundary on some platforms.
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '-'
		}
		return r
	}, s)
	if len(s) > 80 {
		// Keep the tail as well as the head: callers end a name with its id and
		// extension, and two long titles that differ only there must stay two files.
		s = strings.ToValidUTF8(s[:39], "") + "-" + strings.ToValidUTF8(s[len(s)-40:], "")
	}
	s = strings.TrimSpace(s)
	// A component that is entirely dots is a traversal primitive even with the
	// separators stripped: filepath.Join(out, "..") leaves out.
	if strings.Trim(s, ".") == "" {
		return "untitled"
	}
	return s
}

// safeJoin builds a path under root from untrusted components.
//
// Every part is run through sanitize (which strips both separators, so no part
// can introduce a new path segment) and the result is then checked to still be
// inside root. The check is the load-bearing half: this connector names its
// downloads after IDs that come straight out of API responses, and an id of
// "x/../../../../tmp/pwn" turns filepath.Join(outDir, title+"-"+id+ext) into
// /tmp/pwn.mp4 - outside the directory the operator chose, with attacker-chosen
// content. Returns "" when the result would escape, and every caller treats ""
// as "skip this file" rather than writing somewhere else.
func safeJoin(root string, parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		clean = append(clean, sanitize(p))
	}
	joined := filepath.Join(append([]string{root}, clean...)...)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(absRoot, absJoined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return joined
}

func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	// duration form: e.g., 30d, 7d, 24h
	if strings.HasSuffix(s, "d") {
		var days int
		_, err := fmt.Sscanf(s, "%dd", &days)
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid date or duration")
}

// writeUnderDir writes body to name inside dir, refusing to escape dir and
// using 0600 because every payload this connector writes is customer content.
func writeUnderDir(dir, name string, body []byte) error {
	p := safeJoin(dir, name)
	if p == "" {
		return fmt.Errorf("refusing to write %q: it would escape %s", name, dir)
	}
	return credkit.WriteFile(p, body, 0o600)
}
