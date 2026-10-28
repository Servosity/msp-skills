// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"
	"strings"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/config"
)

// Client hook guarding which tenant and which host a request uses.
//
//  1. The Basic-auth token request and every bearer call go only to https hosts
//     (plain http is allowed for loopback test servers).
//  2. A saved profile belongs to one Zoom app (SaveTokens records its client_id,
//     and the local store is keyed by that app). When the environment names a
//     different app, the run is refused instead of silently re-pointing the
//     saved profile and its token at another tenant.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.Config == nil {
			return nil
		}
		if err := zoomRequireSecureURL("base URL", c.BaseURL); err != nil {
			return err
		}
		if err := zoomRequireSecureURL("token URL", c.Config.TokenURL); err != nil {
			return err
		}
		if err := zoomRefuseEnvAppMismatch(c.Config.ClientID); err != nil {
			return err
		}
		if err := zoomRefuseConflictingAppAliases(c.Config); err != nil {
			return err
		}
		// The client refuses it on every request; refusing here makes a run
		// exit non-zero instead of sync recording one failure per resource.
		if err := c.Config.ZoomRefuseAuthorizationOverride(); err != nil {
			return configErr(err)
		}
		return zoomRefuseEnvAccountMismatch(c.Config.ZoomFileAccountID(), c.Config.ClientID != "" || c.Config.AccessToken != "")
	})
}

// zoomRefuseEnvAccountMismatch refuses an environment account ID that differs
// from the saved profile's account while that profile still holds an app or a
// cached token: the token would silently act on the saved tenant.
func zoomRefuseEnvAccountMismatch(savedAccountID string, savedCredentials bool) error {
	envAcct := strings.TrimSpace(os.Getenv("ZOOM_PHONE_ACCOUNT_ID"))
	saved := strings.TrimSpace(savedAccountID)
	if envAcct == "" || saved == "" || envAcct == saved || !savedCredentials {
		return nil
	}
	return configErr(fmt.Errorf("ZOOM_PHONE_ACCOUNT_ID names a different Zoom account than the saved profile, whose cached token would act on the saved account instead; " +
		"for several Zoom accounts save one profile each with 'zoom-phone-cli accounts connect <name>' and use 'fleet' or eval \"$(zoom-phone-cli accounts path <name> --env)\", " +
		"or run 'zoom-phone-cli auth login' with all three ZOOM_PHONE_* variables to replace the saved profile"))
}

func zoomRequireSecureURL(label, raw string) error {
	if err := client.ZoomRequireSecureURL(label, raw); err != nil {
		return configErr(err)
	}
	return nil
}

// zoomRefuseConflictingAppAliases refuses a profile whose client_id (the app
// that mints) and phone_client_id name different Zoom apps: its token and its
// local store would belong to different tenants.
func zoomRefuseConflictingAppAliases(cfg *config.Config) error {
	if _, conflict := cfg.ZoomAppIdentity(); !conflict {
		return nil
	}
	return configErr(fmt.Errorf("the profile names two Zoom apps: client_id (%s...) and phone_client_id (%s...); keep one Server-to-Server app per profile "+
		"(remove phone_client_id from the config file, or run 'zoom-phone-cli auth login' with that app's credentials), "+
		"or save one profile per Zoom account with 'zoom-phone-cli accounts connect <name>'", zoomShortID(strings.TrimSpace(cfg.ClientID)), zoomShortID(strings.TrimSpace(cfg.ZoomPhoneClientId))))
}

func zoomRefuseEnvAppMismatch(savedClientID string) error {
	envID := strings.TrimSpace(os.Getenv("ZOOM_PHONE_CLIENT_ID"))
	saved := strings.TrimSpace(savedClientID)
	if envID == "" || saved == "" || envID == saved {
		return nil
	}
	return configErr(fmt.Errorf("ZOOM_PHONE_CLIENT_ID names a different Zoom app than the saved profile (%s...), so this run could mix two tenants' tokens and data; "+
		"for several Zoom accounts save one profile each with 'zoom-phone-cli accounts connect <name>' and use 'fleet' or eval \"$(zoom-phone-cli accounts path <name> --env)\", "+
		"or run 'zoom-phone-cli auth login' with all three ZOOM_PHONE_* variables to replace the saved profile", zoomShortID(saved)))
}

func zoomShortID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[:4]
}
