// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json netsapiens-cached-read-never-overwrites-a-walk):
// lets a caller see that a read came from the HTTP cache. An answer up to five
// minutes old must not be written over what a later sync walk stored.

package client

import (
	"context"
	"sync/atomic"
)

type cacheHitKey struct{}

// WithCacheHitReport returns a context whose reads set the flag when any of them
// is served from the response cache.
func WithCacheHitReport(ctx context.Context) (context.Context, *atomic.Bool) {
	hit := &atomic.Bool{}
	return context.WithValue(ctx, cacheHitKey{}, hit), hit
}

func reportCacheHit(ctx context.Context) {
	if hit, _ := ctx.Value(cacheHitKey{}).(*atomic.Bool); hit != nil {
		hit.Store(true)
	}
}
