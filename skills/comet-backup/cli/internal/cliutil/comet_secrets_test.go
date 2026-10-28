// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"testing"

)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an API key's clientId, an access
// key ID) counts as a credential; a resource identifier does not (apiKeyId
// keeps matching "apikey", a signal the rule already had).
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"Password", "password", "TargetPassword", "NewPassword", "SessionKey", "session-key", "TOTP", "Token",
		"S3SecretKey", "s3_secret_key", "s3-secret-key", "S3AccessKey", "AZBAccountKey", "CometBucketKey",
		"ReadWriteKey", "AccessKey", "SecretKey", "AppKey", "PartnerKey", "Key", "EncryptedEncryptionKey",
		"TOTPKey", "SFTPPrivateKey", "SSHPrivateKey", "FTPPassword", "SFTPPassword", "SMBPassword",
		"LocalcopyWinSMBPassword", "SMTPPassword", "BindPassword", "PfxFilePassword", "AppleIDPass",
		"Passphrase", "PasswordHash", "PasswordRecovery", "ClientSecret", "WindowsCodeSignAzureAppSecret",
		"WindowsCodeSignPKCS12Password", "AuthToken", "APIKey", "api_key", "x-api-key",
	} {
		if !SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"id", "Username", "TargetUser", "DeviceID", "ProfileHash", "PolicyHash", "ServerLicenseHash",
		"SharedStorageQuotaHash", "KeyID", "Description", "S3Server", "S3BucketName",
		"password_complexity", "SnapshotID", "Destination", "SourceGUID",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}

