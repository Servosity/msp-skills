// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package httpx stands in for a third-party library that builds HTTP clients.
package httpx

import (
	"fmt"
	"net/http"
)

func NewClient() *http.Client { return &http.Client{} }

func NewTransport() http.RoundTripper { return &http.Transport{} }

// Printer prints without being an io.Writer, as a cobra command does.
type Printer struct{}

func (Printer) Println(a ...any) { fmt.Println(a...) }
