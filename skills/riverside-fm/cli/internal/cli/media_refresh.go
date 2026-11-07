// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"riverside-fm-pp-cli/internal/cliutil"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newMediaCmd is the parent for media operations beyond the spec-derived productions list.
func newMediaCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "media",
		Short: "Media operations: refresh CloudFront signed URLs and optionally prefetch bodies before TTL expires.",
	}
	cmd.AddCommand(newMediaRefreshCmd(flags))
	return cmd
}

func newMediaRefreshCmd(flags *rootFlags) *cobra.Command {
	var projectID string
	var productionID string
	var prefetch bool
	var outDir string

	cmd := &cobra.Command{
		Use:         "refresh",
		Short:       "Refresh signed CloudFront URLs for project clips or production media; --prefetch downloads bodies before TTL.",
		Long:        "Walks the production media board (--production) and/or project clip-assets (--project), captures the fresh CloudFront URLs, and optionally streams each asset to <out>/. Riverside's signed URLs are short-lived; this is the only way to refresh them in bulk.",
		Example:     "  riverside-fm-cli media refresh --project 69fcda9fba030a19ae93a526 --prefetch --out ./media",
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:write-flags": "out,prefetch"}, // --prefetch writes files; MCP gets the refresh only
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if projectID == "" && productionID == "" {
				return usageErr(fmt.Errorf("provide --project, --production, or both"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if prefetch {
				if outDir == "" {
					outDir = "./media-refresh"
				}
				if err := os.MkdirAll(outDir, 0o700); err != nil {
					return err
				}
			}

			out := []mediaRow{}

			if productionID != "" {
				data, err := c.Get(cmd.Context(), "/api/v4/production/"+url.PathEscape(productionID)+"/media", nil)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				items := extractProductionMedia(data)
				for _, it := range items {
					row := mediaRow{Kind: "production-media", Name: it.Name, ID: it.ID, URL: it.URL, DurationSec: it.Duration}
					if prefetch && row.URL != "" {
						fp := safeJoin(outDir, it.Name+"-"+it.ID+guessExt(it.URL))
						if fp == "" {
							row.Status = "skipped: name/id would escape --out"
							out = append(out, row)
							continue
						}
						n, derr := streamDownload(row.URL, fp)
						if derr == nil {
							row.LocalPath = fp
							row.Bytes = n
							row.Status = "downloaded"
						} else {
							row.Status = "failed: " + derr.Error()
						}
					}
					out = append(out, row)
				}
			}

			if projectID != "" {
				// project -> exports list -> per-clip -> clip-assets
				clipIDs, err := listAll(cmd.Context(), c, "/api/v4/projects/"+url.PathEscape(projectID)+"/clips/exports", nil, 200,
					extractClipIDsFromExports, func(id string) string { return id })
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				for _, cid := range clipIDs {
					clipData, cerr := c.Get(cmd.Context(), "/api/v4/clip/"+url.PathEscape(cid), nil)
					if cerr != nil {
						// Report it: a clip that silently vanished from the refresh would
						// read as "nothing to refresh / prefetch" for that clip.
						out = append(out, mediaRow{Kind: "clip-export", ID: cid, Status: "fetch-failed: " + cerr.Error()})
						continue
					}
					urls := extractClipExportURLs(clipData)
					for _, u := range urls {
						row := mediaRow{Kind: "clip-export", Name: u.Name, ID: cid, URL: u.URL}
						if prefetch && row.URL != "" {
							fp := safeJoin(outDir, u.Name+"-"+cid+guessExt(u.URL))
							if fp == "" {
								row.Status = "skipped: name/id would escape --out"
								out = append(out, row)
								continue
							}
							n, derr := streamDownload(row.URL, fp)
							if derr == nil {
								row.LocalPath = fp
								row.Bytes = n
								row.Status = "downloaded"
							} else {
								row.Status = "failed: " + derr.Error()
							}
						}
						out = append(out, row)
					}
				}
			}

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				j, _ := json.MarshalIndent(out, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(j))
			} else {
				printMediaRows(cmd.OutOrStdout(), out)
			}
			statuses := make([]string, len(out))
			for i, r := range out {
				statuses[i] = r.Status
			}
			return rowsIncomplete("media refresh", statuses)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "Project ID — refresh URLs for the project's clip exports")
	cmd.Flags().StringVar(&productionID, "production", "", "Production ID — refresh URLs for the production's media board")
	cmd.Flags().BoolVar(&prefetch, "prefetch", false, "Download every asset body to --out before TTL expires")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory when --prefetch (default ./media-refresh)")
	return cmd
}

// mediaRow is one line of media refresh output.
type mediaRow struct {
	Kind        string  `json:"kind"` // "production-media" | "clip-asset"
	Name        string  `json:"name"`
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	DurationSec float64 `json:"duration_sec,omitempty"`
	LocalPath   string  `json:"local_path,omitempty"`
	Bytes       int64   `json:"bytes,omitempty"`
	Status      string  `json:"status,omitempty"`
}

func printMediaRows(w io.Writer, rows []mediaRow) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "No media found.")
		return
	}
	for _, r := range rows {
		if r.LocalPath != "" {
			fmt.Fprintf(w, "%s %s (%s) -> %s (%d bytes) %s\n", r.Kind, r.Name, r.ID, r.LocalPath, r.Bytes, r.Status)
		} else {
			fmt.Fprintf(w, "%s %s (%s) %s %s\n", r.Kind, r.Name, r.ID, truncate(r.URL, 110), r.Status)
		}
	}
}

type productionMediaItem struct {
	ID       string
	Name     string
	URL      string
	Duration float64
}

func extractProductionMedia(data json.RawMessage) []productionMediaItem {
	var probe struct {
		Media []struct {
			ID       string  `json:"_id"`
			Name     string  `json:"name"`
			URL      string  `json:"url"`
			Duration float64 `json:"duration"`
		} `json:"media"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return nil
	}
	out := make([]productionMediaItem, 0, len(probe.Media))
	for _, m := range probe.Media {
		out = append(out, productionMediaItem{ID: m.ID, Name: m.Name, URL: m.URL, Duration: m.Duration})
	}
	return out
}

func extractClipIDsFromExports(data json.RawMessage) []string {
	var probe struct {
		Clips []struct {
			ID string `json:"_id"`
		} `json:"clips"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return nil
	}
	out := make([]string, 0, len(probe.Clips))
	for _, c := range probe.Clips {
		if c.ID != "" {
			out = append(out, c.ID)
		}
	}
	return out
}

type clipExportURL struct{ Name, URL string }

func extractClipExportURLs(data json.RawMessage) []clipExportURL {
	// Clip detail returns deeply nested; we extract any HTTP(S) URLs from "exports.*.key" or "url" fields.
	// Simple approach: walk JSON looking for fields whose value is a CloudFront-signed URL.
	var doc any
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []clipExportURL
	walkForURLs(doc, "", &out)
	return out
}

func walkForURLs(v any, parentKey string, out *[]clipExportURL) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			walkForURLs(child, k, out)
		}
	case []any:
		for _, child := range t {
			walkForURLs(child, parentKey, out)
		}
	case string:
		if strings.HasPrefix(t, "https://") && strings.Contains(t, "riverside.com") && strings.Contains(t, "Signature=") {
			*out = append(*out, clipExportURL{Name: parentKey, URL: t})
		}
		if strings.HasPrefix(t, "https://private-assets.riverside.com") {
			*out = append(*out, clipExportURL{Name: parentKey, URL: t})
		}
	}
}

func streamDownload(srcURL, destPath string) (int64, error) {
	client := &http.Client{Timeout: 5 * time.Minute, Transport: cliutil.OutboundGuard(nil)}
	resp, err := client.Get(srcURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// 0600, not os.Create's 0666-before-umask: what lands here is a customer's
	// recorded audio or video. destPath is built by safeJoin, which proved it
	// stays under the operator-chosen --out directory.
	// The body is customer media from a signed CDN URL, sent no credential, and
	// can run to gigabytes of binary, so it goes to its file outside the kit,
	// whose file sink checks text units (handfixes.json credkit-wiring).
	// #nosec G304 -- safeJoin-validated path under --out.
	f, err := os.OpenFile(filepath.Clean(destPath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return n, err
}

func guessExt(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p := u.Path
	if i := strings.LastIndex(p, "."); i >= 0 && i < len(p)-1 {
		ext := p[i:]
		// strip CloudFront signing params accidentally included
		if len(ext) <= 6 {
			return ext
		}
	}
	return ""
}
