// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"testing"
	"time"
)

// The spec declares http_transport "standard": the client is net/http's own
// transport (a clone of the default, so the environment proxy is kept), the
// transport-layer header wait follows --timeout, and a client built without a
// jar still keeps a session's cookies.
func TestHTTPClientIsStandardTransport(t *testing.T) {
	c := newHTTPClient(42*time.Second, nil)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Error("the environment proxy setting was dropped")
	}
	if tr.ResponseHeaderTimeout != 42*time.Second || c.Timeout != 42*time.Second {
		t.Errorf("header wait %v, client timeout %v; want both 42s", tr.ResponseHeaderTimeout, c.Timeout)
	}
	if c.Jar == nil {
		t.Error("a client built without a jar keeps no session cookies")
	}
}
