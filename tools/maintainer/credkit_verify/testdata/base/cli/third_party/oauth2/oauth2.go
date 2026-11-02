// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package oauth2 stands in for golang.org/x/oauth2.
package oauth2

import (
	"context"
	"net/http"
)

type Token struct{}

// Holder holds a client the way a library config does.
type Holder struct{ HTTPClient *http.Client }

func NewClient(ctx context.Context, src any) *http.Client { return &http.Client{} }
