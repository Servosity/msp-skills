// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Avanan-specific credential persistence.
//
// The generated SaveCredential writes the application ID (the value
// AuthHeader() resolves). Avanan additionally needs the minted session token
// and the app-id/secret pair persisted independently, so those accessors live
// here rather than as edits to the generated config.

package config

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"avanan-pp-cli/internal/cliutil"
)

// notCredentials are Config's string fields that are settings or labels, not
// secrets. Every other exported string field, and every value of a string map
// field ([headers]), is a credential: a field a reprint adds is held until it
// is named here.
var notCredentials = map[string]bool{
	"BaseURL": true, "CACert": true, "TLSServerName": true,
	"AuthSource": true, "CredentialSource": true, "Path": true,
}

// HoldCredentials holds with the credential kit every credential value the
// program has from the config and the environment: each credential field of c (as parsed from a
// config or credentials file, or after Load applied every source), every
// [headers] value (sent on every request, so a server can echo it), and the
// credential env vars whether or not Load applied them. No length or shape
// exemption: a short value is held and withheld like a long one.
//
// Called by the hand-fix lines in parseConfigData (every file parse, values
// later refused included) and at Load's return, and by the savers below for
// the values they persist. c may be nil (environment only).
func HoldCredentials(c *Config) {
	values := []string{
		cliutil.EnvOverride("AVANAN_APP_ID"),
		cliutil.EnvOverride("AVANAN_CLIENT_SECRET"),
		cliutil.EnvOverride("AVANAN_TOKEN"),
	}
	if c != nil {
		v := reflect.ValueOf(c).Elem()
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if !field.IsExported() || notCredentials[field.Name] {
				continue
			}
			switch fv := v.Field(i); fv.Kind() {
			case reflect.String:
				values = append(values, fv.String())
			case reflect.Map:
				if fv.Type().Key().Kind() == reflect.String && fv.Type().Elem().Kind() == reflect.String {
					for it := fv.MapRange(); it.Next(); {
						values = append(values, it.Value().String())
						values = append(values, cliutil.HeldHeaderComponents(it.Value().String())...)
					}
				}
			}
		}
	}
	cliutil.Hold("config:unclassified", values...)
}

// holdConfigBytes holds, before a config file is parsed, every string it
// sets except the values of the non-credential settings (the toml keys of
// Config's notCredentials fields and of its fields that are not strings or
// string maps), so a file that fails to parse (a malformed legacy config is
// skipped with a warning) still holds what it carries, in any TOML form the
// loader accepts.
func holdConfigBytes(data []byte) {
	settings := map[string]bool{}
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
		if name == "" || name == "-" {
			continue
		}
		if notCredentials[f.Name] || f.Type.Kind() != reflect.String && f.Type.Kind() != reflect.Map {
			settings[strings.ToLower(name)] = true
		}
	}
	cliutil.HoldTOMLValues(data, func(key string) bool { return settings[key] })
}

// ReadPaths is every file Load and the credentials store could read: the
// config at explicit (else AVANAN_CONFIG, else the default), its credentials
// file, the legacy config, and the default credentials file.
func ReadPaths(explicit string) []string {
	var out []string
	add := func(p string, err error) {
		if err == nil && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if path, _, err := resolveConfigPath(explicit); err == nil {
		out = append(out, path)
		add(cliutil.CredentialsFilePathForConfig(path))
	}
	add(LegacyConfigPath())
	add(cliutil.CredentialsFilePath())
	return out
}

// SelectedBaseURL is the base URL Load settles on before the environment: the
// file Load reads (the explicit one, whose absent base_url means the built-in;
// never a discovered default beside it), else the built-in. It reads no credential
// and holds nothing, so it can run inside the kit's Files.
func SelectedBaseURL(configPath string) string {
	path, explicit, err := resolveConfigPath(configPath)
	if err != nil {
		return builtinBaseURL()
	}
	paths := []string{path}
	if legacy, err := LegacyConfigPath(); err == nil && !explicit {
		paths = append(paths, legacy)
	}
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue // missing: the next, as Load falls back
		}
		if fi.Mode().IsRegular() {
			if opened, err := cliutil.ReadOpenedConfig(p); err == nil {
				if base := baseURLFromConfigBytes(opened.Data); base != "" {
					return base
				}
			}
		}
		break
	}
	return builtinBaseURL()
}

// HoldFiles holds every credential string in each of paths, read the way Load
// reads a config (cliutil.ReadOpenedConfig; a loose mode still holds). A
// missing file, and anything but a regular file (a FIFO or a device: reading
// one here would block or drain it before Load), holds nothing.
func HoldFiles(paths []string) {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if opened, err := cliutil.ReadOpenedConfig(p); err == nil {
			holdConfigBytes(opened.Data)
		}
	}
}

// SaveAvananSession persists a minted session token and its expiry.
//
// The token is a short-lived derivative of the app-id/secret pair, so it is
// written alongside the other credential fields and cleared by logout.
func (c *Config) SaveAvananSession(token string, expiry time.Time) error {
	cliutil.Hold("minted:session-token", token)
	c.AvananToken = token
	c.TokenExpiry = expiry
	delete(c.envOverrides, "AvananToken")
	delete(c.envOverrides, "TokenExpiry")
	c.updateFileConfigField("AvananToken")
	c.updateFileConfigField("TokenExpiry")
	if err := c.saveCredentialsFirst(); err != nil {
		return err
	}
	return c.save()
}

// SaveAvananCredentials persists the application ID and client secret used to
// mint session tokens.
func (c *Config) SaveAvananCredentials(appID, secret string) error {
	cliutil.Hold("config:client_secret", appID, secret)
	if appID != "" {
		c.AvananAppId = appID
		delete(c.envOverrides, "AvananAppId")
		c.updateFileConfigField("AvananAppId")
	}
	if secret != "" {
		c.ClientSecret = secret
		delete(c.envOverrides, "ClientSecret")
		c.updateFileConfigField("ClientSecret")
	}
	if err := c.saveCredentialsFirst(); err != nil {
		return err
	}
	return c.save()
}

// AvananSessionToken returns the persisted session token, if any.
func (c *Config) AvananSessionToken() string {
	if c == nil {
		return ""
	}
	return c.AvananToken
}

// AvananAppID returns the configured application ID.
func (c *Config) AvananAppID() string {
	if c == nil {
		return ""
	}
	return c.AvananAppId
}

// AvananClientSecret returns the configured client secret.
func (c *Config) AvananClientSecret() string {
	if c == nil {
		return ""
	}
	return c.ClientSecret
}
