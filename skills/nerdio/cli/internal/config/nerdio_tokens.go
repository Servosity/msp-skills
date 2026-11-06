// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"strings"
)

// StoredLayer is the configuration the files recorded (config file and
// credential store) before any environment override: the layer a stored
// minted token was saved beside. A configuration Load did not build is its own
// record. nerdio handfix: minted-token-bound-to-its-client.
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

// ErrTokenPairNotStored refuses a token save that would put a token beside a
// client pair it was not minted (or given) for (nerdio handfix
// minted-token-bound-to-its-client, Codex review r1 class token bound to
// another client pair).
var ErrTokenPairNotStored = errors.New("token not saved: the credential store holds another client pair, and saved beside it this token would be sent for that pair (a pair from NERDIO_CLIENT_ID / NERDIO_CLIENT_SECRET is never written to disk). Pass the pair as --client-id and --client-secret to store it with its token, or unset NERDIO_CLIENT_ID and NERDIO_CLIENT_SECRET first; an environment pair also works unsaved, minting its own token on each run")

// refuseTokenBesideAnotherPair is the one gate every token save passes
// (SaveTokens: auth login, auth set-token, the automatic mint, the refresh
// grant): a token is saved only when the pair the files will hold after the
// save is the pair given with it, or no pair at all (a pairless token, the
// set-token shape, is never sent for a pair: client.tokenBinding mints for
// any active pair it was not saved with). An environment value never reaches
// the files (updateFileConfigField skips it), so a token minted for an
// environment pair was saved beside the stored pair and sent for it on the
// next run.
func (c *Config) refuseTokenBesideAnotherPair(clientID, clientSecret, accessToken, refreshToken string) error {
	if c == nil || c.fileConfig == nil || (accessToken == "" && refreshToken == "") {
		return nil
	}
	id, secret := clientID, clientSecret
	if c.envOverrides["ClientID"] {
		id = c.fileConfig.ClientID
	}
	if c.envOverrides["ClientSecret"] {
		secret = c.fileConfig.ClientSecret
	}
	if id == clientID && secret == clientSecret {
		return nil
	}
	if strings.TrimSpace(id) == "" && secret == "" {
		return nil // the files keep no pair: the token is stored pairless
	}
	return ErrTokenPairNotStored
}
