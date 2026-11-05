// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "strings"

// StoredLayer is the configuration the files recorded (config file and
// credential store) before any environment override: the layer a stored
// minted token was saved beside. A configuration Load did not build is its own
// record. appdirect handfix: minted-token-bound-to-its-client.
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
