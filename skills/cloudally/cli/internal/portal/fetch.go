// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FetchChunk mints a link for one export chunk and streams it to w,
// returning the number of bytes written.
//
// Minting and streaming are deliberately one operation. The link is valid
// for only 60 seconds (see LinkExpiry), so there is no safe way to hand it
// to anything else first. S3 authorises at request time, so once the
// transfer has begun it continues past the expiry even for a long download.
func (s *Session) FetchChunk(ctx context.Context, exportDataID string, w io.Writer) (int64, error) {
	link, err := s.DownloadURL(ctx, exportDataID)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return 0, fmt.Errorf("portal: building the download request: %w", scrubURLError(err))
	}
	// A presigned URL carries its own authorisation; sending the session
	// cookie or CSRF header to object storage would be pointless and
	// would leak them to a third party.
	resp, err := s.downloadClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("portal: downloading chunk %s: %w", exportDataID, scrubURLError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("portal: downloading chunk %s: status %d: %s",
			exportDataID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("portal: streaming chunk %s after %d bytes: %w", exportDataID, n, err)
	}
	return n, nil
}

// downloadClient returns a client with no timeout, for transfers that can
// run for hours. The session's own client has a per-request timeout suited
// to service calls, which would abort a large archive mid-stream.
func (s *Session) downloadClient() *http.Client {
	return &http.Client{
		Transport:     s.hc.Transport,
		CheckRedirect: s.hc.CheckRedirect,
	}
}
