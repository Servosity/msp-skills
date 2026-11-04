// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): where the webhook signature secret lives.
// Pinned by this connector's handfixes.json under the "webhooks-listener"
// entry. client.HoldStartupCredentials holds this file and the environment
// variable before anything prints, like every other credential.

package cliutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zammad-pp-cli/internal/credkit"
)

// WebhookSecretPathNextTo is the secret file beside a credentials file.
func WebhookSecretPathNextTo(credentialsPath string) string {
	return filepath.Join(filepath.Dir(credentialsPath), "webhook_secret")
}

// WebhookSecretPath is the secret file for configPath ("" for the default
// configuration), beside the API token's credentials file.
func WebhookSecretPath(configPath string) (string, error) {
	creds, err := CredentialsFilePath()
	if strings.TrimSpace(configPath) != "" {
		creds, err = CredentialsFilePathForConfig(configPath)
	}
	if err != nil {
		return "", err
	}
	return WebhookSecretPathNextTo(creds), nil
}

// LoadWebhookSecret returns the secret and where it came from ("env" for
// ZAMMAD_WEBHOOK_SECRET, or "store"), or "" and "none". There is no flag: a
// flag value lands in shell history and process listings. A stored secret
// other users can read or change is refused, not used.
func LoadWebhookSecret(configPath string) (secret, source string, err error) {
	if v := strings.TrimSpace(os.Getenv("ZAMMAD_WEBHOOK_SECRET")); v != "" {
		return v, "env", nil
	}
	path, err := WebhookSecretPath(configPath)
	if err != nil {
		return "", "none", err
	}
	opened, err := ReadOpenedConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "none", nil
	}
	if err != nil {
		return "", "none", err
	}
	v := strings.TrimSpace(string(opened.Data))
	credkit.Hold("minted:webhook-secret", v)
	if opened.PermErr != nil {
		return "", "none", fmt.Errorf("webhook secret file %s is not private, refusing it: %w", path, opened.PermErr)
	}
	if v == "" {
		return "", "none", nil
	}
	return v, "store", nil
}

// NewWebhookSecret generates a secret, holds it, and stores it with 0600
// permissions beside the API token.
func NewWebhookSecret(configPath string) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b)
	credkit.Hold("minted:webhook-secret", secret)
	path, err := WebhookSecretPath(configPath)
	if err != nil {
		return "", err
	}
	if err := AtomicWritePrivateFile(path, []byte(secret+"\n"), 0o600, 0o700); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if err := VerifyCredsPerms(real); err != nil {
		return "", fmt.Errorf("webhook secret file %s: %w", path, err)
	}
	return secret, nil
}
