// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"strings"
)

// StoredLayer is the configuration the files recorded (config file and
// credential store) before any environment override: the layer a stored
// minted token was saved beside. A configuration Load did not build is its own
// record. crowdstrike handfix: minted-token-bound-to-its-client.
func (c *Config) StoredLayer() *Config {
	if c == nil || c.fileConfig == nil {
		return c
	}
	return c.fileConfig
}

// PairIsStored reports whether the client pair in effect is the pair the files
// hold, so a token minted for it may be saved beside it. A pair the
// environment supplies keeps its token in memory: saved, it would read as the
// stored client's token on the next run.
func (c *Config) PairIsStored() bool {
	if c == nil || strings.TrimSpace(c.ClientID) == "" || c.ClientSecret == "" {
		return false
	}
	s := c.StoredLayer()
	return c.ClientID == s.ClientID && c.ClientSecret == s.ClientSecret
}

// ErrTokenForAnotherClient marks a token save refused because the credential
// store would keep the token beside a client ID that did not mint it.
var ErrTokenForAnotherClient = errors.New("the credential store holds another client ID, and a token is saved only beside the client that minted it; pass --client-id and --client-secret to store this client, or run auth logout first")

// tokenSavedBesideItsClient is the one rule every save of a minted token (auth
// login, the automatic mint, the refresh grant) passes: the client ID the
// credential store records beside the token is the client ID that minted it,
// or none (a token no stored client claims, which only a run with no client
// pair uses). The save keeps the stored pair whenever the pair in effect came
// from the environment (configForSave), so `auth login` with FALCON_CLIENT_ID
// set saved that client's token beside the stored client, and the next run
// sent it as the stored client's token and read the other tenant. A token
// with no minting client (auth set-token passes none) is not checked.
// crowdstrike handfix minted-token-bound-to-its-client.
func (c *Config) tokenSavedBesideItsClient(clientID, accessToken string) error {
	clientID = strings.TrimSpace(clientID)
	if accessToken == "" || clientID == "" {
		return nil
	}
	recorded := clientID
	if c.envOverrides["ClientID"] && c.fileConfig != nil {
		recorded = strings.TrimSpace(c.fileConfig.ClientID)
	}
	if recorded != "" && recorded != clientID {
		return ErrTokenForAnotherClient
	}
	return nil
}
