// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel command (not generated). Implements binary/multipart
// upload to POST /wp-json/wp/v2/media, which the generated JSON-body client
// path cannot do. Lives in its own file so `generate --force` preserves it
// whole; the AddCommand wiring in media.go is re-injected by regen-merge.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wordpress-pp-cli/internal/cliutil"
	"wordpress-pp-cli/internal/config"

	"github.com/spf13/cobra"
)

func newMediaUploadCmd(flags *rootFlags) *cobra.Command {
	var title, altText, caption, contentType, filename string

	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload an image, video, audio, or document to the media library",
		Long: "Upload a local file to the WordPress media library via a binary multipart request.\n" +
			"Handles images (png/jpg/gif/webp/svg), video, audio, and documents (PDF). The MIME\n" +
			"type is auto-detected from the file extension (override with --content-type).\n\n" +
			"Returns the created media item including its id and source_url. Use the id as\n" +
			"--featured-media on a page or post. Optional --title/--alt-text/--caption set\n" +
			"metadata in a follow-up call (alt text matters for SEO/accessibility).",
		Example: "  wordpress-cli media upload ./hero.png --alt-text \"Landing page hero\"",
		Annotations: map[string]string{
			"pp:endpoint": "media.upload", "pp:method": "POST", "pp:path": "/media",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 || args[0] == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a file path is required\nUsage: %s <file>", cmd.CommandPath()))
			}
			file := args[0]

			// Pure, IO-free resolution so it is safe before the dry-run guard.
			if filename == "" {
				filename = filepath.Base(file)
			}
			if contentType == "" {
				contentType = mediaContentTypeFromExt(file)
			}

			// Verify-mode and --dry-run preview WITHOUT reading or sending the
			// file (Phase 3 rule: no filesystem read before dryRunOK).
			if dryRunOK(flags) || flags.dryRun || cliutil.IsVerifyEnv() {
				size := int64(-1)
				if fi, statErr := os.Stat(file); statErr == nil {
					size = fi.Size()
				}
				ct := contentType
				if ct == "" {
					ct = "(auto-detected at upload)"
				}
				env := map[string]any{
					"action": "upload", "resource": "media", "path": "/media",
					"dry_run": true, "file": file, "filename": filename,
					"content_type": ct, "size_bytes": size,
				}
				b, _ := json.Marshal(env)
				return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage(b), flags)
			}

			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("reading file %q: %w", file, err)
			}
			if len(data) == 0 {
				return usageErr(fmt.Errorf("file %q is empty", file))
			}
			if contentType == "" {
				contentType = http.DetectContentType(data)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if cfg.WordpressBasicAuth == "" {
				return usageErr(fmt.Errorf("not authenticated: set WORDPRESS_BASIC_AUTH (base64 of \"user:app-password\") or run 'auth set-token'"))
			}
			base := strings.TrimRight(cfg.BaseURL, "/")
			authHeader := mediaBasicAuth(cfg.WordpressBasicAuth)

			mediaJSON, status, err := mediaDoUpload(cmd.Context(), flags.timeout, base, authHeader, filename, contentType, data)
			if err != nil {
				return err
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("upload failed: HTTP %d: %s", status, mediaTruncate(string(mediaJSON), 400))
			}
			if !mediaLooksLikeJSON(mediaJSON) {
				return fmt.Errorf("WordPress returned a non-JSON response (HTTP %d) — the REST API may not be routing at this base URL. "+
					"Confirm pretty permalinks are enabled and WORDPRESS_BASE_URL ends in /wp-json/wp/v2. First bytes: %s",
					status, mediaTruncate(string(mediaJSON), 120))
			}

			// Optional metadata in a follow-up JSON call: the binary upload body
			// carries the file only, so title/alt_text/caption are set via update.
			if title != "" || altText != "" || caption != "" {
				if id := mediaExtractID(mediaJSON); id != "" {
					meta := map[string]any{}
					if title != "" {
						meta["title"] = title
					}
					if altText != "" {
						meta["alt_text"] = altText
					}
					if caption != "" {
						meta["caption"] = caption
					}
					updated, ustatus, uerr := mediaDoJSONPost(cmd.Context(), flags.timeout, base+"/media/"+id, authHeader, meta)
					if uerr == nil && ustatus >= 200 && ustatus < 300 {
						mediaJSON = updated
					} else {
						fmt.Fprintf(os.Stderr, "warning: file uploaded (id %s) but metadata update failed (HTTP %d): %v\n", id, ustatus, uerr)
					}
				}
			}

			writeMutationResponseToStore(cmd.Context(), "media", mediaJSON, "")
			return printOutputWithFlags(cmd.OutOrStdout(), mediaJSON, flags)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Media title")
	cmd.Flags().StringVar(&altText, "alt-text", "", "Alt text (SEO/accessibility)")
	cmd.Flags().StringVar(&caption, "caption", "", "Caption HTML")
	cmd.Flags().StringVar(&contentType, "content-type", "", "MIME type (auto-detected from extension if omitted)")
	cmd.Flags().StringVar(&filename, "filename", "", "Upload filename (defaults to the file's base name)")
	return cmd
}

// mediaContentTypeFromExt resolves a MIME type from the file extension only —
// no filesystem read, so it is safe to call before the dry-run guard. Returns
// "" when the extension is unknown (the caller sniffs bytes on the real path).
func mediaContentTypeFromExt(path string) string {
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i] // drop "; charset=..."
	}
	return strings.TrimSpace(ct)
}

// mediaBasicAuth builds the Authorization header value. The token is expected
// to be base64("user:app-password"); tolerate a value already prefixed.
func mediaBasicAuth(token string) string {
	if strings.HasPrefix(token, "Basic ") {
		return token
	}
	return "Basic " + token
}

func mediaHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// mediaDoUpload POSTs raw file bytes to /media with the Content-Disposition
// header WordPress requires to name the upload.
func mediaDoUpload(ctx context.Context, timeout time.Duration, base, auth, filename, contentType string, data []byte) (json.RawMessage, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/media", bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	req.Header.Set("Accept", "application/json")

	resp, err := mediaHTTPClient(timeout).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("uploading media: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return json.RawMessage(body), resp.StatusCode, nil
}

func mediaDoJSONPost(ctx context.Context, timeout time.Duration, url, auth string, body map[string]any) (json.RawMessage, int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := mediaHTTPClient(timeout).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return json.RawMessage(rb), resp.StatusCode, nil
}

// mediaExtractID pulls the integer "id" from a media JSON object as a string.
func mediaExtractID(data json.RawMessage) string {
	var obj struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}
	return obj.ID.String()
}

// mediaLooksLikeJSON reports whether the response body is a JSON object/array,
// distinguishing a real REST payload from an HTML page (homepage/error) that
// WordPress serves when the REST route isn't matched (e.g. plain permalinks).
func mediaLooksLikeJSON(b []byte) bool {
	s := bytes.TrimSpace(b)
	return len(s) > 0 && (s[0] == '{' || s[0] == '[')
}

func mediaTruncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
