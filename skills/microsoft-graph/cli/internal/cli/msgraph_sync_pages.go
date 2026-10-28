// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// microsoft-graph handfix (sync-reads-every-page), class
// incomplete-read-as-complete: `sync` and `workflow archive` (and the MCP sync
// and workflow_archive tools, which run them) read each resource's Graph
// collection to its end before anything is stored or reported complete. The
// generated sync pages every resource with $skip and calls a page shorter than
// $top the end; Graph pages with @odata.nextLink and may return fewer items
// than $top asks (https://learn.microsoft.com/en-us/graph/paging), so a tenant
// with 120 alerts in pages of 50 was stored and reported as 50. syncResource
// now reads each collection whole through the same page walk pull and
// GetCollection use (graphReadPages, or graphReadSkipWindows for the $skip
// paged numberAssignments), under a stricter policy: a nextLink must name the
// configured host and a page not yet read, every page must carry a value
// array, and a failed page fails the read. A read that cannot be finished
// stores nothing and is a sync_error, never sync_complete.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"microsoft-graph-pp-cli/internal/client"
)

// graphRead is sync's read policy for one collection. A nil *graphRead is
// the policy every other reader keeps (see graphReadPages).
type graphRead struct {
	origins  map[string]bool // the configured API origin
	maxPages int             // requests the read may send; 0 = no cap
	pages    int             // requests sent
	capped   bool            // the cap stopped the read before the collection's end
	followed map[string]bool // every nextLink followed
}

func newSyncGraphRead(c *client.Client, maxPages int) *graphRead {
	return &graphRead{origins: pinnedOrigins(c.BaseURL), maxPages: maxPages, followed: map[string]bool{}}
}

// get sends one page request. A failed page after the first is a read that
// stopped part way, and says so.
func (rd *graphRead) get(ctx context.Context, c *client.Client, path string, params, headers map[string]string) (json.RawMessage, error) {
	if rd == nil {
		return c.GetWithHeaders(ctx, path, params, headers)
	}
	rd.pages++
	data, err := c.GetWithHeaders(ctx, path, params, headers)
	if err != nil && rd.pages > 1 {
		return nil, fmt.Errorf("page %d of %s failed, so the collection was not read to its end: %w", rd.pages, path, err)
	}
	return data, err
}

// pageNumber is the page an error names: the request count across the whole
// read under sync's policy, the walk's own count otherwise.
func (rd *graphRead) pageNumber(page int) int {
	if rd == nil {
		return page
	}
	return rd.pages
}

// follow vets the nextLink a page carries before it is followed: it must
// name the configured API origin (one that names another host is an error and
// is never followed) and a page this read has not followed already.
func (rd *graphRead) follow(path, next string, u *url.URL) error {
	if rd == nil {
		return nil
	}
	if err := checkPinned(rd.origins, u); err != nil {
		return fmt.Errorf("%s: page %d's @odata.nextLink names another host, so it was not followed and the collection was not read to its end: %w", path, rd.pages, err)
	}
	if rd.followed[next] {
		return fmt.Errorf("%s: page %d's @odata.nextLink names a page already read; the collection was not read to its end", path, rd.pages)
	}
	rd.followed[next] = true
	return nil
}

// capReached reports, when the read has more to fetch, whether --max-pages
// stops it here, and records the stop.
func (rd *graphRead) capReached() bool {
	if rd == nil || rd.maxPages <= 0 || rd.pages < rd.maxPages {
		return false
	}
	rd.capped = true
	return true
}

func (rd *graphRead) isCapped() bool { return rd != nil && rd.capped }

// syncGraphRead is how sync's read of one resource went.
type syncGraphRead struct {
	whole  bool              // items is the collection, or the --max-pages cap of it
	items  []json.RawMessage // every item read
	capped bool              // --max-pages stopped the read before the collection's end
}

// syncReadsGraphCollections reports whether syncResource reads through
// syncGetGraphCollection: every live sync does (the client is the
// connector's own); another client keeps the generated page loop.
func syncReadsGraphCollections(c any) bool {
	_, ok := c.(*client.Client)
	return ok
}

// syncGetGraphCollection reads one sync resource's collection to its end, or
// to the --max-pages cap, in one call; data is one page envelope holding every
// item. A --dry-run preview takes the generated request.
func syncGetGraphCollection(ctx context.Context, c interface {
	Get(context.Context, string, map[string]string) (json.RawMessage, error)
}, resource, path string, params map[string]string, maxPages int) (json.RawMessage, syncGraphRead, error) {
	gc, ok := c.(*client.Client)
	if !ok || gc.IsDryRun() {
		data, err := syncGet(ctx, c, resource, "", path, params)
		return data, syncGraphRead{}, err
	}
	rd := newSyncGraphRead(gc, maxPages)
	var items []json.RawMessage
	var err error
	if skipPagedCollections[path] {
		items, err = graphReadSkipWindows(ctx, gc, path, params, rd)
	} else {
		items, err = graphReadPages(ctx, gc, path, params, syncHTMLRequestHeaders(resource, ""), false, rd)
	}
	if err != nil {
		return nil, syncGraphRead{}, err
	}
	data, err := collectionPage(nil, items)
	if err != nil {
		return nil, syncGraphRead{}, err
	}
	return data, syncGraphRead{whole: true, items: items, capped: rd.capped}, nil
}

// syncGraphCollectionEnd ends a whole-collection read: read to its end it is
// complete; stopped by --max-pages it reports max_pages_cap exactly as the
// generated page loop does (no warning under --latest-only, which asked for
// the head page only).
func syncGraphCollectionEnd(syncEvents io.Writer, resource string, read syncGraphRead, maxPages int, latestOnly bool, stored int, outcome *partitionOutcome) {
	if !read.capped {
		outcome.complete = true
		return
	}
	outcome.reason = "max_pages_cap"
	if latestOnly {
		return
	}
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "\n  %s: reached --max-pages limit (%d pages, %d items)\n", resource, maxPages, stored)
		return
	}
	fmt.Fprintf(syncEvents, `{"event":"sync_warning","resource":"%s","reason":"max_pages_cap_hit","message":"reached --max-pages cap of %d; data may be truncated. Re-run with --max-pages 0 (unlimited) or higher to verify."}`+"\n", resource, maxPages)
}
