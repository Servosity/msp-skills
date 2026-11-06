// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func decodedBasic(t *testing.T, h string) string {
	t.Helper()
	enc, ok := strings.CutPrefix(h, "Basic ")
	if !ok {
		t.Fatalf("AuthHeader() = %q, want a Basic header", h)
	}
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode %q: %v", h, err)
	}
	return string(dec)
}

func TestLegacyUsernamePasswordPairStillAuthenticates(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("MAXIO_API_KEY", "")
	t.Setenv("MAXIO_USERNAME", "legacy-key-123")
	t.Setenv("MAXIO_PASSWORD", "x")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := decodedBasic(t, cfg.AuthHeader()); got != "legacy-key-123:x" {
		t.Fatalf("Basic payload = %q, want legacy-key-123:x", got)
	}
	if cfg.AuthSource != "env:MAXIO_USERNAME" {
		t.Fatalf("AuthSource = %q", cfg.AuthSource)
	}
}

func TestAPIKeyWinsOverLegacyUsername(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("MAXIO_API_KEY", "canonical-key")
	t.Setenv("MAXIO_USERNAME", "legacy-key")
	t.Setenv("MAXIO_PASSWORD", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := decodedBasic(t, cfg.AuthHeader()); got != "canonical-key:x" {
		t.Fatalf("Basic payload = %q, want canonical-key:x", got)
	}
}

func TestLegacyPasswordOtherThanXIsRefusedWithoutEcho(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("MAXIO_USERNAME", "legacy-key")
	t.Setenv("MAXIO_PASSWORD", "hunter2-not-x")
	_, err := Load("")
	if !errors.Is(err, ErrLegacyPassword) {
		t.Fatalf("Load error = %v, want ErrLegacyPassword", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error repeats the password: %v", err)
	}
}
