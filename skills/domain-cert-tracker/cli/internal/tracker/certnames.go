// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"sort"
	"sync"
	"time"

	"domain-cert-tracker-pp-cli/internal/store"
)

// certDownloader fetches one certificate by its crt.sh id.
type certDownloader interface {
	Certificate(ctx context.Context, id int64) ([]byte, error)
}

// certNamer replaces crt.sh's matched names with each certificate's own SAN
// list during one sweep. It downloads each crt.sh id at most once per
// sweep (and never again once verified), at most limit downloads in all.
type certNamer struct {
	st    *store.Store
	src   certDownloader
	now   func() time.Time
	mu    sync.Mutex
	left  int
	calls map[int64]*nameCall
	recs  []store.CertNames
	stats CertNameStats
}

// nameCall is one certificate's download in this sweep. names is nil
// unless the download was verified against the crt.sh row.
type nameCall struct {
	done     chan struct{}
	names    []string
	err      string
	deferred bool
}

const (
	whyCapped   = "not downloaded: the per-sweep certificate download cap was reached"
	whyTimedOut = "not downloaded: the CT check ran out of time"
)

func newCertNamer(st *store.Store, src certDownloader, limit int, now func() time.Time) *certNamer {
	switch {
	case limit == 0:
		limit = DefaultCertDownloads
	case limit < 0:
		limit = 0
	}
	return &certNamer{st: st, src: src, now: now, left: limit, calls: map[int64]*nameCall{}}
}

// complete sets each certificate's Names to the SAN list read from the
// certificate itself where that is known or can be downloaded now, and
// marks every other certificate as carrying crt.sh's partial list. Nothing
// but a verified download (now or stored) marks a list complete.
func (n *certNamer) complete(ctx context.Context, certs []CTCert) {
	for i := range certs {
		certs[i].NamesSource, certs[i].NamesError = store.NamesFromCrtSh, ""
	}
	if len(certs) == 0 {
		return
	}
	partial := func(c *CTCert, why string, deferred, failed bool) {
		c.NamesError = why
		n.mu.Lock()
		n.stats.Partial++
		if deferred {
			n.stats.Deferred++
		}
		if failed {
			n.stats.DownloadFailed++
		}
		n.mu.Unlock()
	}
	full := func(c *CTCert, names []string) {
		c.Names, c.NamesSource = append([]string(nil), names...), store.NamesFromCertificate
		n.mu.Lock()
		n.stats.Complete++
		n.mu.Unlock()
	}
	if n.src == nil || n.st == nil {
		for i := range certs {
			partial(&certs[i], "certificate download is not configured", false, false)
		}
		return
	}
	ids := make([]int64, len(certs))
	for i, c := range certs {
		ids[i] = c.ID
	}
	stored, err := n.st.CertNamesFor(ctx, ids)
	if err != nil {
		for i := range certs {
			partial(&certs[i], "reading stored certificate names: "+err.Error(), false, false)
		}
		return
	}
	var todo []int
	for i, c := range certs {
		if rec, ok := stored[c.ID]; ok && rec.Status == store.CertNamesOK && len(rec.Names) > 0 {
			full(&certs[i], rec.Names)
			continue
		}
		todo = append(todo, i)
	}
	// Newest first, so the issuer audit's recent window completes first;
	// a certificate that failed before goes after every one never tried.
	sort.SliceStable(todo, func(a, b int) bool {
		ca, cb := certs[todo[a]], certs[todo[b]]
		_, fa := stored[ca.ID]
		_, fb := stored[cb.ID]
		if fa != fb {
			return !fa
		}
		if ca.NotBefore != cb.NotBefore {
			return ca.NotBefore > cb.NotBefore
		}
		return ca.ID > cb.ID
	})
	// Downloads may use half the time left for this check; CAA lookups for
	// the names they add need the rest.
	fctx := ctx
	if dl, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		fctx, cancel = context.WithDeadline(ctx, time.Now().Add(time.Until(dl)/2))
		defer cancel()
	}
	for _, i := range todo {
		r := n.fetch(fctx, certs[i])
		switch {
		case r.names != nil:
			full(&certs[i], r.names)
		case r.deferred:
			partial(&certs[i], r.err, true, false)
		default:
			partial(&certs[i], r.err, false, true)
		}
	}
}

// fetch downloads and verifies one certificate, once per crt.sh id per
// sweep; a second check that needs the same id waits for the first.
func (n *certNamer) fetch(ctx context.Context, c CTCert) nameCall {
	n.mu.Lock()
	if call, ok := n.calls[c.ID]; ok {
		n.mu.Unlock()
		select {
		case <-call.done:
			return *call
		case <-ctx.Done():
			return nameCall{err: whyTimedOut, deferred: true}
		}
	}
	if ctx.Err() != nil {
		n.mu.Unlock()
		return nameCall{err: whyTimedOut, deferred: true}
	}
	if n.left <= 0 {
		n.mu.Unlock()
		return nameCall{err: whyCapped, deferred: true}
	}
	n.left--
	call := &nameCall{done: make(chan struct{})}
	n.calls[c.ID] = call
	n.mu.Unlock()

	body, err := n.src.Certificate(ctx, c.ID)
	var names []string
	if err == nil {
		names, err = CertificateNames(body, c)
	}
	at := n.now().UTC().Format(time.RFC3339Nano)
	n.mu.Lock()
	switch {
	case err == nil:
		call.names = names
		n.recs = append(n.recs, store.CertNames{CrtID: c.ID, Status: store.CertNamesOK, Names: names, CheckedAt: at})
		n.stats.Downloaded++
	case ctx.Err() != nil:
		call.err, call.deferred = whyTimedOut, true
	default:
		call.err = "certificate download failed: " + err.Error()
		n.recs = append(n.recs, store.CertNames{CrtID: c.ID, Status: store.CertNamesFailed, Error: err.Error(), CheckedAt: at})
	}
	n.mu.Unlock()
	close(call.done)
	return *call
}

func (n *certNamer) records() []store.CertNames {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]store.CertNames(nil), n.recs...)
}

func (n *certNamer) totals() CertNameStats {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.stats
}
