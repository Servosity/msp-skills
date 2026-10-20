// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/client"
	"backup-radar-pp-cli/internal/radar"
)

type pushChunkReport struct {
	Chunk   int `json:"chunk"`
	Entries int `json:"entries"`
	Results int `json:"results"`
	Status  int `json:"status,omitempty"`
}

type pushView struct {
	DryRun        bool                `json:"dry_run"`
	Rows          int                 `json:"rows"`
	Chunks        []pushChunkReport   `json:"chunks"`
	PostedChunks  int                 `json:"posted_chunks"`
	PostedResults int                 `json:"posted_results"`
	Failed        *pushFailure        `json:"failed,omitempty"`
	PreviewBody   []radar.IngestEntry `json:"preview_body,omitempty"`
}

type pushFailure struct {
	Chunk   int    `json:"chunk"`
	Error   string `json:"error"`
	Outcome string `json:"outcome"`
}

func newNovelPushCmd(flags *rootFlags) *cobra.Command {
	var from string
	var batch int

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Validate flat CSV/JSONL backup results and push them in batches under the API limit",
		Long: `Read flat backup results (CSV with a header row, JSONL, or a JSON array) with
columns client_name, device_name, job_name, time, status and optionally type
(backups, replications, verifications, boot_verifications, alerts; default
backups), notes (semicolon-separated), data_selected and data_transferred.
Every row is validated locally (status success|warning|failure, parseable time,
non-empty names) and all errors are reported at once. Rows are grouped by
client/device/job and posted to POST /backups in chunks of at most --batch
results, because the API rejects a whole request when any entry is invalid or
it carries more than 10,000 results. Times without a zone are read as UTC.
--dry-run validates and shows the chunk plan plus preview_body, the exact
JSON array the first request would send (usable with 'backups ingest --stdin').

Use this command to push flat CSV/JSONL backup results, or batches over 10,000 results, into Backup Radar.
Do NOT use this command for an already-shaped ingest JSON array under the cap; use 'backups ingest' instead.`,
		Example: `  backup-radar-cli push --from results.csv --dry-run
  backup-radar-cli push --from results.jsonl --json
  my-backup-script --emit-csv | backup-radar-cli push --from -`,
		// --from reads a file path; block it for MCP callers so a tool call can
		// never read arbitrary files from the server account.
		// Hidden from MCP: its only input is a file path, which MCP callers may
		// not supply; agents use the backups_ingest tool with an inline body.
		Annotations: map[string]string{"pp:data-source": "live", "mcp:write-flags": "from", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "push")
				}
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--from is required (a CSV/JSONL/JSON file path, or - for stdin)"))
			}
			if batch < 1 || batch > radar.MaxIngestResults {
				return usageErr(fmt.Errorf("--batch must be between 1 and %d", radar.MaxIngestResults))
			}
			data, err := readPathOrStdin(cmd, from)
			if err != nil {
				if dryRunOK(flags) && requiredInputWaived() {
					return writeDryRun(cmd.OutOrStdout(), flags, "push")
				}
				return fmt.Errorf("reading --from: %w", err)
			}
			rows, rowErrs := radar.ParseFlatRows(data)
			if len(rowErrs) > 0 {
				for _, e := range rowErrs {
					fmt.Fprintln(cmd.ErrOrStderr(), "invalid:", e)
				}
				return usageErr(fmt.Errorf("%d invalid row(s); nothing was pushed", len(rowErrs)))
			}
			chunks := radar.BuildChunks(rows, batch)
			view := pushView{DryRun: flags.dryRun, Rows: len(rows), Chunks: make([]pushChunkReport, 0, len(chunks))}
			for i, ch := range chunks {
				view.Chunks = append(view.Chunks, pushChunkReport{Chunk: i + 1, Entries: len(ch.Entries), Results: ch.Results})
			}
			if flags.dryRun {
				if len(chunks) > 0 {
					view.PreviewBody = chunks[0].Entries
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			for i, ch := range chunks {
				_, status, err := c.PostWithParams(cmd.Context(), "/backups", map[string]string{}, ch.Entries)
				if err == nil && (status < 200 || status >= 300) {
					err = fmt.Errorf("HTTP %d", status)
				}
				if err != nil {
					// Only a 4xx proves the API refused the chunk; a 5xx, timeout or
					// dropped connection can follow an accepted one.
					var apiErrResp *client.APIError
					outcome := "unknown: the request may have been accepted; check Backup Radar before re-sending this chunk"
					if errors.As(err, &apiErrResp) && apiErrResp.StatusCode >= 400 && apiErrResp.StatusCode < 500 {
						outcome = "rejected by the API"
					}
					view.Failed = &pushFailure{Chunk: i + 1, Error: err.Error(), Outcome: outcome}
					if perr := printJSONFiltered(cmd.OutOrStdout(), view, flags); perr != nil {
						return perr
					}
					accepted := "no earlier chunk was sent"
					if i > 0 {
						accepted = fmt.Sprintf("chunks 1-%d were accepted", i)
					}
					chunkErr := errors.Join(fmt.Errorf("chunk %d of %d failed (%s); %s and nothing after it was sent", i+1, len(chunks), outcome, accepted), err)
					if apiErrResp != nil && (apiErrResp.StatusCode == 401 || apiErrResp.StatusCode == 403) {
						// HAND-FIX(push-auth-exit-code): a rejected key is exit 4, like backups ingest.
						return authErr(fmt.Errorf("%w\nhint: push uses the ScalePad API key (BACKUP_RADAR_API_KEY); run 'backup-radar-cli doctor' to check it", chunkErr))
					}
					return apiErr(chunkErr)
				}
				view.Chunks[i].Status = status
				view.PostedChunks++
				view.PostedResults += ch.Results
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Flat results file (CSV with header, JSONL, or JSON array), or - for stdin")
	cmd.Flags().IntVar(&batch, "batch", radar.MaxIngestResults, "Maximum results per request (1-10000)")
	return cmd
}
